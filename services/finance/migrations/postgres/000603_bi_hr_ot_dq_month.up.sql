-- H039 / HD-14: tren kualitas data OT per bulan payroll: total jam |calc − tier 1+2+3| (HR_OT_DQ MONTHLY, Change Off
-- dikecualikan) dan jumlah NIK terdampak (HR_OT_DQ_EMP MONTHLY, dibangun ETL). Dashboard HR_OT_DQ_MONTH (tab
-- "Kualitas Data") hanya untuk role HR, bukan direksi. HR_OT_DQ_EMP hanya sumber kartu NIK terdampak
-- (cross-dashboard, tanpa tab). Idempotent.
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
  ('HR_OT_DQ_MONTH', 'Kualitas Data OT per Bulan',
   'R1: selisih jam |calc − tier 1+2+3| per bulan payroll (Change Off dikecualikan, HD-14); angka besar = data OT kurang bisa dipercaya',
   'HR_OT_DQ', 'bar',
   '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":2,"empty_message":"Belum ada data kualitas OT"}',
   '{"secondary_charts":[{"title":"Karyawan Terdampak (NIK dengan selisih > 0)","chart_type":"dual_line","source_dashboard_code":"HR_OT_DQ_EMP","primary_series_label":"Jam selisih","source_series_label":"NIK terdampak","chart_config":{"number_format":"raw","decimals":0},"span":"full"}]}',
   '[{"label":"Jam Selisih Calc vs Tier","value_field":"value","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":2}]',
   19),
  ('HR_OT_DQ_EMP', 'NIK Terdampak Selisih OT per Bulan',
   'Sumber data kartu "Karyawan Terdampak" di HR_OT_DQ_MONTH',
   'HR_OT_DQ_EMP', 'line',
   '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":0,"empty_message":"Belum ada data kualitas OT"}',
   'null', '[]', 20)
) AS v(code, title, descr, ftype, ctype, cfg, layout, kpi, ord)
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, 'HR'
FROM bi_dashboard d
WHERE d.dashboard_code IN ('HR_OT_DQ_MONTH', 'HR_OT_DQ_EMP')
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

COMMIT;
