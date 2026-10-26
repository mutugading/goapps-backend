-- H024 (HD-18): menu tabel karyawan R2 menjadi "Employee Overtime" (finance 000591, table_mode=employee_ot).
-- URL & permission tetap (finance.bi.workforceemployee.view). Idempotent.
BEGIN;

UPDATE mst_menu
SET menu_title = 'Employee Overtime',
    updated_at = NOW(),
    updated_by = 'seed'
WHERE menu_code = 'BI_HR_OT_EMPLOYEES';

COMMIT;
