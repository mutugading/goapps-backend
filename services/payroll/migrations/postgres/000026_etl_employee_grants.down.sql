DROP POLICY IF EXISTS p_etl ON payroll.pyr_employee;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_employee_nik_hist;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_employee_component;
REVOKE ALL ON payroll.pyr_employee, payroll.pyr_employee_nik_hist, payroll.pyr_employee_component FROM payroll_etl;
