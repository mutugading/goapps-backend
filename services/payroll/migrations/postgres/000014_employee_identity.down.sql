-- [T003] rollback 04§18
DROP TABLE IF EXISTS payroll.pyr_employee_nik_hist CASCADE;
ALTER TABLE payroll.pyr_employee DROP COLUMN IF EXISTS pe_placement_reason, DROP COLUMN IF EXISTS pe_prev_employee_id, DROP COLUMN IF EXISTS pe_hris_sys_id, DROP COLUMN IF EXISTS pe_id;
