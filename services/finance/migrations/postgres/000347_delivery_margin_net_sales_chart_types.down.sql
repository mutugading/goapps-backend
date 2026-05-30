-- Revert: restore original available_chart_types for Net Sales by Delivery Type card
UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts, 1, available_chart_types}',
  '["donut", "data_table"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN'
  AND (layout_config -> 'secondary_charts' -> 1 ->> 'title') = 'Net Sales by Delivery Type';
