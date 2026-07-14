-- MB Costing Suite: lusture (finish/luster) lookup master.
CREATE TABLE IF NOT EXISTS mst_mb_lusture (
  mbl_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbl_code              VARCHAR(10) NOT NULL UNIQUE,
  mbl_display_name      VARCHAR(50),
  mbl_full_description  VARCHAR(200),
  mbl_category          VARCHAR(30),
  mbl_is_active         BOOLEAN NOT NULL DEFAULT TRUE,
  mbl_display_order     INTEGER,
  mbl_created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbl_created_by        VARCHAR(20) NOT NULL,
  mbl_updated_at        TIMESTAMPTZ,
  mbl_updated_by        VARCHAR(20),
  deleted_at            TIMESTAMPTZ,
  deleted_by            VARCHAR(20)
);

CREATE INDEX IF NOT EXISTS idx_mbl_active_order ON mst_mb_lusture (mbl_display_order) WHERE mbl_is_active = TRUE;

COMMENT ON TABLE mst_mb_lusture IS 'MB Costing Suite lusture (finish) lookup master — 54 seeded rows, see migration 000442';
