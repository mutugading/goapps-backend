-- CAVEAT: this down migration blindly restores the original NOT NULL
-- constraints on cps_raw_material_type/cps_box_type/cps_weight_per_bobbin_kg.
-- If any row was inserted (or backfilled to NULL) while the .up migration
-- was applied — i.e. any request created via the revamped form that omits
-- these 3 fields — this ALTER COLUMN ... SET NOT NULL will fail with a
-- "column contains null values" error. Before rolling back in an environment
-- with real data, first verify there are no NULL rows and/or backfill a
-- sentinel value, e.g.:
--   SELECT count(*) FROM cost_product_spec
--     WHERE cps_raw_material_type IS NULL
--        OR cps_box_type IS NULL
--        OR cps_weight_per_bobbin_kg IS NULL;
--   -- if any rows are returned, backfill before rolling back, e.g.:
--   UPDATE cost_product_spec SET cps_raw_material_type = 'POY_BOUGHTOUT'
--     WHERE cps_raw_material_type IS NULL;
--   UPDATE cost_product_spec SET cps_box_type = 'NORMAL'
--     WHERE cps_box_type IS NULL;
--   UPDATE cost_product_spec SET cps_weight_per_bobbin_kg = 0.001
--     WHERE cps_weight_per_bobbin_kg IS NULL;
-- This migration intentionally does NOT perform that backfill automatically
-- (silently inventing business data on rollback is worse than a loud
-- failure) — the operator must decide the correct sentinel/backfill values.

ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_raw_material;
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_box_type;
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_weight_positive;

ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_raw_material
    CHECK (cps_raw_material_type IN ('POY_BOUGHTOUT', 'CHIPS_SD', 'CHIPS_BRT', 'CHIPS_RECYCLE'));
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_box_type
    CHECK (cps_box_type IN ('JUMBO', 'NORMAL', 'PALLET'));
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_weight_positive
    CHECK (cps_weight_per_bobbin_kg > 0);

ALTER TABLE cost_product_spec
    ALTER COLUMN cps_raw_material_type SET NOT NULL,
    ALTER COLUMN cps_box_type SET NOT NULL,
    ALTER COLUMN cps_weight_per_bobbin_kg SET NOT NULL;

COMMENT ON COLUMN cost_product_spec.cps_raw_material_type IS NULL;
COMMENT ON COLUMN cost_product_spec.cps_box_type IS NULL;
COMMENT ON COLUMN cost_product_spec.cps_weight_per_bobbin_kg IS NULL;
