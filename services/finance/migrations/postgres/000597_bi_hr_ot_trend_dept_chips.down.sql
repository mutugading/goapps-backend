-- H035 (DOWN): hapus chip filter dept dari HR_OT_TREND.
BEGIN;
UPDATE bi_dashboard
SET chart_config = chart_config - 'filter_chips' - 'filter_chips_group1' - 'filter_chips_group1_label' - 'filter_chips_group2_label'
WHERE dashboard_code = 'HR_OT_TREND';
COMMIT;
