-- H040: Tren OT per tier 1/2/3 per bulan payroll. Fakta HR_OT_TIER (MONTHLY, group_1 dept, group_2 TIER_1/2/3,
-- group_3 NIK; dibangun ETL hrot.Aggregate). Change Off tidak punya tier (HD-14), jadi tidak muncul di sini.
-- Dashboard HR_OT_TIER_MONTH: batang bertumpuk bulan × tier (L12M), 3 KPI per tier + MoM. Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT 'HR_OT_TIER_MONTH', 'Tren OT per Tier',
       'R1: jam OT tier 1/2/3 per bulan payroll 26–25 (Change Off tidak punya tier, HD-14)',
       'HR_OT_TIER', NULL, 'MONTHLY', 'L12M', 'PAYROLL_26_25',
       'stacked_bar',
       '{"x_axis_field":"period","y_axis_field":"value","stack_by_field":"group_2","number_format":"raw","decimals":1,"empty_message":"Belum ada data tier OT"}'::jsonb,
       '{"secondary_charts":[{"title":"Dept × Tier (jam) — klik dept untuk drill","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full"}]}'::jsonb,
       '[]'::jsonb,
       '[
         {"label":"Tier 1","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"TIER_1","compare":"MoM","format":"raw","decimals":1},
         {"label":"Tier 2","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"TIER_2","compare":"MoM","format":"raw","decimals":1},
         {"label":"Tier 3","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"TIER_3","compare":"MoM","format":"raw","decimals":1}
       ]'::jsonb,
       TRUE, 2, 1800, 0, 16, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code = 'HR_OT_TIER_MONTH'
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

COMMIT;
