-- 000341: Update DELIVERY_MARGIN layout_config with 3 secondary charts:
--   1. Margin % by Category  (computed ratio, horizontal_bar, group_by=group_2)
--   2. Net Sales by Delivery Type (single-metric SUM, horizontal_bar, group_by=group_1)
--   3. Monthly Detail  (monthly_detail_table for MARGIN metric, full-width)
BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    COALESCE(layout_config, '{}'::jsonb),
    '{secondary_charts}',
    '[
      {
        "title": "Margin % by Category",
        "chart_type": "horizontal_bar",
        "drill_enabled": false,
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
        "title": "Net Sales by Delivery Type",
        "chart_type": "horizontal_bar",
        "drill_enabled": false,
        "chart_config": {
          "computed_ratio": {
            "numerator": "NETT_SALES",
            "denominator": "",
            "scale": 1,
            "group_by": "group_1"
          },
          "number_format": "currency_thousands",
          "decimals": 1,
          "color": "#2E75B6"
        },
        "span": "half"
      },
      {
        "title": "Monthly Detail",
        "chart_type": "monthly_detail_table",
        "drill_enabled": false,
        "chart_config": {
          "number_format": "currency_thousands",
          "decimals": 1,
          "metric_name": "MARGIN"
        },
        "span": "full"
      }
    ]'::jsonb
  )
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
