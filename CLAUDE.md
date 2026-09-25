# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

x-mail is a Gmail-like mail platform: a Go API + worker over PostgreSQL/Redis, a React SPA, and
Resend as the email transport. The full specification lives in `docs/PLAN.md`; the live
implementation checklist lives in `PROGRESS.md`.

**Update `PROGRESS.md` as part of the same change that completes an item.** Only tick a box once
the code exists and has been run/tested — that file is the source of truth for what is actually
built versus merely planned.

## Repository layout

```
apps/api/     Go module: cmd/server (HTTP API), cmd/worker (Asynq), cmd/migrate, internal/*, db/migrations
apps/web/     React + TypeScript + Vite SPA (e2e/ holds the Playwright suite + stub provider)
deploy/       Nginx config and production compose
docs/PLAN.md  Product & architecture specification (authoritative)
PROGRESS.md   Implementation tracker + decisions log
```

## Commands

Run from the repo root (see `Makefile` for the full list):

```bash
make dev            # docker compose up: postgres, redis, api, worker, web
make infra          # postgres + redis only (for running api/web on the host)
make migrate-up     # apply migrations       make migrate-down   # roll back one
make migrate-new name=add_foo                # create a migration pair
make api            # run the API on the host    make worker   # run the worker on the host
make web            # vite dev server
make docker-up      # run the whole stack in containers (web on :8180)
make docker-down    # stop the app containers, leaving postgres/redis up
make test           # go test ./... + web typecheck
make e2e            # playwright browser tests against the real stack
make lint           # go vet + oxlint
```

Single Go test: `cd apps/api && go test ./internal/auth/ -run TestLoginRejectsBadCredentials -v`
Single e2e test: `cd apps/web && npx playwright test -g "signs out"`

`make e2e` starts its own isolated stack — a stub Resend (8099), the API (8091) and Vite (5181),
against the `xmail_e2e` database — so it can run alongside `make dev`. Only `make infra` has to
be running first.

Go integration tests share one database and truncate it, so `make test-api` runs with `-p 1`.
Running `go test ./...` directly makes packages delete each other's rows.

**Local ports** are shifted away from the defaults because this machine already runs other
stacks on 5432/6379/8080/5173: Postgres `5433`, Redis `6380`, API `8090`, worker health `8092`,
web `5180`. The e2e stack uses its own: stub provider `8099`, API `8091`, worker health `8093`,
web `5181`. The container stack (`make docker-up`) serves the app through nginx on `8180`, with
the API on `8190` and worker health on `8192`; it shares the same postgres and redis as host
development, so the two see the same database.

Integration tests that need Postgres read `TEST_DATABASE_URL` and skip when it is unset, so
`make infra` must be running for them to actually execute. A suite that reports `ok` having
skipped everything proves nothing — check with `make test-api-v` when it matters.

## Architecture invariants

These are the decisions the rest of the system depends on. Breaking one of them tends to be
expensive to undo, so treat them as fixed unless the user says otherwise.

**A user is not an email account.** `users → mail_accounts → domains → mail_addresses`. Each
mail account carries its own provider credentials and its own mailbox. Never add a `user_id`
column to mailbox tables — ownership is always resolved through `mail_account_id`.

**MCP tools must never gain the ability to send or destroy.** `internal/mcpserver` exposes x-mail
to AI assistants; it reads mail and writes drafts, and that boundary is the security model, not a
preference. Message bodies are untrusted input that lands in a model's context, so the mitigations
(server instructions, tool descriptions, a warning beside each body) are defence in depth — the
actual guarantee is that no tool sends, deletes, or reaches provider credentials. It authenticates
by API token only and shares the domain services, so token expiry, revocation and per-account
limits apply without this package implementing any of them. Adding a tool means adding a test to
`internal/server/mcp_test.go`.

**There are two references, and both must be updated with the route.** The written guide lives
in `apps/web/src/docs/pages/api-*.tsx` and is what people read; it never mentions Swagger, because
it has to stand on its own. `internal/server/guide_test.go` fails the build when an endpoint has no
`<Endpoint>` entry, when an entry names an endpoint that does not exist, or when an exclusion in
`notInTheGuide` outlives its endpoint. Doc pages are registered in `apps/web/src/docs/registry.ts`.

**The OpenAPI document is hand-written and must be updated with the route.**
`apps/api/internal/apidocs/openapi.yaml` is embedded in the binary and served at `/api/v1/openapi.yaml`,
with Swagger UI at `/api/v1/docs`. Two tests in `internal/server/openapi_test.go` walk the real
router and compare it against the spec in both directions, so adding a route without documenting
it fails the build. Write the *why* as well as the shape — the prose is the part a generator
could never produce, and the reason this is not generated from annotations.

**Every request is authenticated by a session cookie *or* an API token.** `RequireAuth` accepts
both, so a route added behind it works for the browser and for a third party without further
thought. A presented `Authorization: Bearer` is never silently retried as a cookie — an expired
key must be told it expired. Two things stay browser-only, behind `RequireSession`: minting or
revoking tokens, and changing the password. A token that could do either would make its own
expiry a suggestion. Tokens are SHA-256 at rest like sessions, the secret is returned exactly
once, and the four lifetimes (15d/30d/1y/never) are validated server-side.

**A token may be confined to one mail account.** When it is, `auth.AccountScopeArg(ctx)` is bound
into the ownership predicate of every user-scoped query — `($n::uuid IS NULL OR a.id = $n)` — so
the restriction is applied in SQL beside the user check rather than threaded through signatures.
A query that filters by user but not by token scope is a silent leak between mailboxes. Anything
outside the restriction reads as 404.

**Every mailbox query is scoped by mail account, and every mail account is authorized against the
session user.** `messages/threads/drafts/labels → mail_accounts → users`. The frontend's selected
account is a hint, never an authorization decision; resolve and check ownership server-side on
every request. Unified-inbox queries take the *set* of account IDs the user owns.

**Resend is transport, not the database.** The application owns users, accounts, addresses,
threads, messages, drafts, labels, contacts, search, notifications and permissions. Resend owns
delivery, inbound transport and provider events. Never treat a Resend API response as the source
of truth for mailbox state.

**Providers go through the `MailProvider` interface** (`internal/providers`). Resend-specific
types must not leak into the mail domain packages — SES/SMTP are expected later.

**The browser never touches provider credentials.** API keys are encrypted at the application
layer (AES-256-GCM, master key from env, never in Postgres) and only ever decrypted inside the
worker/provider layer. API responses expose `api_key_hint` only.

**Sending is asynchronous and idempotent.** API validates → persists the message + recipients →
enqueues an Asynq job → the worker decrypts credentials and calls the provider. A retried job
must never send a duplicate email: carry a deterministic idempotency key and an application-level
send record.

**Webhook handlers verify the signature, record the event, and return immediately.** Parsing,
threading and attachment work happen in a job. Provider events are deduplicated on
`provider_event_id` — the same webhook delivered twice must create exactly one message.

**Inbound HTML is untrusted.** Sanitize server-side before storage/render, and render in a
sandboxed frame. Never inject raw email HTML into the DOM.

**Threading uses headers first** (`Message-ID`, `In-Reply-To`, `References`), with subject/
participant heuristics only as fallback.

**Provider credentials are verified before they are stored,** and re-checkable afterwards, so a
bad or revoked key surfaces in settings rather than as a silently failed send.

**Attachments never live in Postgres.** Bytes go to the storage interface (local FS in dev,
S3/R2 in prod); the row holds `storage_key`. Authorize on every download.

## Conventions

- Migrations are plain `db/migrations/<version>_<name>.{up,down}.sql`, embedded into the binary
  and applied by `internal/database` under a Postgres advisory lock. The API applies pending
  migrations at startup, so a new container never runs against a stale schema.
- Go: standard library `log/slog` for logging, `pgx` pools (no ORM), SQL in `db/queries`,
  handlers thin — business logic in the `internal/<domain>` service, not in the HTTP layer.
- Errors surface to HTTP through one mapping layer; handlers do not write ad-hoc status codes.
- Frontend: TanStack Query owns server state, Zustand owns UI state only (selection, compose
  windows, active account, theme). Do not mirror the mailbox into Zustand.
- Realtime events invalidate TanStack Query keys; they do not patch cached lists by hand.
- shadcn/ui components are added via the shadcn MCP server / CLI rather than hand-copied.
- `RESEND_BASE_URL` and `RATE_LIMIT_SCALE` exist only so tests can run a stub provider and skip
  signup throttling; `config.Load` refuses both in production. Do not reach for them elsewhere.
- Docker: one API image carries `server`, `worker` and `migrate` — they share every package, so
  building them separately would compile the same code twice. The image runs as a non-root user
  and ships no `.env`; configuration comes from the environment. The web image serves
  `deploy/nginx.conf` itself rather than a copy, which is why it builds from the repository root.
- Health endpoints answer HEAD as well as GET: probes commonly use HEAD, and chi — unlike
  `net/http`'s ServeMux — does not imply it from a GET route.
