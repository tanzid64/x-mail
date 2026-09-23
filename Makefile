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
	@for db in xmail_test xmail_e2e; do \
		docker compose exec -T postgres psql -U xmail -d postgres -tAc \
			"SELECT 1 FROM pg_database WHERE datname='$$db'" | grep -q 1 \
			|| docker compose exec -T postgres createdb -U xmail $$db; \
	done
	@echo "test databases ready (xmail_test, xmail_e2e)"

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

## docker-build: build the api and web images
docker-build:
	docker compose --profile app build

## docker-up: run the whole stack in containers (web on :8180)
# Uses the same postgres and redis as `make dev`, so the two share a database.
docker-up:
	docker compose --profile app up -d --build
	@echo "web  http://localhost:8180"
	@echo "api  http://localhost:8190"

## docker-down: stop the application containers, leaving infra running
docker-down:
	docker compose --profile app stop api worker web
	docker compose --profile app rm -f api worker web

## docker-logs: follow the application container logs
docker-logs:
	docker compose --profile app logs -f api worker web

## docker-ps: show container status
docker-ps:
	docker compose ps

## docs: open the API reference (needs the API running on :8090, or :8180 in containers)
docs:
	@url=http://localhost:8090/api/v1/docs; \
	curl -fsS -o /dev/null "$$url" 2>/dev/null || url=http://localhost:8180/api/v1/docs; \
	echo "API reference: $$url"; \
	(xdg-open "$$url" >/dev/null 2>&1 &) || true

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

.PHONY: help infra infra-down infra-reset dev api worker web \
        docker-build docker-up docker-down docker-logs docker-ps \
        migrate-up migrate-down migrate-status migrate-new psql \
        test test-api test-api-v test-web e2e lint build docs

## test-api-v: verbose backend tests
test-api-v:
	cd $(API_DIR) && go test -p 1 ./... -v -count=1
