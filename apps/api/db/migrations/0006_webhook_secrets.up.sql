-- Each provider account signs its webhooks with its own secret, so the
-- endpoint is per-account: /api/v1/webhooks/resend/{mail_account_id}. Knowing
-- which account a callback claims to be for is what lets us pick the one
-- secret to verify against, rather than trying every secret in the system.
--
-- Encrypted with the same master key as the API key: a webhook secret is
-- equally capable of letting an attacker inject mail into someone's inbox.
ALTER TABLE mail_accounts
    ADD COLUMN webhook_secret_encrypted bytea,
    ADD COLUMN webhook_secret_hint      text NOT NULL DEFAULT '';
