#!/usr/bin/env bash
# [T027a] Runbook recon periode OPEN: dump -> ETL -> recon diff (D-44 C, D-46, D-47, D-52).
# Pemakaian (dari container tools, folder services/payroll):
#   scripts/recon/recon_open.sh -yymm 202611 -dump <dir> -out /tmp/<tag> [-company 3] [-print]
# Env: PAYROLL_ETL_DSN (role payroll_etl), PAYROLL_RO_DSN (payroll_ro), PAYROLL_PII_KEY, PAYROLL_PII_HMAC_KEY.
# TIDAK menjalankan sync att/snapshot (menggeser snapshot LEGACY, D-39); langkah D-61 ada di T027b.
# Berhenti pada error pertama; log per langkah di <out>/run.log; stdout tanpa NIK/nominal.
# recon diff keluar 1 = tidak paritas (hasil, bukan error): dilanjutkan ke divisi berikut, exit akhir 1.
set -euo pipefail
yymm=""; dump=""; out=""; company=1; print=0
while [ $# -gt 0 ]; do
  case "$1" in
    -yymm) yymm=${2:?}; shift 2;;
    -dump) dump=${2:?}; shift 2;;
    -out) out=${2:?}; shift 2;;
    -company) company=${2:?}; shift 2;;
    -print) print=1; shift;;
    *) echo "flag tidak dikenal: $1" >&2; exit 2;;
  esac
done
[[ "$yymm" =~ ^[0-9]{6}$ ]] || { echo "-yymm YYYYMM wajib" >&2; exit 2; }
[ -n "$dump" ] && [ -n "$out" ] || { echo "-dump dan -out wajib" >&2; exit 2; }
here=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)   # services/payroll
SYNC="go run ./cmd/sync"; RECON="go run ./cmd/recon"
steps=(
  "employees|$SYNC employees -dir $dump -dsn \$PAYROLL_ETL_DSN -company $company"
  "inputs|$SYNC inputs -dir $dump -dsn \$PAYROLL_ETL_DSN -company $company"
  "salary-legacy|$SYNC salary-legacy -dir $dump -dsn \$PAYROLL_ETL_DSN -company $company"
  "att-legacy|$SYNC att-legacy -dir $dump -dsn \$PAYROLL_ETL_DSN -company $company"
  "att-legacy-fail|$SYNC att-legacy-fail -dir $dump -dsn \$PAYROLL_ETL_DSN -open-periods $yymm"
  "thr-legacy|$SYNC thr-legacy -dir $dump -dsn \$PAYROLL_ETL_DSN -company $company"
)
for d in SOLO JAKARTA; do
  steps+=("recon-$d|$RECON diff -yymm $yymm -division $d -company $company -target dry -rule-set 1 -out $out/recon_$d.json -xlsx $out/recon_$d.xlsx -dsn \$PAYROLL_RO_DSN")
done
if [ "$print" = 1 ]; then
  echo "# cd $here; check_spool.sh $dump open"
  for s in "${steps[@]}"; do echo "[${s%%|*}] ${s#*|}"; done
  exit 0
fi
cd "$here"
mkdir -p "$out"; : > "$out/run.log"
bash "$here/../../scripts/oracle-spool/check_spool.sh" "$dump" open >>"$out/run.log" 2>&1 || { echo "check_spool gagal; lihat $out/run.log" >&2; exit 1; }
: "${PAYROLL_ETL_DSN:?}" "${PAYROLL_RO_DSN:?}"
rc_final=0
for s in "${steps[@]}"; do
  name=${s%%|*}; cmd=${s#*|}
  echo "== $name $(date +%T)" | tee -a "$out/run.log"
  set +e; eval "$cmd" >>"$out/run.log" 2>&1; rc=$?; set -e
  if [ "$rc" != 0 ]; then
    if [[ "$name" == recon-* && "$rc" == 1 ]]; then echo "   tidak paritas ($name), lihat laporan di $out" | tee -a "$out/run.log"; rc_final=1; continue; fi
    echo "GAGAL di $name (exit $rc); lihat $out/run.log" >&2; exit "$rc"
  fi
done
echo "selesai; laporan di $out (exit $rc_final)"
exit $rc_final
