# Sigap Smoke Suite

Lightweight, PowerShell-native end-to-end smoke tests for the local Sigap
demo flow. No test framework — just `Invoke-WebRequest` calls with explicit
PASS/FAIL assertions.

## Files

- `sigap-demo-smoke.ps1` — main script; runs the documented happy path.
- `sigap-notification-smoke.ps1` — notification pipeline smoke; verifies outbox, worker dry-run, and delivery.

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

## Exit codes

| Code | Meaning |
|------|---------|
| `0`  | All smoke steps passed. Safe to treat the demo flow as green. |
| `1`  | At least one smoke step failed (network error, non-2xx HTTP, null/missing response field, or a step assertion failed). The last block of stdout lists which step(s) `[FAIL]`. |
| `2`  | Parameter validation failed. One or more of `-ApiBase`, `-FacilityShortCode`, `-ServiceUnitCode`, `-PractitionerScheduleId`, `-DevUserId` was empty or malformed. The script did not contact the API. |

Use the exit code in CI:

```powershell
pwsh -File scripts/smoke/sigap-demo-smoke.ps1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Smoke suite failed with exit code $LASTEXITCODE"
    exit $LASTEXITCODE
}
```

## Troubleshooting

The four most common failures:

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `[FAIL] parameters` — exit code `2` | `-ApiBase` empty, missing scheme, or whitespace; `-PractitionerScheduleId` not a UUID; one of the other parameters empty | Pass a valid `-ApiBase http://localhost:8080` (or set `$env:SIGAP_API_BASE`); check the script header for parameter shapes |
| `[FAIL] health` — "Network unreachable" | Go API is not running, or the wrong port | `cd apps/api; go run ./cmd/server` and confirm `:8080` is listening |
| `[FAIL] admin.facilities.list` — HTTP 403 | Dev identity is disabled in `.env` | Set `SIGAP_AUTH_MODE=dev` and `SIGAP_DEV_IDENTITY=true`, then restart the API |
| `[FAIL] public.booking` — "appointment_time is in the API's past" | Timezone/clock mismatch between your shell, the API server, and PostgreSQL | Send an explicit UTC timestamp (`...T09:00:00Z`); align clocks via NTP; see [`docs/DEMO_FLOW.md` § Troubleshooting](../../docs/DEMO_FLOW.md#troubleshooting) |

Additional notes:

- **`[FAIL] public.checkin` — Rust engine unavailable.** Start Terminal 1
  (`cd apps/queue-engine; cargo run`). The check-in step needs the gRPC
  server on `:50051`.
- **`[FAIL] public.booking` — schedule slot is full.** The demo seed gives
  you 18 bookable slots per day (2 service units × 6 slots × 3 capacity).
  If you re-run the smoke many times the same day, capacity is exhausted.
  Use `-SkipSeed` to book without capacity validation.
- **`[FAIL] public.checkin` — daily rate limit (HTTP 429).** The script
  generates a fresh random phone per run. If you re-run very rapidly on
  the same day, the per-phone limit (3/day) can still hit.
- **`[FAIL]` with `success=false` in body.** The HTTP status was 2xx but
  the API wrapper reported `success=false`. The body is printed in the
  `[FAIL]` detail; cross-reference the relevant handler in
  `apps/api/internal/handler/`.
- **All steps `[FAIL]` with the same `Network error`.** The `ApiBase`
  resolved but no service is listening. Confirm the API started cleanly
  (look for `listening on :8080` in its stdout).

## Privacy

- All patient data is synthetic. The script generates random names like
  `Pasien Demo 4711` and phones in the `+62-555-01xx` reserved-for-testing
  range.
- Dev identity is **local-only**. Do not run with `SIGAP_DEV_IDENTITY=true`
  in any shared environment.
- The script never prints JWTs, passwords, or API keys. The only
  "identifier" it logs is `DevUserId`, which is a synthetic string
  consumed by the local `DevIdentityProvider`.
