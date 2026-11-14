DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_salary_detail;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_salary_head;
DROP POLICY IF EXISTS p_etl_upd ON payroll.pyr_period;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_period;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_run;
DROP POLICY IF EXISTS p_etl_sel ON payroll.pyr_run;
DELETE FROM payroll.pyr_rule_set WHERE prs_id = 0 AND NOT EXISTS (SELECT 1 FROM payroll.pyr_run WHERE pr_rule_set_id = 0);
ALTER TABLE payroll.pyr_run DROP COLUMN IF EXISTS pr_kind;
