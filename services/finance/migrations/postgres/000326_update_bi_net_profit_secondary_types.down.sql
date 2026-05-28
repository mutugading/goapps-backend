BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    '[
      {
        "title": "Net Profit Trend (Line View)",
        "chart_type": "line",
        "chart_config": {
          "x_axis_field": "period",
          "y_axis_field": "display_value",
          "number_format": "currency_thousands",
          "decimals": 1
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
WHERE dashboard_code = 'NET_PROFIT';

COMMIT;
