-- Migration 000346: Add line/area to Net Profit vs EBITDA secondary chart type switcher
-- The "Net Profit vs EBITDA" dual_line card currently only allows switching to "bar".
-- Adding "area" and "line" gives users the ability to view the trend as a smooth area
-- or plain line chart in addition to the bar and default dual_line views.

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts, 0, available_chart_types}',
  '["bar", "area", "line"]'::jsonb
)
WHERE dashboard_code = 'NET_PROFIT'
  AND (layout_config -> 'secondary_charts' -> 0 ->> 'chart_type') = 'dual_line';
