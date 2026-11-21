-- [T003] 04§5 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 5. RULE SET (versi aturan)
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_rule_set (
  prs_id              int PRIMARY KEY,
  prs_name            varchar(100) NOT NULL,
  prs_effective_from  payroll.yymm NOT NULL,
  prs_legacy_compat   boolean NOT NULL DEFAULT true,
  prs_status          varchar(10) NOT NULL DEFAULT 'DRAFT' CHECK (prs_status IN ('DRAFT','ACTIVE','RETIRED')),
  approved_by     varchar(30), approved_at timestamptz
);
-- division NULL = berlaku untuk semua divisi; override per divisi dipilih lebih dulu oleh loader
CREATE TABLE payroll.pyr_rule_constant (
  prc_id bigserial PRIMARY KEY,
  prc_rule_set_id int NOT NULL REFERENCES payroll.pyr_rule_set,
  prc_key varchar(50) NOT NULL, prc_value numeric(20,6) NOT NULL,
  prc_division payroll.division
);
CREATE UNIQUE INDEX uix_prc_rule_key ON payroll.pyr_rule_constant (prc_rule_set_id, prc_key, coalesce(prc_division,'*'));
CREATE TABLE payroll.pyr_rule_component (
  prcp_id_row bigserial PRIMARY KEY,
  prcp_rule_set_id int NOT NULL REFERENCES payroll.pyr_rule_set,
  prcp_type payroll.comp_type NOT NULL, prcp_id int NOT NULL,
  prcp_division payroll.division,
  prcp_formula varchar(40) NOT NULL,                 -- kode di registry Go
  prcp_params  jsonb NOT NULL DEFAULT '{}',
  prcp_remark_tpl varchar(255),
  FOREIGN KEY (prcp_type, prcp_id) REFERENCES payroll.pyr_component_master
);
CREATE UNIQUE INDEX uix_prcp_rule_comp ON payroll.pyr_rule_component (prcp_rule_set_id, prcp_type, prcp_id, coalesce(prcp_division,'*'));

