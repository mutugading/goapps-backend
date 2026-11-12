-- 000101 (DOWN): remove the two MB spin/FG permissions and their grants.
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code IN ('finance.product.master.sync', 'finance.yarnmaster.mbspin.restore'));
DELETE FROM mst_permission WHERE permission_code IN ('finance.product.master.sync', 'finance.yarnmaster.mbspin.restore');
COMMIT;
