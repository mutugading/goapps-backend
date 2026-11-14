-- 000609 - mst_mb_spin_supersede_item: old/new values needed to revert a supersede
-- (topic mb-spin-fg-poy-link P3, design Revisi 1 D8). Schema only, additive:
-- nullable columns, no data writes. Existing rows (none expected) stay valid.
--   mssi_param_code         param_code of the repointed row (MB_SP_CODE or a fill-group child)
--   mssi_old_mb_spin_id     cpp_value_mb_spin_id before the repoint (no FK: any spin, may be NULL)
--   mssi_old_value_numeric  cpp_value_numeric before (numeric children); text stays in mssi_old_value_text
--   mssi_new_value_numeric  cpp_value_numeric written by the supersede
--   mssi_repoint_kind       POINTER | TEXT_ONLY (MB_SP_CODE row) | CHILD (fill-group child row)

BEGIN;

ALTER TABLE mst_mb_spin_supersede_item
    ADD COLUMN IF NOT EXISTS mssi_param_code VARCHAR(50) NULL,
    ADD COLUMN IF NOT EXISTS mssi_old_mb_spin_id UUID NULL,
    ADD COLUMN IF NOT EXISTS mssi_old_value_numeric NUMERIC NULL,
    ADD COLUMN IF NOT EXISTS mssi_new_value_numeric NUMERIC NULL,
    ADD COLUMN IF NOT EXISTS mssi_repoint_kind VARCHAR(20) NULL;

COMMENT ON COLUMN mst_mb_spin_supersede_item.mssi_repoint_kind IS
    'POINTER | TEXT_ONLY (MB_SP_CODE row matched by pointer / by ORION code text) | CHILD (fill-group child row). NULL old_* on a CHILD row = row did not exist before the supersede.';

COMMIT;
