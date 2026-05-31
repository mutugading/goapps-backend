-- Revert migration 000348: Remove static filter chip values from DELIVERY_MARGIN.
BEGIN;
UPDATE bi_dashboard
SET chart_config = chart_config - 'filter_chips_group1' - 'filter_chips_group2'
WHERE dashboard_code = 'DELIVERY_MARGIN';
COMMIT;
