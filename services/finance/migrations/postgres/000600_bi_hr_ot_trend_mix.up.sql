-- H036: kartu komposisi tipe OT (% stacked 100%, panah pergeseran porsi) di HR_OT_TREND. Dirender frontend
-- (chart_type ot_mix) dari data utama dashboard; rasio dihitung di sisi tampilan (HD-08), tanpa fakta/ETL baru.
-- Idempotent: dijaga oleh keberadaan kartu ot_mix di layout_config.
BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    '[{"title":"Komposisi Tipe OT (%)","chart_type":"ot_mix","chart_config":{"decimals":1},"span":"full","drill_enabled":false}]'::jsonb
      || COALESCE(layout_config->'secondary_charts', '[]'::jsonb))
WHERE dashboard_code = 'HR_OT_TREND'
  AND NOT (COALESCE(layout_config->'secondary_charts', '[]'::jsonb) @> '[{"chart_type":"ot_mix"}]'::jsonb);

COMMIT;
