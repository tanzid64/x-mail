-- ---------------------------------------------------------------------------
-- Drafts
--
-- A draft is kept separate from messages until it is sent. Autosave writes to
-- this table many times a minute, and a half-typed row must never be reachable
-- by the sending path.
-- ---------------------------------------------------------------------------
CREATE TABLE drafts (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id uuid        NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,

    -- Set when the draft is a reply, so sending files it into the conversation.
    thread_id       uuid        REFERENCES threads(id) ON DELETE SET NULL,
    -- The message being replied to or forwarded, for the threading headers.
    in_reply_to_id  uuid        REFERENCES messages(id) ON DELETE SET NULL,

    from_address_id uuid        REFERENCES mail_addresses(id) ON DELETE SET NULL,

    -- Recipients are plain arrays here rather than rows: a draft changes on
    -- every keystroke, and rewriting a child table that often would be wasteful.
    -- They become message_recipients rows when the draft is sent.
    "to"            jsonb       NOT NULL DEFAULT '[]'::jsonb,
    cc              jsonb       NOT NULL DEFAULT '[]'::jsonb,
    bcc             jsonb       NOT NULL DEFAULT '[]'::jsonb,

    subject         text        NOT NULL DEFAULT '',
    html_body       text        NOT NULL DEFAULT '',
    text_body       text        NOT NULL DEFAULT '',

    -- draft | sending: flipped when a send starts, so autosave cannot modify a
    -- draft that is already on its way out.
    status          text        NOT NULL DEFAULT 'draft',

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT drafts_status_valid CHECK (status IN ('draft', 'sending'))
);

CREATE INDEX idx_drafts_account ON drafts(mail_account_id, updated_at DESC);
CREATE INDEX idx_drafts_thread  ON drafts(thread_id);

CREATE TRIGGER drafts_set_updated_at
    BEFORE UPDATE ON drafts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Attachments
--
-- Bytes live in object storage; the row holds only the key and metadata.
-- An attachment starts life owned by a draft and is re-pointed at the message
-- once that draft is sent.
-- ---------------------------------------------------------------------------
CREATE TABLE attachments (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id   uuid        REFERENCES messages(id) ON DELETE CASCADE,
    draft_id     uuid        REFERENCES drafts(id) ON DELETE CASCADE,

    filename     text        NOT NULL,
    mime_type    text        NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   bigint      NOT NULL DEFAULT 0,
    storage_key  text        NOT NULL,

    -- Set for images referenced by cid: in the HTML body.
    content_id   text,
    is_inline    boolean     NOT NULL DEFAULT false,

    created_at   timestamptz NOT NULL DEFAULT now(),

    -- Every attachment belongs to exactly one of a draft or a message.
    CONSTRAINT attachments_one_owner CHECK (
        (message_id IS NOT NULL AND draft_id IS NULL) OR
        (message_id IS NULL AND draft_id IS NOT NULL)
    )
);

CREATE INDEX idx_attachments_message ON attachments(message_id);
CREATE INDEX idx_attachments_draft   ON attachments(draft_id);
CREATE UNIQUE INDEX idx_attachments_storage_key ON attachments(storage_key);

-- ---------------------------------------------------------------------------
-- Sending
--
-- idempotency_key is generated once, when the send is queued, and handed to the
-- provider on every attempt. If a worker crashes after the provider accepted
-- the message but before the result was recorded, the retry carries the same
-- key and the provider collapses it into the original delivery rather than
-- sending twice.
-- ---------------------------------------------------------------------------
ALTER TABLE messages
    ADD COLUMN idempotency_key text,
    ADD COLUMN failure_reason  text NOT NULL DEFAULT '',
    ADD COLUMN attempt_count   integer NOT NULL DEFAULT 0,
    ADD COLUMN last_attempt_at timestamptz;

CREATE UNIQUE INDEX idx_messages_idempotency_key ON messages(idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Finding messages stuck mid-send, for the retry sweeper.
CREATE INDEX idx_messages_status_pending ON messages(status, last_attempt_at)
    WHERE status IN ('queued', 'sending');
