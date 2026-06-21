# Sigap Smoke Suite

Lightweight, PowerShell-native end-to-end smoke tests for the local Sigap
demo flow. No test framework — just `Invoke-WebRequest` calls with explicit
PASS/FAIL assertions.

## Files

- `sigap-demo-smoke.ps1` — main script; runs the documented happy path.

## Prerequisites

- PowerShell 7+ (`pwsh`).
- A running Sigap stack with the API reachable at `http://localhost:8080`
  (override with `$env:SIGAP_API_BASE` or the `-ApiBase` parameter).
- Demo seed loaded (`psql $DATABASE_URL -f packages/db/seed/demo.sql`).
  Without the demo seed, the script still runs but uses a hardcoded service
  unit UUID that may not exist in your DB.

## Quickstart

```powershell
# Terminal 1: start the API
cd apps/api
go run ./cmd/server

# Terminal 2: start the engine
cd apps/queue-engine
cargo run

# Terminal 3: load the demo seed and run the smoke suite
psql $env:DATABASE_URL -f packages/db/seed/demo.sql
pwsh -File scripts/smoke/sigap-demo-smoke.ps1
```

Expected output: `Passed: 6 / 6` and exit code `0`.

## What it covers

| # | Step | Auth | Description |
|---|------|------|-------------|
| 1 | `GET /health` | none | API is up |
| 2 | `GET /api/v1/admin/facilities` | dev | Facility list, picks `f1` by short_code |
| 3 | `POST /api/v1/appointments` | public | Public booking with synthetic data |
| 4 | `POST /api/v1/appointments/{id}/check-in` | public | Returns queue ticket |
| 5 | `GET /api/v1/admin/queues?facility_id=…` | dev | Queue list shows new ticket |
| 6 | `PATCH /api/v1/admin/appointments/{id}/status` | dev | `queued → completed` |

## Parameters

```powershell
pwsh -File scripts/smoke/sigap-demo-smoke.ps1 `
    -ApiBase http://localhost:8080 `
    -FacilityShortCode 'f1' `
    -ServiceUnitCode 'DEMO-UMUM' `
    -PractitionerScheduleId '00000000-0000-0000-0000-00000000d021' `
    -DevUserId 'dev-user-smoke'
```

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `-ApiBase` | `http://localhost:8080` (or `$env:SIGAP_API_BASE`) | API root |
| `-FacilityShortCode` | `f1` | Facility to book against |
| `-ServiceUnitCode` | `DEMO-UMUM` | Demo seed service unit code (informational) |
| `-PractitionerScheduleId` | demo seed `d021` | Optional schedule for capacity-aware booking |
| `-DevUserId` | `dev-user-smoke` | Value of `X-Sigap-Dev-User-ID` header |
| `-SkipSeed` | off | Skip sending `practitioner_schedule_id` (booking still works) |

## Troubleshooting

- **`[FAIL] health` — connection refused** — the Go API isn't running.
  Start it: `cd apps/api; go run ./cmd/server`.
- **`[FAIL] admin.facilities.list` — HTTP 403** — `SIGAP_AUTH_MODE=dev` and
  `SIGAP_DEV_IDENTITY=true` must both be set in `.env`.
- **`[FAIL] public.booking` — invalid time** — the smoke script books at
  09:00 UTC tomorrow; if the API server clock is far off, re-sync or change
  the `apptTime` calculation in the script.
- **`[FAIL] public.checkin` — daily rate limit** — the script generates a
  fresh random phone per run (`+62-555-01xxx`). If you re-run it many times
  on the same day, the per-phone limit can still hit. Edit the script or
  use `-SkipSeed` to book without capacity validation.

## Privacy

- All patient data is synthetic. The script generates random names like
  `Pasien Demo 4711` and phones in the `+62-555-01xx` reserved-for-testing
  range.
- Dev identity is **local-only**. Do not run with `SIGAP_DEV_IDENTITY=true`
  in any shared environment.
