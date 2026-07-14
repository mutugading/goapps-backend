DROP TABLE IF EXISTS mst_mb_lock_log;
ALTER TABLE cost_product_master
  DROP COLUMN IF EXISTS cpm_source,
  DROP COLUMN IF EXISTS cpm_is_locked;
