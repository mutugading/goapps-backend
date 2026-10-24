-- IAM Service Database Migrations
-- 000096: Rollback payroll component/template permissions

DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission
  WHERE permission_code LIKE 'payroll.master.component.%' OR permission_code LIKE 'payroll.master.template.%');

DELETE FROM mst_permission
WHERE permission_code LIKE 'payroll.master.component.%' OR permission_code LIKE 'payroll.master.template.%';
