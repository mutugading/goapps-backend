-- [T028a] D-62 (4)(5): DDL 04§21 dokumen & distribusi email, dengan adaptasi:
--   * aktor (created_by/approved_by/pdb.created_by/pdi_resend_by) varchar(64), bukan 30 (D-54);
--   * pyr_employee_slip_pin TIDAK dibuat (policy CUSTOM tidak dipakai, D-62 (4)); CHECK policy tetap memuat 'CUSTOM'
--     sesuai spec, tetapi tidak ada tabel PIN-nya;
--   * pyr_doc_template dibuat tetapi belum dipakai (PDF pure Go, layout di kode);
--   * pyr_mail_account tanpa kredensial (pma_secret_ref = nama secret); baris akun tidak di-seed (diisi IT);
--   * seed (per company berkode 'MGT'; no-op bila company belum ada, mis. DB test): policy DOB_DDMMYYYY ACTIVE dan
--     template email SLIP_GAJI 16§2.5. Seed ACTIVE memakai created_by='migration' <> approved_by='system' (CHECK 4-eyes);
--   * RLS p_scope (company-only untuk tabel tanpa divisi, fn_in_scope(.., NULL)), p_parent untuk item (via batch),
--     pola 000039. Aktivasi 4-eyes via fungsi SECURITY DEFINER belum ada: payroll_app hanya SELECT pada
--     template/policy/account (task ini), penulisan lewat migrator/admin sampai ada task UI.
CREATE TABLE payroll.pyr_doc_template (
  pdt_id bigserial PRIMARY KEY,
  pdt_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pdt_doc_type varchar(15) NOT NULL
           CHECK (pdt_doc_type IN ('SLIP_GAJI','SLIP_THR','SLIP_BONUS','PPH_THR','PPH_MONTHLY','PPH_ANNUAL','PPH_DEC_SETTLE','BUKTI_POTONG','LOAN_CARD')),
  pdt_version int NOT NULL, pdt_html text NOT NULL, pdt_css text,
  pdt_valid_from payroll.yymm NOT NULL,
  pdt_status varchar(20) NOT NULL DEFAULT 'DRAFT' CHECK (pdt_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  created_by varchar(64) NOT NULL, approved_by varchar(64),
  CHECK (approved_by IS NULL OR approved_by <> created_by),
  UNIQUE (pdt_company_id, pdt_doc_type, pdt_version)
);
CREATE TABLE payroll.pyr_slip_password_policy (
  pspp_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pspp_valid_from date NOT NULL,
  pspp_policy_code varchar(15) NOT NULL DEFAULT 'DOB_DDMMYYYY'
              CHECK (pspp_policy_code IN ('DOB_DDMMYYYY','NIK_DOB','DOB_KTP4','CUSTOM')),
  pspp_status varchar(20) NOT NULL DEFAULT 'DRAFT' CHECK (pspp_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  created_by varchar(64) NOT NULL, approved_by varchar(64),
  CHECK (approved_by IS NULL OR approved_by <> created_by),
  PRIMARY KEY (pspp_company_id, pspp_valid_from)
);
ALTER TABLE payroll.pyr_employee ADD COLUMN pe_email_verified_at timestamptz;
CREATE TABLE payroll.pyr_mail_account (
  pma_id smallserial PRIMARY KEY,
  pma_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pma_host varchar(200) NOT NULL, pma_port int NOT NULL DEFAULT 587,
  pma_tls_mode varchar(10) NOT NULL DEFAULT 'STARTTLS' CHECK (pma_tls_mode IN ('STARTTLS','TLS')),
  pma_from_address varchar(255) NOT NULL, pma_from_name varchar(100), pma_reply_to varchar(255),
  pma_secret_ref varchar(200) NOT NULL,                           -- nama secret; kredensial SMTP tidak di DB
  pma_rate_per_minute int NOT NULL DEFAULT 30, pma_max_attempts int NOT NULL DEFAULT 5,
  pma_active boolean NOT NULL DEFAULT true
);
CREATE TABLE payroll.pyr_mail_template (
  pmt_id bigserial PRIMARY KEY,
  pmt_company_id smallint NOT NULL REFERENCES payroll.pyr_company,
  pmt_doc_type varchar(15) NOT NULL, pmt_version int NOT NULL,
  pmt_subject varchar(255) NOT NULL, pmt_body_html text NOT NULL, pmt_body_text text,
  pmt_status varchar(20) NOT NULL DEFAULT 'DRAFT' CHECK (pmt_status IN ('DRAFT','PENDING_APPROVAL','ACTIVE','RETIRED')),
  created_by varchar(64) NOT NULL, approved_by varchar(64),
  CHECK (approved_by IS NULL OR approved_by <> created_by),
  UNIQUE (pmt_company_id, pmt_doc_type, pmt_version)
);
CREATE TABLE payroll.pyr_distribution_batch (
  pdb_id bigserial PRIMARY KEY,
  pdb_company_id smallint NOT NULL REFERENCES payroll.pyr_company, pdb_division payroll.division NOT NULL,
  pdb_doc_type varchar(15) NOT NULL,
  pdb_yymm payroll.yymm, pdb_thr_event_id bigint REFERENCES payroll.pyr_thr_event,
  pdb_run_id bigint REFERENCES payroll.pyr_run, pdb_thr_run_id bigint REFERENCES payroll.pyr_thr_run,
  pdb_is_revision boolean NOT NULL DEFAULT false,
  pdb_supersedes_batch_id bigint REFERENCES payroll.pyr_distribution_batch,
  pdb_status varchar(12) NOT NULL DEFAULT 'DRAFT' CHECK (pdb_status IN ('DRAFT','RUNNING','DONE','CANCELLED','SUPERSEDED')),
  pdb_scheduled_at timestamptz,
  pdb_total int, pdb_sent int NOT NULL DEFAULT 0, pdb_failed int NOT NULL DEFAULT 0, pdb_blocked int NOT NULL DEFAULT 0,
  pdb_mail_template_id bigint REFERENCES payroll.pyr_mail_template,
  pdb_doc_template_id bigint REFERENCES payroll.pyr_doc_template,
  created_by varchar(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), pdb_finished_at timestamptz
);
CREATE TABLE payroll.pyr_distribution_item (
  pdi_id bigserial PRIMARY KEY,
  pdi_batch_id bigint NOT NULL REFERENCES payroll.pyr_distribution_batch,
  pdi_employee_id bigint NOT NULL REFERENCES payroll.pyr_employee (pe_id), pdi_nik varchar(15) NOT NULL,
  pdi_email_to varchar(255),
  pdi_status varchar(20) NOT NULL DEFAULT 'PENDING'
         CHECK (pdi_status IN ('PENDING','RENDERED','QUEUED','SENDING','SENT','FAILED','FAILED_FINAL','BOUNCED',
                           'BLOCKED_NO_EMAIL','BLOCKED_NO_PASSWORD','EXCLUDED')),
  pdi_attempts int NOT NULL DEFAULT 0, pdi_next_attempt_at timestamptz,
  pdi_message_id varchar(255), pdi_smtp_response text, pdi_last_error text,
  pdi_attachment_name varchar(100), pdi_attachment_sha256 text, pdi_content_sha256 text,
  pdi_sent_at timestamptz,
  pdi_resend_of_item_id bigint REFERENCES payroll.pyr_distribution_item, pdi_resend_reason text, pdi_resend_by varchar(64)
);
CREATE UNIQUE INDEX uix_pdi_first ON payroll.pyr_distribution_item (pdi_batch_id, pdi_employee_id)
  WHERE pdi_resend_of_item_id IS NULL;
CREATE INDEX ix_pdi_queue ON payroll.pyr_distribution_item (pdi_status, pdi_next_attempt_at)
  WHERE pdi_status IN ('QUEUED','FAILED');

-- Seed per company 'MGT' (no-op bila belum ada).
INSERT INTO payroll.pyr_slip_password_policy (pspp_company_id, pspp_valid_from, pspp_policy_code, pspp_status, created_by, approved_by)
SELECT pco_company_id, DATE '2020-01-01', 'DOB_DDMMYYYY', 'ACTIVE', 'migration', 'system'
FROM payroll.pyr_company WHERE pco_code = 'MGT';
INSERT INTO payroll.pyr_mail_template (pmt_company_id, pmt_doc_type, pmt_version, pmt_subject, pmt_body_html, pmt_body_text, pmt_status, created_by, approved_by)
SELECT pco_company_id, 'SLIP_GAJI', 1,
  'Slip Gaji {{.PeriodName}} — {{.Nama}}',
  '<p>Yth. {{.Nama}},</p><p>terlampir slip gaji periode {{.PeriodName}}.</p><p>File dilindungi password: {{.PasswordHint}}.</p><p>Pertanyaan: hubungi HR {{.Division}} ({{.ReplyTo}}).</p>',
  E'Yth. {{.Nama}},\n\nterlampir slip gaji periode {{.PeriodName}}.\nFile dilindungi password: {{.PasswordHint}}.\nPertanyaan: hubungi HR {{.Division}} ({{.ReplyTo}}).\n',
  'ACTIVE', 'migration', 'system'
FROM payroll.pyr_company WHERE pco_code = 'MGT';

-- RLS (D-15, I20)
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT * FROM (VALUES
    ('pyr_doc_template',         'pdt_company_id', NULL),
    ('pyr_slip_password_policy', 'pspp_company_id', NULL),
    ('pyr_mail_account',         'pma_company_id', NULL),
    ('pyr_mail_template',        'pmt_company_id', NULL),
    ('pyr_distribution_batch',   'pdb_company_id', 'pdb_division')
  ) AS v(tbl, co, dv) LOOP
    EXECUTE format('ALTER TABLE payroll.%I ENABLE ROW LEVEL SECURITY', t.tbl);
    IF t.dv IS NULL THEN
      EXECUTE format(
        'CREATE POLICY p_scope ON payroll.%I TO payroll_app, payroll_ro '
        'USING (payroll.fn_in_scope(%I, NULL)) WITH CHECK (payroll.fn_in_scope(%I, NULL))',
        t.tbl, t.co, t.co);
    ELSE
      EXECUTE format(
        'CREATE POLICY p_scope ON payroll.%I TO payroll_app, payroll_ro '
        'USING (payroll.fn_in_scope(%I, %I)) WITH CHECK (payroll.fn_in_scope(%I, %I))',
        t.tbl, t.co, t.dv, t.co, t.dv);
    END IF;
  END LOOP;
END $$;
ALTER TABLE payroll.pyr_distribution_item ENABLE ROW LEVEL SECURITY;
CREATE POLICY p_parent ON payroll.pyr_distribution_item TO payroll_app, payroll_ro
  USING (EXISTS (SELECT 1 FROM payroll.pyr_distribution_batch b WHERE b.pdb_id = pdi_batch_id))
  WITH CHECK (EXISTS (SELECT 1 FROM payroll.pyr_distribution_batch b WHERE b.pdb_id = pdi_batch_id));

-- Grant: template/policy/account hanya SELECT untuk app (D-62); batch/item DML, item tanpa DELETE (jejak audit).
GRANT SELECT ON payroll.pyr_doc_template, payroll.pyr_slip_password_policy, payroll.pyr_mail_account,
  payroll.pyr_mail_template, payroll.pyr_distribution_batch, payroll.pyr_distribution_item TO payroll_app, payroll_ro;
GRANT INSERT, UPDATE, DELETE ON payroll.pyr_distribution_batch TO payroll_app;
GRANT INSERT, UPDATE ON payroll.pyr_distribution_item TO payroll_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA payroll TO payroll_app, payroll_etl;
