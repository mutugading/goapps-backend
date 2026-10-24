-- [T008] Komponen ber-sysid: versi row untuk ETag (D-16) + template PK pindah ke sysid dan ber-RLS (D-20).
-- Attachment karyawan (pyr_employee_component) mengikuti di T008b.
ALTER TABLE payroll.pyr_component ADD COLUMN IF NOT EXISTS pc_version integer NOT NULL DEFAULT 1;

-- Backfill template legacy lewat peta (type,id) -> sysid. Peta bisa punya >1 sysid per (type,id)
-- (PK-nya menyertakan divisi): bila ambigu, berhenti supaya tidak memilih diam-diam (D-20).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM payroll.pyr_component_template t
             WHERE t.pct_component_sysid IS NULL AND (
               SELECT count(DISTINCT m.pclm_sysid) FROM payroll.pyr_component_legacy_map m
               WHERE m.pclm_legacy_type = t.pct_type AND m.pclm_legacy_id = t.pct_id) <> 1) THEN
    RAISE EXCEPTION 'pyr_component_template: ada baris legacy tanpa peta sysid tunggal di pyr_component_legacy_map; selesaikan dulu (D-20)';
  END IF;
END $$;
UPDATE payroll.pyr_component_template t
   SET pct_component_sysid = (SELECT m.pclm_sysid FROM payroll.pyr_component_legacy_map m
                              WHERE m.pclm_legacy_type = t.pct_type AND m.pclm_legacy_id = t.pct_id LIMIT 1)
 WHERE pct_component_sysid IS NULL;

ALTER TABLE payroll.pyr_component_template
  DROP CONSTRAINT IF EXISTS pyr_component_template_pct_type_pct_id_fkey,
  DROP CONSTRAINT IF EXISTS pyr_component_template_pkey,
  ALTER COLUMN pct_component_sysid SET NOT NULL,
  ALTER COLUMN pct_type DROP NOT NULL,
  ALTER COLUMN pct_id DROP NOT NULL;
ALTER TABLE payroll.pyr_component_template ADD PRIMARY KEY (pct_template, pct_component_sysid);
CREATE INDEX IF NOT EXISTS ix_pyr_component_template_sysid ON payroll.pyr_component_template (pct_component_sysid);

-- Template merujuk komponen milik company, jadi baris ikut scope komponennya (pola p_parent, 000017).
-- Tabel keluar dari allowlist I20 (D-15 diubah oleh D-20).
ALTER TABLE payroll.pyr_component_template ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_parent ON payroll.pyr_component_template;
CREATE POLICY p_parent ON payroll.pyr_component_template TO payroll_app, payroll_ro
  USING (EXISTS (SELECT 1 FROM payroll.pyr_component c WHERE c.pc_sysid = pct_component_sysid))
  WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_component c WHERE c.pc_sysid = pct_component_sysid));
