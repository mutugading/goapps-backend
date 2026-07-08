-- CAVEAT: this down migration drops cps_tube_type (any data written into it
-- while the .up migration was applied is irreversibly lost — there is no
-- destination column for it in the pre-migration schema) and restores the
-- original NOT NULL + FK constraint on cps_paper_tube_type_id. If any row
-- was inserted (or backfilled to NULL) while the .up migration was applied
-- — i.e. any request created via the revamped form that only populates
-- cps_tube_type and leaves the legacy cps_paper_tube_type_id unset — the
-- ALTER COLUMN ... SET NOT NULL below will fail with a "column contains
-- null values" error. Before rolling back in an environment with real data,
-- first verify there are no NULL rows and/or backfill a sentinel value,
-- e.g.:
--   SELECT count(*) FROM cost_product_spec WHERE cps_paper_tube_type_id IS NULL;
--   -- if any rows are returned, backfill before rolling back, e.g.:
--   UPDATE cost_product_spec SET cps_paper_tube_type_id = 5 -- 'PALLET (no tube)'
--     WHERE cps_paper_tube_type_id IS NULL;
-- This migration intentionally does NOT perform that backfill automatically
-- (silently inventing business data on rollback is worse than a loud
-- failure) — the operator must decide the correct sentinel/backfill value.

ALTER TABLE cost_product_spec DROP CONSTRAINT IF EXISTS chk_cps_tube_type;
ALTER TABLE cost_product_spec DROP COLUMN IF EXISTS cps_tube_type;

ALTER TABLE cost_product_spec
    ADD CONSTRAINT cost_product_spec_cps_paper_tube_type_id_fkey
    FOREIGN KEY (cps_paper_tube_type_id) REFERENCES cost_paper_tube_type (cptt_paper_tube_type_id) ON DELETE RESTRICT;

ALTER TABLE cost_product_spec ALTER COLUMN cps_paper_tube_type_id SET NOT NULL;

COMMENT ON COLUMN cost_product_spec.cps_paper_tube_type_id IS NULL;
