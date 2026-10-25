DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot;
DROP INDEX IF EXISTS payroll.uix_pas_legacy;
REVOKE INSERT, UPDATE ON payroll.pyr_att_snapshot, payroll.pyr_att_snapshot_row FROM payroll_etl;
REVOKE SELECT ON payroll.pyr_att_snapshot_row FROM payroll_etl;
COMMENT ON COLUMN payroll.pyr_att_snapshot_row.pasr_kind IS 'REKAP | ADD';
