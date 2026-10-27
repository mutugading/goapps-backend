-- IAM Service Database Migrations
-- 000098: Rollback Payroll menu and permissions seed

DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'payroll.%');

DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0002-000000000028';

DELETE FROM mst_permission WHERE permission_code LIKE 'payroll.%';

-- Menu last: mst_permission.menu_id references it.
DELETE FROM mst_menu WHERE menu_code = 'HR_PAYROLL';

-- Restore the 43-value whitelist (main 000094). Safe: no payroll rows remain.
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke','bulkunvalidate','bulksubmit','bulkvalidate',
    'push','valuate','restore'
  ));
