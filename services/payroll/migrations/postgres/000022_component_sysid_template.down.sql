-- Template baru (hanya sysid, tanpa type,id legacy) tidak bisa dikembalikan ke PK legacy: dihapus.
DROP POLICY IF EXISTS p_parent ON payroll.pyr_component_template;
ALTER TABLE payroll.pyr_component_template DISABLE ROW LEVEL SECURITY;
DELETE FROM payroll.pyr_component_template WHERE pct_type IS NULL OR pct_id IS NULL;
ALTER TABLE payroll.pyr_component_template
  DROP CONSTRAINT IF EXISTS pyr_component_template_pkey,
  ALTER COLUMN pct_type SET NOT NULL,
  ALTER COLUMN pct_id SET NOT NULL,
  ALTER COLUMN pct_component_sysid DROP NOT NULL;
ALTER TABLE payroll.pyr_component_template
  ADD PRIMARY KEY (pct_template, pct_type, pct_id),
  ADD CONSTRAINT pyr_component_template_pct_type_pct_id_fkey
    FOREIGN KEY (pct_type, pct_id) REFERENCES payroll.pyr_component_master;
DROP INDEX IF EXISTS payroll.ix_pyr_component_template_sysid;
ALTER TABLE payroll.pyr_component DROP COLUMN IF EXISTS pc_version;
