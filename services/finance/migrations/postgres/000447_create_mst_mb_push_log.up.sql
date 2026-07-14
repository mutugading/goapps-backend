CREATE TABLE IF NOT EXISTS mst_mb_push_log (
  mbpl_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbpl_period          VARCHAR(6) NOT NULL,
  mbpl_pushed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbpl_pushed_by       VARCHAR(20) NOT NULL,
  mbpl_mb_count        INTEGER NOT NULL,
  mbpl_row_count       INTEGER NOT NULL,
  mbpl_cost_types      VARCHAR(50) NOT NULL,
  mbpl_previous_period VARCHAR(6),
  mbpl_snapshot        JSONB,
  mbpl_notes           TEXT
);

CREATE INDEX IF NOT EXISTS idx_mbpl_period ON mst_mb_push_log (mbpl_period);
CREATE INDEX IF NOT EXISTS idx_mbpl_pushed_at ON mst_mb_push_log (mbpl_pushed_at DESC);

COMMENT ON TABLE mst_mb_push_log IS 'Audit trail of Push-to-Head batch executions, one row per execute call';
