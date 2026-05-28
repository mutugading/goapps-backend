-- Fix: FINANCE_PRODUCT_ORDERS menu_url was seeded as '/finance/product-orders'
-- but the actual Next.js page lives at '/finance/routes'. Also rename the title
-- from 'Product Orders' to 'Product Routes' to match local dev.
BEGIN;

UPDATE public.mst_menu
   SET menu_url   = '/finance/routes',
       menu_title = 'Product Routes',
       updated_at = NOW(),
       updated_by = 'seed'
 WHERE menu_code = 'FINANCE_PRODUCT_ORDERS'
   AND deleted_at IS NULL;

COMMIT;
