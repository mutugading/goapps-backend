-- 000078 down: Drop chat_attachment table and its indexes.
DROP INDEX IF EXISTS idx_chat_attachment_conv;
DROP INDEX IF EXISTS idx_chat_attachment_message;
DROP TABLE IF EXISTS chat_attachment;
