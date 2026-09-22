DROP INDEX IF EXISTS idx_messages_delivered_to;
ALTER TABLE messages
    DROP COLUMN IF EXISTS delivered_to_address_id,
    DROP COLUMN IF EXISTS html_sanitized,
    DROP COLUMN IF EXISTS clicked_at,
    DROP COLUMN IF EXISTS opened_at,
    DROP COLUMN IF EXISTS complained_at,
    DROP COLUMN IF EXISTS bounced_at,
    DROP COLUMN IF EXISTS delivered_at;
DROP TABLE IF EXISTS email_events;
