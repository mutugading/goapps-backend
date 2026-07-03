UPDATE mst_parameter SET
    is_approval_visible    = FALSE,
    approval_display_order = NULL
WHERE param_code IN ('ORION_ITEM', 'DELIVERY_NO_OF_BOB')
  AND deleted_at IS NULL;
