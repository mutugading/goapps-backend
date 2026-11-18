-- [T033f] Balikan 000047.
ALTER TABLE payroll.pyr_component_template DROP CONSTRAINT IF EXISTS ck_pct_tax;
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT IF EXISTS ck_pec_tax;
