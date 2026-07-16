-- 000077: Add history_cleared_at to chat_participant for per-user "Clear History".
ALTER TABLE chat_participant ADD COLUMN IF NOT EXISTS history_cleared_at TIMESTAMPTZ;
