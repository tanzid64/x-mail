# x-mail — Implementation Progress

Live tracker for the plan in [`docs/PLAN.md`](docs/PLAN.md).
Legend: `[ ]` not started · `[~]` in progress · `[x]` done and verified

**Current phase:** Phase 7 — Gmail Features (Phases 1–6 complete; MVP scope done; API access shipped)

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

- [x] `drafts` table + draft CRUD API
- [x] Draft autosave (debounced)
- [x] Attachment storage abstraction (local + S3/R2)
- [x] Attachment upload/download API
- [x] Asynq queue + worker binary
- [x] `SendEmail` job (idempotent)
- [x] Resend send integration + status tracking
- [x] Retry/backoff + failure recording
- [x] Compose UI + TipTap editor
- [x] Reply / reply-all / forward

## Phase 5 — Receiving

- [x] `POST /api/v1/webhooks/resend` + signature verification (Svix)
- [x] `email_events` table + idempotency
- [x] `ProcessInboundEmail` job
- [x] Inbound routing (address → account)
- [x] MIME/body parsing + sanitization
- [x] Thread detection (Message-ID / In-Reply-To / References)
- [x] Inbound attachment processing
- [x] Delivery/bounce/complaint event handling

## Phase 6 — Realtime

- [x] WebSocket endpoint `/ws` with session auth
- [x] Redis Pub/Sub fan-out
- [x] Event publishing from API + worker
- [x] Frontend WS client → query invalidation
- [x] Unread count updates

## Phase 7 — Gmail Features

- [~] Custom labels CRUD + UI — API done, label management screens pending
- [ ] PostgreSQL FTS search + query operators
- [ ] Contacts (auto-build + CRUD + autocomplete)
- [ ] Snooze + wake worker
- [ ] Scheduled send + worker
- [ ] Signatures
- [ ] Keyboard shortcuts
- [ ] Filters / rules engine

## Phase 7.5 — API access (third-party)

- [x] `api_tokens` table + 15d / 30d / 1y / never expiry
- [x] Token generation (`xm_…`), SHA-256 at rest, hint-only in responses
- [x] `RequireAuth` accepts a session cookie **or** `Authorization: Bearer`
- [x] Token management API + Settings → API tokens screen
- [x] Per-account restriction (optional, defaults to every account)
- [x] Token cannot mint tokens or change the password
- [x] `last_used_at` tracking + immediate revocation
- [x] OpenAPI 3.1 document + Swagger UI at `/api/v1/docs`
- [x] Tests that fail when the spec and the router drift apart

## Phase 7.6 — Public site & documentation

- [x] Public home page at `/` (redirects a signed-in visitor to their inbox)
- [x] Documentation site at `/docs`, readable signed out
- [x] Installation + configuration guide
- [x] Interface guide — accounts, mailbox, compose, receiving
- [x] Written API reference covering all 51 integrator-facing endpoints
- [x] Tests pinning the written guide to the route table

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
- [x] Webhook idempotency tests
- [x] Worker retry/idempotency tests
- [x] HTML sanitization of inbound mail
- [x] Action feedback (success/error reporting on every mutation)
- [ ] Observability (metrics, tracing)
- [ ] Abuse prevention / sending limits
- [x] Nginx + Docker images (api + web, `make docker-up`)

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
| 26 | Drafts live in their own table, not as messages | Autosave writes many times a minute; keeping drafts out of the mailbox tables stops that churning thread aggregates, and a half-typed row can never be reached by the sending path |
| 27 | Duplicate sends blocked at three layers | Claiming a draft is conditional on it still being a draft; the Asynq task id is the message id; the provider idempotency key is generated once and reused. The third is the only one that covers a worker dying after the provider accepted the message |
| 28 | Send failures acted on by kind | Throttling and outages are retried; a rejected key or unverified domain is recorded as permanently failed with a reason, since retrying those only delays telling the user |
| 29 | Attachments re-pointed from draft to message, never copied | The bytes are already in object storage; copying them would double storage for every send |
| 30 | Attachment downloads are always octet-stream with nosniff | An HTML or SVG attachment rendered inline would execute as a page on our origin |
| 31 | Worker exposes /healthz and /readyz | A worker that cannot be probed cannot be orchestrated; it also gives the e2e stack a readiness signal |
| 32 | Webhook endpoint is per-account, with a per-account signing secret | One secret to verify against rather than every secret in the system; falls back to a deployment-wide secret for single-tenant setups |
| 33 | Webhook idempotency enforced by a unique constraint, not check-then-insert | Providers redeliver by design, and two deliveries can race a check |
| 34 | Inbound HTML sanitised once on arrival into its own column | Cost paid per message rather than per view, and the untouched original means a sanitiser fix can be re-applied instead of losing the message |
| 35 | `InsertMessage` sanitises, so no caller has to remember | The seeder forgot, and inbound mail rendered as plain text; one guarantee beats N call sites |
| 36 | Delivery status only moves forward | Providers reorder events; a late "sent" must not undo a recorded "delivered" |
| 37 | Realtime events carry ids, never content | The client refetches through its normal authorized endpoints, so a bug in the fan-out cannot leak mail |
| 38 | Redis pub/sub between API and worker, one pattern subscription per process | The worker stores the mail but the API holds the socket; this also works unchanged across several API instances |
| 39 | A slow websocket client is dropped, not waited on | Blocking on one stalled browser would hold up the fan-out for everyone |
| 40 | Websocket origins configured explicitly, not inferred from Host | Any proxy that rewrites Host breaks the default Origin-vs-Host check — which is exactly what happened behind the dev proxy |
| 41 | One API image carries server, worker and migrate | They share every package; separate images would compile the same code twice and could drift apart |
| 42 | Images run as a non-root user and contain no `.env` | Nothing needs privilege, and baking a `.env` would ship the encryption master key inside the image |
| 43 | Web image builds from the repo root to serve `deploy/nginx.conf` | A copy inside `apps/web` would drift from the config production actually uses |
| 44 | Health endpoints answer HEAD as well as GET | Container probes commonly use HEAD; chi does not imply it from a GET route, which made the API container report unhealthy while being perfectly fine |
| 45 | Mutation failures are reported by one `MutationCache.onError`, not per call site | A new mutation is then reported by default; forgetting to opt in is a duplicate message, never a silent failure. Screens that render the error next to the offending field opt out with `meta: SILENT_ERROR` |
| 46 | Every action that changes something says so, including when nothing changed | "Test key" and "Sync from provider" spun and then said nothing, which reads exactly like a dead button. A recheck that leaves a domain pending has to report that too |
| 47 | A key test reports from the returned account, not the HTTP status | `CheckCredentials` records a rejected key as account status and still answers 200, so the verdict is in the body |
| 48 | `SyncDomains` takes the complete remote list; one domain goes through `UpsertDomain` | Sync reconciles by deleting whatever it was not given. `RefreshDomain` passed it a single domain, so "Recheck DNS" deleted every other domain on the account and its addresses. Covered by `TestRefreshingOneDomainLeavesTheOthersAlone` |
| 49 | API tokens live in `internal/auth`, not their own package | They authenticate a request and resolve to `*auth.User`, which `RequireAuth` needs; a separate package would have meant an import cycle for no gain |
| 50 | One `RequireAuth` accepts either a cookie or a Bearer token | "Every API works through login or key" is then a property of the middleware rather than a checklist applied route by route |
| 51 | A presented Bearer token is never silently retried as a cookie | It is the caller's stated credential, so an expired key is told it expired rather than being answered "you must be signed in" |
| 52 | Exactly four lifetimes, validated server-side | An arbitrary duration invites tokens that outlive the integration they were made for without anyone choosing that. Calendar arithmetic, so "30 days" is the same date next month across a DST change |
| 53 | Tokens are SHA-256 at rest, like sessions; the secret is returned once | Full-entropy random values have nothing to brute force, and a lost token is replaced rather than recovered |
| 54 | Revocation is a soft `revoked_at`, not a delete | The digest can never be reissued, the row is what an audit log will want, and it lets a revoked key be told it was revoked rather than that it never existed |
| 55 | A token cannot mint a token or change the password | A 15-day key issuing itself a permanent one would make every expiry a suggestion; a key that could change the password could lock the owner out of revoking it |
| 56 | The per-account restriction is applied in SQL, read from the request context | Same reasoning as ownership (#16): threading it through every signature means a query can filter by user and forget the account, which is a silent leak between mailboxes |
| 57 | A restricted token gets 404 for another account, not 403 | Consistent with #15 — a refusal would confirm the account exists |
| 58 | The OpenAPI document is hand-written, not generated from annotations | Generators want annotations on every handler and still describe shapes badly; the value here is the prose about *why* an endpoint behaves as it does, which no generator writes |
| 59 | Two tests compare the spec against a `chi.Walk` of the real router, both ways | This is the only thing that keeps a hand-written spec honest. Verified to fail on a removed path and on a documented route that does not exist |
| 60 | The spec is embedded in the binary and served at `/api/v1/openapi.yaml` | A deployed build then serves the description of *itself* rather than of whatever was last generated and committed |
| 61 | Swagger UI loads from a CDN rather than vendoring ~3 MB of assets | The only part of the service needing the public internet; a locked-down deployment can still consume the raw spec |
| 62 | Docs are public, outside the auth middleware | An API meant for third parties that hides its own description is not much use to them, and the document contains no secrets |
| 63 | `/` is a public home page; a signed-in visitor is redirected to their inbox | There was nowhere to send someone who has not decided to sign up, but someone who bookmarked the root wants their mail, not the pitch |
| 64 | Documentation pages are a registry, not a route each | The sidebar, the previous/next links and the URL all read one list, so a page cannot be in the navigation without existing, or exist without appearing |
| 65 | Doc pages are components over prose primitives, not Markdown | A code block gets a copy button and a callout a tone, without adding a Markdown pipeline and a syntax highlighter to a bundle that is otherwise a mail client |
| 66 | The written guide never mentions Swagger | Swagger is for poking at endpoints; the guide has to stand on its own for a reader who never opens it. The two Swagger routes are explicitly excluded from the coverage test, with a reason |
| 67 | Three Go tests pin the guide to the OpenAPI document, which is pinned to the router | Undocumented endpoint, imaginary endpoint, and stale exclusion all fail the build. A stale exemption is its own drift — it means the next person to delete an endpoint gets no warning |
