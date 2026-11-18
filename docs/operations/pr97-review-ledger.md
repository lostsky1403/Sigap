# PR #97 application security & functional review ledger

**Project:** SIGAP / `lostsky1403/Sigap` — Draft PR #97
**Branch / head reviewed:** `feat/sigap-selfhosted-auth` @ `cff2d35bab3270a985be9c1823f6295e32493976`
**Base:** `main` @ `4813de64fcbac89ae9ce419f003a9120177c6867`
**Review date:** 2026-11-18
**CI at review:** GitHub Actions run `44338539484`, 9/9 required jobs `success` (head `cff2d35`).

> CI green is **not** evidence of application security. Every verdict below comes from
> reading the real source and tests at the exact head above.

## Scope honesty

PR #97 changes **385 files** (196 under `apps/`, 159 under `design/`, plus docs/scripts).
This ledger covers the **high-risk runtime paths** (auth boundary, regional RBAC,
patient-data isolation, SvelteKit frontend) to completion, plus targeted verification
of every historical Strix finding. 94 PNG design assets and low-risk design/documentation
files were **not** pixel-reviewed. Do not read this as "all 385 files reviewed".

## Verdict summary

| Dimension | Verdict |
| --- | --- |
| Authentication trust boundary | **PARTIAL** (no exploitable defect; config/infra gaps open) |
| Regional tenant isolation | **PASS** |
| Patient data isolation | **PASS** |
| Frontend route protection | **PASS** |
| Context recovery | **PASS** (`CHAT_ONLY` contamination) |

**Confirmed defects:** P0 = 0 · P1 = 0 · P2 = 1 (fixed) · P3 = 5 (documented; 1 fixed).

---

## 1. Runtime file ledger

### Authentication / session subsystem

| FILE | SUBSYSTEM | RISK | REVIEW_STATUS | FINDING | TEST_EVIDENCE | ACTION |
| --- | --- | --- | --- | --- | --- | --- |
| `apps/web/src/hooks.server.ts` | auth | High | reviewed | JWT minted only from a real session; opaque session token never forwarded | `session-lifecycle.test.js` (61/0/0), E2E 4/4 | none |
| `apps/web/src/lib/server/auth.better.ts` | auth | High | reviewed | fail-closed secret resolution; `sveltekitCookies` last; no `trustedOrigins`; no sign-in deactivation gate | `auth-config-assertions.test.js` 0/0/0 | ADR proposed for deactivation; trustedOrigins OPEN |
| `apps/web/src/lib/server/auth.ts` | auth | High | reviewed | Bearer-only forwarding; no dev header injection; local actor gated on `SIGAP_ENV=local` | `proxy-contracts`, E2E | none |
| `apps/web/src/routes/auth/{login,register,logout}/+page.server.ts` | auth | High | reviewed | open-redirect guard; generic errors; sign-out deletes session row | `auth-actions.test.js`, E2E | none |
| `apps/api/internal/auth/jwt_provider.go` | auth | High | reviewed | ES256/RS only, iss/aud/exp, zero leeway, kid required, JWKS cache 15m | `jwt_provider_test.go`, `better_auth_compat_test.go` | none |
| `apps/api/internal/auth/{config,provider,factory,dev_provider,local_test_identity_provider}.go` | auth | High | reviewed | dev selectors gated outside `local`; dev provider read-only permissions | `impersonation_negative_test.go`, `envguard_test.go` | none |
| `apps/api/internal/config/envguard.go` | auth | High | reviewed | `GuardDevCapabilities` + loopback binding gate | 19 negative tests | none |
| `apps/api/internal/identity/{actor,authz,dev_middleware}.go` | auth | High | reviewed | deny-by-default `RequirePermission` | `authz_test.go` | none |
| `apps/api/cmd/server/main.go` | auth | High | reviewed | chain DenyByDefault → Auth → Audit → RequirePermission | integration | none |
| `apps/web/svelte.config.js` | frontend | Med | reviewed | per-request nonce CSP, no `unsafe-inline` scripts | build verification | none |

### Regional RBAC / tenant isolation

| FILE | SUBSYSTEM | RISK | REVIEW_STATUS | FINDING | TEST_EVIDENCE | ACTION |
| --- | --- | --- | --- | --- | --- | --- |
| `apps/api/internal/auth/facility_scope.go` | rbac | High | reviewed | actor+DB-derived facility scope; provenance intersection | `facility_scope_test.go` | none |
| `apps/api/internal/auth/org_scope.go` | rbac | High | reviewed | org-scoped expansion; sibling id → 404 | `org_tenancy_test.go` | none |
| `apps/api/internal/auth/rbac_resolver.go` | rbac | High | reviewed | per-request DB resolution, `status='active' AND deleted_at IS NULL`; JWT permissions claim ignored | `rbac_resolver_test.go` | none |
| `apps/api/internal/identity/patient_account.go` | patient | High | reviewed | `PatientAccountID()` fails closed for staff/dev/zero-scope | `patient_account_test.go` | none |
| `apps/api/internal/handler/admin.go` | rbac | High | reviewed | no client id trusted for authorization; forged facility → 404 | `admin_scope_matrix_test.go`, `admin_facility_scope_test.go` | none |
| `apps/api/internal/handler/{org_overview,notifications,schedule_options,catalog}.go` | rbac | High | reviewed | sibling-org/queue scope denied | `notification_facility_scope_test.go` | none |
| `apps/api/internal/handler/patient_appointments.go` | patient | High | reviewed | `WHERE patient_account_id=$1`, code never in predicate | `patient_appointments_test.go` | none |
| `apps/api/internal/handler/booking.go` | patient | High | reviewed | actor-derived ownership on INSERT; cancel re-check; random 6-char codes | `cross_facility_test.go`, `booking_checkin_test.go` | P3 F3 (audit actor) logged |
| `apps/api/internal/handler/patient.go` | patient | High | reviewed | code-only lookup, anti-enumeration fold, no `formatted_number` lookup | `patient_test.go` | none |
| `apps/api/internal/consent/policy.go` | consent | Med | reviewed | pure fail-closed evaluator; design-only, not a live gate | `consent` unit tests | none |
| `apps/api/internal/audit/service.go` | audit | Med | reviewed | hash chain not reproducible from stored row | `audit/service_test.go` | P3 F1 logged |
| `packages/db/seed/regional_tenancy.sql` | rbac | Med | reviewed | `regional_admin` seeded with `organization.manage` (inert today) | `org_tenancy_test.go` | P3 F2 logged |

### Frontend runtime / functional

| FILE | SUBSYSTEM | RISK | REVIEW_STATUS | FINDING | TEST_EVIDENCE | ACTION |
| --- | --- | --- | --- | --- | --- | --- |
| `apps/web/src/routes/+layout.svelte` | frontend | High | reviewed + FIXED | client session store never hydrated → signed-in 403 showed sign-in panel | new source-contract tests; 751/751 | **fixed**: browser-only hydration |
| `apps/web/src/lib/stores/session.ts` + `session.test.ts` | frontend | High | reviewed + FIXED | hydration regression uncovered; +2 tests added | 11/11 in session.test.ts | fixed |
| 7 admin `+page.svelte` pages | frontend | Med | reviewed + FIXED | session subscribers never unsubscribed | svelte-check 0/0 | **fixed**: `onDestroy` teardown |
| `admin/queues/+page.svelte` | frontend | Med | reviewed | reference implementation (correct teardown) | — | none |
| 24 `routes/api/v1/**/+server.ts` proxies | frontend | High | reviewed | JWT from server session; path ids encoded; admin proxies use throwing `apiBase()` | `proxy-contracts` | P3 F4 logged |
| `lib/api/endpoints/{admin,patient,citizen,public}.ts` | frontend | Med | reviewed | paths/verbs correct; one dead helper (`getNotification`) | unit specs | none |
| citizen pages (`patient/appointments`, `patient/status`, `appointments/new`, `check-in`, `queues/new`) | frontend | High | reviewed | no token in storage/URL; deep-link code stripped via `replaceState`; no `{@html}` | E2E 4/4 + citizen specs | none |
| `e2e/{selfhosted-auth-journey,admin-org-isolation,admin-read}.spec.ts` | frontend | High | reviewed | signed-out 403 covered; signed-in 403 presentation not covered | 27/2 dev-data, 4/4 | gap closed by P2 fix |

### Historical Strix findings (2026-09-13 run `sehat-hub_9697`) — all REMEDIATED at this head

| ID | Original severity | Current status | Evidence in current head |
| --- | --- | --- | --- |
| vuln-0002 dev identity header bypass | Critical | **fixed** | `dev_provider.go` grants read-only permissions; `envguard.go` blocks outside `local` + loopback binding; `auth.ts` never injects the header |
| vuln-0003 patient status enumeration | Medium | **fixed** | `patient.go` matches only random `checkin_code`; no `formatted_number` lookup; ambiguous fold fails closed |
| vuln-0001 notification template concatenation | Medium | **fixed** | `booking.go` uses `{queue_number}` placeholder + `TemplateVars`; `renderer.go` RenderTemplate allow-list; `masking.go` denylist covers separated digits |

---

## 2. Confirmed findings and actions

### F-1 — P2 — Client session store never hydrated (FIXED)
`apps/web/src/routes/+layout.svelte`: `setSession()` was called only by tests; the root
layout read `data.hasSession` for the header but never mirrored it into the shared store
that admin pages and four citizen pages consume. A genuine signed-in staff member
receiving a 403 saw the "sign in again" panel instead of the permission-denied panel.
- Root cause: incomplete wiring after the shell rework.
- Correction: `$: if (browser) setSession({ hasSession: data.hasSession ?? false });`
  — browser-only because the store is a module singleton shared across SSR requests.
- Regression: two source-contract tests in `session.test.ts` (hydration present; no SSR write).
- Verification: `pnpm check` 0/0; `pnpm test:unit` 751/751.

### F-2 — P3 — `regional_admin` seeded with `organization.manage`
`packages/db/seed/regional_tenancy.sql:117`. Inert today (no route gates on that key),
but contradicts the documented "no `.manage` keys" invariant in `facility_scope.go:47-49`
and `rbac_resolver.go:243-244`. Proposed: remove the grant from seed and mirrored test
helper, or formally drop the invariant and enforce scope provenance on future org writes.
**Not changed in this pass** (seed/test data; owner decision on the invariant).

### F-3 — P3 — Audit hash not reproducible from stored row
`apps/api/internal/audit/service.go:65,105,198`: `computeHash` internally stamps
`At: time.Now()`, while the INSERT writes a separate `time.Now()` to `occurred_at`.
No verifier can recompute `event_hash` from the stored row. Proposed: compute one
`occurredAt` in `LogEvent`, thread it into the hash and the INSERT. **Not changed**
(best-effort chain; no live verifier exists yet).

### F-4 — P3 — Booking/cancel/check-in audit events drop the authenticated actor
`apps/api/internal/handler/booking.go:235,750,893,1033-1047` hardcode
`ActorType:"anonymous"` even for signed-in citizens; 403 cancel denials are not audited.
Proposed: pass `identity.ActorFromContext` into `logBookingEvent` and emit a cancel-denied
event. **Not changed.**

### F-5 — P3 — Admin proxies use throwing `apiBase()`
~20 admin `+server.ts` files call `apiBase()` (throws when `SIGAP_API_INTERNAL` unset)
instead of `resolveApiBase()` (503 envelope). Patient/public proxies were migrated;
admin proxies were not. Proposed: mirror `patient/appointments/+server.ts:14-15`.
**Not changed.**

### F-6 — P3 — Seven admin pages leaked session subscribers (FIXED)
`admin/{+page, organizations/+page, organizations/[id]/+page, schedules/+page,
appointments/+page, facilities/+page, notifications/+page}.svelte` assigned the
unsubscribe handle from `subscribeToSession` but never called it. Fixed with
`onDestroy(() => unsubscribeSession?.())`. Would have become a stale-state bug once
F-1 was fixed. Verification: svelte-check 0/0, 751/751.

---

## 3. Open gaps register (not application defects; owner/infra-blocked)

Confirmed still OPEN at this head — do not report as new findings:
- **IDENTITY_DEACTIVATION_POLICY=OPEN** — disabled identity still signs in at Better Auth;
  Go denies. Concrete proposal in `design/identity-deactivation-policy-adr.md`.
- `SESSION_LIFECYCLE=PARTIAL`, `STAFF_MFA=PARTIAL`, `PRODUCTION_READINESS=NOT_READY`.
- CSRF `trustedOrigins` unset; compose does not forward `BETTER_AUTH_*`.
- Real `BETTER_AUTH_SECRET` / https origin enforcement (deploy env).
- TLS edge, SMTP transport, least-privilege `auth` DB role, durable rate-limit store,
  sign-in/out audit sink.
- Migration 0011 production convergence and regional migrations 0012–0014 are **not**
  deployed; `release/sigap-regional-v1` is proposed, not created.

## 4. Quality gates preserved

Nine-job CI intact; Better Auth integration with `REQUIRE_DB=1`; zero DB-backed skips in
the enforced suites; Go/Rust vulnerability scanning with two documented cargo exceptions;
Gitleaks default rules with exact-fingerprint exclusions; 19 historical design authors
grandfathered by exact commit SHA; new unauthorized automation identities rejected; no AI
co-author trailers; historical/regional migration classifiers; deployment safety guards.
No gate was weakened to obtain a PASS.

## 5. Local regression after fixes (2026-11-18 baseline)

- `pnpm check` (svelte-kit sync + svelte-check): **0 errors, 0 warnings**.
- `pnpm test:unit` (vitest): **38 files, 751 tests passed** (749 baseline + 2 new).
- `git var GIT_AUTHOR_IDENT`: `lostsky1403 <moch22188ti@student.nurulfikri.ac.id>`.
- No push, no merge, no production deploy, no production DB change, no history rewrite.

---

## 6. Follow-up review 2026-11-19 — reassessment, fixes and expanded coverage

Review head: local `c53d591` (the 2026-11-18 fix commit) on top of remote `cff2d35`.
Go source byte-identical to the ledger's original head except the fixes below.

### 6.1 Previous fixes re-verified
- **F-1 (session hydration) — PASS.** `+layout.svelte:72` is
  `$: if (browser) setSession({ hasSession: data.hasSession ?? false });`.
  Browser-only, so no SSR module-singleton mutation; the reactive statement re-runs
  on navigation/session updates. Added **behavioral component tests**
  (`src/lib/ui/error-presentation.test.ts`, 5 tests): signed-out 403 → sign-in panel,
  signed-in 403 → permission-denied panel, 401 → sign-in in both states.
- **F-3 (subscription teardown) — PASS.** All seven admin pages call
  `onDestroy(() => unsubscribeSession?.())`.
- Source contract remains in `session.test.ts` (11 tests).

### 6.2 Four open findings reassessed — all CONFIRMED, all fixed
- **A — `regional_admin` `organization.manage` — CONFIRMED P3 (latent/inert).**
  No route gates on it today, but unscoped grants expand per-facility, so a future
  org-write route would authorize everywhere. Fixed at the proper location
  (seed + mirrored test helper + seed SQL guard), **no migration touched**. Regression
  assertion added to `org_tenancy_test.go`.
- **B — audit hash not reproducible — CONFIRMED P3.** `computeHash` stamped its own
  `time.Now()` while the INSERT used another. Fixed by threading ONE `occurredAt`
  into both; `computeHash` now takes it as a parameter. New test
  `TestComputeHash_ReproducibleFromStoredAt`. No schema change (`occurred_at` exists).
- **C — booking audit drops actor — CONFIRMED P3 (forensic).** `logBookingEvent` now
  derives actor from `identity.ActorFromContext` (trusted JWT), never client data;
  distinct anonymous/user/dev/system types. Added `appointment.cancel.denied` event
  for the previously unaudited ownership 403. Ownership logic untouched.
- **D — admin proxies throwing `apiBase()` — CONFIRMED P3.** All 19 admin proxies
  plus 2 public proxies migrated to `resolveApiBase()` (graceful 503 envelope,
  fail-closed, no upstream request). New `proxy-config.test.ts` (6 tests) + contract
  manifest updated. No auth/config failure is converted to success.

### 6.3 Remaining high-risk review — 3 new P3, 2 fixed
Recounted: prior 105 + 22 newly reviewed = **127 high-risk files reviewed**;
**≈20 remaining** (mostly unexecuted test suites, a few frontend components, and
94 low-risk design assets — not pixel-reviewed). No P0/P1/P2.
- **NEW-1 — notification audit correlation — OPEN P3 (documented).** `notifications.go`
  `log()` uses `context.Background()` and sets no RequestID/FacilityID/ResourceID
  across 34 call sites. Forensic gap only; no authz bypass. Left OPEN rather than
  risk a 34-site refactor before publication; exact correction recorded in §7.
- **NEW-2 — crashed worker rows stuck in `processing` — FIXED.** Added
  `requeueStaleProcessing()` at the start of the real worker path; idempotent,
  preview stays read-only.
- **NEW-3 — SSE frame injection via `facilityId` — FIXED.** `queue.go` now marshals
  a typed struct with `json.Marshal` instead of `fmt.Sprintf`.

### 6.4 Identity-deactivation ADR — reviewed, corrected, still owner-gated
`design/identity-deactivation-policy-adr.md` corrected: the earlier "fail-open for
citizens" assumption was wrong (the `user.create.after` hook provisions every
`app_users` row). The recommended gate now **fails closed** on inactive/deleted,
on a missing row, and on a query error — satisfying the invariant that a disabled
identity cannot regain access because a dependent check failed. Added a six-state
analysis (§4A) and a DB-outage negative-control test to §5. **Not implemented**;
`IDENTITY_DEACTIVATION_ADR=RECOMMENDED`, `SESSION_LIFECYCLE=PARTIAL`, `STAFF_MFA=PARTIAL`.

### 6.5 Local regression evidence (all PASS)
- `pnpm check`: 0 errors / 0 warnings.
- `pnpm test:unit`: **40 files, 762 tests passed** (751 + 11 new).
- `pnpm build` (production, adapter-node): PASS.
- `node tests/proxy-contracts.test.js`, `auth-actions.test.js`, `build-verification.test.js`: PASS.
- `go build ./...`: PASS. `go test ./...`: PASS (DB-gated suites skip locally without
  `DATABASE_URL`; CI provides disposable Postgres).
- `cargo test --lib`: 7/7. `cargo audit`: 0 vulnerabilities (1 documented allowed warning).
- `gitleaks detect` (975 commits, default rules + exact-fingerprint ignore): no leaks.
- Attribution + commit-identity gates: PASS (verified after commit).

### 6.6 Verdict after follow-up
P0=0 · P1=0 · P2=0 (F-1 fixed) · confirmed P3: A/B/C/D + NEW-2/NEW-3 fixed,
NEW-1 OPEN (forensic). AUTH_TRUST_BOUNDARY=PARTIAL, TENANT_ISOLATION=PASS,
PATIENT_DATA_ISOLATION=PASS. No release branch, no merge, no production deploy.

---

## 7. Open P3 corrections (not applied — recorded for a follow-up)

**NEW-1 — notification admin audit correlation.** `apps/api/internal/handler/
notifications.go:651` `log()` calls `h.audit.LogEvent(context.Background(), ...)`
with no `RequestID`, `FacilityID` or `ResourceID`; 34 call sites pass only actor
and strings. Impact is forensic only (actor is recorded; authorization is enforced
independently). Exact correction when taken up: thread `r.Context()` into `log()`,
set `RequestID: identity.RequestIDFromContext(r.Context())`, pass the target
notification's `ResourceID` and `FacilityID` where the row is available, and keep
`SanitizeMetadata`. Test: as a staff actor call
`POST /api/v1/admin/notifications/{id}/retry`, take the response `X-Request-ID`,
assert the matching `audit_events` row carries that request id, the facility and
the notification id. Deferred to avoid a 34-site refactor in the publication commit.

**Known owner/infra-blocked items remain OPEN** (§3): identity-deactivation policy,
trustedOrigins, deploy-env secret/https, TLS edge, SMTP, least-privilege auth role,
durable rate limiting, audit sink, and the undeployed regional migrations. None are
application defects fixed here; none had a gate weakened to obtain a PASS.

