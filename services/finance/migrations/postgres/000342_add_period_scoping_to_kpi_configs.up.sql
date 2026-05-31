-- Add per-KPI period scoping to all dashboard kpi_configs.
-- Without period scope, all KPIs inherit the viewer's selected period (L12M),
-- which makes Current Month, YTD, and Avg Monthly all return the same L12M value.
BEGIN;

UPDATE bi_dashboard
  SET kpi_config = '[
    {"agg":"sum","label":"Current Month EBITDA","format":"currency_thousands","compare":"MoM","period":"current_month","value_field":"display_value","show_sparkline":true,"sparkline_periods":12},
    {"agg":"sum","label":"YTD EBITDA","format":"currency_thousands","compare":"YTD_vs_LY","period":"ytd","value_field":"display_value"},
    {"agg":"avg","label":"Avg Monthly (L12M)","format":"currency_thousands","compare":"none","period":"l12m","decimals":1,"value_field":"display_value"},
    {"agg":"sum","label":"L12M Total","format":"currency_thousands","compare":"none","period":"l12m","value_field":"display_value"}
  ]'::jsonb
WHERE dashboard_code = 'EBITDA';

UPDATE bi_dashboard
  SET kpi_config = '[
    {"agg":"sum","label":"Current Month Net Profit","format":"currency_thousands","compare":"MoM","period":"current_month","value_field":"display_value","show_sparkline":true,"sparkline_periods":12},
    {"agg":"sum","label":"YTD Net Profit","format":"currency_thousands","compare":"YTD_vs_LY","period":"ytd","value_field":"display_value"},
    {"agg":"cross_ratio","label":"Net Profit Margin","numerator_group_1":"NET PROFIT","denominator_group_1":"EBITDA","period":"l12m","compare":"none","format":"percent","decimals":1},
    {"agg":"sum","label":"L12M Total","format":"currency_thousands","compare":"none","period":"l12m","value_field":"display_value"}
  ]'::jsonb
WHERE dashboard_code = 'NET_PROFIT';

UPDATE bi_dashboard
  SET kpi_config = '[
    {"agg":"sum","label":"Current Month Margin","format":"currency_thousands","compare":"MoM","period":"current_month","metric_name":"MARGIN","value_field":"display_value","show_sparkline":true,"sparkline_periods":12},
    {"agg":"sum","label":"YTD Margin","format":"currency_thousands","compare":"YTD_vs_LY","period":"ytd","metric_name":"MARGIN","value_field":"display_value"},
    {"agg":"avg","label":"Avg Monthly Margin","format":"currency_thousands","compare":"none","period":"l12m","decimals":1,"metric_name":"MARGIN","value_field":"display_value"},
    {"agg":"sum","label":"L12M Total Margin","format":"currency_thousands","compare":"none","period":"l12m","metric_name":"MARGIN","value_field":"display_value"}
  ]'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
