-- 000329 down: Remove secondary_charts from DELIVERY_MARGIN layout_config.

BEGIN;

UPDATE bi_dashboard
SET layout_config = layout_config - 'secondary_charts'
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
