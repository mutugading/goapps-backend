-- MB Costing Suite: immutable composition snapshot, written once per VALIDATED transition.
CREATE TABLE IF NOT EXISTS mst_mb_composition_version (
  mbcv_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbcv_mbh_id            UUID NOT NULL,
  mbcv_version           INTEGER NOT NULL,
  mbcv_validated_at      TIMESTAMPTZ NOT NULL,
  mbcv_validated_by      VARCHAR(20) NOT NULL,
  mbcv_seq_no            INTEGER NOT NULL,
  mbcv_group_head_id     UUID NOT NULL,
  mbcv_composition_pct   NUMERIC(6,3) NOT NULL,
  mbcv_source_type       VARCHAR(20) NOT NULL,
  mbcv_mb_ref_mbh_id     UUID,
  mbcv_is_carrier        BOOLEAN NOT NULL DEFAULT FALSE,
  CONSTRAINT fk_mbcv_mbh FOREIGN KEY (mbcv_mbh_id)
    REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE,
  CONSTRAINT uq_mbcv_seq UNIQUE (mbcv_mbh_id, mbcv_version, mbcv_seq_no)
);

CREATE INDEX IF NOT EXISTS idx_mbcv_mbh_version ON mst_mb_composition_version (mbcv_mbh_id, mbcv_version);

COMMENT ON TABLE mst_mb_composition_version IS 'Append-only snapshot of mst_mb_composition, one full copy per VALIDATED transition — never updated or deleted';
