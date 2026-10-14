-- [T003] 04§8 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 8. PERIODE, RUN, HASIL
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_period (
  pp_yymm payroll.yymm, pp_division payroll.division,
  pp_date_from date NOT NULL, pp_date_to date NOT NULL,   -- 26 bln-1 s/d 25
  pp_status payroll.period_status NOT NULL DEFAULT 'OPEN',
  pp_active_run_id bigint,
  approved_by varchar(30), approved_at timestamptz,
  closed_by varchar(30), closed_at timestamptz,
  pp_reopen_reason text,
  PRIMARY KEY (pp_yymm, pp_division)
);

CREATE TABLE payroll.pyr_run (
  pr_id bigserial PRIMARY KEY,
  pr_yymm payroll.yymm NOT NULL, pr_division payroll.division NOT NULL,
  pr_pay_date date NOT NULL,
  pr_rule_set_id int NOT NULL REFERENCES payroll.pyr_rule_set,
  pr_snapshot_id bigint NOT NULL REFERENCES payroll.pyr_att_snapshot,
  pr_parent_run_id bigint REFERENCES payroll.pyr_run,      -- run 1 NIK
  pr_nik_filter varchar(15)[],
  pr_status payroll.run_status NOT NULL,
  pr_input_hash text, pr_emp_count int, pr_line_count int,
  pr_tot_tunjangan payroll.money_idr, pr_tot_potongan payroll.money_idr,
  pr_warnings jsonb,
  pr_started_by varchar(30), pr_started_at timestamptz NOT NULL DEFAULT now(), pr_finished_at timestamptz,
  FOREIGN KEY (pr_yymm, pr_division) REFERENCES payroll.pyr_period
);
CREATE UNIQUE INDEX uix_pr_one_active ON payroll.pyr_run (pr_yymm, pr_division) WHERE pr_status = 'ACTIVE' AND pr_parent_run_id IS NULL;
ALTER TABLE payroll.pyr_period ADD FOREIGN KEY (pp_active_run_id) REFERENCES payroll.pyr_run;

CREATE TABLE payroll.pyr_salary_head (          -- ≈ MGT_PAYROLL_MAIN
  psh_run_id bigint NOT NULL REFERENCES payroll.pyr_run ON DELETE CASCADE,
  psh_nik varchar(15) NOT NULL,
  psh_yymm payroll.yymm NOT NULL, psh_division payroll.division NOT NULL,
  psh_nama varchar(255), psh_dept varchar(255), psh_dept_code varchar(10), psh_section varchar(255),
  psh_grade varchar(10), psh_jabatan varchar(50), psh_emp_type varchar(50), psh_pembayaran varchar(255),
  psh_date_join date, psh_pay_date date, psh_tax_code varchar(5),
  psh_basic_pay payroll.money_idr, psh_tunj_jabatan payroll.money_idr, psh_tunj_rumah payroll.money_idr,
  psh_tunj_kesejahteraan payroll.money_idr, psh_tunj_transport payroll.money_idr, psh_rate_perjam payroll.money_idr,
  psh_tot_tunjangan payroll.money_idr NOT NULL, psh_tot_potongan payroll.money_idr NOT NULL,
  psh_rounding payroll.money_idr NOT NULL, psh_net_pay payroll.money_idr GENERATED ALWAYS AS (psh_tot_tunjangan - psh_tot_potongan - psh_rounding) STORED,
  psh_day_pay numeric(6,2),
  psh_jam_lembur jsonb,                               -- lembur1/2, nas1-3, off1-3, nasp1-3
  psh_remarks varchar(4000),
  PRIMARY KEY (psh_run_id, psh_nik)
);
CREATE TABLE payroll.pyr_salary_detail (             -- ≈ MGT_PAYROLL_DETAIL
  psd_run_id bigint NOT NULL REFERENCES payroll.pyr_run ON DELETE CASCADE,
  psd_nik varchar(15) NOT NULL,
  psd_type payroll.comp_type NOT NULL, psd_id int NOT NULL,
  psd_description varchar(255),
  psd_amount payroll.money_idr,                       -- NULL diizinkan hanya utk legacy_compat (T40)
  psd_sts varchar(5), psd_tax smallint, psd_tax_paid_company char(1),
  psd_basis_amount payroll.money_idr, psd_qty numeric(10,4), psd_rate numeric(18,6),
  psd_formula varchar(40), psd_remarks varchar(1000),
  PRIMARY KEY (psd_run_id, psd_nik, psd_type, psd_id)
);
CREATE INDEX ix_psd_nik ON payroll.pyr_salary_detail (psd_nik, psd_run_id);

-- thr_detail versi awal DIGANTI pyr_thr_head/pyr_thr_detail (§19, lihat 14).
-- PPh Desember & PPHSLIP01 membaca view payroll.v_pyr_thr_detail (kolom legacy, hanya run ACTIVE).

CREATE TABLE payroll.pyr_audit_log (
  pal_id bigserial PRIMARY KEY, pal_at timestamptz NOT NULL DEFAULT now(),
  pal_actor varchar(30) NOT NULL, pal_action varchar(40) NOT NULL,
  pal_entity varchar(40), pal_entity_key text, pal_division varchar(30),
  pal_before jsonb, pal_after jsonb, pal_ip inet
);

