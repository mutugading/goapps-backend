DROP INDEX IF EXISTS payroll.uq_pyr_mail_account_active;
ALTER TABLE payroll.pyr_mail_account DROP COLUMN IF EXISTS pma_division;
