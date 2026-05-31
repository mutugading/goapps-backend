-- Migration 000347: Add bar chart type to "Net Sales by Delivery Type" secondary card.
-- The card uses a computed_ratio (single-metric aggregation, group_by=group_1) and shows
-- net sales per delivery type. Adding "bar" (vertical bar) alongside the existing
-- "donut" and "data_table" options gives users a third view.

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts, 1, available_chart_types}',
  '["bar", "donut", "data_table"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN'
  AND (layout_config -> 'secondary_charts' -> 1 ->> 'title') = 'Net Sales by Delivery Type';
