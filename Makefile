# Sigap Makefile — cross-language dev orchestration (KISS, no heavy runner)
SHELL := /bin/bash

.PHONY: help dev dev-down db-migrate db-migrate-all db-seed bootstrap dev-api dev-engine dev-web dev-notification-worker build test clean lint security

help:
	@echo "Sigap — available targets:"
	@echo "  make dev          # bring up the full stack via docker compose (reads .env)"
	@echo "  make dev-down     # stop the docker compose stack"
	@echo "  make db-migrate   # apply 0001_init.sql (legacy)"
	@echo "  make db-migrate-all # apply all pending migrations (tracked)"
	@echo "  make db-seed      # load realistic sample facilities"
	@echo "  make bootstrap    # create bootstrap admin (requires SIGAP_BOOTSTRAP_ADMIN=true)"
	@echo "  make dev-api      # Go HTTP gateway (:8080)"
	@echo "  make dev-engine   # Rust gRPC engine (:50051)"
	@echo "  make dev-web      # SvelteKit (:5173)"
	@echo "  make dev-notification-worker  # notification outbox worker (manual; no docker-compose service)"
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

db-migrate-all:
	@echo "==> Running tracked migrations"
	cd apps/api && SIGAP_AUTO_MIGRATE=true go run ./cmd/server

db-seed:
	psql "$$DATABASE_URL" -f packages/db/seed/dev.sql

bootstrap:
	@echo "==> Creating bootstrap admin (requires migrations, seed, and .env)"
	@test -f .env || { echo "WARN: .env not found. Some env vars may be missing."; }
	cd apps/api && SIGAP_BOOTSTRAP_ADMIN=true go run ./cmd/bootstrap

dev-api:
	cd apps/api && go run ./cmd/server

dev-engine:
	cd apps/queue-engine && cargo run

dev-web:
	pnpm --filter sigap-web dev

dev-notification-worker:
	cd apps/api && go run ./cmd/notification-worker

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
	@if which govulncheck >/dev/null 2>&1; then \
		if cd apps/api && govulncheck ./...; then \
			echo "PASS: govulncheck"; \
		else \
			echo "FAIL: govulncheck found issues"; \
			exit 1; \
		fi; \
	else \
		echo "SKIP: govulncheck not installed (run: go install golang.org/x/vuln/cmd/govulncheck@latest)"; \
	fi
	@echo "==> Rust audit"
	@if which cargo-audit >/dev/null 2>&1; then \
		if cd apps/queue-engine && cargo audit; then \
			echo "PASS: cargo-audit"; \
		else \
			echo "FAIL: cargo-audit found issues"; \
			exit 1; \
		fi; \
	else \
		echo "SKIP: cargo-audit not installed (run: cargo install cargo-audit)"; \
	fi
	@echo "==> Secrets scan"
	@if which gitleaks >/dev/null 2>&1; then \
		if gitleaks detect --source . --verbose; then \
			echo "PASS: gitleaks"; \
		else \
			echo "FAIL: gitleaks found issues"; \
			exit 1; \
		fi; \
	else \
		echo "SKIP: gitleaks not installed (see .env.example or install from https://github.com/gitleaks/gitleaks)"; \
	fi
