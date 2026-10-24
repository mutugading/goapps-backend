-- H034 (rollback): kembalikan tiga menu HR terpisah.
BEGIN;

UPDATE mst_menu
SET menu_title = 'Overtime per Dept', icon_name = 'Clock', updated_at = NOW(), updated_by = 'seed'
WHERE menu_code = 'BI_HR_OT_DEPT';

UPDATE mst_menu
SET is_visible = TRUE, updated_at = NOW(), updated_by = 'seed'
WHERE menu_code IN ('BI_HR_OT_OVERLIMIT', 'BI_HR_OT_EMPLOYEES');

COMMIT;
