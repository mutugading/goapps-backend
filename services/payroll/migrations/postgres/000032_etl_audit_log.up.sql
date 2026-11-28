-- [T009e4] ETL salary-legacy mencatat audit per periode yang dibuat & di-CLOSE (04§9, aturan CONTEXT §4).
-- payroll_etl hanya INSERT (append-only, tidak bisa membaca/mengubah log) dan hanya aksi ETL_*.
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_audit_log;
CREATE POLICY p_etl_ins ON payroll.pyr_audit_log FOR INSERT TO payroll_etl
  WITH CHECK (pal_actor = 'etl' AND pal_action LIKE 'ETL\_%');
