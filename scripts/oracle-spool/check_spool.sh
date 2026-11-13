#!/usr/bin/env bash
# Cek hasil spool: jumlah baris data (tanpa header) dan konsistensi jumlah kolom. Pemakaian: check_spool.sh <dir> [thr|open]
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
if [ "${2:-}" = open ]; then  # [T027a] spool_recon_open.sql: cetak jumlah baris + kolom rusak, tanpa angka tetap
  rc=0
  [ -f "$d/DONE" ] && echo "OK     DONE" || { echo "KURANG DONE (spool terputus?)"; rc=1; }
  for t in MGT_PAYROLL_EMP MGT_PAYROLL_COMPNT_EMP MGT_PAYROLL_COMPNT_MASTER MGT_PAYROLL_COMPNT_TEMPLATE \
    MGT_PAYROLL_ADDITION MGT_PAYROLL_ABNORMAL_PREMI MGT_PAYROLL_ATTD_DAYS MGT_PAYROLL_SUBSIDI_PPH \
    MGT_PAYROLL_TAX_ADJ MGT_PAYROLL_HOLIDAY MGT_PAYROLL_BATAL_PREMI MGT_PAYROLL_MAIN MGT_PAYROLL_DETAIL \
    MGT_PAYROLL_ATT_REKAP MGT_PAYROLL_ATT_REKAP_ADD MGT_PAYROLL_SCH_REKAP MGT_THR_MAIN MGT_THR_DETAIL HM_EMP_DATA; do
    f="$d/$t.psv"
    if [ ! -f "$f" ]; then
      if [ "$t" = HM_EMP_DATA ]; then echo "ada?   $t (opsional)"; else echo "KURANG $t"; rc=1; fi
      continue
    fi
    n=$(( $(wc -l < "$f") - 1 ))
    bad=$(awk -F'|' 'NR==1{c=NF;next} NF!=c{b++} END{print b+0}' "$f")
    st=OK; [ "$n" -le 0 ] && { st="KOSONG"; rc=1; }; [ "$bad" != 0 ] && { st="$st KOLOM-RUSAK=$bad"; rc=1; }
    echo "$st  $t baris=$n"
  done
  exit $rc
fi
chk MGT_PAYROLL_ATT_REKAP 70093
chk MGT_PAYROLL_ATT_REKAP_ADD 870
chk MGT_PAYROLL_SCH_REKAP 3996
chk MGT_PAYROLL_MAIN 93083
chk MGT_PAYROLL_DETAIL 2301039
[ -f "$d/MGT_PAYROLL_COMPNT_EMP.psv" ] && echo "ada   MGT_PAYROLL_COMPNT_EMP" || echo "KURANG MGT_PAYROLL_COMPNT_EMP"
