-- 000392 rollback: drop notes column and restore original display_group values.
BEGIN;

ALTER TABLE mst_parameter DROP COLUMN IF EXISTS notes;

-- Restore STEAM_RATE and WATER_RATE to their 000381 Utilities seeding.
UPDATE mst_parameter SET display_group = 'Utilities', display_order = 10
WHERE param_code = 'STEAM_RATE' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'Utilities', display_order = 20
WHERE param_code = 'WATER_RATE' AND deleted_at IS NULL;

-- Restore COST_ELEC, COST_LABOR, COST_CONVERSION to their 000381 Conversion seeding.
UPDATE mst_parameter SET display_group = 'Conversion', display_order = 62
WHERE param_code = 'COST_ELEC' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'Conversion', display_order = 72
WHERE param_code = 'COST_LABOR' AND deleted_at IS NULL;

UPDATE mst_parameter SET display_group = 'Conversion', display_order = 105
WHERE param_code = 'COST_CONVERSION' AND deleted_at IS NULL;

-- Restore original param_name and param_short_name from 000381 seed.
UPDATE mst_parameter SET param_name = v.name, param_short_name = v.short
FROM (VALUES
    ('VOL_BUCKET_1_QTY', 'Volume Bucket 1 Qty threshold',      'VB1 Qty'),
    ('VOL_BUCKET_2_QTY', 'Volume Bucket 2 Qty threshold',      'VB2 Qty'),
    ('VOL_BUCKET_3_QTY', 'Volume Bucket 3 Qty threshold',      'VB3 Qty'),
    ('VOL_BUCKET_4_QTY', 'Volume Bucket 4 Qty threshold',      'VB4 Qty'),
    ('VOL_BUCKET_5_QTY', 'Volume Bucket 5 Qty threshold',      'VB5 Qty'),
    ('RAW_MATERIAL',     'Raw Material Reference',              'RM Ref'),
    ('ELEC_KWH',         'Electricity Consumption kWh/kg',     'Elec kWh'),
    ('STEAM_RATE',       'Steam Cost Rate (USD/kg steam)',      'Steam Rate'),
    ('WATER_RATE',       'Water Cost Rate (USD/m3)',            'Water Rate'),
    ('LABOR_OVERHEAD_PCT','Labor Overhead % (benefits)',        'Labor OH%'),
    ('MAT_OVERHEAD_PCT', 'Material Overhead Percentage (%)',   'Mat OH%'),
    ('RM_NORMS',         'RM Consumption Norms',               'RM Norms'),
    ('CONV_CAP_EX_MB',   'Conv + Captive Pack excl MB',        'Conv Cap'),
    ('CONV_DEL_EX_MB',   'Conv + Delivery Pack excl MB',       'Conv Del'),
    ('CAP_COST_PRE_QL',  'Captive Cost Before Qloss',          'Cap Pre-QL'),
    ('DEL_COST_PRE_QL',  'Delivery Cost Before Qloss',         'Del Pre-QL'),
    ('NON_STD_LOSS',     'Non-Standard Value Loss/kg',         'Non-Std Loss'),
    ('BC_LOSS_CAP',      'BC Value Loss Captive/kg',           'BC Loss Cap'),
    ('BC_LOSS_DEL',      'BC Value Loss Delivery/kg',          'BC Loss Del'),
    ('QLOSS_CAP',        'Quality Loss Captive/kg',            'QLoss Cap'),
    ('QLOSS_DEL',        'Quality Loss Delivery/kg',           'QLoss Del'),
    ('COST_ELEC',        'Electricity Cost per kg (USD)',      'Cost Elec'),
    ('COST_LABOR',       'Labor Cost per kg (USD)',            'Cost Labor'),
    ('COST_CONVERSION',  'Total Conversion Cost per kg',       'Conv Total'),
    ('VB1_LOSS',         'VB1 Change-Over Loss/kg',            'VB1 Loss'),
    ('VB2_LOSS',         'VB2 Change-Over Loss/kg',            'VB2 Loss'),
    ('VB3_LOSS',         'VB3 Change-Over Loss/kg',            'VB3 Loss'),
    ('VB4_LOSS',         'VB4 Change-Over Loss/kg',            'VB4 Loss'),
    ('VB5_LOSS',         'VB5 Change-Over Loss/kg',            'VB5 Loss'),
    ('VB1_DEL_COST',     'VB1 Delivery Cost/kg',               'VB1 Cost'),
    ('VB2_DEL_COST',     'VB2 Delivery Cost/kg',               'VB2 Cost'),
    ('VB3_DEL_COST',     'VB3 Delivery Cost/kg',               'VB3 Cost'),
    ('VB4_DEL_COST',     'VB4 Delivery Cost/kg',               'VB4 Cost'),
    ('VB5_DEL_COST',     'VB5 Delivery Cost/kg',               'VB5 Cost')
) AS v(param_code, name, short)
WHERE mst_parameter.param_code = v.param_code AND mst_parameter.deleted_at IS NULL;

-- Clear owner/required/period for CALCULATED params.
UPDATE mst_parameter SET
    owner_department        = NULL,
    is_required_for_costing = FALSE,
    is_period_dependent     = FALSE,
    uom_id                  = NULL
WHERE param_category = 'CALCULATED' AND deleted_at IS NULL;

COMMIT;
