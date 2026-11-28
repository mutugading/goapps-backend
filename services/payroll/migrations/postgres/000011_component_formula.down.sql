-- [T003] rollback 04§15
DROP TABLE IF EXISTS payroll.pyr_run_formula, payroll.pyr_formula_test_case, payroll.pyr_formula_dependency, payroll.pyr_formula_version CASCADE;
ALTER TABLE payroll.pyr_salary_detail DROP COLUMN IF EXISTS psd_component_sysid, DROP COLUMN IF EXISTS psd_currency, DROP COLUMN IF EXISTS psd_amount_base, DROP COLUMN IF EXISTS psd_formula_version_id, DROP COLUMN IF EXISTS psd_meta;
ALTER TABLE payroll.pyr_employee_component DROP COLUMN IF EXISTS pec_source, DROP COLUMN IF EXISTS pec_component_sysid;
ALTER TABLE payroll.pyr_component_template DROP COLUMN IF EXISTS pct_component_sysid;
DROP TABLE IF EXISTS payroll.pyr_component_legacy_map, payroll.pyr_component CASCADE;
DROP EXTENSION IF EXISTS btree_gist;
