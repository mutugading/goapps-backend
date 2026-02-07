-- Rollback migration 000001: Drop organization hierarchy tables

DROP TABLE IF EXISTS mst_section CASCADE;
DROP TABLE IF EXISTS mst_department CASCADE;
DROP TABLE IF EXISTS mst_division CASCADE;
DROP TABLE IF EXISTS mst_company CASCADE;
