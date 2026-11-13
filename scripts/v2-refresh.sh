#!/usr/bin/env bash
# v2-refresh: refresh the LOCAL v2 Postgres (localhost:5440) from SANITIZED staging data (P9).
#
# Safety rules:
#   - the source DB comment must start with 'sanitized p9' (set by the sanitize step on staging);
#     otherwise refuse: raw/unsanitized data must never reach the laptop.
#   - the target must be 127.0.0.1/localhost:5440 (the docker-compose.v2.yaml container).
#   - the source password comes from ~/.pgpass only (never on the command line / env).
#   - dumps live in a mktemp dir that is removed on exit.
#
# Env:
#   STAGING_PG_HOST   source host            (default 192.168.0.60)
#   STAGING_PG_PORT   source port            (default 5432)
#   STAGING_PG_USER   READ-ONLY source role  (required)
#   LOCAL_PG_HOST / LOCAL_PG_PORT            (must be 127.0.0.1|localhost / 5440)
#   V2_PG_CONTAINER   local container name   (default goapps-v2-postgres)
set -euo pipefail

SRC_HOST="${STAGING_PG_HOST:-192.168.0.60}"
SRC_PORT="${STAGING_PG_PORT:-5432}"
SRC_USER="${STAGING_PG_USER:-}"
LOCAL_HOST="${LOCAL_PG_HOST:-127.0.0.1}"
LOCAL_PORT="${LOCAL_PG_PORT:-5440}"
CONTAINER="${V2_PG_CONTAINER:-goapps-v2-postgres}"
SERVICES=(iam finance ppc)
PGPASS_FILE="${PGPASSFILE:-$HOME/.pgpass}"

die() { echo "v2-refresh: $*" >&2; exit 1; }

[[ -n "$SRC_USER" ]] || die "STAGING_PG_USER is required (a read-only role), e.g. STAGING_PG_USER=p9_ro make v2-refresh"
case "$LOCAL_HOST" in 127.0.0.1|localhost) ;; *) die "refusing: local target host must be 127.0.0.1/localhost, got '$LOCAL_HOST'" ;; esac
[[ "$LOCAL_PORT" == "5440" ]] || die "refusing: local target port must be 5440, got '$LOCAL_PORT'"
[[ -f "$PGPASS_FILE" ]] || die "$PGPASS_FILE not found; add '$SRC_HOST:$SRC_PORT:*:$SRC_USER:<password>' (chmod 600)"
grep -q "^\(${SRC_HOST}\|\*\):\(${SRC_PORT}\|\*\):" "$PGPASS_FILE" || die "no matching entry for $SRC_HOST in $PGPASS_FILE"

export DOCKER_HOST="${DOCKER_HOST:-unix://$HOME/.colima/default/docker.sock}"
docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q true \
  || die "container $CONTAINER is not running (make v2-up)"

# Client selection: host PG18 clients if present, else run them inside the v2 container.
host_pg18() { command -v pg_dump >/dev/null && command -v pg_restore >/dev/null && command -v psql >/dev/null \
  && [[ "$(pg_dump --version | sed -E 's/.* ([0-9]+).*/\1/')" -ge 18 ]]; }

WORK="$(mktemp -d -t v2-refresh.XXXXXX)"
PGPASS_IN_CONTAINER=/tmp/.pgpass-v2-refresh
cleanup() {
  rm -rf "$WORK"
  docker exec -u root "$CONTAINER" rm -f "$PGPASS_IN_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if host_pg18; then
  MODE=host
  export PGPASSFILE="$PGPASS_FILE"
  src_psql()  { psql -X -h "$SRC_HOST" -p "$SRC_PORT" -U "$SRC_USER" -w "$@"; }
  src_dump()  { pg_dump -h "$SRC_HOST" -p "$SRC_PORT" -U "$SRC_USER" -w -Fc "$@"; }
  loc_psql()  { PGPASSFILE=/dev/null PGPASSWORD=dev_postgres psql -X -q -v ON_ERROR_STOP=1 -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U postgres "$@"; }
  loc_restore() { PGPASSWORD=dev_postgres pg_restore -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U postgres "$@"; }
else
  MODE=container
  # stage ~/.pgpass inside the container (removed by the trap); password never on a cmdline
  docker cp "$PGPASS_FILE" "$CONTAINER:$PGPASS_IN_CONTAINER" >/dev/null
  docker exec -u root "$CONTAINER" sh -c "chown postgres:postgres $PGPASS_IN_CONTAINER && chmod 600 $PGPASS_IN_CONTAINER"
  DX=(docker exec -i -e "PGPASSFILE=$PGPASS_IN_CONTAINER" "$CONTAINER")
  src_psql()  { "${DX[@]}" psql -X -h "$SRC_HOST" -p "$SRC_PORT" -U "$SRC_USER" -w "$@"; }
  src_dump()  { "${DX[@]}" pg_dump -h "$SRC_HOST" -p "$SRC_PORT" -U "$SRC_USER" -w -Fc "$@"; }
  loc_psql()  { docker exec -i "$CONTAINER" psql -X -q -v ON_ERROR_STOP=1 -U postgres "$@"; }
  loc_restore() { docker exec -i "$CONTAINER" pg_restore -U postgres "$@"; }
fi
echo "v2-refresh: source=$SRC_USER@$SRC_HOST:$SRC_PORT target=$LOCAL_HOST:$LOCAL_PORT clients=$MODE tmp=$WORK"

# 1) preflight ALL sources before touching anything
for s in "${SERVICES[@]}"; do
  db="${s}_db"
  comment="$(src_psql -d postgres -Atc "SELECT coalesce(shobj_description(oid,'pg_database'),'') FROM pg_database WHERE datname='${db}'")" \
    || die "cannot read comment of ${db} on source"
  [[ "$comment" == "sanitized p9"* ]] \
    || die "REFUSING ${db}: database comment is not 'sanitized p9...' (got: '${comment}'). Raw data must never reach the laptop."
done

# 2) per DB: dump -> restore into <db>_refresh -> swap
for s in "${SERVICES[@]}"; do
  db="${s}_db"; new="${db}_refresh"; old="${db}_old"
  echo "== ${db}: dump"
  src_dump -d "$db" > "$WORK/${db}.dump"
  echo "== ${db}: restore into ${new}"
  loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${new}" -c "DROP DATABASE IF EXISTS ${old}" \
           -c "CREATE DATABASE ${new} OWNER ${s}_owner"
  if ! loc_restore --no-owner --no-acl --role="${s}_owner" -d "$new" < "$WORK/${db}.dump"; then
    loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${new}" >/dev/null || true
    die "pg_restore of ${db} reported errors; local ${db} left untouched"
  fi
  echo "== ${db}: swap"
  loc_psql -d postgres \
    -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('${db}','${new}') AND pid <> pg_backend_pid()" \
    -c "ALTER DATABASE ${db} RENAME TO ${old}" \
    -c "ALTER DATABASE ${new} RENAME TO ${db}" \
    -c "DROP DATABASE ${old}"
  rm -f "$WORK/${db}.dump"
done

echo "v2-refresh: done (dumps removed). Next: make v2-migrate  (re-applies pending migrations and grants)"
