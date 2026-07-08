# Sigap Local Demo Flow

This document is the canonical "10-minute demo" guide for Sigap. Follow it
on Windows PowerShell (or PowerShell 7+) to bring up the stack, exercise
the happy path end-to-end, and explore the admin and patient UIs.

> **Audience**: contributors, reviewers, demo recipients.
> **Goal**: prove that a fresh checkout can run the full appointment → check-in → queue → admin flow without any external services, real patient data, or paid tooling.

---

## 1. Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.22+ | for the API |
| Rust | 1.78+ with `cargo` | for the queue engine (needs `protoc` too) |
| Node.js | 20+ with `pnpm` | for the SvelteKit web |
| PostgreSQL | 16+ | local server or Docker container |
| PowerShell | 7+ | for the smoke script (`pwsh`) |
| `psql` | any recent | for loading migrations and seeds |

If you are missing `protoc`, you can still run the full demo in fallback
mode (`SIGAP_ENGINE_FALLBACK=dev`). The API creates a real queue ticket
in the database, so check-in, queue listing, and status transitions all
work. See [§ Runtime Modes](#runtime-modes) below.

> **Timezone notes**
>
> - The demo seed schedules use `CURRENT_DATE + INTERVAL '1 day'` in the
>   **PostgreSQL server's** timezone, not the client's.
> - The smoke script computes `appointment_time` using
>   `(Get-Date).ToUniversalTime().Date.AddDays(1).AddHours(9)` and formats
>   the timestamp as `yyyy-MM-ddTHH:mm:ssZ` (UTC). The API server then
>   validates the timestamp against its own clock.
> - If the PostgreSQL server, the API server, and your shell all use
>   different timezone settings, `appointment_time` may appear "in the
>   past" and the booking will return **400 — "Waktu janji temu harus di
>   masa depan."** See [§ Troubleshooting](#troubleshooting) for fixes.

---

## 2. One-time setup

```powershell
# Clone and enter the repo
git clone <repo>
cd Sigap

# Copy environment template — set POSTGRES_PASSWORD + DATABASE_URL
Copy-Item .env.example .env
# Edit .env in your editor of choice. Set:
#   POSTGRES_PASSWORD=sigap
#   DATABASE_URL=postgresql://postgres:sigap@localhost:5432/sigap
#   SIGAP_AUTH_MODE=dev
#   SIGAP_DEV_IDENTITY=true
```

Start PostgreSQL (Docker example):

```powershell
docker run --name sigap-db -e POSTGRES_PASSWORD=sigap -e POSTGRES_DB=sigap -p 5432:5432 -d postgres:16
```

Apply migrations + RBAC seed + demo seed:

```powershell
psql $env:DATABASE_URL -f packages/db/migrations/0001_init.sql
psql $env:DATABASE_URL -f packages/db/migrations/0002_medical_records.sql
psql $env:DATABASE_URL -f packages/db/migrations/0003_identity_rbac.sql
psql $env:DATABASE_URL -f packages/db/migrations/0004_audit_events.sql
psql $env:DATABASE_URL -f packages/db/migrations/0005_appointments.sql
psql $env:DATABASE_URL -f packages/db/seed/rbac.sql
psql $env:DATABASE_URL -f packages/db/seed/dev.sql
psql $env:DATABASE_URL -f packages/db/seed/demo.sql   # NEW: demo-only synthetic data
```

Install dependencies:

```powershell
pnpm install
cd apps/api; go mod download; cd ../..
```

---

## 3. Runtime Modes

Sigap supports two local runtime modes. Choose based on what you need.

### Mode A — Fast local demo (no Rust engine)

The API falls back to an in-memory queue service that persists a real
`queue_tickets` row in the database. No Rust engine or `protoc` needed.

```powershell
# Required env vars (export before starting the API)
$env:DATABASE_URL          = "postgresql://postgres:sigap@localhost:5432/sigap"
$env:SIGAP_DATABASE_URL    = $env:DATABASE_URL   # notification worker uses this
$env:SIGAP_AUTH_MODE       = "dev"
$env:SIGAP_DEV_IDENTITY    = "true"
$env:SIGAP_ENGINE_FALLBACK = "dev"
```

```powershell
# Terminal 1: Go API
cd apps/api
go run ./cmd/server

# Terminal 2: SvelteKit web (optional)
pnpm --filter sigap-web dev

# Terminal 3: run demo smoke
pwsh -File scripts/smoke/sigap-demo-smoke.ps1
```

The demo smoke (`sigap-demo-smoke.ps1`) passes in this mode.

### Mode B — Full integration (Rust queue engine)

The real gRPC queue engine generates tickets with microsecond-level
traceability and SHA-256 signatures.

```powershell
# Required env vars (export before starting the API)
$env:DATABASE_URL       = "postgresql://postgres:sigap@localhost:5432/sigap"
$env:SIGAP_DATABASE_URL = $env:DATABASE_URL
$env:SIGAP_AUTH_MODE    = "dev"
$env:SIGAP_DEV_IDENTITY = "true"
# Do NOT set SIGAP_ENGINE_FALLBACK — the API connects to the real engine
```

```powershell
# Terminal 1: Rust queue engine
cd apps/queue-engine
cargo run

# Terminal 2: Go API (after engine is listening on :50051)
cd apps/api
go run ./cmd/server

# Terminal 3: SvelteKit web (optional)
pnpm --filter sigap-web dev

# Terminal 4: full local demo (seeds + all 3 smoke suites)
pwsh -File scripts/smoke/sigap-full-local-demo.ps1
```

The full local demo (`sigap-full-local-demo.ps1`) passes in this mode.

### Environment variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `DATABASE_URL` | yes | PostgreSQL connection string for `psql` seeds and Go API |
| `SIGAP_DATABASE_URL` | yes (worker) | Same as `DATABASE_URL`; used by the notification worker |
| `SIGAP_AUTH_MODE` | yes | Set to `dev` for local dev-identity auth |
| `SIGAP_DEV_IDENTITY` | yes | Set to `true` to enable `X-Sigap-Dev-User-ID` header |
| `SIGAP_ENGINE_FALLBACK` | Mode A only | Set to `dev` to skip the Rust queue engine |
| `SIGAP_API_BASE` | no | Override API base URL (default `http://[::1]:8080`) |

> **Restart-safe**: These env vars live in your shell session. After a
> terminal restart or new shell, re-export them (or source your `.env`).
> If `psql` prompts for a user/password, `DATABASE_URL` is not set.

### Seed idempotency

All seed files (`dev.sql`, `rbac.sql`, `demo.sql`) are idempotent. Re-running
them does not create duplicate facilities, roles, or demo data. The `RSK`
facility count stays at 6 regardless of how many times you seed.

---

## 4. Start the stack

The stack setup depends on your chosen runtime mode (see [§ 3](#3-runtime-modes)).
Below is the full-integration layout (Mode B). For Mode A (fast demo),
skip Terminal 1 (Rust engine) and add `SIGAP_ENGINE_FALLBACK=dev` to your env.

Open **three** PowerShell 7 terminals side by side.

### Terminal 1 — Rust engine (gRPC on :50051)

```powershell
cd apps/queue-engine
cargo run
```

Wait for the line that confirms the tonic server is listening on `:50051`.

### Terminal 2 — Go API (HTTP on :8080)

```powershell
cd apps/api
go run ./cmd/server
```

Wait for the line `listening on :8080` (or equivalent).

### Terminal 3 — SvelteKit web (Vite on :5173)

```powershell
pnpm --filter sigap-web dev
```

Wait for the line `Local: http://localhost:5173/`.

---

## 5. Run the smoke suite

In a fourth terminal (or Terminal 3 once the web is up):

```powershell
pwsh -File scripts/smoke/sigap-demo-smoke.ps1
```

Expected output (truncated):

```
==> Step 1/6: GET /health
[PASS] health
==> Step 2/6: GET /api/v1/admin/facilities (dev identity)
[PASS] admin.facilities.list
==> Step 3/6: POST /api/v1/appointments (public booking)
[PASS] public.booking
==> Step 4/6: POST /api/v1/appointments/{id}/check-in
[PASS] public.checkin
==> Step 5/6: GET /api/v1/admin/queues?facility_id=...
[PASS] admin.queues.list
==> Step 6/6: PATCH /api/v1/admin/appointments/{id}/status
[PASS] admin.appointments.status

Passed: 6 / 6
```

Exit code is `0` on success. If any step fails, the offending HTTP status
and response body are printed.

### Other smoke scripts

Two additional smoke scripts cover the notification pipeline and patient portal:

```powershell
# Notification pipeline smoke (requires dev identity + notification seed)
pwsh -File scripts/smoke/sigap-notification-smoke.ps1

# Patient portal smoke (public endpoint, no auth required)
pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1
```

See [`scripts/smoke/README.md`](../scripts/smoke/README.md) for parameters and exit codes.

---

## 6. Walk through the UIs

Open <http://localhost:5173> in your browser.

### Admin pages

| Page | URL | What to check |
|------|-----|---------------|
| Facilities | `/admin/facilities` | List of 6 seeded facilities; create / edit / deactivate |
| Queues | `/admin/queues` | Status console; your smoke appointment shows as `queued` → `completed` |
| Schedules | `/admin/schedules` | Two demo schedules for tomorrow (Poli Umum, Poli Gigi) |
| Appointments | `/admin/appointments` | Your smoke appointment, status transitions |

### Patient pages

| Page | URL | What to check |
|------|-----|---------------|
| New appointment | `/appointments/new` | Public booking form. Submit with a fake name and `+62-555-01xx` phone. You receive a 6-character check-in code. |
| Check-in | `/appointments/check-in` | Enter the code from the previous step. A queue number is issued and the appointment moves to `queued`. |

### Patient status portal

| Page | URL | What to check |
|------|-----|---------------|
| Status lookup | `/patient/status` | Enter synthetic code `SMOKE01` (from demo seed). Shows facility name, appointment status, scheduled time, and check-in status. No PII is displayed. |

### Real-time dashboard

The home page renders the **Bed Availability Dashboard** with live SSE
updates from the API at `/api/v1/events/beds`. Open it in two browser tabs
and toggle a bed count via the admin page — the dashboard updates without
refresh.

---

## 7. One-liner verification (no scripts)

If you only want to spot-check the API without running the full smoke
suite, here are six PowerShell-friendly curl-equivalents.

### Health

```powershell
Invoke-RestMethod http://localhost:8080/health
```

### Admin facility list (dev identity)

```powershell
Invoke-RestMethod -Headers @{ 'X-Sigap-Dev-User-ID' = 'dev-user-demo' } `
    http://localhost:8080/api/v1/admin/facilities
```

### Public booking

```powershell
$fid = (Invoke-RestMethod -Headers @{ 'X-Sigap-Dev-User-ID' = 'dev-user-demo' } `
    http://localhost:8080/api/v1/admin/facilities).data[0].id

$body = @{
    facility_id          = $fid
    service_unit_id      = '00000000-0000-0000-0000-00000000d001'
    patient_display_name = 'Pasien Demo Manual'
    patient_phone        = '+62-555-0199'
    appointment_time     = (Get-Date).ToUniversalTime().Date.AddDays(1).AddHours(10).ToString('yyyy-MM-ddTHH:mm:ssZ')
} | ConvertTo-Json

Invoke-RestMethod -Method Post -ContentType 'application/json' -Body $body `
    http://localhost:8080/api/v1/appointments
```

Save the returned `id` and `checkin_code` for the next step.

### Check-in

```powershell
Invoke-RestMethod -Method Post -ContentType 'application/json' `
    -Body (@{ checkin_code = 'A3B9K2' } | ConvertTo-Json) `
    http://localhost:8080/api/v1/appointments/<id>/check-in
```

### Admin queue list

```powershell
Invoke-RestMethod -Headers @{ 'X-Sigap-Dev-User-ID' = 'dev-user-demo' } `
    "http://localhost:8080/api/v1/admin/queues?facility_id=$fid"
```

### Admin appointment status update

```powershell
Invoke-RestMethod -Method Patch -Headers @{ 'X-Sigap-Dev-User-ID' = 'dev-user-demo' } `
    -ContentType 'application/json' `
    -Body (@{ status = 'completed' } | ConvertTo-Json) `
    http://localhost:8080/api/v1/admin/appointments/<id>/status
```

---

## 8. Demo data

`packages/db/seed/demo.sql` adds **synthetic** demo data only:

- 2 service units (`DEMO-UMUM`, `DEMO-GIGI`) tied to facility `f1`.
- 2 practitioners (`Dokter Demo A`, `Dokter Demo B`).
- 2 schedules for **tomorrow** (rolled forward at seed time), 09:00–12:00,
  30-minute slots, capacity 3 per slot.

All IDs are fixed UUIDs so the smoke script can find them by string. The
seed is **idempotent** — re-running `psql … -f demo.sql` is safe and does
not duplicate rows.

The seed uses the `+62-555-01xx` phone range, which is reserved by
ITU-T for fictional use, and patient names like `Pasien Demo A`. **Never
replace this data with real patient information.**

---

## 9. Troubleshooting

### `[FAIL] health` — connection refused

The Go API isn't running. Start it (see [§ 4](#4-start-the-stack)).

### `[FAIL] admin.facilities.list` — HTTP 403

`SIGAP_AUTH_MODE=dev` and `SIGAP_DEV_IDENTITY=true` are not set in `.env`.
The script sends the dev header but the API is in `disabled` mode and
rejects every request.

### `[FAIL] public.checkin` — queue service unavailable

The Rust engine isn't running. Two options:

1. **Start the engine**: `cd apps/queue-engine; cargo run` (Terminal 1).
2. **Use fallback mode**: restart the API with `SIGAP_ENGINE_FALLBACK=dev`.
   The API creates a real `queue_tickets` row in the database, so check-in
   and all downstream steps work.

If you see "Gagal mengambil nomor antrean" but no fallback log, the API
was started without `SIGAP_ENGINE_FALLBACK=dev`. Restart it with the env
var set.

### Terminal restart lost my env vars

Env vars are shell-scoped. After closing a terminal, re-export them:

```powershell
$env:DATABASE_URL          = "postgresql://postgres:sigap@localhost:5432/sigap"
$env:SIGAP_DATABASE_URL    = $env:DATABASE_URL
$env:SIGAP_AUTH_MODE       = "dev"
$env:SIGAP_DEV_IDENTITY    = "true"
$env:SIGAP_ENGINE_FALLBACK = "dev"   # Mode A only
```

If `psql` prompts for a user/password, `DATABASE_URL` is not set.

### `[FAIL] public.checkin` — daily rate limit

The smoke script generates a fresh random phone per run, but if you re-run
it many times on the same day, the per-phone limit can still hit. Edit
the script's `$phone` calculation or use the `-SkipSeed` parameter to book
without capacity validation.

### `[FAIL] public.booking` — "Waktu janji temu harus di masa depan" (past appointment)

The API server rejected `appointment_time` because, in its own clock view,
the timestamp is in the past. This is a **clock/timezone mismatch** between
your shell, the PostgreSQL server, and/or the API server.

Quick checks:

```powershell
# Compare three clocks. All three must agree within a minute or two.
Get-Date                                                # your shell
Invoke-RestMethod http://localhost:8080/health           # API server
psql $env:DATABASE_URL -c "SELECT NOW() AT TIME ZONE 'UTC' AS pg_utc"   # Postgres
```

Fixes (pick one):

1. **Send an explicit UTC ISO 8601 timestamp** with a `Z` suffix (the smoke
   script already does this; do the same in ad-hoc curls):
   `-d '{\"appointment_time\":\"2026-06-22T09:00:00Z\"}'`
2. **Align the clocks** so PostgreSQL, the API server, and your shell all
   run NTP-synced UTC.
3. **Re-run after a few seconds** if the API just booted and `appointment_time`
   is computed on the very second the API calls `time.Now().UTC()`.

### `[FAIL] public.booking` — schedule slot is full

The demo seed creates 3 slots per 30-minute window, so on a fresh DB you
have 18 bookable slots (2 service units × 6 slots × 3 capacity). If you
re-run the smoke script ~18 times the same day, capacity will be exhausted
for that schedule. Use `-SkipSeed` to book without capacity validation,
or remove the schedule with
`psql $env:DATABASE_URL -c "DELETE FROM practitioner_schedules WHERE id='00000000-0000-0000-0000-00000000d021'"`.

### `pnpm install` warns about Svelte 5 + vite-plugin-svelte version

This is a known warning. The build still works. See
[`docs/STABILIZATION_REPORT.md`](./STABILIZATION_REPORT.md) for context.

### `protoc` not found

Install it: `winget install Google.Protobuf` on Windows, or
`brew install protobuf` on macOS, or `apt-get install -y protobuf-compiler`
on Debian/Ubuntu. The Rust engine needs it to compile the gRPC stubs.

---

## 10. Next steps

- Browse [`docs/APPOINTMENTS_REPORT.md`](./APPOINTMENTS_REPORT.md) for the
  full appointments/check-in design.
- Browse [`docs/FACILITY_ADMIN_REPORT.md`](./FACILITY_ADMIN_REPORT.md) for
  the admin surface.
- See [`README.md`](../README.md) and [`docs/DEV_SETUP.md`](./DEV_SETUP.md)
  for the longer-form developer guide.
- See [`SECURITY.md`](../SECURITY.md) for the demo-data safety policy.
