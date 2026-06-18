# Developer Setup Guide

This guide walks you through setting up Sigap for local development.

## Prerequisites

- **Go** 1.22+
- **Rust** 1.78+ with `cargo`
- **Node.js** 20+ with `pnpm`
- **PostgreSQL** 16+ (or Docker)
- **protoc** (Protocol Buffers compiler) — required for Rust gRPC code generation
- **Make**

## Quick Start (Local)

### 1. Clone & install dependencies

```bash
git clone <repo>
cd Sigap
```

Install frontend dependencies:
```bash
pnpm install
```

Install Go dependencies:
```bash
cd apps/api && go mod download
```

### 2. Environment Configuration

Copy the example environment file and set your values:

```bash
cp .env.example .env
```

At minimum, set:
- `POSTGRES_PASSWORD` — strong password for local database
- `DATABASE_URL` — connection string matching your Postgres setup

> **Never commit `.env` or put real secrets in `.env.example`.**

### 3. Start PostgreSQL

With Docker:
```bash
docker compose up -d postgres
```

Or using a local Postgres:
```bash
# macOS with Homebrew
brew services start postgresql@16

# Or create the database manually
psql -c "CREATE DATABASE sigap;"
psql -c "CREATE USER sigap WITH PASSWORD 'your_password';"
```

### 4. Apply Migrations

```bash
make db-migrate
```

This applies `packages/db/migrations/0001_init.sql` to create all tables and indexes.

### 5. Seed Data (Optional)

```bash
make db-seed
```

Populates the database with 6 sample facilities for local testing.

### 6. Run Services

You have three options for running services:

#### Option A: Full Docker Compose (Recommended for first-time setup)
```bash
make dev
```

This builds and starts all containers (Postgres + Engine + API + Web) and auto-connects them.

#### Option B: Individual services (for iterative development)

Use separate terminals:

```bash
# Terminal 1 — Rust gRPC engine
make dev-engine

# Terminal 2 — Go API gateway
make dev-api

# Terminal 3 — SvelteKit frontend
make dev-web
```

Service ports:
- API: http://localhost:8080
- Web: http://localhost:5173
- gRPC: localhost:50051
- Web (Docker): http://localhost:3000

### 7. Verify Endpoints

```bash
# Health check
curl http://localhost:8080/health

# Generate a queue number
curl -X POST http://localhost:8080/api/v1/queues/generate \
  -H "Content-Type: application/json" \
  -d '{"facilityId":"f1","patient":{"fullName":"Test Pasien","phone":"081234567890"}}'
```

## Optional: Dev Environment Without Rust Engine

If you don't have `protoc` installed, you can still develop the Go API and Web frontend using the fake queue service:

```bash
# In .env:
# SIGAP_ENGINE_FALLBACK=dev
# SIGAP_ENGINE_ADDR=localhost:50051

make dev-api
make dev-web
```

The API will use an in-memory fake queue service instead of connecting to the real Rust engine.

> ⚠️ The fallback flag is **for local development only**. Never enable `SIGAP_ENGINE_FALLBACK=dev` in production.

## Dev Identity (Local Testing)

The API includes a synthetic dev-identity middleware for local testing when real authentication is not configured.

Enable it in `.env`:
```env
SIGAP_DEV_IDENTITY=true
```

Then send the `X-Sigap-Dev-User-ID` header with any request:
```bash
curl -X POST http://localhost:8080/api/v1/queues/generate \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"facilityId":"f1","patient":{"fullName":"Test Pasien","phone":"081234567890"}}'
```

When enabled, this header injects a synthetic `Actor` with type `dev` and a full set of synthetic permissions so you can test protected routes locally.

> ⚠️ **NEVER** enable `SIGAP_DEV_IDENTITY=true` in production, staging, or any shared environment. The middleware fails closed when this variable is absent or not set to `"true"` exactly.

## Testing

```bash
# All unit tests (Go + Rust)
make test

# Type checking, linting, and formatting
make lint

# Security scanning (requires govulncheck, cargo-audit, gitleaks)
make security
```

## Docker Compose (Full Stack)

```bash
# Start all services
make dev

# View logs
make watch
docker compose logs -f api
docker compose logs -f engine

# Stop everything
make dev-down
```

Common Docker Compose commands:
```bash
docker compose up -d --build          # Build and start
docker compose down -v                # Stop and remove volumes
docker compose ps                     # Check container status
docker compose exec api go test ./... # Run API tests in container
```

## Troubleshooting

### `protoc` not found

The Rust engine needs `protoc` to compile `.proto` files during build.

```bash
# Ubuntu/Debian
sudo apt-get install -y protobuf-compiler

# macOS
brew install protobuf

# Windows (with winget)
winget install Google.Protobuf
```

### Port conflicts

If ports 5432, 8080, 50051, 5173, or 3000 are in use:

Edit `.env` to remap:
```bash
# .env
POSTGRES_PORT=5433
API_PORT=8081
ENGINE_PORT=50052
```

Then update `docker-compose.yml` `ports:` mapping accordingly.

### Database connection issues

```bash
# Verify database is reachable
psql "$DATABASE_URL" -c "SELECT 1"

# Reset database
make dev-down
docker volume rm sigap_postgres_data
make db-migrate
make db-seed
```

### SvelteKit web shows errors

```bash
# Regenerate SvelteKit type stubs
pnpm --filter sigap-web exec svelte-kit sync

# Clean build cache
rm -rf apps/web/.svelte-kit apps/web/build
pnpm --filter sigap-web run build
```

## Security Notes

- **Never use real patient data** in local development.
- **Do not enable** `SIGAP_ENGINE_FALLBACK=dev` in production.
- **Use `.env.example`** as the source of truth for required environment variables.
- **Run `make security`** before submitting PRs.

## Further Reading

- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — PR workflow, commit conventions, code review standards
- [`ROADMAP.md`](../ROADMAP.md) — Project phases, completed milestones, and future backlog
- [`.env.example`](../.env.example) — Full list of environment variables
