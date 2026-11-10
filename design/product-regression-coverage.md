# Product Regression Coverage — Track F (Regional Pilot)

This map answers one question for each Track F workflow: **is it already
proven by a RUNNING test, or is it DESIGN-ONLY?** It does not re-test; it
points at the existing evidence and names the exact file. A "RUNNING" verdict
means an executable test asserts the claim today; "DESIGN-ONLY" means the
claim lives in a design doc or is inferred, with no test that would fail if it
broke.

> Scope caveat, stated once: per-actor scoping (org-A / org-B / regional) is
> enforced server-side in Go and is NOT browser-switchable. Those boundaries
> are proven by Go tenant/scenario tests + the direct API, not by Playwright.
> The browser specs intentionally cover the surfaces a browser can drive:
> super-admin, zero-scope (denied), and responsive rendering.

---

## 1. Citizen facility discovery across orgs

**Verdict: RUNNING (Go), browser-approved on a single seat.**

| Evidence | Location | Kind |
|---|---|---|
| Scenario A — cross-org discovery + booking | `apps/api/internal/handler/regional_scenarios_test.go` → `TestRegionalScenarios_CitizenCrossOrgDiscoveryAndBooking` | Go (integration) |
| Public catalog contract A–F | `apps/api/internal/handler/catalog_test.go` → `TestListPublicFacilities_ExactFieldSet`, `_TypeIsEnumValue`, `_TypeIsWireValueNotLabel`, `_ExcludesInactive`, `_NoAdminOnlyFields`, `_ExplicitlyNoBedCountsOrAddress`, `_NoAuthRequired`, `_EmptyCatalogIsNullOrEmpty` | Go (integration) |
| Catalog → booking handoff | `apps/web/e2e/citizen-transactions.spec.ts` (reads `/api/v1/public/facilities` + `/public/service-units`, books by click) | Playwright |
| /faskes shell | `apps/web/e2e/citizen-shell.spec.ts` | Playwright |

Scenario A proves the public `ListPublicFacilities` catalog surfaces **both**
organizations' facilities (`facA` and `facB`) and never leaks `organization_id`,
`address`, `phone`, `total_beds`, or `available_beds`. The browser half proves
the catalog → booking-selection handoff on the seeded stack, but the
**cross-org surface itself is server-side only** (see §7).

---

## 2. Booking at a selected facility

**Verdict: RUNNING (Go + Playwright).**

| Evidence | Location |
|---|---|
| Book route, validation, code generation | `apps/api/internal/handler/booking_test.go` → `TestPublicAppointmentsRouter_BookAppointment`, `TestBookAppointment_*`, `TestValidateBookAppointment`, `TestGenerateCheckinCode*`, `TestNormalizePhone` |
| Scenario A booking leg | `regional_scenarios_test.go` → `TestRegionalScenarios_CitizenCrossOrgDiscoveryAndBooking` (appointment persisted against the CHOSEN `facA`) |
| Booking → check-in code | `apps/web/e2e/citizen-transactions.spec.ts` ("A. booking") |
| Booking helper (confirmation code) | `apps/web/e2e/checkin-code-journey.spec.ts` → `book()` |

---

## 3. Facility-specific check-in

**Verdict: RUNNING (Go + Playwright).**

| Evidence | Location |
|---|---|
| Atomic transition, rate limiter, concurrency | `apps/api/internal/handler/booking_checkin_test.go` (`TestCheckIn_AtomicTransition_*`, `TestCheckIn_RateLimiter_*`, `TestCheckIn_ConcurrentAttempts_ExactlyOneWins`, `TestCheckIn_QueueFailure_RollsBackStatus`) |
| Check-in route + engine fallback | `apps/api/internal/handler/booking_test.go` (`TestCheckIn_*`, `TestCheckIn_QueueFallback_*`, `TestCheckIn_EngineTicketNotInLocalDB_IsRefusedAndReleased`) |
| Admin-side mints the ticket | `apps/api/internal/handler/admin_checkin_ticket_test.go` |
| Check-in deep link → queue ticket | `apps/web/e2e/citizen-transactions.spec.ts` ("B. check-in") |

---

## 4. Queue creation through the Rust engine

**Verdict: RUNNING (cargo + Go); browser mint path NOT exercised.**

| Evidence | Location |
|---|---|
| Concurrency guardrail (unique sequential numbers) | `apps/queue-engine/tests/concurrency_guardrail.rs` — `cargo test --test concurrency_guardrail` |
| estimated-wait regression (== 25) | `apps/queue-engine/tests/estimated_wait_regression.rs` |
| WIB service-day unit tests | `apps/queue-engine/src/engine/queue.rs` (7 × `service_day_wib_*`) |
| Engine mint bridge (Go → gRPC) | `apps/api/internal/handler/admin_checkin_ticket_test.go` (`_MintsQueueTicketAndAttaches`, `_SecondAttemptDoesNotMintSecondTicket`, `_EngineUnavailableRollsBackToScheduled`, `_ConcurrentOnlyOneTicket`) + `booking_test.go` (`TestCheckIn_EngineTicketNotInLocalDB_IsRefusedAndReleased`, `_AlreadyLocal_NotDuplicated`) |

The Rust engine's mint path — sequential uniqueness, `estimated_wait_minutes`,
the WIB counter key — is proven by `cargo test` and its Go bridge. **No
Playwright spec asserts a ticket minted through the engine** (the `admin-*`
mutation specs only move existing tickets between statuses).

---

## 5. WIB service-day allocation / listing

**Verdict: RUNNING (Go + Rust); midnight reset not browser-exercised.**

| Evidence | Location |
|---|---|
| Scenario F — DB-backed allocator | `regional_scenarios_test.go` → `TestRegionalScenarios_MultiFacilityWibMidnightQueueSequence` (per-facility/per-service-day counter, WIB-midnight reset, `ListQueueTickets` agreement) |
| Pure WIB service-day internals | `apps/api/internal/handler/service_day_test.go` (8 × `TestServiceDayRange_*`, incl. `_AllocationMatchesListingAcrossBoundary`) |
| Rust boundary | `apps/queue-engine/src/engine/queue.rs` (7 × `service_day_wib_*`) + `concurrency_guardrail.rs` |
| Rendered board == API | `apps/web/scripts/verify-service-day-render.mjs` (Track A deliverable 1; headless Chromium, 48/22 rows) |
| /admin/queues READ | `apps/web/e2e/admin-read.spec.ts` |

The allocation↔listing invariant at the WIB-midnight boundary is Go + Rust
proven. The only browser-level evidence is the standalone
`verify-service-day-render.mjs` script, which is **not part of the Playwright
`.spec.ts` suite** and does not drive the midnight reset.

---

## 6. Facility-scoped admin ops

**Verdict: RUNNING (Go + Playwright).**

| Evidence | Location |
|---|---|
| Facility scope: consumers, fail-closed, service-day | `apps/api/internal/handler/admin_facility_scope_test.go` (`TestFacilityScope_AdminGlobalSuperAdminConsumers`, `_GlobalScopeDoesNotGrantPermission`, `_AdminConsumersFailClosed`, `_ListQueueTicketsServiceDay`) |
| Facility scope matrix (19 cases) | `apps/api/internal/handler/facility_scope_test.go` + `apps/api/internal/auth/facility_scope_test.go` + `admin_scope_matrix_test.go` |
| Facility create/update/deactivate, scoped refusal | `apps/web/e2e/admin-facility-mutations.spec.ts` |
| Denied schedule / notification destinations | `apps/web/e2e/admin-schedule-denied.spec.ts`, `apps/web/e2e/admin-notification-denied.spec.ts` |

---

## 7. Org-scoped administration

**Verdict: RUNNING (Go); the org list destination only is browser-exercised.**

| Evidence | Location |
|---|---|
| Org tenancy (7 cases) | `apps/api/internal/handler/org_tenancy_test.go` (`_OrgAdminFacilityScopeExpansion`, `_OrgAdminListOrganizationsOnlyOwn`, `_ForgedOrganizationIDDenied`, `_ForgedFacilityIDDenied`, `_OperatorCrossOrgQueueIsolation`, `_RegionalAdminReadOnly`, `_AuditCarriesActorAndScope`) |
| `/admin/organizations` read | `apps/web/e2e/admin-read.spec.ts` (super-admin populated + zero-scope fail-closed) |

Org-admin isolation — cross-org queue isolation, regional-admin read-only,
forged org/facility ID denial — is proven in Go only. The browser spec covers
the `/admin/organizations` **read destination** rendering, because the
isolation itself is server-enforced and not switchable from a browser session.

---

## 8. Status / cancellation

**Verdict: RUNNING (Go + Playwright + vitest); citizen self-cancel has no browser E2E.**

| Evidence | Location |
|---|---|
| Citizen self-cancel by code (8 cases) | `apps/api/internal/handler/booking_test.go` (`TestCancel_EmptyCode_Returns400`, `_InvalidCharacters_Returns400`, `_RateLimited_Returns429`, `_Scheduled_Returns200AndCancels`, `_ConfusableFolded_Resolves`, `_AlreadyCancelled_Idempotent200`, `_CheckedIn_Returns409`, `_UnknownCode_Returns404`) |
| Status lookup round trip + confusable | `apps/web/e2e/checkin-code-journey.spec.ts` |
| Citizen status (find visit, unknown code) | `apps/web/e2e/citizen-transactions.spec.ts` ("C. status") |
| Admin appointment/queue/notification cancel | `apps/web/e2e/admin-mutations.spec.ts` (T-3B5-02 dialogue, T-3B5-05 retry/cancel) |
| Vitest: status transitions + progress | `apps/web/src/lib/citizen/patient-status-flows.test.ts`, `checkin-flows.test.ts`, `walkin-flows.test.ts`, `src/lib/domain/status.transitions.test.ts` |

Status lookup and admin-side cancellation are fully browser-exercised. The
**citizen self-cancellation** workflow (`POST /api/v1/appointments/cancel`) has
full Go coverage and an API proxy route
(`apps/web/src/routes/api/v1/appointments/cancel`), but no citizen UI page and
no Playwright spec.

---

# Browser E2E not yet exercised (honest gaps)

1. **Cross-org / org-scoped isolation** — facility discovery spanning two
   organizations (leak guard), org-admin scope expansion, cross-org queue
   isolation, and regional-admin read-only are Go-proven only; not browser-
   exercisable because scoping is server-side, not switchable per tab.
2. **Rust-engine queue mint path** — ticket sequence uniqueness and
   `estimated_wait_minutes` are `cargo test` / Go-bridge proven; no Playwright
   assertion mints a ticket through the engine.
3. **WIB-midnight allocation reset** — allocation↔listing boundary agreement is
   Go (`service_day_test.go`, Scenario F) + Rust proven; the only browser
   render evidence (`verify-service-day-render.mjs`) lives outside the
   `.spec.ts` suite and does not exercise the reset.
4. **Citizen self-cancellation** — the public `CancelAppointment` endpoint is
   Go-tested and API-proxied, but there is no citizen UI and no browser E2E
   for the cancellation workflow.

**EXISTING_SIGAP_REGRESSIONS verdict basis:** Track F is already regression-
covered by RUNNING evidence end-to-end — Go tenant/scenario tests
(`org_tenancy_test.go`, `regional_scenarios_test.go`, Scope A/F, `catalog_test.go`,
`service_day_test.go`, `facility_scope_test.go`, `booking_test.go`),
Rust engine tests (`cargo test` + `queue.rs` boundary), SvelteKit vitest, and
the Playwright suite — leaving only four browser-level paths (cross-org
isolation, engine mint, WIB-midnight reset, citizen self-cancel) unexercised by
E2E, each already proven elsewhere or server-side by design.