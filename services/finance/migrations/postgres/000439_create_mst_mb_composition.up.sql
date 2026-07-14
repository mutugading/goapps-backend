-- MB Costing Suite: working-set composition (RM-group / MB / carrier discriminator),
-- editable only while mst_mb_head.mbh_entry_status = 'DRAFT'.
CREATE TABLE IF NOT EXISTS mst_mb_composition (
  mbcm_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbcm_mbh_id            UUID NOT NULL,
  mbcm_seq_no            INTEGER NOT NULL,
  mbcm_group_head_id     UUID NOT NULL,
  mbcm_composition_pct   NUMERIC(6,3) NOT NULL,
  mbcm_source_type       VARCHAR(20) NOT NULL DEFAULT 'GROUP'
    CHECK (mbcm_source_type IN ('GROUP','MB','CARRIER')),
  mbcm_mb_ref_mbh_id     UUID,
  mbcm_is_carrier        BOOLEAN NOT NULL DEFAULT FALSE,
  mbcm_legacy_sys_id     VARCHAR(30),
  mbcm_created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbcm_created_by        VARCHAR(20) NOT NULL,
  mbcm_updated_at        TIMESTAMPTZ,
  mbcm_updated_by        VARCHAR(20),
  deleted_at             TIMESTAMPTZ,
  deleted_by             VARCHAR(20),
  CONSTRAINT fk_mbcm_mbh FOREIGN KEY (mbcm_mbh_id)
    REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE,
  CONSTRAINT fk_mbcm_group FOREIGN KEY (mbcm_group_head_id)
    REFERENCES cst_rm_group_head (group_head_id),
  CONSTRAINT fk_mbcm_mb_ref FOREIGN KEY (mbcm_mb_ref_mbh_id)
    REFERENCES mst_mb_head (mbh_id),
  CONSTRAINT chk_mbcm_composition CHECK (mbcm_composition_pct >= 0 AND mbcm_composition_pct <= 100),
  CONSTRAINT uq_mbcm_seq UNIQUE (mbcm_mbh_id, mbcm_seq_no)
);

CREATE INDEX IF NOT EXISTS idx_mbcm_mbh_id ON mst_mb_composition (mbcm_mbh_id);
CREATE INDEX IF NOT EXISTS idx_mbcm_group_head_id ON mst_mb_composition (mbcm_group_head_id);

COMMENT ON TABLE mst_mb_composition IS 'MB recipe working-set composition rows; editable only while parent mst_mb_head.mbh_entry_status = DRAFT';
COMMENT ON COLUMN mst_mb_composition.mbcm_source_type IS 'GROUP = RM-group %, MB = nested MB reference, CARRIER = carrier-only row';
