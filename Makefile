SHELL := /bin/bash
API_DIR := apps/api
WEB_DIR := apps/web

# Load .env so host-run commands (api, worker, migrations) see the same config
# the containers get via env_file.
ifneq (,$(wildcard .env))
include .env
export
endif

.DEFAULT_GOAL := help

## help: list targets
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

## infra: start postgres + redis only
infra:
	docker compose up -d postgres redis
	@docker compose exec -T postgres sh -c 'until pg_isready -U xmail -d xmail -q; do sleep 0.5; done' && echo "postgres ready"
	@docker compose exec -T postgres psql -U xmail -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='xmail_test'" \
		| grep -q 1 || docker compose exec -T postgres createdb -U xmail xmail_test
	@echo "test database ready"

## infra-down: stop infra (keeps volumes)
infra-down:
	docker compose down

## infra-reset: stop infra and DELETE all data volumes
infra-reset:
	docker compose down -v

## dev: start infra, then run api + worker + web together (ctrl-c stops all)
dev: infra
	@trap 'kill 0' EXIT INT TERM; \
	$(MAKE) --no-print-directory api & \
	$(MAKE) --no-print-directory worker & \
	$(MAKE) --no-print-directory web & \
	wait

## docker-up: run the whole stack in containers instead
docker-up:
	docker compose --profile app up --build

## api: run the HTTP API on the host
api:
	cd $(API_DIR) && go run ./cmd/server

## worker: run the background worker on the host
worker:
	cd $(API_DIR) && go run ./cmd/worker

## web: run the vite dev server
web:
	cd $(WEB_DIR) && npm run dev

## migrate-up: apply all pending migrations
migrate-up:
	cd $(API_DIR) && go run ./cmd/migrate up

## migrate-down: roll back the most recent migration
migrate-down:
	cd $(API_DIR) && go run ./cmd/migrate down

## migrate-status: show migration state
migrate-status:
	cd $(API_DIR) && go run ./cmd/migrate status

## migrate-new: create a migration (make migrate-new name=add_foo)
migrate-new:
	@test -n "$(name)" || { echo "usage: make migrate-new name=add_foo"; exit 1; }
	cd $(API_DIR) && go run ./cmd/migrate create $(name)

## psql: open a shell on the dev database
psql:
	docker compose exec postgres psql -U xmail -d xmail

## test: run backend and frontend tests
test: test-api test-web

## test-api: go tests (integration tests need `make infra`)
# -p 1 runs one package at a time: the integration tests share a single test
# database and truncate it between tests, so parallel packages would delete
# each other's rows and fail in ways that look like real bugs.
test-api:
	cd $(API_DIR) && go test -p 1 ./...

## test-web: frontend typecheck (unit tests land with the mailbox UI)
test-web:
	cd $(WEB_DIR) && npm run typecheck

## e2e: browser tests against the real stack (needs `make infra`)
e2e:
	cd $(WEB_DIR) && npm run e2e

## lint: vet + oxlint
lint:
	cd $(API_DIR) && go vet ./...
	cd $(WEB_DIR) && npm run lint

## build: compile server + worker binaries
build:
	cd $(API_DIR) && go build -o bin/server ./cmd/server && go build -o bin/worker ./cmd/worker

.PHONY: help infra infra-down infra-reset dev docker-up api worker web \
        migrate-up migrate-down migrate-status migrate-new psql \
        test test-api test-api-v test-web e2e lint build

## test-api-v: verbose backend tests
test-api-v:
	cd $(API_DIR) && go test -p 1 ./... -v -count=1
