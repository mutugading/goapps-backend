-- Extend mst_parameter with approval-review visibility metadata (item #4 —
-- approval-scoped parameter subset shown to the fill-approver before they
-- approve a fill task). Additive and nullable/defaulted so existing rows
-- remain valid.

ALTER TABLE mst_parameter
    ADD COLUMN IF NOT EXISTS is_approval_visible    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS approval_display_order INT;

COMMENT ON COLUMN mst_parameter.is_approval_visible    IS 'TRUE = shown (read-only) to the fill-approver in the approval review drawer before they approve a fill task.';
COMMENT ON COLUMN mst_parameter.approval_display_order IS 'Render order within the approval review drawer. NULL when is_approval_visible is FALSE.';

CREATE INDEX IF NOT EXISTS idx_mst_parameter_approval_visible
    ON mst_parameter(is_approval_visible, approval_display_order)
    WHERE is_approval_visible = TRUE;
