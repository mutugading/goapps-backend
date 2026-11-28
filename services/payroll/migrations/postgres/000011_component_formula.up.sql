-- [T003] 04§15 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 15. KOMPONEN BER-SYSID & RUMUS TERKONFIGURASI  (lihat 12 §2–§3)
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_component (
  pc_sysid            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- tidak pernah dipakai ulang
  pc_company_id       smallint NOT NULL REFERENCES payroll.pyr_company,
  pc_code             varchar(20) NOT NULL,                             -- 'T001', 'P032', 'T096'
  pc_type             payroll.comp_type NOT NULL,
  pc_name             varchar(100) NOT NULL,
  pc_scope_divisions  varchar(30)[],                                    -- NULL = semua divisi company
  pc_currency         char(3) REFERENCES payroll.pyr_currency,              -- NULL = ikut karyawan
  pc_default_sts      char(1) NOT NULL DEFAULT 'F' CHECK (pc_default_sts IN ('F','D')),
  pc_tax_class        varchar(25) NOT NULL DEFAULT 'TAXABLE',
  pc_subsidy_eligible boolean NOT NULL DEFAULT false,
  pc_fix_income       char(1),
  pc_include_in_estimate boolean NOT NULL DEFAULT true,
  pc_tags             varchar(30)[] NOT NULL DEFAULT '{}',              -- bpjs_base, taxable, ...
  pc_serial           int,
  pc_base_component_sysid bigint REFERENCES payroll.pyr_component,
  pc_status           varchar(10) NOT NULL DEFAULT 'DRAFT' CHECK (pc_status IN ('DRAFT','ACTIVE','RETIRED')),
  pc_valid_from payroll.yymm NOT NULL, pc_valid_to payroll.yymm,
  UNIQUE (pc_company_id, pc_code)
);
CREATE TABLE payroll.pyr_component_legacy_map (   -- (type,id,divisi) Oracle → sysid; untuk migrasi, recon, GL
  pclm_legacy_type payroll.comp_type NOT NULL, pclm_legacy_id int NOT NULL, pclm_legacy_division varchar(30) NOT NULL,
  pclm_sysid bigint NOT NULL REFERENCES payroll.pyr_component,
  PRIMARY KEY (pclm_legacy_type, pclm_legacy_id, pclm_legacy_division)
);
-- (type,id) legacy dipertahankan selama paralel-run untuk recon, lalu di-drop setelah go-live.
ALTER TABLE payroll.pyr_employee_component ADD COLUMN pec_component_sysid bigint REFERENCES payroll.pyr_component;
ALTER TABLE payroll.pyr_component_template ADD COLUMN pct_component_sysid bigint REFERENCES payroll.pyr_component;
ALTER TABLE payroll.pyr_employee_component
  ADD COLUMN pec_source varchar(10) NOT NULL DEFAULT 'TEMPLATE' CHECK (pec_source IN ('TEMPLATE','MANUAL'));
-- Eligibility = attachment: hanya komponen ter-attach (frz_flag = 2, periode berlaku) yang dihitung.
-- Level-change hanya menyentuh source = 'TEMPLATE'; komponen MANUAL tidak pernah dihapus otomatis.
ALTER TABLE payroll.pyr_salary_detail
  ADD COLUMN psd_component_sysid bigint REFERENCES payroll.pyr_component,
  ADD COLUMN psd_currency char(3) NOT NULL DEFAULT 'IDR',
  ADD COLUMN psd_amount_base payroll.money_idr,               -- ekuivalen IDR
  ADD COLUMN psd_formula_version_id bigint,
  ADD COLUMN psd_meta jsonb;                                  -- mis. subsidy rule_id, jejak evaluasi

CREATE TABLE payroll.pyr_formula_version (
  pfv_id               bigserial PRIMARY KEY,
  pfv_company_id       smallint NOT NULL REFERENCES payroll.pyr_company,
  pfv_component_sysid  bigint NOT NULL REFERENCES payroll.pyr_component,
  pfv_division         varchar(30),                                      -- NULL = default company
  pfv_valid_from payroll.yymm NOT NULL, pfv_valid_to payroll.yymm,
  pfv_expression       text NOT NULL,                                    -- expr-lang (sandbox, decimal)
  pfv_rounding         varchar(10) NOT NULL DEFAULT 'NONE',              -- NONE|ROUND0|ROUND_-2|TRUNC_-3
  pfv_null_policy      varchar(4)  NOT NULL DEFAULT 'ZERO' CHECK (pfv_null_policy IN ('ZERO','NULL')),
  pfv_remark_template  varchar(255),
  pfv_status           varchar(20) NOT NULL DEFAULT 'DRAFT'
                   CHECK (pfv_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  pfv_change_reason    text,
  created_by varchar(30) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  approved_by varchar(30), approved_at timestamptz,
  CHECK (approved_by IS NULL OR approved_by <> created_by)           -- 4-eyes
);
-- btree_gist masuk allowlist extension (bukan dblink/fdw). Tidak boleh ada 2 versi ACTIVE yang
-- periodenya tumpang tindih untuk komponen + divisi yang sama.
CREATE EXTENSION IF NOT EXISTS btree_gist SCHEMA payroll;
ALTER TABLE payroll.pyr_formula_version ADD CONSTRAINT ex_pfv_no_overlap EXCLUDE USING gist (
  pfv_component_sysid WITH =, (coalesce(pfv_division,'*')) WITH =,
  int4range(pfv_valid_from, coalesce(pfv_valid_to, 209912), '[]') WITH &&
) WHERE (pfv_status = 'ACTIVE');
CREATE TABLE payroll.pyr_formula_dependency (    -- hasil parser C("..")/SUM_C(tag) → urutan topologis
  pfd_formula_version_id bigint REFERENCES payroll.pyr_formula_version ON DELETE CASCADE,
  pfd_depends_on_sysid   bigint REFERENCES payroll.pyr_component,
  PRIMARY KEY (pfd_formula_version_id, pfd_depends_on_sysid)
);
CREATE TABLE payroll.pyr_formula_test_case (     -- aktivasi ditolak bila ada test case gagal
  pftc_id bigserial PRIMARY KEY,
  pftc_formula_version_id bigint NOT NULL REFERENCES payroll.pyr_formula_version ON DELETE CASCADE,
  pftc_input jsonb NOT NULL, pftc_expected numeric(20,6), pftc_expected_null boolean NOT NULL DEFAULT false
);
CREATE TABLE payroll.pyr_run_formula (           -- versi rumus yang dipakai run (reproducibility)
  prfm_run_id bigint REFERENCES payroll.pyr_run ON DELETE CASCADE,
  prfm_formula_version_id bigint REFERENCES payroll.pyr_formula_version,
  PRIMARY KEY (prfm_run_id, prfm_formula_version_id)
);

