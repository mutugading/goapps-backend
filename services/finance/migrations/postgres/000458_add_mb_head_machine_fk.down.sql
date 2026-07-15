BEGIN;

ALTER TABLE mst_mb_head
    DROP COLUMN IF EXISTS mbh_machine_id;

COMMIT;
