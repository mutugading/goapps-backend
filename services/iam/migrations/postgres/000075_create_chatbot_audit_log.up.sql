-- 000075: Create chatbot_audit_log table for AI chatbot usage tracking.
CREATE TABLE IF NOT EXISTS chatbot_audit_log (
    log_id          BIGSERIAL    PRIMARY KEY,
    user_id         UUID         NOT NULL,
    session_id      VARCHAR(100) NOT NULL,
    request_tokens  INT          NOT NULL DEFAULT 0,
    response_tokens INT          NOT NULL DEFAULT 0,
    tools_called    TEXT[],
    was_blocked     BOOLEAN      NOT NULL DEFAULT FALSE,
    block_reason    VARCHAR(200),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chatbot_audit_user_date
    ON chatbot_audit_log(user_id, created_at DESC);
