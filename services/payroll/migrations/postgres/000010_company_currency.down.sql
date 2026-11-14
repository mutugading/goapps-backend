-- [T003] rollback 04§14
DROP TABLE IF EXISTS payroll.pyr_run_fx, payroll.pyr_fx_rate CASCADE;
ALTER TABLE payroll.pyr_run DROP COLUMN IF EXISTS pr_company_id;
ALTER TABLE payroll.pyr_employee DROP COLUMN IF EXISTS pe_company_id, DROP COLUMN IF EXISTS pe_pay_currency, DROP COLUMN IF EXISTS pe_employment_status, DROP COLUMN IF EXISTS pe_cost_group;
ALTER DOMAIN payroll.division ADD CONSTRAINT division_check CHECK (VALUE IN ('SOLO','JAKARTA'));
DROP TABLE IF EXISTS payroll.pyr_division_ref, payroll.pyr_company, payroll.pyr_currency CASCADE;
