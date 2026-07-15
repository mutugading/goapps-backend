-- 000074: Create chat_read_receipt table for per-message read tracking.
CREATE TABLE IF NOT EXISTS chat_read_receipt (
    message_id UUID        NOT NULL REFERENCES chat_message(message_id),
    user_id    UUID        NOT NULL,
    read_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_read_receipt_msg
    ON chat_read_receipt(message_id);
