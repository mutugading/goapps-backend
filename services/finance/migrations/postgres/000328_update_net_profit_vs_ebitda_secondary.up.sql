BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts}',
    '[
      {
        "title": "Net Profit vs EBITDA",
        "chart_type": "dual_line",
        "source_dashboard_code": "EBITDA",
        "source_series_label": "EBITDA",
        "primary_series_label": "Net Profit",
        "available_chart_types": ["bar"],
        "chart_config": {
          "number_format": "currency_thousands",
          "decimals": 1,
          "colors": {"Net Profit": "#1d9e75", "EBITDA": "#534AB7"}
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
