-- 000059: Add update permission to lookup master and assign to SUPER_ADMIN.
BEGIN;

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES ('finance.yarnmaster.lookupmaster.update', 'Update Lookup Master', 'Update lookup master display name, table name, and active status', 'finance', 'yarnmaster', 'update', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r, mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code = 'finance.yarnmaster.lookupmaster.update'
ON CONFLICT DO NOTHING;

COMMIT;
