-- 000101: permissions for MB spin <-> FG/POY link feature. No DDL ('sync' and 'restore' already in chk_permission_action, 000094).
-- Granted only to SUPER_ADMIN. Idempotent.
BEGIN;

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.product.master.sync', 'Sync Product Master from Oracle', 'Preview and apply ETL Oracle items into Product Master', 'finance', 'product', 'sync', true, 'seed'),
    ('finance.yarnmaster.mbspin.restore', 'Un-supersede MB Spin', 'Restore a superseded MB spin and revert repointed params', 'finance', 'yarnmaster', 'restore', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = true
  AND p.permission_code IN ('finance.product.master.sync', 'finance.yarnmaster.mbspin.restore') AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
