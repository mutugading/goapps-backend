-- Revert: restore original available_chart_types for Net Profit vs EBITDA secondary card
UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts, 0, available_chart_types}',
  '["bar"]'::jsonb
)
WHERE dashboard_code = 'NET_PROFIT'
  AND (layout_config -> 'secondary_charts' -> 0 ->> 'chart_type') = 'dual_line';
