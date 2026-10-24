-- H035 (DOWN): kembalikan HR_OT_TREND ke tren jam OT harian (000553).
BEGIN;
UPDATE bi_dashboard
SET dashboard_title = 'Tren Jam OT Harian',
    description = 'R1: total jam OT per hari dalam periode payroll',
    periode_grain = 'DAILY', default_period = 'THIS_MONTH', chart_type = 'line',
    chart_config = '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":1,"empty_message":"Belum ada data OT"}'::jsonb,
    layout_config = NULL,
    kpi_config = '[{"label":"Total Jam OT","value_field":"value","agg":"sum","compare":"none","format":"raw","decimals":1}]'::jsonb,
    drill_enabled = FALSE, max_drill_level = 1, display_order = 20
WHERE dashboard_code = 'HR_OT_TREND';
COMMIT;
