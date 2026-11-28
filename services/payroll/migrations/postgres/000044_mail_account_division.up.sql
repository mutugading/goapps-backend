-- T028g3 / D-69: Reply-To HR per divisi. pma_division NULL = baris default company; satu baris aktif per
-- (company, divisi). Kolom lain (host, From, secret_ref) tidak berubah; grants tidak berubah (payroll_app hanya SELECT, D-62).
ALTER TABLE payroll.pyr_mail_account ADD COLUMN pma_division payroll.division;
CREATE UNIQUE INDEX uq_pyr_mail_account_active ON payroll.pyr_mail_account (pma_company_id, COALESCE(pma_division::text, ''))
  WHERE pma_active;
