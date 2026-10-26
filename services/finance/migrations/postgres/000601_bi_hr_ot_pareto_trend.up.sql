-- H037: garis tren "% jam OT oleh 10% karyawan teratas" per bulan di HR_OT_TREND. Sumber HR_OT_PARETO
-- (group_1 = TOP_10PCT / TOTAL, MONTHLY, se-perusahaan; HD-09). Dashboard pendamping HR_OT_PARETO (grup
-- WORKFORCE, wajib group_id; tanpa tab di workforce-nav) hanya sumber data kartu. chart_type 'line' (bukan
-- stacked_bar): jalur pivot membaca mv_bi_metric_g2 yang membuang baris group_2 NULL, padahal fakta ini tanpa group_2. Rasio TOP/TOTAL×100 dan Δ poin dihitung di frontend
-- (chart_type ot_pareto, HD-08); tanpa fakta/ETL baru. Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT
    'HR_OT_PARETO', 'Konsentrasi Lembur (Pareto)',
    'Sumber data kartu ot_pareto di HR_OT_TREND: jam OT 10% karyawan teratas vs total per bulan',
    'HR_OT_PARETO', NULL, 'MONTHLY', 'L12M', 'PAYROLL_26_25',
    'line',
    '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":1,"empty_message":"Belum ada data Pareto OT"}'::jsonb,
    NULL, '[]'::jsonb, '[]'::jsonb,
    FALSE, 1, 1800, 0, 16, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code = 'HR_OT_PARETO'
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    COALESCE(layout_config->'secondary_charts', '[]'::jsonb)
      || '[{"title":"Konsentrasi Lembur: % Jam OT oleh 10% Karyawan Teratas","chart_type":"ot_pareto","chart_config":{"source_dashboard_code":"HR_OT_PARETO","top_key":"TOP_10PCT","total_key":"TOTAL","period_calendar":"PAYROLL_26_25","decimals":1},"span":"full","drill_enabled":false}]'::jsonb)
WHERE dashboard_code = 'HR_OT_TREND'
  AND NOT (COALESCE(layout_config->'secondary_charts', '[]'::jsonb) @> '[{"chart_type":"ot_pareto"}]'::jsonb);

COMMIT;
