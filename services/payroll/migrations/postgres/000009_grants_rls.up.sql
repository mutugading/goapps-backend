-- [T003] 04§11-12 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 11. GRANT OBJEK (least privilege)
-- ---------------------------------------------------------------------------


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

