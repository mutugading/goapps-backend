DROP INDEX IF EXISTS idx_cpr_reference_product;
ALTER TABLE cost_product_request DROP COLUMN IF EXISTS cpr_reference_product_sys_id;
