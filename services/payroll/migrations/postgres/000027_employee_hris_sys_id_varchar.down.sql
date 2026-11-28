-- Gagal (22P02) bila sudah ada kode teks non-numerik; kosongkan atau hapus barisnya dulu.
ALTER TABLE payroll.pyr_employee
  ALTER COLUMN pe_hris_sys_id TYPE numeric USING pe_hris_sys_id::numeric;
