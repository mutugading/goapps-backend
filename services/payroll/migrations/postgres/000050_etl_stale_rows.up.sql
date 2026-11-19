-- [T033h2] Review PR #283 #6: baris basi ETL. (1) Template komponen yang diubah/dihapus HR lewat aplikasi tidak boleh
--     dihidupkan lagi oleh ETL COMPNT_TEMPLATE. ReplaceTemplate menghapus baris level lalu menulis ulang, jadi penanda per baris hilang bila HR
--     mengosongkan level; penandanya per (company, level): baris di sini = level dikelola aplikasi, ETL melewatinya (tanpa jalur lepas).
CREATE TABLE IF NOT EXISTS payroll.pyr_component_template_owner (
  pcto_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pcto_template   varchar(255) NOT NULL,
  pcto_managed_by varchar(64) NOT NULL,
  pcto_managed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pcto_company_id, pcto_template)
);

ALTER TABLE payroll.pyr_component_template_owner ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_scope ON payroll.pyr_component_template_owner;
CREATE POLICY p_scope ON payroll.pyr_component_template_owner TO payroll_app
  USING (payroll.fn_in_scope(pcto_company_id, NULL)) WITH CHECK (payroll.fn_in_scope(pcto_company_id, NULL));
DROP POLICY IF EXISTS p_ro ON payroll.pyr_component_template_owner;
CREATE POLICY p_ro ON payroll.pyr_component_template_owner FOR SELECT TO payroll_ro USING (true);
DROP POLICY IF EXISTS p_etl_sel ON payroll.pyr_component_template_owner;
CREATE POLICY p_etl_sel ON payroll.pyr_component_template_owner FOR SELECT TO payroll_etl USING (true);

-- Level yang sudah diubah lewat aplikasi sebelum migrasi ini: penanda diisi dari jejak audit PutTemplate.
INSERT INTO payroll.pyr_component_template_owner (pcto_company_id, pcto_template, pcto_managed_by, pcto_managed_at)
SELECT DISTINCT ON (pal_company_id, pal_entity_key) pal_company_id, pal_entity_key, pal_actor, pal_at
FROM payroll.pyr_audit_log
WHERE pal_entity = 'pyr_component_template' AND pal_actor <> 'etl' AND pal_entity_key IS NOT NULL
ORDER BY pal_company_id, pal_entity_key, pal_at DESC
ON CONFLICT DO NOTHING;

-- ETL kini menghapus baris snapshot LEGACY yang hilang dari spool (T033h2). p_etl (000030) FOR ALL USING (true) dipecah
-- supaya DELETE hanya mengenai snapshot LEGACY (pas_source_batch_ids = '{}'), bukan snapshot ENGINE hasil sync.
DROP POLICY IF EXISTS p_etl ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_sel ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_upd ON payroll.pyr_att_snapshot_row;
DROP POLICY IF EXISTS p_etl_del ON payroll.pyr_att_snapshot_row;
CREATE POLICY p_etl_sel ON payroll.pyr_att_snapshot_row FOR SELECT TO payroll_etl USING (true);
CREATE POLICY p_etl_ins ON payroll.pyr_att_snapshot_row FOR INSERT TO payroll_etl WITH CHECK (true);
CREATE POLICY p_etl_upd ON payroll.pyr_att_snapshot_row FOR UPDATE TO payroll_etl USING (true) WITH CHECK (true);
CREATE POLICY p_etl_del ON payroll.pyr_att_snapshot_row FOR DELETE TO payroll_etl
  USING (EXISTS (SELECT 1 FROM payroll.pyr_att_snapshot s WHERE s.pas_snapshot_id = pasr_snapshot_id AND s.pas_source_batch_ids = '{}'));
