BEGIN;
UPDATE bi_dashboard
  SET chart_config = chart_config || '{"available_chart_types":["bar","line","data_table"]}'::jsonb
WHERE dashboard_code = 'EBITDA';
COMMIT;
