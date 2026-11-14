-- [T006d] Kunci komposit company untuk tabel ber-divisi (04§14, D-14, D-15).
-- * PK/unique yang memuat <prefix>_division kini diawali <prefix>_company_id, supaya dua company
--   dengan kode divisi sama (mis. SOLO) tidak bertabrakan.
-- * FK turunan ikut komposit: pyr_run → pyr_period, pyr_employee_component → pyr_employee.
-- * Guard periode CLOSED (04§9) dan view kompatibilitas (04§10) mencocokkan periode per company.
-- Domain payroll.division sudah tanpa CHECK sejak 000010 (integritas via FK pyr_division_ref).

-- FK turunan dilepas dulu sebelum PK induk diganti.
ALTER TABLE payroll.pyr_run DROP CONSTRAINT pyr_run_pr_yymm_pr_division_fkey;
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT pyr_employee_component_pec_nik_pec_division_fkey;

ALTER TABLE payroll.pyr_employee       DROP CONSTRAINT pyr_employee_pkey,       ADD PRIMARY KEY (pe_company_id, pe_nik, pe_division);
ALTER TABLE payroll.pyr_period         DROP CONSTRAINT pyr_period_pkey,         ADD PRIMARY KEY (pp_company_id, pp_yymm, pp_division);
ALTER TABLE payroll.pyr_employee_component DROP CONSTRAINT pyr_employee_component_pkey,
  ADD PRIMARY KEY (pec_company_id, pec_nik, pec_division, pec_type, pec_id);
ALTER TABLE payroll.pyr_attd_days      DROP CONSTRAINT pyr_attd_days_pkey,      ADD PRIMARY KEY (pad_company_id, pad_yymm, pad_division);
ALTER TABLE payroll.pyr_subsidi_pph    DROP CONSTRAINT pyr_subsidi_pph_pkey,    ADD PRIMARY KEY (psp_company_id, psp_yymm, psp_division, psp_dept);
ALTER TABLE payroll.pyr_tax_adj        DROP CONSTRAINT pyr_tax_adj_pkey,        ADD PRIMARY KEY (pta_company_id, pta_yymm, pta_division, pta_nik, pta_type);
ALTER TABLE payroll.pyr_addition       DROP CONSTRAINT pyr_addition_pkey,       ADD PRIMARY KEY (pa_company_id, pa_yymm, pa_nik, pa_type, pa_id, pa_division);
ALTER TABLE payroll.pyr_abnormal_premi DROP CONSTRAINT pyr_abnormal_premi_pkey,
  ADD PRIMARY KEY (pap_company_id, pap_yymm, pap_nik, pap_date, pap_code, pap_division);
ALTER TABLE payroll.pyr_component_legacy_map DROP CONSTRAINT pyr_component_legacy_map_pkey,
  ADD PRIMARY KEY (pclm_company_id, pclm_legacy_type, pclm_legacy_id, pclm_legacy_division);

ALTER TABLE payroll_stg.pyr_stg_att_rekap_hm  DROP CONSTRAINT pyr_stg_att_rekap_hm_pkey,  ADD PRIMARY KEY (psar_company_id, psar_yymm, psar_nik, psar_division);
ALTER TABLE payroll_stg.pyr_stg_att_rekap_add DROP CONSTRAINT pyr_stg_att_rekap_add_pkey, ADD PRIMARY KEY (psar_company_id, psar_yymm, psar_nik, psar_division);
ALTER TABLE payroll_stg.pyr_stg_att_daily     DROP CONSTRAINT pyr_stg_att_daily_pkey,     ADD PRIMARY KEY (psad_company_id, psad_work_date, psad_nik, psad_division);
ALTER TABLE payroll_stg.pyr_stg_sch_rekap     DROP CONSTRAINT pyr_stg_sch_rekap_pkey,
  ADD PRIMARY KEY (pssr_company_id, pssr_yymm, pssr_nik, pssr_schedule, pssr_division);

DROP INDEX payroll.uix_prc_rule_key;
CREATE UNIQUE INDEX uix_prc_rule_key ON payroll.pyr_rule_constant
  (prc_rule_set_id, prc_company_id, prc_key, COALESCE(prc_division, '*'));
DROP INDEX payroll.uix_prcp_rule_comp;
CREATE UNIQUE INDEX uix_prcp_rule_comp ON payroll.pyr_rule_component
  (prcp_rule_set_id, prcp_company_id, prcp_type, prcp_id, COALESCE(prcp_division, '*'));
DROP INDEX payroll.uix_pr_one_active;
CREATE UNIQUE INDEX uix_pr_one_active ON payroll.pyr_run (pr_company_id, pr_yymm, pr_division)
  WHERE pr_status = 'ACTIVE' AND pr_parent_run_id IS NULL;

ALTER TABLE payroll.pyr_run ADD CONSTRAINT fk_pyr_run_period
  FOREIGN KEY (pr_company_id, pr_yymm, pr_division) REFERENCES payroll.pyr_period;
ALTER TABLE payroll.pyr_employee_component ADD CONSTRAINT fk_pyr_employee_component_employee
  FOREIGN KEY (pec_company_id, pec_nik, pec_division) REFERENCES payroll.pyr_employee;

CREATE OR REPLACE FUNCTION payroll.fn_guard_closed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
DECLARE v_co smallint; v_yymm int; v_div text; v_sts payroll.period_status;
BEGIN
  IF TG_TABLE_NAME = 'pyr_salary_head' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psh_run_id, OLD.psh_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_salary_detail' THEN
    SELECT r.pr_company_id, r.pr_yymm, r.pr_division INTO v_co, v_yymm, v_div FROM payroll.pyr_run r WHERE r.pr_id = COALESCE(NEW.psd_run_id, OLD.psd_run_id);
  ELSIF TG_TABLE_NAME = 'pyr_addition' THEN
    v_co := COALESCE(NEW.pa_company_id, OLD.pa_company_id);
    v_yymm := COALESCE(NEW.pa_yymm, OLD.pa_yymm); v_div := COALESCE(NEW.pa_division, OLD.pa_division);
  END IF;
  SELECT pp_status INTO v_sts FROM payroll.pyr_period WHERE pp_company_id = v_co AND pp_yymm = v_yymm AND pp_division = v_div;
  IF v_sts = 'CLOSED' THEN
    RAISE EXCEPTION 'Periode % % % sudah CLOSED, data tidak boleh diubah', v_co, v_yymm, v_div USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;

CREATE OR REPLACE VIEW payroll.v_pyr_mgt_payroll_main WITH (security_barrier, security_invoker) AS
SELECT h.psh_yymm pm_yymm, h.psh_nik pm_nik, h.psh_nama pm_nama, h.psh_dept pm_dept, h.psh_division pm_division,
       h.psh_basic_pay pm_basic_pay, h.psh_tot_tunjangan pm_tot_tunjangan, h.psh_tot_potongan pm_tot_potongan,
       h.psh_rounding pm_rounding, h.psh_remarks pm_remarks,
       CASE p.pp_status WHEN 'CLOSED' THEN 3 ELSE 1 END pm_status
FROM payroll.pyr_salary_head h
JOIN payroll.pyr_period p ON p.pp_company_id = h.psh_company_id AND p.pp_yymm = h.psh_yymm
  AND p.pp_division = h.psh_division AND p.pp_active_run_id = h.psh_run_id;
