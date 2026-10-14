# Sigap Smoke Suite

Lightweight, PowerShell-native end-to-end smoke tests for the local Sigap
demo flow. No test framework — just `Invoke-WebRequest` calls with explicit
PASS/FAIL assertions.

## Files

- `sigap-demo-smoke.ps1` — main script; runs the documented happy path.
- `sigap-notification-smoke.ps1` — notification pipeline smoke; verifies outbox, worker dry-run, and delivery.
- `sigap-patient-portal-smoke.ps1` — patient portal smoke; validates public status lookup API.
- `sigap-production-readiness-smoke.ps1` — post-deploy readiness (Phase 3B7): the web routes (`/`, `/faskes`, `/queues/new`, `/admin`, `/appointments/new`, `/appointments/check-in`, `/patient/status`), the public catalog, and the two P0 proofs (out-of-scope notification retry → 404 **and** no row write; zero-assignment actor → all-zero summary, with the global super_admin contrast). Requires a loopback API + web target and the local test identity selector armed.
- `sigap-full-local-demo.ps1` — orchestrates seed + smoke in one command; runs all three seeds then all four smoke suites.

> **Every smoke target is loopback-only.** `sigap-production-readiness-smoke.ps1`
> refuses any non-loopback `-ApiBase`/`-WebBase` **and** a non-loopback `-DatabaseUrl`
> before making a request or running any DML (the same allow-list as
> `apps/web/playwright.config.ts`). Override the database check only for an explicitly
> authorized staging rehearsal, with `-AllowNonLocalDatabase`. Smoke runs never target
> production.

## Full local demo — one command

`sigap-full-local-demo.ps1` runs the complete local demo readiness suite:
seeds the database (dev, rbac, demo) then runs all four smoke suites.

### When to use

- Before merging a feature branch that touches the API, queue engine, or
  notification pipeline.
- After a fresh `git clone` or database reset to verify the stack works
  end-to-end.
- As a pre-push sanity check (faster than full CI).

### Required environment variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `DATABASE_URL` | yes | PostgreSQL connection string for psql seed commands |
| `SIGAP_DATABASE_URL` | yes (worker) | Same as `DATABASE_URL`; used by the notification worker |
| `SIGAP_AUTH_MODE` | yes | Set to `dev` for local dev-identity auth |
| `SIGAP_DEV_IDENTITY` | yes | Set to `true` to enable `X-Sigap-Dev-User-ID` header |
| `SIGAP_ENGINE_FALLBACK` | Mode A only | Set to `dev` to skip the Rust queue engine |
| `SIGAP_API_BASE` | no | API base URL (default `http://127.0.0.1:8080`) |
| `SIGAP_WEB_BASE` | prod-readiness suite | Web origin (default `http://127.0.0.1:4173`) |
| `SIGAP_LOCAL_RBAC_TEST_IDENTITY` | prod-readiness suite | Set to `true` (with `SIGAP_ENV=local`) to arm the local test identity selector the P0 proofs need |

> **Restart-safe**: Env vars are shell-scoped. After a terminal restart or
> new shell, re-export them. If `psql` prompts for a user/password,
> `DATABASE_URL` is not set.

> **Seed idempotency**: All seed files (`dev.sql`, `rbac.sql`, `demo.sql`)
> are idempotent. Re-running them does not create duplicate facilities,
> roles, or demo data.

The Sigap API **must already be running** before you start the script.
The script does not start or stop any services.

### Command

```powershell
pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1
```

Skip re-seeding (use when DB is already seeded):

```powershell
pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1 -SkipSeed
```

### What it runs

| Phase | Step | Description |
|-------|------|-------------|
| Seed | `dev.sql` | Facilities, service units, synthetic dev user |
| Seed | `rbac.sql` | Roles, permissions, role_permissions |
| Seed | `demo.sql` | Schedules, demo appointments, notification outbox |
| Smoke | `sigap-demo-smoke.ps1` | 8-step booking / check-in / queue flow |
| Smoke | `sigap-notification-smoke.ps1` | 9-step notification pipeline |
| Smoke | `sigap-patient-portal-smoke.ps1` | 5-step public status lookup |
| Smoke | `sigap-production-readiness-smoke.ps1` | web routes + admin mutations + the two P0 proofs (16 checks) |

The script fails fast: any non-zero exit code stops the run immediately.

> **The production-readiness suite needs more than the API.** It additionally requires:
> a **web preview** on `127.0.0.1:4173` (or `-WebBase` / `$env:SIGAP_WEB_BASE`), the API
> started with **`SIGAP_ENV=local` + `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`** (so the local
> test identity selector resolves the P0 subjects; `Start-LocalE2E.ps1` arms both), and
> `DATABASE_URL` (for the disposable probe rows and the row snapshots). If the selector is
> not armed the suite exits **2** with a clear message rather than reporting false failures.
>
> The precondition requires HTTP **200** on the probe. An unarmed API fails closed with
> **403** (`authz.go` deny-by-default) because the provider returns a zero actor on every
> failure path; 403 therefore means "not armed" and aborts before any fixture is created.

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | All seeds and smoke suites passed |
| `1` | DATABASE_URL not set, a seed command failed, or a smoke suite failed |

## Quick Usage

```powershell
# Demo smoke — full happy path (6 steps)
pwsh -File scripts/smoke/sigap-demo-smoke.ps1

# Notification pipeline smoke — outbox, worker dry-run, delivery (9 steps)
pwsh -File scripts/smoke/sigap-notification-smoke.ps1

# Patient portal smoke — public status lookup API (5 steps)
pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1
```

Override the API base if needed:

```powershell
$env:SIGAP_API_BASE = 'http://127.0.0.1:8080'
pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1
```

See the detailed sections below for parameters, exit codes, and troubleshooting.

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
| `-ApiBase` | `http://127.0.0.1:8080` (or `$env:SIGAP_API_BASE`) | API root (IPv4 loopback) |
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
| `[FAIL] public.checkin` — "Gagal mengambil nomor antrean" | Queue engine unavailable and fallback not enabled | Start the engine (`cd apps/queue-engine; cargo run`) **or** restart the API with `SIGAP_ENGINE_FALLBACK=dev` |
| `[FAIL] public.booking` — "appointment_time is in the API's past" | Timezone/clock mismatch between your shell, the API server, and PostgreSQL | Send an explicit UTC timestamp (`...T09:00:00Z`); align clocks via NTP; see [`docs/DEMO_FLOW.md` § Troubleshooting](../../docs/DEMO_FLOW.md#troubleshooting) |
| `psql` prompts for user/password | `DATABASE_URL` is not set in the current shell | Re-export `$env:DATABASE_URL` (env vars are lost on terminal restart) |

Additional notes:

- **`[FAIL] public.checkin` — Rust engine unavailable.** Either start
  Terminal 1 (`cd apps/queue-engine; cargo run`) or restart the API with
  `SIGAP_ENGINE_FALLBACK=dev`. Fallback mode now creates a real queue
  ticket in the database, so all downstream steps work.
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

---

## Notification smoke

`sigap-notification-smoke.ps1` exercises the notification pipeline
end-to-end: API summary/listing, worker dry-run (no mutation), and
worker once-mode delivery.

### Prerequisites

- PowerShell 7+ (`pwsh`).
- A running Sigap API at `http://127.0.0.1:8080` (or `$env:SIGAP_API_BASE`).
  On bare-metal Windows, use `127.0.0.1` rather than `localhost` to avoid
  IPv6 loopback resolution; Windows IP Helper service may occupy `0.0.0.0:8080`.
- PostgreSQL running with `$env:SIGAP_DATABASE_URL` set.
- Dev seed loaded (`psql $DATABASE_URL -f packages/db/seed/dev.sql`)
  — creates the demo facilities (short_codes: `RSK`, `PKM`, `RSM`,
  `PMI`, `RSJ`, `PHB`). The notification smoke rows insert against
  the first facility found.
- Demo seed loaded (`psql $DATABASE_URL -f packages/db/seed/demo.sql`)
  — this seeds 2 pending `notification_outbox` rows.
- Go 1.22+ available for `go run ./cmd/notification-worker`.
- Dev identity enabled (`SIGAP_AUTH_MODE=dev`, `SIGAP_DEV_IDENTITY=true`).
- **No Rust queue engine required.**

### Quickstart

```powershell
# Terminal 1: start the API
cd apps/api
go run ./cmd/server

# Terminal 2: load seeds and run the notification smoke
psql $env:DATABASE_URL -f packages/db/seed/dev.sql
psql $env:DATABASE_URL -f packages/db/seed/demo.sql
pwsh -File scripts/smoke/sigap-notification-smoke.ps1
```

Expected output: `Passed: 9 / 9` and exit code `0`.

### What it covers

| # | Step | Auth | Description |
|---|------|------|-------------|
| 1 | `GET /health` | none | API is up |
| 2 | `GET /api/v1/admin/facilities` | dev | Dev identity works, obtain facility_id |
| 3 | `GET /api/v1/admin/notifications/summary` | dev | Snapshot pending count before worker |
| 4 | `GET /api/v1/admin/notifications?status=pending` | dev | Verify seeded pending rows exist |
| 5 | Worker dry-run | — | `DRY_RUN=true ONCE=true` subprocess; parses slog output |
| 6 | Summary re-check | dev | Pending count unchanged after dry-run |
| 7 | Worker once-mode | — | `ONCE=true` subprocess; real delivery |
| 8 | Summary after | dev | Pending decreased or delivered/failed increased |
| 9 | List after | dev | Delivered or failed rows exist |

### Parameters

```powershell
pwsh -File scripts/smoke/sigap-notification-smoke.ps1 `
    -ApiBase 'http://127.0.0.1:8080' `
    -DevUserId 'dev-user-smoke' `
    -WorkerDir 'apps\api'
```

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `-ApiBase` | `http://127.0.0.1:8080` (or `$env:SIGAP_API_BASE`) | API root |
| `-DevUserId` | `dev-user-smoke` | Value of `X-Sigap-Dev-User-ID` header |
| `-WorkerDir` | `apps\api` (relative to script) | Go module root for `cmd/notification-worker` |

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | All 9 notification smoke steps passed. |
| `1` | At least one step failed. The summary block lists `[FAIL]` steps. |
| `2` | Parameter validation failed. |

### Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `[FAIL] api.health` — "connection forcibly closed" or timeout | Port conflict on Windows | Use `http://127.0.0.1:18080` and start the API with `SIGAP_API_PORT=18080`. Windows IP Helper service (`iphlpsvc`) or WSL relay may occupy port 8080. |
| `[FAIL] api.health` — "Network unreachable" | Go API not running | `cd apps/api; go run ./cmd/server` |
| `[FAIL] dev.identity` — HTTP 403 | Dev identity disabled | Set `SIGAP_AUTH_MODE=dev` and `SIGAP_DEV_IDENTITY=true` in `.env` |
| `[FAIL] notification.list.before` — 0 rows | Demo seed not loaded | `psql $DATABASE_URL -f packages/db/seed/demo.sql` |
| `[FAIL] worker.dry_run` — build error | Go not installed or `WorkerDir` wrong | Install Go 1.22+; pass correct `-WorkerDir` |
| `[FAIL] worker.dry_run` — timeout | Worker hung or DB unreachable | Check `$env:SIGAP_DATABASE_URL` is set and PostgreSQL is running |
| `[FAIL] notification.summary.after` — unchanged | Worker processed 0 rows | Check `SIGAP_DATABASE_URL` in the worker process; verify outbox has due rows |

### Privacy

- This script never prints `recipient_contact_masked`,
  `recipient_contact_hash`, `subject`, `body_template`, raw phone
  numbers, emails, or rendered notification bodies.
- Only counts, UUIDs, status strings, and slog-parsed counters are
  displayed.
- Dev identity is **local-only**; never enable in shared environments.

---

## Patient portal smoke

`sigap-patient-portal-smoke.ps1` validates the public patient status
lookup endpoint (`GET /api/v1/patient/status`) using synthetic demo data.
No authentication required — the endpoint is public.

### Prerequisites

- PowerShell 7+ (`pwsh`).
- A running Sigap API at `http://127.0.0.1:8080` (or `$env:SIGAP_API_BASE`).
- Demo seed loaded (`psql $DATABASE_URL -f packages/db/seed/demo.sql`)
  — creates a deterministic appointment with `checkin_code = 'SMOKE01'`.
- **No Rust queue engine required.**
- **No dev identity required** (public endpoint).

### Quickstart

```powershell
# Terminal 1: start the API
cd apps/api
go run ./cmd/server

# Terminal 2: load demo seed and run the patient portal smoke
psql $env:DATABASE_URL -f packages/db/seed/demo.sql
pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1
```

Expected output: `Passed: 5 / 5` and exit code `0`.

### What it covers

| # | Step | Auth | Description |
|---|------|------|-------------|
| 1 | `GET /health` | none | API is up |
| 2 | `GET /api/v1/patient/status?code=SMOKE01` | none | Valid lookup returns 200 with `found_by=checkin_code` |
| 3 | `GET /api/v1/patient/status?code=<script>` | none | Invalid characters return 400 |
| 4 | `GET /api/v1/patient/status?code=ZZZZZXXXXX999` | none | Unknown code returns 404 |
| 5 | PII absence check | none | Response body does not contain forbidden PII field names |

### Parameters

```powershell
pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1 `
    -ApiBase 'http://127.0.0.1:8080'
```

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `-ApiBase` | `http://127.0.0.1:8080` (or `$env:SIGAP_API_BASE`) | API root |

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | All 5 patient portal smoke steps passed. |
| `1` | At least one step failed. |
| `2` | Parameter validation failed. |

### Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `[FAIL] api.health` — "Network unreachable" | Go API not running | `cd apps/api; go run ./cmd/server` |
| `[FAIL] patient.status.valid_lookup` — 404 | Demo seed not loaded | `psql $DATABASE_URL -f packages/db/seed/demo.sql` |
| `[FAIL] patient.status.invalid_code` — not 400 | Input validation not applied | Ensure latest code is running (rebuild API) |

### Privacy

- This script never prints patient data, phone numbers, or any PII.
- Only HTTP status codes, boolean flags, and field-name presence
  checks are displayed.

## Production readiness smoke

Post-deploy / pre-production verification for Phase 3B7 (T-3B7-02). Covers the web routes,
the public catalog, the two admin mutations, and the two P0 security proofs.

> **Rehearsal only.** It refuses any non-loopback `-ApiBase`/`-WebBase` (exit 2, before a
> request) and it creates/removes disposable probe rows and a disposable facility. It is for
> a production-equivalent **local/staging** stack, never a live production deployment. See
> `docs/operations/DEPLOYMENT_RUNBOOK.md` §4a/§4b.

### Prerequisites

- A running production-equivalent local stack: real Go API, real Postgres, real web preview,
  real Rust engine.
- The API started with `SIGAP_ENV=local` **and** `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`, so the
  local test identity selector resolves the P0 subjects. `scripts/dev/Start-LocalE2E.ps1`
  arms both.
- `DATABASE_URL` set (probe fixtures + whole-row snapshots).
- The seed loaded (`dev.sql`, `rbac.sql`, `demo.sql`) so the seeded subjects and the demo
  facility exist.

### Quickstart

```powershell
# Bring up the stack (arms SIGAP_ENV=local + the local selector).
pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1 -SkipEngineBuild -KeepRunning

$env:SIGAP_API_BASE = 'http://127.0.0.1:18080'
$env:SIGAP_WEB_BASE = 'http://127.0.0.1:4173'
$env:DATABASE_URL   = 'postgresql://sigap:sigap@127.0.0.1:55433/sigap_e2e?sslmode=disable'

pwsh -NoProfile -File scripts/smoke/sigap-production-readiness-smoke.ps1
```

### What it covers

| Group | Checks |
|-------|--------|
| Web routes | `/`, `/faskes`, `/queues/new`, `/admin`, `/appointments/new`, `/appointments/check-in`, `/patient/status` → 200 + a stable SSR marker |
| Public catalog | `GET /api/v1/public/facilities` → 200 with a non-empty `data` array |
| P0-1 (positive control) | in-scope retry → 200 **and** the row mutates |
| P0-1 (denial) | cross-facility retry → 404 **and** the whole row is unchanged |
| P0-3 | zero-assignment non-super_admin → all five declared status keys present and zero |
| §4.5 contrast | DB-resolved active global super_admin → non-zero global counts |
| Admin mutations | queue `waiting → called` persists; disposable facility deactivate sets `is_active=false` |

The positive control is what makes the 404 meaningful: without it, a 404 from a *missing*
endpoint would look identical to a 404 from a *denied* scope.

### Parameters

| Parameter | Default | Purpose |
|-----------|---------|---------|
| `-ApiBase` | `$env:SIGAP_API_BASE` or `http://127.0.0.1:8080` | API root (loopback only) |
| `-WebBase` | `$env:SIGAP_WEB_BASE` or `http://127.0.0.1:4173` | Web origin (loopback only) |
| `-DatabaseUrl` | `$env:DATABASE_URL` | psql connection for probe fixtures + snapshots |
| `-ZeroScopeSubject` | `local-zero-scope-admin` | Zero-assignment actor (P0-3) |
| `-GlobalSuperAdminSubject` | `local-global-super-admin` | Global super_admin (§4.5 contrast) |
| `-ScopedSubject` | `local-facility-admin` | Facility-scoped actor (P0-1 + admin mutations) |
| `-ScopedFacilityId` | `…d000` | Facility the scoped subject manages |
| `-OtherFacilityId` | `…e000` | A different facility (the cross-facility target) |

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | All checks passed |
| `1` | At least one assertion failed |
| `2` | Parameter/precondition failure: non-loopback target, missing `DATABASE_URL`, or the local identity selector is not armed |

### Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `[FAIL] parameters` — "must be a LOOPBACK host" | Target pointed at a non-loopback host | Use `127.0.0.1`/`localhost`; this script never runs against production |
| exit `2` — "local test identity selector is not armed" | API not started with `SIGAP_ENV=local` + `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`, **or** the subject is unresolvable. The probe answers 403 (fail-closed), not 401 | Start via `Start-LocalE2E.ps1`, or set both and restart the API |
| `[FAIL] fixtures.create` | `DATABASE_URL` wrong, or the schema is not migrated | Check the DSN; apply `packages/db/migrations` |
| `[FAIL] p0.retry.in_scope.status` | the seeded demo facility or the scoped subject is missing | Load `packages/db/seed/dev.sql` + `demo.sql` |
| all `web.route.*` `[FAIL]` | no web preview on `-WebBase` | Start the preview (`Start-LocalE2E.ps1`) or set `SIGAP_WEB_BASE` |

### Privacy

- Never prints recipient contacts or hashes; probe rows use a fixed masked placeholder.
- Probe rows and the disposable facility are removed in a `finally` block, and the script
  reports how many remain (expected: 0).
