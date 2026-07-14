DELETE FROM role_permissions WHERE permission_id IN (
  SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'finance.mb.%'
     OR permission_code LIKE 'finance.master.mblusture.%' OR permission_code LIKE 'finance.master.mbparam.%'
);
DELETE FROM mst_role WHERE role_code IN ('MB_DRAFTER','MB_APPROVER','MB_VALIDATOR');
DELETE FROM mst_permission WHERE permission_code LIKE 'finance.mb.%'
   OR permission_code LIKE 'finance.master.mblusture.%' OR permission_code LIKE 'finance.master.mbparam.%';
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm'
  ));
