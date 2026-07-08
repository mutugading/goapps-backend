-- Revert 000067: remove finance.product.request.review from CPR_SUBMITTER.
DELETE FROM role_permissions
WHERE role_id = (SELECT role_id FROM mst_role WHERE role_code = 'CPR_SUBMITTER')
  AND permission_id = (
    SELECT permission_id FROM mst_permission
    WHERE permission_code = 'finance.product.request.review'
  );
