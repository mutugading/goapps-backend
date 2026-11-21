DROP POLICY IF EXISTS p_etl ON payroll.pyr_component;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_component_legacy_map;
DROP POLICY IF EXISTS p_etl ON payroll.pyr_component_template;
DROP TABLE IF EXISTS payroll.pyr_migration_log;
