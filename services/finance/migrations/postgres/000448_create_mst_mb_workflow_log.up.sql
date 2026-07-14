CREATE TABLE IF NOT EXISTS mst_mb_workflow_log (
  mbwl_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbwl_mbh_id        UUID NOT NULL,
  mbwl_from_state    VARCHAR(20),
  mbwl_to_state      VARCHAR(20) NOT NULL,
  mbwl_actor_user_id VARCHAR(20) NOT NULL,
  mbwl_actor_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbwl_reason        TEXT,
  mbwl_version       INTEGER,
  mbwl_meta          JSONB,
  CONSTRAINT fk_mbwl_mbh FOREIGN KEY (mbwl_mbh_id) REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mbwl_mbh_at ON mst_mb_workflow_log (mbwl_mbh_id, mbwl_actor_at DESC);

COMMENT ON TABLE mst_mb_workflow_log IS 'Audit trail of every mst_mb_head.mbh_entry_status transition — reason required for REVOKED';
