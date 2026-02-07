-- IAM Service Database Migrations
-- 000006: Create audit log table
--
-- Table: audit_logs
-- Comprehensive audit logging for all user activities

-- =============================================================================
-- AUDIT LOGS TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    log_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(50) NOT NULL,  -- LOGIN, LOGOUT, LOGIN_FAILED, CREATE, UPDATE, DELETE, EXPORT, IMPORT
    table_name VARCHAR(100),  -- Target table for CRUD events (null for auth events)
    record_id UUID,           -- Target record ID (null for auth events)
    user_id UUID,             -- Who performed the action (null for anonymous)
    username VARCHAR(100),    -- Denormalized for history (in case user is deleted)
    full_name VARCHAR(100),   -- Denormalized
    ip_address VARCHAR(50),
    user_agent VARCHAR(255),
    service_name VARCHAR(50) NOT NULL DEFAULT 'iam',  -- Which service generated this log
    old_data JSONB,           -- Previous state (for UPDATE/DELETE)
    new_data JSONB,           -- New state (for CREATE/UPDATE)
    changes JSONB,            -- Only changed fields (for UPDATE)
    performed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_audit_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE SET NULL,
    CONSTRAINT chk_event_type CHECK (event_type IN (
        'LOGIN', 'LOGOUT', 'LOGIN_FAILED', 'PASSWORD_RESET', 'PASSWORD_CHANGE',
        '2FA_ENABLED', '2FA_DISABLED', 'CREATE', 'UPDATE', 'DELETE', 'EXPORT', 'IMPORT'
    ))
);

-- Audit log indexes (optimized for common queries)
CREATE INDEX idx_audit_event ON audit_logs(event_type);
CREATE INDEX idx_audit_user ON audit_logs(user_id);
CREATE INDEX idx_audit_table ON audit_logs(table_name) WHERE table_name IS NOT NULL;
CREATE INDEX idx_audit_record ON audit_logs(table_name, record_id) WHERE record_id IS NOT NULL;
CREATE INDEX idx_audit_service ON audit_logs(service_name);
CREATE INDEX idx_audit_performed ON audit_logs(performed_at DESC);
CREATE INDEX idx_audit_user_time ON audit_logs(user_id, performed_at DESC);
CREATE INDEX idx_audit_search ON audit_logs USING gin(
    (COALESCE(username, '') || ' ' || COALESCE(full_name, '')) gin_trgm_ops
);

-- Partitioning by month for better performance (optional, can be enabled later)
-- This is a comment placeholder - actual partitioning would require table recreation

COMMENT ON TABLE audit_logs IS 'Comprehensive audit log for all user activities';
COMMENT ON COLUMN audit_logs.old_data IS 'Previous state as JSON (for UPDATE/DELETE)';
COMMENT ON COLUMN audit_logs.new_data IS 'New state as JSON (for CREATE/UPDATE)';
COMMENT ON COLUMN audit_logs.changes IS 'Only changed fields as JSON (for UPDATE)';
