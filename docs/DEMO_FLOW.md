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

If you are missing `protoc`, you can still run the API + web in fallback
mode (`SIGAP_ENGINE_FALLBACK=dev`). The smoke script's check-in step will
fail in fallback mode because no real queue ticket is generated — see
[§ Troubleshooting](#troubleshooting).

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

## 3. Start the stack (three PowerShell terminals)

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

## 4. Run the smoke suite

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

---

## 5. Walk through the UIs

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

### Real-time dashboard

The home page renders the **Bed Availability Dashboard** with live SSE
updates from the API at `/api/v1/events/beds`. Open it in two browser tabs
and toggle a bed count via the admin page — the dashboard updates without
refresh.

---

## 6. One-liner verification (no scripts)

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

## 7. Demo data

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

## 8. Troubleshooting

### `[FAIL] health` — connection refused

The Go API isn't running. Start it (see [§ 3](#3-start-the-stack-three-powershell-terminals)).

### `[FAIL] admin.facilities.list` — HTTP 403

`SIGAP_AUTH_MODE=dev` and `SIGAP_DEV_IDENTITY=true` are not set in `.env`.
The script sends the dev header but the API is in `disabled` mode and
rejects every request.

### `[FAIL] public.checkin` — queue service unavailable

The Rust engine isn't running, or `SIGAP_ENGINE_FALLBACK=dev` is set
but the gRPC client is still trying to reach it. Either start the engine
(Terminal 1) or accept that check-in will fail until the engine is up.

### `[FAIL] public.checkin` — daily rate limit

The smoke script generates a fresh random phone per run, but if you re-run
it many times on the same day, the per-phone limit can still hit. Edit
the script's `$phone` calculation or use the `-SkipSeed` parameter to book
without capacity validation.

### `pnpm install` warns about Svelte 5 + vite-plugin-svelte version

This is a known warning. The build still works. See
[`docs/STABILIZATION_REPORT.md`](./STABILIZATION_REPORT.md) for context.

### `protoc` not found

Install it: `winget install Google.Protobuf` on Windows, or
`brew install protobuf` on macOS, or `apt-get install -y protobuf-compiler`
on Debian/Ubuntu. The Rust engine needs it to compile the gRPC stubs.

---

## 9. Next steps

- Browse [`docs/APPOINTMENTS_REPORT.md`](./APPOINTMENTS_REPORT.md) for the
  full appointments/check-in design.
- Browse [`docs/FACILITY_ADMIN_REPORT.md`](./FACILITY_ADMIN_REPORT.md) for
  the admin surface.
- See [`README.md`](../README.md) and [`docs/DEV_SETUP.md`](./DEV_SETUP.md)
  for the longer-form developer guide.
- See [`SECURITY.md`](../SECURITY.md) for the demo-data safety policy.
