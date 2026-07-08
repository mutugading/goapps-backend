-- Relax cost_product_spec's raw-material-type/box-type/weight-per-bobbin
-- columns: these 3 fields are being removed from the create/edit form (per
-- product-request-workflow-revamp design.md §2 D1), but the columns are kept
-- (user decision: "biarkan kolom ada, cuma stop diisi dari form baru" — keep
-- the columns, just stop populating them from the new form). Since they were
-- NOT NULL with no DEFAULT, new rows that omit them would otherwise fail
-- INSERT — so drop NOT NULL and re-add each CHECK as "NULL OR valid" so NULL
-- now means "not collected", while any value that IS provided (e.g. a manual
-- backfill of historical data) is still constrained to the original allowed
-- set.

ALTER TABLE cost_product_spec
    ALTER COLUMN cps_raw_material_type DROP NOT NULL,
    ALTER COLUMN cps_box_type DROP NOT NULL,
    ALTER COLUMN cps_weight_per_bobbin_kg DROP NOT NULL;

ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_raw_material;
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_box_type;
ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_weight_positive;

ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_raw_material
    CHECK (cps_raw_material_type IS NULL OR cps_raw_material_type IN ('POY_BOUGHTOUT', 'CHIPS_SD', 'CHIPS_BRT', 'CHIPS_RECYCLE'));
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_box_type
    CHECK (cps_box_type IS NULL OR cps_box_type IN ('JUMBO', 'NORMAL', 'PALLET'));
ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_weight_positive
    CHECK (cps_weight_per_bobbin_kg IS NULL OR cps_weight_per_bobbin_kg > 0);

COMMENT ON COLUMN cost_product_spec.cps_raw_material_type IS 'Deprecated for new writes (product-request-workflow-revamp D1) — retained for historical rows. Nullable.';
COMMENT ON COLUMN cost_product_spec.cps_box_type IS 'Deprecated for new writes (product-request-workflow-revamp D1) — retained for historical rows. Nullable.';
COMMENT ON COLUMN cost_product_spec.cps_weight_per_bobbin_kg IS 'Deprecated for new writes (product-request-workflow-revamp D1) — retained for historical rows. Nullable.';
