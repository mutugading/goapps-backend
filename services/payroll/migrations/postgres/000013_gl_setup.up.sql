-- [T003] 04§17 (sumber: spec 04, baris dipotong apa adanya; D-04)
-- ---------------------------------------------------------------------------
-- 17. GL SETUP & EKSPOR JURNAL  (lihat 11 §4; dari MGTDAT.MGT_PAYROLL_SETUP)
--     Tidak ada koneksi ke Orion: jurnal diekspor sebagai file xlsx/csv.
-- ---------------------------------------------------------------------------
CREATE TABLE payroll.pyr_gl_group_map (           -- pengganti CASE Level_Status di RDF
  pggm_company_id smallint NOT NULL REFERENCES payroll.pyr_company, pggm_division varchar(30) NOT NULL,
  pggm_level varchar(50) NOT NULL, pggm_gl_group varchar(50) NOT NULL,
  PRIMARY KEY (pggm_company_id, pggm_division, pggm_level)
);
CREATE TABLE payroll.pyr_gl_setup (
  pgs_id bigserial PRIMARY KEY,
  pgs_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pgs_division varchar(30) NOT NULL,
  pgs_journal_type varchar(5) NOT NULL CHECK (pgs_journal_type IN ('GAJI','THR','BONUS')),
  pgs_gl_group varchar(50) NOT NULL,
  pgs_cost_group varchar(15),                                    -- NULL = semua
  pgs_component_sysid bigint REFERENCES payroll.pyr_component,
  pgs_line_kind varchar(20) NOT NULL DEFAULT 'COMPONENT'
            CHECK (pgs_line_kind IN ('COMPONENT','EMPLOYEE_BALANCE','LOAN_PRINCIPAL','LOAN_INTEREST')),
  pgs_account_main varchar(20) NOT NULL, pgs_account_sub varchar(20),
  pgs_sub_from_employee boolean NOT NULL DEFAULT false,          -- sub = pyr_employee.pe_orion_sub_acnt (tanpa fallback NIK)
  pgs_drcr char(1) NOT NULL CHECK (pgs_drcr IN ('D','C')),
  pgs_description varchar(200),
  pgs_valid_from payroll.yymm NOT NULL, pgs_valid_to payroll.yymm,
  pgs_legacy_row jsonb                                           -- baris asli MGT_PAYROLL_SETUP
);
CREATE UNIQUE INDEX uix_pgs_setup ON payroll.pyr_gl_setup (pgs_company_id, pgs_division, pgs_journal_type, pgs_gl_group,
  coalesce(pgs_cost_group,'*'), coalesce(pgs_component_sysid,0), pgs_line_kind, pgs_valid_from);
CREATE TABLE payroll.pyr_gl_export (
  pge_id bigserial PRIMARY KEY,
  pge_run_id bigint NOT NULL REFERENCES payroll.pyr_run,
  pge_journal_type varchar(5) NOT NULL,
  pge_status varchar(10) NOT NULL CHECK (pge_status IN ('PREVIEW','FINAL','VOID')),
  pge_row_count int, pge_total_debit numeric(20,2), pge_total_credit numeric(20,2),
  pge_file_sha256 text, pge_generated_by varchar(30) NOT NULL, pge_generated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (pge_status <> 'FINAL' OR pge_total_debit = pge_total_credit)
);

-- Catatan grant/RLS untuk objek §14–§17 (jalankan ulang §11 setelah CREATE):
-- * pyr_formula_version / pyr_subsidy_rule: payroll_app hanya boleh menulis status DRAFT/PENDING_APPROVAL.
--   Aktivasi lewat fungsi SECURITY DEFINER payroll.fn_activate_formula(id) / fn_activate_subsidy(id)
--   yang memeriksa 4-eyes, test case, dan valid_from tidak jatuh di periode CLOSED.
-- * RLS p_div diganti p_scope (company + division):
--   USING ((company_id::text || ':' || division) = ANY (string_to_array(current_setting('app.scopes', true), ',')))

