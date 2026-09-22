DROP INDEX IF EXISTS idx_messages_status_pending;
DROP INDEX IF EXISTS idx_messages_idempotency_key;
ALTER TABLE messages
    DROP COLUMN IF EXISTS last_attempt_at,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS idempotency_key;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS drafts;
