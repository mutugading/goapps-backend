-- [T028p] rollback: baris yang tidak muat di skema lama dihapus dulu.
ALTER TABLE payroll.pyr_run_fx ALTER COLUMN prfx_rate TYPE numeric(20,8);
ALTER TABLE payroll.pyr_fx_rate ALTER COLUMN pfr_rate TYPE numeric(20,8);

DELETE FROM payroll.pyr_gl_group_map WHERE pggm_dept_code IS NOT NULL;
DROP INDEX IF EXISTS payroll.uix_pggm_level_dept;
ALTER TABLE payroll.pyr_gl_group_map DROP COLUMN IF EXISTS pggm_dept_code;
ALTER TABLE payroll.pyr_gl_group_map ADD CONSTRAINT pyr_gl_group_map_pkey PRIMARY KEY (pggm_company_id, pggm_division, pggm_level);

DELETE FROM payroll.pyr_gl_setup WHERE pgs_line_kind = 'PAYABLE';
ALTER TABLE payroll.pyr_gl_setup DROP CONSTRAINT IF EXISTS pyr_gl_setup_pgs_line_kind_check;
ALTER TABLE payroll.pyr_gl_setup ADD CONSTRAINT pyr_gl_setup_pgs_line_kind_check
  CHECK (pgs_line_kind IN ('COMPONENT','EMPLOYEE_BALANCE','LOAN_PRINCIPAL','LOAN_INTEREST'));
