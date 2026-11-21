-- [T006c] rollback celah RLS (000017).
DO $$
DECLARE t record; tb text;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_abnormal_premi', 'pap_company_id'), ('pyr_addition', 'pa_company_id'),
    ('pyr_att_snapshot', 'pas_company_id'), ('pyr_attd_days', 'pad_company_id'),
    ('pyr_audit_log', 'pal_company_id'), ('pyr_component_legacy_map', 'pclm_company_id'),
    ('pyr_employee', 'pe_company_id'), ('pyr_employee_component', 'pec_company_id'),
    ('pyr_employee_component_hist', 'pec_company_id'), ('pyr_formula_version', 'pfv_company_id'),
    ('pyr_gl_group_map', 'pggm_company_id'), ('pyr_gl_setup', 'pgs_company_id'),
    ('pyr_period', 'pp_company_id'), ('pyr_rule_component', 'prcp_company_id'),
    ('pyr_rule_constant', 'prc_company_id'), ('pyr_run', 'pr_company_id'),
    ('pyr_salary_head', 'psh_company_id'), ('pyr_subsidi_pph', 'psp_company_id'),
    ('pyr_subsidy_override', 'pso_company_id'), ('pyr_tax_adj', 'pta_company_id')
  ) AS v(tbl, co) LOOP
    EXECUTE format('ALTER TABLE payroll.%I DROP CONSTRAINT IF EXISTS %I', t.tbl, 'fk_' || t.tbl || '_div');
    IF t.tbl NOT IN ('pyr_formula_version', 'pyr_gl_group_map', 'pyr_gl_setup', 'pyr_subsidy_override') THEN
      EXECUTE format('ALTER TABLE payroll.%I ALTER COLUMN %I SET DEFAULT 1', t.tbl, t.co);
    END IF;
  END LOOP;

  FOREACH tb IN ARRAY ARRAY['pyr_stg_sync_batch','pyr_stg_att_rekap_hm','pyr_stg_att_rekap_add',
    'pyr_stg_att_daily','pyr_stg_sch_rekap'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_scope ON payroll_stg.%I', tb);
    EXECUTE format('DROP POLICY IF EXISTS p_etl ON payroll_stg.%I', tb);
    EXECUTE format('ALTER TABLE payroll_stg.%I DISABLE ROW LEVEL SECURITY', tb);
  END LOOP;

  FOREACH tb IN ARRAY ARRAY['pyr_salary_detail','pyr_run_formula','pyr_run_fx','pyr_gl_export',
    'pyr_att_snapshot_row','pyr_employee_nik_hist','pyr_formula_dependency','pyr_formula_test_case'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_parent ON payroll.%I', tb);
    EXECUTE format('ALTER TABLE payroll.%I DISABLE ROW LEVEL SECURITY', tb);
  END LOOP;
  FOREACH tb IN ARRAY ARRAY['pyr_component','pyr_subsidy_rule'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_scope ON payroll.%I', tb);
    EXECUTE format('ALTER TABLE payroll.%I DISABLE ROW LEVEL SECURITY', tb);
  END LOOP;
END $$;

ALTER TABLE payroll_stg.pyr_stg_sync_batch    DROP COLUMN psb_company_id;
ALTER TABLE payroll_stg.pyr_stg_att_rekap_hm  DROP COLUMN psar_company_id;
ALTER TABLE payroll_stg.pyr_stg_att_rekap_add DROP COLUMN psar_company_id;
ALTER TABLE payroll_stg.pyr_stg_att_daily     DROP COLUMN psad_company_id;
ALTER TABLE payroll_stg.pyr_stg_sch_rekap     DROP COLUMN pssr_company_id;
