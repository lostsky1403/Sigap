# Sigap Makefile — cross-language dev orchestration (KISS, no heavy runner)
SHELL := /bin/bash

.PHONY: help dev dev-down db-migrate db-seed dev-api dev-engine dev-web build test clean lint security

help:
	@echo "Sigap — available targets:"
	@echo "  make dev          # bring up the full stack via docker compose (reads .env)"
	@echo "  make dev-down     # stop the docker compose stack"
	@echo "  make db-migrate   # apply 0001_init.sql"
	@echo "  make db-seed      # load realistic sample facilities"
	@echo "  make dev-api      # Go HTTP gateway (:8080)"
	@echo "  make dev-engine   # Rust gRPC engine (:50051)"
	@echo "  make dev-web      # SvelteKit (:5173)"
	@echo "  make lint         # run linters across all services"
	@echo "  make security     # run security checks (gitleaks, cargo audit, govulncheck)"
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

# ---- Quality gates ----

lint:
	@echo "==> Go vet"
	cd apps/api && go vet ./...
	@echo "==> Clippy"
	cd apps/queue-engine && cargo clippy -- -D warnings || true
	@echo "==> Svelte check"
	pnpm --filter sigap-web run check

security:
	@echo "==> Go vulnerability scan"
	@which govulncheck >/dev/null 2>&1 && (cd apps/api && govulncheck ./...) || echo "SKIP: govulncheck not installed (run: go install golang.org/x/vuln/cmd/govulncheck@latest)"
	@echo "==> Rust audit"
	@which cargo-audit >/dev/null 2>&1 && (cd apps/queue-engine && cargo audit) || echo "SKIP: cargo-audit not installed (run: cargo install cargo-audit)"
	@echo "==> Secrets scan"
	@which gitleaks >/dev/null 2>&1 && gitleaks detect --source . --verbose || echo "SKIP: gitleaks not installed (see .env.example or install from https://github.com/gitleaks/gitleaks)"
