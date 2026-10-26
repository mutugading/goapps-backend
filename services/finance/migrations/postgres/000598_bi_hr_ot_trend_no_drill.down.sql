-- H035 (DOWN): kembalikan drill & judul tabel seperti 000596.
BEGIN;
UPDATE bi_dashboard
SET drill_enabled = TRUE,
    max_drill_level = 2,
    layout_config = '{"secondary_charts":[{"title":"Dept × Tipe OT (jam) — klik dept untuk drill","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full"}]}'::jsonb
WHERE dashboard_code = 'HR_OT_TREND';
COMMIT;
