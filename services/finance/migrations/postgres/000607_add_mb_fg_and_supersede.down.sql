-- 000607 down - remove only objects created by 000607.

BEGIN;

DROP TABLE IF EXISTS mst_mb_spin_supersede_item;

DROP INDEX IF EXISTS idx_mbs_superseded_by_spin_id;
DROP INDEX IF EXISTS idx_mbs_fg_product_id;
DROP INDEX IF EXISTS uix_mbs_shade_fg_live;
DROP INDEX IF EXISTS idx_mbh_fg_product_id;

ALTER TABLE mst_mb_spin DROP CONSTRAINT IF EXISTS chk_mbs_fg_link_source;

ALTER TABLE mst_mb_spin
    DROP COLUMN IF EXISTS mbs_superseded_by,
    DROP COLUMN IF EXISTS mbs_superseded_at,
    DROP COLUMN IF EXISTS mbs_superseded_by_spin_id,
    DROP COLUMN IF EXISTS mbs_fg_link_source,
    DROP COLUMN IF EXISTS mbs_fg_item_code,
    DROP COLUMN IF EXISTS mbs_fg_product_id;

ALTER TABLE mst_mb_head
    DROP COLUMN IF EXISTS mbh_superseded_by,
    DROP COLUMN IF EXISTS mbh_superseded_at,
    DROP COLUMN IF EXISTS mbh_superseded_by_head_id,
    DROP COLUMN IF EXISTS mbh_fg_item_code,
    DROP COLUMN IF EXISTS mbh_fg_product_id;

COMMIT;
