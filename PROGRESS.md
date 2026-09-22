# x-mail — Implementation Progress

Live tracker for the plan in [`docs/PLAN.md`](docs/PLAN.md).
Legend: `[ ]` not started · `[~]` in progress · `[x]` done and verified

**Current phase:** Phase 4 — Sending (Phases 1–3 complete)

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
- [x] React + Vite + TS + Tailwind + shadcn/ui
- [x] TanStack Router + TanStack Query
- [x] Zustand — holds the active account selection
- [x] API client + auth guard
- [x] Login / Register / Forgot-password screens
- [x] Base app shell (sidebar + header)

## Phase 2 — Multi-Account

- [x] `mail_accounts`, `domains`, `mail_addresses` tables
- [x] Credential encryption (AES-256-GCM, master key from env)
- [x] `MailProvider` interface
- [x] Resend provider implementation
- [x] Mail account CRUD API
- [x] Domain sync/management API
- [x] Mail address CRUD API
- [x] Ownership/authorization enforcement
- [x] Account switcher UI
- [x] Settings → Accounts screens

## Phase 3 — Mailbox MVP

- [x] `threads`, `messages`, `message_recipients`, `labels`, `message_labels` tables
- [x] System label bootstrap per mail account
- [x] Thread list API (pagination + folder filter)
- [x] Thread detail API
- [x] Read/unread, star, important
- [x] Archive / delete / spam / trash
- [x] Bulk actions
- [x] Inbox UI (list + rows + toolbar)
- [x] Thread view UI
- [x] Folder routes (inbox/starred/sent/drafts/spam/trash)

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

- [~] Custom labels CRUD + UI — API done, label management screens pending
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

- [x] Account isolation tests
- [x] Authorization tests
- [ ] Webhook idempotency tests
- [ ] Worker retry/idempotency tests
- [~] HTML sanitization of inbound mail — rendered in a sandboxed frame; server-side sanitising lands with inbound mail
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
| 10 | Vite proxies `/api` and `/ws` to the Go API in dev | Keeps the session cookie same-origin, so dev matches production behind Nginx — no CORS preflights, no SameSite surprises |
| 11 | Code-based TanStack Router, not file-based | No codegen step to keep in sync; the route tree stays explicit and fully typed |
| 12 | Folders are one `/$folder` route, not eight | Inbox/Sent/Trash differ only by which label is queried; `INBOX` in `router.tsx` is the typed "go to inbox" target |
| 13 | Zustand deferred until there is UI state worth holding | Phase 1 had none — TanStack Query owns the session. It now holds the active-account selection, persisted so reopening returns to the same mailbox |
| 14 | Mail addresses are globally unique | Inbound routing resolves a recipient by address alone (plan §22); two accounts claiming one address would make routing ambiguous |
| 15 | Anything another user owns returns 404, never 403 | A 403 confirms the record exists, which is itself a disclosure |
| 16 | Ownership predicates live in the SQL, not in a separate check | A handler cannot forget to authorize if the query itself cannot match another user's row |
| 17 | Credentials verified against the provider before being stored | A bad key is reported at the form rather than at the user's first send, days later |
| 18 | `RATE_LIMIT_SCALE` and `RESEND_BASE_URL` env overrides, both rejected in production | Lets the e2e suite run a stub provider and register many users from one IP, without weakening the deployed system |
| 19 | e2e runs its own stack: ports 8099/8091/5181 and the xmail_e2e database | Can run alongside `make dev` without either disturbing the other, and never touches the Go tests' database |
| 20 | Placement uses system labels; read/starred/important stay columns | A message can sit in a folder and carry user labels at once; the three flags are per-message booleans that would gain nothing from the label machinery |
| 21 | `refresh_thread()` in SQL is the only definition of a thread summary | Called in the same transaction as the change that made it stale, so two code paths cannot compute counters differently |
| 22 | Thread lists paginate by cursor, not offset | A mailbox changes under the reader; an offset silently skips or repeats rows as mail arrives |
| 23 | Email HTML renders in an iframe with `allow-same-origin allow-popups` and no `allow-scripts` | Scripts off is what neutralises the content; same-origin is only so the frame can be measured and sized. Granting it *with* scripts would defeat the sandbox, which is why scripts stay off |
| 24 | `cmd/seed` fills a mailbox with sample conversations | Inbound mail does not arrive until Phase 5; without it the mailbox UI could not be exercised or tested at all |
| 25 | e2e truncates its database in globalSetup | Globally-unique rows (mail addresses) collided with leftovers, so a suite that passed once began failing for reasons unrelated to the change |
