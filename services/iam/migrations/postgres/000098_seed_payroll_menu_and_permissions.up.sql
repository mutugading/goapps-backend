-- IAM Service Database Migrations
-- 000098: Seed Payroll menu (HR > Payroll) and payroll.* permissions (T004, D-05)
--
-- Permission map comes from spec 07 (RBAC table) adapted to the IAM 4-segment
-- format {service}.{module}.{entity}.{action} (D-05). Full map and rationale:
-- goapps-backend/services/payroll/docs/permissions.md.
--
-- chk_permission_action WIDENED: 'commit' and 'close' are not in the live
-- whitelist (43 values after main 000094); payroll needs payroll.run.salary.commit,
-- payroll.period.salary.close and payroll.thr.run.close. 43 -> 45 values.
--
-- Menu: HR_PAYROLL is level 2 under HR (0001-...004), next free level-2 id is
-- ...0028 (highest seeded is ...0027, PPC_MASTERS). Gated by payroll.module.root.view
-- (a menu with no menu_permissions rows is visible to every user).
--
-- SUPER_ADMIN gets every payroll permission EXCEPT payroll.salary.amount.view:
-- spec 07 says admin must not see salary amounts (see D-12 PROPOSED).

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke','bulkunvalidate','bulksubmit','bulkvalidate',
    'push','valuate','restore','commit','close'
  ));

-- =============================================================================
-- 1. PERMISSIONS
-- =============================================================================
INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('payroll.module.root.view',         'View Payroll Module',        'Show the Payroll menu',                                   'payroll', 'module',     'view',    true, 'seed'),
    ('payroll.master.employee.view',     'View Payroll Employee',      'View payroll employees and their components',             'payroll', 'master',     'view',    true, 'seed'),
    ('payroll.master.employee.create',   'Create Payroll Employee',    'Create payroll employee',                                 'payroll', 'master',     'create',  true, 'seed'),
    ('payroll.master.employee.update',   'Update Payroll Employee',    'Update employee, components, level change, resign',       'payroll', 'master',     'update',  true, 'seed'),
    ('payroll.master.orionaccount.update','Update Orion Sub Account',  'Change employee Orion sub account (audited)',             'payroll', 'master',     'update',  true, 'seed'),
    ('payroll.rule.set.update',          'Update Rule Set',            'Edit payroll rule sets',                                  'payroll', 'rule',       'update',  true, 'seed'),
    ('payroll.rule.set.approve',         'Approve Rule Set',           'Activate rule set (4-eyes)',                              'payroll', 'rule',       'approve', true, 'seed'),
    ('payroll.input.addition.create',    'Create Addition',            'Create payroll addition input',                           'payroll', 'input',      'create',  true, 'seed'),
    ('payroll.input.addition.update',    'Update Addition',            'Update payroll addition input',                           'payroll', 'input',      'update',  true, 'seed'),
    ('payroll.attendance.data.sync',     'Sync Attendance',            'Sync attendance from HRIS',                               'payroll', 'attendance', 'sync',    true, 'seed'),
    ('payroll.loan.request.create',      'Create Loan Request',        'Create employee loan request',                            'payroll', 'loan',       'create',  true, 'seed'),
    ('payroll.loan.request.update',      'Update Loan Request',        'Update employee loan request',                            'payroll', 'loan',       'update',  true, 'seed'),
    ('payroll.loan.request.approve',     'Approve Loan Request',       'Approve employee loan request',                           'payroll', 'loan',       'approve', true, 'seed'),
    ('payroll.loan.exception.approve',   'Approve Loan Exception',     'Approve exception to limit/tenor/ratio/SP (HR Manager)',  'payroll', 'loan',       'approve', true, 'seed'),
    ('payroll.loan.inbound.execute',     'Loan Inbound (HRIS)',        'HRIS service: simulate and submit loan request',          'payroll', 'loan',       'execute', true, 'seed'),
    ('payroll.loan.disbursement.create', 'Record Loan Disbursement',   'Record loan disbursement',                                'payroll', 'loan',       'create',  true, 'seed'),
    ('payroll.loan.product.update',      'Update Loan Product',        'Edit loan products (4-eyes)',                             'payroll', 'loan',       'update',  true, 'seed'),
    ('payroll.loan.product.approve',     'Approve Loan Product',       'Approve loan product change (4-eyes)',                    'payroll', 'loan',       'approve', true, 'seed'),
    ('payroll.thr.event.create',         'Create THR Event',           'Create THR/Bonus event',                                  'payroll', 'thr',        'create',  true, 'seed'),
    ('payroll.thr.event.update',         'Update THR Event',           'Update THR/Bonus event and input',                        'payroll', 'thr',        'update',  true, 'seed'),
    ('payroll.thr.run.execute',          'Run THR',                    'Run THR/Bonus calculation',                               'payroll', 'thr',        'execute', true, 'seed'),
    ('payroll.thr.run.approve',          'Approve THR Run',            'Approve THR/Bonus run',                                   'payroll', 'thr',        'approve', true, 'seed'),
    ('payroll.thr.run.close',            'Close THR Run',              'Close THR/Bonus run',                                     'payroll', 'thr',        'close',   true, 'seed'),
    ('payroll.thr.scheme.update',        'Update THR Scheme',          'Edit THR/Bonus schemes (4-eyes)',                         'payroll', 'thr',        'update',  true, 'seed'),
    ('payroll.thr.scheme.approve',       'Approve THR Scheme',         'Approve THR/Bonus scheme change (4-eyes)',                'payroll', 'thr',        'approve', true, 'seed'),
    ('payroll.slip.mail.send',           'Send Payslip Email',         'Distribute payslips by email',                            'payroll', 'slip',       'send',    true, 'seed'),
    ('payroll.slip.file.export',         'Download Payslip',           'Download payslip (also needs payroll.salary.amount.view)','payroll', 'slip',       'export',  true, 'seed'),
    ('payroll.mail.config.update',       'Update Mail Config',         'SMTP accounts, templates, password policy',               'payroll', 'mail',       'update',  true, 'seed'),
    ('payroll.mail.config.approve',      'Approve Mail Config',        'Approve mail config change (4-eyes)',                     'payroll', 'mail',       'approve', true, 'seed'),
    ('payroll.run.dryrun.execute',       'Dry Run Payroll',            'Run payroll dry-run (no commit)',                         'payroll', 'run',        'execute', true, 'seed'),
    ('payroll.run.salary.commit',        'Commit Payroll Run',         'Commit payroll run',                                      'payroll', 'run',        'commit',  true, 'seed'),
    ('payroll.period.salary.approve',    'Approve Payroll Period',     'Approve payroll period',                                  'payroll', 'period',     'approve', true, 'seed'),
    ('payroll.period.salary.close',      'Close Payroll Period',       'Close payroll period',                                    'payroll', 'period',     'close',   true, 'seed'),
    ('payroll.period.salary.reopen',     'Reopen Payroll Period',      'Reopen a closed payroll period',                          'payroll', 'period',     'reopen',  true, 'seed'),
    ('payroll.report.data.view',         'View Payroll Reports',       'View payroll reports',                                    'payroll', 'report',     'view',    true, 'seed'),
    ('payroll.salary.amount.view',       'View Salary Amounts',        'See salary nominal values (not for admin)',               'payroll', 'salary',     'view',    true, 'seed'),
    ('payroll.audit.log.view',           'View Payroll Audit Log',     'View payroll audit log',                                  'payroll', 'audit',      'view',    true, 'seed'),
    ('payroll.formula.rule.update',      'Update Formula',             'Edit configurable formulas',                              'payroll', 'formula',    'update',  true, 'seed'),
    ('payroll.formula.rule.approve',     'Approve Formula',            'Approve formula change (4-eyes, approver != creator)',    'payroll', 'formula',    'approve', true, 'seed'),
    ('payroll.subsidy.rule.update',      'Update Subsidy Rule',        'Edit subsidy rules',                                      'payroll', 'subsidy',    'update',  true, 'seed'),
    ('payroll.subsidy.rule.approve',     'Approve Subsidy Rule',       'Approve subsidy rule change (4-eyes)',                    'payroll', 'subsidy',    'approve', true, 'seed'),
    ('payroll.fx.rate.update',           'Update FX Rate',             'Maintain FX rates',                                       'payroll', 'fx',         'update',  true, 'seed'),
    ('payroll.gl.setup.update',          'Update GL Setup',            'Edit GL setup (4-eyes)',                                  'payroll', 'gl',         'update',  true, 'seed'),
    ('payroll.gl.export.execute',        'Export GL Journal',          'Export payroll GL journal',                               'payroll', 'gl',         'execute', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. MENU — HR > Payroll
-- =============================================================================
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000028', '00000000-0000-0000-0001-000000000004', 'HR_PAYROLL', 'Payroll', '/hr/payroll', 'Wallet', 'payroll', 2, 20, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m, mst_permission p
WHERE m.menu_code = 'HR_PAYROLL' AND p.permission_code = 'payroll.module.root.view' AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- 3. BACKFILL permission.menu_id (convention from 000066)
-- =============================================================================
UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'HR_PAYROLL'
  AND p.permission_code LIKE 'payroll.%'
  AND p.menu_id IS NULL;

-- =============================================================================
-- 4. SUPER_ADMIN — all payroll permissions except salary amounts (spec 07)
-- =============================================================================
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'payroll.%'
  AND p.permission_code <> 'payroll.salary.amount.view'
  AND r.is_active = true
  AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
