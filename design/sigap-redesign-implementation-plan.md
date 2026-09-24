# SIGAP Redesign Implementation Plan (Phase 3A)

Status: Phase 3A planning artifact. PLAN ONLY. No production code was implemented.

Companion to `design/sigap-redesign-implementation-handoff.md` (the frozen design contract) and
`design/signap-phase1-ux-audit.md` (UX audit and owner decisions D1-D14, R3-F1, R3-F2).

Source of truth precedence applied throughout this document:

1. CURRENT production source (`apps/api`, `apps/web`, `apps/queue-engine`, `packages/db`) wins for backend truth.
2. Frozen design (`design/generated/**`, `design/DESIGN.md`) remains authoritative for intended UX.
3. Where the handoff conflicts with current source, the conflict is recorded in Section 2 and corrected in the handoff.

Verification method: every implementation-critical contract below was re-read directly from source during
Phase 3A. File paths and line numbers are cited so each claim can be re-verified. Nothing was inferred from
the handoff document.

---

## 1. Verified Repository Baseline

Inspection only. No files were modified during this inspection.

### 1.1 Git state

| Item | Verified value |
|---|---|
| Current branch | `design/ui-ux-overhaul` |
| HEAD commit | `6d7f940` deploy: make production origin env-driven and route Sigap through the shared edge (#83) |
| Recent history | `ba2cb4c` fix(web): wire auth forms to default actions so POST reaches handlers not 404 (#82); `1e0cfdb` fix(deploy): bind prod backend ports to loopback only (#81) |
| Relationship to main | 5 commits ahead of `origin/main`, 0 behind (fast-forward-able) |
| Uncommitted changes | `M .gitignore`; `?? design/` (all design artifacts untracked) |

`.gitignore` diff (`+5 -1`): removed `.kiro/`, added `.byNara/`, and added:

```
# Design working area: specs are version-controlled; only generated/tmp artifacts are ignored
design/generated/
design/tmp/
```

### 1.2 `design/generated` tracking verification

| Check | Result |
|---|---|
| `git check-ignore -v design/generated/sigap-warga-desktop/pages/beranda.html` | Matches `.gitignore:49:design/generated/` |
| `design/*.md` (specs) ignored? | NO. Intended to be tracked per owner decision D1. Currently untracked only because nothing in `design/` has been added yet. |
| `design/system/**` ignored? | NO. Tracked-eligible. |
| Verdict | Generated artifacts are ignored as intended; specs are trackable as intended. **No `.gitignore` change is required by Phase 3A.** The pending `.gitignore` modification is pre-existing work, not created by this phase. |

### 1.3 Frontend stack (`apps/web/package.json`)

| Package | Version | Notes |
|---|---|---|
| `@sveltejs/kit` | `^2.5.0` | SvelteKit 2 |
| `svelte` | `^5.0.0` | Svelte 5 (runes available; existing pages already use `$state`/`$derived`) |
| `@sveltejs/adapter-node` | `^3.0.0` | Node server output in `build/` |
| `@sveltejs/vite-plugin-svelte` | `^3.0.0` | |
| `vite` | `^5.2.0` | |
| `typescript` | `^5.4.5` | |
| `svelte-check` | `^3.8.0` | CI gate |
| `tailwindcss` | `^3.4.4` | Tailwind v3 (NOT v4) |
| `@supabase/ssr` | `^0.12.6` | SSR cookie session |
| `@supabase/supabase-js` | `^2.116.0` | |
| `maplibre-gl` | `^5.24.0` | Used only by the legacy `ReferralMap` demo component |

Scripts: `dev`, `build`, `preview`, `check` (`svelte-kit sync && svelte-check`), `test`
(`sync && svelte-check && node tests/build-verification.test.js && vite build && node tests/auth-actions.test.js`).

### 1.4 Backend and engine

| Component | Verified |
|---|---|
| Go module | `github.com/sigap/sigap/apps/api`, `go 1.26.6` |
| Go deps | `golang-jwt/jwt/v5 v5.3.1`, `google/uuid v1.6.0`, `jackc/pgx/v5 v5.10.0`, `grpc v1.82.1`, `protobuf v1.36.11` |
| Go entrypoints | `cmd/server`, `cmd/bootstrap`, `cmd/ci-migrate`, `cmd/notification-worker` |
| Rust engine | `sigap-queue-engine` v0.1.0, edition 2021; `tonic 0.12`, `prost 0.13`, `sqlx 0.8`, `tokio 1`, `sha2 0.10` |
| Proto | `protos/sigap/queue_engine.proto`; generated Go stubs committed at `apps/api/internal/pb/queueenginepb/` |
| Engine tests | `tests/concurrency_guardrail.rs`, `tests/estimated_wait_regression.rs` |

### 1.5 Database migrations (`packages/db/migrations`)

`0001_init.sql`, `0002_medical_records.sql`, `0003_identity_rbac.sql`, `0004_audit_events.sql`,
`0005_appointments.sql`, `0006_notifications.sql`, `0007_checkin_constraints.sql`,
`0008_identity_subject.sql`, `0009_user_roles_lifecycle.sql`, `0010_demo_notes_column.sql`.

Verified enum/constraint anchors used by this plan:

- `0001_init.sql`: `CREATE TYPE queue_status AS ENUM ('waiting','called','in_service','completed','cancelled','skipped')`;
  `CREATE TYPE facility_type AS ENUM ('rumah_sakit','puskesmas')`; `facilities.is_active BOOLEAN NOT NULL DEFAULT TRUE`; `facilities.short_code TEXT NOT NULL`.
- `0005_appointments.sql`: `CREATE TYPE appointment_status AS ENUM ('scheduled','checked_in','queued','completed','cancelled','no_show')`;
  `appointments.status ... DEFAULT 'scheduled'`; schedule CHECK constraints (`end_time > start_time`, `slot_minutes > 0`, `capacity_per_slot > 0`).

Seeds: `packages/db/seed/{dev,rbac,demo}.sql`. `make db-seed` refuses to run unless `SIGAP_ENV=local`.

### 1.6 Tests present today

| Area | Artifact |
|---|---|
| Go | ~30 `_test.go` files incl. `handler/admin_test.go`, `handler/booking_test.go`, `handler/booking_checkin_test.go`, `handler/queue_test.go`, `handler/facility_scope_test.go`, `handler/patient_test.go`, `auth/facility_scope_test.go`, `auth/rbac_resolver_test.go`, `identity/authz_test.go`, `router/router_test.go`, `router/proxy_test.go`, `notification/*_test.go` |
| Rust | 2 integration tests (concurrency guardrail, estimated-wait regression) |
| Web | `tests/build-verification.test.js` (static source assertions + build artifact presence), `tests/auth-actions.test.js` (spawns built server against a mock Supabase, exercises login/register/logout actions) |

**No browser/E2E automation exists today.** No Playwright dependency. No component test runner. This is a
gap Section 13 addresses.

### 1.7 CI workflows (`.github/workflows/ci.yml`)

Six jobs, all triggered on push/PR to `main`:

| Job | Gate |
|---|---|
| `api` | Postgres 16 service; runs `cmd/ci-migrate`; `go test ./... -coverprofile`; **asserts `TestRBACResolver`, `TestFacilityScope`, `TestCheckIn` actually ran and did not skip**; **router coverage floor 90%** |
| `engine` | `cargo test`; `cargo clippy -- -D warnings` (non-blocking) |
| `web` | pnpm install `--frozen-lockfile`; `pnpm --filter sigap-web run check` |
| `security-go` | `govulncheck ./...` (blocking) |
| `security-rust` | `cargo audit` (non-blocking) |
| `security-secrets` | gitleaks (blocking, full history) |

Two consequences that constrain this plan:

1. The `api` job's explicit grep assertions mean **any new DB-backed security test must be named so it can be
   proven to run**, and the existing names must not be renamed.
2. The router coverage floor is 90%. Any new route added to `router.Registry` must ship with tests that keep
   `internal/router` coverage at or above 90%.

Also present: `.github/CODEOWNERS`, `pull_request_template.md`, `ISSUE_TEMPLATE/bug_report.md`,
`Makefile` (dev/db-migrate-all/db-seed/bootstrap/build/test/lint/security), `docker-compose*.yml`,
`deploy/systemd/` backup units.

### 1.8 Production surface

`https://sigap.chaerulchalik.web.id/` is live. Production deployment is docker-compose based with an
env-driven origin (`6d7f940`) and backend ports bound to loopback (`1e0cfdb`). Phase 3A did not touch,
deploy, or reconfigure any of it.

### 1.9 Baseline verdict

The worktree is clean apart from the pre-existing `.gitignore` edit and the untracked `design/` tree.
No production code, backend behavior, migration, or deployment state was altered by Phase 3A.
Planning can proceed from this baseline without a cleanup step.

---

## 2. Handoff Discrepancies Resolved

Every row was verified against current source. "Handoff said" quotes the frozen contract verbatim.

### D-1 (BLOCKER) Appointment status transition map is wrong

| | |
|---|---|
| Handoff said (Section 5) | `scheduled -> checked_in/cancelled/no_show; checked_in or queued -> completed/cancelled/no_show` |
| Current source | `apps/api/internal/handler/admin.go:1950-1958` `validAppointmentTransitions` |
| Actual | `scheduled -> {checked_in, cancelled, no_show}`; `checked_in -> {queued, cancelled, no_show}`; `queued -> {completed, cancelled, no_show}`; `completed -> {}`; `cancelled -> {}`; `no_show -> {}` |
| Discrepancy | The handoff omits `queued` as a destination of `checked_in`, and incorrectly implies `checked_in -> completed` is allowed. It is not. `completed` is reachable only from `queued`. |
| Resolution | Handoff corrected to the exact map. See Section 4.2 for the full state machine and the separate public check-in flow. |

### D-2 (BLOCKER) Admin invalid transitions return 400, not 409

| | |
|---|---|
| Handoff said (Section 6) | Admin Antrean: "Invalid transition returns conflict state"; Admin Janji Temu: "Unsupported transition shown as conflict, not success" |
| Current source | `admin.go:740-746` (queue) and `admin.go:2117-2121` (appointment) both call `writeError(w, http.StatusBadRequest, ...)` |
| Actual | **400 Bad Request**, body `{"success":false,"error":"Transisi status tidak valid: <from> -> <to>."}` for queues, and `{"success":false,"error":"Transisi status '<from>' -> '<to>' tidak diizinkan."}` for appointments (the appointment message embeds a Unicode right arrow) |
| Discrepancy | The handoff's "conflict" framing would lead implementers to build a 409 conflict state that never occurs. |
| Resolution | Handoff state matrix corrected: invalid transitions are a **validation (400) state**, not a conflict (409) state. 409 is reserved for check-in races and notification retry/cancel invalid state. |

### D-3 (BLOCKER) Admin authentication failure returns 403, not 401

| | |
|---|---|
| Handoff said (Sections 6, 10) | Admin screens list generic "401/403" |
| Current source | `apps/api/internal/identity/authz.go:70-80` (missing actor) and `:82-92` (insufficient permission) both call `writeAuthzError(w, http.StatusForbidden, ...)` |
| Actual | Missing or invalid authentication -> **403** `{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}`. Authenticated but unauthorized -> **403** `{"success":false,"error":"Akses ditolak: izin tidak mencukupi."}`. **401 is never produced by the authorization layer.** |
| Where 401 does occur | `router.DenyByDefault` (`router.go:109`) for an undeclared method+path: 401 `{"success":false,"error":"Akses ditolak: rute tidak dikenali atau memerlukan otorisasi."}`. And the public check-in code-mismatch case: `booking.go:516` returns 401 `"Kode check-in tidak cocok."` |
| Discrepancy | The handoff implies a distinguishable 401 unauthenticated state on Admin screens. There is none. |
| Resolution | Handoff matrix corrected to 403 for both cases, with the explicit implementation consequence recorded in Section 4.3: **the UI cannot distinguish unauthenticated from unauthorized by status code alone.** |

### D-4 (BLOCKER) Walk-in production route was undefined

| | |
|---|---|
| Handoff said (Section 3) | Ambil Antrean Tanpa Janji -> "/ existing queue entry or approved Check-In entry point ... Confirm final production entry route during implementation planning." |
| Current source | No citizen walk-in route exists. Citizen routes are `/`, `/appointments/new`, `/appointments/check-in`, `/patient/status`, `/auth/{login,register,logout}`, `/wallet`. |
| Resolution | Resolved in Section 4.1. `WALK_IN_ROUTE = /queues/new`. |

### D-5 Notification summary empty-scope fallback is a data leak, not just an inconsistency

| | |
|---|---|
| Handoff said (Section 10, P0-3) | "Notification summary behavior for empty scope/global fallback must be made consistent with list and mutation scope rules." |
| Current source | `apps/api/internal/handler/notifications.go:188-213`; `apps/api/internal/notification/service.go:227-251` |
| Actual | On scope-resolution error the handler returns `200 {"success":true,"data":{}}`. On a **valid but empty** scope (`allowedFacilities` is a non-nil empty slice) the `if allowedFacilities != nil` guard is entered, no facility matches, so `facilityFilter` stays `uuid.Nil`, and `Summary(ctx, uuid.Nil)` counts **across all facilities platform-wide** (`service.go:228-233` `WHERE ($1 = '000...0' OR facility_id = $1::uuid)`). |
| Discrepancy | Described as an "inconsistency". It is an authorization bypass: a zero-assignment **non-super_admin** actor receives platform-wide notification counts (after the Section 4.5 decision, a global `super_admin` legitimately receives global counts via its explicit unrestricted scope; the leak is the extension of that behavior to every empty-scope actor). |
| Resolution | Reclassified as a P0 security blocker with an exact exploit path (Section 5, P0-4). |

### D-6 (NEW, not in handoff) Systemic fail-open scope predicate on detail and mutation endpoints

| | |
|---|---|
| Handoff said | Nothing. Only "zero-assignment facility scope behavior is inconsistent" (Section 10, P0-2). |
| Current source | `admin.go` lines `190, 362, 409, 658, 999, 1202, 1459, 1693` all use the pattern `if len(allowedFacilities) > 0 { query += " AND facility_id = ANY($n)" }` |
| Actual | When a non-dev actor has **zero** facility assignments, the predicate is **omitted entirely**, so the query is unrestricted and operates on any row by ID. List endpoints correctly short-circuit to an empty result (`admin.go:121-127`, `:584-591`, `:921-928`, `:1376-1383`, `:1997-2004`), so the failure mode is specific to detail and mutation paths. |
| Discrepancy | The handoff treats zero-assignment scope as a list/summary consistency issue. In reality the detail and mutation paths **fail open**, which is the opposite of fail-closed. |
| Resolution | Raised as **P0-5 (new)** with the full affected-endpoint list in Section 5. This is the highest-severity finding of Phase 3A. |

### D-7 Rate-limit facts: booking limit is 2, not 3

| | |
|---|---|
| Current source | `booking.go:97` comment says "max 3 bookings per phone per day", but the injected limiter is the shared `limiter.NewDailyLimiter(2)` created at `cmd/server/main.go:114` and wired at `main.go:304` |
| Actual | **2 requests per 25-hour window per phone** for `POST /api/v1/appointments`, identical to `POST /api/v1/queues/generate` |
| Resolution | Code wins. The UI must not surface a "3 per day" claim. Verified limits are enumerated in Section 5.6. |

### D-8 Facility deactivate response type is a string, not a boolean

| | |
|---|---|
| Current source | `admin.go:426-429` returns `map[string]string{"id": id, "is_active": "false"}` |
| Actual | `is_active` serialises as the JSON **string** `"false"`, whereas `facilityResponse` (`admin.go:60`) and `publicFacility` (`catalog.go:29`) use a JSON boolean |
| Resolution | The client must not `JSON.parse`-style coerce this field into a boolean comparison. Recorded as an implementation note for the facilities module. |

### D-9 Design-source token drift in `design/SKILLS.md`

| | |
|---|---|
| `design/SKILLS.md` | Palette quick reference lists Muted as `#706b65` |
| `design/DESIGN.md`, handoff Section 2, UX audit D14 | Muted is `#57534E` |
| Resolution | `DESIGN.md` and the handoff are authoritative. `#57534E` is canonical. `design/SKILLS.md` is a generated boilerplate readme and is not a token source. No UX change; recorded so implementers do not pick up the wrong muted value. |

### D-10 `Route.PHI` is declared but never set

| | |
|---|---|
| Current source | `router.go:21` declares `PHI bool`; `router_test.go:11` `TestRegistry_PHIRoutesHavePolicy` asserts PHI routes carry a policy |
| Actual | No entry in `router.Registry` sets `PHI: true`, so the test is currently vacuous |
| Resolution | Not a redesign blocker. Recorded as a latent test-coverage gap. No change proposed in Phase 3A (out of scope, would be a behavior change). |

### Discrepancies checked and confirmed CORRECT (no change)

| Handoff claim | Verification |
|---|---|
| Queue state machine `waiting -> called/cancelled; called -> in_service/cancelled/skipped; in_service -> completed` | Matches `admin.go:810-817` |
| `estimated_wait_minutes` fixed at 25 | Matches `apps/queue-engine/src/engine/queue.rs:181` |
| Public facilities catalog exposes only name/type/short_code/is_active | Matches `catalog.go:25-30`, `:48-51`. `GET /api/v1/public/facilities` reads **no** query params. |
| Public service-units filters by optional `facility_id`, returns name/code | Matches `catalog.go:76-118` |
| Patient status lookup accepts a single `code` (tried as checkin_code, then formatted_number) | Matches `patient.go:87-125` |
| Walk-in request is `{facilityId, patient:{fullName, phone, gender?, dateOfBirth?}}` (camelCase) | Matches `queue.go:118-126` |
| Booking request is snake_case `{facility_id, service_unit_id, patient_display_name, patient_phone, appointment_time}` | Matches `booking.go:56-65` |
| Check-in requires `appointment_id` + `checkin_code` | Matches `booking.go:435-445`, `:480-489` |
| Notification `limit` default 100, honoured only when `0 < n <= 500` | Matches `notifications.go:73-79` |
| Notifications have no server pagination | Confirmed, no offset/cursor/page param exists |
| Supabase SSR cookie session, Bearer forwarded, token permission claims ignored | Matches `hooks.server.ts`, `lib/server/auth.ts`, `jwt_provider.go:246` |
| `/faskes` and `/admin` require new route implementations | Confirmed absent |
| `schedule.manage` absent from dev identity | Confirmed: `auth/dev_provider.go:57-69` lists 11 permissions and `schedule.manage` is not among them |

---

## 3. Canonical Route Decisions

### 3.1 Final route table (additions only, no renames)

| Design surface | Production route | Status |
|---|---|---|
| Beranda | `/` | exists, redesign in place |
| Cari Faskes | `/faskes` | **NEW route required** |
| Buat Janji Temu | `/appointments/new` | exists, preserve URL |
| Check-In Janji Temu | `/appointments/check-in` | exists, preserve URL |
| Ambil Antrean Tanpa Janji | `/queues/new` | **NEW route required (decision below)** |
| Status Kunjungan | `/patient/status` | exists, preserve URL |
| Masuk / Daftar / Keluar | `/auth/login`, `/auth/register`, `/auth/logout` | exist, preserve URLs |
| Ringkasan | `/admin` | **NEW route required** |
| Antrean | `/admin/queues` | exists, preserve URL |
| Janji Temu | `/admin/appointments` | exists, preserve URL |
| Jadwal | `/admin/schedules` | exists, preserve URL |
| Fasilitas | `/admin/facilities` | exists, preserve URL |
| Notifikasi | `/admin/notifications` | exists, preserve URL |
| Wallet | `/wallet` | exists, **excluded from navigation** per D7; code untouched |

Exactly three new routes, matching the two route classes identified in the UX audit (`/faskes`, `/admin`)
plus the one walk-in route the handoff left undecided. No existing URL is renamed.

### 3.2 WALK_IN_ROUTE = `/queues/new`

Decision recorded in the required format:

```
WALK_IN_ROUTE = /queues/new
```

Why:

1. **It must be separate from appointment check-in.** The frozen design already treats them as two pages:
   `design/generated/sigap-warga-desktop/pages/check-in.html:251` and
   `design/generated/sigap-warga-mobile/pages/check-in.html:218` both link out to `antrean.html` with the copy
   "Datang tanpa janji? Ambil antrean tanpa janji." and the separate page title "Ambil Antrean Tanpa Janji".
   The two flows also have genuinely different contracts (R3-F2): check-in takes `appointment_id` +
   `checkin_code` and calls `POST /api/v1/appointments/{id}/check-in`; walk-in takes name + phone and calls
   `POST /api/v1/queues/generate`. Overloading `/appointments/check-in` would merge two backend contracts
   behind one URL, which R3-F2 explicitly forbids.
2. **It must not overload `/appointments/check-in`.** Rejected outright.
3. **Existing URLs are preserved.** `/appointments/check-in`, `/appointments/new`, `/patient/status` and all
   `/admin/*` routes keep their current paths. `/queues/new` is purely additive.
4. **It is not chosen from the prototype filename.** The prototype file is `antrean.html`; `/antrean` was
   considered and rejected (see below). The choice is driven by the existing production URL convention.
5. **It matches the established convention.** The citizen transactional routes use an English
   `<resource>/<action>` shape: `/appointments/new` creates an appointment. `/queues/new` creates a queue
   ticket, which is exactly what `POST /api/v1/queues/generate` does. The path space does not collide with the
   API proxy, which lives under `/api/v1/**`.
6. **It reads correctly for the label.** The Indonesian UI label stays "Ambil Antrean Tanpa Janji"; the URL
   stays resource-oriented, consistent with every other route in the app.

Alternatives considered and rejected:

| Candidate | Rejected because |
|---|---|
| `/appointments/check-in` (reuse) | Merges two distinct backend contracts; explicitly forbidden by R3-F2 and the handoff. |
| `/antrean` | Chosen-from-filename, and breaks the English `<resource>/<action>` convention used by every existing transactional route. |
| `/walk-in` | Introduces English UI vocabulary that does not exist anywhere in the Indonesian product copy. |
| `/queues/generate` | Mirrors a backend verb in a page URL and sits one segment away from the `/api/v1/queues/generate` proxy, which invites confusion. |
| `/queues` (index + form) | Implies a queue listing surface for citizens, which the backend does not expose (citizens have no identity, per M.1). |

Navigation placement: `/queues/new` is **not** a bottom-nav tab. The frozen mobile bottom nav has exactly
four destinations (Beranda, Faskes, Check-In, Status; verified at
`design/generated/sigap-warga-mobile/pages/beranda.html:345-357`). Walk-in is reached from the Check-In page
and from Beranda quick actions, exactly as the frozen design wires it
(`beranda.html:244` links to `antrean.html`).

### 3.3 Route-level contract for `/queues/new`

| Aspect | Value |
|---|---|
| Backend call | `POST /api/v1/queues/generate` via existing proxy `apps/web/src/routes/api/v1/queues/generate/+server.ts` (verified correct) |
| Request body | `{"facilityId": "<uuid>", "patient": {"fullName": "...", "phone": "..."}}` (camelCase, verified `queue.go:118-126`) |
| Required fields | `facilityId`, `patient.phone`, `patient.fullName` |
| Success | 200 `{"success":true,"data":{...}}` with `formatted_number`, `estimated_wait_minutes` |
| Facility source | `GET /api/v1/public/facilities` |
| Rate limit | 429, key `YYYY-MM-DD:phone:facilityId`, 2 per 25h |

Implementation caveat to carry into tasks: `service.GenerateResult` (`internal/service/queue.go:24-32`)
serialises `TicketID` and `Status` with **no** `json` tag, so the wire keys are `"TicketID"` and `"Status"`
(PascalCase) while siblings are snake_case. The existing `BedAvailabilityDashboard.svelte` already defensively
reads both `formatted_number` and `FormattedNumber`. New code should read `formatted_number` primarily and
tolerate the PascalCase variants. Normalising the Go tags is a separate, optional backend cleanup and is
**not** part of the redesign.

---

## 4. Exact Status and State Machines

### 4.1 Two independent state domains

The appointment lifecycle and the queue lifecycle are **separate** enums in separate tables
(`appointment_status` in `0005_appointments.sql`, `queue_status` in `0001_init.sql`). The handoff's
Section 5 blends them into one line. They must be implemented as two distinct maps.

### 4.2 Appointment state machine (verified, authoritative)

Admin mutation flow, `PATCH /api/v1/admin/appointments/{id}/status`, source `admin.go:1950-1958`:

| source_status | allowed destination statuses |
|---|---|
| `scheduled` | `checked_in`, `cancelled`, `no_show` |
| `checked_in` | `queued`, `cancelled`, `no_show` |
| `queued` | `completed`, `cancelled`, `no_show` |
| `completed` | (none - terminal) |
| `cancelled` | (none - terminal) |
| `no_show` | (none - terminal) |

Rejection semantics (`admin.go:2061-2121`):

| Condition | Status | Body `error` |
|---|---|---|
| malformed JSON | 400 | `Format permintaan tidak valid.` |
| empty `status` | 400 | `Status wajib diisi.` |
| status outside the 6-value enum | 400 | `Status janji temu tidak valid.` |
| transition not in the map above | 400 | `Transisi status '<from>' -> '<to>' tidak diizinkan.` |
| id missing/not a UUID | 400 | `ID janji temu tidak valid.` |
| out of scope, or empty scope, or scope resolution failure | 404 | `Janji temu tidak ditemukan.` |
| row absent | 404 | `Janji temu tidak ditemukan.` |
| success | 200 | `{"success":true,"data":{"id":..,"status":..,"updated_at":..}}` |

Side effects on transition (`admin.go:2123-2132`): entering `checked_in` sets `checkin_at = NOW()`;
entering `completed` sets `completed_at = NOW()`; entering `cancelled` sets `cancelled_at = NOW()`.

**PUBLIC CHECK-IN flow is a different mechanism entirely.** `POST /api/v1/appointments/{id}/check-in`
(`booking.go:416-673`) does **not** consult `validAppointmentTransitions`. It performs a guarded two-step
transition inside one request:

1. Atomic claim (`booking.go:480-489`): `UPDATE appointments SET status='checked_in', checkin_at=NOW() WHERE id=$1 AND LOWER(checkin_code)=LOWER($2) AND status='scheduled' RETURNING ...`. This enforces `scheduled -> checked_in` and the code match in one statement.
2. Finalisation (`booking.go:649-651`): `UPDATE appointments SET status='queued', queue_ticket_id=$1 WHERE id=$2 AND status='checked_in' AND queue_ticket_id IS NULL`.

Net effect of one successful public check-in: `scheduled -> checked_in -> queued`, and the response reports
`"status": "queued"` (`booking.go:669`). There are compensating rollbacks to `scheduled` at `booking.go:542`,
`:562`, `:609`, `:636` if queue generation fails.

Public check-in error semantics (documented verbatim at `booking.go:416-420`, information-oracle resistant):

| Condition | Status | Body `error` |
|---|---|---|
| bad URL shape | 400 | `URL tidak valid.` |
| id not a UUID | 400 | `ID janji temu tidak valid.` |
| malformed JSON | 400 | `Format permintaan tidak valid.` |
| empty `checkin_code` | 400 | `Kode check-in wajib diisi.` |
| brute-force limit (5 per 5 min per IP+appointment) | 429 | `Terlalu banyak percobaan. Coba lagi nanti.` |
| appointment absent | 404 | `Janji temu tidak ditemukan.` |
| code mismatch | **401** | `Kode check-in tidak cocok.` |
| code matched but status not `scheduled` | 409 | `Janji temu tidak dapat check-in dengan status: <dbStatus>` |
| lost race during finalisation | 409 | `Janji temu sudah diproses oleh permintaan lain.` |

Implementation consequence: the UI must map the public check-in 401 to the "wrong code" state, **not** to an
authentication state. This is the one place a 401 legitimately means something other than "not logged in".

### 4.3 Queue state machine (verified, authoritative)

Admin mutation flow, `PATCH /api/v1/admin/queues/{id}/status`, source `admin.go:810-817`:

| source_status | allowed destination statuses |
|---|---|
| `waiting` | `called`, `cancelled` |
| `called` | `in_service`, `cancelled`, `skipped` |
| `in_service` | `completed` |
| `completed` | (none - terminal) |
| `cancelled` | (none - terminal) |
| `skipped` | (none - terminal) |

Invalid transition -> **400** `Transisi status tidak valid: <from> -> <to>.` (`admin.go:740-746`).

Read-only default: a newly generated ticket is created with status `waiting`
(`apps/queue-engine/src/engine/queue.rs:140`).

### 4.4 Admin authentication and authorization error semantics (verified)

This is the corrected matrix the handoff must adopt.

| Runtime condition | HTTP status | Body `error` | Source |
|---|---|---|---|
| Request to an undeclared method+path | **401** | `Akses ditolak: rute tidak dikenali atau memerlukan otorisasi.` | `router.go:109` |
| Authenticated route, no credentials / invalid token / expired token | **403** | `Akses ditolak: autentikasi diperlukan.` | `authz.go:78` |
| Authenticated, missing required permission | **403** | `Akses ditolak: izin tidak mencukupi.` | `authz.go:90` |
| Valid permission, empty facility assignment, **non-super_admin** (list endpoints) | **200** | `{"success":true,"data":[]}` | `admin.go:121-127` etc. |
| Valid permission, empty facility assignment, **non-super_admin** (detail/mutation endpoints) | 404 normally, **but see D-6: currently fails open** (the Section 4.5 fix makes the 404 + zero-side-effect behavior the guaranteed result) | - | `admin.go:190` etc. |
| Dev actor, or DB-resolved active global `super_admin` (Section 4.5) | unrestricted facility scope | predicate intentionally omitted; endpoint permissions still enforced | resolver per Section 4.5.1 |
| Scope resolution failure (list endpoints) | **200** | `{"success":true,"data":[]}` | `admin.go:105-112` |
| Scope resolution failure (detail/mutation endpoints) | **404** | `... tidak ditemukan.` | `admin.go:186`, `:355`, `:401` |

**Critical implementation consequence.** Because both "not authenticated" and "not authorized" return 403 with
only the `error` string differing, the UI cannot branch on status code. The Admin shell must distinguish the
two panels as follows:

- `UnauthPanel` ("Masuk untuk melanjutkan"): render when there is **no client-side Supabase session**.
  The session presence is already available from `+layout.server.ts` (`locals.session?.user?.email`) and can
  be widened to expose a boolean `hasSession` without exposing the token.
- `ForbiddenPanel` ("Izin tidak mencukupi"): render when a session exists but the API returned 403.
- Fallback: if a 403 arrives with no session, prefer `UnauthPanel`, since the message
  `Akses ditolak: autentikasi diperlukan.` indicates missing authentication. Matching on that string is
  acceptable as a secondary signal but must not be the primary mechanism.

The handoff previously implied a clean 401/403 split. It does not exist. Section 12 of the handoff and
Section 6 of the state matrix have been corrected accordingly.

### 4.5 Global super_admin facility-scope decision (Phase 3A.1, owner decision)

Recorded owner decision. This section supersedes the "dedicated flag" suggestion in P0-5 and closes
GATE 0's open product decision.

| Item | Decision |
|---|---|
| Global unrestricted administration | **REQUIRED.** A genuine `super_admin` is intended to have unrestricted administration across facilities. |
| Source of truth | The **DB-resolved, active global `super_admin` assignment** in the existing role model: a `user_roles` row where the joined role is named `super_admin`, `facility_id IS NULL`, `status = 'active'`, and `deleted_at IS NULL` (lifecycle columns from `0009_user_roles_lifecycle.sql`). Nothing else grants unrestricted scope. |
| Empty facility set alone | **NEVER unrestricted.** `len(allowedFacilities) == 0` is a fail-closed condition for every non-dev, non-global-super_admin actor. |
| Dev actor | `actor.IsDev == true` remains its **separate, development-only** unrestricted mechanism (guarded by `SIGAP_ENV=local`). It is not super_admin semantics and must not be conflated with it. |
| Permissions | **Unrestricted facility scope does not grant API permissions.** The per-route policy check (`identity.RequirePermission`) still runs for every admin route. A global super_admin without `schedule.manage` cannot mutate schedules. |
| Zero-assignment non-super_admin | Fail closed: lists return 200 + empty; detail returns 404; mutation returns 404 with **zero side effects**; notification summary returns all five status keys at zero. |
| Client-side inference | **Forbidden.** No role switching, no client-side authorization, no frontend inference from email, role name, or an empty scope. Unrestricted scope is server-side authorization state only. |
| DB migration expected | **NO.** The current schema expresses this safely: `roles.name`, `user_roles.facility_id`, and the 0009 lifecycle columns are sufficient. No new column is invented. |

#### 4.5.1 Exact resolver semantics

The scope layer centralizes the decision. Handlers must consume one explicit result, not scattered
per-handler special cases. Target contract (implementation in 3B0, plan Section 5 P0-4/P0-5):

```go
// Returned by the auth/facility-scope layer, not computed ad hoc in handlers.
type FacilityScopeResult struct {
    IDs          []uuid.UUID // active user_roles facility IDs (facility_id NOT NULL)
    Unrestricted bool        // true ONLY for: dev actor, OR a DB-resolved active global
                              // super_admin assignment. Never true for an empty set alone.
    Err          error       // any resolver failure => fail-closed
}
```

| State | Handler behavior |
|---|---|
| `Unrestricted = true` (dev, or DB-resolved global super_admin) | The facility predicate may be **intentionally omitted**. A consumer **must not** short-circuit on an empty `IDs` set in this case, because an unrestricted actor typically has `IDs` empty. Return global data / permit the operation; the endpoint's permission check still applies. |
| `Unrestricted = false`, IDs present | The facility predicate **MUST be applied**. |
| `Unrestricted = false`, zero IDs | **Fail closed.** List: 200 + empty. Detail: 404. Mutation: 404 with no write executed. Summary: all-zero. |
| `Err != nil` (scope-resolution failure) | **Fail closed**, according to endpoint class: list returns 200 + empty; detail/mutation returns 404 with no write; summary returns all-zero. Never reinterpreted as unrestricted. |

Every facility-scope consumer repository-wide must implement this table identically (Admin LIST paths, Admin
DETAIL/MUTATION paths, and notification paths). `CanAccessFacilityForActor`, if retained, must consume the
same centralized result rather than a separate decision. A source-level audit that enumerates every call site
is the mandatory first implementation step of 3B0 T-3B0-07; the discovered list is authoritative over any
pre-recorded list.

The global-super_admin determination is an additive read-only query (no schema change):

```sql
EXISTS (
  SELECT 1 FROM user_roles ur
  JOIN roles r ON r.id = ur.role_id
  WHERE ur.user_id = $1
    AND r.name = 'super_admin'
    AND ur.facility_id IS NULL
    AND ur.status = 'active'
    AND ur.deleted_at IS NULL
)
```

Token claims never contribute: the JWT `permissions` claim is already ignored by the provider
(`jwt_provider.go:246`, AUDIT-101), and `AppUserID` (the subject) is the only input to both the
permission and scope resolvers.

### 4.6 Other error envelopes the UI must normalise

| Source | Status | `error` string | UI treatment |
|---|---|---|---|
| `writeError` (all handlers) | various | Indonesian message | Generic server/validation panel, message shown as-is |
| `writeAuthzError` | 403 | see 4.4 | Forbidden or Unauth panel |
| `DenyByDefault` | 401 | see 4.4 | Treat as forbidden; a 401 here means the proxy or method is wrong, not that the user is logged out |
| `POST /queues/generate` 429 | 429 | `Nomor HP ini sudah mencapai batas maksimal 2 antrean per hari untuk fasilitas tersebut. Silakan coba lagi besok atau daftar di fasilitas lain.` | Rate-limit panel, message as-is |
| `POST /appointments` 429 | 429 | `Nomor HP ini sudah melebihi batas pemesanan per hari. Silakan coba lagi besok.` | Rate-limit panel, message as-is |
| check-in 429 | 429 | `Terlalu banyak percobaan. Coba lagi nanti.` | Rate-limit panel |
| `GET /patient/status` 429 | 429 | `Terlalu banyak permintaan. Coba lagi nanti.` | Rate-limit panel |

Note: `writeError` uses `json.NewEncoder(...).Encode`, which appends a trailing newline, whereas
`writeAuthzError` and `DenyByDefault` use raw `w.Write` without one. Parsers must not assume a trimmed body.

---

## 5. P0 Root-Cause Matrix

All five items were re-verified against current source. **None were fixed.** All are code-level; none requires
a database migration.

### P0-1 Notification retry authorizes after mutating

| Field | Detail |
|---|---|
| Affected file(s) | `apps/api/internal/handler/notifications.go`; underlying write in `apps/api/internal/notification/service.go` |
| Affected handler | `NotificationsHandler.RetryNotification` |
| Current behavior | `notifications.go:299` calls `h.svc.Retry(...)` (which executes the `UPDATE notification_outbox ... RETURNING id`) and only **then**, at `:314`, calls `auth.CanAccessFacilityForActor(...)` |
| Exact exploit | An authenticated actor holding `notification.manage` but assigned to facility A sends `POST /api/v1/admin/notifications/{id-of-facility-B}/retry`. The outbox row for facility B is reset to `pending` with `attempt_count + 1` and `next_attempt_at = NOW()`. The response is 404, but the mutation has already been committed. The actor can thus force delivery retries on facilities outside their scope, and can infer row existence from the 404-vs-409 split (`:302-307` returns 404 for absent, 409 for wrong state). |
| Expected safe behavior | Facility scope must be resolved and enforced **before** any write. An out-of-scope or non-existent id must produce an identical 404 with no side effect. |
| Proposed remediation | Resolve the row's facility (or resolve the actor's allowed set and add `AND facility_id = ANY($n)` to the `Retry` SQL) before mutating. Preferred minimal change: add a pre-check that loads the row's `facility_id` and calls `CanAccessFacilityForActor`, returning 404 on failure, then perform the mutation. Alternative: push the facility predicate into `Service.Retry` so the `UPDATE` itself is scoped, and map a zero-row result to 404. |
| Required tests | (a) out-of-scope retry returns 404 **and** leaves `status`/`attempt_count`/`next_attempt_at` unchanged; (b) in-scope retry succeeds and mutates; (c) absent id returns 404 with no mutation; (d) wrong-state in-scope returns 409. Test name must be greppable by CI; follow the existing DB-backed naming pattern. |
| Compatibility risk | Low. Behavior only tightens; no response shape changes. A client that previously relied on the (incorrect) side effect would be affected, but no legitimate client depends on cross-facility retry. |
| DB migration required | **NO** |

### P0-2 Notification cancel authorizes after mutating

| Field | Detail |
|---|---|
| Affected file(s) | `apps/api/internal/handler/notifications.go`; `apps/api/internal/notification/service.go` |
| Affected handler | `NotificationsHandler.CancelNotification` |
| Current behavior | `notifications.go:334` calls `h.svc.Cancel(...)` (executes `UPDATE ... SET status='cancelled'`) and only at `:349` performs the scope check |
| Exact exploit | Same shape as P0-1: an actor scoped to facility A can cancel facility B's queued notifications. Because `Cancel` is idempotent (`service.go:285-309`, allows `pending`, `failed`, `cancelled`), the actor can silently suppress notifications for any facility by id. This is a denial-of-notification primitive affecting patient-facing messages. |
| Expected safe behavior | Scope enforced before write; out-of-scope id returns 404 with no state change. |
| Proposed remediation | Identical approach to P0-1: pre-check scope, or push `AND facility_id = ANY($n)` into `Service.Cancel` and map zero rows to 404. |
| Required tests | (a) out-of-scope cancel returns 404 and leaves `status` unchanged; (b) in-scope cancel mutates; (c) idempotent in-scope re-cancel still succeeds; (d) delivered-row in-scope returns 409. |
| Compatibility risk | Low, same reasoning as P0-1. |
| DB migration required | **NO** |

### P0-3 Notification summary global fallback leaks platform-wide counts

| Field | Detail |
|---|---|
| Affected file(s) | `apps/api/internal/handler/notifications.go:188-221`; `apps/api/internal/notification/service.go:225-251` |
| Affected handler | `NotificationsHandler.Summary` (route `GET /api/v1/admin/notifications/summary`) |
| Current behavior | Empty-but-valid scope leaves `facilityFilter = uuid.Nil`; `Service.Summary` treats `uuid.Nil` as the "all facilities" super-admin sentinel (`service.go:228-233`). The list handler has a post-fetch scope filter (`notifications.go:153-172`) that drops out-of-scope rows; **Summary has no equivalent**. On scope-resolution error the handler returns 200 with an empty map, which is safe but silently indistinguishable from a genuine zero. |
| Exact exploit | An actor with `notification.read` and zero active facility assignments - i.e. a **non-super_admin** actor (after the Section 4.5 decision, the global `super_admin` receives global counts by design; the exploit actor is any other empty-scope actor) - calls `GET /api/v1/admin/notifications/summary` and receives counts of `pending`/`processing`/`delivered`/`failed`/`cancelled` aggregated across every facility on the platform. This discloses operational volume and failure rates for facilities the actor has no assignment to. |
| Expected safe behavior | An empty scope must yield an all-zero map, never a global aggregate. The `uuid.Nil` sentinel must only be reachable for a genuinely unrestricted actor (dev, or a DB-resolved active global `super_admin` per Section 4.5), or be removed entirely. |
| Proposed remediation | Two parts. (1) In the handler, short-circuit to an all-zero map when `!unrestricted && len(allowedFacilities) == 0`, mirroring the list endpoints. (2) In `Service.Summary`, stop using a magic UUID as an "all facilities" flag; add an explicit `unrestricted bool` parameter (or a nullable filter) so the global path is only reachable by explicit intent. Part (2) is the durable fix because it removes the sentinel that caused the bug. |
| Required tests | (a) zero-assignment **non-super_admin** actor gets all-zero counts; (b) facility-scoped actor gets only their facility's counts; (c) dev actor and a DB-resolved active global `super_admin` get global counts; (d) scope-resolution failure yields all-zero (not an error); (e) the returned map always contains all five status keys, zero-filled. |
| Compatibility risk | Medium. Part (1) is purely tightening. Part (2) changes an internal function signature (`Summary`), so all callers must be updated in the same change; the HTTP response shape is unchanged. If the `Summary` signature change is judged too invasive for 3B0, ship part (1) alone and record part (2) as a follow-up, since part (1) closes the leak. |
| DB migration required | **NO** |

### P0-4 Zero-assignment facility scope is inconsistent across endpoint classes

| Field | Detail |
|---|---|
| Affected file(s) | `apps/api/internal/auth/facility_scope.go`; `apps/api/internal/handler/admin.go`; `apps/api/internal/handler/notifications.go` |
| Affected functions | `AllowedFacilityIDsForActor`; every admin handler that consumes the resolved facility scope; `CanAccessFacilityForActor`; `Summary` |
| Current behavior | `AllowedFacilityIDsForActor` (`facility_scope.go:122-144`) returns `(emptySlice, false, nil)` for a non-dev actor with no assignments. Consumers diverge: list endpoints treat empty as "return nothing"; detail/mutation endpoints omit the scope predicate and therefore operate **unscoped**. A `super_admin` whose role row has `facility_id = NULL` (the documented global-role case, `facility_scope.go:48-57` excludes NULL) is exactly this actor, so the failure is reachable by a real seeded role, not only by misconfiguration. |
| Exact exploit | See P0-5, which is the concrete manifestation. The design-level defect is the divergence itself. |
| Expected safe behavior | One fail-closed rule everywhere: an actor with no resolvable facility scope may read nothing and mutate nothing. |
| Proposed remediation | Make the rule structural rather than per-handler, following the explicit decision in Section 4.5. The auth/facility-scope layer returns one centralized result (`FacilityScopeResult`: `IDs`, `Unrestricted`, `Err`) where `Unrestricted` is true **only** for the dev actor or a DB-resolved active global `super_admin` assignment. **Every** facility-scope consumer repository-wide must be migrated, not only the detail/mutation handlers: all Admin LIST paths, all Admin DETAIL/MUTATION paths (including `UpdateQueueStatus`, `UpdateAppointmentStatus`, and all four create/update paths that authorize a client-supplied `facility_id`), and all notification paths (list, summary, get, retry, cancel). The mandatory first implementation step is a source-level audit that greps for every call site of `AllowedFacilityIDsForActor` and `CanAccessFacilityForActor` and records the exact consumer list; source grep wins and the discovered list is authoritative. Handlers never re-derive scope from role names and never treat an empty set as unrestricted: every consumer must evaluate `Unrestricted` **before** it interprets an empty `IDs` set. A regression test must assert that no consumer treats `len(IDs) == 0` as either unrestricted or automatically empty without first considering `Unrestricted`. |
| Required tests | A table-driven test over **every** facility-scope consumer (not only the eight P0-5 handlers) asserting that a zero-assignment **non-super_admin** actor receives no data and performs no mutation, while a DB-resolved active global `super_admin` retains unrestricted access and the dev actor remains unrestricted (the full case split is the Section 13.2 matrix A-G). Must cover: all five Admin LIST paths; facility get/update/deactivate; queue get and status update; service-unit get/update plus both create and update paths that authorize a client-supplied `facility_id`; schedule get/update plus both create and update paths that authorize a client-supplied `facility_id`; appointment list/status; and notification list/summary/get/retry/cancel. The notification-list post-fetch membership filter and all four supplied-`facility_id` authorization paths require explicit assertions: for a DB-resolved active global `super_admin` they must permit the target facility, and for a zero-assignment **non-super_admin** they must deny it. |
| Compatibility risk | Medium. This is a deliberate behavior change for the zero-assignment, **non-super_admin** case only. Per the Section 4.5 owner decision, a DB-resolved active global `super_admin` keeps unrestricted access explicitly; everyone else with zero active facility assignments loses their (incorrect) access and the affected surfaces render their empty/404 states. Release notes must call this out, and the seed data must be verified to contain facility-scoped `user_roles` rows for demo admins (UX audit M.9) plus one global `super_admin` identity. |
| DB migration required | **NO** (seed data verification is an operational check, not a migration) |

### P0-5 (NEW, CRITICAL) Detail and mutation endpoints fail open on empty scope

| Field | Detail |
|---|---|
| Affected file(s) | `apps/api/internal/handler/admin.go` |
| Affected handlers and lines | `GetFacility` (`:190`), `UpdateFacility` (`:362`), `DeactivateFacility` (`:409`), `GetQueueTicket` (`:658`), `GetServiceUnit` (`:999`), `UpdateServiceUnit` (`:1202`), `GetSchedule` (`:1459`), `UpdateSchedule` (`:1693`) |
| Current behavior | Each uses `if !actor.IsDev && len(allowedFacilities) > 0 { query += " AND facility_id = ANY($n)" }`. When a non-dev actor has zero assignments, the predicate is skipped and the statement runs against **all** facilities. |
| Exact exploit | An authenticated actor holding `facility.manage` but with **zero active facility assignments** (for example a user whose `user_roles` rows are inactive or soft-deleted, or a holder of a global role that is **not** `super_admin`) sends `PATCH /api/v1/admin/facilities/{any-uuid}` with a body such as `{"name":"x"}`. Because the predicate is absent, the `UPDATE facilities SET name='x' WHERE id=$n` matches and `RowsAffected()` is 1, so the response is **200 OK** and another facility's record is modified. The same shape applies to deactivate (a destructive one-way action), to service-unit update, and to schedule update. Read paths (`GetFacility`, `GetQueueTicket`, `GetServiceUnit`, `GetSchedule`) leak full records including `address`, `phone`, `total_beds` for any facility by id. Note: after the Section 4.5 fix, the seeded global `super_admin` is no longer an exploit actor: its unrestricted scope is granted explicitly and structurally. |
| Why it was missed | The list endpoints look correct (they return empty), and `UpdateQueueStatus` (`:715`) and `UpdateAppointmentStatus` (`:2092-2098`) do have explicit empty-scope guards, so a spot check suggests the rule is enforced. Only the eight endpoints above omit it. |
| Expected safe behavior | Empty scope denies all reads and all mutations for every actor whose scope is not explicitly unrestricted. A zero-row result must be the outcome, surfaced as 404 `... tidak ditemukan.` with no existence leak. An explicitly unrestricted actor (dev, or DB-resolved active global `super_admin` per Section 4.5) is the only case where the predicate is intentionally omitted. |
| Proposed remediation | Centralize the decision per Section 4.5. The auth/facility-scope layer computes and returns `FacilityScopeResult{IDs, Unrestricted, Err}`: `Unrestricted` is true only for the dev actor or a DB-resolved active global `super_admin` assignment (Section 4.5.1 query). In each of the eight handlers, consume that result: `Unrestricted = true` -> omit the predicate; `Unrestricted = false` with IDs present -> apply the predicate unconditionally; `Unrestricted = false` with zero IDs -> immediate 404 guard **before** any statement is built (no write can execute); `Err != nil` -> immediate 404. Do not scatter role-name checks through handlers. |
| Required tests | The super_admin security test matrix (Section 13.2, cases A-G) plus, for each of the eight handlers: an in-scope actor (facility A) can read/mutate A and cannot read/mutate B by id; out-of-scope read/mutation returns 404 and the target row is byte-identical after the call; scope-resolution failure yields 404 with no write. |
| Compatibility risk | Medium-high for the zero-assignment, non-super_admin case, low otherwise. This closes a real privilege-escalation path, so the behavior change is required, not optional. The `super_admin` question is now **closed by the owner decision in Section 4.5**: global unrestricted administration is required and is modelled as an explicit, DB-resolved unrestricted scope - not as an accident of an empty set, and not by reverting the fix. |
| DB migration required | **NO** |

### 5.6 Verified rate-limit inventory

| Endpoint | Limit | Window | Key | 429 body source |
|---|---|---|---|---|
| `POST /api/v1/queues/generate` | 2 | 25h | `YYYY-MM-DD:phone:facilityId` | `queue.go:59-61` |
| `POST /api/v1/appointments` | 2 (comment says 3; code wins) | 25h | `phone:<normalizedPhone>:YYYY-MM-DD` | `booking.go:100` |
| `POST /api/v1/appointments/{id}/check-in` | 5 | 5 min | `checkin:<clientIP>:<appointmentId>` | `booking.go:458` |
| `GET /api/v1/patient/status` | 30 | 1 min | client IP | `patient.go:72` |
| All `/api/v1/admin/*` | **none** | - | - | - |
| `/api/v1/events/beds`, `/api/v1/facilities/nearby`, `/api/v1/public/*` | **none** | - | - | - |

Implementation rule: Admin modules must **not** render a rate-limit state. The handoff's caution
("Do not manufacture a generic rate-limit state for Admin modules") is confirmed correct by this inventory.

---

## 6. Proxy Audit Matrix

All 23 proxy files under `apps/web/src/routes/api/v1/**/+server.ts` were audited individually. The literal
`ID`/`STATUS` defect is confirmed on 9 files, matching the handoff's suspicion exactly, but the handoff's
"including ... and appointment status" wording understates it: the appointment-status proxy uses the literal
`STATUS`, not `ID`, and `facilities/[id]/deactivate` carries a **second, independent** defect (method mismatch).

| # | Frontend proxy | HTTP method(s) | Expected backend path | Current forwarded path | Correct? | Required fix |
|---|---|---|---|---|---|---|
| 1 | `admin/appointments/+server.ts` | GET, POST | `/api/v1/admin/appointments` | `/api/v1/admin/appointments` | Yes | none |
| 2 | `admin/appointments/[id]/status/+server.ts` | PATCH | `/api/v1/admin/appointments/{id}/status` | `/api/v1/admin/appointments/STATUS` | **No** | interpolate `event.params.id`; the literal is `STATUS`, not even `ID` |
| 3 | `admin/facilities/+server.ts` | GET, POST | `/api/v1/admin/facilities` | `/api/v1/admin/facilities` | Yes | none |
| 4 | `admin/facilities/[id]/+server.ts` | GET, PATCH | `/api/v1/admin/facilities/{id}` | `/api/v1/admin/facilities/ID` | **No** | interpolate `event.params.id` |
| 5 | `admin/facilities/[id]/deactivate/+server.ts` | **POST** | `PATCH /api/v1/admin/facilities/{id}/deactivate` | `/api/v1/admin/facilities/ID/deactivate` | **No (two defects)** | (a) interpolate `event.params.id`; (b) change the export from `POST` to `PATCH` |
| 6 | `admin/notifications/+server.ts` | GET, POST | `/api/v1/admin/notifications` | `/api/v1/admin/notifications` | Yes | none |
| 7 | `admin/notifications/summary/+server.ts` | GET | `/api/v1/admin/notifications/summary` | `/api/v1/admin/notifications/summary` | Yes | none |
| 8 | `admin/notifications/[id]/cancel/+server.ts` | POST | `/api/v1/admin/notifications/{id}/cancel` | `/api/v1/admin/notifications/ID/cancel` | **No** | interpolate `event.params.id` |
| 9 | `admin/notifications/[id]/retry/+server.ts` | POST | `/api/v1/admin/notifications/{id}/retry` | `/api/v1/admin/notifications/ID/retry` | **No** | interpolate `event.params.id` |
| 10 | `admin/queues/+server.ts` | GET | `/api/v1/admin/queues` | `/api/v1/admin/queues` | Yes | none |
| 11 | `admin/queues/[id]/+server.ts` | GET | `/api/v1/admin/queues/{id}` | `/api/v1/admin/queues/ID` | **No** | interpolate `event.params.id` |
| 12 | `admin/queues/[id]/status/+server.ts` | PATCH | `/api/v1/admin/queues/{id}/status` | `/api/v1/admin/queues/ID/status` | **No** | interpolate `event.params.id` |
| 13 | `admin/schedules/+server.ts` | GET, POST | `/api/v1/admin/schedules` | `/api/v1/admin/schedules` | Yes | none |
| 14 | `admin/schedules/[id]/+server.ts` | GET, PATCH | `/api/v1/admin/schedules/{id}` | `/api/v1/admin/schedules/ID` | **No** | interpolate `event.params.id` |
| 15 | `admin/service-units/+server.ts` | GET, POST | `/api/v1/admin/service-units` | `/api/v1/admin/service-units` | Yes | none |
| 16 | `admin/service-units/[id]/+server.ts` | GET, PATCH | `/api/v1/admin/service-units/{id}` | `/api/v1/admin/service-units/ID` | **No** | interpolate `event.params.id` |
| 17 | `appointments/+server.ts` | POST | `/api/v1/appointments` | `/api/v1/appointments` | Yes | none |
| 18 | `appointments/[id]/check-in/+server.ts` | POST | `/api/v1/appointments/{id}/check-in` | `` `/api/v1/appointments/${encodeURIComponent(event.params.id ?? '')}/check-in` `` | Yes | none. This is the only dynamic proxy that interpolates correctly. Use it as the reference pattern. |
| 19 | `events/beds/+server.ts` | GET (SSE) | `/api/v1/events/beds` | `/api/v1/events/beds` | Yes | none |
| 20 | `patient/status/+server.ts` | GET | `/api/v1/patient/status` | `/api/v1/patient/status{query}` | Yes | none (forwards `url.search`) |
| 21 | `public/facilities/+server.ts` | GET | `/api/v1/public/facilities` | `/api/v1/public/facilities` | Yes | none |
| 22 | `public/service-units/+server.ts` | GET | `/api/v1/public/service-units` | `/api/v1/public/service-units` | Yes | none |
| 23 | `queues/generate/+server.ts` | POST | `/api/v1/queues/generate` | `/api/v1/queues/generate` | Yes | none |

Summary: **9 incorrect, 14 correct.** Backend consequence of the literal defect: every `admin.go` id extractor
runs `uuid.Parse` on the segment and returns `""` on failure, so the literal produces 400
`ID ... tidak valid.` (or 404 depending on handler) rather than reaching the intended resource.

### 6.1 Facility deactivate: method contract verification

| Layer | Verified behavior |
|---|---|
| UI (`admin/facilities/+page.svelte:138`) | Calls `apiFetch('/admin/facilities/{id}/deactivate', { method: 'PATCH' })` |
| Proxy (`admin/facilities/[id]/deactivate/+server.ts`) | Exports **only `POST`** -> SvelteKit answers **405** before the backend is reached |
| Go router (`router.go:43`) | `PATCH /api/v1/admin/facilities/` prefix, policy `facility.manage` |
| Go handler (`admin.go:434-453`) | `FacilitiesRouter` dispatches `PATCH` + `/deactivate` suffix to `DeactivateFacility`; `POST` on the same prefix goes to `CreateFacility`; anything else -> 405 |
| `DeactivateFacility` (`admin.go:386-431`) | One-way soft delete: `UPDATE facilities SET is_active = false, updated_at = NOW()`. No reactivation path exists. Success returns `{"id":..,"is_active":"false"}` (string). |

**Conclusion.** The handoff's P0 claim is correct in direction but imprecise in detail: the proxy's path is
also wrong (`ID`), and the method mismatch is UI-`PATCH` versus proxy-`POST`, not backend-`PATCH` versus
proxy-`POST` (the proxy's `GET`/`PATCH` sibling at `facilities/[id]/+server.ts` is already correctly `PATCH`).
The fix is to change the `deactivate` proxy to export `PATCH` and interpolate the id.

### 6.2 Proxy helper inconsistency

Two helper shapes exist and should be unified during 3B1:

- Shape A (admin + patient + appointments): imports `proxyHeaders`, `apiBase`, `authHeaders` from `$lib/server/auth`. `apiBase()` **throws** when `SIGAP_API_INTERNAL` is unset.
- Shape B (public + queues + events): declares a local `const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080'`, and does not attach `proxyHeaders()`.

Shape B silently falls back to a docker-internal hostname, which will fail confusingly outside docker. 3B1
should consolidate on Shape A. This is a robustness improvement, not a P0.

---

## 7. Frontend Architecture Plan

Design principle: **minimise churn**. The redesign replaces page internals and adds a component layer, but it
does not restructure the SvelteKit route tree, does not change the adapter, does not change the session
mechanism, and does not introduce a state-management library.

### 7.1 Target directory layout

```
apps/web/src/
  lib/
    design/
      tokens.css                 # semantic CSS variables, single source for colour/space/radius
      tokens.ts                  # typed token names for use in TS/Svelte where a string is needed
    ui/                          # shared visual primitives (the design-system layer)
      Button.svelte
      IconButton.svelte
      Field.svelte
      Input.svelte
      Select.svelte
      Textarea.svelte
      Alert.svelte
      StatusBadge.svelte
      Dialog.svelte
      EmptyState.svelte
      ErrorState.svelte
      LoadingState.svelte
      Skeleton.svelte
      ForbiddenPanel.svelte
      UnauthPanel.svelte
      CodeDisplay.svelte
      DataTable.svelte
      Toast.svelte
    citizen/
      CitizenHeader.svelte
      CitizenDesktopNav.svelte
      CitizenBottomNav.svelte
      QuickActions.svelte
      FacilitySearch.svelte
      FacilityResultRow.svelte
      BookingStepper.svelte
      BookingSummary.svelte
      CheckinForm.svelte
      WalkInForm.svelte
      QueueTicket.svelte
      VisitProgress.svelte
      AuthForms.svelte
      AccountMenu.svelte
    admin/
      AdminShell.svelte
      AdminSidebar.svelte
      AdminPageHeader.svelte
      AdminToolbar.svelte
      FacilityFilter.svelte
      AdminTable.svelte
      QueueBoard.svelte
      QueueBoardRow.svelte
      AppointmentRow.svelte
      ScheduleEditor.svelte
      FacilityEditor.svelte
      NotificationRow.svelte
      OutboxSummary.svelte
      AdminResponsiveTablePattern.svelte
    api/
      client.ts                  # apiFetch wrapper: base URL, JSON, error normalisation, abort
      errors.ts                  # typed ApiError union + normaliser
      endpoints/
        public.ts                # facilities, service-units
        citizen.ts               # appointments, check-in, queues/generate, patient/status
        admin.ts                 # facilities, queues, schedules, service-units, appointments, notifications
      types/
        api.ts                   # wire types mirroring the Go response structs exactly
    domain/
      status.ts                  # queue + appointment status maps, labels, badge variants
      joins.ts                   # id -> name resolution helpers
      format.ts                  # date/time/phone/duration formatting (id-ID)
      polling.ts                 # createPolling helper
    server/
      auth.ts                    # EXISTING, unchanged
    supabase/
      server.ts                  # EXISTING, unchanged
  routes/                        # existing tree, internals replaced per Section 9
```

### 7.2 Decisions and rationale

| Decision | Choice | Rationale |
|---|---|---|
| Design tokens | CSS custom properties in `lib/design/tokens.css`, plus Tailwind theme extension mapping to those variables | Tailwind v3 is already present. Tokens as CSS variables allow both utility classes and raw CSS (dialog focus rings, `:focus-visible`) to read the same values. Avoids a Tailwind v4 migration, which would be churn without benefit. |
| Component styling | Tailwind utilities reading semantic tokens (`bg-surface`, `text-foreground`, `border-border`, `rounded-control`) | Prevents the current drift where every page hardcodes `emerald-600`/`slate-*`. One accent, one radius scale, enforced by the token names. |
| Component library | None. Hand-built Svelte components in `lib/ui` | The design system is deliberately small and border-first. Adding shadcn-style generators or a component framework would fight the frozen visual contract and add a second design language. |
| Icons | One icon family, single stroke width, tree-shakeable. The frozen HTML uses `lucide` (`data-lucide` attributes in every generated page) | The frozen design already committed to Lucide glyph names. Honouring the frozen contract means using the same glyphs; deviating would be a visual reinterpretation, which the handoff forbids. Install `lucide-svelte` and standardise `strokeWidth` in one wrapper. |
| API layer | `lib/api/client.ts` `apiFetch` + per-domain endpoint modules | Single place for base URL, headers, JSON parsing, abort signals, and error normalisation. Replaces the current per-page `fetch` duplication. |
| Error normalisation | `lib/api/errors.ts` returns a discriminated union, never throws raw `Error` | Required because 403 is overloaded (Section 4.4) and because the UI must render `UnauthPanel`, `ForbiddenPanel`, `ErrorState`, and rate-limit states differently. |
| State management | **No store library.** Svelte 5 runes (`$state`, `$derived`) plus component props. One narrow exception: a `lib/stores/session.ts` rune module exposing `hasSession` so the Admin shell can choose between `UnauthPanel` and `ForbiddenPanel` without prop-drilling | The app has no cross-route shared mutable state that warrants a store. Facility filter is route-local. Session presence is the only genuinely cross-cutting value, and it is already available server-side via `+layout.server.ts`. |
| `lib/stores/` scope | Only `session.ts`. If a second store is proposed, it must be justified in review against "could this be a prop or a route load?" | The handoff asks for stores "where genuinely needed". Genuinely needed = session presence only. |
| Data loading | SvelteKit `load` functions for server-side fetches where the session is needed; client-side `apiFetch` inside `onMount`/event handlers for user-triggered and polled data | Preserves the existing security model: the proxy attaches the Bearer token server-side. Client-side polling continues to go through the same-origin proxy, so the token never reaches the browser. |
| Facility scope authority | Server-side, always. Client joins and client facility filters are **display narrowing only** | Non-negotiable per the handoff and UX audit D9/M.14. Section 10 states the rule operationally. |
| Responsive approach | Mobile-first for Citizen, desktop-first for Admin. Two separate shells. No hybrid shell. | Frozen contract, UX audit design decision 1. |
| Dark mode | **Deferred, not implemented.** The frozen design defines a light theme only; `tokens.css` will be structured so a dark block can be added later without touching components | The current `dark:` classes are a half-finished media-query variant (UX audit finding 8) and the frozen design has no dark tokens. Implementing dark mode would be inventing visuals. Removing the stray `dark:` classes from replaced pages is part of 3B1/3B2 cleanup. |
| Legacy demo components | `BedAvailabilityDashboard.svelte` and `ReferralMap.svelte` are not part of the target architecture. They are replaced by real Citizen surfaces and deleted later per Section 8 | They embody the fake-metrics and demo-stage patterns that D12 removes. |

### 7.3 What is deliberately NOT built

- No SSR-rendered Admin data. Admin pages fetch through the proxy with a Bearer token; keeping that on the
  client preserves the current, working security boundary and avoids duplicating auth logic server-side.
- No SSE or WebSocket for queues. Polling only, per D8.
- No client-side RBAC. The frontend has no role source (D5) and must not invent one.
- No service-worker, no offline mode, no PWA manifest.
- No pagination controls implying server pagination for notifications (R3-F1).
- No practitioner picker, no practitioner names (no read API exists), and no raw-UUID or free-text
  practitioner field anywhere in the schedule UX. CREATE omits `practitioner_id` (the backend accepts its
  absence, `CreateScheduleRequest` is `omitempty`); UPDATE leaves it untouched (the backend PATCH is
  pointer/omitempty, so an omitted field is never re-sent or cleared). No fake practitioner catalog is
  created. If a practitioner read source is added later, that is a new product decision, not part of the
  redesign.

---

## 8. Component Migration Plan

Format: component -> proposed file -> existing component to reuse/replace -> routes using it -> implementation order.

Order legend: 3B1 shared primitives and tokens, 3B2 Citizen shell and catalog, 3B3 Citizen transactional,
3B4 Admin shell and read pages, 3B5 Admin mutations, 3B6 integration and E2E.

### 8.1 Shared

| Component | Proposed file | Reuse / replace | Routes | Order |
|---|---|---|---|---|
| Button | `lib/ui/Button.svelte` | new (replaces ad-hoc `<button class="...">` everywhere) | all | 3B1 |
| IconButton | `lib/ui/IconButton.svelte` | new | admin tables, dialogs | 3B1 |
| Field | `lib/ui/Field.svelte` | new (label-above-control wrapper) | all forms | 3B1 |
| Input | `lib/ui/Input.svelte` | new | all forms | 3B1 |
| Select | `lib/ui/Select.svelte` | new | booking, schedules | 3B1 |
| Textarea | `lib/ui/Textarea.svelte` | new | booking notes | 3B1 |
| Alert | `lib/ui/Alert.svelte` | replaces inline error banners | all | 3B1 |
| StatusBadge | `lib/ui/StatusBadge.svelte` | replaces per-page status chips | queues, appointments, notifications, status | 3B1 |
| Dialog | `lib/ui/Dialog.svelte` | replaces native `confirm()` and the hand-rolled modals in `admin/facilities` and `admin/schedules` | facilities, schedules | 3B1 |
| EmptyState | `lib/ui/EmptyState.svelte` | replaces ad-hoc empty divs | all lists | 3B1 |
| ErrorState | `lib/ui/ErrorState.svelte` | new (retry affordance) | all | 3B1 |
| LoadingState / Skeleton | `lib/ui/LoadingState.svelte`, `lib/ui/Skeleton.svelte` | generalises the notification page's skeleton pattern (the audit calls it the best existing pattern) | all lists | 3B1 |
| ForbiddenPanel | `lib/ui/ForbiddenPanel.svelte` | replaces the generic "Akses ditolak" banner | all admin | 3B1 |
| UnauthPanel | `lib/ui/UnauthPanel.svelte` | new | all admin | 3B1 |
| CodeDisplay | `lib/ui/CodeDisplay.svelte` | new (the only legitimate monospace surface) | booking success, check-in, status | 3B1 |
| DataTable | `lib/ui/DataTable.svelte` | new (semantic table, `scope` on headers, optional column hiding) | all admin tables | 3B4 |
| Toast | `lib/ui/Toast.svelte` | new (action success, ~4s) | admin mutations | 3B5 |

### 8.2 Citizen

| Component | Proposed file | Reuse / replace | Routes | Order |
|---|---|---|---|---|
| CitizenHeader | `lib/citizen/CitizenHeader.svelte` | replaces the shared header in `+layout.svelte` | all citizen | 3B2 |
| CitizenDesktopNav | `lib/citizen/CitizenDesktopNav.svelte` | replaces the `hidden md:flex` nav | desktop citizen | 3B2 |
| CitizenBottomNav | `lib/citizen/CitizenBottomNav.svelte` | new (there is currently no mobile nav at all) | mobile citizen | 3B2 |
| QuickActions | `lib/citizen/QuickActions.svelte` | replaces the demo dashboard on `/` | `/` | 3B2 |
| FacilitySearch | `lib/citizen/FacilitySearch.svelte` | new (type filter group, per frozen `faskes.html`) | `/faskes` | 3B2 |
| FacilityResultRow | `lib/citizen/FacilityResultRow.svelte` | new | `/faskes` | 3B2 |
| BookingStepper | `lib/citizen/BookingStepper.svelte` | refactor of `/appointments/new` internals | `/appointments/new` | 3B3 |
| BookingSummary | `lib/citizen/BookingSummary.svelte` | new (desktop two-column summary) | `/appointments/new` | 3B3 |
| CheckinForm | `lib/citizen/CheckinForm.svelte` | refactor of `/appointments/check-in` internals | `/appointments/check-in` | 3B3 |
| WalkInForm | `lib/citizen/WalkInForm.svelte` | new page; logic derived from the queue form currently buried in `BedAvailabilityDashboard` | `/queues/new` | 3B3 |
| QueueTicket | `lib/citizen/QueueTicket.svelte` | replaces the wallet-style ticket markup | check-in success, walk-in success | 3B3 |
| VisitProgress | `lib/citizen/VisitProgress.svelte` | replaces the status result card | `/patient/status` | 3B3 |
| AuthForms | `lib/citizen/AuthForms.svelte` | refactor of `/auth/{login,register}` markup; **the `+page.server.ts` actions stay untouched** | `/auth/login`, `/auth/register` | 3B3 |
| AccountMenu | `lib/citizen/AccountMenu.svelte` | replaces the inline header account area | all citizen | 3B2 |

### 8.3 Admin

| Component | Proposed file | Reuse / replace | Routes | Order |
|---|---|---|---|---|
| AdminShell | `lib/admin/AdminShell.svelte` | new (no admin shell exists today) | all admin | 3B4 |
| AdminSidebar | `lib/admin/AdminSidebar.svelte` | new (six destinations, icon rail at ~1024px) | all admin | 3B4 |
| AdminPageHeader | `lib/admin/AdminPageHeader.svelte` | new | all admin | 3B4 |
| AdminToolbar | `lib/admin/AdminToolbar.svelte` | new | all admin | 3B4 |
| FacilityFilter | `lib/admin/FacilityFilter.svelte` | replaces the raw-UUID input on `admin/queues` and the missing filters elsewhere | queues, appointments, schedules, notifications | 3B4 |
| AdminTable | `lib/admin/AdminTable.svelte` | thin wrapper over `lib/ui/DataTable.svelte` adding admin density | all admin | 3B4 |
| QueueBoard / QueueBoardRow | `lib/admin/QueueBoard.svelte`, `QueueBoardRow.svelte` | refactor of `admin/queues/+page.svelte` internals | `/admin/queues` | 3B4 |
| AppointmentRow | `lib/admin/AppointmentRow.svelte` | refactor of `admin/appointments/+page.svelte` internals | `/admin/appointments` | 3B4 |
| ScheduleEditor | `lib/admin/ScheduleEditor.svelte` | replaces the modal in `admin/schedules` | `/admin/schedules` | 3B5 |
| FacilityEditor | `lib/admin/FacilityEditor.svelte` | replaces the modal in `admin/facilities` | `/admin/facilities` | 3B5 |
| NotificationRow | `lib/admin/NotificationRow.svelte` | refactor of `admin/notifications/+page.svelte` row | `/admin/notifications` | 3B4 |
| OutboxSummary | `lib/admin/OutboxSummary.svelte` | refactor of the summary cards | `/admin/notifications` | 3B4 |
| AdminResponsiveTablePattern | `lib/admin/AdminResponsiveTablePattern.svelte` | new (column hiding below 1280, scoped horizontal overflow) | all admin tables | 3B4 |
| Ringkasan | `routes/admin/+page.svelte` | new | `/admin` | 3B4 |

### 8.4 Existing components: disposition

| Existing artifact | Disposition | Reason |
|---|---|---|
| `lib/components/dashboard/BedAvailabilityDashboard.svelte` | **REPLACE** (content), **DELETE LATER** | Demo stage: hardcoded `f1`-`f6` facilities, fake bed availability, Chaos Mode, gamified log, emoji buttons, geolocation mock. D12 removes all of it from production surfaces. The queue-submission logic is the only salvageable part and moves into `WalkInForm`. |
| `lib/components/ReferralMap.svelte` | **DELETE LATER** | MapLibre over hardcoded coordinates; `facilities/nearby` is not a real backend source. Not in the target inventory. |
| `routes/+layout.svelte` | **REFACTOR** | Becomes a thin shell that delegates to `CitizenHeader`/`CitizenBottomNav` for citizen routes and `AdminShell` for admin routes. Currently it renders one hybrid header for every audience. |
| `routes/+page.svelte` | **REPLACE** | Becomes Beranda with QuickActions; drops the dashboard. |
| `routes/appointments/new/+page.svelte` | **REFACTOR** | Logic is correct and tested by `build-verification.test.js`; internals move into `BookingStepper` + `BookingSummary`. Preserve the `checkin_code` / `result.id` markers the test asserts. |
| `routes/appointments/check-in/+page.svelte` | **REFACTOR** | Preserve deep-link prefill (`appointment_id`/`id`, `checkin_code`/`code`) and the `appointment_id`/`queue_ticket_id`/`formatted_number` markers the test asserts. |
| `routes/patient/status/+page.svelte` | **REFACTOR** | Preserve the two-stage code lookup and the status label maps. |
| `routes/auth/login`, `register`, `logout` (`+page.server.ts`) | **KEEP unchanged** | Actions are covered by `tests/auth-actions.test.js` (400/401/503/303 paths). Touching them risks that regression suite for zero design benefit. |
| `routes/auth/*/+page.svelte` | **REFACTOR** | Presentation only; keep field names `email`, `password`, `confirm` so the actions and the test keep passing. |
| `routes/wallet/+page.svelte` | **DELETE LATER** (excluded from nav now) | D7. Its data path is dead: it fetches `/api/v1/medical-records`, which has neither a proxy nor a backend route. Leave the file in place during the redesign phase; remove from navigation immediately. |
| `routes/admin/queues/+page.svelte` | **REFACTOR** | Preserve the `updated_at` marker asserted by `build-verification.test.js`. |
| `routes/admin/appointments/+page.svelte` | **REFACTOR** | Preserve the `updated_at` marker. Replace the incorrect per-row transition buttons (they currently offer `checked_in -> completed`, which the backend rejects with 400) with the Section 4.2 map. |
| `routes/admin/schedules/+page.svelte` | **REFACTOR** | Replace raw UUID inputs with scoped selects; add the ForbiddenPanel path for missing `schedule.manage`. |
| `routes/admin/facilities/+page.svelte` | **REFACTOR** | Replace native `confirm()` with `Dialog`; fix the deactivate call to `PATCH`; handle the string `"false"` response. |
| `routes/admin/notifications/+page.svelte` | **REFACTOR** | Best existing page; keep the skeleton, split list/summary errors, URL-synced filters, and masked-recipient discipline. Fix the `relativeTime()` ordering bug (UX audit finding 13). |
| `lib/types.ts` | **REFACTOR** | Split into `lib/api/types/api.ts` (wire types) and domain types. Keep `MedicalRecord` only while `/wallet` exists. |
| `lib/server/auth.ts`, `lib/supabase/server.ts`, `hooks.server.ts` | **KEEP** | Security-critical and working. `hooks.server.ts` gains nothing in 3B0-3B5; admin gating stays at the API. |
| `app.css` | **REFACTOR** | Keeps the Tailwind directives; the `--accent: #059669` variable is replaced by the frozen token set. |
| `tailwind.config.ts` | **REFACTOR** | Extend theme with semantic token colours, the 6/8/12 radius scale, and the two control heights. |

No existing component is deleted during Phase 3A. Deletions are scheduled in 3B6 after the replacements are
verified, and `wallet` deletion is deferred beyond the redesign entirely (D7).

---

## 9. Route Migration Matrix

For every production route. "Regression risk" is judged against the automated tests that exist today plus the
new tests planned in Section 13.

| Route | Current implementation | Frozen design target | Dependencies | Phase | Regression risk |
|---|---|---|---|---|---|
| `/` | Renders `BedAvailabilityDashboard`: 6 hardcoded facilities, fake bed bars, Chaos Mode, geolocation mock, referral map, SSE listener | Beranda: quick actions, real catalog summary, 3-step "how it works", no demo content | `lib/ui/*`, `CitizenHeader`, `CitizenBottomNav`, `QuickActions`, public facilities endpoint | 3B2 | **Medium.** `build-verification.test.js` asserts `types.ts` exports; keep them until 3B6. No test covers the dashboard itself. |
| `/faskes` | **Does not exist** | Cari Faskes: facility list with type filter, loading/empty/no-result/error states | `FacilitySearch`, `FacilityResultRow`, `GET /api/v1/public/facilities` | 3B2 | **Low.** New route; no existing behavior to regress. |
| `/appointments/new` | Working 3-field+ flow: parallel catalog load, client-side service filter by `facility_id`, fallback manual UUID input, POST, success card with copy + deep link | 3-step stepper, desktop two-column form + summary, same backend contract | `BookingStepper`, `BookingSummary`, `Select`, `Input`, `Field`, public catalogs, `POST /api/v1/appointments` | 3B3 | **Medium-high.** `build-verification.test.js` asserts `checkin_code` and `result.id` are present. The 429/409/400 error copy must be preserved. |
| `/appointments/check-in` | Working: deep-link prefill from `appointment_id`/`id` + `checkin_code`/`code`, POST, success with `formatted_number`, `estimated_wait_minutes` | Check-In Janji Temu: prefilled form, prominent queue ticket, next-step CTA to Status, plus a cross-link to `/queues/new` | `CheckinForm`, `QueueTicket`, `CodeDisplay`, `POST /api/v1/appointments/{id}/check-in` | 3B3 | **Medium-high.** Asserted markers `appointment_id`, `queue_ticket_id`, `formatted_number`. Error taxonomy (401 wrong code, 404 not found, 409 wrong state, 429) must be preserved exactly. |
| `/queues/new` | **Does not exist** | Ambil Antrean Tanpa Janji: name + phone form, queue ticket result, cross-link back to Check-In | `WalkInForm`, `QueueTicket`, public facilities, `POST /api/v1/queues/generate` | 3B3 | **Low.** New route. Contract must stay separate from check-in (R3-F2). |
| `/patient/status` | Working: single code input, two-stage lookup, status label maps | Status Kunjungan: code form, current-state VisitProgress (Check-In -> Antre -> Dilayani -> Selesai), no fabricated history | `VisitProgress`, `CodeDisplay`, `GET /api/v1/patient/status?code=` | 3B3 | **Medium.** `checkin_status` -> `queue_status` -> progress mapping must match `mapCheckinStatus` (`patient.go:153-170`). |
| `/auth/login` | Working form action: 503 unconfigured, 400 empty, 401 bad credentials, 303 redirect; `?registered=1` banner | Masuk: same behavior, restyled | `AuthForms`; **`+page.server.ts` untouched** | 3B3 | **High if the action is touched.** `tests/auth-actions.test.js` asserts all four paths. Presentation-only change. |
| `/auth/register` | Working: 503/400 (min 8, mismatch)/303 to login | Daftar: same behavior, restyled | `AuthForms`; action untouched | 3B3 | **High if the action is touched.** Test asserts mismatch 400 and success 303. Presentation-only. |
| `/auth/logout` | Action signs out and redirects; page is an interstitial | Keluar: restyled interstitial | `+page.svelte` only; action untouched | 3B3 | **Low.** Test asserts the 303. |
| `/admin` | **Does not exist** | Ringkasan: derived counts from already-loaded scoped endpoints, each labelled with source + last-updated time, no invented KPI | `AdminShell`, `AdminSidebar`, all admin list endpoints, client-side aggregation | 3B4 | **Low.** New route. Must render the empty-scope state as a class-1 state. |
| `/admin/queues` | Working: list, raw-UUID facility filter input, client-side transition map, PATCH, optimistic patch then reload | Antrean: QueueBoard (Dipanggil / Menunggu / Selesai), 30s polling + manual refresh + "Diperbarui pukul", scoped facility filter, inline status updates | `QueueBoard`, `FacilityFilter`, `polling.ts`, `PATCH /api/v1/admin/queues/{id}/status` (**proxy must be fixed first**) | 3B4 | **High.** The status-action path is currently broken end to end by the `ID` literal. `updated_at` is asserted by `build-verification.test.js`. |
| `/admin/appointments` | Working list; per-row buttons offer transitions the backend rejects (`checked_in -> completed`) | Janji Temu: table + filters, correct Section 4.2 transitions, no conflict state | `AppointmentRow`, `FacilityFilter`, `PATCH /api/v1/admin/appointments/{id}/status` (**proxy must be fixed first**) | 3B4/3B5 | **High.** Correcting the button set changes observable behavior, which is the point. `updated_at` asserted. |
| `/admin/schedules` | Working list + modal; raw UUID inputs for facility/service/practitioner; dev identity cannot mutate (403) | Jadwal: table + editor dialog with scoped selects; ForbiddenPanel when `schedule.manage` is absent; **practitioner is not shown or edited** (no read source; CREATE omits it, UPDATE preserves it via pointer PATCH) | `ScheduleEditor`, `Dialog`, `Select`, admin facilities + service-units endpoints, `PATCH /api/v1/admin/schedules/{id}` (**proxy must be fixed first**) | 3B4/3B5 | **High.** Proxy broken; also needs an explicit 403 path. |
| `/admin/facilities` | Working list + modal; native `confirm()` for deactivate; deactivate call is broken (PATCH vs POST proxy) | Fasilitas: table + editor dialog; dialog-confirmed one-way deactivate with public-catalog impact copy | `FacilityEditor`, `Dialog`, `PATCH /api/v1/admin/facilities/{id}/deactivate` (**proxy must be fixed first**) | 3B4/3B5 | **High.** Deactivate is currently unreachable; also must handle the string `"false"` response. |
| `/admin/notifications` | Most complete page: summary cards, URL-synced filters, skeletons, separate list/summary errors, masked recipient, retry/cancel | Notifikasi: keep all of the above; fix `relativeTime()` ordering; ensure retry/cancel hidden without `notification.manage` | `NotificationRow`, `OutboxSummary`, admin notifications endpoints, **P0-1/P0-2/P0-3 fixes** | 3B4/3B5 | **Medium.** The page is good; the backend it depends on has three P0 defects. Retry/cancel proxies are broken. |
| `/wallet` | Broken: fetches a non-existent `/api/v1/medical-records` | Excluded from navigation; code retained | none | n/a (excluded) | **None** while excluded. Removing the nav entry is part of 3B2. |

### 9.1 Route-level non-negotiables

1. No existing URL is renamed.
2. `/faskes`, `/queues/new`, and `/admin` are the only additions.
3. `/queues/new` must never call the check-in endpoint.
4. Admin pages must not render a rate-limit state (no admin limits exist).
5. The mobile bottom nav keeps exactly four tabs; `/queues/new` is not one of them.
6. `/wallet` leaves navigation but is not deleted in this phase.

---

## 10. Data Join Plan

Purpose: resolve foreign keys to human-readable names in the Admin UI without turning the client into an
authorization boundary.

### 10.1 Required joins

| Display need | Source id | Lookup source | Notes |
|---|---|---|---|
| Queue facility name | `queue.facility_id` | `GET /api/v1/admin/facilities` (scoped) | exact id match |
| Appointment facility name | `appointment.facility_id` | same | exact id match |
| Schedule facility name | `schedule.facility_id` | same | exact id match |
| Appointment service name | `appointment.service_unit_id` | `GET /api/v1/admin/service-units` (scoped) | must match id **and** be within the permitted facility scope; if unresolved, render `-` |
| Schedule service name | `schedule.service_unit_id` | same | never infer from facility type, code prefix, or a hardcoded map |
| Practitioner | `practitioner_id` | **none exists** | keep out of user-facing display entirely; never invent a name. No raw-UUID or free-text practitioner field in the schedule editor (Section 7.3): CREATE omits `practitioner_id`; UPDATE preserves the existing value because the backend PATCH is pointer/omitempty. No fake practitioner catalog. |
| Notification facility name | row `facility_id` | scoped facilities | row may have `facility_id = NULL` (global); render `-` or omit |

Verified response shapes to map against (all under `{"success":true,"data":[...]}`):

- `facilityResponse` (`admin.go:49-64`): `id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, is_active, short_code, created_at?, updated_at?`
- `serviceUnitResponse` (`admin.go:869-878`): `id, facility_id, name, code?, description?, is_active, created_at?, updated_at?`
- `queueTicketResponse` (`admin.go:554-563`): `id, facility_id, queue_number, formatted_number, status, registered_at, called_at?, completed_at?`
- `scheduleResponse` (`admin.go:1312-1324`): `id, facility_id, practitioner_id?, service_unit_id, schedule_date, start_time, end_time, slot_minutes, capacity_per_slot, is_active, created_at?, updated_at?`
- `appointmentResponse` (`admin.go:1929-1942`): `id, facility_id, service_unit_id, practitioner_id?, practitioner_schedule_id?, appointment_time?, status, patient_display_name, checkin_code?, queue_ticket_id?, notes?, created_at?, updated_at?`

### 10.2 Load strategy

- **Parallel loads.** Each Admin module issues its primary list request and its lookup requests concurrently
  (`Promise.all`). Example for Antrean: `[GET /admin/queues, GET /admin/facilities]`. Example for Jadwal:
  `[GET /admin/schedules, GET /admin/facilities, GET /admin/service-units]`.
- **No waterfall.** A lookup must never be sequenced behind the primary list unless it depends on an id that
  only the list can produce (it does not; facilities and service-units are independently listable).
- **Single fetch per lookup per module load.** Not per row.

### 10.3 Failure behavior

| Condition | Behavior |
|---|---|
| Primary list fails | Module-level `ErrorState` with retry. Do not render a partial table. |
| Lookup source fails (e.g. facilities 403 or 500) | **Render the table anyway**, with the affected name cells showing `-`, plus a non-blocking inline notice: the id could not be resolved. Rationale: the primary operational data is still useful, and hiding the whole table because a secondary lookup failed would be a worse failure. The notice must be text, not colour-only. |
| Lookup returns 403 (missing `facility.read`) | Treat as "no lookup available" -> all name cells `-`, and surface the permission notice once at module level. |
| Scope-resolution failure on the primary list | The backend returns 200 with an empty array; render the empty-scope state, not an error. |

### 10.4 Unresolved id behavior

| Case | Rendering |
|---|---|
| id present, no matching record in the lookup result | `-` |
| id present but the record is outside the actor's scope (so absent from the scoped list) | `-` |
| id absent/null on the row (`practitioner_id`, `queue_ticket_id`, global `facility_id`) | `-` or the field is omitted |
| Never | Never render a raw UUID in a name position. Raw identifiers belong only in an opt-in "Detail" disclosure. |

### 10.5 Cache scope

- **Route-local.** Each Admin route builds its lookup maps inside its own load and discards them on navigation.
- **No shared/global cache.** Rationale: data is small (a scoped facility list is typically tens of rows), and a
  shared cache introduces staleness questions that the "Diperbarui pukul ..." contract (D8) would then have to
  answer for two different timestamps.
- **Map construction.** Build `Map<string, string>` keyed by exact id, once per load, and reuse for every row.
- **Polling interaction (Antrean only).** The facilities lookup is loaded once at mount and is **not** re-fetched
  on each 30s poll. Only the primary queue list is polled. If a queue row references a facility id absent from
  the cached map, it renders `-` until the next full reload. This is deliberate: facility metadata changes far
  less often than queue state, and re-fetching it every 30s would double the request volume for no gain.

### 10.6 Facility scope stays server-authoritative

This is the hard rule:

1. The client never sends a facility id as an authorization claim. `?facility_id=` on notifications is honoured
   by the backend only when it is inside the actor's allowed set (`notifications.go:88-105`); everywhere else
   the backend ignores or does not accept it.
2. The client facility filter is a **narrowing control over an already-scoped dataset**. Its label must express
   that: "Menampilkan: X dari Y" (per D9), never "filter" language that implies the server narrowed the query.
3. If a client filter selects a facility that the server did not return, the result is an empty list with the
   no-result state, not an error and not a re-fetch with a different claim.
4. The Ringkasan counts are computed from the already-scoped, already-loaded lists. They must never be computed
   from a global source.
5. Name resolution must never be used to decide access. A row whose facility name resolves to `-` is still
   displayed if the server returned it; the server already decided the row is in scope.

---

## 11. Polling Plan

Scope: **Admin Antrean only** (`/admin/queues`), per D8. Approximately 30 seconds. No SSE, no WebSocket.

### 11.1 Required behaviors

| Aspect | Plan |
|---|---|
| Initial load | Immediate fetch on mount. Skeleton shown until the first response resolves. The last-updated timestamp is set from the client receive time of the first successful response. |
| Interval | `setInterval` at 30000 ms, created only after the first load completes, so a slow first load does not immediately stack a second request. |
| Overlap guard | An in-flight flag prevents a new tick from starting while the previous request is unresolved. Without this, a slow backend produces request pile-up. |
| `visibilitychange` | On `document.hidden === true`, clear the interval (pause, not merely slow). On becoming visible, perform one immediate refresh and restart the interval. Rationale: a background tab polling every 30s is pure waste and, for an operator dashboard left open overnight, it is sustained load. |
| Manual refresh | A "Muat ulang" control triggers the same refresh path as a tick, bypasses the interval, respects the overlap guard, and shows a brief inline busy state on the button. |
| Stale-data behavior | On a refresh failure, **keep the previously loaded rows visible** and switch the timestamp line to a stale warning ("Gagal memperbarui. Data terakhir pukul HH:MM."). Never blank the table on a failed refresh; the operator still needs the last known board. |
| Abort / cancellation | Every request is created with an `AbortController`. The controller is aborted on: component destroy, and on a new manual refresh superseding an in-flight poll. Aborting must be treated as a no-op, not an error state. |
| Error behavior | First-load failure -> module `ErrorState` with retry. Refresh failure -> stale warning as above, no error panel. A 403 on refresh -> switch to the forbidden/unauth panel (the session or permission changed). |
| Last-updated timestamp | Always rendered as `Diperbarui pukul HH:MM` in `id-ID` local time. Must be text, must be present in every state including stale, and must never claim realtime. |
| Cleanup on navigation | `onDestroy` clears the interval, removes the `visibilitychange` listener, and aborts any in-flight request. |
| Copy discipline | The word "realtime" must not appear anywhere in the Antrean UI. |

### 11.2 Shape

`lib/domain/polling.ts` exports a single helper, for example `createPolling({ fetcher, intervalMs, onData, onError, onStale })`,
returning `{ start, stop, refresh, isRefreshing }`. The component owns the state; the helper owns the timer,
the overlap guard, the abort controller, and the visibility listener. One helper, used once, but written so a
second polled surface could reuse it without modification.

### 11.3 Explicit non-goals

- No polling on any other Admin page. Other modules use manual refresh only (per D8, polling is scoped to the
  operational queue view).
- No optimistic removal of rows that disappear between polls without a visible reason; if a row vanishes, the
  list simply re-renders. Silent disappearance is acceptable only for queue rows that the server no longer returns.
- No exponential backoff in this phase. If a refresh fails, the next scheduled tick still fires. (Backoff is a
  reasonable 3B6+ refinement; it is not required for correctness.)

---

## 12. Accessibility Implementation Plan

Target: WCAG 2.2 AA as an implementation target (not a certification claim). The handoff's Section 8 is the
baseline; this section turns each item into a concrete, testable requirement.

| Requirement | Concrete implementation | Acceptance criterion |
|---|---|---|
| Focus trapping | `lib/ui/Dialog.svelte` wraps the native `<dialog>` element, opened with `showModal()`, which provides a real focus trap, an inert background, and a top-layer stack without hand-rolled tab-cycling code | Tab and Shift+Tab cycle only within the dialog; background is not reachable |
| Escape closes | Native `<dialog>` fires `cancel` on Escape; the component handles it, closes, and does not lose data silently | Escape closes the dialog; if the dialog holds unsaved input, a confirmation step is shown |
| Return focus | On close, focus returns to the element that opened the dialog (recorded at open time) | After Escape and after the confirm action, focus is on the trigger button |
| Dialog semantics | `role="dialog"`, `aria-modal="true"`, `aria-labelledby` pointing at the dialog title, `aria-describedby` for the body when useful | Screen reader announces the dialog name on open |
| Live regions | A single polite live region per Admin module for mutation results and refresh announcements. The queue refresh announces "Data diperbarui pukul HH:MM", not every cell | Announced once per refresh, not per row |
| Table refresh announcements | The live region must **not** be the table container itself | The accessibility tree does not re-announce the whole table on each poll |
| Keyboard table actions | Row action buttons are real `<button>` elements in tab order, not click-handler `<tr>`s. No action is reachable only by mouse | Every transition can be performed with keyboard only |
| `aria-current` | Citizen nav and Admin sidebar set `aria-current="page"` on the active destination. The frozen HTML already does this (`sw-nav-link[aria-current="page"]`, `sw-m-tab[aria-current="page"]`) | Exactly one `aria-current="page"` per nav |
| `aria-invalid` + error association | Every invalid control gets `aria-invalid="true"` and `aria-describedby` pointing at the field error element, which is rendered below the control | Screen reader reads the error when the field is focused |
| Error text placement | Label above control, helper text optional, error text below control | Matches the frozen form anatomy |
| Loading announcements | Loading states use `aria-busy="true"` on the region; skeletons are `aria-hidden="true"` so they are not read as content | A screen reader user hears "busy" not the skeleton shapes |
| Icon-rail accessible labels | At the ~1024px collapse, each sidebar item keeps a visible-on-focus tooltip **and** an `aria-label`; the icon itself is `aria-hidden="true"` | Rail items are announced by name when collapsed |
| Decorative icons | All decorative glyphs get `aria-hidden="true"`; icons never carry meaning alone | No icon-only meaning |
| Status not colour-only | `StatusBadge` always renders the label text plus colour plus an optional icon | Status is legible in greyscale |
| Touch targets | Citizen controls >= 44px; Admin controls 36-40px | Measured in the browser at 390px and 1440px |
| Focus visibility | `:focus-visible` rings with sufficient contrast on every interactive element, including within dialogs and tables | Visible ring on keyboard focus, absent on mouse click |
| Heading hierarchy | One logical `h1` per page, ordered `h2`/`h3` within | No skipped levels |
| Links vs buttons | Navigation uses `<a>`/SvelteKit `<a>`; actions use `<button>` | No `<a href="#" onclick>` action patterns |
| Readonly for identifiers | Displayed ids (appointment id, check-in code, queue number) use `readonly` inputs or plain text with a copy button, never `disabled` | Identifier remains copyable and readable |
| Reduced motion | Any transition added in 3B1 honours `prefers-reduced-motion: reduce` by degrading to an instant or opacity-only change | No transform-based motion under reduced motion |

### 12.1 Static-design gaps that become implementation acceptance criteria

These cannot be guaranteed by the frozen HTML and are therefore explicit acceptance criteria on the tasks that
own them:

1. Dialog focus trap, Escape, and focus return (facilities editor, schedules editor, deactivate confirmation).
2. Live-region announcements for queue refresh and mutation results.
3. `aria-invalid` and error association on every form field in booking, check-in, walk-in, status, and auth.
4. Keyboard reachability of every Admin table row action.
5. Accessible names on the collapsed Admin icon rail.
6. Focus return after the deactivate confirmation, and after a successful mutation.

---

## 13. Test Strategy

Preference: automation over manual QA. Manual QA is not the primary acceptance mechanism.

### 13.1 Unit and component tests

| Target | What is tested | Runner |
|---|---|---|
| `lib/domain/status.ts` | Queue and appointment transition maps produce exactly the backend's allowed sets; label maps cover every enum value; unknown status falls through safely | Vitest |
| `lib/domain/joins.ts` | id -> name resolution: exact match, missing id -> `-`, out-of-scope id -> `-`, null id -> `-`, duplicate ids | Vitest |
| `lib/domain/format.ts` | `id-ID` date/time formatting, the corrected `relativeTime` ordering (minutes vs hours), phone normalisation | Vitest |
| `lib/domain/polling.ts` | Interval fires, overlap guard suppresses concurrent ticks, `visibilitychange` pauses and resumes with an immediate refresh, abort is a no-op, cleanup stops the timer and listener | Vitest with fake timers |
| `lib/api/errors.ts` | 400/401/403/404/409/429/500 plus network failure and abort map to the correct discriminated variants; 403 with and without a session selects the right panel; trailing-newline bodies parse | Vitest |
| `lib/ui/*` | `Dialog` trap/Escape/return-focus; `StatusBadge` renders label + colour + icon; `Field` associates label, helper, error, and `aria-invalid` | `@testing-library/svelte` + jsdom |

### 13.2 Backend and API tests

These are Go tests and must satisfy the CI naming/coverage constraints in Section 1.7.

| Target | What is tested |
|---|---|
| P0-1 regression | Out-of-scope notification retry returns 404 **and** leaves the row unmutated; in-scope succeeds; absent id 404; wrong state 409 |
| P0-2 regression | Same matrix for cancel, including idempotent re-cancel |
| P0-3 regression | Zero-assignment **non-super_admin** summary is all-zero; facility-scoped summary is scoped; dev or DB-resolved active global `super_admin` is global; scope-resolution failure is all-zero; all five keys always present |
| P0-4 / P0-5 regression | Table-driven over **every** discovered facility-scope consumer (all five Admin LIST paths, all DETAIL/MUTATION paths including the queue/appointment status mutations and the two create-with-supplied-`facility_id` paths, and all notification paths): zero-assignment **non-super_admin** actor reads nothing and mutates nothing; a DB-resolved active global `super_admin` receives **global** LIST results (never `[]` because its `IDs` are empty), can read/mutate any facility-scoped record, and still receives 403 on a route whose permission it lacks; facility-A actor cannot touch facility-B by id; dev remains unrestricted; inactive/soft-deleted global assignments and resolver errors are fail-closed |
| Super_admin security matrix (Phase 3A.1, required) | **CASE A** - dev actor: unrestricted via the dev-only path (`actor.IsDev`), separate from super_admin semantics. **CASE B** - DB-resolved active global `super_admin` (Section 4.5.1): unrestricted facility scope, **and** each admin route still denies requests missing its per-route permission (unrestricted scope never grants permissions). **CASE C** - `facility_admin` assigned facility A: may access A; may not access B (404, zero side effect). **CASE D** - non-super_admin with zero active facility assignments: list = 200 + empty; detail = 404; mutation = 404 with zero side effects; notification summary = all five status keys present at zero. **CASE E** - scope-resolution error: fail closed (404 detail/mutation; empty/all-zero lists/summary; never unrestricted). **CASE F** - inactive or soft-deleted global `super_admin` assignment: MUST NOT grant unrestricted scope (behaves as CASE D). **CASE G** - role/permission claims supplied only by client/JWT/token claims: MUST NOT grant unrestricted scope; only the DB-resolved assignment does. All cases use DB-resolved authorization only. |
| Facility scope | Extends the existing `TestFacilityScope` family; must keep that name prefix so the CI grep keeps proving execution |
| Notification mutation authz | The two regression suites above; must run under the `TestCheckIn`/`TestRBACResolver`-style DB-backed pattern so CI's skip-check applies |
| Proxy path tests | Assert each proxy's forwarded path interpolates `event.params.id` and does not contain a literal `ID`/`STATUS`; assert the deactivate proxy exports `PATCH` |
| Method contract tests | Deactivate is `PATCH` end to end (UI call, proxy export, Go route); check-in is `POST`; notification retry/cancel are `POST`; queue/appointment status are `PATCH` |
| Router coverage | Any new `Registry` entry keeps `internal/router` coverage >= 90% (CI-enforced) |

### 13.3 Frontend tests

| Target | What is tested |
|---|---|
| Route rendering | Each citizen route renders its normal state; each admin route renders with a mocked proxy |
| Form validation | Booking phone 10-15 digits and future time; walk-in required name/phone; check-in required id/code; status required code; auth min-8 and mismatch |
| Error states | Each mapped error variant renders the correct panel: `UnauthPanel`, `ForbiddenPanel`, `ErrorState`, rate-limit, empty, no-result, stale |
| Empty-scope state | Admin lists render the class-1 empty-scope message, not a generic empty state |
| Responsive behavior | Citizen bottom nav present at 390px and absent at 1440px; Admin sidebar full at 1440px and icon rail at ~1024px; table columns hidden below 1280px |
| Accessibility assertions | `aria-current`, `aria-invalid` + association, dialog semantics, live region presence, focus return |

### 13.4 E2E and browser automation

Tooling: **Playwright**, wired as a repo-level `@playwright/test` suite so CI can run it, and developed
against the `mcp_Playwright` / `integrated_browser` tooling available in this environment. The repo has no
browser automation today. **Introduction order (Phase 3A.2/3A.3 correction):** the Playwright E2E tooling
foundation is established as the **first task of 3B1**, T-3B1-01 ("Add unit/component/E2E test tooling
foundation": `@playwright/test`, `playwright.config.ts`, repo E2E script/command, baseline `apps/web/e2e/`
structure, a local-seeded-stack target, and a hard guard that refuses a production target). The same task
establishes Vitest, `@testing-library/svelte`, and jsdom, so no unit, component, or E2E test anywhere in the
plan is required before its runner exists. E2E scenarios are then authored **incrementally from 3B2 onward**
as each stage lands. The suite is **completed and integrated into CI in 3B6** (T-3B6-01), which does not
introduce Playwright for the first time. Gates that require E2E (GATE 3, GATE 4, GATE 5) therefore all find
the runner in place, because 3B1 - and hence the Playwright foundation - finishes before GATE 2.

| Flow | Assertions |
|---|---|
| Citizen critical path | Beranda -> Faskes -> booking -> success code -> check-in -> queue number -> status lookup, using a seeded local stack |
| Booking | Validation failures, 429 copy, success card, deep link carries `appointment_id` + `checkin_code` |
| Prefilled check-in | Arriving with query params prefills both fields; wrong code -> the wrong-code state (401 mapping); not found -> 404 state |
| Walk-in | `/queues/new` produces a queue number and does **not** touch the check-in endpoint |
| Status lookup | Found by check-in code and found by formatted number both render the progress indicator |
| Auth | Login success, login failure, register, logout, unconfigured 503 |
| Admin queue actions | Polling updates the timestamp; a valid transition succeeds; an invalid transition surfaces the 400 validation message |
| Admin appointment actions | Only Section 4.2 transitions are offered; an offered transition succeeds |
| Schedule management | Create/update with scoped selects; missing `schedule.manage` shows ForbiddenPanel |
| Facility management | Create/update; deactivate via dialog succeeds (proves the PATCH fix) and the row shows inactive |
| Notification actions | Summary cards render; retry/cancel succeed in scope; out-of-scope is not reachable from the UI; masked recipient never leaks |

### 13.5 Test environment

- Local stack via `make dev` (docker compose) with `SIGAP_ENV=local` so dev identity and seeds are available.
- E2E must be able to run against seeded data. Per T-3B0-08 the seed provides three identities: (1) a
  facility-scoped admin (sees data, CASE C); (2) a zero-assignment **non-super_admin** actor (the
  fail-closed empty-scope states are reachable, CASE D); and (3) a global `super_admin` (the explicit
  unrestricted path is reachable, CASE B). No test may target a production environment.
- No test may run against production. The Playwright foundation (T-3B1-01) enforces this with a hard
  guard that refuses a production target; the only supported E2E target is the local seeded stack.
- Auth E2E reuses the mock-Supabase approach already proven in `tests/auth-actions.test.js` where a real
  Supabase is unavailable.

### 13.6 Explicit non-goals

- No visual-regression screenshot diffing in this phase.
- No load or performance testing (the queue engine already has a concurrency guardrail test).
- No accessibility audit tooling gate (axe can be run manually; it is not a merge gate in 3B).

---

## 14. Implementation Sequencing

Phases are dependency-aware. Each phase is independently shippable and independently revertable. Production
behavior changes are called out explicitly.

### 3B0 - P0 remediation

| Aspect | Detail |
|---|---|
| Goal | Close the five P0 security defects and repair the nine broken proxies plus the deactivate method mismatch, **before** any visual work, because the redesign's Admin surfaces depend on working endpoints. |
| Files / modules | `apps/api/internal/handler/notifications.go`, `apps/api/internal/handler/admin.go`, `apps/api/internal/notification/service.go`, `apps/api/internal/auth/facility_scope.go` (possibly), plus the 9 proxy files under `apps/web/src/routes/api/v1/admin/**` and the corresponding Go tests. |
| Prerequisites | None. This phase is independent of the design system and of every other phase. Internal order: the common facility-scope contract (T-3B0-07) lands first, then the notification fixes that consume it (T-3B0-04, T-3B0-05, T-3B0-06), then the seed verification (T-3B0-08) and GATE 1 (T-3B0-09). The proxy repairs (T-3B0-02, T-3B0-03) are independent frontend work. |
| Acceptance criteria | P0-1..P0-5 regressions pass; **every** facility-scope consumer discovered by the source audit is migrated to the explicit `FacilityScopeResult` contract and covered by the table-driven matrix; all 23 proxies audited correct; deactivate works end to end with `PATCH`; `go test ./...` green; router coverage still >= 90%; `pnpm --filter sigap-web run check` green. |
| Automated tests | Section 13.2 in full, plus the proxy path/method contract tests. |
| Rollback strategy | Single revert of the phase commit. No migration, so no data rollback is required. The API is stateless; redeploying the previous image restores prior behavior exactly. |
| Production behavior changes | **YES, deliberately.** Zero-assignment **non-super_admin** actors lose all read/mutation access (this is the security fix). A DB-resolved active global `super_admin` keeps unrestricted access, now explicit (Section 4.5). Cross-facility notification retry/cancel stops working (it should never have worked). Notification summary stops leaking global counts to non-super_admin actors. Admin queue/appointment/schedule/facility/service-unit detail and mutation endpoints start responding correctly through the fixed proxies. All changes are tightening, so no legitimate client regresses. |
| Risk note | The `super_admin` global-role case was decided at Phase 3A.1 (Section 4.5): global unrestricted administration is required and is modelled as an explicit, DB-resolved unrestricted scope. A DB-resolved active global `super_admin` keeps unrestricted access; zero-assignment non-super_admin actors are denied. Do not revert the fix to restore accidental access. |

### 3B1 - Test tooling foundation, shared primitives, tokens, helpers

| Aspect | Detail |
|---|---|
| Goal | Establish the complete test tooling foundation **first** (T-3B1-01), then the design system in code and the shared helpers, with no route yet consuming them visibly. |
| Files / modules | **T-3B1-01** (tooling): `apps/web/package.json`, `apps/web/vitest.config.ts`, test setup file, `apps/web/playwright.config.ts`, repo E2E script/command, baseline `apps/web/e2e/`. Then T-3B1-02..07: `lib/design/tokens.css`, `lib/design/tokens.ts`, all of `lib/ui/*`, `lib/api/{client,errors}.ts`, `lib/api/types/api.ts`, `lib/domain/{status,joins,format,polling}.ts`, `tailwind.config.ts`, `app.css`, `lib/stores/session.ts`. |
| Prerequisites | **GATE 1 must be passed first (3B0 complete).** 3B1 MUST NOT begin before GATE 1: the redesign builds on the corrected authorization/proxy contract and must not develop against knowingly broken security behavior. Within 3B1, the test tooling foundation (T-3B1-01) must land before any test-dependent task. |
| Acceptance criteria | Token values match `DESIGN.md` exactly (primary `#0F766E`, hover `#0B6B63`, active `#084F49`, canvas `#F7F6F3`, surface `#FFFFFF`, foreground `#1C1B1A`, muted `#57534E`, border `#E0DDD8`, success `#2F7D32`, warning `#B45309`, danger `#C4322A`, info `#1D6BB5`); radii 6/8/12; control heights 44 and 36-40; one icon family at one stroke width; zero gradients, zero glassmorphism, zero decorative shadows. GATE 2 (T-3B1-08) additionally verifies the Vitest and Playwright runners, the local E2E smoke scenario, and the production-target guard. |
| Automated tests | Section 13.1 in full, plus the T-3B1-01 E2E smoke scenario and production-target guard test. |
| Rollback strategy | Revert the phase commit. Nothing user-visible depends on it yet. |
| Production behavior changes | **NO.** Additive only. |

### 3B2 - Citizen shell and public catalog foundation

| Aspect | Detail |
|---|---|
| Goal | Ship the Citizen shell, Beranda, and `/faskes`, and remove the demo stage from the public entry point. |
| Files / modules | `routes/+layout.svelte` (refactor), `routes/+page.svelte` (replace), `routes/faskes/+page.svelte` (new), `lib/citizen/{CitizenHeader,CitizenDesktopNav,CitizenBottomNav,QuickActions,FacilitySearch,FacilityResultRow,AccountMenu}.svelte`. Remove `/wallet` from navigation. |
| Prerequisites | 3B1 complete and GATE 2 passed. |
| Acceptance criteria | Beranda contains no demo content (no bed bars, no chaos mode, no gamified log, no emoji buttons, no fake metrics); mobile bottom nav has exactly four tabs; desktop nav is one line; `/faskes` renders normal/loading/empty/no-result/error states; `/wallet` is unreachable from navigation. |
| Automated tests | Route rendering, empty/no-result/error states, responsive nav assertions, `aria-current` assertion. |
| Rollback strategy | Revert the phase commit; the old root page returns. |
| Production behavior changes | **YES.** The public home page changes entirely, and `/faskes` becomes available. This is the first user-visible redesign phase. |

### 3B3 - Citizen transactional flows

| Aspect | Detail |
|---|---|
| Goal | Redesign booking, check-in, walk-in, status, and auth presentation. |
| Files / modules | `routes/appointments/new/+page.svelte`, `routes/appointments/check-in/+page.svelte`, `routes/queues/new/+page.svelte` (new), `routes/patient/status/+page.svelte`, `routes/auth/{login,register,logout}/+page.svelte`, `lib/citizen/{BookingStepper,BookingSummary,CheckinForm,WalkInForm,QueueTicket,VisitProgress,AuthForms}.svelte`. `+page.server.ts` files untouched. |
| Prerequisites | 3B2 complete (strict order: the citizen stage is 3B2 then 3B3; GATE 3 is the start gate for 3B4). |
| Acceptance criteria | All asserted markers preserved (`checkin_code`, `result.id`, `appointment_id`, `queue_ticket_id`, `formatted_number`); deep-link prefill works; `/queues/new` never calls check-in; the full error taxonomy renders correctly (401 wrong code, 404, 409, 429, 400 validation); auth actions still return 400/401/503/303; `tests/auth-actions.test.js` passes unchanged. |
| Automated tests | Section 13.1 component tests for forms; Section 13.3 validation and error-state tests; Section 13.4 citizen critical path E2E. |
| Rollback strategy | Revert the phase commit. |
| Production behavior changes | **YES.** All citizen transactional surfaces change visually. Backend contracts are unchanged. |

### 3B4 - Admin shell and read-only operational pages

| Aspect | Detail |
|---|---|
| Goal | Ship the Admin shell, Ringkasan, and the read paths of all six modules. |
| Files / modules | `routes/admin/+page.svelte` (new), `routes/admin/{queues,appointments,schedules,facilities,notifications}/+page.svelte` (read paths), `lib/admin/{AdminShell,AdminSidebar,AdminPageHeader,AdminToolbar,FacilityFilter,AdminTable,QueueBoard,QueueBoardRow,AppointmentRow,NotificationRow,OutboxSummary,AdminResponsiveTablePattern}.svelte`, `lib/ui/DataTable.svelte`. |
| Prerequisites | **3B3 complete and GATE 3 passed (strict Phase 3A.2 order).** Transitively: 3B0 (working proxies, GATE 1) and 3B1 (GATE 2). The admin stage does not start alongside the citizen stage. |
| Acceptance criteria | Six sidebar destinations with `aria-current`; icon rail at ~1024px with accessible labels; Ringkasan shows only derived, source-labelled counts; Antrean polls at 30s with pause-on-hidden, manual refresh, and "Diperbarui pukul"; no admin rate-limit state anywhere; empty-scope renders as a class-1 state; facility/service names resolve via joins or show `-`; raw UUID inputs are gone. |
| Automated tests | Join tests, polling tests, rendering tests, empty-scope tests, responsive assertions, Admin read E2E. |
| Rollback strategy | Revert the phase commit. |
| Production behavior changes | **YES.** Admin surfaces change and `/admin` becomes available. Read behavior only; no mutation semantics change in this phase. |

### 3B5 - Admin mutations

| Aspect | Detail |
|---|---|
| Goal | Wire every Admin mutation to the corrected contracts. |
| Files / modules | `lib/admin/{ScheduleEditor,FacilityEditor}.svelte`, `lib/ui/Dialog.svelte` consumers, mutation handlers in all five mutating modules, `lib/ui/Toast.svelte`. |
| Prerequisites | 3B4 complete and GATE 4 passed (admin read flows green). |
| Acceptance criteria | Appointment buttons offer exactly the Section 4.2 transitions; queue actions offer exactly Section 4.3; invalid transitions surface the 400 validation message verbatim; deactivate is dialog-confirmed, one-way, and handles the string `"false"`; schedule create/edit shows ForbiddenPanel when `schedule.manage` is absent; retry/cancel are hidden without `notification.manage`; dialogs satisfy trap/Escape/return-focus; mutation results announce via the live region. |
| Automated tests | Section 13.2 method/contract tests, Section 13.3 error-state tests, Section 13.4 Admin mutation E2E. |
| Rollback strategy | Revert the phase commit. |
| Production behavior changes | **YES.** Admin mutation affordances change; several previously broken actions start working. |

### 3B6 - Cross-product integration and automated E2E

| Aspect | Detail |
|---|---|
| Goal | Full-suite green, legacy deletion, cross-product consistency verification. |
| Files / modules | Delete `BedAvailabilityDashboard.svelte` and `ReferralMap.svelte`; remove the `maplibre-gl` dependency if nothing else uses it; finalise the Playwright suite; reconcile `lib/types.ts`. |
| Prerequisites | 3B5 complete and GATE 5 passed (GATE 5 subsumes 3B2, 3B3, 3B4; strict Phase 3A.2 order). |
| Acceptance criteria | Full Playwright suite green; Go suite green with the coverage gate; web check green; no orphaned routes; Citizen and Admin share tokens while keeping distinct density; no dead links; no `maplibre-gl` import remains. |
| Automated tests | The complete Section 13 suite, run as the integration gate. |
| Rollback strategy | Revert the phase commit; deletions are the only irreversible-looking change, and they are recoverable from git history. |
| Production behavior changes | **NO** functional change; only removal of dead code. |

### 3B7 - Production migration and deployment readiness

| Aspect | Detail |
|---|---|
| Goal | Prepare and document the production cutover. No deployment happens in 3A or in this planning document. |
| Files / modules | Deployment docs under `docs/operations/`, updated runbooks, a smoke-check script aligned with `scripts/smoke/`. |
| Prerequisites | 3B6 complete and GATE 6 passed. |
| Acceptance criteria | Build artifacts reproducible; environment variables documented (`SIGAP_API_INTERNAL`, `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY`, `SIGAP_DEV_IDENTITY`, `SIGAP_ENV`); rollback point identified per phase; smoke checks defined; `SIGAP_DEV_IDENTITY` confirmed off in production. |
| Automated tests | The existing `scripts/smoke/*` scripts, extended for the new routes. |
| Rollback strategy | Documented per phase; because no migration is introduced, rollback is a redeploy of the previous image. |
| Production behavior changes | **NO** by itself; it enables the deployment that follows. |

### 14.1 Dependency graph

```
GATE 0
  -> 3B0 (P0 security + broken proxies)
  -> GATE 1
  -> 3B1 (tokens/primitives/helpers + test tooling foundation)
  -> GATE 2
  -> 3B2 (citizen shell + catalog)
  -> 3B3 (citizen transactional flows)
  -> GATE 3
  -> 3B4 (admin shell + read pages)
  -> GATE 4
  -> 3B5 (admin mutations)
  -> GATE 5
  -> 3B6 (cross-product integration + full E2E/CI)
  -> GATE 6
  -> 3B7 (deployment readiness)
```

Strict canonical order (Phase 3A.2): **GATE 0 -> 3B0 -> GATE 1 -> 3B1 -> GATE 2 -> 3B2 -> 3B3 -> GATE 3
-> 3B4 -> GATE 4 -> 3B5 -> GATE 5 -> 3B6 -> GATE 6 -> 3B7.** One reviewable implementation stream.
3B1 MUST NOT begin before GATE 1: the redesign builds on the corrected authorization/proxy contract rather
than developing against knowingly broken security behavior. 3B4 MUST NOT begin before GATE 3: the citizen
stage proves the shared-component foundation (tokens, primitives, polling, joins, E2E runner) before the
admin conversion starts. No stage runs alongside another; every gate subsumes all earlier stages.

---

## 15. Zero-Downtime and Production Safety Plan

Production is live at `https://sigap.chaerulchalik.web.id/`. Nothing is deployed during Phase 3A.

### 15.1 Worktree-only development

- All work happens on the existing `design/ui-ux-overhaul` branch (or short-lived branches off it), never
  directly on `main`.
- The pending `.gitignore` change and the untracked `design/` tree must be committed (or deliberately left) as
  the first act of 3B0, so that design specs become version-controlled per D1 while `design/generated/` stays ignored.
- No production host access is required for any phase except 3B7's documented cutover.

### 15.2 Migration safety

- **No migration is introduced by any phase in this plan.** Every P0 fix and every redesign change is
  code-level. Section 16 records the per-fix verdict.
- Because there is no schema change, old application code and new application code are both compatible with the
  current database at every point during the rollout. This is what makes per-phase rollback a plain image swap.
- If a future phase genuinely needs a migration, it must be additive-only and must be applied before the code
  that depends on it, per Section 16.

### 15.3 Compatibility between the old UI and the updated backend

This is the key sequencing insight: **3B0 ships backend and proxy fixes while the old UI is still live.**

| Concern | Analysis |
|---|---|
| Does 3B0 break the current UI? | The current UI's admin mutations are already broken by the `ID`/`STATUS` literals, so fixing the proxies can only improve it. The current UI calls deactivate with `PATCH` while the proxy exports `POST`; changing the proxy to `PATCH` aligns the two. |
| Does the P0-4/P0-5 fix break the current UI? | Only for **non-super_admin** actors with zero active facility assignments, whose detail/mutation calls currently (incorrectly) succeed. The current UI would begin showing 404s for such actors. A DB-resolved active global `super_admin` is unaffected by the fix: it keeps unrestricted access, now explicitly and structurally (Section 4.5). This is the intended correction, but it is the one place where 3B0 changes observable behavior for an existing (misconfigured) user. It must be called out in release notes and verified against the seed data. |
| Does the P0-1/P0-2 fix break the current UI? | No. The current notification page only ever retries/cancels rows it loaded from a scoped list, so it never triggers the out-of-scope path. |
| Does the P0-3 fix break the current UI? | Only for a zero-assignment **non-super_admin** actor, who currently sees inflated global counts. Correcting them to zero is the intended behavior. A global `super_admin` continues to see global counts through its explicit unrestricted scope (Section 4.5). |
| Response shape changes | None. All P0 fixes preserve the response envelope and status codes for legitimate callers. |

### 15.4 Feature-independent P0 fixes

3B0 is deliberately independent of the design system. It can be reviewed, merged, and deployed as a
security/bugfix release without waiting for any visual work, and without the old UI needing to change. This is
the recommended first production deployment.

### 15.5 Build and test gates

Every phase must pass, before merge:

| Gate | Command |
|---|---|
| Go tests | `cd apps/api && go test ./...` |
| Go vet | `cd apps/api && go vet ./...` |
| Router coverage | `go test ./internal/router/ -coverprofile` and assert >= 90% |
| Rust tests | `cd apps/queue-engine && cargo test` |
| Web check | `pnpm --filter sigap-web run check` |
| Web test | `pnpm --filter sigap-web test` |
| E2E (3B2 onward) | Playwright suite against a local seeded stack |
| Secrets | gitleaks clean |

### 15.6 Deployment sequencing

1. Deploy 3B0 alone (backend + proxies + tests). Smoke-check admin reads and the citizen flows.
2. Deploy 3B1 (additive, no user-visible change) bundled with 3B2 if convenient, or alone.
3. Deploy 3B2, then 3B3 (citizen surface changes).
4. Deploy 3B4, then 3B5 (admin surface changes).
5. Deploy 3B6 (dead-code removal) only after the E2E suite is green in production-equivalent staging.
6. 3B7 documents the above; it does not add a deployment.

### 15.7 Rollback points

| Point | Rollback action |
|---|---|
| After 3B0 | Redeploy the previous API image and the previous web image. No DB action needed. |
| After 3B1 | Redeploy the previous web image. |
| After 3B2 / 3B3 | Redeploy the previous web image. Citizen URLs are preserved, so no link breakage. |
| After 3B4 / 3B5 | Redeploy the previous web image. `/admin` and `/faskes` would disappear; document this as a known effect of rolling back past 3B2/3B4. |
| After 3B6 | Redeploy the previous web image; deleted components return from git history. |

Because no migration exists, **no rollback ever requires a database restore.**

### 15.8 Smoke checks

Minimum post-deploy checks, aligned with the existing `scripts/smoke/` approach:

| Check | Expected |
|---|---|
| `GET /` | 200, Beranda renders, no demo content |
| `GET /faskes` | 200, catalog renders |
| `GET /api/v1/public/facilities` | 200 with `data` array |
| Booking end to end | 200 with `checkin_code` |
| Check-in with that code | 200 with `formatted_number` |
| `GET /queues/new` | 200, walk-in form renders |
| `GET /patient/status?code=<formatted>` | 200 with `found_by` set |
| `GET /admin` with a scoped admin session | 200, counts render |
| Admin queue status transition | 200 and the row updates |
| Admin deactivate (staging only) | 200 and `is_active` becomes false |
| Notification retry in scope | 200 |
| Notification retry out of scope | 404 **and** the row is unchanged (the P0-1 proof) |
| Zero-assignment **non-super_admin** actor on `/admin/notifications/summary` | all-zero counts (the P0-3 proof) |
| DB-resolved active global `super_admin` on `/admin/notifications/summary` | global counts (the Section 4.5 proof) |
| `SIGAP_DEV_IDENTITY` in production | must be unset/false |

### 15.9 Post-deploy verification

- Confirm no 5xx increase in API logs.
- Confirm the router's deny-by-default 401 rate is unchanged (a spike would indicate a proxy regression).
- Confirm no new 403s for the demo/scoped admin identities, which would indicate an over-tightened scope fix.
- Confirm the Admin Antrean last-updated timestamp advances during polling.

---

## 16. Database Change Policy

Rule: do not assume a migration is needed. Never edit an already-applied historical migration to support the
redesign. Additive migrations only.

| Proposed change | DB migration required? | Why |
|---|---|---|
| P0-1 notification retry authorization ordering | **NO** | Code-level ordering/scope change. The columns and indexes already exist. |
| P0-2 notification cancel authorization ordering | **NO** | Same. |
| P0-3 notification summary scope correctness | **NO** | Code-level. Removing the `uuid.Nil` sentinel is a Go signature change, not a schema change. |
| P0-4 zero-assignment scope rule | **NO** | Code-level predicate change. |
| P0-5 detail/mutation fail-open fix | **NO** | Code-level predicate change. |
| Proxy path and method repairs | **NO** | Frontend only. |
| Design tokens and UI components | **NO** | Frontend only. |
| New routes `/faskes`, `/queues/new`, `/admin` | **NO** | Frontend only; all data comes from existing endpoints. |
| Queue polling | **NO** | Client-side timer over an existing endpoint. |
| Test infrastructure (Vitest, Playwright) | **NO** | Tooling only. |

**Verdict: no phase in this plan requires a database migration.**

If a future phase does require one, it must: be additive (new nullable columns, new tables, new indexes), never
modify an applied migration file, and be applied before the dependent code ships. Any proposed change that would
require altering `0001`-`0010` must be rejected and re-modelled additively.

One operational (non-migration) requirement is carried forward: verify that the seed data gives at least one
facility-scoped admin `user_roles` row, one zero-assignment **non-super_admin** actor, and one global
`super_admin` (per T-3B0-08), so the corrected scope behavior - including the explicit unrestricted path -
is demonstrable and testable locally. This is a `packages/db/seed/` change, not a migration.

---

## 17. Quality Gates

A gate must pass before the next phase begins. Gates are cumulative; a later gate implies all earlier gates still pass.

| Gate | Name | Condition to pass |
|---|---|---|
| **GATE 0** | P0 remediation plan approved | All Phase 3A.1 final conditions recorded, plus the Phase 3A.4 facility-scope consumer correction: (1) implementation plan reviewed; (2) `WALK_IN_ROUTE = /queues/new` accepted; (3) global super_admin unrestricted administration = YES; (4) unrestricted facility scope explicitly modelled server-side (Section 4.5, DB-resolved, no dedicated column invented); (5) zero-assignment **non-super_admin** semantics fail closed; (6) no client-side role/scope inference; (7) task count documented as 50; (8) strict canonical order holds: 3B0 precedes 3B1 (GATE 1 before 3B1), and post-GATE 2 the order is strictly 3B2 -> 3B3 -> GATE 3 -> 3B4 -> GATE 4 -> 3B5 -> GATE 5 -> 3B6 -> GATE 6 -> 3B7 (Phase 3A.2); (9) practitioner raw/free input removed from schedule UX; (10) design token drift (muted) removed, `DESIGN.md` confirmed canonical; (11) T-3B0-07 covers **all** facility-scope consumers via a mandatory source-discovered inventory, the `Unrestricted`-before-empty-`IDs` semantics, the centralized `CanAccessFacilityForActor` contract, and a table-driven regression matrix over every consumer; (12) T-3B0-04/05/06 explicitly consume T-3B0-07 without duplicating its scope logic. |
| **GATE 1** | P0 tests pass | All P0-1..P0-5 regression tests green; the super_admin security matrix (Section 13.2, cases A-G) green; all 23 proxies audited correct; deactivate works end to end with `PATCH`; `go test ./...` green; router coverage >= 90%; no DB migration introduced. **3B1 must not begin before this gate passes.** |
| **GATE 2** | Shared primitives match frozen tokens | (Reached only after 3B0/GATE 1.) Every `DESIGN.md` token value present and exact in `tokens.css`; radii 6/8/12; control heights 44 and 36-40; one icon family, one stroke width; no gradients, no glassmorphism, no decorative shadows; `lib/ui` unit tests green; the Vitest + Playwright test tooling foundation (T-3B1-01) is in place, its E2E smoke scenario passes against a local seeded stack, and its production-target guard is explicitly tested and verified (the production URL is rejected as an E2E target). Identical in substance to T-3B1-08 in the task breakdown. |
| **GATE 3** | Citizen E2E green | Citizen critical path E2E green; all five transactional flows render every required state; no demo content anywhere in the citizen shell; bottom nav exactly four tabs; `tests/auth-actions.test.js` unchanged and passing. |
| **GATE 4** | Admin read flows green | Six sidebar destinations reachable with `aria-current`; Ringkasan derived-only counts; Antrean polling with pause-on-hidden and last-updated label; joins resolve or show `-`; empty-scope class-1 state everywhere; no admin rate-limit state; icon rail accessible at ~1024px. |
| **GATE 5** | Admin mutation flows green | Appointment and queue actions offer exactly the verified transitions; invalid transitions surface the 400 message; deactivate dialog-confirmed and one-way; `schedule.manage` absence shows ForbiddenPanel; dialogs satisfy trap/Escape/return-focus; live region announces mutations. |
| **GATE 6** | Full regression and production readiness | Full Playwright suite green; full Go suite green with coverage gate; Rust suite green; web check and web test green; legacy demo components removed; no orphaned routes; smoke-check script defined and passing against a production-equivalent stack; rollback point documented per phase; `SIGAP_DEV_IDENTITY` confirmed off in production config. |

Additional standing constraints (apply to every gate):

- No existing URL renamed.
- No production deployment performed as part of a gate.
- No migration introduced without an explicit additive justification.
- No new dependency added without a documented reason (Vitest, `@testing-library/svelte`, jsdom, and Playwright in T-3B1-01; Lucide in T-3B1-04 are the only justified additions).

---

## 18. Rollback Strategy

### 18.1 Principles

1. **No migration means no data rollback.** Every phase in this plan is code-only, so rollback is always a
   redeploy of the previous container image.
2. **One phase, one revertable commit series.** Each phase lands as its own reviewable unit so a bad phase can be
   reverted without unpicking its neighbours.
3. **Tightening changes are safe to roll back, but should not be rolled back for convenience.** Reverting 3B0
   would reopen the P0 privilege-escalation path. If 3B0 causes an operational problem, fix forward (for example,
   model unrestricted administration explicitly) rather than reverting the security fix.
4. **Frontend rollback is cheap and non-destructive** because URLs are preserved and no client state persists
   beyond `localStorage` conveniences that the design does not depend on.

### 18.2 Per-phase rollback

| Phase | Rollback action | Residual effects | Data risk |
|---|---|---|---|
| 3B0 | Redeploy previous API + web images | None | None |
| 3B1 | Redeploy previous web image | None (additive) | None |
| 3B2 | Redeploy previous web image | `/faskes` becomes 404; Beranda reverts to the demo dashboard | None |
| 3B3 | Redeploy previous web image | `/queues/new` becomes 404; transactional surfaces revert | None |
| 3B4 | Redeploy previous web image | `/admin` becomes 404; admin surfaces revert | None |
| 3B5 | Redeploy previous web image | Mutation affordances revert; previously broken actions break again | None |
| 3B6 | Redeploy previous web image | Deleted components return from git history | None |
| 3B7 | No rollback needed (documentation only) | None | None |

### 18.3 Rollback triggers

Roll back the most recent phase if any of the following is observed after deploy:

- A 5xx rate increase attributable to the phase.
- A citizen critical path break (booking, check-in, walk-in, or status).
- An Admin read or mutation regression that is not covered by a known, accepted behavior change.
- An unexpected 403 increase for scoped admin identities (indicates an over-tightened scope fix).
- A missing `/faskes`, `/queues/new`, or `/admin` after the phase that introduced it.

Do **not** roll back for:

- Zero-assignment **non-super_admin** actors receiving 404s after 3B0. That is the fix.
- Cross-facility notification retry/cancel failing after 3B0. That is the fix.
- Zero-assignment **non-super_admin** actors seeing zero summary counts after 3B0. That is the fix.
- A global `super_admin` no longer relying on the accidental empty-set path: under Section 4.5 it retains
  unrestricted access, now explicitly and structurally. If a global `super_admin` **loses** access, that is a
  regression in the fix and must be corrected forward.

### 18.4 Forward-fix preference

For 3B0 specifically, the preferred response to any operational surprise is a forward fix, because the phase
closes a privilege-escalation path. The unrestricted-global-administration requirement was resolved at GATE 0
by the Phase 3A.1 owner decision (Section 4.5): an explicit, DB-resolved unrestricted scope for the active
global `super_admin` assignment. No further product decision is open; deploy-time surprises must be fixed
forward, never by reverting the security fix or re-introducing empty-set-unrestricted semantics.

### 18.5 Preserved rollback anchors

| Anchor | Value |
|---|---|
| Last known-good commit before any 3B work | `6d7f940` (current HEAD) |
| Branch | `design/ui-ux-overhaul` |
| Pre-existing uncommitted state | `M .gitignore`, `?? design/` (must be committed or explicitly retained before 3B0 begins) |
| Migrations to restore | None. Migrations `0001`-`0010` remain untouched by this plan. |

---

## Appendix A - Verified Endpoint Inventory (implementation reference)

Every endpoint the redesign will consume, with its verified contract.

| Method | Path | Auth | Request | Success | Notable errors |
|---|---|---|---|---|---|
| GET | `/api/v1/public/facilities` | none | no params | 200 `{data:[{id,name,short_code,is_active}]}` | 500 |
| GET | `/api/v1/public/service-units` | none | `?facility_id=` optional | 200 `{data:[{id,facility_id,name,code,is_active}]}` | 500 |
| POST | `/api/v1/appointments` | none | `{facility_id,service_unit_id,practitioner_schedule_id?,patient_display_name,patient_phone,appointment_time,notes?}` | 200 `{data:{id,checkin_code,status,appointment_time}}` | 400 validation, 429 phone limit, 409 slot full, 500 |
| POST | `/api/v1/appointments/{id}/check-in` | none | `{checkin_code}` | 200 `{data:{appointment_id,queue_ticket_id,formatted_number,status:"queued",estimated_wait_minutes,processing_time}}` | 400, 401 wrong code, 404, 409 wrong state / lost race, 429 |
| POST | `/api/v1/queues/generate` | none | `{facilityId,patient:{fullName,phone,gender?,dateOfBirth?}}` | 200 `{data:{TicketID,formatted_number,Status,registered_at,estimated_wait_minutes,processing_time,signature}}` | 400, 429 daily limit, 500 |
| GET | `/api/v1/patient/status` | none | `?code=` | 200 `{data:{found_by,facility_name,appointment_status,appointment_time,checkin_status,queue_number?,queue_status?,queue_formatted_number?}}` | 400, 404, 429 |
| GET | `/api/v1/admin/facilities` | `facility.read` | none | 200 `{data:[facilityResponse]}` | 200 empty on empty scope |
| GET | `/api/v1/admin/facilities/{id}` | `facility.read` | - | 200 `{data:facilityResponse}` | 400, 404, **fail-open (P0-5)** |
| POST | `/api/v1/admin/facilities` | `facility.manage` | create body | 200 | 400, 403 |
| PATCH | `/api/v1/admin/facilities/{id}` | `facility.manage` | partial body | 200 `{data:{id}}` | 400 no fields, 404, **fail-open (P0-5)** |
| PATCH | `/api/v1/admin/facilities/{id}/deactivate` | `facility.manage` | none | 200 `{data:{id,is_active:"false"}}` | 400, 404, **fail-open (P0-5)**, 405 for POST |
| GET | `/api/v1/admin/queues` | `queue.read` | none | 200 `{data:[queueTicketResponse]}` | 200 empty on empty scope |
| GET | `/api/v1/admin/queues/{id}` | `queue.read` | - | 200 `{data:queueTicketResponse}` | 400, 404, **fail-open (P0-5)** |
| PATCH | `/api/v1/admin/queues/{id}/status` | `queue.manage` | `{status}` | 200 | 400 invalid transition, 404 |
| GET | `/api/v1/admin/service-units` | `schedule.read` | none | 200 `{data:[serviceUnitResponse]}` | 200 empty on empty scope |
| GET | `/api/v1/admin/service-units/{id}` | `schedule.read` | - | 200 | 400, 404, **fail-open (P0-5)** |
| POST | `/api/v1/admin/service-units` | `schedule.manage` | create body | 200 | 400, 403 invalid scope |
| PATCH | `/api/v1/admin/service-units/{id}` | `schedule.manage` | partial body | 200 | 404, **fail-open (P0-5)** |
| GET | `/api/v1/admin/schedules` | `schedule.read` | none | 200 `{data:[scheduleResponse]}` | 200 empty on empty scope |
| GET | `/api/v1/admin/schedules/{id}` | `schedule.read` | - | 200 | 400, 404, **fail-open (P0-5)** |
| POST | `/api/v1/admin/schedules` | `schedule.manage` | create body | 200 | 400, 403 |
| PATCH | `/api/v1/admin/schedules/{id}` | `schedule.manage` | partial body | 200 | 400, 404, **fail-open (P0-5)** |
| GET | `/api/v1/admin/appointments` | `appointment.read` | none | 200 `{data:[appointmentResponse]}` | 200 empty on empty scope |
| PATCH | `/api/v1/admin/appointments/{id}/status` | `appointment.manage` | `{status}` | 200 `{data:{id,status,updated_at}}` | 400 invalid transition/status, 404 |
| GET | `/api/v1/admin/notifications` | `notification.read` | `?facility_id&limit&status&channel&template_key&created_from&created_to` | 200 `{data:[masked rows]}` | 400 invalid filter, 500 |
| GET | `/api/v1/admin/notifications/summary` | `notification.read` | none | 200 `{data:{pending,processing,delivered,failed,cancelled}}` | **global leak (P0-3)** |
| GET | `/api/v1/admin/notifications/{id}` | `notification.read` | - | 200 `{data:row}` | 400, 404 |
| POST | `/api/v1/admin/notifications/{id}/retry` | `notification.manage` | none | 200 `{data:row}` | 400, 404, 409, **authz-after-mutate (P0-1)** |
| POST | `/api/v1/admin/notifications/{id}/cancel` | `notification.manage` | none | 200 `{data:row}` | 400, 404, 409, **authz-after-mutate (P0-2)** |
| GET | `/api/v1/events/beds` | none | - | SSE; in practice emits `queue_created` only | - |

## Appendix B - Documents Created or Modified

| Document | Action |
|---|---|
| `design/sigap-redesign-implementation-plan.md` | **CREATED** in Phase 3A; **MODIFIED** in Phase 3A.1 (Section 4.5 decision, P0-4/P0-5 remediation, test matrix, sequencing, GATE 0/1/2, practitioner rule); **MODIFIED** in Phase 3A.2 (zero-assignment actor-class normalization, Playwright foundation order, strict sequencing, GATE 2 conditions); **MODIFIED** in Phase 3A.3 (3B1 tooling-first renumbering reflected in Section 13.4 and Section 14, GATE 2 cross-reference to T-3B1-01/T-3B1-08, dependency-list task IDs); **MODIFIED** in Phase 3A.4 (facility-scope consumer coverage broadened to every discovered call site, Section 4.5.1 consumer semantics table, P0-4/P0-5 remediation and test matrix, 3B0 internal execution order) |
| `design/sigap-redesign-task-breakdown.md` | **CREATED** in Phase 3A; **MODIFIED** in Phase 3A.1 (50-task count, strict GATE-1-before-3B1 sequencing, T-3B0-07/T-3B0-08/T-3B5-03 updates); **MODIFIED** in Phase 3A.2 (actor-class wording, E2E tooling foundation placed in 3B1, T-3B4-01, T-3B6-01, T-3B7-02, strict sequencing); **MODIFIED** in Phase 3A.3 (Phase 3B1 reordered/renumbered with T-3B1-01 as the tooling foundation, T-3B1-08 GATE 2 synchronized with the plan, T-3B2-05 renamed to a neutral checkpoint); **MODIFIED** in Phase 3A.4 (T-3B0-07 broadened to every facility-scope consumer with the source-audit first step and the verified consumer inventory, 3B0 internal execution order with the scope contract first, T-3B0-04/05/06/08 dependency updates) |
| `design/sigap-redesign-implementation-handoff.md` | **MODIFIED** in Phase 3A (only where source verification proved a statement stale or ambiguous: Sections 3, 5, 6, 10); **MODIFIED** in Phase 3A.1 (P0 register + practitioner note only). No frozen visual or product decision was changed in either pass. |
| `design/SKILLS.md` | **MODIFIED** in Phase 3A.1: corrected muted token `#706b65` -> `#57534E` and marked `DESIGN.md` canonical. No visual change. |
| `design/DESIGN.md` | **MODIFIED** in Phase 3A.1: one clarification line confirming it is the canonical design-token source. No token redesign. |

No production source file was created or modified. No migration was created or modified. Nothing was deployed.

No planning document states a task count other than 50 (3B0=9, 3B1=8, 3B2=5, 3B3=6, 3B4=9, 3B5=6,
3B6=4, 3B7=3). No planning document permits concurrent execution of 3B0 and 3B1: the canonical order is
strictly GATE 0 -> 3B0 -> GATE 1 -> 3B1, and 3B1 starts only after GATE 1.
