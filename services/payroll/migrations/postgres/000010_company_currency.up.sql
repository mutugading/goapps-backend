-- [T003] 04§14 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 14. COMPANY / DIVISION / CURRENCY  (lihat 12 §1, §5)
--     Domain payroll.division (CHECK 'SOLO','JAKARTA') dilepas; integritas via FK pyr_division_ref.
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_currency (
  pcy_code char(3) PRIMARY KEY,
  pcy_minor_unit smallint NOT NULL,
  pcy_takehome_round smallint NOT NULL          -- IDR: -2 ; USD: 2
);
CREATE TABLE payroll.pyr_company (
  pco_company_id    smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  pco_code          varchar(10) NOT NULL UNIQUE,          -- 'MGT'
  pco_name          varchar(200) NOT NULL,
  pco_npwp_enc      bytea,                                -- NPWP pemotong PPh
  pco_base_currency char(3) NOT NULL DEFAULT 'IDR' REFERENCES payroll.pyr_currency,
  pco_active        boolean NOT NULL DEFAULT true
);
CREATE TABLE payroll.pyr_division_ref (
  pdr_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pdr_code       varchar(30) NOT NULL,                     -- 'SOLO' | 'JAKARTA' | ...
  pdr_name       varchar(100),
  PRIMARY KEY (pdr_company_id, pdr_code)
);
ALTER DOMAIN payroll.division DROP CONSTRAINT division_check;
ALTER TABLE payroll.pyr_employee
  ADD COLUMN pe_company_id smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company,
  ADD COLUMN pe_pay_currency char(3) NOT NULL DEFAULT 'IDR' REFERENCES payroll.pyr_currency,
  ADD COLUMN pe_employment_status varchar(15),   -- PERMANENT|PKWT|PROBATION|OUTSOURCE (ganti hack P_NIK_PKWT)
  ADD COLUMN pe_cost_group varchar(15),          -- MGT|MB|CONTRACT (ganti FM_DEPARTMENT.dept_field_01)
  ADD FOREIGN KEY (pe_company_id, pe_division) REFERENCES payroll.pyr_division_ref;
ALTER TABLE payroll.pyr_run ADD COLUMN pr_company_id smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;

CREATE TABLE payroll.pyr_fx_rate (                -- pengganti FM_EXCHANGE_RATE Orion (tanpa koneksi ke Orion)
  pfr_rate_date date NOT NULL, pfr_from_ccy char(3) NOT NULL, pfr_to_ccy char(3) NOT NULL,
  pfr_rate_type varchar(5) NOT NULL CHECK (pfr_rate_type IN ('TAX','BOOK','PAY')),
  pfr_rate numeric(20,8) NOT NULL CHECK (pfr_rate > 0),
  pfr_source varchar(30) NOT NULL,                -- MANUAL | UPLOAD_ORION | KMK
  created_by varchar(30), created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pfr_rate_date, pfr_from_ccy, pfr_to_ccy, pfr_rate_type)
);
CREATE TABLE payroll.pyr_run_fx (                 -- kurs dikunci per run
  prfx_run_id bigint REFERENCES payroll.pyr_run ON DELETE CASCADE,
  prfx_from_ccy char(3), prfx_to_ccy char(3), prfx_rate_type varchar(5), prfx_rate numeric(20,8) NOT NULL,
  PRIMARY KEY (prfx_run_id, prfx_from_ccy, prfx_to_ccy, prfx_rate_type)
);

