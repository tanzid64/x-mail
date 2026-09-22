DROP TRIGGER IF EXISTS mail_accounts_system_labels ON mail_accounts;
DROP FUNCTION IF EXISTS mail_accounts_create_system_labels();
DROP FUNCTION IF EXISTS create_system_labels(uuid);
DROP FUNCTION IF EXISTS refresh_thread(uuid);
DROP TABLE IF EXISTS message_labels;
DROP TABLE IF EXISTS message_recipients;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS threads;
DROP TABLE IF EXISTS labels;
