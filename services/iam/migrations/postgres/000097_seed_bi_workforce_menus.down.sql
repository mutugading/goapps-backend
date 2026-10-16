-- H013 (DOWN): hapus menu dashboard Workforce.
BEGIN;
DELETE FROM menu_permissions WHERE menu_id IN (SELECT menu_id FROM mst_menu WHERE menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT'));
DELETE FROM mst_menu WHERE menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT');
COMMIT;
