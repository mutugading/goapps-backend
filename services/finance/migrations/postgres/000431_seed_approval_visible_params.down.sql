UPDATE mst_parameter
   SET is_approval_visible = FALSE,
       approval_display_order = NULL
 WHERE param_code IN (
    'MC_NAME', 'MC_EFFICIENCY', 'MC_SPEED', 'TPM',
    'DENIER', 'ACT_DENIER', 'NO_OF_PLY', 'NO_OF_FILAMENTS',
    'RM_TYPE', 'RAW_MATERIAL', 'CROSS_SECTION', 'INTERMINGLE',
    'WASTE_PERC', 'OPU', 'AX_PERC', 'AE_PERC', 'A9_PERC', 'A_PERC',
    'B_PERC', 'C_PERC', 'NET_BOB_WT'
 )
   AND deleted_at IS NULL;
