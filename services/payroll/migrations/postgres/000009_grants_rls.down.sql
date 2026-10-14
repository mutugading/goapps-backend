-- [T003] rollback 04§11-12
DROP POLICY IF EXISTS p_div ON payroll.pyr_addition;
DROP POLICY IF EXISTS p_div ON payroll.pyr_employee;
DROP POLICY IF EXISTS p_div ON payroll.pyr_salary_head;
ALTER TABLE payroll.pyr_addition DISABLE ROW LEVEL SECURITY;
ALTER TABLE payroll.pyr_employee DISABLE ROW LEVEL SECURITY;
ALTER TABLE payroll.pyr_salary_head DISABLE ROW LEVEL SECURITY;
REVOKE ALL ON ALL TABLES IN SCHEMA payroll FROM payroll_app, payroll_ro;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA payroll FROM payroll_app;
REVOKE ALL ON ALL TABLES IN SCHEMA payroll_stg FROM payroll_app, payroll_ro, payroll_etl;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA payroll_stg FROM payroll_etl;
