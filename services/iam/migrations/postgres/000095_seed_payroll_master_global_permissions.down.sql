-- IAM Service Database Migrations
-- 000095: Rollback payroll company/currency permissions

DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission
  WHERE permission_code LIKE 'payroll.master.company.%' OR permission_code LIKE 'payroll.master.currency.%');

DELETE FROM mst_permission
WHERE permission_code LIKE 'payroll.master.company.%' OR permission_code LIKE 'payroll.master.currency.%';
