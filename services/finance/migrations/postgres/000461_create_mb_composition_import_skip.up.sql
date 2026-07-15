-- 000461: Audit skip-log for MB composition imports. Any composition row whose head, RM
-- group, or nested-MB reference cannot be resolved during a backfill/import is recorded
-- here (instead of being silently dropped), so still-missing masters are visible. Populated
-- by the 000463 composition seeder and re-populated on any future re-run after the user
-- imports the residual masters.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_mb_composition_import_skip (
  mcis_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mcis_period        VARCHAR(6)   NOT NULL,        -- e.g. '202606'
  mcis_head_sys_id   VARCHAR(30)  NOT NULL,        -- Oracle CMCI_CMBH_SYS_ID
  mcis_seq_no        INTEGER,                       -- Oracle CMCI_CMBI_SEC_NO
  mcis_group_sys_id  VARCHAR(30),                   -- Oracle CMCI_CGH_SYS_ID (may be empty for MB rows)
  mcis_item_code     VARCHAR(200),                  -- Oracle CMCI_CMBI_CODE (nested MB sys_id for MB rows)
  mcis_legacy_sys_id VARCHAR(30),                   -- Oracle CMCI_CMBI_SYS_ID
  mcis_reason        VARCHAR(50)  NOT NULL,         -- 'HEAD_NOT_FOUND' | 'GROUP_NOT_FOUND' | 'MB_REF_NOT_FOUND'
  mcis_created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mcis_reason ON mst_mb_composition_import_skip (mcis_reason);
CREATE INDEX IF NOT EXISTS idx_mcis_head   ON mst_mb_composition_import_skip (mcis_head_sys_id);
CREATE INDEX IF NOT EXISTS idx_mcis_period ON mst_mb_composition_import_skip (mcis_period);

COMMENT ON TABLE mst_mb_composition_import_skip IS
  'Records MB composition import rows that could not be resolved to a head/group/MB-ref, for visibility of still-missing masters.';

COMMIT;
