DROP POLICY IF EXISTS p_etl_sel ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_upd ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_del ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot_row;
CREATE POLICY p_etl ON payroll.pyr_att_snapshot_row TO payroll_etl USING (true) WITH CHECK (true);

DROP TABLE IF EXISTS payroll.pyr_component_template_owner;
