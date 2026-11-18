-- [T033f] review #4: flag pajak lampiran komponen hanya 0 (belum diisi), 1 (tidak kena pajak), 2 (kena pajak, legacy MPCE_TAX).
-- Default saat attach UI diambil dari pc_tax_class di service (template: 0/NULL = ikut kelas); loader memberi warning
-- TAX_CLASS_MISMATCH bila beda. Data ETL saat ini hanya berisi 1/2. Satu berkas = satu transaksi golang-migrate, jadi
-- lock ACCESS EXCLUSIVE dipegang sampai validasi selesai (±121 ribu baris, cepat).
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT IF EXISTS ck_pec_tax;
ALTER TABLE payroll.pyr_employee_component ADD CONSTRAINT ck_pec_tax CHECK (pec_tax IN (0, 1, 2));
ALTER TABLE payroll.pyr_component_template DROP CONSTRAINT IF EXISTS ck_pct_tax;
ALTER TABLE payroll.pyr_component_template ADD CONSTRAINT ck_pct_tax CHECK (pct_tax IN (0, 1, 2));
