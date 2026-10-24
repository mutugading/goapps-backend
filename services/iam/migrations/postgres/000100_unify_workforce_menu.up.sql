-- H034: tiga menu HR di bawah Executive Dashboard disatukan jadi satu menu "Workforce"
-- (BI_HR_OT_DEPT -> /finance/bi/HR_OT_DEPT). Dua dashboard lain dijangkau lewat tab di halaman viewer.
-- Menu disembunyikan (is_visible=FALSE), bukan dihapus: permission & akses URL langsung tidak berubah. Idempotent.
BEGIN;

UPDATE mst_menu
SET menu_title = 'Workforce', icon_name = 'Users', updated_at = NOW(), updated_by = 'seed'
WHERE menu_code = 'BI_HR_OT_DEPT';

UPDATE mst_menu
SET is_visible = FALSE, updated_at = NOW(), updated_by = 'seed'
WHERE menu_code IN ('BI_HR_OT_OVERLIMIT', 'BI_HR_OT_EMPLOYEES');

COMMIT;
