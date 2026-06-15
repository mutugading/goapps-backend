-- 000056 down: remove finance.costing.paramvalue.update permission.
DELETE FROM role_permissions
WHERE permission_id = (
    SELECT permission_id FROM mst_permission
    WHERE permission_code = 'finance.costing.paramvalue.update'
);

DELETE FROM mst_permission WHERE permission_code = 'finance.costing.paramvalue.update';
