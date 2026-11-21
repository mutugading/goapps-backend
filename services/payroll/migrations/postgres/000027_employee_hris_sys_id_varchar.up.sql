-- D-24: HMEMD_SYS_ID production berbentuk kode teks (MGTSLO<digit>, MGTJKT<digit>), bukan angka.
-- UNIQUE tetap; nilai lama (numeric) dikonversi ke teks apa adanya.
ALTER TABLE payroll.pyr_employee
  ALTER COLUMN pe_hris_sys_id TYPE varchar(30) USING pe_hris_sys_id::text;
