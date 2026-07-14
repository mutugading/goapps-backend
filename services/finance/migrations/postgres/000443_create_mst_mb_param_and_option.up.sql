-- MB Costing Suite: recipe parameter defaults + picklist options.
CREATE TABLE IF NOT EXISTS mst_mb_param (
  mbp_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbp_code              VARCHAR(30) NOT NULL UNIQUE,
  mbp_name              VARCHAR(100) NOT NULL,
  mbp_description       TEXT,
  mbp_type              VARCHAR(20) NOT NULL CHECK (mbp_type IN ('SCALAR','PICKLIST')),
  mbp_default_value     NUMERIC(20,6),
  mbp_default_option    VARCHAR(10),
  mbp_unit              VARCHAR(20),
  mbp_display_order     INTEGER,
  mbp_is_active         BOOLEAN NOT NULL DEFAULT TRUE,
  mbp_created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbp_created_by        VARCHAR(20) NOT NULL,
  mbp_updated_at        TIMESTAMPTZ,
  mbp_updated_by        VARCHAR(20),
  deleted_at            TIMESTAMPTZ,
  deleted_by            VARCHAR(20)
);

CREATE TABLE IF NOT EXISTS mst_mb_param_option (
  mbpo_id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbpo_mbp_code          VARCHAR(30) NOT NULL,
  mbpo_code              VARCHAR(10) NOT NULL,
  mbpo_numeric_value     NUMERIC(20,6) NOT NULL,
  mbpo_description       TEXT,
  mbpo_display_order     INTEGER,
  mbpo_is_active         BOOLEAN NOT NULL DEFAULT TRUE,
  deleted_at             TIMESTAMPTZ,
  deleted_by             VARCHAR(20),
  CONSTRAINT fk_mbpo_mbp FOREIGN KEY (mbpo_mbp_code) REFERENCES mst_mb_param (mbp_code) ON UPDATE CASCADE,
  CONSTRAINT uq_mbpo_mbp_code UNIQUE (mbpo_mbp_code, mbpo_code)
);

CREATE INDEX IF NOT EXISTS idx_mbpo_mbp_code ON mst_mb_param_option (mbpo_mbp_code);

COMMENT ON TABLE mst_mb_param IS 'MB Costing Suite recipe parameter defaults — 8 seeded rows, see migration 000444';
COMMENT ON TABLE mst_mb_param_option IS 'Picklist options for PICKLIST-type mst_mb_param rows';
