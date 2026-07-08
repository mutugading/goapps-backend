-- Add cpr_reference_product_sys_id so a requester can optionally point a
-- new/edited request at an existing, similar product master as a routing
-- hint (per product-request-workflow-revamp design.md §2 D4). This is a
-- request-level field (not cost_product_spec) because "reference an
-- existing similar product" is orthogonal to whether the request itself is
-- for something new — it applies to all requests regardless of
-- classification. Distinct from cpr_existing_product_sys_id (migration
-- 000219), which records the product whose costing was actually reused
-- once classification is verified as EXISTING; this column is only a
-- reviewer-facing prefill hint set (optionally) at request creation/edit
-- time, before classification happens.
--
-- ON DELETE SET NULL (unlike 000219's unqualified FK, which defaults to
-- NO ACTION) since this is a soft hint, not a hard business fact — if the
-- referenced product master is ever deleted, the request should simply
-- lose the hint rather than block the master's deletion or blow up.

ALTER TABLE cost_product_request
    ADD COLUMN IF NOT EXISTS cpr_reference_product_sys_id BIGINT
        REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_cpr_reference_product
    ON cost_product_request(cpr_reference_product_sys_id)
    WHERE cpr_reference_product_sys_id IS NOT NULL;

COMMENT ON COLUMN cost_product_request.cpr_reference_product_sys_id IS
    'Optional reference to an existing similar product master, set by the requester at create/edit time to prefill routing suggestions during review. NULL when no reference was chosen. Distinct from cpr_existing_product_sys_id, which records the product whose costing was actually reused after classification.';
