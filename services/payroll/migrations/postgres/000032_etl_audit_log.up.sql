-- [T009e4] ETL salary-legacy mencatat audit per periode yang dibuat & di-CLOSE (04§9, aturan CONTEXT §4).
-- payroll_etl hanya INSERT (append-only, tidak bisa membaca/mengubah log) dan hanya aksi ETL_*.
GRANT INSERT ON payroll.pyr_audit_log TO payroll_etl;
GRANT USAGE ON SEQUENCE payroll.pyr_audit_log_pal_id_seq TO payroll_etl;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_audit_log;
CREATE POLICY p_etl_ins ON payroll.pyr_audit_log FOR INSERT TO payroll_etl
  WITH CHECK (pal_actor = 'etl' AND pal_action LIKE 'ETL\_%');
