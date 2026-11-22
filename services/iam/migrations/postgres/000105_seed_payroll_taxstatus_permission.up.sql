-- IAM Service Database Migrations
-- 000105: Permission for the backdated payroll tax-status correction (T033n2, D-82 (5)).
--
-- Separate from payroll.master.employee.update: correcting the status valid on 1 January changes the annual PTKP
-- of the whole tax year, so it is granted explicitly (SUPER_ADMIN only by default, D-12).

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('payroll.master.taxstatus.update', 'Correct Payroll Tax Status', 'Backdated correction of a wrongly entered employee tax status (audited)',
     'payroll', 'master', 'update', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'HR_PAYROLL'
  AND p.permission_code = 'payroll.master.taxstatus.update'
  AND p.menu_id IS NULL;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code = 'payroll.master.taxstatus.update'
  AND r.is_active = true AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
