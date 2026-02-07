-- Rollback migration 000005: Drop menu tables

DROP TABLE IF EXISTS menu_permissions CASCADE;
DROP TABLE IF EXISTS mst_menu CASCADE;
