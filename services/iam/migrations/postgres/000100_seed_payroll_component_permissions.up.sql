-- IAM Service Database Migrations
-- 000100: Permissions for payroll component master + level template (T008)
--
-- spec 07 maps both to payroll.master.write; split into view/create/update per
-- entity like 000099 (D-05). Retire uses .update. SUPER_ADMIN gets all five (D-12).

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('payroll.master.component.view',   'View Payroll Component',   'View payroll components (sysid master)',          'payroll', 'master', 'view',   true, 'seed'),
    ('payroll.master.component.create', 'Create Payroll Component', 'Create payroll component',                         'payroll', 'master', 'create', true, 'seed'),
    ('payroll.master.component.update', 'Update Payroll Component', 'Update or retire payroll component',              'payroll', 'master', 'update', true, 'seed'),
    ('payroll.master.template.view',    'View Component Template',  'View component template per employee level',     'payroll', 'master', 'view',   true, 'seed'),
    ('payroll.master.template.update',  'Update Component Template','Replace component template of an employee level', 'payroll', 'master', 'update', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'HR_PAYROLL'
  AND p.permission_code IN ('payroll.master.component.view','payroll.master.component.create','payroll.master.component.update',
                            'payroll.master.template.view','payroll.master.template.update')
  AND p.menu_id IS NULL;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN ('payroll.master.component.view','payroll.master.component.create','payroll.master.component.update',
                            'payroll.master.template.view','payroll.master.template.update')
  AND r.is_active = true AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
