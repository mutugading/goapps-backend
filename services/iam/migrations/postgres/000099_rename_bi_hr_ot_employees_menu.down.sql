-- H024 (DOWN): kembalikan label menu tabel karyawan R2.
BEGIN;

UPDATE mst_menu
SET menu_title = 'Karyawan Melebihi 14 Jam',
    updated_at = NOW(),
    updated_by = 'seed'
WHERE menu_code = 'BI_HR_OT_EMPLOYEES';

COMMIT;
