-- H042 / HD-22: Tren Over-limit digabung ke Kepatuhan 14 Jam. HR_OT_OVER_NIK (MONTHLY, g1 dept, g3 NIK, nilai =
-- jumlah minggu over-limit per NIK per bulan, dibangun ETL) hanya sumber drill orang (/computed group_by=group_3,
-- digerbangi workforceemployee). Bagian atas HR_OT_OVERLIMIT dideklarasikan di layout_config.top_section
-- (komponen ot_over_month di viewer). Dashboard HR_OT_OVER_MONTH/HR_OT_OVER_EMP tetap sebagai sumber data
-- (tab-nya dihapus di FE). Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT 'HR_OT_OVER_NIK', 'Minggu Over-limit per Karyawan per Bulan',
       'Sumber drill orang di HR_OT_OVERLIMIT (HD-22); level karyawan, gerbang workforceemployee',
       'HR_OT_OVER_NIK', NULL, 'MONTHLY', 'L12M', 'PAYROLL_26_25',
       'bar', '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":0}'::jsonb,
       NULL, '[]'::jsonb, '[]'::jsonb, FALSE, 1, 1800, 0, 21, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code = 'HR_OT_OVER_NIK'
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

UPDATE bi_dashboard
SET layout_config = COALESCE(layout_config, '{}'::jsonb) || '{"top_section":{"type":"ot_over_month","trend_dashboard":"HR_OT_OVER_MONTH","emp_dashboard":"HR_OT_OVER_EMP","nik_dashboard":"HR_OT_OVER_NIK","period_calendar":"PAYROLL_26_25"}}'::jsonb
WHERE dashboard_code = 'HR_OT_OVERLIMIT';

COMMIT;
