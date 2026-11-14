-- [T028p] D-63: setup GL lanjutan untuk jurnal `upload` (T028l).
-- (1) Baris legacy MPS_JENIS 'Salary Payables'/'THR Payables' (per jabatan, C) dimuat sebagai line_kind PAYABLE.
ALTER TABLE payroll.pyr_gl_setup DROP CONSTRAINT IF EXISTS pyr_gl_setup_pgs_line_kind_check;
ALTER TABLE payroll.pyr_gl_setup ADD CONSTRAINT pyr_gl_setup_pgs_line_kind_check
  CHECK (pgs_line_kind IN ('COMPONENT','EMPLOYEE_BALANCE','LOAN_PRINCIPAL','LOAN_INTEREST','PAYABLE'));

-- (3) gl_group bisa bergantung dept (legacy: SOLO PM_EMP_TYPE='Driver' AND PM_DEPT_CODE='HRMC' → 'Driver Contract').
-- Baris ber-dept (pggm_dept_code terisi) berprioritas lebih tinggi daripada baris level saja (NULL = semua dept).
ALTER TABLE payroll.pyr_gl_group_map ADD COLUMN IF NOT EXISTS pggm_dept_code varchar(10);
ALTER TABLE payroll.pyr_gl_group_map DROP CONSTRAINT IF EXISTS pyr_gl_group_map_pkey;
CREATE UNIQUE INDEX IF NOT EXISTS uix_pggm_level_dept ON payroll.pyr_gl_group_map
  (pggm_company_id, pggm_division, pggm_level, coalesce(pggm_dept_code, '*'));

-- (4) Kurs BOOK dari FM_EXCHANGE_RATE punya 12 desimal (202610: 0.000055561729, dipakai sheet upload contoh);
-- numeric(20,8) memotongnya menjadi 0.00005556. Skala dinaikkan agar LC = ROUND(FC × rate, 2) sama dengan legacy.
ALTER TABLE payroll.pyr_fx_rate ALTER COLUMN pfr_rate TYPE numeric(24,12);
ALTER TABLE payroll.pyr_run_fx ALTER COLUMN prfx_rate TYPE numeric(24,12);
