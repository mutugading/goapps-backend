-- Rollback migration 000006: Drop audit log table

DROP TABLE IF EXISTS audit_logs CASCADE;
