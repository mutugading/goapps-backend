-- [T009a] Jalur ETL master komponen (D-22): role payroll_etl menulis pyr_component, legacy_map,
-- template, level_rule, dan mencatat hasil ke pyr_migration_log (08§1, 08§6). Role ETL tidak
-- terikat scope (p_etl, pola 000017); service tetap lewat payroll_app dengan RLS.
CREATE TABLE payroll.pyr_migration_log (
  pml_id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  pml_run_at    timestamptz NOT NULL DEFAULT now(),
  pml_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pml_table     varchar(60) NOT NULL,           -- nama tabel sumber legacy
  pml_src_rows  integer NOT NULL,
  pml_dst_rows  integer NOT NULL,
  pml_checksum  varchar(64) NOT NULL,           -- sha256 isi sumber terurut
  pml_ok        boolean NOT NULL
);

GRANT USAGE ON SCHEMA payroll TO payroll_etl;
GRANT SELECT ON payroll.pyr_company, payroll.pyr_division_ref, payroll.pyr_currency TO payroll_etl;
GRANT SELECT, INSERT, UPDATE ON payroll.pyr_component, payroll.pyr_component_legacy_map,
  payroll.pyr_component_template, payroll.pyr_level_rule TO payroll_etl;
GRANT SELECT, INSERT ON payroll.pyr_migration_log TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_etl;
GRANT SELECT ON payroll.pyr_migration_log TO payroll_ro, payroll_app;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_component','pyr_component_legacy_map','pyr_component_template'] LOOP
    EXECUTE format('CREATE POLICY p_etl ON payroll.%I TO payroll_etl USING (true) WITH CHECK (true)', t);
  END LOOP;
END $$;

ALTER TABLE payroll.pyr_migration_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_etl ON payroll.pyr_migration_log TO payroll_etl USING (true) WITH CHECK (true);
CREATE POLICY p_scope ON payroll.pyr_migration_log TO payroll_app, payroll_ro
  USING (payroll.fn_in_scope(pml_company_id, NULL));
