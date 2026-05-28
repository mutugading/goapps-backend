-- Revert: restore the original (incorrect) menu_url and title.
BEGIN;

UPDATE public.mst_menu
   SET menu_url   = '/finance/product-orders',
       menu_title = 'Product Orders'
 WHERE menu_code = 'FINANCE_PRODUCT_ORDERS';

COMMIT;
