-- 000055: add finance.costing.route.unlock permission and assign to SUPER_ADMIN.
-- Permission code format: each segment matches [a-z][a-z0-9]* (no underscores).

DO $$
DECLARE
    v_perm_id UUID;
BEGIN
    INSERT INTO mst_permission (
        permission_id, permission_code, permission_name, description,
        service_name, module_name, action_type, is_active, created_by
    ) VALUES (
        gen_random_uuid(),
        'finance.costing.route.unlock',
        'Finance Costing Route Lock/Unlock',
        'Allows locking and unlocking cost routes from the CPR detail page.',
        'finance',
        'costing',
        'unlock',
        TRUE,
        'seed'
    )
    ON CONFLICT (permission_code) DO NOTHING;

    SELECT permission_id INTO v_perm_id
    FROM mst_permission
    WHERE permission_code = 'finance.costing.route.unlock';

    INSERT INTO role_permissions (role_id, permission_id, assigned_by)
    SELECT r.role_id, v_perm_id, 'seed'
    FROM mst_role r
    WHERE r.role_code = 'SUPER_ADMIN'
      AND v_perm_id IS NOT NULL
    ON CONFLICT DO NOTHING;
END $$;
