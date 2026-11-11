-- 000607 - MB spin <-> FG product link + supersede (topic mb-spin-fg-poy-link).
-- Schema only, additive: new nullable columns, CHECK, indexes and the per-row
-- supersede audit table. No data writes (backfill is a separate reviewed command).

BEGIN;

-- 1. mst_mb_head: chosen FG + supersede markers.
ALTER TABLE mst_mb_head
    ADD COLUMN IF NOT EXISTS mbh_fg_product_id BIGINT NULL
        REFERENCES cost_product_master(cpm_product_sys_id),
    ADD COLUMN IF NOT EXISTS mbh_fg_item_code VARCHAR(50) NULL,
    ADD COLUMN IF NOT EXISTS mbh_superseded_by_head_id UUID NULL
        REFERENCES mst_mb_head(mbh_id),
    ADD COLUMN IF NOT EXISTS mbh_superseded_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS mbh_superseded_by VARCHAR(100) NULL;

CREATE INDEX IF NOT EXISTS idx_mbh_fg_product_id
    ON mst_mb_head (mbh_fg_product_id) WHERE deleted_at IS NULL;

-- 2. mst_mb_spin: FG link + supersede markers.
ALTER TABLE mst_mb_spin
    ADD COLUMN IF NOT EXISTS mbs_fg_product_id BIGINT NULL
        REFERENCES cost_product_master(cpm_product_sys_id),
    ADD COLUMN IF NOT EXISTS mbs_fg_item_code VARCHAR(50) NULL,
    ADD COLUMN IF NOT EXISTS mbs_fg_link_source VARCHAR(20) NULL,
    ADD COLUMN IF NOT EXISTS mbs_superseded_by_spin_id UUID NULL
        REFERENCES mst_mb_spin(mbs_id),
    ADD COLUMN IF NOT EXISTS mbs_superseded_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS mbs_superseded_by VARCHAR(100) NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_mbs_fg_link_source') THEN
        ALTER TABLE mst_mb_spin ADD CONSTRAINT chk_mbs_fg_link_source
            CHECK (mbs_fg_link_source IS NULL
                   OR mbs_fg_link_source IN ('RECIPE','DUPLICATE','MANUAL','BACKFILL')) NOT VALID;
    END IF;
END $$;

ALTER TABLE mst_mb_spin VALIDATE CONSTRAINT chk_mbs_fg_link_source;

-- Live spin identity: (normalized shade, FG). Legacy rows (FG NULL) are not checked.
CREATE UNIQUE INDEX IF NOT EXISTS uix_mbs_shade_fg_live
    ON mst_mb_spin (UPPER(TRIM(mbs_shade_code)), mbs_fg_product_id)
    WHERE mbs_fg_product_id IS NOT NULL
      AND deleted_at IS NULL
      AND mbs_is_active
      AND mbs_superseded_by_spin_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_mbs_fg_product_id
    ON mst_mb_spin (mbs_fg_product_id) WHERE deleted_at IS NULL;

-- FK lookup for un-supersede.
CREATE INDEX IF NOT EXISTS idx_mbs_superseded_by_spin_id
    ON mst_mb_spin (mbs_superseded_by_spin_id) WHERE mbs_superseded_by_spin_id IS NOT NULL;

-- 3. Per-row supersede audit (design Revisi 1 D8).
CREATE TABLE IF NOT EXISTS mst_mb_spin_supersede_item (
    mssi_id             BIGSERIAL PRIMARY KEY,
    mssi_batch_id       UUID        NOT NULL,
    mssi_old_spin_id    UUID        NOT NULL REFERENCES mst_mb_spin(mbs_id),
    mssi_new_spin_id    UUID        NOT NULL REFERENCES mst_mb_spin(mbs_id),
    mssi_cpp_value_id   BIGINT      NOT NULL REFERENCES cost_product_parameter(cpp_value_id) ON DELETE CASCADE,
    mssi_product_sys_id BIGINT      NOT NULL,
    mssi_old_value_text TEXT,
    mssi_old_filled_by  VARCHAR(100),
    mssi_new_value_text TEXT,
    mssi_reverted_at    TIMESTAMPTZ,
    mssi_reverted_by    VARCHAR(100),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by          VARCHAR(100) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_mssi_batch ON mst_mb_spin_supersede_item (mssi_batch_id);
CREATE INDEX IF NOT EXISTS idx_mssi_old_spin
    ON mst_mb_spin_supersede_item (mssi_old_spin_id) WHERE mssi_reverted_at IS NULL;

COMMENT ON TABLE mst_mb_spin_supersede_item IS
    'Per-row audit of cost_product_parameter rows repointed by a spin supersede action (1 batch = 1 action). Used by un-supersede to restore old values per batch (design D8).';

COMMIT;
