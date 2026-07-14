-- Widen chk_permission_action: append validate/unapprove/revoke/preview/execute (30 → 35 values)
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute'
  ));

-- MB Head workflow permissions
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
  (gen_random_uuid(), 'finance.mb.head.create','Create MB Head','Create MB head recipe','finance','mb','create',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.view','View MB Head','View MB head recipe','finance','mb','view',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.update','Update MB Head','Update MB head recipe','finance','mb','update',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.delete','Delete MB Head','Delete MB head recipe','finance','mb','delete',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.submit','Submit MB Head','Submit MB head recipe for validation','finance','mb','submit',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.approve','Approve MB Head','Approve MB head recipe','finance','mb','approve',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.validate','Validate MB Head','Validate MB head recipe','finance','mb','validate',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.unapprove','Un-approve MB Head','Revert MB head recipe approval','finance','mb','unapprove',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.head.revoke','Revoke MB Head','Revoke MB head recipe validation','finance','mb','revoke',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.pushtohead.preview','Preview Push-to-Head','Preview push of MB costs to product head','finance','mb','preview',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.pushtohead.execute','Execute Push-to-Head','Execute push of MB costs to product head','finance','mb','execute',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mblusture.view','View MB Lusture','View MB lusture master data','finance','master','view',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mblusture.create','Create MB Lusture','Create MB lusture master data','finance','master','create',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mblusture.update','Update MB Lusture','Update MB lusture master data','finance','master','update',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mblusture.delete','Delete MB Lusture','Delete MB lusture master data','finance','master','delete',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mbparam.view','View MB Param','View MB parameter master data','finance','master','view',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mbparam.create','Create MB Param','Create MB parameter master data','finance','master','create',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mbparam.update','Update MB Param','Update MB parameter master data','finance','master','update',TRUE,'seed'),
  (gen_random_uuid(), 'finance.master.mbparam.delete','Delete MB Param','Delete MB parameter master data','finance','master','delete',TRUE,'seed'),
  (gen_random_uuid(), 'finance.mb.batch.trigger','Trigger MB Batch','Trigger MB_BATCH cost compute for validated MB heads','finance','mb','trigger',TRUE,'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- Convenience roles
INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by) VALUES
  (gen_random_uuid(), 'MB_DRAFTER','MB Recipe Drafter','Drafts and submits MB head recipes',FALSE,TRUE,'seed'),
  (gen_random_uuid(), 'MB_APPROVER','MB Recipe Approver','Approves MB head recipes',FALSE,TRUE,'seed'),
  (gen_random_uuid(), 'MB_VALIDATOR','MB Recipe Validator','Validates MB head recipes and executes push-to-head',FALSE,TRUE,'seed')
ON CONFLICT (role_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_DRAFTER' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.mb.head.create','finance.mb.head.view','finance.mb.head.update','finance.mb.head.submit',
  'finance.master.mblusture.view','finance.master.mbparam.view'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_APPROVER' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.mb.head.view','finance.mb.head.approve','finance.mb.head.unapprove',
  'finance.master.mblusture.view','finance.master.mbparam.view'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_VALIDATOR' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.mb.head.view','finance.mb.head.validate','finance.mb.head.revoke',
  'finance.mb.pushtohead.preview','finance.mb.pushtohead.execute','finance.mb.batch.trigger',
  'finance.master.mblusture.view','finance.master.mbparam.view'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- SUPER_ADMIN gets everything explicitly (parenthesized OR chain — see resolution note above)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = TRUE AND p.is_active = TRUE
  AND (p.permission_code LIKE 'finance.mb.%' OR p.permission_code LIKE 'finance.master.mblusture.%' OR p.permission_code LIKE 'finance.master.mbparam.%')
ON CONFLICT (role_id, permission_id) DO NOTHING;
