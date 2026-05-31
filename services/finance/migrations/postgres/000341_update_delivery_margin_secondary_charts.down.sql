-- 000341 down: restore DELIVERY_MARGIN to the 2-chart layout from 000337.
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
        "title": "Monthly Detail",
        "chart_type": "data_table",
        "drill_enabled": false,
        "span": "half"
      }
    ]'::jsonb
  )
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
