-- 000459 down: restore NOT NULL on mbcm_group_head_id.
-- Guarded: re-adding NOT NULL fails if any NULL rows exist (MB/CARRIER backfill rows).
-- The caller must remove/So those rows first (e.g. roll back the composition seeder 000463)
-- before rolling this back. Kept strict rather than silently deleting data.
BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM mst_mb_composition WHERE mbcm_group_head_id IS NULL) THEN
    RAISE EXCEPTION '000459 down: mst_mb_composition has NULL mbcm_group_head_id rows (MB/CARRIER). Roll back the composition seeder first.';
  END IF;
  IF EXISTS (SELECT 1 FROM mst_mb_composition_version WHERE mbcv_group_head_id IS NULL) THEN
    RAISE EXCEPTION '000459 down: mst_mb_composition_version has NULL mbcv_group_head_id rows. Roll back the snapshots first.';
  END IF;
END $$;

ALTER TABLE mst_mb_composition ALTER COLUMN mbcm_group_head_id SET NOT NULL;
ALTER TABLE mst_mb_composition_version ALTER COLUMN mbcv_group_head_id SET NOT NULL;

COMMENT ON COLUMN mst_mb_composition.mbcm_group_head_id IS NULL;
COMMENT ON COLUMN mst_mb_composition_version.mbcv_group_head_id IS NULL;

COMMIT;
