-- [T006a] Scope user + RLS p_scope (04§17 catatan RLS, D-06, D-11, D-14).
-- * pyr_user_scope: scope company:division per user IAM (multi-baris), admin = pus_all_scopes.
-- * Semua tabel ber-kolom <prefix>_division mendapat <prefix>_company_id (D-14) supaya policy
--   memakai pasangan company_id:division seperti 04§17.
-- * p_div (app.divisions, 04§12) diganti p_scope (app.scopes). Divisi NULL = baris global
--   company itu, terlihat oleh user yang punya scope di company tsb.
-- * View kompatibilitas dijadikan security_invoker, kalau tidak view (milik payroll_owner) melewati RLS.

CREATE TABLE payroll.pyr_user_scope (
  pus_id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  pus_user_id    varchar(64) NOT NULL,                   -- user_id JWT IAM
  pus_all_scopes boolean NOT NULL DEFAULT false,         -- admin: semua company:division aktif
  pus_company_id smallint REFERENCES payroll.pyr_company,
  pus_division   varchar(30),
  created_by     varchar(30), created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (pus_company_id, pus_division) REFERENCES payroll.pyr_division_ref,
  CHECK (pus_all_scopes = (pus_company_id IS NULL AND pus_division IS NULL)),
  CHECK ((pus_company_id IS NULL) = (pus_division IS NULL))
);
CREATE UNIQUE INDEX uix_pus_scope ON payroll.pyr_user_scope
  (pus_user_id, COALESCE(pus_company_id, 0), COALESCE(pus_division, '*'));

-- Kolom company pada 14 tabel ber-divisi yang belum punya (data masih kosong, default company 1).
ALTER TABLE payroll.pyr_abnormal_premi          ADD COLUMN pap_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_addition                ADD COLUMN pa_company_id   smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_att_snapshot            ADD COLUMN pas_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_attd_days               ADD COLUMN pad_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_audit_log               ADD COLUMN pal_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_component_legacy_map    ADD COLUMN pclm_company_id smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_employee_component      ADD COLUMN pec_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_employee_component_hist ADD COLUMN pec_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_period                  ADD COLUMN pp_company_id   smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_rule_component          ADD COLUMN prcp_company_id smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_rule_constant           ADD COLUMN prc_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_salary_head             ADD COLUMN psh_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_subsidi_pph             ADD COLUMN psp_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;
ALTER TABLE payroll.pyr_tax_adj                 ADD COLUMN pta_company_id  smallint NOT NULL DEFAULT 1 REFERENCES payroll.pyr_company;

-- p_div (04§12) dilepas.
DROP POLICY p_div ON payroll.pyr_salary_head;
DROP POLICY p_div ON payroll.pyr_employee;
DROP POLICY p_div ON payroll.pyr_addition;

-- Predikat scope. SQL STABLE tanpa SECURITY DEFINER supaya di-inline planner.
-- app.scopes = 'company_id:division,...'; tidak di-set/kosong = tidak ada scope (fail closed).
CREATE FUNCTION payroll.fn_in_scope(p_company smallint, p_division varchar) RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
  SELECT CASE
    WHEN p_division IS NULL THEN EXISTS (
      SELECT 1 FROM unnest(string_to_array(current_setting('app.scopes', true), ',')) s
      WHERE split_part(s, ':', 1) = p_company::text)
    ELSE (p_company::text || ':' || p_division) = ANY (string_to_array(current_setting('app.scopes', true), ','))
  END
$$;
REVOKE ALL ON FUNCTION payroll.fn_in_scope(smallint, varchar) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION payroll.fn_in_scope(smallint, varchar) TO payroll_app, payroll_ro;

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
    ('pyr_employee', 'pe_company_id', 'pe_division'),
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
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    EXECUTE format(
      'CREATE POLICY p_scope ON payroll.%I TO payroll_app, payroll_ro '
      'USING (payroll.fn_in_scope(%I, %I)) WITH CHECK (payroll.fn_in_scope(%I, %I))',
      t.tbl, t.co, t.dv, t.co, t.dv);
  END LOOP;
END $$;

-- pyr_user_scope juga ber-divisi (I20): user hanya melihat barisnya sendiri (app.user_id).
-- Dikelola lewat migrator/admin DB sampai ada task UI scope; payroll_app hanya SELECT.
ALTER TABLE payroll.pyr_user_scope ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_self ON payroll.pyr_user_scope TO payroll_app, payroll_ro
  USING (pus_user_id = current_setting('app.user_id', true));
GRANT SELECT ON payroll.pyr_user_scope TO payroll_app, payroll_ro;

ALTER VIEW payroll.v_pyr_mgt_payroll_main   SET (security_invoker = true);
ALTER VIEW payroll.v_pyr_mgt_payroll_detail SET (security_invoker = true);
