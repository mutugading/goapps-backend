-- Migration 000432: seed additional approval-visible params
-- Adds ORION_ITEM (item code, display order 5) and DELIVERY_NO_OF_BOB (no of bobbin, display order 220)
-- to the approval summary. Other requested param codes (lustre, quality grades by specific code,
-- type/production variants) are TBD pending DBA confirmation — add via a later migration once confirmed.

UPDATE mst_parameter SET
    is_approval_visible  = TRUE,
    approval_display_order = v.ord
FROM (VALUES
    ('ORION_ITEM',        5),   -- item code (before machine params)
    ('DELIVERY_NO_OF_BOB', 220) -- no of bobbin (after bobin weight 210)
) AS v(param_code, ord)
WHERE mst_parameter.param_code = v.param_code
  AND mst_parameter.deleted_at IS NULL;
