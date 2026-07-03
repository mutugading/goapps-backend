DROP INDEX IF EXISTS idx_mst_parameter_approval_visible;

ALTER TABLE mst_parameter
    DROP COLUMN IF EXISTS approval_display_order,
    DROP COLUMN IF EXISTS is_approval_visible;
