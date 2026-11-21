-- [T006d] Balikan 000018: kunci tanpa company (gagal kalau sudah ada kunci duplikat lintas company).

CREATE OR REPLACE VIEW payroll.v_pyr_mgt_payroll_main WITH (security_barrier, security_invoker) AS
SELECT h.psh_yymm pm_yymm, h.psh_nik pm_nik, h.psh_nama pm_nama, h.psh_dept pm_dept, h.psh_division pm_division,
       h.psh_basic_pay pm_basic_pay, h.psh_tot_tunjangan pm_tot_tunjangan, h.psh_tot_potongan pm_tot_potongan,
       h.psh_rounding pm_rounding, h.psh_remarks pm_remarks,
       CASE p.pp_status WHEN 'CLOSED' THEN 3 ELSE 1 END pm_status
FROM payroll.pyr_salary_head h
JOIN payroll.pyr_period p ON p.pp_yymm = h.psh_yymm AND p.pp_division = h.psh_division AND p.pp_active_run_id = h.psh_run_id;

CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_yymm int; v_div text; v_sts payroll.period_status;
BEGIN
  IF TG_TABLE_NAME = 'pyr_salary_head' THEN
    SELECT r.pr_yymm, r.pr_division INTO v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psh_run_id, OLD.psh_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
    SELECT r.pr_yymm, r.pr_division INTO v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psd_run_id, OLD.psd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
    v_yymm := COALESCE(NEW.pa_yymm, OLD.pa_yymm); v_div := COALESCE(NEW.pa_division, OLD.pa_division);
  END IF;
  SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_yymm = v_yymm AND pp_division = v_div;
  IF v_sts = 'CLOSED' THEN
    RAISE EXCEPTION 'Periode % % sudah CLOSED, data tidak boleh diubah', v_yymm, v_div USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;

ALTER TABLE payroll.pyr_run DROP CONSTRAINT fk_pyr_run_period;
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT fk_pyr_employee_component_employee;

DROP INDEX payroll.uix_pr_one_active;
CREATE UNIQUE INDEX uix_pr_one_active ON payroll.pyr_run (pr_yymm, pr_division)
  WHERE pr_status = 'ACTIVE' AND pr_parent_run_id IS NULL;
DROP INDEX payroll.uix_prcp_rule_comp;
CREATE UNIQUE INDEX uix_prcp_rule_comp ON payroll.pyr_rule_component (prcp_rule_set_id, prcp_type, prcp_id, COALESCE(prcp_division, '*'));
DROP INDEX payroll.uix_prc_rule_key;
CREATE UNIQUE INDEX uix_prc_rule_key ON payroll.pyr_rule_constant (prc_rule_set_id, prc_key, COALESCE(prc_division, '*'));

ALTER TABLE payroll_stg.pyr_stg_sch_rekap     DROP CONSTRAINT pyr_stg_sch_rekap_pkey,     ADD PRIMARY KEY (pssr_yymm, pssr_nik, pssr_schedule, pssr_division);
ALTER TABLE payroll_stg.pyr_stg_att_daily     DROP CONSTRAINT pyr_stg_att_daily_pkey,     ADD PRIMARY KEY (psad_work_date, psad_nik, psad_division);
ALTER TABLE payroll_stg.pyr_stg_att_rekap_add DROP CONSTRAINT pyr_stg_att_rekap_add_pkey, ADD PRIMARY KEY (psar_yymm, psar_nik, psar_division);
ALTER TABLE payroll_stg.pyr_stg_att_rekap_hm  DROP CONSTRAINT pyr_stg_att_rekap_hm_pkey,  ADD PRIMARY KEY (psar_yymm, psar_nik, psar_division);

ALTER TABLE payroll.pyr_component_legacy_map DROP CONSTRAINT pyr_component_legacy_map_pkey,
  ADD PRIMARY KEY (pclm_legacy_type, pclm_legacy_id, pclm_legacy_division);
ALTER TABLE payroll.pyr_abnormal_premi DROP CONSTRAINT pyr_abnormal_premi_pkey, ADD PRIMARY KEY (pap_yymm, pap_nik, pap_date, pap_code, pap_division);
ALTER TABLE payroll.pyr_addition       DROP CONSTRAINT pyr_addition_pkey,       ADD PRIMARY KEY (pa_yymm, pa_nik, pa_type, pa_id, pa_division);
ALTER TABLE payroll.pyr_tax_adj        DROP CONSTRAINT pyr_tax_adj_pkey,        ADD PRIMARY KEY (pta_yymm, pta_division, pta_nik, pta_type);
ALTER TABLE payroll.pyr_subsidi_pph    DROP CONSTRAINT pyr_subsidi_pph_pkey,    ADD PRIMARY KEY (psp_yymm, psp_division, psp_dept);
ALTER TABLE payroll.pyr_attd_days      DROP CONSTRAINT pyr_attd_days_pkey,      ADD PRIMARY KEY (pad_yymm, pad_division);
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT pyr_employee_component_pkey,
  ADD PRIMARY KEY (pec_nik, pec_division, pec_type, pec_id);
ALTER TABLE payroll.pyr_period         DROP CONSTRAINT pyr_period_pkey,         ADD PRIMARY KEY (pp_yymm, pp_division);
ALTER TABLE payroll.pyr_employee       DROP CONSTRAINT pyr_employee_pkey,       ADD PRIMARY KEY (pe_nik, pe_division);

ALTER TABLE payroll.pyr_employee_component ADD CONSTRAINT pyr_employee_component_pec_nik_pec_division_fkey
  FOREIGN KEY (pec_nik, pec_division) REFERENCES payroll.pyr_employee (pe_nik, pe_division);
ALTER TABLE payroll.pyr_run ADD CONSTRAINT pyr_run_pr_yymm_pr_division_fkey
  FOREIGN KEY (pr_yymm, pr_division) REFERENCES payroll.pyr_period (pp_yymm, pp_division);
