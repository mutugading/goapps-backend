-- IAM Service Database Migrations
-- 000007: Create recovery codes table for 2FA
--
-- Tables: user_recovery_codes
-- Stores hashed recovery codes for 2FA backup authentication

-- =============================================================================
-- USER RECOVERY CODES TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_recovery_codes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    code_hash VARCHAR(255) NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_recovery_code_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE
);

-- Recovery code indexes
CREATE INDEX IF NOT EXISTS idx_recovery_code_user ON user_recovery_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_recovery_code_lookup ON user_recovery_codes(user_id, code_hash) WHERE used_at IS NULL;

COMMENT ON TABLE user_recovery_codes IS 'Hashed recovery codes for 2FA backup authentication';
COMMENT ON COLUMN user_recovery_codes.code_hash IS 'SHA256 hash of the recovery code';
