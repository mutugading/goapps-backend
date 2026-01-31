-- Rollback: Drop mst_uom table

DROP INDEX IF EXISTS idx_mst_uom_search;
DROP INDEX IF EXISTS idx_mst_uom_created_at;
DROP INDEX IF EXISTS idx_mst_uom_active;
DROP INDEX IF EXISTS idx_mst_uom_category;
DROP INDEX IF EXISTS idx_mst_uom_code;
DROP TABLE IF EXISTS mst_uom;
