-- 000392: Align mst_parameter data with Excel spec (1_mst_parameter sheet).
-- 1. Add notes column (formula/description text from Excel).
-- 2. Fix display_group + display_order for misclassified params.
-- 3. Set owner_department, is_required_for_costing, is_period_dependent for CALCULATED params.
-- 4. Set uom_id for CALCULATED params that carry a unit (all KG).
-- 5. Populate notes from Excel spec.

BEGIN;

-- 1. Add notes column (idempotent).
ALTER TABLE mst_parameter ADD COLUMN IF NOT EXISTS notes VARCHAR(500);

-- 2a. STEAM_RATE and WATER_RATE were seeded under 'Utilities' in 000381 but Excel places them in 'Rates'.
UPDATE mst_parameter SET display_group = 'Rates', display_order = 80
WHERE param_code = 'STEAM_RATE' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'Rates', display_order = 90
WHERE param_code = 'WATER_RATE' AND deleted_at IS NULL;

-- 2b. COST_ELEC, COST_LABOR, COST_CONVERSION were seeded under 'Conversion' in 000381
--     but Excel places them in 'CostOutput'.
UPDATE mst_parameter SET display_group = 'CostOutput', display_order = 30
WHERE param_code = 'COST_ELEC' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'CostOutput', display_order = 40
WHERE param_code = 'COST_LABOR' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'CostOutput', display_order = 50
WHERE param_code = 'COST_CONVERSION' AND deleted_at IS NULL;

-- 3a. Engineering-owned CALCULATED params (owner per Excel column E).
UPDATE mst_parameter SET
    owner_department         = 'Engineering',
    is_required_for_costing  = FALSE,
    is_period_dependent      = FALSE
WHERE param_code IN (
    'DRAW_RATIO',
    'RM_NORMS',
    'AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT', 'NET_BOB_WT',
    'CAP_BOX_WT',
    'DEL_BOX_WT',
    'BATCH_WEIGHT',
    'RP_DOZING',
    'NET_PRODUCTION'
) AND param_category = 'CALCULATED' AND deleted_at IS NULL;

-- 3b. Finance-owned CALCULATED params.
UPDATE mst_parameter SET
    owner_department         = 'Finance',
    is_required_for_costing  = FALSE,
    is_period_dependent      = FALSE
WHERE param_code IN (
    'CAP_PACK_COST',
    'DEL_PACK_COST',
    'HEATSET_COST_KG',
    'RM_LANDED_COST',
    'OIL_COST',
    'MB_COST',
    'INTERMINGLE_COST',
    'POWER_PER_KG', 'MANPOWER_PER_KG', 'OVERHEAD_PER_KG', 'SPARES_PER_KG', 'TOTAL_FIXED_COST',
    'CONV_CAP_EX_MB', 'CONV_DEL_EX_MB', 'CAP_COST_PRE_QL', 'DEL_COST_PRE_QL',
    'NON_STD_LOSS', 'BC_LOSS_CAP', 'BC_LOSS_DEL', 'QLOSS_CAP', 'QLOSS_DEL',
    'COST_CAP_FINAL', 'COST_DEL_FINAL',
    'COST_ELEC', 'COST_LABOR', 'COST_CONVERSION',
    'VB1_LOSS', 'VB2_LOSS', 'VB3_LOSS', 'VB4_LOSS', 'VB5_LOSS',
    'VB1_DEL_COST', 'VB2_DEL_COST', 'VB3_DEL_COST', 'VB4_DEL_COST', 'VB5_DEL_COST'
) AND param_category = 'CALCULATED' AND deleted_at IS NULL;

-- 4. Set uom_id for CALCULATED params that carry KG unit per Excel.
UPDATE mst_parameter SET
    uom_id = (SELECT uom_id FROM mst_uom WHERE uom_code = 'KG' AND deleted_at IS NULL LIMIT 1)
WHERE param_code IN (
    'AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT', 'NET_BOB_WT',
    'CAP_BOX_WT', 'DEL_BOX_WT', 'BATCH_WEIGHT', 'NET_PRODUCTION'
) AND param_category = 'CALCULATED' AND deleted_at IS NULL;

-- 6. Fix param_name and param_short_name to match Excel spec exactly.
UPDATE mst_parameter SET param_name = v.name, param_short_name = v.short
FROM (VALUES
    -- INPUT params: em-dash added and parentheses corrected
    ('VOL_BUCKET_1_QTY', 'Volume Bucket 1 — Qty threshold',      'VB1 Qty'),
    ('VOL_BUCKET_2_QTY', 'Volume Bucket 2 — Qty threshold',      'VB2 Qty'),
    ('VOL_BUCKET_3_QTY', 'Volume Bucket 3 — Qty threshold',      'VB3 Qty'),
    ('VOL_BUCKET_4_QTY', 'Volume Bucket 4 — Qty threshold',      'VB4 Qty'),
    ('VOL_BUCKET_5_QTY', 'Volume Bucket 5 — Qty threshold',      'VB5 Qty'),
    ('RAW_MATERIAL',     'Raw Material Reference (link)',         'RM Ref'),
    ('ELEC_KWH',         'Electricity Consumption (kWh/kg)',      'Elec kWh'),
    ('STEAM_RATE',       'Steam Rate (USD/kg)',                   'Steam Rate'),
    ('WATER_RATE',       'Water Rate (USD/m3)',                   'Water Rate'),
    ('LABOR_OVERHEAD_PCT','Labor Overhead % (benefits/HSE)',      'Labor OH%'),
    ('MAT_OVERHEAD_PCT', 'Material Handling Overhead %',         'Mat OH%'),
    -- CALCULATED params: full names from Excel
    ('RM_NORMS',         'Raw Material Consumption Norms',        'RM Norms'),
    ('CONV_CAP_EX_MB',   'Conversion + Captive Pack (excl MB)',   'Conv Cap'),
    ('CONV_DEL_EX_MB',   'Conversion + Delivery Pack (excl MB)',  'Conv Del'),
    ('CAP_COST_PRE_QL',  'Captive Cost Before Quality Loss',      'Cap Pre-QL'),
    ('DEL_COST_PRE_QL',  'Delivery Cost Before Quality Loss',     'Del Pre-QL'),
    ('NON_STD_LOSS',     'Non-Standard Value Loss/kg (USD)',       'Non-Std Loss'),
    ('BC_LOSS_CAP',      'BC Value Loss — Captive/kg (USD)',      'BC Loss Cap'),
    ('BC_LOSS_DEL',      'BC Value Loss — Delivery/kg (USD)',     'BC Loss Del'),
    ('QLOSS_CAP',        'Quality Loss — Captive/kg (USD)',       'QLoss Cap'),
    ('QLOSS_DEL',        'Quality Loss — Delivery/kg (USD)',      'QLoss Del'),
    ('COST_ELEC',        'Electricity Cost/kg (USD)',              'Elec Cost'),
    ('COST_LABOR',       'Labor Cost/kg (USD)',                    'Labor Cost'),
    ('COST_CONVERSION',  'Total Conversion Cost/kg (USD)',         'Conv Cost'),
    ('VB1_LOSS',         'Volume Bucket 1 — Change-Over Loss/kg', 'VB1 Loss'),
    ('VB2_LOSS',         'Volume Bucket 2 — Change-Over Loss/kg', 'VB2 Loss'),
    ('VB3_LOSS',         'Volume Bucket 3 — Change-Over Loss/kg', 'VB3 Loss'),
    ('VB4_LOSS',         'Volume Bucket 4 — Change-Over Loss/kg', 'VB4 Loss'),
    ('VB5_LOSS',         'Volume Bucket 5 — Change-Over Loss/kg', 'VB5 Loss'),
    ('VB1_DEL_COST',     'Volume Bucket 1 — Delivery Cost/kg',    'VB1 Cost'),
    ('VB2_DEL_COST',     'Volume Bucket 2 — Delivery Cost/kg',    'VB2 Cost'),
    ('VB3_DEL_COST',     'Volume Bucket 3 — Delivery Cost/kg',    'VB3 Cost'),
    ('VB4_DEL_COST',     'Volume Bucket 4 — Delivery Cost/kg',    'VB4 Cost'),
    ('VB5_DEL_COST',     'Volume Bucket 5 — Delivery Cost/kg',    'VB5 Cost')
) AS v(param_code, name, short)
WHERE mst_parameter.param_code = v.param_code AND mst_parameter.deleted_at IS NULL;

-- 5. Populate notes from Excel spec column N (only for params that have a note).
UPDATE mst_parameter SET notes = v.note
FROM (VALUES
    ('MC_NAME',          'Machine code from master machine table'),
    ('YARN_DENIER',      'Nominal denier (e.g. 75, 150, 250)'),
    ('ACT_DENIER',       'Measured actual denier after draw'),
    ('NO_OF_PLY',        'Default 1'),
    ('CROSS_SECTION',    'e.g. RND, TRI, OCT'),
    ('LUSTRE_TYPE',      'BRIGHT, SD, FD'),
    ('INTERMINGLE',      'HIM, SIM, LIM, IM, NIM'),
    ('RM_TYPE',          'Single, Captive, Multi-Yarn'),
    ('Y_TYPE',           'D=Drawn, S=Spun, etc'),
    ('POLYMER_IV',       'Intrinsic viscosity'),
    ('DRAW_RATIO',       'ACT_DENIER / YARN_DENIER of upstream POY'),
    ('TPM',              'Twists per meter for twisted yarn'),
    ('MC_EFFICIENCY',    '% uptime'),
    ('NO_OF_POSITION',   'Spindle/position count'),
    ('CHANGE_OVER_KG',   'Yarn lost per machine changeover'),
    ('RAW_MATERIAL',     'Marketing cost link of upstream RM product'),
    ('WASTE_PCT',        '% RM lost as waste (stored as %, e.g. 0.7 means 0.7%)'),
    ('OPU',              'Oil application in g per kg yarn'),
    ('OIL_NAME',         'Oil group code from item master'),
    ('RM_NORMS',         '=1 / (1 - waste_pct/100)'),
    ('AX_PERC',          'Primary/best grade'),
    ('AX_WT',            'Base bobbin weight for AX grade'),
    ('AE_WT',            '=ax_wt × AE_PERC / AX_PERC'),
    ('A9_WT',            '=ax_wt × A9_PERC / AX_PERC'),
    ('A_WT',             '=ax_wt × A_PERC / AX_PERC'),
    ('B_WT',             '=ax_wt × B_PERC / AX_PERC'),
    ('C_WT',             '=ax_wt × C_PERC / AX_PERC'),
    ('NET_BOB_WT',       '=sum of all grade weights'),
    ('CAP_PACK_CODE',    'From box/bobbin cost master'),
    ('CAP_BOX_WT',       '=cap_no_of_bob × NET_BOB_WT / (1-WASTE_PCT/100) + tare'),
    ('CAP_BOB_RATE',     'Rate from box/bobbin cost master'),
    ('CAP_PACK_COST',    '=(cap_no_of_bob×CAP_BOB_RATE + CAP_BOX_RATE) / CAP_BOX_WT'),
    ('DEL_BOX_WT',       '=del_no_of_bob × NET_BOB_WT / (1-WASTE_PCT/100) + tare'),
    ('DEL_PACK_COST',    '=(del_no_of_bob×DEL_BOB_RATE + DEL_BOX_RATE) / DEL_BOX_WT'),
    ('BATCH_WEIGHT',     '=no_of_trollies × NO_BOB_PER_TROL × NET_BOB_WT'),
    ('HEATSET_COST_KG',  '=heatset_cost_batch / batch_weight'),
    ('RM_RATE',          'From group item rate master, period-dependent'),
    ('RM_LANDED_COST',   '=rm_rate (mirror, may add lc surcharge)'),
    ('OIL_RATE',         'From group item rate master'),
    ('OIL_COST',         '=oil_rate × OPU / 100'),
    ('MB_CODE',          'From MB spinning master'),
    ('MB_DOZING_PCT',    '% of MB in final yarn'),
    ('MB_COST',          '=mb_rate × MB_DOZING_PCT / 100'),
    ('RP_DOZING',        'Conditional: if MB exists → MB_DOZING_PCT, else 0'),
    ('INTERMINGLE_COST', 'Lookup from intermingling master / 100'),
    ('SPECIAL_COST_1',   'Hardcoded special cost'),
    ('NET_PRODUCTION',   '=(no_of_position × MC_SPEED × MC_EFF/100 × 1440) / (9000 × YARN_DENIER/1000)'),
    ('POWER_PER_DAY',    'From machine master'),
    ('POWER_PER_KG',     '=power_per_day / net_production'),
    ('MANPOWER_PER_KG',  '=manpower_per_day / net_production'),
    ('OVERHEAD_PER_KG',  '=overhead_per_day × NO_OF_END / NET_PRODUCTION'),
    ('SPARES_PER_KG',    '=spares_per_day / net_production'),
    ('TOTAL_FIXED_COST', '=power_per_kg + manpower_per_kg + overhead_per_kg + spares_per_kg'),
    ('CONV_CAP_EX_MB',   '=total_fixed_cost + cap_pack_cost + oil_cost + intermingle_cost + special_cost_1'),
    ('CONV_DEL_EX_MB',   '=total_fixed_cost + del_pack_cost + oil_cost + intermingle_cost + special_cost_1'),
    ('CAP_COST_PRE_QL',  '=rm_norms × RM_LANDED_COST + CONV_CAP_EX_MB'),
    ('DEL_COST_PRE_QL',  '=rm_norms × RM_LANDED_COST + CONV_DEL_EX_MB'),
    ('STD_LOSS_GRADE',   'e.g. Type 5 NS — from product grade master'),
    ('BC_LOSS_GRADE',    'e.g. Type 5 BC'),
    ('NON_STD_PERC',     '% of output that is Non-Standard Special'),
    ('BC_PERC',          '% of output classified as BC grade'),
    ('BC_RECOVERY_RATE', '% value recovered from BC grade'),
    ('NON_STD_LOSS',     '=cap_cost_pre_ql × NON_STD_PERC/100 × (1-BC_RECOVERY_RATE/100)'),
    ('BC_LOSS_CAP',      '=cap_cost_pre_ql × BC_PERC/100 × (1-BC_RECOVERY_RATE/100)'),
    ('BC_LOSS_DEL',      '=del_cost_pre_ql × BC_PERC/100 × (1-BC_RECOVERY_RATE/100)'),
    ('QLOSS_CAP',        '=bc_loss_cap + non_std_loss'),
    ('QLOSS_DEL',        '=bc_loss_del + non_std_loss'),
    ('COST_CAP_FINAL',   '=cap_cost_pre_ql + qloss_cap'),
    ('COST_DEL_FINAL',   '=del_cost_pre_ql + qloss_del'),
    ('COST_ELEC',        '=elec_kwh × ELEC_RATE'),
    ('COST_LABOR',       '=labor_hrs × LABOR_RATE × (1 + LABOR_OVERHEAD_PCT/100)'),
    ('COST_CONVERSION',  '=cost_elec + cost_labor + deprec_per_kg'),
    ('VB1_LOSS',         '=change_over_kg / vol_bucket_1_qty'),
    ('VB2_LOSS',         '=change_over_kg / vol_bucket_2_qty'),
    ('VB3_LOSS',         '=change_over_kg / vol_bucket_3_qty'),
    ('VB4_LOSS',         '=change_over_kg / vol_bucket_4_qty'),
    ('VB5_LOSS',         '=change_over_kg / vol_bucket_5_qty'),
    ('VB1_DEL_COST',     '=cost_del_final + vb1_loss'),
    ('VB2_DEL_COST',     '=cost_del_final + vb2_loss'),
    ('VB3_DEL_COST',     '=cost_del_final + vb3_loss'),
    ('VB4_DEL_COST',     '=cost_del_final + vb4_loss (key output)'),
    ('VB5_DEL_COST',     '=cost_del_final + vb5_loss')
) AS v(param_code, note)
WHERE mst_parameter.param_code = v.param_code AND mst_parameter.deleted_at IS NULL;

COMMIT;
