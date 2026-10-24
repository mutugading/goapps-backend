DROP POLICY IF EXISTS p_etl ON payroll.pyr_component;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_component_legacy_map;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_component_template;
REVOKE ALL ON payroll.pyr_component, payroll.pyr_component_legacy_map, payroll.pyr_component_template,
  payroll.pyr_level_rule, payroll.pyr_company, payroll.pyr_division_ref, payroll.pyr_currency FROM payroll_etl;
DROP TABLE IF EXISTS payroll.pyr_migration_log;
REVOKE USAGE ON SCHEMA payroll FROM payroll_etl;
