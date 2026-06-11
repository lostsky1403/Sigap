# Sigap Makefile — cross-language dev orchestration (KISS, no heavy runner)
SHELL := /bin/bash

.PHONY: help dev db-migrate db-seed dev-api dev-engine dev-web build test clean

help:
	@echo "Sigap — available targets:"
	@echo "  make dev          # all services (requires DB + 3 terminals recommended)"
	@echo "  make db-migrate   # apply 0001_init.sql"
	@echo "  make db-seed      # load realistic sample facilities"
	@echo "  make dev-api      # Go HTTP gateway (:8080)"
	@echo "  make dev-engine   # Rust gRPC engine (:50051)"
	@echo "  make dev-web      # SvelteKit (:5173)"
	@echo "  make build        # build all"
	@echo "  make test         # run Go + Rust tests"
	@echo "  make clean        # remove build artifacts"

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
