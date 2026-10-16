-- H013: menu dashboard Workforce (R1 OT, R2 kepatuhan 14 jam) di bawah Executive Dashboard
-- (BI_PARENT 00000000-0000-0000-0002-000000000020, migrasi 000044). Menu hanya tampil bagi pemegang
-- finance.bi.workforce.view (H004, HD-10). UUID level-3 berikutnya setelah ...052. Idempotent.
BEGIN;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000053', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_DEPT', 'Overtime per Dept', '/finance/bi/HR_OT_DEPT', 'Clock',
     'finance', 3, 40, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000054', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_OVERLIMIT', 'Kepatuhan Lembur 14 Jam', '/finance/bi/HR_OT_OVERLIMIT', 'AlertTriangle',
     'finance', 3, 41, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT')
  AND p.permission_code = 'finance.bi.workforce.view' AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

COMMIT;
