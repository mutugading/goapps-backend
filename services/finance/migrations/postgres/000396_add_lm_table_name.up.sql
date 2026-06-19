-- 000396: Add lm_table_name to mst_lookup_master for DB introspection.
-- Allows backend to discover columns + types without hardcoding api_path.

BEGIN;

ALTER TABLE mst_lookup_master
    ADD COLUMN IF NOT EXISTS lm_table_name VARCHAR(63);

COMMENT ON COLUMN mst_lookup_master.lm_table_name IS
    'PostgreSQL table name to introspect columns from (e.g., mst_machine). Used by ListTableColumns and ListMasterOptions.';

-- Seed table_name for the 5 existing masters
UPDATE mst_lookup_master SET lm_table_name = 'mst_machine'          WHERE lm_code = 'MACHINE';
UPDATE mst_lookup_master SET lm_table_name = 'mst_intermingling'    WHERE lm_code = 'INTERMINGLING';
UPDATE mst_lookup_master SET lm_table_name = 'mst_product_grade'    WHERE lm_code = 'PRODUCT_GRADE';
UPDATE mst_lookup_master SET lm_table_name = 'mst_mb_head'          WHERE lm_code = 'MB_HEAD';
UPDATE mst_lookup_master SET lm_table_name = 'mst_box_bobbin_cost'  WHERE lm_code = 'BOX_BOBBIN_COST';

COMMIT;
