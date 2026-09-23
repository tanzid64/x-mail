-- API tokens let a third-party integration act as a user without a browser
-- session, so every endpoint can be reached by key as well as by login.
--
-- The secret is stored as SHA-256 for the same reason session tokens are: it is
-- a full-entropy random value, so there is nothing to brute force and no reason
-- to pay a slow-hash cost on every authenticated request.
CREATE TABLE api_tokens (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- NULL means every mail account the user owns. A value confines the token
    -- to one mailbox, so an integration that only needs to read support@ is not
    -- also trusted with the owner's personal mail.
    mail_account_id uuid        REFERENCES mail_accounts(id) ON DELETE CASCADE,

    name            text        NOT NULL,
    token_hash      bytea       NOT NULL UNIQUE,
    -- Masked form kept for the list: enough to recognise a token, useless as one.
    hint            text        NOT NULL,

    -- NULL means the token never expires.
    expires_at      timestamptz,
    last_used_at    timestamptz,
    revoked_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT api_tokens_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX idx_api_tokens_user    ON api_tokens(user_id, created_at DESC);
CREATE INDEX idx_api_tokens_account ON api_tokens(mail_account_id) WHERE mail_account_id IS NOT NULL;
