-- 000609 down - remove only the columns added by 000609.

BEGIN;

ALTER TABLE mst_mb_spin_supersede_item
    DROP COLUMN IF EXISTS mssi_repoint_kind,
    DROP COLUMN IF EXISTS mssi_new_value_numeric,
    DROP COLUMN IF EXISTS mssi_old_value_numeric,
    DROP COLUMN IF EXISTS mssi_old_mb_spin_id,
    DROP COLUMN IF EXISTS mssi_param_code;

COMMIT;
