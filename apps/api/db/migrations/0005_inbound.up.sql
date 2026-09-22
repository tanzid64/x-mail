-- ---------------------------------------------------------------------------
-- Provider events
--
-- Every webhook the provider sends is recorded here before anything is done
-- with it. provider_event_id is unique, so a webhook delivered twice — which
-- providers do, by design, whenever they are unsure we received it — creates
-- exactly one row and therefore at most one piece of work.
-- ---------------------------------------------------------------------------
CREATE TABLE email_events (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id   uuid        REFERENCES mail_accounts(id) ON DELETE CASCADE,
    message_id        uuid        REFERENCES messages(id) ON DELETE SET NULL,

    provider          text        NOT NULL,
    provider_event_id text        NOT NULL,
    event_type        text        NOT NULL,

    payload           jsonb       NOT NULL,

    -- Set once the event has been acted on, so a replay is visible as such.
    processed_at      timestamptz,
    process_error     text        NOT NULL DEFAULT '',

    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_email_events_provider_event
    ON email_events(provider, provider_event_id);
CREATE INDEX idx_email_events_message ON email_events(message_id);
CREATE INDEX idx_email_events_account ON email_events(mail_account_id, created_at DESC);
CREATE INDEX idx_email_events_unprocessed ON email_events(created_at) WHERE processed_at IS NULL;

-- ---------------------------------------------------------------------------
-- Inbound message support
-- ---------------------------------------------------------------------------

-- Delivery lifecycle timestamps, so the UI can show what happened and when
-- without replaying the event log.
ALTER TABLE messages
    ADD COLUMN delivered_at  timestamptz,
    ADD COLUMN bounced_at    timestamptz,
    ADD COLUMN complained_at timestamptz,
    ADD COLUMN opened_at     timestamptz,
    ADD COLUMN clicked_at    timestamptz,
    -- The sanitised HTML actually rendered to the reader. The original stays in
    -- html_body so a sanitiser bug can be corrected by re-sanitising rather
    -- than by losing the message.
    ADD COLUMN html_sanitized text NOT NULL DEFAULT '';

-- An inbound message arrives addressed to one of our own addresses; recording
-- which one makes replying pick the right identity automatically.
ALTER TABLE messages
    ADD COLUMN delivered_to_address_id uuid REFERENCES mail_addresses(id) ON DELETE SET NULL;

CREATE INDEX idx_messages_delivered_to ON messages(delivered_to_address_id);
