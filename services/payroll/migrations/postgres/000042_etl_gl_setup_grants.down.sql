DROP POLICY IF EXISTS p_etl ON payroll.pyr_gl_setup;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_gl_group_map;
REVOKE SELECT, INSERT, UPDATE ON payroll.pyr_gl_setup, payroll.pyr_gl_group_map FROM payroll_etl;
