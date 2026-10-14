-- IAM Service Database Migrations
-- 000095: Permissions for payroll global masters (company, currency) (T007c, D-18)
--
-- pyr_company / pyr_currency are not RLS-protected (D-15), so they get their own
-- permissions instead of reusing payroll.master.employee.*. SUPER_ADMIN gets all
-- six, consistent with 000094 (D-12).

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('payroll.master.company.view',   'View Payroll Company',    'View payroll companies and divisions',        'payroll', 'master', 'view',   true, 'seed'),
    ('payroll.master.company.create', 'Create Payroll Company',  'Create payroll company and division',         'payroll', 'master', 'create', true, 'seed'),
    ('payroll.master.company.update', 'Update Payroll Company',  'Update payroll company and division',         'payroll', 'master', 'update', true, 'seed'),
    ('payroll.master.currency.view',   'View Payroll Currency',   'View payroll currencies',                    'payroll', 'master', 'view',   true, 'seed'),
    ('payroll.master.currency.create', 'Create Payroll Currency', 'Create payroll currency',                    'payroll', 'master', 'create', true, 'seed'),
    ('payroll.master.currency.update', 'Update Payroll Currency', 'Update payroll currency (rounding rules)',   'payroll', 'master', 'update', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'HR_PAYROLL'
  AND p.permission_code IN ('payroll.master.company.view','payroll.master.company.create','payroll.master.company.update',
                            'payroll.master.currency.view','payroll.master.currency.create','payroll.master.currency.update')
  AND p.menu_id IS NULL;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN ('payroll.master.company.view','payroll.master.company.create','payroll.master.company.update',
                            'payroll.master.currency.view','payroll.master.currency.create','payroll.master.currency.update')
  AND r.is_active = true AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
