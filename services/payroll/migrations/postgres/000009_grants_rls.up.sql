-- [T003] 04§11-12 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 11. GRANT OBJEK (least privilege)
-- ---------------------------------------------------------------------------
GRANT SELECT ON ALL TABLES IN SCHEMA payroll TO payroll_ro;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payroll TO payroll_app;
REVOKE UPDATE, DELETE ON payroll.pyr_audit_log FROM payroll_app;           -- append-only
REVOKE UPDATE, DELETE, INSERT ON payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_rule_component FROM payroll_app;
GRANT  SELECT ON payroll.pyr_rule_set, payroll.pyr_rule_constant, payroll.pyr_rule_component TO payroll_app;
-- (rule dikelola lewat migrator/role admin terpisah dengan approval)
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_app;

GRANT SELECT ON ALL TABLES IN SCHEMA payroll_stg TO payroll_app, payroll_ro;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA payroll_stg TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll_stg TO payroll_etl;

-- ---------------------------------------------------------------------------
-- 12. RLS per divisi (opsional, lapis ketiga). CATATAN: pada rilis multi-company, p_div diganti p_scope dengan app.scopes (lihat §17)
-- ---------------------------------------------------------------------------
ALTER TABLE payroll.pyr_salary_head ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_salary_head TO payroll_app, payroll_ro
  USING (psh_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));
ALTER TABLE payroll.pyr_employee ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_employee TO payroll_app, payroll_ro
  USING (pe_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));
ALTER TABLE payroll.pyr_addition ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_div ON payroll.pyr_addition TO payroll_app, payroll_ro
  USING (pa_division = ANY (string_to_array(current_setting('app.divisions', true), ',')));

