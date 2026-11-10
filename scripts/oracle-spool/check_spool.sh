#!/usr/bin/env bash
# Cek hasil spool: jumlah baris data (tanpa header) dan konsistensi jumlah kolom. Pemakaian: check_spool.sh <dir> [thr]
set -euo pipefail
d=${1:?dir}
chk() { f="$d/$1.psv"; [ -f "$f" ] || { echo "KURANG  $1"; return; }
  n=$(( $(wc -l < "$f") - 1 ))
  bad=$(awk -F'|' 'NR==1{c=NF;next} NF!=c{b++} END{print b+0}' "$f")
  st=OK; [ "$n" != "$2" ] && st="BEDA(harap $2)"; [ "$bad" != 0 ] && st="$st KOLOM-RUSAK=$bad"
  echo "$st  $1 baris=$n"; }
if [ "${2:-}" = thr ]; then  # [T024b] spool THR: check_spool.sh <dir> thr
  chk MGT_THR_MAIN 14917     # dump 2026-11 diterima user (T024c); spec ±14.918/±62.246 beda, diterima
  chk MGT_THR_DETAIL 62275
  exit 0
fi
chk MGT_PAYROLL_ATT_REKAP 70093
chk MGT_PAYROLL_ATT_REKAP_ADD 870
chk MGT_PAYROLL_SCH_REKAP 3996
chk MGT_PAYROLL_MAIN 93083
chk MGT_PAYROLL_DETAIL 2301039
[ -f "$d/MGT_PAYROLL_COMPNT_EMP.psv" ] && echo "ada   MGT_PAYROLL_COMPNT_EMP" || echo "KURANG MGT_PAYROLL_COMPNT_EMP"
