-- 000073: Create chat_message_edit_history table for message edit tracking.
CREATE TABLE IF NOT EXISTS chat_message_edit_history (
    history_id     BIGSERIAL   PRIMARY KEY,
    message_id     UUID        NOT NULL REFERENCES chat_message(message_id),
    body_encrypted BYTEA       NOT NULL,
    edited_by      UUID        NOT NULL,
    edited_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_edit_history_msg
    ON chat_message_edit_history(message_id, edited_at DESC);
