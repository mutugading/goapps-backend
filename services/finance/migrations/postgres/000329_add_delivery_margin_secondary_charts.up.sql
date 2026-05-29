-- 000329: Add secondary charts to DELIVERY_MARGIN dashboard layout_config.
-- Adds "Margin % by Category" (horizontal_bar via computed_ratio) and
-- "Monthly Detail" (data_table) to the secondary_charts array.

BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  COALESCE(layout_config, '{}'::jsonb),
  '{secondary_charts}',
  '[
    {
      "title": "Margin % by Category",
      "chart_type": "horizontal_bar",
      "chart_config": {
        "computed_ratio": {
          "numerator": "MARGIN",
          "denominator": "NETT_SALES",
          "scale": 100,
          "group_by": "group_2"
        },
        "number_format": "percent",
        "decimals": 1,
        "color": "#1d9e75"
      },
      "span": "half"
    },
    {
      "title": "Monthly Detail",
      "chart_type": "data_table",
      "span": "half"
    }
  ]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
