-- H035: HR_OT_TREND (tren harian, 000588) diganti dashboard "Tren OT" bulanan per tipe OT. Tanpa ETL baru:
-- fakta MONTHLY HR_OT_HOURS (group_1 = dept, group_2 = tipe OT), HR_OT_EMP_COUNT, HR_HEADCOUNT (HD-06, HD-08).
-- Maks. 6 kartu KPI (validasi kpi_config): 4 tipe OT + jam OT/karyawan + % karyawan lembur; total tampak dari tumpukan batang.
-- Utama: batang bertumpuk bulan × tipe (default L12M). KPI per tipe memakai period current_month + MoM,
-- jadi tidak ikut rentang 12 bulan. Tabel sekunder: dept × tipe (drill dept → tipe), Δ per dept lewat compare viewer.
-- Idempotent: UPDATE dijaga label sentinel pada kpi_config.
BEGIN;

UPDATE bi_dashboard
SET dashboard_title = 'Tren OT Bulanan per Tipe',
    description = 'R1: naik/turun jam OT antar bulan per tipe (Normal/Off/Holiday/Short Holiday/Change Off), periode payroll 26–25',
    filter_type = 'HR_OT_HOURS',
    periode_grain = 'MONTHLY',
    default_period = 'L12M',
    chart_type = 'stacked_bar',
    chart_config = '{"x_axis_field":"period","y_axis_field":"value","stack_by_field":"group_2","number_format":"raw","decimals":1,"empty_message":"Belum ada data OT"}'::jsonb,
    layout_config = '{"secondary_charts":[{"title":"Dept × Tipe OT (jam) — klik dept untuk drill","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full"}]}'::jsonb,
    compare_modes = '[]'::jsonb,
    kpi_config = '[
      {"label":"Normal","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Normal","compare":"MoM","format":"raw","decimals":1},
      {"label":"Off","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Off","compare":"MoM","format":"raw","decimals":1},
      {"label":"Holiday","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Holiday","compare":"MoM","format":"raw","decimals":1},
      {"label":"Short Holiday","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Short Holiday","compare":"MoM","format":"raw","decimals":1},
      {"label":"Jam OT per Karyawan","agg":"cross_ratio","period":"current_month","cross_ratio_numerator_type":"HR_OT_HOURS","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":1,"compare":"MoM","format":"raw","decimals":2},
      {"label":"% Karyawan Lembur","agg":"cross_ratio","period":"current_month","cross_ratio_numerator_type":"HR_OT_EMP_COUNT","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":100,"compare":"MoM","format":"raw","decimals":1}
    ]'::jsonb,
    drill_enabled = TRUE,
    max_drill_level = 2,
    display_order = 15
WHERE dashboard_code = 'HR_OT_TREND'
  AND NOT (kpi_config @> '[{"label":"Jam OT per Karyawan"}]'::jsonb);

COMMIT;
