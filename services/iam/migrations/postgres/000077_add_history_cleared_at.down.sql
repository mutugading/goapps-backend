-- 000077: Rollback history_cleared_at from chat_participant.
ALTER TABLE chat_participant DROP COLUMN IF EXISTS history_cleared_at;
