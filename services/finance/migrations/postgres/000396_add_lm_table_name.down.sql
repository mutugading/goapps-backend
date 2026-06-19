BEGIN;
ALTER TABLE mst_lookup_master DROP COLUMN IF EXISTS lm_table_name;
COMMIT;
