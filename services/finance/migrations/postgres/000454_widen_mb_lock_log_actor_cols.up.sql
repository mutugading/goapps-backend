-- MB Costing Suite: mbll_unlocked_by/mbll_relocked_by were VARCHAR(20), too narrow for UUID actor IDs
-- (36 chars), causing every unlock to fail with "value too long for type character varying(20)".
ALTER TABLE mst_mb_lock_log
  ALTER COLUMN mbll_unlocked_by TYPE VARCHAR(64),
  ALTER COLUMN mbll_relocked_by TYPE VARCHAR(64);
