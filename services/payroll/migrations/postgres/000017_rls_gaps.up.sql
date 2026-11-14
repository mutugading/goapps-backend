-- [T006c] Celah RLS hasil review T006a (D-14, D-15).
-- * Tabel anak tanpa kolom divisi ikut scope induknya lewat EXISTS (RLS induk berlaku
--   karena subquery dijalankan sebagai invoker): p_parent.
-- * Tabel company-level (divisi berupa array) memakai fn_in_scope(company, NULL).
-- * payroll_stg mendapat kolom company + RLS: app/ro per scope, ETL penuh (p_etl).
-- * company_id tidak lagi DEFAULT 1 (insert lupa company harus gagal) dan dikunci ke
--   pyr_division_ref lewat FK komposit (company, division).
-- Tabel global (tanpa company/divisi) tetap tanpa RLS; daftarnya dijaga test I20.

DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_salary_detail',      'payroll.pyr_run r WHERE r.pr_id = psd_run_id'),
    ('pyr_run_formula',        'payroll.pyr_run r WHERE r.pr_id = prfm_run_id'),
    ('pyr_run_fx',             'payroll.pyr_run r WHERE r.pr_id = prfx_run_id'),
    ('pyr_gl_export',          'payroll.pyr_run r WHERE r.pr_id = pge_run_id'),
    ('pyr_att_snapshot_row',   'payroll.pyr_att_snapshot s WHERE s.pas_snapshot_id = pasr_snapshot_id'),
    ('pyr_employee_nik_hist',  'payroll.pyr_employee e WHERE e.pe_id = penh_employee_id'),
    ('pyr_formula_dependency', 'payroll.pyr_formula_version v WHERE v.pfv_id = pfd_formula_version_id'),
    ('pyr_formula_test_case',  'payroll.pyr_formula_version v WHERE v.pfv_id = pftc_formula_version_id')
  ) AS v(tbl, parent) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_parent ON payroll.%I TO payroll_app, payroll_ro '
      'USING (EXISTS (SELECT 1 FROM %s)) WITH CHECK (EXISTS (SELECT 1 FROM %s))',
      t.tbl, t.parent, t.parent);
  END LOOP;

  FOR t IN SELECT * FROM (VALUES
    ('pyr_component', 'pc_company_id'),
    ('pyr_subsidy_rule', 'psr_company_id')
  ) AS v(tbl, co) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_scope ON payroll.%I TO payroll_app, payroll_ro '
      'USING (payroll.fn_in_scope(%I, NULL)) WITH CHECK (payroll.fn_in_scope(%I, NULL))',
      t.tbl, t.co, t.co);
  END LOOP;
END $$;

-- Staging (data masih kosong; ETL wajib mengisi company).
ALTER TABLE payroll_stg.pyr_stg_sync_batch    ADD COLUMN psb_company_id  smallint NOT NULL REFERENCES payroll.pyr_company;
ALTER TABLE payroll_stg.pyr_stg_att_rekap_hm  ADD COLUMN psar_company_id smallint NOT NULL REFERENCES payroll.pyr_company;
ALTER TABLE payroll_stg.pyr_stg_att_rekap_add ADD COLUMN psar_company_id smallint NOT NULL REFERENCES payroll.pyr_company;
ALTER TABLE payroll_stg.pyr_stg_att_daily     ADD COLUMN psad_company_id smallint NOT NULL REFERENCES payroll.pyr_company;
ALTER TABLE payroll_stg.pyr_stg_sch_rekap     ADD COLUMN pssr_company_id smallint NOT NULL REFERENCES payroll.pyr_company;

DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_stg_sync_batch', 'psb_company_id', 'psb_division'),
    ('pyr_stg_att_rekap_hm', 'psar_company_id', 'psar_division'),
    ('pyr_stg_att_rekap_add', 'psar_company_id', 'psar_division'),
    ('pyr_stg_att_daily', 'psad_company_id', 'psad_division'),
    ('pyr_stg_sch_rekap', 'pssr_company_id', 'pssr_division')
  ) AS v(tbl, co, dv) LOOP
    EXECUTE format('ALTER TABLE payroll_stg.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_scope ON payroll_stg.%I FOR SELECT TO payroll_app, payroll_ro '
      'USING (payroll.fn_in_scope(%I, %I))', t.tbl, t.co, t.dv);
    EXECUTE format('CREATE POLICY p_etl ON payroll_stg.%I TO payroll_etl USING (true) WITH CHECK (true)', t.tbl);
    EXECUTE format(
      'ALTER TABLE payroll_stg.%I ADD CONSTRAINT %I FOREIGN KEY (%I, %I) REFERENCES payroll.pyr_division_ref',
      t.tbl, 'fk_' || t.tbl || '_div', t.co, t.dv);
  END LOOP;
END $$;
-- Cek FK dijalankan sebagai pemilik tabel (payroll_owner): ETL tidak butuh grant ke schema payroll.

-- company_id tanpa default + FK komposit ke pyr_division_ref (pyr_employee sudah, 000010).
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_abnormal_premi', 'pap_company_id', 'pap_division'),
    ('pyr_addition', 'pa_company_id', 'pa_division'),
    ('pyr_att_snapshot', 'pas_company_id', 'pas_division'),
    ('pyr_attd_days', 'pad_company_id', 'pad_division'),
    ('pyr_audit_log', 'pal_company_id', 'pal_division'),
    ('pyr_component_legacy_map', 'pclm_company_id', 'pclm_legacy_division'),
    ('pyr_employee', 'pe_company_id', NULL),
    ('pyr_employee_component', 'pec_company_id', 'pec_division'),
    ('pyr_employee_component_hist', 'pec_company_id', 'pec_division'),
    ('pyr_formula_version', 'pfv_company_id', 'pfv_division'),
    ('pyr_gl_group_map', 'pggm_company_id', 'pggm_division'),
    ('pyr_gl_setup', 'pgs_company_id', 'pgs_division'),
    ('pyr_period', 'pp_company_id', 'pp_division'),
    ('pyr_rule_component', 'prcp_company_id', 'prcp_division'),
    ('pyr_rule_constant', 'prc_company_id', 'prc_division'),
    ('pyr_run', 'pr_company_id', 'pr_division'),
    ('pyr_salary_head', 'psh_company_id', 'psh_division'),
    ('pyr_subsidi_pph', 'psp_company_id', 'psp_division'),
    ('pyr_subsidy_override', 'pso_company_id', 'pso_division'),
    ('pyr_tax_adj', 'pta_company_id', 'pta_division')
  ) AS v(tbl, co, dv) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ALTER COLUMN %I DROP DEFAULT', t.tbl, t.co);
    IF t.dv IS NOT NULL THEN
      EXECUTE format(
        'ALTER TABLE payroll.%I ADD CONSTRAINT %I FOREIGN KEY (%I, %I) REFERENCES payroll.pyr_division_ref',
        t.tbl, 'fk_' || t.tbl || '_div', t.co, t.dv);
    END IF;
  END LOOP;
END $$;
