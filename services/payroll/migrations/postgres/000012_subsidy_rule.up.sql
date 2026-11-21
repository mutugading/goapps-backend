-- [T003] 04§16 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 16. ATURAN SUBSIDI PPh  (lihat 12 §4; pengganti pyr_subsidi_pph + hardcode 202601)
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_subsidy_rule (
  psr_id bigserial PRIMARY KEY,
  psr_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  psr_name varchar(100) NOT NULL,
  psr_valid_from payroll.yymm NOT NULL, psr_valid_to payroll.yymm,
  psr_priority int NOT NULL DEFAULT 100,                         -- rule pertama yang cocok menang
  psr_divisions varchar(30)[], psr_depts varchar(30)[], psr_levels varchar(50)[], psr_employment_status varchar(15)[],
  psr_condition_expr text,
  psr_basis_kind varchar(20) NOT NULL CHECK (psr_basis_kind IN ('COMPONENTS_PERIOD','COMPONENTS_CURRENT','GROSS_TAX')),
  psr_basis_period_yymm payroll.yymm,                            -- pengganti hardcode 202601
  psr_basis_fallback varchar(13) NOT NULL DEFAULT 'CURRENT' CHECK (psr_basis_fallback IN ('CURRENT','NONE','LATEST_BEFORE')),
  psr_basis_tag varchar(30) NOT NULL DEFAULT 'subsidy_eligible',
  psr_limit_amount payroll.money_idr,                            -- SUBSIDIPPH 10.000.000
  psr_subsidy_kind varchar(12) NOT NULL CHECK (psr_subsidy_kind IN ('FULL_PPH','PERCENT_PPH','FIXED','CAP','EXPR')),
  psr_subsidy_value numeric(20,6), psr_subsidy_expr text,
  psr_target_component_sysid bigint NOT NULL REFERENCES payroll.pyr_component,   -- P000 atau T010 (gross-up JKT)
  psr_status varchar(20) NOT NULL DEFAULT 'DRAFT' CHECK (psr_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  psr_change_reason text,
  created_by varchar(30) NOT NULL, approved_by varchar(30), approved_at timestamptz,
  CHECK (approved_by IS NULL OR approved_by <> created_by)
);
CREATE TABLE payroll.pyr_subsidy_override (       -- koreksi kasus individual
  pso_yymm payroll.yymm NOT NULL, pso_company_id smallint NOT NULL, pso_division varchar(30) NOT NULL, pso_nik varchar(15) NOT NULL,
  pso_rule_id bigint REFERENCES payroll.pyr_subsidy_rule,
  pso_action varchar(8) NOT NULL CHECK (pso_action IN ('INCLUDE','EXCLUDE','AMOUNT')),
  pso_amount payroll.money_idr, pso_reason text NOT NULL,
  created_by varchar(30) NOT NULL, approved_by varchar(30),
  PRIMARY KEY (pso_yymm, pso_company_id, pso_division, pso_nik)
);

