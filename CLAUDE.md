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
apps/web/     React + TypeScript + Vite SPA
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
make test           # go test ./... + vitest
make lint           # go vet + golangci-lint + eslint + tsc
```

Single Go test: `cd apps/api && go test ./internal/auth/ -run TestArgon2Verify -v`
Single web test: `cd apps/web && npx vitest run src/lib/api.test.ts -t "refreshes session"`

**Local ports** are shifted away from the defaults because this machine already runs other
stacks on 5432/6379/8080/5173: Postgres `5433`, Redis `6380`, API `8090`, web `5180`.

Integration tests that need Postgres read `TEST_DATABASE_URL` and skip when it is unset, so
`make infra` must be running for them to actually execute.

## Architecture invariants

These are the decisions the rest of the system depends on. Breaking one of them tends to be
expensive to undo, so treat them as fixed unless the user says otherwise.

**A user is not an email account.** `users → mail_accounts → domains → mail_addresses`. Each
mail account carries its own provider credentials and its own mailbox. Never add a `user_id`
column to mailbox tables — ownership is always resolved through `mail_account_id`.

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
