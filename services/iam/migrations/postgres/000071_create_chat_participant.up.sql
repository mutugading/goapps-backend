-- 000071: Create chat_participant table linking users to conversations.
CREATE TABLE IF NOT EXISTS chat_participant (
    conversation_id UUID        NOT NULL REFERENCES chat_conversation(conversation_id),
    user_id         UUID        NOT NULL,
    role            VARCHAR(20) NOT NULL DEFAULT 'MEMBER'
                                CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER')),
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    left_at         TIMESTAMPTZ,
    last_read_at    TIMESTAMPTZ,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_participant_user
    ON chat_participant(user_id)
    WHERE left_at IS NULL;
