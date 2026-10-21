-- H019 (DOWN): hapus menu tabel karyawan R2.
BEGIN;
DELETE FROM menu_permissions WHERE menu_id IN (SELECT menu_id FROM mst_menu WHERE menu_code = 'BI_HR_OT_EMPLOYEES');
DELETE FROM mst_menu WHERE menu_code = 'BI_HR_OT_EMPLOYEES';
COMMIT;
