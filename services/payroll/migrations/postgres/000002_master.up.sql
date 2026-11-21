-- [T003] 04§4 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 4. MASTER
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_employee (
  pe_nik              varchar(15)  NOT NULL,
  pe_division         payroll.division NOT NULL,
  pe_nama             varchar(255) NOT NULL,
  pe_email            varchar(255),
  pe_dept_id          varchar(50), pe_dept varchar(50),
  pe_section_id       varchar(50), pe_section varchar(50),
  pe_designation_id   varchar(50), pe_designation varchar(50),
  pe_grade            varchar(10),
  pe_level            varchar(50),                  -- PE_LEVEL → FK pyr_level_rule
  pe_tax_id           varchar(10),                  -- TK/TK1/K0/…
  pe_sts_emp          smallint,
  pe_sts_payroll      smallint NOT NULL DEFAULT 2,  -- 1 nonaktif, 2 aktif (legacy)
  pe_sts_bonus        smallint,
  pe_date_join        date,
  pe_resign_date      date,
  pe_birth_date       date,                         -- pengganti PE_PASSWORD(ddmmyyyy)
  pe_birth_place      varchar(100),
  pe_sex              varchar(15), pe_married_sts varchar(15), pe_religion varchar(50),
  pe_npwp_enc         bytea,                        -- pgp_sym_encrypt
  pe_ktp_enc          bytea,
  pe_bank             varchar(50), pe_bank_branch varchar(50),
  pe_bank_acnt_enc    bytea, pe_bank_acnt_name varchar(50),
  pe_orion_sub_acnt   varchar(30) NOT NULL,         -- sub-akun Orion (13 §4.1): NIK lama/baru, diisi eksplisit, tidak ikut perubahan NIK
  pe_npwp_hash        bytea,                        -- HMAC-SHA256 NPWP ternormalisasi; kunci YTD PPh (13 §3.3)
  pe_id_mesin         varchar(15),
  pe_cuti_tahunan     numeric(5,1),
  created_by varchar(30), created_at timestamptz NOT NULL DEFAULT now(),
  updated_by varchar(30), updated_at timestamptz,
  PRIMARY KEY (pe_nik, pe_division)
);

CREATE TABLE payroll.pyr_component_master (
  pcm_type            payroll.comp_type NOT NULL,
  pcm_id              integer NOT NULL,
  pcm_name            varchar(50) NOT NULL,
  pcm_sts             varchar(5),              -- F / D default
  pcm_tax             smallint,                -- 2 = kena pajak
  pcm_amount          payroll.money_idr,
  pcm_serial          integer,
  pcm_fix_income      char(1),
  pcm_tax_paid_company char(1),
  pcm_tax_subsidy     char(1),
  PRIMARY KEY (pcm_type, pcm_id)
);

CREATE TABLE payroll.pyr_component_template (
  pct_template   varchar(255) NOT NULL,        -- = level
  pct_type       payroll.comp_type NOT NULL,
  pct_id         integer NOT NULL,
  pct_name       varchar(50),
  pct_sts        varchar(5), pct_tax smallint,
  pct_amount     numeric(18,6),                -- rate (mis. 0.0877) atau rupiah
  pct_tax_paid_company char(1),
  PRIMARY KEY (pct_template, pct_type, pct_id),
  FOREIGN KEY (pct_type, pct_id) REFERENCES payroll.pyr_component_master
);

CREATE TABLE payroll.pyr_employee_component (
  pec_nik        varchar(15) NOT NULL,
  pec_division   payroll.division NOT NULL,
  pec_type       payroll.comp_type NOT NULL,
  pec_id         integer NOT NULL,
  pec_name       varchar(255),
  pec_sts        varchar(5),
  pec_tax        smallint,
  pec_amount     numeric(18,6),
  pec_frz_flag   smallint NOT NULL DEFAULT 2,  -- 2 = aktif
  pec_tax_paid_company char(1),
  pec_valid_from yymm, pec_valid_to yymm,         -- histori (pengganti tabel _2022.._2025)
  updated_by varchar(30), updated_at timestamptz,
  PRIMARY KEY (pec_nik, pec_division, pec_type, pec_id),
  FOREIGN KEY (pec_nik, pec_division) REFERENCES payroll.pyr_employee,
  FOREIGN KEY (pec_type, pec_id) REFERENCES payroll.pyr_component_master
);
CREATE TABLE payroll.pyr_employee_component_hist (LIKE payroll.pyr_employee_component, pec_snapshot_yymm payroll.yymm NOT NULL);

CREATE TABLE payroll.pyr_level_rule (             -- isi TRG_MGT_PAYROLL_EMP_MAST
  plr_level            varchar(50) PRIMARY KEY,
  plr_grp              varchar(10) NOT NULL CHECK (plr_grp IN ('SHIFT','GS','DRIVER','DIRUMAHKAN','STAFF')),
  plr_otinwork_rate    numeric(10,4) NOT NULL,
  plr_otinwork_sts     smallint NOT NULL,
  plr_shiftadj_sts     smallint NOT NULL,
  plr_tunjshift_sts    smallint NOT NULL,
  plr_tunjshift_rate   payroll.money_idr NOT NULL,
  plr_rate_ot_basis    varchar(20) NOT NULL      -- 'T1_7_55' | 'T1_2_3_4' | 'T1'
);

CREATE TABLE payroll.pyr_static_value (           -- IM_VS_STATIC_VALUE PAYROLLSETUP/EMPTAX
  psv_rule_set_id int NOT NULL,
  psv_vs_code     varchar(30) NOT NULL,
  psv_code        varchar(30) NOT NULL,
  psv_name        varchar(100),
  psv_short_name  varchar(100),
  psv_field_01    varchar(100),
  psv_field_02    varchar(100),
  PRIMARY KEY (psv_rule_set_id, psv_vs_code, psv_code)
);

CREATE TABLE payroll.pyr_batal_premi (pbp_code varchar(10) PRIMARY KEY, pbp_beras varchar(10), pbp_abnormal char(1));
CREATE TABLE payroll.pyr_attd_days  (pad_yymm payroll.yymm, pad_division payroll.division, pad_days int NOT NULL, PRIMARY KEY (pad_yymm, pad_division));
CREATE TABLE payroll.pyr_subsidi_pph(psp_yymm payroll.yymm, psp_division payroll.division, psp_dept varchar(15), psp_yn char(1), psp_limit_amt payroll.money_idr,
                                 PRIMARY KEY (psp_yymm, psp_division, psp_dept));
CREATE TABLE payroll.pyr_holiday    (phd_id bigserial PRIMARY KEY, phd_date date NOT NULL, phd_descr varchar(100), phd_status varchar(30), phd_year int);
CREATE TABLE payroll.pyr_tax_adj    (pta_yymm payroll.yymm, pta_division payroll.division, pta_nik varchar(15), pta_type varchar(10), pta_nama varchar(255),
                                 pta_reason varchar(255), pta_amount payroll.money_idr, PRIMARY KEY (pta_yymm, pta_division, pta_nik, pta_type));

