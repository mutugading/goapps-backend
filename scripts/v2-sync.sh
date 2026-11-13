#!/usr/bin/env bash
# v2-sync: refresh the LOCAL v2 Postgres (container goapps-v2-postgres, 127.0.0.1:5440) from the weekly
# SANITIZED developer snapshot published on the staging DB server (see docs/local-db-sync.md).
# Works with bash 3.2 (macOS), Ubuntu and WSL2. No host Postgres tools needed: restore runs in the container.
#
# Usage: scripts/v2-sync.sh [--profile slim|full] [--dry-run] [--yes] [--keep-cache] [-h]
#   --dry-run     download + verify only, no restore
#   --yes         do not ask for confirmation
#   --keep-cache  keep older snapshots in the cache too (default: keep only the latest one)
# Env (or gitignored .env.devsnap in the repo root):
#   DEVSNAP_HOST (192.168.0.60)  DEVSNAP_USER (devsync)  DEVSNAP_KEY (optional ssh key path)
#   DEVSNAP_PROFILE (slim)       DEVSNAP_CACHE (${XDG_CACHE_HOME:-$HOME/.cache}/goapps/devsnap)
#   V2_PG_CONTAINER (goapps-v2-postgres)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ -f "$REPO_ROOT/.env.devsnap" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$REPO_ROOT/.env.devsnap"
  set +a
fi

HOST="${DEVSNAP_HOST:-192.168.0.60}"
RUSER="${DEVSNAP_USER:-devsync}"
KEY="${DEVSNAP_KEY:-}"
PROFILE="${DEVSNAP_PROFILE:-slim}"
CACHE="${DEVSNAP_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/goapps/devsnap}"
CONTAINER="${V2_PG_CONTAINER:-goapps-v2-postgres}"
SERVICES="iam finance ppc"
DRY=0; YES=0; KEEP_CACHE=0

die()  { echo "v2-sync: ERROR: $*" >&2; exit 1; }
step() { echo; echo "[$1/$TOTAL] $2"; }
TOTAL=7

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) shift; PROFILE="${1:-}" ;;
    --profile=*) PROFILE="${1#--profile=}" ;;
    --dry-run) DRY=1 ;;
    --yes|-y) YES=1 ;;
    --keep-cache) KEEP_CACHE=1 ;;
    -h|--help) sed -n 2,13p "$0"; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
  shift
done
case "$PROFILE" in slim|full) ;; *) die "profile must be slim or full, got '$PROFILE'" ;; esac

if [ -z "${DOCKER_HOST:-}" ] && [ -S "$HOME/.colima/default/docker.sock" ]; then
  export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/v2-sync.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT
trap 'echo; echo "v2-sync: interrupted (partial downloads are kept in the cache and will resume)" >&2; exit 130' INT TERM

# ---------------------------------------------------------------- 1. preflight
step 1 "preflight (profile=$PROFILE, source=$RUSER@$HOST)"
SHA=""
if command -v sha256sum >/dev/null 2>&1; then SHA="sha256sum"
elif command -v shasum >/dev/null 2>&1; then SHA="shasum -a 256"; fi
for t in docker ssh rsync; do command -v "$t" >/dev/null 2>&1 || die "required tool missing: $t"; done
[ -n "$SHA" ] || die "need sha256sum or shasum"

docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q true \
  || die "container $CONTAINER is not running. Start it with: make v2-up   (Docker running? DOCKER_HOST=${DOCKER_HOST:-default})"

SSH_OPTS="-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4"
if [ -n "$KEY" ]; then SSH_OPTS="$SSH_OPTS -i $KEY -o IdentitiesOnly=yes"; fi
SSH_CMD="ssh $SSH_OPTS"
REMOTE="$RUSER@$HOST"

# Fetch MANIFEST + SHA256SUMS (also the reachability test)
mkdir -p "$WORK/meta"
if ! rsync -q -e "$SSH_CMD" "$REMOTE:MANIFEST" "$REMOTE:SHA256SUMS" "$WORK/meta/" 2>"$WORK/rsync.err"; then
  sed 's/^/  /' "$WORK/rsync.err" >&2 || true
  if grep -qi "permission denied" "$WORK/rsync.err"; then
    die "permission denied (publickey): your key is not authorised. Send your PUBLIC key to the admin (DEVSNAP_KEY=path to pick another key)."
  fi
  die "cannot fetch snapshot from $HOST. Are you on the office network or VPN? Is the snapshot built yet (Sunday ~09:30 WIB)?"
fi

# ---------------------------------------------------------------- 2. manifest
step 2 "read MANIFEST"
MAN="$WORK/meta/MANIFEST"
SRC_RUN="$(sed -n 's/^source_run=//p' "$MAN" | head -n1)"
case "$SRC_RUN" in ""|*[!A-Za-z0-9._-]*) die "MANIFEST has missing/unsafe source_run '$SRC_RUN'" ;; esac
echo "  source_run: $SRC_RUN   created: $(sed -n 's/^created=//p' "$MAN" | head -n1)"

# Sanity gate #1: every db comment must be sanitized p9
for s in $SERVICES; do
  c="$(sed -n "s/^comment\.${s}_db=//p" "$MAN" | head -n1)"
  case "$c" in
    "sanitized p9"*) ;;
    *) die "REFUSING ${s}_db: MANIFEST comment is not 'sanitized p9...' (got: '$c'). Raw data must never reach a laptop." ;;
  esac
done
echo "  sanitized p9 comments: ok"

# files of the chosen profile: lines "file.<profile>=<path> <size>"
FILES=""; TOTAL_BYTES=0
while read -r path size; do
  [ -n "$path" ] || continue
  case "$path" in */*.dump) ;; *) die "unexpected file in MANIFEST: $path" ;; esac
  case "$path" in *..*|/*) die "unsafe path in MANIFEST: $path" ;; esac
  FILES="$FILES $path"; TOTAL_BYTES=$((TOTAL_BYTES + size))
done <<EOT
$(sed -n "s/^file\.${PROFILE}=//p" "$MAN")
EOT
[ -n "$FILES" ] || die "profile '$PROFILE' is not in this snapshot"
for s in $SERVICES; do
  case "$FILES" in *"$PROFILE/${s}_db.dump"*) ;; *) die "snapshot lacks $PROFILE/${s}_db.dump" ;; esac
done
echo "  download size: $((TOTAL_BYTES / 1048576)) MiB"

# Disk space: restored DB is larger than the compressed dump; require 3x
RUN_DIR="$CACHE/$SRC_RUN"
mkdir -p "$RUN_DIR/$PROFILE"
FREE_KB="$(df -Pk "$RUN_DIR" | awk 'NR==2{print $4}')"
NEED_KB=$((TOTAL_BYTES * 3 / 1024))
[ "$FREE_KB" -ge "$NEED_KB" ] || die "not enough disk in $CACHE: free $((FREE_KB / 1024)) MiB, need ~$((NEED_KB / 1024)) MiB (3x snapshot). Free some space or set DEVSNAP_CACHE."

cp "$WORK/meta/MANIFEST" "$WORK/meta/SHA256SUMS" "$RUN_DIR/"

# ---------------------------------------------------------------- 3. download
step 3 "download (resumable; cached files that already match are skipped)"
RSYNC_APPEND="--append-verify"
rsync --help 2>&1 | grep -q -- "--append-verify" || RSYNC_APPEND="--append"

sum_of() { # path-relative-to-RUN_DIR -> expected sha
  awk -v p="$1" '$2==p || $2=="*"p {print $1}' "$RUN_DIR/SHA256SUMS"
}
file_ok() {
  local want got
  want="$(sum_of "$1")"; [ -n "$want" ] || return 1
  [ -f "$RUN_DIR/$1" ] || return 1
  got="$($SHA "$RUN_DIR/$1" | awk '{print $1}')"
  [ "$got" = "$want" ]
}
for f in $FILES; do
  if file_ok "$f"; then echo "  cached ok: $f"; continue; fi
  echo "  fetching: $f"
  # shellcheck disable=SC2086
  rsync -e "$SSH_CMD" --partial $RSYNC_APPEND --progress "$REMOTE:$f" "$RUN_DIR/$PROFILE/" \
    || die "download of $f failed (VPN dropped?). Re-run to resume."
done

# ---------------------------------------------------------------- 4. verify
step 4 "verify sha256"
for f in $FILES; do
  if ! file_ok "$f"; then
    rm -f "$RUN_DIR/$f"
    die "checksum mismatch for $f (removed from cache). Re-run make v2-sync; if it persists, tell the admin."
  fi
  echo "  ok: $f"
done

# prune old snapshots
if [ "$KEEP_CACHE" -eq 0 ]; then
  for d in "$CACHE"/*/; do
    [ -d "$d" ] || continue
    [ "$(basename "$d")" = "$SRC_RUN" ] || { echo "  prune old cache: $d"; rm -rf "$d"; }
  done
fi

if [ "$DRY" -eq 1 ]; then
  echo; echo "v2-sync: --dry-run: download + verify done ($RUN_DIR). No restore performed."
  exit 0
fi

# ---------------------------------------------------------------- 5. confirm
step 5 "confirm"
if [ "$YES" -ne 1 ]; then
  if [ -t 0 ]; then
    printf '  This REPLACES your local iam_db, finance_db and ppc_db (container %s) with snapshot %s. Continue? [y/N] ' "$CONTAINER" "$SRC_RUN"
    read -r ans
    case "$ans" in y|Y|yes|YES) ;; *) die "aborted by user" ;; esac
  else
    die "stdin is not a terminal; pass --yes to confirm the replace of local iam_db/finance_db/ppc_db"
  fi
fi

# ---------------------------------------------------------------- 6. restore
step 6 "restore into <db>_refresh and swap"
loc_psql() { docker exec -i "$CONTAINER" psql -X -q -v ON_ERROR_STOP=1 -U postgres "$@"; }
for s in $SERVICES; do
  db="${s}_db"; new="${db}_refresh"; old="${db}_old"
  echo "== $db: restore into $new"
  loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${new}" -c "DROP DATABASE IF EXISTS ${old}" \
           -c "CREATE DATABASE ${new} OWNER ${s}_owner"
  if ! docker exec -i "$CONTAINER" pg_restore -U postgres --no-owner --no-acl --role="${s}_owner" -d "$new" \
        < "$RUN_DIR/$PROFILE/${db}.dump"; then
    loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${new}" >/dev/null || true
    die "pg_restore of $db reported errors; local $db left untouched"
  fi
  echo "== $db: swap"
  loc_psql -d postgres \
    -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('${db}','${new}') AND pid <> pg_backend_pid()" \
    -c "ALTER DATABASE ${db} RENAME TO ${old}" \
    -c "ALTER DATABASE ${new} RENAME TO ${db}" \
    -c "DROP DATABASE ${old}"
done

# ---------------------------------------------------------------- 7. migrate + report
step 7 "make v2-migrate (pending migrations + grants)"
if [ "${V2_SYNC_SKIP_MIGRATE:-0}" = "1" ]; then
  echo "  skipped (V2_SYNC_SKIP_MIGRATE=1)"
else
  make -C "$REPO_ROOT" --no-print-directory v2-migrate \
    || die "restore done but v2-migrate failed (is the 'migrate' CLI installed?). Fix and run: make v2-migrate"
fi

echo
echo "schema_migrations versions:"
for s in $SERVICES; do
  v="$(loc_psql -d "${s}_db" -Atc "SELECT version || CASE WHEN dirty THEN ' (DIRTY)' ELSE '' END FROM schema_migrations_${s} LIMIT 1" 2>/dev/null || echo '?')"
  echo "  $s: $v"
done
echo "source_run: $SRC_RUN  (profile: $PROFILE)"
echo
echo "NOTE: application logins in the synced data now use the shared STAGING password"
echo "      (password manager entry: goapps/staging/app-default-password)."
