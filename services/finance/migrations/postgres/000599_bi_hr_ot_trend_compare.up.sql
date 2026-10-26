-- H041: kartu pembanding periode di HR_OT_TREND (bar berkelompok bulan terpilih vs MoM/YoY, tabel Δ per tipe,
-- kontribusi dept ke Δ total). Dirender frontend (chart_type ot_compare) dari endpoint /computed; tanpa ETL baru.
-- Idempotent: dijaga oleh keberadaan kartu ot_compare di layout_config.
BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    '[{"title":"Perbandingan Periode","chart_type":"ot_compare","chart_config":{"metric":"VALUE","period_calendar":"PAYROLL_26_25","decimals":1},"span":"full","drill_enabled":false}]'::jsonb
      || COALESCE(layout_config->'secondary_charts', '[]'::jsonb))
WHERE dashboard_code = 'HR_OT_TREND'
  AND NOT (COALESCE(layout_config->'secondary_charts', '[]'::jsonb) @> '[{"chart_type":"ot_compare"}]'::jsonb);

COMMIT;
