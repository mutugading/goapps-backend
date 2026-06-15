-- 000055 down: remove finance.costing.route.unlock permission.
DO $$
BEGIN
    DELETE FROM role_permissions rp
    USING mst_permission p
    WHERE rp.permission_id = p.permission_id
      AND p.permission_code = 'finance.costing.route.unlock';

    DELETE FROM mst_permission WHERE permission_code = 'finance.costing.route.unlock';
END $$;
