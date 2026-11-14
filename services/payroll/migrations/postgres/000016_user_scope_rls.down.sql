-- [T006a] rollback scope user + RLS p_scope; kembali ke p_div 04§12.
ALTER VIEW payroll.v_pyr_mgt_payroll_main   RESET (security_invoker);
ALTER VIEW payroll.v_pyr_mgt_payroll_detail RESET (security_invoker);

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_abnormal_premi','pyr_addition','pyr_att_snapshot','pyr_attd_days',
    'pyr_audit_log','pyr_component_legacy_map','pyr_employee','pyr_employee_component',
    'pyr_employee_component_hist','pyr_formula_version','pyr_gl_group_map','pyr_gl_setup','pyr_period',
    'pyr_rule_component','pyr_rule_constant','pyr_run','pyr_salary_head','pyr_subsidi_pph',
    'pyr_subsidy_override','pyr_tax_adj'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS p_scope ON payroll.%I', t);
    EXECUTE format('ALTER TABLE payroll.%I DISABLE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;
DROP FUNCTION IF EXISTS payroll.fn_in_scope(smallint, varchar);

ALTER TABLE payroll.pyr_abnormal_premi          DROP COLUMN IF EXISTS pap_company_id;
ALTER TABLE payroll.pyr_addition                DROP COLUMN IF EXISTS pa_company_id;
ALTER TABLE payroll.pyr_att_snapshot            DROP COLUMN IF EXISTS pas_company_id;
ALTER TABLE payroll.pyr_attd_days               DROP COLUMN IF EXISTS pad_company_id;
ALTER TABLE payroll.pyr_audit_log               DROP COLUMN IF EXISTS pal_company_id;
ALTER TABLE payroll.pyr_component_legacy_map    DROP COLUMN IF EXISTS pclm_company_id;
ALTER TABLE payroll.pyr_employee_component      DROP COLUMN IF EXISTS pec_company_id;
ALTER TABLE payroll.pyr_employee_component_hist DROP COLUMN IF EXISTS pec_company_id;
ALTER TABLE payroll.pyr_period                  DROP COLUMN IF EXISTS pp_company_id;
ALTER TABLE payroll.pyr_rule_component          DROP COLUMN IF EXISTS prcp_company_id;
ALTER TABLE payroll.pyr_rule_constant           DROP COLUMN IF EXISTS prc_company_id;
ALTER TABLE payroll.pyr_salary_head             DROP COLUMN IF EXISTS psh_company_id;
ALTER TABLE payroll.pyr_subsidi_pph             DROP COLUMN IF EXISTS psp_company_id;
ALTER TABLE payroll.pyr_tax_adj                 DROP COLUMN IF EXISTS pta_company_id;

DROP TABLE IF EXISTS payroll.pyr_user_scope;

ALTER TABLE payroll.pyr_salary_head ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_salary_head TO payroll_app, payroll_ro
  USING (psh_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));
ALTER TABLE payroll.pyr_employee ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_employee TO payroll_app, payroll_ro
  USING (pe_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));
ALTER TABLE payroll.pyr_addition ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_addition TO payroll_app, payroll_ro
  USING (pa_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));
