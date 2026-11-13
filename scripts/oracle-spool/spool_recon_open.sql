-- [T027a] Spool gabungan untuk runbook recon periode OPEN (recon_open.sh): urutan ETL
-- employees -> inputs -> salary-legacy -> att-legacy -> att-legacy-fail -> thr-legacy.
-- Gabungan _spool_t019c + spool_t009d2_e2.sql + spool_thr.sql. Jalankan dari folder tujuan
-- yang berisi spool_one.sql: sqlplus ... @spool_recon_open.sql. Read-only, butuh SQL*Plus >= 12.2
-- (Instant Client 23, lihat memori oracle-sqlplus-env). Berkas 'DONE' ditulis paling akhir;
-- tidak ada DONE = spool terputus. HM_EMP_DATA bersifat opsional bagi loader (pe_hris_sys_id).
ALTER SESSION SET NLS_NUMERIC_CHARACTERS='.,';
ALTER SESSION SET NLS_DATE_FORMAT='YYYY-MM-DD HH24:MI:SS';
SET NUMWIDTH 40
-- employees
@spool_one.sql MGT_PAYROLL_EMP
@spool_one.sql MGT_PAYROLL_COMPNT_EMP
@spool_one.sql MGT_PAYROLL_COMPNT_MASTER
@spool_one.sql MGT_PAYROLL_COMPNT_TEMPLATE
@spool_one.sql HM_EMP_DATA
-- inputs
@spool_one.sql MGT_PAYROLL_ADDITION
@spool_one.sql MGT_PAYROLL_ABNORMAL_PREMI
@spool_one.sql MGT_PAYROLL_ATTD_DAYS
@spool_one.sql MGT_PAYROLL_SUBSIDI_PPH
@spool_one.sql MGT_PAYROLL_TAX_ADJ
@spool_one.sql MGT_PAYROLL_HOLIDAY
@spool_one.sql MGT_PAYROLL_BATAL_PREMI
-- salary-legacy + att-legacy-fail
@spool_one.sql MGT_PAYROLL_MAIN
@spool_one.sql MGT_PAYROLL_DETAIL
-- att-legacy
@spool_one.sql MGT_PAYROLL_ATT_REKAP
@spool_one.sql MGT_PAYROLL_ATT_REKAP_ADD
@spool_one.sql MGT_PAYROLL_SCH_REKAP
-- thr-legacy
@spool_one.sql MGT_THR_MAIN
@spool_one.sql MGT_THR_DETAIL
SPOOL DONE
PROMPT DONE
SPOOL OFF
EXIT
