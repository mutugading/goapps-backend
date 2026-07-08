-- CAVEAT: this down migration drops cps_shade_name and renames
-- cps_shade_code back to cps_shade_custom_text. Any data written into
-- cps_shade_name while the .up migration was applied is irreversibly lost —
-- there is no destination column for it in the pre-migration schema.

ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_shade_present;

ALTER TABLE cost_product_spec DROP COLUMN IF EXISTS cps_shade_name;
ALTER TABLE cost_product_spec RENAME COLUMN cps_shade_code TO cps_shade_custom_text;

ALTER TABLE cost_product_spec ADD CONSTRAINT chk_cps_shade_present CHECK (
    cps_shade_id IS NOT NULL OR cps_shade_custom_text IS NOT NULL
);

COMMENT ON COLUMN cost_product_spec.cps_shade_custom_text IS NULL;
