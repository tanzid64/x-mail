# Gmail-Like Web Mail Application — Full Technical & Product Plan

> This is the authoritative product/architecture specification for x-mail.
> Implementation progress is tracked in [`PROGRESS.md`](../PROGRESS.md).

## 1. Project Vision

Build a modern, Gmail-like web mail application using:

- **Frontend:** React + TypeScript + Vite + shadcn/ui + Tailwind CSS
- **Backend:** Go
- **Database:** PostgreSQL
- **Cache / Queue:** Redis + Asynq
- **Object Storage:** Cloudflare R2 or S3-compatible storage or local storage
- **Email provider:** Resend
- **Search:** PostgreSQL FTS initially
- **Realtime:** WebSocket
- **Rich text:** Free opensource and native app support
- **Deployment:** Docker + Nginx + Cloudflare
- **MCP:** Shadcn and resend connected

The application should support **multiple independent Resend accounts per user**, similar to Gmail's multiple-account experience.

The core architectural principle is:

> **Resend is the email transport/infrastructure layer. The Go application owns the mailbox, threads, labels, drafts, search, users, permissions, and UI state.**

The system should be designed as a **mail platform**, not merely a Gmail UI clone. Resend should be treated as one provider, allowing future support for SES, SendGrid, Mailgun, Postmark, or SMTP.

---

# 2. Product Goals

## Primary goals

1. Gmail-like inbox experience.
2. Multiple Resend accounts per user.
3. Multiple domains and sender addresses per Resend account.
4. Send and receive emails.
5. Thread/conversation support.
6. Draft autosaving.
7. Attachments.
8. Search.
9. Labels.
10. Realtime incoming mail.
11. Scheduled sending.
12. Snoozing.
13. Contacts.
14. Signatures.
15. Account switching.
16. Unified inbox across accounts.
17. Secure provider credential storage.
18. Background email processing.
19. Extensible provider abstraction.
20. Future support for teams/shared inboxes.

---

# 3. High-Level Architecture

```text
                         ┌──────────────────────┐
                         │        React         │
                         │ TypeScript + Vite    │
                         │ shadcn/ui + Tailwind │
                         └──────────┬───────────┘
                                    │
                             REST + WebSocket
                                    │
                         ┌──────────▼───────────┐
                         │        Go API        │
                         │   Mail Application   │
                         └──────────┬───────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
              ▼                     ▼                     ▼
        ┌───────────┐         ┌───────────┐         ┌────────────┐
        │ PostgreSQL│         │   Redis   │         │ Storage    │
        │ Mail data │         │ Cache     │         │ Attachments│
        │ Users     │         │ Queue     │         │            │
        └───────────┘         └─────┬─────┘         └────────────┘
                                    │
                              ┌─────▼─────┐
                              │ Go Worker │
                              │  Asynq    │
                              └─────┬─────┘
                                    │
                    ┌───────────────┼────────────────┐
                    │               │                │
                    ▼               ▼                ▼
              ┌──────────┐    ┌──────────┐     ┌──────────┐
              │ Resend #1│    │ Resend #2│     │ Resend #3│
              │ Personal │    │   Work   │     │ Support  │
              └──────────┘    └──────────┘     └──────────┘
```

---

# 4. Core Architectural Principle: Multiple Mail Accounts

Do **not** model:

```text
User = Email Account
```

Instead:

```text
User
 ├── Mail Account 1
 ├── Mail Account 2
 └── Mail Account 3
```

Each `MailAccount` has its own provider credentials, domains, addresses, mailboxes, settings, and messages.

---

# 5. Provider Abstraction

Even though Resend is the first provider, the application should not hard-code Resend into the entire mail domain.

```go
type MailProvider interface {
    Send(ctx context.Context, email Email) (*SendResult, error)
    GetDomains(ctx context.Context) ([]Domain, error)
    GetDomain(ctx context.Context, id string) (*Domain, error)
}
```

Future providers (SES, SMTP, …) implement the same interface.

---

# 6. Recommended Technology Stack

## Backend

```text
Go
Chi
pgx
sqlc
PostgreSQL
Redis
Asynq
WebSocket
Resend API
OpenTelemetry
```

## Frontend

```text
React
TypeScript
Vite
Tailwind CSS
shadcn/ui
TanStack Router
TanStack Query
Zustand
React Hook Form
Zod
TipTap
```

## Infrastructure

```text
Docker
Docker Compose
Nginx
Cloudflare
Cloudflare R2
PostgreSQL
Redis
```

## Optional later

```text
Typesense
Prometheus
Grafana
Sentry
OpenTelemetry Collector
```

---

# 7. Product Modules

```text
1. Authentication        14. Contacts
2. Users                 15. Notifications
3. Mail Accounts         16. Realtime
4. Domains               17. Scheduled Mail
5. Email Addresses       18. Snooze
6. Inbox                 19. Filters
7. Threads               20. Signatures
8. Messages              21. Settings
9. Drafts                22. Administration
10. Compose              23. Provider Integrations
11. Attachments          24. Security
12. Labels               25. Audit Logs
13. Search               26. AI Features
```

---

# 8. Authentication

## MVP

- Registration, Login, Logout
- Email verification
- Forgot password / Password reset
- Session management

## Later

- Google OAuth, GitHub OAuth, Passkeys, 2FA, Recovery codes, Trusted devices, Session/device management

Tables: `users`, `sessions`, `oauth_accounts`, `password_resets`, `email_verifications`, `two_factor_auth`

Passwords should use **Argon2id**.

---

# 9. Mail Account Model

```sql
mail_accounts
-------------
id
user_id
name
provider
provider_account_id
api_key_encrypted
api_key_hint
status
is_default
is_active
created_at
updated_at
```

Important:

- Never expose provider API keys to React.
- Never store API keys in plaintext.
- Encrypt credentials at the application layer.
- Store the encryption master key outside PostgreSQL.
- Consider a secrets manager in production.

---

# 10. Resend Account vs Mail Address

Do not assume `1 Resend account = 1 email address`.

```text
Resend Account
     ├── Domain A ── hello@, support@
     └── Domain B ── sales@, billing@
```

### domains

```sql
domains
-------
id
mail_account_id
domain
provider_domain_id
status
created_at
updated_at
```

### mail_addresses

```sql
mail_addresses
--------------
id
domain_id
email
display_name
is_default
is_active
created_at
updated_at
```

---

# 11. Message Ownership

Messages belong to a mail account, never directly to a user.

```sql
messages
--------
id
mail_account_id
thread_id
...
```

This prevents accidental mixing between Personal / Work / Support.

---

# 12. Threads

```sql
threads
-------
id
mail_account_id
subject
snippet
message_count
last_message_at
created_at
updated_at
```

Use email headers to establish relationships: `Message-ID`, `In-Reply-To`, `References`.
The thread engine should use these headers first and have fallback heuristics when required.

---

# 13. Messages

```sql
messages
--------
id
mail_account_id
thread_id

message_id
in_reply_to
references

sender_name
sender_email

subject

html_body
text_body

direction
status

is_read
is_starred
is_important

sent_at
received_at

created_at
updated_at
```

Direction: `inbound`, `outbound`
Status: `draft`, `queued`, `sending`, `sent`, `delivered`, `bounced`, `failed`

---

# 14. Recipients

Do not store recipients in a comma-separated field.

```sql
message_recipients
------------------
id
message_id
email
name
type   -- to | cc | bcc
```

---

# 15. Inbox

System labels/folders: `Inbox`, `Starred`, `Snoozed`, `Sent`, `Drafts`, `Important`, `Spam`, `Trash`

Features: pagination, infinite scrolling, read/unread, star, important, archive, delete, spam,
bulk actions, labels, snooze, select all, select visible, keyboard navigation.

---

# 16. Unified Inbox

Two modes: account-specific and "All accounts".
Unified inbox queries messages across selected `mail_account_id` values and sorts by `last_message_at`.
Every row should clearly indicate its originating account.

---

# 17. Account Switcher UI

```text
┌─────────────────────────────┐
│ 🟢 Personal                 │
│    dark@gmail.com           │
│ 🔵 Work                     │
│    dark@company.com         │
│ 🟣 Support                  │
│    support@company.com      │
│ ─────────────────────────── │
│ ✓ All accounts              │
│ + Add mail account          │
└─────────────────────────────┘
```

---

# 18. Compose

Features: from-account selector, To/CC/BCC, subject, rich text, attachments, inline images,
draft autosave, reply, reply all, forward, signature, scheduled send.

The selected mail account determines: provider, API key, sender identity, domain, signature, account settings.

---

# 19. Sending Architecture

Never call Resend directly from the browser.

```text
React → Go API → Validate → Create message → Create recipients → Store attachments
      → Queue job → Redis → Go Worker → Load mail account → Decrypt credentials
      → Create Resend provider → Send → Store result
```

This allows retries, rate limiting, idempotency, failure recovery, logging, delivery tracking.

---

# 20. Idempotency

Email sending must be idempotent. Use deterministic idempotency keys for send operations where
supported, and maintain an application-level send record.

---

# 21. Receiving Email

```text
External sender → Recipient address → Resend → email.received webhook → Go webhook
→ Verify signature → Parse recipient → Find mail address → Find mail account
→ Queue processing job → Parse/store message → Find/create thread → Store attachments
→ Index search → Publish realtime event → React inbox updates
```

---

# 22. Inbound Routing

The routing table should make the recipient address the primary lookup key:

```text
mail_addresses → domain → mail_account → user / organization
```

---

# 23. Resend Webhook

`POST /api/v1/webhooks/resend`

1. Read raw request body
2. Verify signature
3. Parse event
4. Check idempotency
5. Identify provider/account
6. Queue processing
7. Return quickly

Never perform heavy processing inside the webhook request.

```sql
email_events
------------
id
mail_account_id
message_id
provider
provider_event_id
event_type
payload
created_at
```

---

# 24. Attachments

Never store large attachment blobs in PostgreSQL. Use R2 / S3-compatible storage.

```sql
attachments
-----------
id
message_id
filename
mime_type
size
storage_key
content_id
is_inline
created_at
```

Layout: `mail/attachments/2026/09/uuid.pdf`

Security: file size limits, allowed MIME types, filename sanitization, content-type validation,
malware scanning where appropriate, signed download URLs, authorization before every download.

---

# 25. Drafts

Autosave with a 2–5 second debounce → `PATCH /drafts/:id`.

```sql
drafts
------
id
mail_account_id
thread_id
to
cc
bcc
subject
html_body
text_body
updated_at
```

Drafts must never be accidentally sent by autosave.

---

# 26. Rich Text Editor

Use TipTap. Extensions: Bold, Italic, Underline, Strike, Links, Headings, Lists, Blockquote,
Code, Images, Text alignment, Mentions.

Store both `html_body` and `text_body`. Plain text is used for search, fallback mail,
accessibility, previews, and indexing.

---

# 27. Labels

```sql
labels
------
id
mail_account_id
name
color
system
```

System labels: `INBOX`, `SENT`, `DRAFT`, `SPAM`, `TRASH`, `STARRED`, `IMPORTANT`

```sql
message_labels
--------------
message_id
label_id
```

---

# 28. Search

## V1 — PostgreSQL full-text search

Operators to support eventually:

```text
from:john@example.com   to:jane@example.com   subject:invoice
has:attachment          is:unread             is:starred
after:2026-09-01        before:2026-10-01
```

## V2 — Typesense

```text
PostgreSQL → Indexer → Typesense
```

---

# 29. Contacts

```sql
contacts
--------
id
user_id
name
email
avatar
company
notes
last_contacted_at
```

Features: recent contacts, frequent contacts, autocomplete, contact details, groups, manual contacts.

---

# 30. Signatures

Signatures are associated with the appropriate mail account (different accounts need different
signatures). Toggles for "new emails" and "replies".

---

# 31. Snooze

```sql
snoozed_messages
----------------
message_id
user_id
wake_at
```

Worker: every minute → find `wake_at <= now()` → restore inbox state → notify user.

---

# 32. Scheduled Sending

```sql
scheduled_messages
------------------
id
message_id
scheduled_at
status
```

Worker: `scheduled_at <= NOW()` → enqueue SendEmail job → send through selected provider/account.

---

# 33. Notifications

```text
Resend webhook → Go → PostgreSQL → Redis Pub/Sub → WebSocket → React
```

Events: `email.received`, `email.read`, `email.updated`, `email.deleted`, `draft.updated`,
`notification.created`

---

# 34. WebSocket

Endpoint `/ws`, authenticated with the application's secure session mechanism.

```json
{
  "type": "email.received",
  "data": { "mail_account_id": "123", "thread_id": "456", "message_id": "789" }
}
```

React should use events to invalidate/refetch TanStack Query data rather than manually
reconstructing inbox state.

---

# 35. Filters and Automation

```text
IF sender contains "github.com"
THEN apply label "Development", mark important, never spam
```

Tables: `filters`, `filter_conditions`, `filter_actions`

---

# 36. Multiple Accounts and Sending

```json
{
  "mail_account_id": "work-account-id",
  "from_address_id": "dark-company-address-id",
  "to": [],
  "subject": "Project update",
  "html": "..."
}
```

The backend loads the provider configuration and never trusts provider credentials sent by the frontend.

---

# 37. Multiple Resend API Keys

Each mail account has its own API key and is independently configurable. The user can disable or
delete one account without affecting the others.

---

# 38. Account Settings

Name, provider, email addresses, domains, default sender, signature, reply-to,
notification preferences, sending limits, status.

---

# 39. Security

## Authentication

Argon2id, secure sessions, refresh token rotation if token-based auth is used, rate limiting,
login throttling, email verification, optional 2FA.

## Email content

Inbound HTML must be sanitized. Never directly render arbitrary email HTML.
Protect against: XSS, malicious links, tracking pixels, unsafe iframes, JavaScript URLs,
dangerous HTML, malicious SVG, attachment attacks.

## Provider credentials

Never expose the Resend API key, webhook secret, or encryption master key to the browser.

---

# 40. API Rate Limiting

Limit at the level of: IP, user, mail account, provider account, endpoint.
Applied to: login, send email, create account, upload attachment, search, webhook.

---

# 41. Abuse Prevention

Per-user sending limits, per-account sending limits, IP rate limits, daily limits,
bounce monitoring, complaint monitoring, suspicious activity detection, domain restrictions,
queue concurrency limits, retry limits.

---

# 42. Admin Panel

Dashboard, Users, Organizations, Mail Accounts, Domains, Email Activity, Webhooks, Queue,
Storage, Abuse, Audit Logs, System Settings.

Metrics: total users, active users, messages today, inbound/outbound messages, failed sends,
bounces, complaints, queue depth, storage usage, provider errors.

---

# 43. Organizations and Teams

Tables: `organizations`, `organization_members`, `mail_accounts`, `mail_account_members`
Roles: `owner`, `admin`, `agent`, `member`, `viewer`

---

# 44. Shared Inbox

Assignment, internal notes, status, tags, team permissions, collision detection, presence,
shared drafts, SLA tracking later.

---

# 45. AI Features — Later

Not part of the initial MVP. Smart reply, summarization, ask-about-email, task extraction, AI search.
The AI layer should produce structured search/filter operations rather than directly querying the database.

---

# 46. Frontend Routes

```text
/
├── login
├── register
├── forgot-password
└── app
    ├── inbox
    ├── starred
    ├── snoozed
    ├── sent
    ├── drafts
    ├── spam
    ├── trash
    ├── label/:id
    ├── thread/:id
    ├── search
    ├── contacts
    ├── settings
    │   ├── general
    │   ├── accounts
    │   ├── signatures
    │   ├── labels
    │   ├── domains
    │   ├── security
    │   └── notifications
    └── admin
```

---

# 47. React Component Architecture

```text
components/
├── layout/    AppShell, Sidebar, Header, AccountSwitcher, MobileNav
├── mail/      Inbox, EmailRow, EmailList, ThreadView, MessageCard,
│              RecipientList, AttachmentList, EmailToolbar
├── compose/   ComposeWindow, AccountSelector, RecipientInput,
│              RichTextEditor, AttachmentUploader, SendButton
├── search/    SearchBar, SearchFilters, SearchResults
└── settings/
```

---

# 48. Frontend State Strategy

**TanStack Query** for server state: threads, messages, labels, contacts, mail accounts, domains,
notifications, settings, search results.

**Zustand** for UI state: sidebar collapsed, selected messages, compose windows, active account,
theme, mobile navigation.

Avoid putting the entire mailbox into Zustand.

---

# 49. Backend Project Structure

```text
cmd/server/main.go

internal/
├── auth/
├── users/
├── organizations/
├── accounts/
├── providers/
│   ├── mail.go
│   └── resend/
├── domains/
├── addresses/
├── mail/
│   ├── threads/
│   ├── messages/
│   ├── drafts/
│   ├── labels/
│   └── attachments/
├── contacts/
├── search/
├── notifications/
├── websocket/
├── worker/
├── storage/
├── audit/
└── security/

db/
├── migrations/
└── queries/

pkg/
```

---

# 50. Go Worker Jobs

```text
SendEmail            IndexEmail
ProcessInboundEmail  DeleteExpiredEmail
ProcessResendWebhook WakeSnoozedEmail
DownloadAttachment   SendScheduledEmail
GeneratePreview      SendNotification
ProcessBounce        ProcessComplaint
```

Workers should be retryable and idempotent.

---

# 51. Database Overview

```text
users
  ├── mail_accounts
  │      ├── domains ── mail_addresses
  │      ├── threads ── messages ── recipients, attachments
  │      ├── drafts
  │      ├── labels
  │      └── settings
  ├── contacts
  ├── sessions
  └── notifications
```

---

# 52. Suggested Core Tables

```text
users                 threads              contacts            organizations
sessions              messages             contact_groups      organization_members
oauth_accounts        message_recipients   notifications       mail_account_members
password_resets       message_labels       email_events        audit_logs
email_verifications   attachments          filters
mail_accounts         drafts               filter_conditions
domains               labels               filter_actions
mail_addresses        snoozed_messages     signatures
                      scheduled_messages
```

---

# 53. Important Indexes

```sql
CREATE INDEX idx_messages_account            ON messages(mail_account_id);
CREATE INDEX idx_messages_thread             ON messages(thread_id);
CREATE INDEX idx_messages_received_at        ON messages(received_at DESC);
CREATE INDEX idx_threads_account_last_message ON threads(mail_account_id, last_message_at DESC);
CREATE INDEX idx_recipients_email            ON message_recipients(email);
CREATE INDEX idx_mail_addresses_email        ON mail_addresses(email);
CREATE INDEX idx_scheduled_messages_time     ON scheduled_messages(scheduled_at);
CREATE INDEX idx_snoozed_messages_wake       ON snoozed_messages(wake_at);
```

Search indexes should be added after measuring real query patterns.

---

# 54. Email Event Tracking

Provider events: `sent`, `delivered`, `bounced`, `failed`, `opened`, `clicked`, `complained`

Application lifecycle: `queued → sending → sent → delivered`
Failure paths: `sending → failed`, `sent → bounced`, `delivered → complained`

---

# 55. Email UI

```text
┌─────────────────────────────────────────────────────────────┐
│ ☰  Mail       🔍 Search                     🔔 👤           │
├───────────────┬─────────────────────────────────────────────┤
│ Account       │ Inbox                         ⚙            │
│ Switcher      │                                             │
│ + Compose     │ ☑ ⭐ John       Project update      9:42   │
│               │ ☑   GitHub     Security alert       8:21   │
│ Inbox    12   │ ☑   AWS        Invoice              7:13   │
│ Starred       │                                             │
│ Snoozed       │                                             │
│ Sent          │                                             │
│ Drafts    2   │                                             │
│ Spam          │                                             │
│ Trash         │                                             │
│ Labels        │                                             │
└───────────────┴─────────────────────────────────────────────┘
```

---

# 56. Thread UI

```text
┌─────────────────────────────────────────────────────────────┐
│ ←  Project Update                          Archive Delete   │
├─────────────────────────────────────────────────────────────┤
│ 👤 John Smith                         Sep 22, 8:32 PM       │
│ john@example.com                                             │
│ Hey Dark, here's the latest update...                        │
│ 📎 project.pdf                                               │
│                         ↩ Reply    ⋮ More                    │
└─────────────────────────────────────────────────────────────┘
```

---

# 57. Keyboard Shortcuts

```text
c Compose   r Reply     a Reply all   f Forward   e Archive
# Delete    s Star      v Move        l Label     / Search
u Back to inbox         j Next        k Previous
```

Shortcuts should be configurable and disabled while typing in inputs/editors.

---

# 58. PWA / Offline

Later phase: PWA, Service Worker, IndexedDB, offline cached inbox, offline drafts.
Do not prioritize this before the core mail system is stable.

---

# 59. Deployment Architecture

```text
Cloudflare → Nginx → React + Go API → PostgreSQL + Redis → Go Worker
Cloudflare R2 → Attachments
```

---

# 60. Docker Development Environment

```text
postgres   redis   typesense   mail-api   mail-worker   web
```

Typesense can be omitted initially if PostgreSQL FTS is sufficient.

---

# 61. Observability

Structured logging, OpenTelemetry, Prometheus, Grafana, Sentry.

Track: `email_send_latency`, `email_receive_latency`, `queue_depth`, `failed_jobs`,
`webhook_failures`, `database_latency`, `provider_api_errors`, `attachment_upload_latency`,
`search_latency`, `websocket_connections`.

---

# 62. Testing Strategy

Backend: unit, integration, API, webhook, worker, provider tests.

## Critical scenarios

- **Duplicate webhook** — same webhook received twice → only one message created.
- **Worker retry** — provider succeeds, worker crashes, retry → no duplicate email.
- **Account isolation** — Personal message must never appear in Work inbox.
- **Authorization** — User A cannot access User B's messages.

---

# 63. Development Roadmap

See [`PROGRESS.md`](../PROGRESS.md) for the live checklist.

- **Phase 1 — Foundation:** Go project, React project, Docker Compose, PostgreSQL, Redis, Authentication, User model, Session system, Base UI
- **Phase 2 — Multi-Account:** mail_accounts, provider abstraction, Resend provider, encrypted API keys, domain management, mail address management, account switcher
- **Phase 3 — Mailbox MVP:** Inbox, Sent, Drafts, Trash, Spam, Starred, read/unread, archive, delete, bulk actions
- **Phase 4 — Sending:** Compose, TipTap, To/CC/BCC, attachments, SendEmail job, Resend integration, idempotency, retry, status tracking
- **Phase 5 — Receiving:** Resend webhook, signature verification, inbound routing, message parser, thread detection, attachment processing, email event storage
- **Phase 6 — Realtime:** WebSocket, Redis Pub/Sub, new email notification, unread count updates, draft synchronization
- **Phase 7 — Gmail Features:** Labels, search, contacts, snooze, scheduled send, signatures, keyboard shortcuts, filters
- **Phase 8 — Advanced Accounts:** Unified inbox, multiple sender addresses, custom domains, aliases, provider health, account-level settings
- **Phase 9 — Team Platform:** Organizations, team members, shared inbox, assignment, internal notes, roles, permissions, audit logs
- **Phase 10 — AI:** Smart reply, thread summaries, AI search, task extraction, draft assistance, email classification

---

# 64. MVP Scope

```text
AUTH          Register, Login, Logout, Password reset
MULTI ACCOUNT Add Resend account, Remove account, Account switcher, Default account
MAIL          Inbox, Sent, Drafts, Trash, Spam, Starred, Read/unread, Archive, Delete
COMPOSE       From, To, CC, BCC, Subject, Rich text, Attachments, Send
THREADS       Thread detection, Reply, Reply all, Forward
RESEND        Send, Receive, Webhook, Delivery status, Bounce, Attachments
REALTIME      WebSocket
INFRA         PostgreSQL, Redis, R2, Go Worker
```

# 65. V2

Labels, snooze, scheduled sending, contacts, signatures, advanced search, filters,
multiple sender identities, unified inbox, custom domains, keyboard shortcuts.

# 66. V3

Organizations, shared inbox, team permissions, assignment, internal notes, admin dashboard,
analytics, audit logs, billing, provider abstraction expansion, API access.

# 67. V4

AI assistant, smart replies, AI search, thread summaries, action extraction, AI drafting,
automatic classification.

---

# 68. API Structure

```text
/api/v1

/auth       POST /login  /register  /logout  /refresh  /forgot-password  /reset-password
/me         GET /   PATCH /
/mail-accounts  GET /   POST /   GET /:id   PATCH /:id   DELETE /:id
/domains    GET /   POST /   GET /:id   DELETE /:id
/addresses  GET /   POST /   PATCH /:id   DELETE /:id
/threads    GET /   GET /:id   PATCH /:id   DELETE /:id
/messages   GET /:id   POST /   DELETE /:id
/drafts     GET /   POST /   PATCH /:id   DELETE /:id
/labels     GET /   POST /   PATCH /:id   DELETE /:id
/attachments POST /   GET /:id
/search     GET /
/contacts   GET /   POST /   PATCH /:id   DELETE /:id
/scheduled  GET /   POST /   DELETE /:id
/notifications GET /   PATCH /:id/read
/settings   GET /   PATCH /
/webhooks   POST /resend
```

---

# 69. Example Send Request

```json
{
  "mail_account_id": "account_123",
  "from_address_id": "address_456",
  "to": [{ "email": "john@example.com", "name": "John" }],
  "cc": [],
  "bcc": [],
  "subject": "Project Update",
  "html_body": "<p>Hello John...</p>",
  "text_body": "Hello John..."
}
```

---

# 70. Account Isolation

Every account-sensitive table has an ownership path: `messages → mail_accounts → users`.

Authorization verifies: `current_user owns mail_account owns message`.

Do not rely solely on frontend account selection. **The backend must enforce isolation.**

---

# 71. Mail Provider Service Layer

```text
internal/providers/
    mail.go
    resend/
        client.go
        send.go
        domains.go
        webhooks.go
```

```text
MailService → MailProvider → { Resend, SES, SMTP }
```

---

# 72. Important Resend Considerations

**The application owns:** users, mail accounts, addresses, threads, messages, drafts, labels,
contacts, search, notifications, permissions.

**Resend owns:** email transport, delivery, inbound transport, provider events, bounce handling,
provider-side sending infrastructure.

The database remains the source of truth for the user experience.

---

# 73. Final Product Architecture

```text
                           MAIL PLATFORM
                    ┌───────────┴───────────┐
                 USER                    ORGANIZATION
              MAIL ACCOUNTS            SHARED ACCOUNTS
         ┌──────────┼──────────┐            │
       Resend     Resend     Resend        Resend
      Personal     Work      Support      Team
              MAILBOX ENGINE          SHARED INBOX
        ┌───────────┼────────────┐
      Threads     Drafts       Labels
      Search     Contacts     Notifications
                    │
             React Web App
```

---

# 74. Recommended Implementation Philosophy

```text
Architecture → Database model → Authentication → Mail account management
→ Resend provider abstraction → Inbox → Compose → Sending → Inbound webhooks
→ Thread engine → Attachments → Realtime → Search → Labels → Snooze
→ Scheduled send → Filters → Unified inbox → Custom domains → Teams/shared inbox → AI
```

The most important design decisions to lock down before writing large amounts of code:

1. **User → MailAccount relationship**
2. **MailAccount → Provider relationship**
3. **MailAccount → Domain → Address relationship**
4. **MailAccount → Thread → Message relationship**
5. **Provider abstraction**
6. **Inbound email routing**
7. **Credential encryption**
8. **Message/thread ownership**
9. **Idempotent sending and webhook processing**
10. **Background worker architecture**

If these ten pieces are designed correctly, the Gmail-like UI becomes relatively straightforward.
