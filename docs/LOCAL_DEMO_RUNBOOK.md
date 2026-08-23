# Sigap Live Presentation Runbook

> **Bahasa Indonesia**: [LOCAL_DEMO_RUNBOOK.id.md](./LOCAL_DEMO_RUNBOOK.id.md) (synchronized translation — update both files in the same PR).

| | |
|---|---|
| **Audience** | The presenter driving the 5–10 minute live demo, and the operator keeping the stack healthy |
| **Job** | Set up, present the standardized 7-screen story, recover from common failures, and run the final pre-presentation checklist |
| **Type** | Operational how-to (runbook) |
| **Source of truth** | This file is the single canonical demo guide. `DEMO_FLOW.md` is a retired stub; `DEMO_HANDOFF.md` is a point-in-time checkpoint, not a guide. Do not add a second demo guide. |
| **Last verified** | 2026-08-23 @ main `c755042`, Windows bare-metal bootstrap |
| **Update trigger** | If ports, `Start-LocalDev.ps1`, any screen in the demo flow, seed behavior, or smoke step count changes — update this doc and its Indonesian twin **in the same PR** |

---

## What the demo shows

Sigap is a regional health information and queueing platform: patients book
appointments and check in from a phone, facility staff run the queue console,
and administrators see live facility/bed status across the region — without
real-time data plumbing being visible to anyone.

**Users:** patients (booking, check-in, status lookup), facility staff (queue
console, appointment management), regional administrators (facilities, beds,
notifications).

**Core flow:** book an appointment → receive a check-in code → check in → get a
queue number with an estimated wait → staff serve the visit → status is
verifiable publicly without exposing personal data.

**Demo-only vs production-ready:**

| Capability | Status during demo |
|---|---|
| Booking / check-in / queue APIs, validation, rate limits | Production-grade design |
| RBAC + audit events on admin routes | Production-grade design |
| PII masking & privacy-safe public status lookup | Production-grade design (synthetic data only) |
| Queue engine (Rust) with traceability signatures | Real service locally; fallback mode is demo-only |
| Dev identity headers (`X-Sigap-Dev-User-ID`) | **Demo-only** — never outside local development |
| Notification delivery provider | **Demo-only** — offline deterministic provider, nothing is actually sent |

All demo data is synthetic: names like `Pasien Demo*` and phone numbers in the
ITU-T reserved-for-fiction range `+62-555-01xx`. Never substitute real patient
data.

---

## Pre-demo setup

### 1. Bootstrap (one command)

Requires PowerShell 7+, Go, Rust+`cargo` (+`protoc`), Node 20+/pnpm, PostgreSQL,
and `psql` on PATH.

```powershell
# DATABASE_URL must be set in the calling shell (never commit it).
$env:DATABASE_URL = "postgresql://<user>:<password>@localhost:5434/sigap"
pwsh -NoProfile -File scripts/dev/Start-LocalDev.ps1
```

The script spawns three windows:

| Window | Service | Address |
|---|---|---|
| 1 | Rust queue engine (gRPC) | `127.0.0.1:50051` |
| 2 | Go API | `http://127.0.0.1:18080` |
| 3 | SvelteKit web | `http://localhost:5173` |

It also sets `SIGAP_API_BASE=http://127.0.0.1:18080` inside its own shell — if
you run smoke scripts from a *different* terminal, set it there too:

```powershell
$env:SIGAP_API_BASE = "http://127.0.0.1:18080"
```

Wait ~5–10 seconds after launch before checking health.

### 2. Verify everything is up

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health    # -> ok / status json
Invoke-RestMethod http://127.0.0.1:18080/readyz    # -> ready (DB reachable)
Invoke-WebRequest -UseBasicParsing http://localhost:5173   # -> 200 OK
```

Engine: the queue-engine window should show the gRPC server listening on
`:50051`. (If the engine cannot run, restart the API with
`SIGAP_ENGINE_FALLBACK=dev` — see Recovery.)

Ports that must be free: **5434** (PostgreSQL), **18080** (API),
**50051** (engine), **5173** (web). Check:
`netstat -ano | findstr ":18080 :50051 :5173"`.

### 3. Reset / seed (safe, idempotent)

Seeds are additive and idempotent — re-running never duplicates rows:

```powershell
psql $env:DATABASE_URL -f packages/db/seed/rbac.sql
psql $env:DATABASE_URL -f packages/db/seed/dev.sql     # facilities
psql $env:DATABASE_URL -f packages/db/seed/demo.sql    # schedules, SMOKE01, outbox
```

- All seeded data is synthetic (`+62-555-01xx`, `Pasien Demo*`). Nothing is
  sent to real recipients.
- **SMOKE01 baseline restore**: re-running `demo.sql` restores the seeded
  appointment with check-in code `SMOKE01` (status `scheduled`) and resets the
  deterministic notification outbox rows to `pending`.
- Full reset alternative: `pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1`
  seeds and runs all three smoke suites in one command (needs `DATABASE_URL`,
  `SIGAP_DATABASE_URL`, and `SIGAP_API_BASE` exported in that shell).

---

## Demo script

Synthetic presenter identity: name **Demo Rehearsal Patient**, phone
**08555001999** (reserved test range). Copy IDs into a scratchpad as you go —
screens 4–6 reuse them.

### Screen 1 — Homepage (live overview)

- **URL:** <http://localhost:5173>
- **Do:** point at the bed availability board; open DevTools → Network tab and
  highlight the `/api/v1/events/beds` event stream row (live updates without
  refresh).
- **Say:** "Every facility's bed status streams to this dashboard the moment it
  changes — this is the same live view administrators use region-wide."
- **Expected:** dashboard renders with bed counts; SSE connection stays open;
  no console errors.
- **Copy:** nothing.
- **Don't show:** network payload details, terminal windows.

### Screen 2 — Appointment booking

- **URL:** <http://localhost:5173/appointments/new>
- **Do:** pick **Sigap Demo Facility** → **Poli Umum Demo** from the dropdowns;
  date = tomorrow, time = 09:30; name `Demo Rehearsal Patient`; phone
  `08555001999`; submit.
- **Say:** "A patient picks a facility and clinic like choosing a product — no
  codes, no paperwork. The system validates capacity and returns a check-in
  code instantly."
- **Expected:** green success card showing the 6-character check-in code plus
  appointment reference.
- **Copy:** the **check-in code** and the **appointment_id**.
- **Don't show:** raw JSON responses, dev headers, UUIDs typed by hand.

### Screen 3 — Patient check-in

- **URL:** use the success card's "→ Check-In sekarang" deep link (or
  <http://localhost:5173/appointments/check-in> and paste the code).
- **Do:** confirm both fields are prefilled (appointment ID + code); click
  **Check-In**.
- **Say:** "At the door the patient checks in with one code and gets a fair
  queue number with an honest wait estimate."
- **Expected:** success panel: queue number `RSK-000x`, estimated wait (~25
  minutes), status `queued`.
- **Copy:** the **queue number** (e.g. `RSK-0002`).
- **Don't show:** the gRPC/engine internals behind ticket generation.

### Screen 4 — Patient status (privacy-safe public lookup)

- **URL:** <http://localhost:5173/patient/status>
- **Do:** look up your fresh code first (shows queue number + waiting status);
  then click "Cek kode lain" and enter **SMOKE01** (shows the seeded
  appointment still *Belum check-in*).
- **Say:** "Anyone holding a code can verify their visit state — and only that.
  No names, no phone numbers, no medical record leaves the building."
- **Expected:** clean status cards for both codes; zero personal data rendered.
- **Copy:** nothing.
- **Don't show:** API response bodies (keep the privacy claim visual).

### Screen 5 — Admin queue console

- **URL:** <http://localhost:5173/admin/queues>
- **Do:** find your ticket (`RSK-000x`, badge **Menunggu**). Click **→ Dipanggil**
  (patient called to the counter), then **→ Dilayani** (being served). Press
  **Muat** or reload the page and show the state persisted.
- **Say:** "Staff move patients through the visit with one click. Every change
  is validated against allowed transitions and written to an audit trail."
- **Expected:** badges progress Menunggu → Dipanggil → Dilayani; state survives
  reload.
- **Copy:** nothing.
- **Don't show:** cancel/skip paths, DB tooling.

### Screen 6 — Admin appointments

- **URL:** <http://localhost:5173/admin/appointments>
- **Do:** find today's booking ("Demo Rehearsal Patient", badge **Antre**);
  click **Selesai**; press **Muat ulang** to prove persistence. Optionally use
  the status filter to show list narrowing.
- **Say:** "The appointment lifecycle mirrors reality end-to-end — scheduled,
  checked in, queued, completed."
- **Expected:** badge flips to **Selesai** and stays after reload.
- **Copy:** nothing.
- **Don't show:** Batal/Tidak Hadir flows (avoid implying patient-facing blame).

### Screen 7 — Admin facilities (overview only)

- **URL:** <http://localhost:5173/admin/facilities>
- **Do:** scroll briefly; mention capacity editing exists.
- **Say:** "Facility registry powers everything upstream — beds on the
  dashboard, dropdowns at booking, queues per site."
- **Expected:** facility list loads.
- **Copy:** nothing. **Do not perform CRUD operations live.**

---

## Recovery steps

| Failure | Action |
|---|---|
| **API unavailable** (health fails) | Check the API window for a crash. Restart stack: close spawned windows, re-run `scripts/dev/Start-LocalDev.ps1`, wait ~10 s, re-check `/health` and `/readyz`. |
| **Engine unreachable** (check-in fails with queue error) | Restart engine window (`cd apps/queue-engine; cargo run`). If it can't start, kill API window, set `$env:SIGAP_ENGINE_FALLBACK = "dev"`, relaunch via `Start-LocalDev.ps1` — check-in then works via database-backed fallback. |
| **Port conflict** (window exits immediately) | `netstat -ano | findstr ":<port>"` for 5434/18080/50051/5173; stop the conflicting process or free the port, then relaunch that service. |
| **Stale browser state** (old UI after code changes) | Hard reload: Ctrl+F5 / DevTools right-click reload clearing cache; reopen `http://localhost:5173/appointments/new`. |
| **Queue already transitioned** (no Menunggu button) | Pick another ticket still showing **Menunggu**, or create a fresh one: book again (Screen 2) and check in (Screen 3). Transitions are one-way by design. |
| **No waiting ticket** | Same as above — book + check in a fresh synthetic appointment; takes under a minute. |
| **Seed drift** (SMOKE01 unknown, odd facility/unit lists) | Re-run the three seed commands from [Reset / seed](#3-reset--seed-safe-idempotent); `demo.sql` self-heals demo service-unit linkage and restores SMOKE01. Then hard-reload the browser. |
| **Admin pages 401/403 / dev identity missing** | Admin routes need dev identity enabled at API start: ensure the API window was launched by `Start-LocalDev.ps1` (sets `SIGAP_AUTH_MODE=dev` + `SIGAP_DEV_IDENTITY=true`). Restart the stack if the API was started manually without them. |
| **Booking says time must be in the future** | Use tomorrow's date; keep shell/API/PostgreSQL clocks NTP-synced. |
| **Smoke script hits port 8080** | Pass `-ApiBase http://127.0.0.1:18080` (or export `SIGAP_API_BASE`) — the bare-metal runtime uses 18080, not the script default. |

---

## Final validation checklist (immediately before presenting)

Run top to bottom; stop and fix anything unchecked:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/health          # 200
Invoke-RestMethod http://127.0.0.1:18080/readyz           # 200
$env:SIGAP_API_BASE = "http://127.0.0.1:18080"
pwsh -NoProfile -File scripts/smoke/sigap-demo-smoke.ps1  # 8/8 PASS
```

- [ ] `/health` returns 200
- [ ] `/readyz` returns 200
- [ ] Homepage loads (<http://localhost:5173>) with bed dashboard
- [ ] Booking works (dropdowns populate; fresh code returned)
- [ ] Check-in works (queue number issued)
- [ ] Patient status works (fresh code + `SMOKE01`)
- [ ] Queue admin works (list + transitions)
- [ ] Appointment admin works (list + one transition)
- [ ] Smoke suite: 8/8 PASS
- [ ] Browser console: no errors (only a harmless favicon 404 may appear)

Optional deeper gate: `sigap-full-local-demo.ps1` → expect
**FULL LOCAL DEMO: PASS**.

---

## Further reading

- [`LOCAL_DEMO_RUNBOOK.id.md`](./LOCAL_DEMO_RUNBOOK.id.md) — Bahasa Indonesia twin
- [`../scripts/smoke/README.md`](../scripts/smoke/README.md) — smoke parameters, exit codes
- [`DEV_SETUP.md`](./DEV_SETUP.md) — full developer setup and auth modes
- [`DEMO_HANDOFF.md`](./DEMO_HANDOFF.md) — latest green checkpoint record
