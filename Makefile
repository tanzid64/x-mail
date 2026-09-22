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
test-api:
	cd $(API_DIR) && go test ./...

## test-web: frontend tests
test-web:
	@test -d $(WEB_DIR)/node_modules && cd $(WEB_DIR) && npm test --if-present || echo "web not scaffolded yet, skipping"

## lint: vet + typecheck
lint:
	cd $(API_DIR) && go vet ./...
	@test -d $(WEB_DIR)/node_modules && cd $(WEB_DIR) && npm run lint --if-present || true

## build: compile server + worker binaries
build:
	cd $(API_DIR) && go build -o bin/server ./cmd/server && go build -o bin/worker ./cmd/worker

.PHONY: help infra infra-down infra-reset dev docker-up api worker web \
        migrate-up migrate-down migrate-status migrate-new psql \
        test test-api test-web lint build
