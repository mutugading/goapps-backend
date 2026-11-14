-- [T003] rollback 04§11-12
DROP POLICY IF EXISTS p_div ON payroll.pyr_addition;
DROP POLICY IF EXISTS p_div ON payroll.pyr_employee;
DROP POLICY IF EXISTS p_div ON payroll.pyr_salary_head;
ALTER TABLE payroll.pyr_addition DISABLE ROW LEVEL SECURITY;
ALTER TABLE payroll.pyr_employee DISABLE ROW LEVEL SECURITY;
ALTER TABLE payroll.pyr_salary_head DISABLE ROW LEVEL SECURITY;
