-- 000459: Allow MB/CARRIER composition rows to have a NULL group_head_id.
-- The 000439 schema declared mbcm_group_head_id NOT NULL, but only GROUP-source rows
-- reference an RM group; MB (nested-MB reference) and CARRIER rows conceptually have no
-- group. The domain entity already permits an empty group for non-GROUP rows
-- (mbcomposition/entity.go), and the frontend sends "" for MB/CARRIER lines — but the
-- NOT NULL column + a non-NULLIF repo INSERT made those rows unstorable. This relaxes the
-- constraint so MB/CARRIER rows persist group_head_id = NULL. The FK is unchanged (a NULL
-- FK value is allowed and simply not checked). Prerequisite for the 202606 composition
-- backfill (350 MB-source rows) and a latent correctness fix for the live feature.
BEGIN;

-- Working-set table.
ALTER TABLE mst_mb_composition ALTER COLUMN mbcm_group_head_id DROP NOT NULL;

-- Immutable version-snapshot table (000440) — SnapshotVersion copies group_head_id straight
-- across, so it must accept NULL for MB/CARRIER rows too, or VALIDATE of any recipe with an
-- MB/CARRIER line would fail here.
ALTER TABLE mst_mb_composition_version ALTER COLUMN mbcv_group_head_id DROP NOT NULL;

COMMENT ON COLUMN mst_mb_composition.mbcm_group_head_id IS
  'RM-group head ref. Set only for GROUP rows; MB/CARRIER rows store NULL.';
COMMENT ON COLUMN mst_mb_composition_version.mbcv_group_head_id IS
  'Frozen RM-group head ref. Set only for GROUP rows; MB/CARRIER rows store NULL.';

COMMIT;
