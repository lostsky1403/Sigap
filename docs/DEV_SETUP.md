# Developer Setup Guide

This guide walks you through setting up Sigap for local development.

> **Want to demo Sigap in 10 minutes?** See [`docs/DEMO_FLOW.md`](./DEMO_FLOW.md)
> for the PowerShell-friendly happy-path walkthrough plus the
> `scripts/smoke/sigap-demo-smoke.ps1` smoke suite.

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

### Admin Endpoints (Dev Identity)

The following admin endpoints require `X-Sigap-Dev-User-ID` in `dev` auth mode. The SvelteKit admin UI (`/admin/facilities`, `/admin/queues`, `/admin/schedules`, `/admin/appointments`) proxies these via same-origin `+server.ts` routes.

**Facility CRUD** (needs `facility.read` / `facility.manage`):
```bash
# List all facilities
curl http://localhost:8080/api/v1/admin/facilities \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Get facility by ID
curl http://localhost:8080/api/v1/admin/facilities/f1 \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Create facility
curl -X POST http://localhost:8080/api/v1/admin/facilities \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"name":"RS Baru","type":"rumah_sakit","kecamatan":"Sukmajaya","kabupaten_kota":"Depok","province":"Jawa Barat","total_beds":50,"available_beds":10}'

# Update facility
curl -X PATCH http://localhost:8080/api/v1/admin/facilities/f1 \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"name":"RS Baru Updated","total_beds":60}'

# Deactivate (soft delete)
curl -X PATCH http://localhost:8080/api/v1/admin/facilities/f1/deactivate \
  -H "X-Sigap-Dev-User-ID: dev-user-42"
```

**Queue Operator Console** (needs `queue.read` / `queue.manage`):
```bash
# List queue tickets by facility
curl "http://localhost:8080/api/v1/admin/queues?facility_id=f1" \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Get queue ticket detail
curl http://localhost:8080/api/v1/admin/queues/a1b2c3d4 \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Update status (state-machine enforced)
curl -X PATCH http://localhost:8080/api/v1/admin/queues/a1b2c3d4/status \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"status":"called"}'
```

**Appointment Operator Console** (needs `appointment.read` / `appointment.manage`):
```bash
# List appointments with status filter
curl "http://localhost:8080/api/v1/admin/appointments" \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Update appointment status (state machine enforced)
curl -X PATCH http://localhost:8080/api/v1/admin/appointments/a1b2c3d4/status \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"status":"completed"}'
```

**Schedule Management** (needs `schedule.read` / `schedule.manage`):
```bash
# List schedules
curl "http://localhost:8080/api/v1/admin/schedules" \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# Create schedule
curl -X POST http://localhost:8080/api/v1/admin/schedules \
  -H "Content-Type: application/json" \
  -H "X-Sigap-Dev-User-ID: dev-user-42" \
  -d '{"facility_id":"f1","service_unit_id":"u1","schedule_date":"2026-06-22","start_time":"08:00","end_time":"12:00","slot_minutes":30,"capacity_per_slot":2}'
```

**Public Booking** (no auth required):
```bash
# Book appointment (rate-limited by phone)
curl -X POST http://localhost:8080/api/v1/appointments \
  -H "Content-Type: application/json" \
  -d '{"facility_id":"f1","service_unit_id":"u1","appointment_time":"2026-06-22T09:00:00Z","patient_phone":"081234567890","patient_display_name":"Budi Santoso"}'

# Check-in with code (returns queue number)
curl -X POST http://localhost:8080/api/v1/appointments/{id}/check-in \
  -H "Content-Type: application/json" \
  -d '{"checkin_code":"A3B9K2"}'
```

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

## Auth Mode Selection

The API gateway supports three authentication modes, controlled by the `SIGAP_AUTH_MODE` environment variable. Set it in `.env` before starting the API.

### Mode: `disabled` (default)

No authentication provider is active. The middleware chain passes through transparently, but the authorization layer (`RequirePermission`) still denies requests for protected routes unless an actor is present. This is the safest default for a fresh checkout.

```env
SIGAP_AUTH_MODE=disabled
```

Expected behavior:
- Public routes (`/health`, `/readyz`) return `200`.
- Protected routes (e.g., `/api/v1/admin/facilities`) return `403` for all requests.

### Mode: `dev`

Uses `DevIdentityProvider`. When `SIGAP_DEV_IDENTITY=true`, requests carrying the `X-Sigap-Dev-User-ID` header receive a synthetic `Actor` with full permissions. This is the fastest way to test protected routes locally.

```env
SIGAP_AUTH_MODE=dev
SIGAP_DEV_IDENTITY=true
```

Then send requests with the dev header:
```bash
curl http://localhost:8080/api/v1/admin/facilities \
  -H "X-Sigap-Dev-User-ID: dev-user-42"
```

> ⚠️ **Never** use `dev` mode in production or shared environments. It is trivially bypassable.

### Mode: `jwt`

Uses `JWTProvider` with JWKS validation. Tokens must be signed with RS256/RS384/RS512 or ES256/ES384/ES512 and include standard claims (`iss`, `aud`, `sub`, `exp`).

```env
SIGAP_AUTH_MODE=jwt
SIGAP_AUTH_ISSUER=https://your-oidc-provider
SIGAP_AUTH_AUDIENCE=sigap-api
SIGAP_AUTH_JWKS_URL=https://your-oidc-provider/.well-known/jwks.json
```

 Minimal test with a valid JWT (replace `$TOKEN`):
```bash
curl http://localhost:8080/api/v1/admin/facilities \
  -H "Authorization: Bearer $TOKEN"
```

### Auth Environment Variables

| Variable | Required | Mode | Description |
|----------|----------|------|-------------|
| `SIGAP_AUTH_MODE` | No | all | `disabled` (default), `dev`, or `jwt`. |
| `SIGAP_DEV_IDENTITY` | No | `dev` | Enable synthetic dev actor when `"true"`. |
| `SIGAP_AUTH_ISSUER` | Yes | `jwt` | Expected `iss` claim. |
| `SIGAP_AUTH_AUDIENCE` | Yes | `jwt` | Expected `aud` claim. |
| `SIGAP_AUTH_JWKS_URL` | No | `jwt` | JWKS endpoint for key retrieval. |

> **Validation:** Invalid auth configuration (e.g., `jwt` mode without `SIGAP_AUTH_ISSUER`) causes the server to exit with code `1` at boot time. This fail-closed behavior prevents a misconfigured server from starting.

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

## Bootstrap Admin (One-Time Setup)

After applying migrations and RBAC seed data, you can create the first admin user with the bootstrap CLI. This tool is **disabled by default** and must be explicitly enabled.

> 🚫 **DANGER: Never enable `SIGAP_BOOTSTRAP_ADMIN` in production or shared environments.** The tool creates a synthetic account (`admin@sigap.local`) with full system access (`super_admin`). It is intended solely for standing up a fresh local development database.

### Requirements

- PostgreSQL must be running with migrations applied (`make db-migrate`)
- RBAC seed must be loaded (`make db-seed` or `psql $DATABASE_URL -f packages/db/seed/rbac.sql`) so the `super_admin` role exists

### Run Bootstrap

```bash
SIGAP_BOOTSTRAP_ADMIN=true \
  DATABASE_URL=postgres://sigap:sigap@localhost:5432/sigap \
  go run ./cmd/bootstrap
```

On success, the output will be similar to:

```
[bootstrap] Created bootstrap admin user: a1b2c3d4-... (admin@sigap.local)
[bootstrap] Assigned super_admin role to a1b2c3d4-...
```

Rerunning the command is safe (idempotent). If the admin already exists, it will print:

```
[bootstrap] Bootstrap admin user already exists: a1b2c3d4-... (admin@sigap.local)
[bootstrap] super_admin role already assigned to a1b2c3d4-...
```

### What It Does

1. Creates a single user in `app_users` with:
   - `email`: `admin@sigap.local` (`.local` TLD prevents accidental email delivery)
   - `display_name`: `Bootstrap Admin`
   - `status`: `active`
2. Assigns the existing `super_admin` role from the RBAC seed via `user_roles`

### Safety Controls

- **Env gate**: The tool exits immediately with code `1` unless `SIGAP_BOOTSTRAP_ADMIN` is exactly `"true"`.
- **No hardcoded secrets**: No passwords or API keys are embedded.
- **No network exposure**: This is a CLI tool, not an HTTP endpoint.
- **Synthetic data only**: Email uses `.local` domain; no real PII is involved.

## Testing & Security Scanning

### Running Tests

```bash
# All unit tests (Go + Rust + Web)
make test

# Go tests only
cd apps/api && go test ./...

# Rust tests only
cd apps/queue-engine && cargo test

# Web type-checking only
pnpm --filter sigap-web run check
```

### Lint & Format Checks

```bash
# Type checking, linting, and formatting
make lint

# Auto-fix Go formatting
cd apps/api && gofmt -w .

# Auto-fix Rust formatting
cd apps/queue-engine && cargo fmt

# Auto-fix web formatting
pnpm --filter sigap-web run format
```

### Security Scanning

The `make security` target runs three security tools. Install any missing tools before running it.

#### 1. Go Vulnerability Check (`govulncheck`)

Install:
```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```

Requires: Go 1.21+ (uses the Go toolchain automatic download for newer versions).

Run standalone:
```bash
cd apps/api && govulncheck ./...
```

#### 2. Rust Audit (`cargo-audit`)

Install:
```bash
cargo install cargo-audit
```

Run standalone:
```bash
cd apps/queue-engine && cargo audit
```

#### 3. Secrets Scan (`gitleaks`)

Install (choose one):

**With package managers:**
```bash
# macOS
brew install gitleaks

# Ubuntu/Debian (requires adding the repo)
echo "deb [trusted=yes] https://apt.gitleaks.io/ /" | sudo tee /etc/apt/sources.list.d/gitleaks.list
sudo apt update && sudo apt install gitleaks
```

**With Go install** (requires Go 1.22+):
```bash
go install github.com/gitleaks/gitleaks/v8@latest
```

**Manual binary** (Linux AMD64):
```bash
curl -sSL https://github.com/gitleaks/gitleaks/releases/latest/download/gitleaks_$(curl -s https://api.github.com/repos/gitleaks/gitleaks/releases/latest | grep tag_name | cut -d '"' -f 4 | tr -d 'v')_linux_x64.tar.gz | tar -xz -C /tmp
sudo mv /tmp/gitleaks /usr/local/bin/
```

Run standalone:
```bash
gitleaks detect --source . --no-git
```

#### Running the Full Security Suite

After installing all three tools:

```bash
make security
```

Output meaning:
- `PASS` — the tool ran and found no issues.
- `FAIL` — the tool ran and found real issues. The Makefile exits with code 1.
- `SKIP` — the tool is not installed on your machine.

> **Note:** If a tool prints `SKIP`, install it using the instructions above. We do not commit `gitleaks` binaries to the repository.

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
