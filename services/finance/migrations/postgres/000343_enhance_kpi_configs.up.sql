-- Enhance KPI configs: add compare modes to all cards and sparklines to more cards.
-- EBITDA: 4 KPIs with full compare + sparklines
-- NET_PROFIT: 4 KPIs with full compare + sparklines
-- DELIVERY_MARGIN: 5 KPIs (add YTD Net Sales)
BEGIN;

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month EBITDA",
      "value_field": "display_value", "period": "current_month",
      "compare": "MoM", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "YTD EBITDA",
      "value_field": "display_value", "period": "ytd",
      "compare": "YTD_vs_LY", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 6
    },
    {
      "agg": "avg", "label": "Avg Monthly (L12M)",
      "value_field": "display_value", "period": "l12m",
      "compare": "YoY", "format": "currency_thousands",
      "decimals": 1,
      "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "L12M Total",
      "value_field": "display_value", "period": "l12m",
      "compare": "YoY", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'EBITDA';

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month Net Profit",
      "value_field": "display_value", "period": "current_month",
      "compare": "MoM", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "YTD Net Profit",
      "value_field": "display_value", "period": "ytd",
      "compare": "YTD_vs_LY", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 6
    },
    {
      "agg": "avg", "label": "Avg Monthly (L12M)",
      "value_field": "display_value", "period": "l12m",
      "compare": "YoY", "format": "currency_thousands",
      "decimals": 1,
      "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "L12M Total",
      "value_field": "display_value", "period": "l12m",
      "compare": "YoY", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'NET_PROFIT';

UPDATE bi_dashboard
  SET kpi_config = '[
    {
      "agg": "sum", "label": "Current Month Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "current_month", "compare": "MoM", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "YTD Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "ytd", "compare": "YTD_vs_LY", "format": "currency_thousands",
      "show_sparkline": true, "sparkline_periods": 6
    },
    {
      "agg": "avg", "label": "Avg Monthly Margin (L12M)",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "l12m", "compare": "YoY", "format": "currency_thousands",
      "decimals": 1, "show_sparkline": true, "sparkline_periods": 12
    },
    {
      "agg": "sum", "label": "L12M Total Margin",
      "value_field": "display_value", "metric_name": "MARGIN",
      "period": "l12m", "compare": "YoY", "format": "currency_thousands"
    },
    {
      "agg": "sum", "label": "YTD Net Sales",
      "value_field": "display_value", "metric_name": "NETT_SALES",
      "period": "ytd", "compare": "YTD_vs_LY", "format": "currency_thousands"
    }
  ]'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
