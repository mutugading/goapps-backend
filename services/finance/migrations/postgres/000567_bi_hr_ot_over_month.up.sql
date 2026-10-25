-- H038 / HD-20: tren over-limit 14 jam per bulan payroll. Fakta MONTHLY dibangun ETL dari HR_OT_WEEK_STATUS
-- (minggu masuk periode tempat hari Minggu-nya jatuh, K10): HR_OT_OVER_WEEKS (orang-minggu) dan
-- HR_OT_OVER_EMP (karyawan unik), group_1 = dept. Dashboard HR_OT_OVER_MONTH (tab "Tren Over-limit")
-- memakai orang-minggu; HR_OT_OVER_EMP hanya sumber kartu karyawan unik (cross-dashboard, tanpa tab).
-- Dua ukuran itu tidak ditumpuk di satu batang. Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT v.code, v.title, v.descr, v.ftype, NULL, 'MONTHLY', 'L12M', 'PAYROLL_26_25',
       v.ctype, v.cfg::jsonb, NULLIF(v.layout, 'null')::jsonb, '[]'::jsonb, v.kpi::jsonb,
       FALSE, 1, 1800, 0, v.ord, grp.group_id, TRUE
FROM grp, (VALUES
  ('HR_OT_OVER_MONTH', 'Tren Over-limit 14 Jam per Bulan',
   'R2: orang-minggu over-limit per bulan payroll (26–25); minggu dihitung di periode hari Minggunya',
   'HR_OT_OVER_WEEKS', 'bar',
   '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":0,"empty_message":"Belum ada data over-limit"}',
   '{"secondary_charts":[{"title":"Orang-minggu vs Karyawan Unik Over-limit","chart_type":"dual_line","source_dashboard_code":"HR_OT_OVER_EMP","primary_series_label":"Orang-minggu","source_series_label":"Karyawan unik","chart_config":{"number_format":"raw","decimals":0},"span":"full"}]}',
   '[{"label":"Orang-minggu Over-limit","value_field":"value","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0},
     {"label":"% Karyawan Over-limit","agg":"cross_ratio","period":"current_month","cross_ratio_numerator_type":"HR_OT_OVER_EMP","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":100,"compare":"MoM","format":"raw","decimals":1}]',
   17),
  ('HR_OT_OVER_EMP', 'Karyawan Unik Over-limit per Bulan',
   'Sumber data kartu "Karyawan unik" di HR_OT_OVER_MONTH',
   'HR_OT_OVER_EMP', 'line',
   '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":0,"empty_message":"Belum ada data over-limit"}',
   'null', '[]', 18)
) AS v(code, title, descr, ftype, ctype, cfg, layout, kpi, ord)
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code IN ('HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP')
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

COMMIT;
