-- 000078: Create chat_attachment table for file/image attachments on chat messages.
CREATE TABLE IF NOT EXISTS chat_attachment (
    attachment_id    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id  UUID        NOT NULL REFERENCES chat_conversation(conversation_id),
    message_id       UUID        REFERENCES chat_message(message_id),
    uploader_user_id UUID        NOT NULL,
    file_name        VARCHAR(255) NOT NULL,
    file_url         VARCHAR(500) NOT NULL,
    content_type     VARCHAR(100) NOT NULL,
    file_size        BIGINT      NOT NULL,
    thumbnail_url    VARCHAR(500),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_attachment_message ON chat_attachment(message_id);
CREATE INDEX IF NOT EXISTS idx_chat_attachment_conv ON chat_attachment(conversation_id);
