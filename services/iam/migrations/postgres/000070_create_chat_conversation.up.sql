-- 000070: Create chat_conversation table for direct and group conversations.
CREATE TABLE IF NOT EXISTS chat_conversation (
    conversation_id UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    type            VARCHAR(10)  NOT NULL CHECK (type IN ('DIRECT', 'GROUP')),
    name            VARCHAR(200),
    avatar_url      VARCHAR(500),
    encryption_key  BYTEA        NOT NULL,
    created_by      VARCHAR(100) NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    deleted_by      VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_chat_conversation_created
    ON chat_conversation(created_at DESC)
    WHERE deleted_at IS NULL;
