-- H019: menu dashboard tabel karyawan R2 (HR_OT_EMPLOYEES, finance 000589) di bawah Executive Dashboard.
-- Data level karyawan (NIK) -> menu hanya untuk pemegang finance.bi.workforceemployee.view (HD-07, HD-10). Idempotent.
BEGIN;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000059', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_EMPLOYEES', 'Karyawan Melebihi 14 Jam', '/finance/bi/HR_OT_EMPLOYEES', 'Users',
     'finance', 3, 42, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code = 'BI_HR_OT_EMPLOYEES'
  AND p.permission_code = 'finance.bi.workforceemployee.view' AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

COMMIT;
