-- Migration 000348: Store static filter chip values for DELIVERY_MARGIN dashboard.
-- These values are stored in chart_config so the viewer can display chips
-- even when the fact table has no data yet (static fallback).
BEGIN;
UPDATE bi_dashboard
SET chart_config = jsonb_set(
  jsonb_set(
    chart_config,
    '{filter_chips_group1}',
    '["Export","JobWork","Local","Popcorn"]'::jsonb
  ),
  '{filter_chips_group2}',
  '["ACY","ATY","FG","HANK","ITY","MEERABAH","NYLON","POPCORN","POY","SUPERBA"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';
COMMIT;
