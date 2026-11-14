-- [T003] 04§18 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 18. IDENTITAS KARYAWAN STABIL (kesiapan NIK baru HRIS, lihat 13)
--     NIK menjadi atribut yang bisa berubah; kunci relasi = pyr_employee.pe_id / pe_hris_sys_id.
-- ---------------------------------------------------------------------------
ALTER TABLE payroll.pyr_employee
  ADD COLUMN pe_id               bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
  ADD COLUMN pe_hris_sys_id      numeric UNIQUE,                      -- MGTHRIS.HM_EMP_DATA.HMEMD_SYS_ID
  ADD COLUMN pe_prev_employee_id bigint REFERENCES payroll.pyr_employee (pe_id),  -- HMEMD_PREV_SYS_ID
  ADD COLUMN pe_placement_reason varchar(25)
    CHECK (pe_placement_reason IN ('REHIRE','CONTRACT_TO_PERMANENT','TRANSFER_COMPANY'));
-- Implementasi: PK pyr_employee menjadi (pe_id); tabel transaksi (pyr_employee_component, pyr_addition, pyr_loan_*,
-- pyr_att_snapshot_row, pyr_salary_head/pyr_salary_detail, pyr_thr_*, pyr_tax_adj) memakai FK <prefix>_employee_id menggantikan
-- (nik, division). Kolom nik di pyr_salary_head/pyr_salary_detail tetap disimpan sebagai snapshot NIK saat run.
-- orion_sub_acnt (sub-akun Orion) diisi eksplisit dan TIDAK diturunkan otomatis dari NIK (D-N3).
-- YTD PPh diagregasi per (company_id, tahun pajak, npwp_hash) lintas penempatan (D-N4).
CREATE INDEX ix_pe_npwp_hash ON payroll.pyr_employee (pe_company_id, pe_npwp_hash);

CREATE TABLE payroll.pyr_employee_nik_hist (
  penh_employee_id bigint      NOT NULL REFERENCES payroll.pyr_employee (pe_id),
  penh_nik         varchar(15) NOT NULL,
  penh_valid_from  date        NOT NULL,
  penh_valid_to    date,
  penh_source      varchar(20) NOT NULL
    CHECK (penh_source IN ('HRIS','PAYROLL_NIK','PAYROLL_SUBACNT','MERGED_ROW')),
  PRIMARY KEY (penh_employee_id, penh_nik, penh_source)
);
CREATE INDEX ix_penh_nik ON payroll.pyr_employee_nik_hist (penh_nik);
-- Resolver NIK lama/baru → employee_id; NIK yang cocok ke >1 employee_id ditolak di aplikasi.

