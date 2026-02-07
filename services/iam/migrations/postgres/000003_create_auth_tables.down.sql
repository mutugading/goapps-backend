-- Rollback migration 000003: Drop authentication tables

DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS password_reset_tokens CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
