DROP POLICY IF EXISTS p_etl_lock ON payroll.pyr_period;
DROP POLICY IF EXISTS p_etl_lock ON payroll.pyr_att_snapshot;
REVOKE SELECT ON payroll.pyr_period, payroll.pyr_att_snapshot FROM payroll_etl;
