-- [T003] 04§6 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 6. TRANSAKSI INPUT
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_addition (
  pa_yymm payroll.yymm, pa_nik varchar(15), pa_division payroll.division,
  pa_type payroll.comp_type, pa_id int,
  pa_nama varchar(255), pa_compnt_name varchar(50), pa_compnt_sts varchar(5), pa_compnt_tax smallint,
  pa_amount payroll.money_idr NOT NULL, pa_remarks varchar(255),
  pa_source varchar(20) NOT NULL DEFAULT 'MANUAL',   -- MANUAL | UPLOAD | LOAN | MIGRASI
  pa_loan_id bigint,
  created_by varchar(30), created_at timestamptz DEFAULT now(),
  PRIMARY KEY (pa_yymm, pa_nik, pa_type, pa_id, pa_division)
);
CREATE TABLE payroll.pyr_abnormal_premi (
  pap_yymm payroll.yymm, pap_nik varchar(15), pap_date date, pap_code varchar(5), pap_division payroll.division,
  pap_nama varchar(300), pap_dept varchar(15), PRIMARY KEY (pap_yymm, pap_nik, pap_date, pap_code, pap_division)
);
-- loan_request / loan_schedule versi awal DIGANTI sub-ledger pinjaman §20 (lihat 15).
-- pyr_addition.pa_loan_id tetap dipakai untuk cicilan hasil migrasi (pa_source = 'LOAN', pa_generated = true).
ALTER TABLE payroll.pyr_addition ADD COLUMN pa_generated boolean NOT NULL DEFAULT false;

