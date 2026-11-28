-- [T028a] Balikan 000041 (urutan terbalik).
DROP TABLE IF EXISTS payroll.pyr_distribution_item, payroll.pyr_distribution_batch, payroll.pyr_mail_template,
  payroll.pyr_mail_account, payroll.pyr_slip_password_policy, payroll.pyr_doc_template;
ALTER TABLE payroll.pyr_employee DROP COLUMN IF EXISTS pe_email_verified_at;
