ALTER TABLE mst_mb_lock_log
  ALTER COLUMN mbll_unlocked_by TYPE VARCHAR(20),
  ALTER COLUMN mbll_relocked_by TYPE VARCHAR(20);
