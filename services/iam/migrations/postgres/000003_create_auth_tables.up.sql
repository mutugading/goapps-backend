-- IAM Service Database Migrations
-- 000003: Create authentication tables
--
-- Tables: user_sessions, password_reset_tokens, api_keys
-- Session management, password reset, and API key authentication

-- =============================================================================
-- USER SESSIONS TABLE (Single device policy - only one active session per user)
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_sessions (
    session_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    refresh_token_hash VARCHAR(500) NOT NULL,  -- SHA256 hash of refresh token
    device_info VARCHAR(100),
    ip_address VARCHAR(50),
    service_name VARCHAR(50) NOT NULL DEFAULT 'goapps',  -- Which service initiated login
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMP WITH TIME ZONE,  -- Null if active
    CONSTRAINT fk_session_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE
);

-- Partial unique index for single device policy (only one active session per user)
CREATE UNIQUE INDEX idx_user_active_session ON user_sessions(user_id) WHERE revoked_at IS NULL;

-- Session indexes
CREATE INDEX idx_session_user ON user_sessions(user_id);
CREATE INDEX idx_session_token ON user_sessions(refresh_token_hash);
CREATE INDEX idx_session_expires ON user_sessions(expires_at) WHERE revoked_at IS NULL;
CREATE INDEX idx_session_service ON user_sessions(service_name) WHERE revoked_at IS NULL;

COMMENT ON TABLE user_sessions IS 'Active user sessions for single-device login policy';
COMMENT ON COLUMN user_sessions.refresh_token_hash IS 'SHA256 hash of the refresh token';

-- =============================================================================
-- PASSWORD RESET TOKENS TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(255) NOT NULL,  -- SHA256 hash of reset token
    otp_code VARCHAR(10),  -- 6-digit OTP for email verification
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_reset_token_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE
);

-- Reset token indexes
CREATE INDEX idx_reset_token_user ON password_reset_tokens(user_id);
CREATE INDEX idx_reset_token_hash ON password_reset_tokens(token_hash);
CREATE INDEX idx_reset_token_otp ON password_reset_tokens(user_id, otp_code) WHERE is_used = false;
CREATE INDEX idx_reset_token_expires ON password_reset_tokens(expires_at) WHERE is_used = false;

COMMENT ON TABLE password_reset_tokens IS 'Password reset tokens and OTP codes';

-- =============================================================================
-- API KEYS TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS api_keys (
    key_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID,  -- Null for service-level keys
    key_name VARCHAR(100) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,  -- SHA256 hash of full API key
    key_prefix VARCHAR(16) NOT NULL,  -- First 16 chars for identification (goapps_xxxx)
    allowed_ips TEXT[],  -- IP whitelist (null = any)
    allowed_scopes TEXT[],  -- Permission codes this key can access
    service_name VARCHAR(50),  -- Target service for service-to-service auth
    rate_limit_per_minute INTEGER DEFAULT 100,
    expires_at TIMESTAMP WITH TIME ZONE,  -- Null = never expires
    last_used_at TIMESTAMP WITH TIME ZONE,
    last_used_ip VARCHAR(50),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_by VARCHAR(100),
    CONSTRAINT fk_api_key_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE,
    CONSTRAINT uq_api_key_name UNIQUE (key_name),
    CONSTRAINT uq_api_key_prefix UNIQUE (key_prefix)
);

-- API key indexes
CREATE INDEX idx_api_key_user ON api_keys(user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_key_prefix ON api_keys(key_prefix) WHERE revoked_at IS NULL AND is_active = true;
CREATE INDEX idx_api_key_service ON api_keys(service_name) WHERE revoked_at IS NULL AND is_active = true;

COMMENT ON TABLE api_keys IS 'API keys for service-to-service and external integrations';
COMMENT ON COLUMN api_keys.key_prefix IS 'First 16 chars of key for lookup without exposing full key';
COMMENT ON COLUMN api_keys.allowed_scopes IS 'Array of permission codes this key can access';
