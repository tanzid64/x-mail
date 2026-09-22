-- A user owns many mail accounts; each carries its own provider credentials.
-- Deliberately NOT "one user = one email address" (see docs/PLAN.md §4).
CREATE TABLE mail_accounts (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                text        NOT NULL,
    provider            text        NOT NULL,
    provider_account_id text,

    -- AES-256-GCM ciphertext; the master key lives outside PostgreSQL.
    -- api_key_hint is the masked form, and the only part the API may return.
    api_key_encrypted   bytea       NOT NULL,
    api_key_hint        text        NOT NULL DEFAULT '',

    -- active | invalid_credentials | disabled
    status              text        NOT NULL DEFAULT 'active',
    status_message      text        NOT NULL DEFAULT '',
    last_checked_at     timestamptz,

    -- Shown as the dot beside the account in the switcher.
    color               text        NOT NULL DEFAULT 'slate',

    is_default          boolean     NOT NULL DEFAULT false,
    is_active           boolean     NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT mail_accounts_provider_valid CHECK (provider IN ('resend')),
    CONSTRAINT mail_accounts_status_valid
        CHECK (status IN ('active', 'invalid_credentials', 'disabled')),
    CONSTRAINT mail_accounts_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX idx_mail_accounts_user ON mail_accounts(user_id);

-- Distinct names per user, so the switcher is never ambiguous.
CREATE UNIQUE INDEX idx_mail_accounts_user_name ON mail_accounts(user_id, lower(btrim(name)));

-- At most one default account per user, enforced by the database rather than
-- by whichever code path happens to set the flag.
CREATE UNIQUE INDEX idx_mail_accounts_one_default
    ON mail_accounts(user_id) WHERE is_default;

CREATE TRIGGER mail_accounts_set_updated_at
    BEFORE UPDATE ON mail_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A provider account may hold several sending domains.
CREATE TABLE domains (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    mail_account_id    uuid        NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
    domain             citext      NOT NULL,
    provider_domain_id text        NOT NULL,

    -- pending | verified | failed
    status             text        NOT NULL DEFAULT 'pending',
    region             text        NOT NULL DEFAULT '',
    -- DNS records the user must publish, as returned by the provider.
    records            jsonb       NOT NULL DEFAULT '[]'::jsonb,
    last_synced_at     timestamptz,

    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT domains_status_valid CHECK (status IN ('pending', 'verified', 'failed'))
);

CREATE INDEX idx_domains_mail_account ON domains(mail_account_id);
CREATE UNIQUE INDEX idx_domains_account_domain ON domains(mail_account_id, domain);
CREATE UNIQUE INDEX idx_domains_provider_id ON domains(mail_account_id, provider_domain_id);

CREATE TRIGGER domains_set_updated_at
    BEFORE UPDATE ON domains
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Individual sender/recipient addresses on a domain.
CREATE TABLE mail_addresses (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id    uuid        NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    email        citext      NOT NULL,
    display_name text        NOT NULL DEFAULT '',
    is_default   boolean     NOT NULL DEFAULT false,
    is_active    boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Globally unique: inbound routing looks an incoming recipient up by address
-- alone, so two accounts claiming the same address would make routing
-- ambiguous (docs/PLAN.md §22).
CREATE UNIQUE INDEX idx_mail_addresses_email ON mail_addresses(email);
CREATE INDEX idx_mail_addresses_domain ON mail_addresses(domain_id);

-- One default sender per domain.
CREATE UNIQUE INDEX idx_mail_addresses_one_default
    ON mail_addresses(domain_id) WHERE is_default;

CREATE TRIGGER mail_addresses_set_updated_at
    BEFORE UPDATE ON mail_addresses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
