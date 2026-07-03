# Sigap Local Demo and Operator Runbook

## Purpose

This runbook gives reviewers and operators a single, end-to-end reference
for running the Sigap local demo on a Windows workstation. It covers
environment setup, database provisioning, service startup, automated
smoke verification, and the manual UI walkthrough — all using synthetic
data, offline notification delivery, and no external services.

> **Audience**: code reviewers, demo recipients, QA operators.
> **Goal**: prove the full appointment → check-in → queue → notification
> → patient-status flow works on a fresh checkout.

---

## Safety and Privacy Rules

| Rule | Detail |
|------|--------|
| **Synthetic data only** | Never insert real patient names, phone numbers, or medical records. All demo data uses `+62-555-01xx` (ITU-T reserved range) and `Pasien Demo` names. |
| **No real notification vendor** | The only delivery provider shipped is `DevProvider` (offline, deterministic). No SMS, WhatsApp, or email is ever sent. |
| **No secrets in docs/logs** | Never print JWTs, passwords, API keys, or raw patient contacts in documentation, log output, or screenshots. |
| **Dev identity is local-only** | `SIGAP_DEV_IDENTITY=true` is trivially bypassable. Never enable it in staging, production, or any shared environment. |

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.22+ | Runs the API server and notification worker |
| Rust | 1.78+ with `cargo` | Runs the queue engine (also needs `protoc`) |
| Node.js | 20+ with `pnpm` | Runs the SvelteKit web frontend |
| PostgreSQL | 16+ | Local server or Docker container |
| PowerShell | 7+ (`pwsh`) | Required for smoke scripts and `pr-autopilot` |
| `psql` | any recent | For applying migrations and loading seed data |
| GitHub CLI (`gh`) | optional | Only needed for PR workflow (`pr-autopilot`) |

If `protoc` is not installed, the Rust engine cannot compile its gRPC
stubs. Use `SIGAP_ENGINE_FALLBACK=dev` to skip the engine and run API +
web only. The check-in step of the demo smoke will fail in fallback mode.

---

## Environment Variables

All values below use placeholders. Replace `<…>` with your local values.
**No real passwords should appear in this document.**

| Variable | Example / Default | Purpose |
|----------|-------------------|---------|
| `DATABASE_URL` | `postgresql://<user>:<password>@<host>:<port>/<database>` | Primary connection string for the Go API |
| `SIGAP_DATABASE_URL` | `postgresql://<user>:<password>@<host>:<port>/<database>` | Connection string for the notification worker (typically same as `DATABASE_URL`) |
| `SIGAP_API_BASE` | `http://[::1]:8080` | Override API base URL used by smoke scripts |
| `SIGAP_AUTH_MODE` | `dev` | Auth mode; `dev` enables synthetic dev-identity for local demo |
| `SIGAP_DEV_IDENTITY` | `true` | Enables the `X-Sigap-Dev-User-ID` header injection |
| `SIGAP_ENGINE_FALLBACK` | `dev` | Optional; use in-memory fake queue when Rust engine is unavailable |
| `SIGAP_NOTIFICATION_WORKER_DRY_RUN` | `true` / `false` | If `true`, worker logs but does not mutate outbox rows |
| `SIGAP_NOTIFICATION_WORKER_ONCE` | `true` / `false` | If `true`, drain pending rows once and exit (useful for smoke/CI) |
| `SIGAP_NOTIFICATION_WORKER_ENABLED` | `true` / `false` | Set `false` to disable the loop tick (ONCE mode is unaffected) |
| `SIGAP_NOTIFICATION_WORKER_INTERVAL_SECONDS` | `30` | Loop tick interval; ignored in ONCE mode |
| `SIGAP_NOTIFICATION_WORKER_BATCH_SIZE` | `25` | Maximum outbox rows claimed per tick |

> **Reminder**: Replace `<user>`, `<password>`, `<host>`, `<port>`, and
> `<database>` with your own values. Never hard-code real credentials in
> `.env.example` or version-controlled files.

---

## Database Setup

### 1. Start PostgreSQL

Using Docker with a placeholder password:

```powershell
docker run --name sigap-db `
    -e POSTGRES_PASSWORD=<password> `
    -e POSTGRES_DB=sigap `
    -p 5432:5432 `
    -d postgres:16
```

Set the connection string in your environment:

```powershell
$env:DATABASE_URL = "postgresql://<user>:<password>@localhost:5432/sigap"
```

### 2. Apply migrations (in order)

The migration directory (`packages/db/migrations/`) contains six
forward-only SQL files:

| # | File | Purpose |
|---|------|---------|
| 1 | `0001_init.sql` | Core tables — facilities, service units, practitioners, beds |
| 2 | `0002_medical_records.sql` | Medical record and encounter tables |
| 3 | `0003_identity_rbac.sql` | Identity, users, roles, and RBAC permission tables |
| 4 | `0004_audit_events.sql` | Audit event log table |
| 5 | `0005_appointments.sql` | Appointment, schedule, and check-in tables |
| 6 | `0006_notifications.sql` | Notification templates, outbox, and delivery attempt tables |

Apply them:

```powershell
psql $env:DATABASE_URL -f packages/db/migrations/0001_init.sql
psql $env:DATABASE_URL -f packages/db/migrations/0002_medical_records.sql
psql $env:DATABASE_URL -f packages/db/migrations/0003_identity_rbac.sql
psql $env:DATABASE_URL -f packages/db/migrations/0004_audit_events.sql
psql $env:DATABASE_URL -f packages/db/migrations/0005_appointments.sql
psql $env:DATABASE_URL -f packages/db/migrations/0006_notifications.sql
```

### 3. Load seed data

```powershell
psql $env:DATABASE_URL -f packages/db/seed/rbac.sql    # roles + permissions
psql $env:DATABASE_URL -f packages/db/seed/dev.sql     # 6 demo facilities
psql $env:DATABASE_URL -f packages/db/seed/demo.sql    # synthetic appointments, schedules, notification rows
```

The RBAC seed is additive — re-applying it after the notification
migration adds `notification.read` and `notification.manage` permissions.
The demo seed is idempotent; re-running it does not duplicate rows.

---

## Starting Services

Open **three** PowerShell 7 terminals side by side.

### Terminal 1 — Rust queue engine (gRPC on :50051)

```powershell
cd apps/queue-engine
cargo run
```

Wait for the line confirming the tonic server is listening on `:50051`.

> **No protoc / no Rust?** Skip this terminal and set
> `SIGAP_ENGINE_FALLBACK=dev` in your `.env`. The API will use an
> in-memory fake queue service. Check-in will not produce real queue
> tickets.

### Terminal 2 — Go API (HTTP on :8080)

```powershell
cd apps/api
go run ./cmd/server
```

Wait for the line `listening on :8080`.

### Terminal 3 — SvelteKit web (Vite on :5173)

```powershell
pnpm --filter sigap-web dev
```

Wait for `Local: http://localhost:5173/`.

### Optional Terminal 4 — Notification worker

```powershell
$env:SIGAP_DATABASE_URL = $env:DATABASE_URL
$env:SIGAP_NOTIFICATION_WORKER_ENABLED = "true"
cd apps/api
go run ./cmd/notification-worker
```

Or for a one-shot drain (smoke/CI):

```powershell
$env:SIGAP_NOTIFICATION_WORKER_ONCE = "true"
$env:SIGAP_DATABASE_URL = $env:DATABASE_URL
cd apps/api
go run ./cmd/notification-worker
```

---

## Verification

### Full automated verification

Runs all lint, vet, type-check, and security gates in one command:

```powershell
pwsh -NoProfile -File .\scripts\dev\pr-autopilot.ps1 -VerifyOnly -CommandTimeoutSeconds 300
```

### Individual checks

| Check | Command |
|-------|---------|
| Go unit tests | `cd apps/api; go test ./...` |
| Go vet | `cd apps/api; go vet ./...` |
| SvelteKit type-check | `pnpm --filter sigap-web run check` |
| Secrets scan | `gitleaks detect --source . --redact` |

---

## Smoke Scripts

Three PowerShell-native end-to-end smoke scripts in `scripts/smoke/`:

| Script | Command | Covers |
|--------|---------|--------|
| **Demo smoke** | `pwsh -File scripts/smoke/sigap-demo-smoke.ps1` | Health, admin facilities, public booking, check-in, queue listing, appointment status transition (6 steps) |
| **Notification smoke** | `pwsh -File scripts/smoke/sigap-notification-smoke.ps1` | Health, dev identity, notification summary/listing, worker dry-run, worker once-mode delivery, post-delivery verification (9 steps) |
| **Patient portal smoke** | `pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1` | Health, valid status lookup (`SMOKE01`), invalid-input rejection, unknown-code 404, PII absence check (5 steps) |

All scripts exit `0` on full pass, `1` on any step failure, `2` on
parameter validation error.

---

## Manual Demo Flow

Open <http://localhost:5173> in your browser after starting all three
services.

### 1. Admin dashboard — Facilities

Navigate to `/admin/facilities`. Verify 6 seeded facilities are listed.
Try creating, editing, and deactivating a facility.

### 2. Appointment booking

Navigate to `/appointments/new`. Fill in the public booking form with
synthetic data:

- Select a facility and service unit (`DEMO-UMUM` or `DEMO-GIGI`).
- Patient name: `Pasien Demo Manual`
- Phone: `+62-555-0199`
- Appointment time: tomorrow 10:00 UTC

Submit. Save the returned **6-character check-in code**.

### 3. Check-in

Navigate to `/appointments/check-in`. Enter the check-in code from the
previous step. A queue number is issued and the appointment moves to
`queued` status.

### 4. Queue operator

Navigate to `/admin/queues`. Your smoke appointment appears as a queue
ticket. Advance the status through `called` → `completed`.

### 5. Notification admin

Navigate to `/admin/notifications`. Verify outbox rows appear with
masked recipients, status badges, and channel information. Use retry /
cancel buttons where safe. Raw contacts and PII are never rendered.

### 6. Patient status portal

Navigate to `/patient/status` and enter code `SMOKE01`. The seeded demo
appointment's status is returned without any PII exposure.

---

## Troubleshooting

### Windows localhost vs `[::1]`

Go's `ListenAndServe(":8080")` on Windows creates an IPv6-only socket.
If smoke scripts or manual curls fail with "connection refused" or
"forcibly closed", use `http://[::1]:8080` instead of
`http://localhost:8080`.

### Port conflicts

Ensure these ports are free before starting services:

| Port | Service |
|------|---------|
| `5432` | PostgreSQL |
| `8080` | Go API |
| `50051` | Rust queue engine (gRPC) |
| `5173` | SvelteKit web (Vite dev server) |

Check with: `netstat -ano | findstr ":8080"` (PowerShell).

### Reset notification smoke rows

If re-running the notification smoke leaves stale rows:

```powershell
psql $env:DATABASE_URL -c "DELETE FROM notification_outbox WHERE template_key LIKE '%smoke%'"
```

### API health check

```powershell
Invoke-RestMethod http://[::1]:8080/health
```

### "Waktu janji temu harus di masa depan" (400 — past appointment)

Timezone/clock mismatch between your shell, the API server, and
PostgreSQL. Compare all three clocks and send an explicit UTC ISO 8601
timestamp with a `Z` suffix.

### Schedule slot full

The demo seed creates 18 bookable slots per day (2 service units ×
6 slots × 3 capacity). If exhausted, use `-SkipSeed` on the smoke
script or reset the schedule row.

### `protoc` not found

Install: `winget install Google.Protobuf` (Windows), `brew install protobuf` (macOS), or `apt-get install -y protobuf-compiler` (Debian/Ubuntu).

---

## Known Limitations

| Limitation | Detail |
|------------|--------|
| Dev identity is local-only | `SIGAP_DEV_IDENTITY=true` must never be enabled in production or shared environments. |
| Notification provider is dev/local only | `DevProvider` is offline and deterministic. No real SMS, WhatsApp, or email delivery occurs. |
| Patient portal is foundation only | `/patient/status` is a read-only status lookup, not a full patient account system. |
| Rust engine needs protoc | Without `protoc`, the queue engine cannot compile. Use `SIGAP_ENGINE_FALLBACK=dev` as a workaround. |
| Fallback mode skips queue tickets | Check-in succeeds at the API layer but no real queue ticket is generated in fallback mode. |

---

## Further Reading

- [`docs/DEMO_FLOW.md`](./DEMO_FLOW.md) — 10-minute happy-path demo walkthrough
- [`docs/DEV_SETUP.md`](./DEV_SETUP.md) — Full developer setup, auth modes, notification worker, bootstrap admin
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — PR workflow, commit conventions, code review standards
- [`ROADMAP.md`](../ROADMAP.md) — Project phases, completed milestones, and future backlog
- [`scripts/smoke/README.md`](../scripts/smoke/README.md) — Smoke script parameters, exit codes, and troubleshooting
