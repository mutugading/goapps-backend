BEGIN;
UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    jsonb_set(
      jsonb_set(
        layout_config,
        '{secondary_charts, 0, available_chart_types}',
        'null'::jsonb
      ),
      '{secondary_charts, 1, available_chart_types}',
      'null'::jsonb
    ),
    '{secondary_charts, 2, available_chart_types}',
    'null'::jsonb
  )
WHERE dashboard_code = 'DELIVERY_MARGIN';
COMMIT;
