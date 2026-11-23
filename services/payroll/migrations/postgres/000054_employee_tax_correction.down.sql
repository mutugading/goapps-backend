-- [T033n2] Baris koreksi (CORRECT) dipertahankan sebagai UPDATE supaya PTKP 1 Jan tidak berubah diam-diam; alasan hilang
-- (jejaknya tetap di pyr_audit_log TAX_STATUS_CORRECT).
DROP FUNCTION IF EXISTS payroll.fn_correct_employee_tax(bigint, varchar, date, varchar);
ALTER TABLE payroll.pyr_employee_tax_hist DROP CONSTRAINT IF EXISTS chk_peth_reason;
ALTER TABLE payroll.pyr_employee_tax_hist DROP CONSTRAINT IF EXISTS pyr_employee_tax_hist_peth_source_check;
UPDATE payroll.pyr_employee_tax_hist SET peth_source = 'UPDATE' WHERE peth_source = 'CORRECT';
ALTER TABLE payroll.pyr_employee_tax_hist ADD CONSTRAINT pyr_employee_tax_hist_peth_source_check
  CHECK (peth_source IN ('BACKFILL','INSERT','UPDATE'));
ALTER TABLE payroll.pyr_employee_tax_hist DROP COLUMN IF EXISTS peth_reason;
