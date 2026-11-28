-- [T008b] Attachment karyawan menunjuk komponen lewat sysid (D-20); (type,id) legacy jadi nullable tanpa FK.
-- Backfill lewat pyr_component_legacy_map se-company. Berhenti bila ada baris tanpa peta sysid tunggal
-- (tabel masih kosong sebelum ETL T009), supaya tidak memilih diam-diam.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component e
             WHERE e.pec_component_sysid IS NULL AND (
               SELECT count(DISTINCT m.pclm_sysid) FROM payroll.pyr_component_legacy_map m
               WHERE m.pclm_company_id = e.pec_company_id
                 AND m.pclm_legacy_type = e.pec_type AND m.pclm_legacy_id = e.pec_id) <> 1) THEN
    RAISE EXCEPTION 'pyr_employee_component: ada baris legacy tanpa peta sysid tunggal di pyr_component_legacy_map; selesaikan dulu (D-20)';
  END IF;
END $$;
UPDATE payroll.pyr_employee_component e
   SET pec_component_sysid = (SELECT m.pclm_sysid FROM payroll.pyr_component_legacy_map m
                              WHERE m.pclm_company_id = e.pec_company_id
                                AND m.pclm_legacy_type = e.pec_type AND m.pclm_legacy_id = e.pec_id LIMIT 1)
 WHERE pec_component_sysid IS NULL;

ALTER TABLE payroll.pyr_employee_component
  DROP CONSTRAINT IF EXISTS pyr_employee_component_pec_type_pec_id_fkey,
  DROP CONSTRAINT IF EXISTS pyr_employee_component_pkey,
  ALTER COLUMN pec_component_sysid SET NOT NULL,
  ALTER COLUMN pec_type DROP NOT NULL,
  ALTER COLUMN pec_id DROP NOT NULL;
ALTER TABLE payroll.pyr_employee_component
  ADD PRIMARY KEY (pec_company_id, pec_nik, pec_division, pec_component_sysid);
CREATE INDEX IF NOT EXISTS ix_pyr_employee_component_sysid ON payroll.pyr_employee_component (pec_component_sysid);

-- Riwayat (snapshot saat level-change) mengikuti bentuk baris hidup.
ALTER TABLE payroll.pyr_employee_component_hist
  ADD COLUMN IF NOT EXISTS pec_component_sysid bigint,
  ADD COLUMN IF NOT EXISTS pec_source varchar(10),
  ALTER COLUMN pec_type DROP NOT NULL,
  ALTER COLUMN pec_id DROP NOT NULL;
