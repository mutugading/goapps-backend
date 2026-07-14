-- MB Costing Suite: marks auto-generated MB products + enforces PRD §10 hard-lock.
ALTER TABLE cost_product_master
  ADD COLUMN IF NOT EXISTS cpm_source VARCHAR(30),
  ADD COLUMN IF NOT EXISTS cpm_is_locked BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_cpm_source ON cost_product_master (cpm_source) WHERE cpm_source IS NOT NULL;

COMMENT ON COLUMN cost_product_master.cpm_source IS 'e.g. MB_RECIPE for auto-generated MB products; NULL for manually-created products';
COMMENT ON COLUMN cost_product_master.cpm_is_locked IS 'TRUE blocks manual edits to route/params outside the MB recipe workflow; cleared via finance-admin escape hatch with 24h auto-relock';

-- Escape-hatch audit trail
CREATE TABLE IF NOT EXISTS mst_mb_lock_log (
  mbll_id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbll_cost_product_id    BIGINT NOT NULL,
  mbll_unlocked_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbll_unlocked_by        VARCHAR(20) NOT NULL,
  mbll_reason             TEXT NOT NULL,
  mbll_auto_relock_at     TIMESTAMPTZ NOT NULL,
  mbll_relocked_at        TIMESTAMPTZ,
  mbll_relocked_by        VARCHAR(20),
  mbll_manual_edits       JSONB,
  CONSTRAINT fk_mbll_cpm FOREIGN KEY (mbll_cost_product_id) REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mbll_cost_product ON mst_mb_lock_log (mbll_cost_product_id);
CREATE INDEX IF NOT EXISTS idx_mbll_auto_relock ON mst_mb_lock_log (mbll_auto_relock_at) WHERE mbll_relocked_at IS NULL;

COMMENT ON TABLE mst_mb_lock_log IS 'Audit trail of cost_product_master.cpm_is_locked escape-hatch unlocks, with 24h auto-relock deadline';
