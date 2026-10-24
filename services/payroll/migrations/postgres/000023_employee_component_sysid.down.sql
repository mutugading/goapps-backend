-- [T008b] Kembali ke PK (type,id). Berhenti bila ada baris tanpa (type,id) legacy (komponen baru ber-sysid).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM payroll.pyr_employee_component WHERE pec_type IS NULL OR pec_id IS NULL) THEN
    RAISE EXCEPTION 'pyr_employee_component: ada baris tanpa (type,id) legacy; hapus/isi dulu sebelum down';
  END IF;
END $$;
ALTER TABLE payroll.pyr_employee_component_hist
  DROP COLUMN IF EXISTS pec_component_sysid, DROP COLUMN IF EXISTS pec_source;
DROP INDEX IF EXISTS payroll.ix_pyr_employee_component_sysid;
ALTER TABLE payroll.pyr_employee_component
  DROP CONSTRAINT IF EXISTS pyr_employee_component_pkey,
  ALTER COLUMN pec_type SET NOT NULL,
  ALTER COLUMN pec_id SET NOT NULL,
  ALTER COLUMN pec_component_sysid DROP NOT NULL;
ALTER TABLE payroll.pyr_employee_component
  ADD PRIMARY KEY (pec_company_id, pec_nik, pec_division, pec_type, pec_id),
  ADD CONSTRAINT pyr_employee_component_pec_type_pec_id_fkey
    FOREIGN KEY (pec_type, pec_id) REFERENCES payroll.pyr_component_master;
