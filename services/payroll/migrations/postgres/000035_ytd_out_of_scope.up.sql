-- [T016e] Deteksi penempatan YTD di divisi di luar app.scopes (13§3.3, D-38 batas loader).
-- RLS membuat loader YTD hanya melihat divisi dalam scope. Fungsi SECURITY DEFINER ini (pemilik =
-- migrator, tanpa FORCE RLS) hanya mengembalikan JUMLAH penempatan di luar scope per NIK target yang
-- punya DETAIL run ACTIVE pada rentang YTD, tanpa nominal. Rantai & NPWP dihitung sama dengan loader.
-- Jumlah itu sengaja diekspos hanya untuk NIK yang sudah dalam scope pemanggil. Konflik NPWP yang
-- hanya terlihat di luar scope tidak ditandai (v1); v2 sudah menolak run lewat sinyal ini.
-- Pemanggil wajib punya scope atas (company, division) target; kalau tidak, hasil kosong.
CREATE OR REPLACE FUNCTION payroll.fn_ytd_out_of_scope(
  p_company smallint, p_division varchar, p_from int, p_to int, p_niks text[], p_depth int)
RETURNS TABLE (out_nik text, out_placements int)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = payroll, pg_catalog AS $$
  WITH RECURSIVE tgt AS (
    SELECT e.pe_nik nik, e.pe_id id FROM payroll.pyr_employee e
    WHERE payroll.fn_in_scope(p_company, p_division)
      AND e.pe_company_id = p_company AND e.pe_division = p_division AND e.pe_sts_payroll = 2
      AND (p_niks = '{}' OR e.pe_nik = ANY (p_niks))
  ), chain AS (
    SELECT t.nik, t.id, e.pe_prev_employee_id prev, 1 depth
    FROM tgt t JOIN payroll.pyr_employee e ON e.pe_id = t.id
    UNION ALL
    SELECT c.nik, e.pe_id, e.pe_prev_employee_id, c.depth + 1
    FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.prev AND e.pe_company_id = p_company
    WHERE c.depth < p_depth
  ), hashes AS (
    SELECT DISTINCT c.nik, e.pe_npwp_hash h FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.id
    WHERE e.pe_npwp_hash IS NOT NULL
  ), mem AS (
    SELECT c.nik, e.pe_nik m_nik, e.pe_division m_div FROM chain c JOIN payroll.pyr_employee e ON e.pe_id = c.id
    UNION
    SELECT x.nik, e.pe_nik, e.pe_division
    FROM hashes x JOIN payroll.pyr_employee e ON e.pe_company_id = p_company AND e.pe_npwp_hash = x.h
  )
  SELECT m.nik, count(DISTINCT (m.m_nik, m.m_div))::int
  FROM mem m
  WHERE NOT payroll.fn_in_scope(p_company, m.m_div)
    AND (EXISTS (
      SELECT 1 FROM payroll.pyr_run r
      JOIN payroll.pyr_salary_detail d ON d.psd_run_id = r.pr_id AND d.psd_nik = m.m_nik
      WHERE r.pr_company_id = p_company AND r.pr_division = m.m_div AND r.pr_status = 'ACTIVE'
        AND r.pr_parent_run_id IS NULL AND r.pr_yymm BETWEEN p_from AND p_to)
    OR EXISTS (
      SELECT 1 FROM payroll.pyr_addition a
      WHERE a.pa_company_id = p_company AND a.pa_division = m.m_div AND a.pa_nik = m.m_nik
        AND a.pa_type = 'Potongan' AND a.pa_id = 7 AND a.pa_yymm BETWEEN p_from AND p_to))
  GROUP BY m.nik
$$;
REVOKE ALL ON FUNCTION payroll.fn_ytd_out_of_scope(smallint, varchar, int, int, text[], int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION payroll.fn_ytd_out_of_scope(smallint, varchar, int, int, text[], int) TO payroll_app;
