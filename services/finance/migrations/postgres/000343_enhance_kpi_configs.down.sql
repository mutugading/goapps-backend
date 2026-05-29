-- Rollback 000343: restore KPI configs to 000342 state
BEGIN;

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month EBITDA",
      "value_field": "display_value", "period": "current_month",
      "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "sum", "label": "YTD EBITDA",
      "value_field": "display_value", "period": "ytd",
      "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "avg", "label": "Avg Monthly EBITDA",
      "value_field": "display_value", "period": "l12m",
      "compare": "none", "format": "currency_thousands",
      "decimals": 1
    },
    {
      "agg": "sum", "label": "L12M Total EBITDA",
      "value_field": "display_value", "period": "l12m",
      "compare": "none", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'EBITDA';

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month Net Profit",
      "value_field": "display_value", "period": "current_month",
      "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "sum", "label": "YTD Net Profit",
      "value_field": "display_value", "period": "ytd",
      "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "avg", "label": "Avg Monthly Net Profit",
      "value_field": "display_value", "period": "l12m",
      "compare": "none", "format": "currency_thousands",
      "decimals": 1
    },
    {
      "agg": "sum", "label": "L12M Total Net Profit",
      "value_field": "display_value", "period": "l12m",
      "compare": "none", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'NET_PROFIT';

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "current_month", "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "sum", "label": "YTD Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "ytd", "compare": "none", "format": "currency_thousands"
    },
    {
      "agg": "avg", "label": "Avg Monthly Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "l12m", "compare": "none", "format": "currency_thousands",
      "decimals": 1
    },
    {
      "agg": "sum", "label": "L12M Total Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "l12m", "compare": "none", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
