-- mst_mb_composition soft-deletes rows (deleted_at), but its uq_mbcm_seq UNIQUE(mbcm_mbh_id,
-- mbcm_seq_no) constraint from migration 000439 is table-wide and does not exclude deleted rows.
-- A deleted row's seq_no stays permanently reserved, so re-adding a line after deleting all
-- active rows collides on seq_no=1 with the soft-deleted row ("duplicate key value violates
-- unique constraint uq_mbcm_seq"). Replace with a partial unique index scoped to active rows,
-- matching the deleted_at-partial-index convention used elsewhere (see CLAUDE.md §7).
ALTER TABLE mst_mb_composition DROP CONSTRAINT IF EXISTS uq_mbcm_seq;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mbcm_seq_active
  ON mst_mb_composition (mbcm_mbh_id, mbcm_seq_no)
  WHERE deleted_at IS NULL;
