-- [T009e] ETL MAIN/DETAIL historis → run LEGACY (08§2, 04§8-9). pr_kind membedakan run hasil engine dan run hasil
-- migrasi Oracle; rule set 0 = "legacy" (hasil Oracle apa adanya, bukan rule v1 yang dipakai engine).
ALTER TABLE payroll.pyr_run
  ADD COLUMN IF NOT EXISTS pr_kind varchar(10) NOT NULL DEFAULT 'ENGINE' CHECK (pr_kind IN ('ENGINE','LEGACY'));

INSERT INTO payroll.pyr_rule_set (prs_id, prs_name, prs_effective_from, prs_legacy_compat, prs_status)
VALUES (0, 'Legacy (hasil Oracle MGT, dimigrasi)', 201712, true, 'RETIRED') ON CONFLICT (prs_id) DO NOTHING;

-- payroll_etl: hanya menulis ke run LEGACY dan periode yang belum punya run engine; tidak bisa SELECT gaji.
-- INSERT saja di head/detail (tetap tidak bisa membaca gaji, I10). Periode dibuat OPEN, diisi, lalu di-CLOSE
-- oleh ETL sendiri (fn_guard_closed menolak insert ke periode CLOSED); UPDATE periode hanya untuk yang belum CLOSED.
GRANT INSERT ON payroll.pyr_run TO payroll_etl;
-- SELECT per kolom, tanpa total gaji (pr_tot_*): cukup untuk cek keberadaan/jumlah run, termasuk run engine.
GRANT SELECT (pr_id, pr_company_id, pr_yymm, pr_division, pr_kind, pr_status, pr_snapshot_id, pr_emp_count, pr_line_count)
  ON payroll.pyr_run TO payroll_etl;
GRANT INSERT ON payroll.pyr_period, payroll.pyr_salary_head, payroll.pyr_salary_detail TO payroll_etl;
GRANT UPDATE (pp_status, pp_active_run_id, closed_by, closed_at) ON payroll.pyr_period TO payroll_etl;
GRANT SELECT (pp_active_run_id) ON payroll.pyr_period TO payroll_etl;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_etl;
-- COPY ditolak di tabel ber-RLS: ETL memuat ke tabel TEMP lalu INSERT..SELECT (policy + guard tetap berlaku).
DO $$ BEGIN EXECUTE format('GRANT TEMPORARY ON DATABASE %I TO payroll_etl', current_database()); END $$;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['pyr_run','pyr_period','pyr_salary_head','pyr_salary_detail'] LOOP
    IF NOT (SELECT relrowsecurity FROM pg_class WHERE oid = ('payroll.' || t)::regclass) THEN
      RAISE EXCEPTION 'payroll.% tidak ber-RLS; p_etl tidak bisa dipasang', t;
    END IF;
  END LOOP;
  DROP POLICY IF EXISTS p_etl_sel ON payroll.pyr_run;
  CREATE POLICY p_etl_sel ON payroll.pyr_run FOR SELECT TO payroll_etl USING (true);
  DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_run;
  CREATE POLICY p_etl_ins ON payroll.pyr_run FOR INSERT TO payroll_etl WITH CHECK (pr_kind = 'LEGACY');
  DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_period;
  CREATE POLICY p_etl_ins ON payroll.pyr_period FOR INSERT TO payroll_etl WITH CHECK (pp_status = 'OPEN');
  DROP POLICY IF EXISTS p_etl_upd ON payroll.pyr_period;
  CREATE POLICY p_etl_upd ON payroll.pyr_period FOR UPDATE TO payroll_etl
    USING (pp_status = 'OPEN' AND NOT EXISTS (SELECT 1 FROM payroll.pyr_run r WHERE r.pr_company_id = pp_company_id
           AND r.pr_yymm = pp_yymm AND r.pr_division = pp_division AND r.pr_kind <> 'LEGACY'))
    WITH CHECK (true);
  DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_salary_head;
  CREATE POLICY p_etl_ins ON payroll.pyr_salary_head FOR INSERT TO payroll_etl
    WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_run r WHERE r.pr_id = psh_run_id AND r.pr_kind = 'LEGACY'));
  DROP POLICY IF EXISTS p_etl_ins ON payroll.pyr_salary_detail;
  CREATE POLICY p_etl_ins ON payroll.pyr_salary_detail FOR INSERT TO payroll_etl
    WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_run r WHERE r.pr_id = psd_run_id AND r.pr_kind = 'LEGACY'));
END $$;

COMMENT ON COLUMN payroll.pyr_run.pr_kind IS 'ENGINE = hasil engine goapps | LEGACY = hasil Oracle yang dimigrasi (T009e), rule_set_id 0';
