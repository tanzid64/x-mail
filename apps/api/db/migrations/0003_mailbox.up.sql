-- ---------------------------------------------------------------------------
-- Labels
--
-- Placement is expressed with system labels (INBOX, SENT, DRAFT, SPAM, TRASH,
-- ARCHIVE) rather than a folder column, so a message can carry user labels at
-- the same time. Read/starred/important are columns on messages instead:
-- they are per-message booleans with no need for the label machinery, and
-- keeping them as columns keeps the hot mailbox queries index-only.
-- ---------------------------------------------------------------------------
CREATE TABLE labels (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id uuid        NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
    name            text        NOT NULL,
    color           text        NOT NULL DEFAULT 'slate',

    -- Set for the built-in labels, NULL for user-created ones. This is what
    -- code matches on, so renaming a system label cannot break placement.
    system_key      text,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT labels_system_key_valid CHECK (
        system_key IS NULL OR
        system_key IN ('INBOX', 'SENT', 'DRAFT', 'SPAM', 'TRASH', 'ARCHIVE')
    ),
    CONSTRAINT labels_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX idx_labels_account ON labels(mail_account_id);
CREATE UNIQUE INDEX idx_labels_account_name ON labels(mail_account_id, lower(btrim(name)));
CREATE UNIQUE INDEX idx_labels_account_system ON labels(mail_account_id, system_key)
    WHERE system_key IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Threads
--
-- Aggregates (message_count, unread_count, …) are maintained by
-- refresh_thread() so the mailbox list never has to touch the messages table.
-- ---------------------------------------------------------------------------
CREATE TABLE threads (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id uuid        NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
    subject         text        NOT NULL DEFAULT '',
    snippet         text        NOT NULL DEFAULT '',

    message_count   integer     NOT NULL DEFAULT 0,
    unread_count    integer     NOT NULL DEFAULT 0,
    has_attachments boolean     NOT NULL DEFAULT false,
    is_starred      boolean     NOT NULL DEFAULT false,

    -- [{"name": "...", "email": "..."}], in the order they joined the thread.
    participants    jsonb       NOT NULL DEFAULT '[]'::jsonb,

    last_message_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_threads_account_last_message
    ON threads(mail_account_id, last_message_at DESC);

CREATE TRIGGER threads_set_updated_at
    BEFORE UPDATE ON threads
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Messages
-- ---------------------------------------------------------------------------
CREATE TABLE messages (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id uuid        NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
    thread_id       uuid        NOT NULL REFERENCES threads(id) ON DELETE CASCADE,

    -- RFC 5322 Message-ID and threading headers.
    message_id      text,
    in_reply_to     text,
    "references"    text[]      NOT NULL DEFAULT '{}',

    -- The provider's own id, used to correlate delivery and bounce webhooks.
    provider_message_id text,

    sender_name     text        NOT NULL DEFAULT '',
    sender_email    citext      NOT NULL,

    subject         text        NOT NULL DEFAULT '',
    snippet         text        NOT NULL DEFAULT '',
    html_body       text        NOT NULL DEFAULT '',
    text_body       text        NOT NULL DEFAULT '',

    direction       text        NOT NULL,
    status          text        NOT NULL DEFAULT 'sent',

    is_read         boolean     NOT NULL DEFAULT false,
    is_starred      boolean     NOT NULL DEFAULT false,
    is_important    boolean     NOT NULL DEFAULT false,
    has_attachments boolean     NOT NULL DEFAULT false,

    size_bytes      integer     NOT NULL DEFAULT 0,

    sent_at         timestamptz,
    received_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_direction_valid CHECK (direction IN ('inbound', 'outbound')),
    CONSTRAINT messages_status_valid CHECK (
        status IN ('draft', 'queued', 'sending', 'sent', 'delivered', 'bounced', 'failed', 'received')
    )
);

CREATE INDEX idx_messages_account     ON messages(mail_account_id);
CREATE INDEX idx_messages_thread      ON messages(thread_id, created_at);
CREATE INDEX idx_messages_received_at ON messages(received_at DESC);
CREATE INDEX idx_messages_sender      ON messages(sender_email);

-- Threading looks a parent up by its Message-ID within one account.
CREATE INDEX idx_messages_message_id ON messages(mail_account_id, message_id)
    WHERE message_id IS NOT NULL;

-- Correlating a provider webhook back to its message must be unambiguous.
CREATE UNIQUE INDEX idx_messages_provider_id ON messages(provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE TRIGGER messages_set_updated_at
    BEFORE UPDATE ON messages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Recipients — never a comma-separated column (docs/PLAN.md §14)
-- ---------------------------------------------------------------------------
CREATE TABLE message_recipients (
    id         uuid   PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id uuid   NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    email      citext NOT NULL,
    name       text   NOT NULL DEFAULT '',
    type       text   NOT NULL,

    CONSTRAINT message_recipients_type_valid CHECK (type IN ('to', 'cc', 'bcc'))
);

CREATE INDEX idx_recipients_message ON message_recipients(message_id);
CREATE INDEX idx_recipients_email   ON message_recipients(email);

-- ---------------------------------------------------------------------------
-- Message ↔ label
-- ---------------------------------------------------------------------------
CREATE TABLE message_labels (
    message_id uuid        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    label_id   uuid        NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (message_id, label_id)
);

CREATE INDEX idx_message_labels_label ON message_labels(label_id);

-- ---------------------------------------------------------------------------
-- refresh_thread keeps a thread's denormalised aggregates truthful.
--
-- Callers invoke it in the same transaction as the change that made it stale,
-- so there is one definition of how a thread summarises its messages and no
-- opportunity for two code paths to compute it differently.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION refresh_thread(target_thread uuid) RETURNS void AS $$
BEGIN
    UPDATE threads t SET
        message_count   = COALESCE(agg.message_count, 0),
        unread_count    = COALESCE(agg.unread_count, 0),
        has_attachments = COALESCE(agg.has_attachments, false),
        is_starred      = COALESCE(agg.is_starred, false),
        subject         = COALESCE(agg.subject, t.subject),
        snippet         = COALESCE(agg.snippet, t.snippet),
        last_message_at = COALESCE(agg.last_message_at, t.last_message_at),
        participants    = COALESCE(agg.participants, '[]'::jsonb)
    FROM (
        SELECT
            count(*)                                      AS message_count,
            count(*) FILTER (WHERE NOT m.is_read
                               AND m.direction = 'inbound') AS unread_count,
            bool_or(m.has_attachments)                    AS has_attachments,
            bool_or(m.is_starred)                         AS is_starred,
            -- The thread takes its subject from the message that started it
            -- and its preview from the most recent one.
            (array_agg(m.subject ORDER BY m.created_at))[1] AS subject,
            (array_agg(m.snippet ORDER BY m.created_at DESC))[1] AS snippet,
            max(COALESCE(m.received_at, m.sent_at, m.created_at)) AS last_message_at,
            (
                SELECT jsonb_agg(p)
                FROM (
                    SELECT DISTINCT ON (lower(inner_m.sender_email))
                           jsonb_build_object('name', inner_m.sender_name,
                                              'email', inner_m.sender_email::text) AS p,
                           min(inner_m.created_at) AS first_seen
                    FROM messages inner_m
                    WHERE inner_m.thread_id = target_thread
                    GROUP BY lower(inner_m.sender_email), inner_m.sender_name, inner_m.sender_email
                    ORDER BY lower(inner_m.sender_email), first_seen
                ) ordered
            ) AS participants
        FROM messages m
        WHERE m.thread_id = target_thread
    ) agg
    WHERE t.id = target_thread;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Every mail account starts with the built-in labels.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION create_system_labels(target_account uuid) RETURNS void AS $$
BEGIN
    INSERT INTO labels (mail_account_id, name, system_key, color)
    VALUES
        (target_account, 'Inbox',   'INBOX',   'slate'),
        (target_account, 'Sent',    'SENT',    'slate'),
        (target_account, 'Drafts',  'DRAFT',   'slate'),
        (target_account, 'Archive', 'ARCHIVE', 'slate'),
        (target_account, 'Spam',    'SPAM',    'amber'),
        (target_account, 'Trash',   'TRASH',   'rose')
    ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql;

-- Backfill for accounts that already exist.
DO $$
DECLARE account_id uuid;
BEGIN
    FOR account_id IN SELECT id FROM mail_accounts LOOP
        PERFORM create_system_labels(account_id);
    END LOOP;
END;
$$;

-- Create them for every account added from here on.
CREATE OR REPLACE FUNCTION mail_accounts_create_system_labels() RETURNS trigger AS $$
BEGIN
    PERFORM create_system_labels(NEW.id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER mail_accounts_system_labels
    AFTER INSERT ON mail_accounts
    FOR EACH ROW EXECUTE FUNCTION mail_accounts_create_system_labels();
