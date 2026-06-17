# Sigap Makefile — cross-language dev orchestration (KISS, no heavy runner)
SHELL := /bin/bash

.PHONY: help dev dev-down db-migrate db-seed dev-api dev-engine dev-web build test clean

help:
	@echo "Sigap — available targets:"
	@echo "  make dev          # bring up the full stack via docker compose (reads .env)"
	@echo "  make dev-down     # stop the docker compose stack"
	@echo "  make db-migrate   # apply 0001_init.sql"
	@echo "  make db-seed      # load realistic sample facilities"
	@echo "  make dev-api      # Go HTTP gateway (:8080)"
	@echo "  make dev-engine   # Rust gRPC engine (:50051)"
	@echo "  make dev-web      # SvelteKit (:5173)"
	@echo "  make build        # build all"
	@echo "  make test         # run Go + Rust tests"
	@echo "  make clean        # remove build artifacts"

# Boots the whole stack. docker compose auto-loads .env from the repo root,
# which must define POSTGRES_PASSWORD (compose fails fast otherwise).
dev:
	@test -f .env || { echo "ERROR: .env not found. Run: cp .env.example .env && edit POSTGRES_PASSWORD"; exit 1; }
	docker compose up -d --build

dev-down:
	docker compose down

db-migrate:
	psql "$$DATABASE_URL" -f packages/db/migrations/0001_init.sql

db-seed:
	psql "$$DATABASE_URL" -f packages/db/seed/dev.sql

dev-api:
	cd apps/api && go run ./cmd/server

dev-engine:
	cd apps/queue-engine && cargo run

dev-web:
	pnpm --filter sigap-web dev

build:
	cd apps/api && go build -o bin/sigap-api ./cmd/server
	cd apps/queue-engine && cargo build --release
	pnpm --filter sigap-web build

test:
	cd apps/api && go test ./...
	cd apps/queue-engine && cargo test

clean:
	rm -rf apps/api/bin apps/queue-engine/target apps/web/.svelte-kit apps/web/build
