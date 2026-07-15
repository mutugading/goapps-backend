-- 000072: Create chat_message table with encrypted body storage.
CREATE TABLE IF NOT EXISTS chat_message (
    message_id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id      UUID        NOT NULL REFERENCES chat_conversation(conversation_id),
    sender_user_id       UUID        NOT NULL,
    body_encrypted       BYTEA       NOT NULL,
    body_plain_encrypted BYTEA       NOT NULL,
    is_edited            BOOLEAN     NOT NULL DEFAULT FALSE,
    is_deleted           BOOLEAN     NOT NULL DEFAULT FALSE,
    reply_to_id          UUID        REFERENCES chat_message(message_id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_message_conv_created
    ON chat_message(conversation_id, created_at DESC)
    WHERE is_deleted = FALSE;

CREATE INDEX IF NOT EXISTS idx_chat_message_sender
    ON chat_message(sender_user_id);
