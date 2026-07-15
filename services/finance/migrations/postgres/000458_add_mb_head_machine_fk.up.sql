BEGIN;

ALTER TABLE mst_mb_head
    ADD COLUMN IF NOT EXISTS mbh_machine_id UUID REFERENCES mst_machine(mc_id);

UPDATE mst_mb_head h
SET mbh_machine_id = m.mc_id
FROM mst_machine m
WHERE m.mc_code = 'MB' AND m.deleted_at IS NULL AND h.mbh_machine_id IS NULL;

UPDATE mst_machine SET mc_type = 'MB' WHERE mc_code = 'MB' AND deleted_at IS NULL;

COMMIT;
