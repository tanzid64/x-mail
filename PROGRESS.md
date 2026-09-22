# x-mail — Implementation Progress

Live tracker for the plan in [`docs/PLAN.md`](docs/PLAN.md).
Legend: `[ ]` not started · `[~]` in progress · `[x]` done and verified

**Current phase:** Phase 1 — Foundation (backend auth done; frontend next)

---

## Phase 0 — Repo & Tooling

- [x] Git repository initialized
- [x] `docs/PLAN.md` — spec captured
- [x] `PROGRESS.md` — tracker
- [x] `CLAUDE.md` — agent/project conventions
- [x] Monorepo layout (`apps/api`, `apps/web`, `deploy/`)
- [x] `docker-compose.yml` (postgres, redis, api, worker, web)
- [x] `.env.example` + config loading
- [x] Makefile / task runner

## Phase 1 — Foundation

- [x] Go module + chi router + graceful shutdown
- [x] Structured logging (slog) + request ID + recovery middleware
- [x] PostgreSQL connection pool (pgx)
- [x] Migration tooling + `db/migrations`
- [x] Redis connection
- [x] `users` + `sessions` + `password_resets` + `email_verifications` tables
- [x] Argon2id password hashing
- [x] Register / Login / Logout
- [x] Session middleware (httpOnly cookie)
- [x] `GET /api/v1/me`, `PATCH /api/v1/me`
- [x] Forgot password / reset password
- [x] Email verification
- [x] Rate limiting (IP + endpoint)
- [ ] React + Vite + TS + Tailwind + shadcn/ui
- [ ] TanStack Router + TanStack Query + Zustand
- [ ] API client + auth guard
- [ ] Login / Register / Forgot-password screens
- [ ] Base app shell (sidebar + header)

## Phase 2 — Multi-Account

- [ ] `mail_accounts`, `domains`, `mail_addresses` tables
- [ ] Credential encryption (AES-256-GCM, master key from env)
- [ ] `MailProvider` interface
- [ ] Resend provider implementation
- [ ] Mail account CRUD API
- [ ] Domain sync/management API
- [ ] Mail address CRUD API
- [ ] Ownership/authorization enforcement
- [ ] Account switcher UI
- [ ] Settings → Accounts screens

## Phase 3 — Mailbox MVP

- [ ] `threads`, `messages`, `message_recipients`, `labels`, `message_labels` tables
- [ ] System label bootstrap per mail account
- [ ] Thread list API (pagination + folder filter)
- [ ] Thread detail API
- [ ] Read/unread, star, important
- [ ] Archive / delete / spam / trash
- [ ] Bulk actions
- [ ] Inbox UI (list + rows + toolbar)
- [ ] Thread view UI
- [ ] Folder routes (inbox/starred/sent/drafts/spam/trash)

## Phase 4 — Sending

- [ ] `drafts` table + draft CRUD API
- [ ] Draft autosave (debounced)
- [ ] Attachment storage abstraction (local + S3/R2)
- [ ] Attachment upload/download API
- [ ] Asynq queue + worker binary
- [ ] `SendEmail` job (idempotent)
- [ ] Resend send integration + status tracking
- [ ] Retry/backoff + failure recording
- [ ] Compose UI + TipTap editor
- [ ] Reply / reply-all / forward

## Phase 5 — Receiving

- [ ] `POST /api/v1/webhooks/resend` + signature verification (Svix)
- [ ] `email_events` table + idempotency
- [ ] `ProcessInboundEmail` job
- [ ] Inbound routing (address → account)
- [ ] MIME/body parsing + sanitization
- [ ] Thread detection (Message-ID / In-Reply-To / References)
- [ ] Inbound attachment processing
- [ ] Delivery/bounce/complaint event handling

## Phase 6 — Realtime

- [ ] WebSocket endpoint `/ws` with session auth
- [ ] Redis Pub/Sub fan-out
- [ ] Event publishing from API + worker
- [ ] Frontend WS client → query invalidation
- [ ] Unread count updates

## Phase 7 — Gmail Features

- [ ] Custom labels CRUD + UI
- [ ] PostgreSQL FTS search + query operators
- [ ] Contacts (auto-build + CRUD + autocomplete)
- [ ] Snooze + wake worker
- [ ] Scheduled send + worker
- [ ] Signatures
- [ ] Keyboard shortcuts
- [ ] Filters / rules engine

## Phase 8 — Advanced Accounts

- [ ] Unified inbox (all accounts)
- [ ] Multiple sender identities
- [ ] Custom domain verification flow
- [ ] Aliases
- [ ] Provider health checks
- [ ] Account-level settings + sending limits

## Phase 9 — Team Platform

- [ ] Organizations + members
- [ ] Shared mail accounts + roles
- [ ] Shared inbox (assignment, internal notes, status)
- [ ] Permissions
- [ ] Audit logs
- [ ] Admin dashboard

## Phase 10 — AI

- [ ] Smart reply
- [ ] Thread summaries
- [ ] AI search → structured filters
- [ ] Task extraction
- [ ] Draft assistance
- [ ] Classification

---

## Cross-cutting (tracked continuously)

- [ ] Account isolation tests
- [~] Authorization tests — auth/session coverage in place; account isolation lands with Phase 2
- [ ] Webhook idempotency tests
- [ ] Worker retry/idempotency tests
- [ ] HTML sanitization of inbound mail
- [ ] Observability (metrics, tracing)
- [ ] Abuse prevention / sending limits
- [ ] Nginx + production Docker setup

---

## Decisions log

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Monorepo: `apps/api` (Go), `apps/web` (React) | Matches plan §60 compose services |
| 2 | Attachment storage: local FS in dev, S3/R2 in prod, behind one interface | Plan §1 allows local; keeps dev dependency-free |
| 3 | Session auth via httpOnly cookie, not JWT | Plan §34 "secure session mechanism"; simplest safe default for WS auth too |
| 4 | Hand-rolled embedded migrator instead of goose | The goose CLI pulls every DB driver (ClickHouse, Azure, …) into go.mod; we need ~120 lines and Postgres only |
| 5 | Ports shifted: pg 5433, redis 6380, api 8090, web 5180 | 5432/6379/8080/5173 are already taken by other stacks on this machine |
| 6 | API applies pending migrations at startup | Keeps a deployed container in sync with its code; advisory lock makes api+worker startup safe |
| 7 | Session tokens stored as SHA-256, passwords as Argon2id | Tokens are full-entropy random values with nothing to brute force; paying a slow-hash cost per authenticated request would be waste |
| 8 | Password reset revokes all sessions; password change keeps only the current one | A reset may be driven by an attacker who already holds a session; a voluntary change should not log you out of the tab you are in |
| 9 | Integration tests skip without `TEST_DATABASE_URL` | `go test ./...` stays green with no infra, while `make infra` turns the same tests into real end-to-end coverage |
