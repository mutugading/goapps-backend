-- Migration 000356: Add tooltip labels to Delivery Margin secondary charts.
--
-- The rich tooltip on "Margin % by Category" shows three values:
--   tooltip_denom_label — label for the denominator raw value (Net Sales)
--   tooltip_numer_label — label for the computed numerator value (Margin)
-- Without these, the chart uses the defaults ("Net Sales" / "Margin").
-- Setting them explicitly makes the labels match the dashboard language.

BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN (chart->>'title') = 'Margin % by Category'
        THEN chart
            || '{"chart_config": {}}'::jsonb
            || jsonb_build_object(
                 'chart_config',
                 COALESCE(chart->'chart_config', '{}'::jsonb)
                   || '{"tooltip_denom_label": "Net Sales", "tooltip_numer_label": "Margin"}'::jsonb
               )
        ELSE chart
      END
    )
    FROM jsonb_array_elements(layout_config->'secondary_charts') AS chart
  )
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
