DROP POLICY IF EXISTS p_etl ON payroll.pyr_subsidy_rule;
REVOKE SELECT, INSERT, UPDATE ON payroll.pyr_subsidy_rule FROM payroll_etl;
