-- Rollback migration 000004: Drop RBAC tables

DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS user_permissions CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS mst_permission CASCADE;
DROP TABLE IF EXISTS mst_role CASCADE;
