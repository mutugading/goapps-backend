DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code = 'finance.yarnmaster.lookupmaster.update');
DELETE FROM mst_permission WHERE permission_code = 'finance.yarnmaster.lookupmaster.update';
