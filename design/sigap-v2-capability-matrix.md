# SIGAP V2.2 — capability gap analysis

What the product can actually do today, what the backend already supports but the UI
does not surface, and what would need real work. Every claim below was checked against
the source; the file:line references are the evidence.

**Nothing here is a promise.** Items in *NEXT RELEASE* and *FUTURE* are proposals and
need owner approval before implementation.

---

## 1. What exists, as measured

### Citizen surface — the complete public API

| Method | Path | Handler | What it returns |
|---|---|---|---|
| POST | `/api/v1/appointments` | `booking.go:76` | appointment id, **check-in code**, time, status |
| POST | `/api/v1/appointments/{id}/check-in` | `booking.go:423` | queue ticket + formatted number |
| GET | `/api/v1/patient/status?code=` | `patient.go:63` | facility name, appointment status, queue number + status |
| GET | `/api/v1/public/facilities` | `catalog.go:59` | id, name, short code, type, active |
| GET | `/api/v1/public/service-units?facility_id=` | `catalog.go:94` | service units of one facility |
| POST | `/api/v1/queues/generate` | `queue.go:39` | walk-in queue ticket |
| GET | `/api/v1/facilities/nearby` | `main.go:476` | hardcoded list |
| GET | `/api/v1/events/beds` | `main.go:365` | SSE stream |

That is the whole citizen surface. There is **no** `GET /api/v1/appointments` — a citizen
cannot list their own appointments, because there is no identity to list them against.

### The honest summary

| | |
|---|---|
| Endpoints the citizen can reach | 8 |
| Endpoints behind admin permission | 26 |
| Tables with no reader endpoint | `medical_records`, `audit_events` |
| Dead client helpers pointing at undeclared routes | `myAppointments()` → `/api/v1/patient/appointments` (`citizen.ts:20`) |

---

## 2. Feature matrix

### QUICK WINS — the backend already supports it; only UI work

| # | Feature | Backend evidence | Work | Risk |
|---|---|---|---|---|
| Q1 | **Show the appointment time and queue number on the status page** | `PatientStatusLookup` returns `queue_formatted_number` and `queue_status`; the page renders both | Already shipped | none |
| Q2 | **Explain the check-in code on the booking confirmation** so a citizen can find it again | `booking.go:70` returns `checkin_code`; the confirmation renders it | Copy + hierarchy work | none |
| Q3 | **Link the booking confirmation directly into check-in** with id + code prefilled | `CheckinForm` already accepts both; the booking page already builds the deep link (`+page.svelte:360`) | Already shipped | none |
| Q4 | **Client-side facility search + type filter** | `GET /api/v1/public/facilities` returns the full catalog; the page already filters locally | Already shipped | none |
| Q5 | **Queue transition buttons that name the action** (Panggil / Layani / Lewati) | `admin.go:1057-1064` transition map; already shipped | Already shipped | none |
| Q6 | **Notification retry / cancel** | `notifications.go:432,490`; `service.go:276-330` | Already shipped | none |
| Q7 | **Schedule create + update** | `admin.go:1839,1915`; slot validation `admin.go:2197-2211` | Already shipped | none |
| Q8 | **Show *why* a lookup failed, distinctly from "not found"** | The API returns 400/404/429/5xx distinctly; `errors.ts` already models them | Mapping already correct — verified by `verify-status-error-map.mjs` | none |

### NEXT RELEASE — bounded backend work, no new security model

| # | Feature | What exists | What is missing | Value | Complexity |
|---|---|---|---|---|---|
| N1 | **Retrieve more of my appointment without an account** | `checkin_code` is a 6-char unguessable token (32^6 ≈ 1.07e9); the status lookup already resolves by code and, as of v2.3, returns `service_unit_name` | Bounded: the status response still deliberately withholds `appointment_time` (PHI-adjacent correlation) — do NOT re-add it; any retrieval stays code-only and non-enumerating | Medium | **S** — additive fields only, rate-limited |
| N2 | **Cancel my appointment — IMPLEMENTED (v2.3)** | `POST /api/v1/appointments/cancel` ships: scheduled-only, idempotent, audited, shared 30/min limiter, guarded transition | n/a | Shipped | — |
| N3 | **Reschedule my appointment** | `appointments.appointment_time` is mutable; the admin path already updates it | A citizen route keyed by code | High — today a citizen must phone the facility | **M** — needs slot validation against `practitioner_schedules`, and the same rule the admin editor enforces |
| N4 | **Staff check-in that issues a queue number — IMPLEMENTED (v2.3)** | `PATCH /api/v1/admin/appointments/{id}/status` to `checked_in` now mints and atomically attaches a queue ticket through the queue service, guarded by `queue_ticket_id IS NULL` + optimistic `AND status=$current`, rolling back to `scheduled` on failure; covered by a concurrency test | n/a | Shipped | — |
| N5 | **Read the audit trail** | `audit_events` exists and every admin action writes to it (`audit/service.go:97`) | A `GET /api/v1/admin/audit` scoped by facility; `audit.read` is granted to `super_admin` only | High for administrators; accountability for a civic system | **M** — plus a **data fix**: admin rows never set `Event.FacilityID` (`admin.go:733-742`), so facility filtering cannot work on existing rows |
| N6 | **Facility queue capacity as a setting** | `DAILY_QUEUE_LIMIT = 300` is a Rust constant (`queue.rs:10,113`) | A per-facility cap column + enforcement | Medium — one busy clinic and one quiet one cannot share a constant | **M** — schema change, engine change, migration |
| N7 | **Notification template management** | `notification_templates` table exists with `channel` CHECK (dev/sms/whatsapp/email) | No create/update route (`router.go:74-76`) | Medium | **M** — needs the template lifecycle and validation |
| N8 | **Server-side queue filtering by facility** | The handler's doc comment claims it (`admin.go:763-765`) but parses no query | Query parsing + scope-aware filter | Medium — the UI filters locally, which breaks past a page of rows | **S** |
| N9 | **Service-unit creation from the admin UI — IMPLEMENTED** | `createServiceUnit` posts to the registered `/api/v1/admin/service-units` with `facility_id` in the body | n/a | Shipped | — |

**Walk-in status retrieval — PROPOSED, not implemented.** A walk-in ticket has no
check-in code, so a walk-in citizen cannot retrieve their visit status today, and
lookup by `formatted_number` would re-introduce the enumeration vector that was
deliberately removed. Proposal: at walk-in issuance, mint an opaque, unguessable
token (crypto-random, stored beside the ticket, NEVER derived from the number),
accept it on the same code-only, rate-limited `/patient/status` path, and expire
it at end of service day. Must NOT expose patient name/phone through that path.
Deferred until the privacy design is written down; do not silently add it.

### FUTURE — needs a product and security decision first

| # | Feature | Why it is not a quick win |
|---|---|---|
| F1 | **Citizen accounts / appointment hub** | Requires an identity provider that actually works. Supabase is configured but not provisioned in this environment; every "my appointments" design depends on that decision, plus a data-retention policy for health data. |
| F2 | **Digital visit pass with QR** | A QR encodes the check-in code. Rendering it is trivial; deciding whether a screenshot of it should be sufficient at a counter is a security decision, and the answer changes the design. |
| F3 | **QR-assisted check-in** | Same token as F2, plus a scanner at the facility. Needs hardware and an offline-failure story. |
| F4 | **Citizen notifications / reminders** | The outbox exists but only `DevProvider` ships (`cmd/notification-worker/main.go:6-10`) — sms/whatsapp/email are schema-valid and **undeliverable**. Choosing a provider is a procurement and privacy decision. |
| F5 | **Medical record access** | `medical_records` exists with a SHA-256 signature and **no reader endpoint**. Exposing it is a clinical-data decision, not an engineering one. |
| F6 | **Practitioner catalogue and per-practitioner booking** | No practitioner endpoint exists at all; the schedule editor deliberately omits `practitioner_id`. Adds a new scope dimension (today the only scope is `user_roles.facility_id`). |
| F7 | **Wait-time estimates** | The engine returns `estimated_wait_minutes`, but no queue-depth model is exposed and the number is currently a constant. Showing it without a real model would be a fabricated claim. |

---

## 3. Where the product is thinner than it looks

Four gaps that matter more than the feature count suggests:

1. **A citizen cannot get back to their own appointment.** The check-in code is the only
   key, it is shown once on the confirmation, and there is no lookup that returns it.
   Every other gap is smaller than this one.
2. **The desk cannot check anyone in.** Admin check-in sets the status and stops. The
   queue number is only minted by the citizen's own check-in.
3. **Nobody can read the audit log**, and the rows are not facility-attributed, so even a
   future reader could not scope them.
4. **Nothing is configurable per facility** — not capacity, not templates, not the
   catalogue beyond what already exists.

---

## 4. What was deliberately not built

- **No appointment list for citizens.** There is no identity to scope it to. Building one
  against a phone number would put a patient lookup behind a guessable identifier.
- **No QR anywhere.** See F2/F4.
- **No wait estimate on the status page.** The value exists in the engine response but is
  a constant today; printing it would be inventing a number.
- **`/wallet` stays an honest empty state.** `GET /api/v1/medical-records` does not exist;
  the previous version fabricated a QR block and a signature claim.

---

## 5. Approval needed

| Decision | Why it needs the owner |
|---|---|
| Which of N1–N9 to build next | Each is a product choice, not a technical one |
| Whether citizens get accounts (F1) | Sets the shape of everything else in the citizen column |
| Which notification provider (F4) | Procurement + data-processing agreement |
| Whether a screenshot of a visit pass is acceptable at a counter (F2) | Security policy |

---

## 6. Regional multi-facility pilot (feat/sigap-regional-pilot) — status

**RUNNING CODE (backend-enforced, not decorative):**
- `organizations` table + `facilities.organization_id` + `user_roles.organization_id` (migration 0013).
- Facility/org/region scope resolution folding into the existing `AuthorizeFacilityMutation/Read` shape; deny-by-default; handlers never trust client `facility_id`/`organization_id`.
- Roles `organization_admin` (org-scoped) + `regional_admin` (stats read-only) reusing the existing roles/permissions model; system admin = `super_admin`.
- `GET /api/v1/admin/organizations` + `/{id}` (scoped directory + operational overview); forged/sibling org id → 404.
- Frontend `/admin/organizations` directory + overview, backend-authorized.
- WIB service-day end-to-end (allocator + cap + guard + listing).
- Synthetic seed: 2 orgs, 4 facilities, distinct admins/operators (labelled [synthetic]).

**Verified:** `go test ./...` (7 cross-tenant isolation tests + scenarios A/F + 10 WIB boundary tests, with DB); `cargo test --lib`; `vitest`; `svelte-check`. Live: super-admin sees 2 orgs, org-admin sees 1, forged org → 404.

**DESIGN-ONLY (not running):** SATUSEHAT/FHIR integration, patient identity/consent ledger, blockchain options B/C, cross-region discovery beyond one region. See `regional-architecture-rfc.md`, `satusehat-interoperability-plan.md`, `regional-interop-contracts.md`, `migration-ledger-reconciliation.md`.

### Acceptance hardening (this milestone)

- **Patient identity foundation — IMPLEMENTED (design + code).** `internal/consent` deny-by-default evaluator (`Evaluate(request, grant, at)` → revocation > validity > emergency > exact-match). Four identity layers separated in `patient-identity-consent.md` (auth / verified-patient / facility-local record / future SATUSEHAT); no NIK stored, no public identity endpoint.
- **Migration baseline — VERIFIED / PROPOSED.** `migration-baseline-recovery-plan.md`: checksum backfill procedure, duplicate-CREATE-TYPE gate, ledger-only rollback; owner-gated (not executed).
- **SATUSEHAT sandbox — NOT_CONNECTED.** `satusehat-sandbox-readiness.md`: PoC prerequisites BLOCKED on partner registration/OAuth creds/fasyankes DFO.
- **Regional visual QA — OPEN.** 16 screenshots (1440/768/390/360 × organizations/detail/overview/queues) + geometry (no page overflow, no undersized targets); vision inspection not performed.
- **E2E — RUN.** `admin-org-isolation.spec.ts` 4/4; `admin-read.spec.ts` 27 passed / 2 dev-data-failed (empty-today board → no "Menunggu" band; not a regression). Per-actor scoping (org-A/B/regional) proven via Go tenant tests + direct API, not browser-switchable.

### Patient Continuity V0 (this milestone)

- **Patient account ownership — IMPLEMENTED.** `0014_patient_account.sql` (`appointments.patient_account_id` nullable; anonymous stays NULL); booking INSERT writes the owner; cancel re-checks ownership (`owned → 403` unless matching account; `NULL` keeps code-only). `identity.Actor.PatientAccountID()` fail-closed; synthetic citizen `local-patient-p1` (c307).
- **"Janji Saya" hub — IMPLEMENTED.** `GET /api/v1/patient/appointments` (account-keyed, code never scopes it) + `/patient/appointments` page (6 states + owned-cancel) + citizen-nav entry.
- **Cross-facility + isolation — TESTED.** `cross_facility_test.go` scenarios A–G + negatives (guessed id → 404, forged account/org → 404, non-owner cancel → 403). Real handler + DB, not seeded ownership.
- **Consent evaluator — IMPLEMENTED.** subject/actor/scope/purpose/category/expiration/revocation/audit inputs, deny-by-default + synthetic tests.
- **Auth — IMPLEMENTED (local Supabase Auth).** `npx supabase start` (Auth :54321, Studio :54323); two synthetic patient accounts mapped to `app_users.subject`; ES256/JWKS validated by the unchanged Go `JWTProvider`. `Start-V22Preview.ps1 -Jwt` runs the API in real jwt mode (synthetic selectors off). Verified live: anon 401, patient-a JWT 200 on the owned hub; a live booking across two org facilities under the real session yields "Janji Saya" = 2 for patient A (0 for B), own-cancel 200, cross-patient cancel 403. Email confirmation is NOT real-world medical identity; "verified login" of a real person remains out of scope.
- **SATUSEHAT — NOT_CONNECTED** (contract boundary only).

### Self-hosted auth (Better Auth) — this milestone

- **BETTER_AUTH_RUNNING=PASS.** `better-auth@1.7.20` (email/password, ES256/JWKS jwt plugin, isolated `auth` schema on the same Postgres). Live: `GET /api/auth/jwks` → ES256/P-256; sign-up + sign-in 200.
- **GO_JWT_JWKS_INTEGRATION=PASS.** Zero Go source change: the existing JWKS provider validates Better Auth's ES256 tokens (issuer `http://127.0.0.1:5173`, JWKS `/api/auth/jwks`). Live full chain: sign-in → `getToken` → Bearer → Go `/patient/appointments` **200** (subject → `app_users.subject` → `PatientAccountID`).
- **Web session authority swapped.** `hooks.server.ts` (svelteKitHandler + getSession + mint the JWT), `app.d.ts`/`Locals` (named `AuthSession`), `authHeaders()` (forwards `locals.jwt`, not the opaque session token), `+layout.server.ts`, and login/register/logout server actions (`auth.api.signInEmail/signUpEmail/signOut` + `applySetCookie`).
- **Identity mapping.** `identity-mapping.md` + `backfill-auth-subjects.mjs` (13 synthetic accounts, explicit by `app_users.id`, never email auto-link; rollback = restore subjects + re-point Go env).
- **SUPABASE_DECOUPLING=PARTIAL.** Web session authority fully decoupled; `$lib/supabase/*` + `@supabase/*` deps retained as the §8 rollback reference (not an authority). `PUBLIC_SUPABASE_*` removed from the web tier env.
- **NOT done (Wave 2):** admin sign-in through the SAME Better Auth system (dev/test selector still serves admin in local), the multi-step browser journey + screenshots, e2e spec re-wording away from "Supabase", and visual QA.

### Auth hardening + GitHub attribution (this milestone)

**Auth (running code):**
- **Production-build unblocker FIXED.** Better Auth's OTel loader (`@better-auth/core/dist/instrumentation/api.mjs`) dynamically imports `@opentelemetry/api`; the package was absent, so the bundler resolved the specifier to an unrelated 4-line chunk and **every `/api/auth/*` endpoint 500'd in the built app** (dev worked). `@opentelemetry/api@^1.9.0` added as a dependency; the built app now serves sign-up/sign-in/session/JWT.
- **`sveltekitCookies` moved LAST** in the plugin array (Better Auth's required order for cookie interception) — caught by the new session-lifecycle contract test.
- **Session lifecycle — IMPLEMENTED.** Explicit `session:{expiresIn 7d, updateAge 1d, freshAge 1h}` + `user.changePassword`; `design/session-lifecycle-policy.md` states the token-lifetime + authorization-freshness policy (a revoked session is not a revoked JWT; the JWT TTL is the max revocation delay; DB RBAC re-resolves every request).
- **IMPERSONATION PRODUCTION-BLOCKED — PASS + F1 FIXED.** 19 negative tests (`envguard_test.go`, `impersonation_negative_test.go`) prove every selector is inert outside `SIGAP_ENV=local`, forged `X-Sigap-*` headers grant nothing, and token permission claims are ignored. F1 (the loopback guard not covering `SIGAP_LOCAL_RBAC_TEST_IDENTITY`) is closed in `envguard.go`.
- **SUPABASE_RUNTIME_DECOUPLING=COMPLETE.** 49/49 touchpoints re-classified: **zero** runtime Supabase dependency; only 5 rollback-only legacy adapter files + unused deps remain.
- **DEPLOYMENT_READINESS=NOT_READY** (1 READY / 9 GAP): top gaps = `BETTER_AUTH_SECRET` not wired into compose/.env.example, Better Auth sign-in/out not reaching the Go audit table, `trustedOrigins` unset, no JWKS `rotationInterval`, no least-privilege DB role.

**GitHub attribution (read-only audit + prevention):**
- **ROOT CAUSE (verified):** the visible Contributors **graph** shows 4 entries — `lostsky1403` (334), `codebuff-team` (11), `traeagent` (11), `claude` (6) — because `Co-authored-by:` trailers whose e-mails map to real GitHub accounts are attributed *on top of* the author. The repo side-panel `/contributors` still shows only `lostsky1403`.
- **32 trailer commits** are ancestors of public `main`; stripping them is a **message-only history rewrite** that would mint new SHAs for **296 of 342 main commits** (45 branch refs, 16 PRs). **No remote history was touched.** `.mailmap`, collaborator removal and branch renames cannot change the graph.
- **PREVENTION=IMPLEMENTED + VERIFIED.** `.githooks/commit-msg` (strips known AI trailers, preserves human co-authors — verified end-to-end), `scripts/check-attribution.mjs` (exit 1 on AI / 0 clean — verified on real commits `030447b5` and `a22313f5`), a CI `Commit Attribution Check` job (new commits only), `AGENTS.md`, and `docs/operations/git-attribution.md`.
- **CONTRIBUTOR_DISPLAY_CLEANED=NO** (the 3 historical graph entries remain until an owner-approved rewrite). **OWNER_APPROVAL_REQUIRED=YES** (Option 2 only).
- **HISTORICAL_REWRITE_REHEARSAL=COMPLETE** (`github-attribution-rewrite-rehearsal.md`): a disposable-clone message-only strip rewrote **438 of 484** commits (32 messages altered; 406 only via ancestry); public main 296 changed / 46 preserved; **484/484 trees identical** with 0 author/committer/date mismatches; recovery = mirror clone + bundle + refs manifest; **nothing was pushed**.
- **SESSION_LIFECYCLE=PARTIAL**: `tests/session-lifecycle.test.js` **60 passed / 0 failed / 2 documented skips** (session clocks, cross-device revocation, password change + `revokeOtherSessions`, sign-out cookie rejection, JWT TTL); the 2 skips are the absent `auth.user.banned` column (base Better Auth; SIGAP enforces deactivation in the Go resolver) and the 1-hour `freshAge` window.
- **EMAIL_VERIFICATION=PASS · PASSWORD_RESET=PASS · STAFF_MFA=PARTIAL (local-capture scope: enrol/verify/backup-code sign-in proven, but the backup-code lifecycle, /two-factor/disable, /generate-backup-codes and MFA recovery are unverified).** All three driven live end-to-end (`email-mfa-flows-verification.md`): sign-up → mailed link → `emailVerified` flips (replay idempotent); reset → OLD password 401 / NEW 200 / spent token 400; `two-factor` enable → sign-in `twoFactorRedirect` with zero session rows → wrong code 401 → correct 200 → backup code 200 → replayed backup 401. **Production SMTP is BLOCKED/absent** and 12 further steps are listed as unverified (requireEmailVerification policy branch, lockout ceiling, token-expiry clocks, disable/regenerate endpoints, the Go trust boundary for an MFA subject).
- **STAFF_PROVISIONING=PASS (applied + idempotent + real-JWT RBAC).** `provision-staff.mjs --apply` created 11 Better Auth users + wrote 11 `app_users.subject` mappings (1 by-design adopt-existing refusal); a 2nd/3rd `--apply` reported `changed=0`. Four real form sign-ins → JWTs → Go scopes (`staff-provisioning-live-verification.md`): operator-a1 sees only facility c101 (siblings 404, `/admin/organizations` 403); org-admin-a sees c101+c102 and only org c001; the b-side mirror reproduces it; a forged `X-Sigap-Local-Test-Subject` is 403 identical to no header.
- **Registration now provisions a SIGAP identity (defect fixed).** `databaseHooks.user.create.after` inserts a ROLELESS `app_users` row keyed by the auth user id (never email-linked); before the fix a new citizen could sign in but `/patient/appointments` 401'd and bookings stayed `patient_account_id NULL`.
- **Secret fail-open closed.** The build placeholder is used only while `building` (or `NODE_ENV !== production`); production without `BETTER_AUTH_SECRET` now THROWS. `jwt.issuer`/`audience` read `SIGAP_AUTH_ISSUER`/`SIGAP_AUTH_AUDIENCE`. Asserted in `tests/auth-config-assertions.test.js` (0 failed / 0 skipped with a live DB).
- **Wrong-password 401 restored.** better-call's `APIError.status` is the *string* `"UNAUTHORIZED"` and the number lives in `statusCode`; the discrimination compared `status === 401`, so a wrong password rendered the 503 outage message. Now accepts both shapes (verified live: `status:401, 'Email atau kata sandi salah.'`).
- **Open redirect closed (CWE-601).** `?redirectTo=/\evil.com` bypassed the `//` check (browsers normalise it to `//evil.com`); a backslash is now rejected.
- **VISUAL_QA=OPEN — screenshots delivered, pixels NOT inspected.** 16 shots (`review-shots/auth-qa/`, gitignored) at 1440/768/390/360 for login, register, signed-out Janji Saya and the admin nav, with zero page overflow, zero viewport-past elements and zero sub-36px controls.
- **Gates (all green):** `go test ./...` 12/12 (DB) · `cargo test --lib` 7/7 · `vitest` 749 · `svelte-check` 0/0 · `vite build` clean · `proxy-contracts` / `build-verification` / `auth-actions` / `auth-config-assertions` PASS · `session-lifecycle` 60/0/2 · self-hosted auth E2E 4/4.

### Post-rewrite CI stabilization + security (2026-11-13)

- **GO_TOOLCHAIN_VERSION=go1.26.11.** `apps/api/go.mod` (`go 1.26.11`) + Dockerfile builder pinned `golang:1.26.11-alpine3.24` (verified published tag; `1.26.11-alpine3.21` is a Docker Hub 404).
- **GO_2026_6809=RESOLVED** (`crypto/x509`, fixed go1.26.10) and **GO-2026-6859=RESOLVED** (`net/http`, fixed go1.26.11) — both reachable stdlib findings cleared by the single bump. `govulncheck ./...` now exits **0** ("0 vulnerabilities" reachable). Two module-only `golang.org/x/net@v0.60.0` findings (`GO-2026-6866`, `GO-2026-6860`, fixed v0.62.0) remain **uncalled** (indirect dep; govulncheck does not flag them) — recorded, not silenced.
- **Rust Engine Check + Deployment Safety Guards = PASS.** Both `apt-get install` steps failed on transient runner DNS (`Temporary failure resolving 'azure.archive.ubuntu.com'`); they now retry the index refresh 3x/10s and fail loudly after, with a protoc pre-skip.
- **Deployment guards re-gated by branch identity** (release = `design/ui-ux-overhaul`, asserted against the durable anchor `d7705f09` with the exact inherited set; every other branch must not delete/rename/modify an applied migration). The old assertion ("every non-main branch must add 0011") was a pre-merge premise dissolved by the rewrite and misfired on any non-release PR.
- **PR #94** (fix/go-security-toolchain-1.26.11 -> main): **all 7 CI jobs green**, MERGEABLE, **not merged** (owner approval pending).
- **OLD_HISTORY_REINTRODUCTION_BLOCKED=PASS.** The stale `F:\Sehat-Hub` checkout was re-synced non-destructively: 23 archive tags, 13 content-identical mirror branches + local `main` re-pointed to the rewritten tips, checkout moved off `local/r2-drill` onto `main` (a5c0e34). Verified: **no branch reaches the old main tip (b96c5174)**; no `reflog expire`/`gc` was run, so every pre-rewrite tip stays recoverable.
- **AUTH_READINESS LEDGER.** `design/auth-readiness-review-v1.md` — 23 documented gaps classified (9 INFRA-BLOCKED, 6 OWNER-BLOCKED, 5 CODE-FIXABLE, 10 TEST-FIXABLE). Session-security Phase 1 **10/10 PASS** this run (JWKS rotation pinned at 2592000s, Go iss+aud enforcement, 15-min JWKS cache TTL, active-user RBAC gate). `VISUAL_QA` stays **OPEN** (48 PNGs, geometry proved, pixels not inspected).

### 6.2 Regional release integration strategy — single source of truth

The obsolete suggestion to **"promote `feat/sigap-selfhosted-auth` as the declared release
branch" is removed**. It is not an acceptance gate and must not be repeated in any further
report. A feature branch is not a release branch; relabelling it would falsify the release
migration guard.

The one strategy (full contract in `design/regional-release-integration-contract.md`):

1. Preserve `feat/sigap-selfhosted-auth` as the development branch; no rename or reset.
2. Integrate canonical `origin/main` (`4813de64fcbac89ae9ce419f003a9120177c6867` after PR #96;
   `51ba1d53…` was the pre-PR-#96 main) into the feature branch with a normal, non-rewriting
   merge, when separately authorized. This integration was performed locally on
   `b466c2d50a19f5a4aa53e4cb1e901f0db401a850`; the branch was not pushed.
3. Resolve conflicts while preserving Better Auth, regional RBAC, Patient Continuity, and
   migrations `0012`–`0014`.
4. After integration and regression verification, create a **NEW dedicated release branch
   from the integrated development tip**: proposed `release/sigap-regional-v1` (a proposed
   future name, not proof of creation).
5. Do not rename/relabel the feature branch to satisfy the guard; do not merge the feature
   branch into `main` in this task.

`design/ui-ux-overhaul` above was the *historical* release branch for the `d7705f09`-anchored
guard; it is not the regional release branch. The historical anchor is not repurposed.

### 6.3 Post-integration regression closure (local development, not published)
- Real Better Auth schema provisioned with the project's migration API; real form sign-in →
  JWT/JWKS → Go authorization journey passed 4/4 (citizen A/B, org admin, super admin).
- Session-security 61/0/0; session-lifecycle 61/0/2 (documented skips); full Go 1080/0/0;
  Vitest 749; Rust tests pass; svelte-check 0/0; cargo audit blocking with two narrow
  exceptions; govulncheck 0 reachable.
- Remaining PARTIAL/OPEN: staff MFA recovery requirements, production SMTP/TLS/secrets,
  least-privilege `auth` role, durable rate-limit store, audit sink, and the 0011 production
  state — none are production-ready. No release branch was created; nothing was pushed.
