-- Level-2 section header: "MB Costing"
INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES (
  '00000000-0000-0000-0002-000000000021', '00000000-0000-0000-0001-000000000002',
  'FINANCE_MB_SECTION', 'MB Costing', NULL, 'FlaskConical',
  'finance', 2, 70, TRUE, TRUE, 'seed'
)
ON CONFLICT (menu_code) DO NOTHING;

-- Level-3 leaves (UUIDs 0042-0045: next free slots after existing max 0041 = FINANCE_ERP_ITEM)
INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
('00000000-0000-0000-0003-000000000042', '00000000-0000-0000-0002-000000000021',
 'FINANCE_MB_RECIPE', 'MB Recipe', '/finance/mb-recipe', 'TestTube',
 'finance', 3, 1, TRUE, TRUE, 'seed'),
('00000000-0000-0000-0003-000000000043', '00000000-0000-0000-0002-000000000021',
 'FINANCE_MB_PUSH_TO_HEAD', 'MB Push-to-Head', '/finance/mb-push-to-head', 'Send',
 'finance', 3, 2, TRUE, TRUE, 'seed'),
('00000000-0000-0000-0003-000000000044', '00000000-0000-0000-0002-000000000021',
 'FINANCE_MB_LUSTURE', 'MB Lusture', '/finance/master/mb-lusture', 'Sparkles',
 'finance', 3, 3, TRUE, TRUE, 'seed'),
('00000000-0000-0000-0003-000000000045', '00000000-0000-0000-0002-000000000021',
 'FINANCE_MB_PARAM', 'MB Param', '/finance/master/mb-param', 'SlidersHorizontal',
 'finance', 3, 4, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- menu_permissions gating (no entries = visible to all; entries = need matching permission)
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_MB_RECIPE' AND p.permission_code = 'finance.mb.head.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_MB_PUSH_TO_HEAD' AND p.permission_code = 'finance.mb.pushtohead.preview'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_MB_LUSTURE' AND p.permission_code = 'finance.master.mblusture.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_MB_PARAM' AND p.permission_code = 'finance.master.mbparam.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;
