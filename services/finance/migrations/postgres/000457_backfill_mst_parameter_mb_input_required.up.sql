-- Migration 000444 seeded the 11 MB_* INPUT mst_parameter rows without setting
-- is_required_for_costing (relying on the column default FALSE). This doesn't affect the
-- existing MB product (mbFreezeCostParams sets capp_is_required=TRUE explicitly at the
-- per-product CAPP row, independent of this master flag), but the master flag drives the
-- "Add Parameter" dialog's default Req-toggle state for any future manual setup of an MB
-- product outside the auto-gen path. Backfill to TRUE to match mbBuildParamValues' actual
-- required-input set (mb_autogen_repository.go:29-39).
UPDATE mst_parameter
SET is_required_for_costing = TRUE
WHERE param_category = 'INPUT'
  AND param_code IN (
    'MB_WASTE','MB_QUALITY_LOSS','MB_EFFICIENCY','MB_DEV_EXPENSE','MB_PACKING',
    'MB_PROD_PER_DAY','MB_THROUGHPUT','MB_NO_PROCESS','IS_BOUGHTOUT',
    'MACHINE_MB_FIXED_TOTAL','MB_COMPOSITION_VERSION'
  )
  AND deleted_at IS NULL;
