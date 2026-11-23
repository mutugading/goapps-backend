-- IAM Service Database Migrations
-- 000105: Rollback payroll tax-status correction permission

DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code = 'payroll.master.taxstatus.update');

DELETE FROM mst_permission WHERE permission_code = 'payroll.master.taxstatus.update';
