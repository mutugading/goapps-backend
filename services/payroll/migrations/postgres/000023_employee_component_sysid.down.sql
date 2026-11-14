-- [T008b/T008d] Kembali ke PK (company,nik,division,type,id) + FK master. Idempotent; berhenti dengan pesan jelas bila data tak bisa
-- dikembalikan: baris tanpa (type,id) legacy, dua sysid dengan (type,id) sama pada satu karyawan, atau (type,id) tanpa baris
-- pyr_component_master. FK komposit ke pyr_employee (000018) tidak disentuh oleh up maupun down.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component WHERE pec_type IS NULL OR pec_id IS NULL) THEN
    RAISE EXCEPTION 'pyr_employee_component: ada baris tanpa (type,id) legacy; hapus/isi dulu sebelum down';
  END IF;
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component
             GROUP BY pec_company_id, pec_nik, pec_division, pec_type, pec_id HAVING count(*) > 1) THEN
    RAISE EXCEPTION 'pyr_employee_component: (type,id) legacy dobel pada satu karyawan; PK lama tidak bisa dipulihkan';
  END IF;
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component e
             WHERE NOT EXISTS (SELECT 1 FROM payroll.pyr_component_master m WHERE m.pcm_type = e.pec_type AND m.pcm_id = e.pec_id)) THEN
    RAISE EXCEPTION 'pyr_employee_component: (type,id) tanpa baris pyr_component_master; FK lama tidak bisa dipulihkan';
  END IF;
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component_hist WHERE pec_type IS NULL OR pec_id IS NULL) THEN
    RAISE EXCEPTION 'pyr_employee_component_hist: ada baris tanpa (type,id) legacy; hapus dulu sebelum down';
  END IF;
END $$;
ALTER TABLE payroll.pyr_employee_component_hist
  DROP COLUMN IF EXISTS pec_component_sysid, DROP COLUMN IF EXISTS pec_source,
  ALTER COLUMN pec_type SET NOT NULL,
  ALTER COLUMN pec_id SET NOT NULL;
DROP INDEX IF EXISTS payroll.ix_pyr_employee_component_sysid;
ALTER TABLE payroll.pyr_employee_component
  DROP CONSTRAINT IF EXISTS pyr_employee_component_pkey,
  DROP CONSTRAINT IF EXISTS pyr_employee_component_pec_type_pec_id_fkey,
  ALTER COLUMN pec_type SET NOT NULL,
  ALTER COLUMN pec_id SET NOT NULL,
  ALTER COLUMN pec_component_sysid DROP NOT NULL;
ALTER TABLE payroll.pyr_employee_component
  ADD PRIMARY KEY (pec_company_id, pec_nik, pec_division, pec_type, pec_id),
  ADD CONSTRAINT pyr_employee_component_pec_type_pec_id_fkey
    FOREIGN KEY (pec_type, pec_id) REFERENCES payroll.pyr_component_master;
