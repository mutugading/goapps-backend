-- Migration 000060: Add Calc Schedule menu under Product Costing.
-- Menu ID: 00000000-0000-0000-0003-000000000039 (next available level-3 sequence after 38)
-- Permission gate: finance.cost.caljob.view (same as Calc Jobs — schedule page is a view/trigger surface)

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url,
    icon_name, service_name, menu_level, sort_order,
    is_visible, is_active, created_by
) VALUES (
    '00000000-0000-0000-0003-000000000039',
    '00000000-0000-0000-0002-000000000015',  -- FINANCE_PRODUCT_COSTING
    'FINANCE_CALC_SCHEDULE',
    'Calc Schedule',
    '/finance/calc-schedule',
    'CalendarClock',
    'finance',
    3,
    25,  -- between Calc Jobs (23) and Cost Results (24) → after cost-results
    TRUE,
    TRUE,
    'seed'
)
ON CONFLICT (menu_code) DO NOTHING;

-- Link the menu to the existing finance.cost.caljob.view permission
-- so only users with that permission see the schedule page.
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT
    '00000000-0000-0000-0003-000000000039',
    p.permission_id,
    'seed'
FROM mst_permission p
WHERE p.permission_code = 'finance.cost.caljob.view'
  AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;
