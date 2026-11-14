DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot;
DROP INDEX IF EXISTS payroll.uix_pas_legacy;
COMMENT ON COLUMN pyr_att_snapshot_row.pasr_kind IS 'REKAP | ADD';
