ALTER TABLE mail_accounts
    DROP COLUMN IF EXISTS webhook_secret_hint,
    DROP COLUMN IF EXISTS webhook_secret_encrypted;
