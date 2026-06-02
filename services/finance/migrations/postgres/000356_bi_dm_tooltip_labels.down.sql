-- Revert 000356: remove tooltip label overrides from Margin % by Category.
BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN (chart->>'title') = 'Margin % by Category'
        THEN jsonb_set(
               jsonb_set(chart, '{chart_config}',
                 (chart->'chart_config') - 'tooltip_denom_label' - 'tooltip_numer_label'
               ), '{}', '{}'::jsonb
             )
        ELSE chart
      END
    )
    FROM jsonb_array_elements(layout_config->'secondary_charts') AS chart
  )
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
