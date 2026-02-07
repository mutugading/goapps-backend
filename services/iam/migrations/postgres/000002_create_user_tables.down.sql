-- Rollback migration 000002: Drop user tables

DROP TABLE IF EXISTS mst_user_detail CASCADE;
DROP TABLE IF EXISTS mst_user CASCADE;
