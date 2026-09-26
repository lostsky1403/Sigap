# SIGAP Redesign - Executable Task Breakdown (Phase 3A, corrected in Phase 3A.1)

Status: Phase 3A planning artifact. DO NOT EXECUTE. These tasks are the ordered work list for Phase 3B0-3B7.
Phase 3A.1 applied the GATE 0 closure corrections: the global-super_admin decision, strict
GATE-1-before-3B1 sequencing, the 50-task count, the practitioner rule, and the token-drift fix.
Phase 3A.3 applied the final executable-graph normalization: the test tooling foundation is now **first**
inside 3B1 (T-3B1-01), GATE 2 in this document now matches the plan exactly, and the Phase 3B2 checkpoint
(T-3B2-05) is no longer labelled as a GATE 3. Phase 3A.2 applied the execution-consistency corrections:
strict post-GATE-2 ordering, actor-class normalization of zero-assignment wording, and the Playwright E2E
tooling moved to the front of 3B1.

TOTAL TASKS: **50** (3B0 = 9, 3B1 = 8, 3B2 = 5, 3B3 = 6, 3B4 = 9, 3B5 = 6, 3B6 = 4, 3B7 = 3).

Canonical phase order (STRICT, Phase 3A.2): GATE 0 -> 3B0 -> GATE 1 -> 3B1 -> GATE 2 -> 3B2 -> 3B3 ->
GATE 3 -> 3B4 -> GATE 4 -> 3B5 -> GATE 5 -> 3B6 -> GATE 6 -> 3B7. 3B1 MUST NOT begin before GATE 1.
No later stage starts before the previous stage's gate passes.

Companion documents:

- `design/sigap-redesign-implementation-plan.md` (the plan; Sections referenced below are its sections)
- `design/sigap-redesign-implementation-handoff.md` (the frozen design contract, Phase 3A corrected)
- `design/signap-phase1-ux-audit.md` (owner decisions D1-D14, R3-F1, R3-F2)

Each task is sized for one focused implementation pass, is dependency-aware, testable, and rollback-aware.

Verification commands used by acceptance criteria:

| Alias | Command |
|---|---|
| `GOTEST` | `cd apps/api && go test ./...` |
| `GOVET` | `cd apps/api && go vet ./...` |
| `ROUTERCOV` | `cd apps/api && go test ./internal/router/ -coverprofile=router.cov` then assert total >= 90% |
| `RUSTTEST` | `cd apps/queue-engine && cargo test` |
| `WEBCHECK` | `pnpm --filter sigap-web run check` |
| `WEBTEST` | `pnpm --filter sigap-web test` |
| `E2E` | Playwright suite against a local seeded stack (`SIGAP_ENV=local`) |

---

## Phase 3B0 - P0 Security and Broken Proxy Remediation

Phase 3B0 execution order inside the backend security work (Phase 3A.4): the common facility-scope
contract (**T-3B0-07**) lands **first**, because T-3B0-04, T-3B0-05, and T-3B0-06 all consume the same
centralized scope decision and must not duplicate it. The three notification fixes then run against that
contract, followed by the seed verification (T-3B0-08) and the GATE 1 verification (T-3B0-09). The proxy
repairs (T-3B0-02, T-3B0-03) are independent frontend work and may proceed in parallel with the backend
security chain.

### T-3B0-01 - Commit the Phase 3A/3A.1 design artifacts

| Field | Detail |
|---|---|
| Goal | Bring the `design/` specs under version control as intended by D1, while `design/generated/` stays ignored. Includes the Phase 3A.1 corrections (token drift in `SKILLS.md`, canonical note in `DESIGN.md`). |
| Files / areas | `design/sigap-redesign-implementation-plan.md`, `design/sigap-redesign-task-breakdown.md`, `design/sigap-redesign-implementation-handoff.md`, `design/signap-phase1-ux-audit.md`, `design/DESIGN.md`, `design/SKILLS.md`, `design/brand.json`, `design/system/**`, `.gitignore` |
| Dependencies | None |
| Acceptance criteria | `git status` shows the design specs staged; `git check-ignore design/generated/sigap-warga-desktop/pages/beranda.html` still matches; no generated page HTML is staged. |
| Tests | `git check-ignore -v design/generated/...` and a `git status` review |
| Risk | Low. Committing the pending `.gitignore` edit could accidentally un-ignore generated artifacts if edited carelessly. |
| Rollback | `git reset` the commit; nothing in production changes. |

### T-3B0-02 - Fix the nine literal-parameter proxies

| Field | Detail |
|---|---|
| Goal | Make every dynamic admin proxy forward `event.params.id` instead of a literal `ID`/`STATUS`. |
| Files / areas | `apps/web/src/routes/api/v1/admin/appointments/[id]/status/+server.ts`, `admin/facilities/[id]/+server.ts`, `admin/facilities/[id]/deactivate/+server.ts`, `admin/notifications/[id]/cancel/+server.ts`, `admin/notifications/[id]/retry/+server.ts`, `admin/queues/[id]/+server.ts`, `admin/queues/[id]/status/+server.ts`, `admin/schedules/[id]/+server.ts`, `admin/service-units/[id]/+server.ts` |
| Dependencies | None |
| Acceptance criteria | No proxy source contains the literals `'/api/v1/admin/.../ID'` or `.../STATUS'`; every dynamic proxy interpolates `event.params.id` using the `encodeURIComponent` pattern from `appointments/[id]/check-in/+server.ts`; the two non-dynamic proxies are untouched. |
| Tests | New proxy path test asserting each of the 9 files interpolates and none contains a literal; `WEBCHECK`; `WEBTEST` |
| Risk | Low. The current behavior is broken, so this can only improve. |
| Rollback | Revert the task commit. |

### T-3B0-03 - Fix the facility deactivate proxy method

| Field | Detail |
|---|---|
| Goal | Make the deactivate call reach the backend. |
| Files / areas | `apps/web/src/routes/api/v1/admin/facilities/[id]/deactivate/+server.ts` |
| Dependencies | T-3B0-02 (same file family) |
| Acceptance criteria | The file exports `PATCH` (not `POST`); the path interpolates the id; the UI's `PATCH` call at `admin/facilities/+page.svelte:138` now reaches `PATCH /api/v1/admin/facilities/{id}/deactivate`; the backend's one-way soft delete is preserved. |
| Tests | Method contract test asserting UI method == proxy export == Go route method; `WEBCHECK` |
| Risk | Low-medium. `POST` on that prefix maps to `CreateFacility` on the backend, so the previous behavior was worse than broken. |
| Rollback | Revert the task commit. |

### T-3B0-04 - Fix notification retry authorization ordering (P0-1)

| Field | Detail |
|---|---|
| Goal | Enforce facility scope before the retry mutation. |
| Files / areas | `apps/api/internal/handler/notifications.go` (`RetryNotification`), optionally `apps/api/internal/notification/service.go` (`Retry`) |
| Dependencies | **T-3B0-07 (common facility-scope contract).** Source inspection confirms the dependency: `RetryNotification` enforces scope via `auth.CanAccessFacilityForActor` (`notifications.go:314`), which under the approved contract must consume the same centralized `FacilityScopeResult` as `AllowedFacilityIDsForActor`. Scope must be decided before the write, so the contract lands first. This task fixes **ordering**; it does not redefine scope. |
| Acceptance criteria | An out-of-scope retry returns 404 **and** leaves `status`, `attempt_count`, and `next_attempt_at` byte-identical; an in-scope retry succeeds and mutates; an absent id returns 404 with no mutation; an in-scope wrong-state retry returns 409. A DB-resolved active global `super_admin` may retry any facility's row; a zero-assignment non-super_admin cannot. |
| Tests | New DB-backed regression test (greppable name following the existing pattern, so CI's execution proof applies); `GOTEST`; `ROUTERCOV` |
| Risk | Low. Response shapes unchanged for legitimate callers. |
| Rollback | Revert the task commit. No migration, so no data action. |

### T-3B0-05 - Fix notification cancel authorization ordering (P0-2)

| Field | Detail |
|---|---|
| Goal | Enforce facility scope before the cancel mutation. |
| Files / areas | `apps/api/internal/handler/notifications.go` (`CancelNotification`), optionally `apps/api/internal/notification/service.go` (`Cancel`) |
| Dependencies | **T-3B0-07 (common facility-scope contract).** Source inspection confirms the dependency: `CancelNotification` enforces scope via `auth.CanAccessFacilityForActor` (`notifications.go:349`), which under the approved contract consumes the centralized `FacilityScopeResult`. This task fixes **ordering**; it does not redefine scope. T-3B0-04 is a sibling consumer, not a prerequisite; the two may share an ordering helper but neither task may duplicate the scope decision. |
| Acceptance criteria | An out-of-scope cancel returns 404 and leaves `status` unchanged; an in-scope cancel mutates; in-scope re-cancel of an already-cancelled row still succeeds (idempotency preserved); an in-scope delivered row returns 409. A DB-resolved active global `super_admin` may cancel any facility's row; a zero-assignment non-super_admin cannot. |
| Tests | New DB-backed regression test; `GOTEST`; `ROUTERCOV` |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B0-06 - Fix notification summary empty-scope fallback (P0-3)

| Field | Detail |
|---|---|
| Goal | An empty scope must yield all-zero counts, never a platform-wide aggregate. |
| Files / areas | `apps/api/internal/handler/notifications.go` (`GetNotificationSummary`); `apps/api/internal/notification/service.go` (`Summary`) for the durable sentinel removal |
| Dependencies | **T-3B0-07 (common facility-scope contract).** Source inspection confirms it: `GetNotificationSummary` (`notifications.go:188-213`) calls `AllowedFacilityIDsForActor` at `:191` and its current empty-scope path leaves `facilityFilter = uuid.Nil`, the global sentinel. Under the approved contract that branch must test `Unrestricted` (dev actor or DB-resolved active global `super_admin`) before deciding, which is the contract T-3B0-07 introduces. This task fixes the **sentinel/global fallback**; it does not redefine scope. |
| Acceptance criteria | Summary counts by actor class: zero-assignment **non-super_admin** -> all five counts zero; facility-scoped actor -> only that facility's counts; dev actor -> global counts; DB-resolved active global `super_admin` -> global counts; inactive/soft-deleted global `super_admin` assignment -> all-zero / fail closed; resolver failure -> all-zero; the response always contains all five status keys (`pending`, `processing`, `delivered`, `failed`, `cancelled`), zero-filled where absent. |
| Tests | New regression test covering every assertion above (seven actor-class outcomes plus the five-key invariant); `GOTEST` |
| Risk | Medium. If the `Service.Summary` signature is changed, update all callers in the same commit. If that is judged too invasive, ship the handler-side short-circuit alone (which closes the leak) and record the sentinel removal as a follow-up. |
| Rollback | Revert the task commit. |

### T-3B0-07 - Introduce the explicit facility-scope contract across ALL consumers (P0-4, P0-5)

| Field | Detail |
|---|---|
| Goal | Introduce the centralized `FacilityScopeResult{IDs, Unrestricted, Err}` contract (plan Section 4.5) and migrate **every** facility-scope consumer in the repository to it - not only the eight P0-5 detail/mutation handlers. A DB-resolved active global `super_admin` typically has `Unrestricted = true` **and** `IDs` empty, so every consumer must evaluate `Unrestricted` **before** it interprets an empty `IDs` set. The previously-approved decisions are unchanged: unrestricted is explicit (dev actor, or DB-resolved active global `super_admin`), an empty set alone is never unrestricted, permissions stay mandatory, and there is no DB migration. |
| Files / areas | `apps/api/internal/auth/facility_scope.go` (the centralized resolver and `CanAccessFacilityForActor`); every consumer listed in the inventory below across `apps/api/internal/handler/admin.go` and `apps/api/internal/handler/notifications.go`; the global-super_admin determination query from plan Section 4.5.1. |
| Dependencies | None. This task **owns the common facility-scope contract**; T-3B0-04, T-3B0-05, and T-3B0-06 may consume it (see their dependency lines). |
| First implementation step (mandatory) | **Source-level audit before any code change.** Run a repository-wide search for every call site of `AllowedFacilityIDsForActor` and `CanAccessFacilityForActor` (and any replacement `FacilityScopeResult` resolver/helper introduced by this task), and record the exact discovered consumer list with file and line numbers. **Source grep wins**: if the repository contains consumers beyond the inventory below, the discovered list is authoritative and this task's scope grows to cover them. Do not invent endpoint names that the source does not contain. |
| Verified consumer inventory (Phase 3A.4, 19 call sites across 17 handlers in `admin.go` + 5 call sites across 5 handlers in `notifications.go`; **24 production call sites / 22 production consumer handlers total**, plus 2 direct test-only calls in `facility_scope_test.go`) | **Admin LIST paths (5 handlers / 5 call sites):** `ListFacilities` (`admin.go:104`), `ListQueueTickets` (`:575`), `ListServiceUnits` (`:912`), `ListSchedules` (`:1367`), `ListAppointments` (`:1988`). **Admin DETAIL/MUTATION paths (12 handlers / 14 call sites):** the eight P0-5 handlers `GetFacility` (`:184`), `UpdateFacility` (`:353`), `DeactivateFacility` (`:400`), `GetQueueTicket` (`:652`), `GetServiceUnit` (`:993`), `UpdateServiceUnit` (`:1196`), `GetSchedule` (`:1453`), `UpdateSchedule` (`:1687`); plus `UpdateQueueStatus` (`:709`), `UpdateAppointmentStatus` (`:2086`), and the two create paths that authorize a **client-supplied** `facility_id`: `CreateServiceUnit` (`:1051`) and `CreateSchedule` (`:1516`). `UpdateServiceUnit` has an additional facility-scope call at `:1169`, and `UpdateSchedule` an additional call at `:1660`; those calls validate a client-supplied `facility_id` before their record-scope calls. **Notifications (5 handlers / 5 call sites):** `ListNotifications` (`:81`), `GetNotificationSummary` (`:191`), and the three `CanAccessFacilityForActor` consumers `GetNotification` (`:279`), `RetryNotification` (`:314`), `CancelNotification` (`:349`). **Direct test-only calls (2):** `facility_scope_test.go:889` (`AllowedFacilityIDsForActor`) and `:899` (`CanAccessFacilityForActor`); these must be migrated to the replacement helper signature but are not endpoint consumers. |
| Canonical consumer semantics (every consumer must implement this) | **CASE A** `Unrestricted = true` (dev actor OR DB-resolved active global `super_admin`): do **NOT** short-circuit merely because `IDs` is empty; intentionally omit the facility restriction; return global data / permit the operation; the endpoint's own permission check still applies. **CASE B** `Unrestricted = false` and `IDs` non-empty: the facility predicate/restriction MUST be applied. **CASE C** `Unrestricted = false` and `IDs` empty: LIST -> 200 + empty; DETAIL -> 404; MUTATION -> 404 with **zero** side effects; SUMMARY -> all five notification counts at zero. **CASE D** `Err != nil`: fail closed by endpoint class; a resolver failure is **never** reinterpreted as unrestricted. |
| `CanAccessFacilityForActor` contract | If it remains in use after the refactor it must internally consume the same explicit scope result (or an equivalent single server-side source of truth) and return **accessible** for the dev actor, a DB-resolved active global `super_admin`, and an actor whose facility id is in `IDs`; and **inaccessible** for a zero-assignment non-super_admin, an inactive/soft-deleted global assignment, an out-of-scope facility, and any resolver failure. No ad-hoc `if role == "super_admin"` checks may be scattered through handlers: the decision stays centralized in the auth/facility-scope layer. |
| Acceptance criteria | **(A)** Every facility-scope consumer found by the repository search is migrated to the explicit scope contract. **(B)** No consumer interprets `len(IDs) == 0` as unrestricted, or as automatically empty, without first considering `Unrestricted`. **(C)** A DB-resolved active global `super_admin` receives **global** results on every facility-scoped LIST endpoint for which it holds the endpoint permission (today's list handlers branch on `len(allowedFacilities) == 0` **before** any `unrestricted` test in several paths, and `ListNotifications` applies a post-fetch `allowedFacilities` membership filter that discards every row when `IDs` is empty - both patterns must be corrected, not preserved). **(D)** A DB-resolved active global `super_admin` can use every permitted detail/mutation endpoint globally. **(E)** A global `super_admin` that lacks a route's permission still receives the existing 403 permission denial, because unrestricted scope never grants permissions. **(F)** A facility-scoped actor remains limited to its assigned facilities. **(G)** A zero-assignment non-super_admin remains fail-closed. **(H)** A resolver failure remains fail-closed. Inactive/soft-deleted global assignments never grant unrestricted scope; token/role claims and client input never grant it. No new DB column. |
| Tests | The super_admin security matrix from plan Section 13.2 (cases A-G) **plus** a table-driven matrix over **every discovered facility-scope consumer** (not only the eight P0-5 handlers). For each applicable endpoint class: **global super_admin** - LIST returns global permitted rows and not `[]`; DETAIL can read any facility-scoped record; MUTATION can mutate any facility-scoped record where the route permission exists; SUMMARY receives global counts. **Facility-scoped actor A** - sees and mutates A, cannot see or mutate B. **Zero-assignment non-super_admin** - list empty; detail 404; mutation 404 with a byte-identical target row; summary all-zero. **Inactive / soft-deleted global super_admin** - behaves fail-closed. **Resolver error** - fail-closed. **Missing endpoint permission** - still denied even when `Unrestricted = true`. Regression assertions MUST cover the notification list post-fetch filter and all four supplied-`facility_id` authorization paths (`CreateServiceUnit`, `CreateSchedule`, the additional `UpdateServiceUnit` pre-check at `:1169`, and the additional `UpdateSchedule` pre-check at `:1660`), since these are consumer shapes not covered by the eight P0-5 handlers. `GOTEST`; `ROUTERCOV` |
| Risk | **Medium-high, intentional.** Zero-assignment **non-super_admin** actors lose their (incorrect) blanket access; a DB-resolved active global `super_admin` keeps unrestricted access per the GATE 0 decision (plan Section 4.5), so no legitimate demo identity regresses once the seed in T-3B0-08 is in place. The newly-in-scope list and create paths add regression surface; the migration is deliberately behaviour-preserving for unrestricted actors. Do not resolve any of this by reverting the fix. |
| Rollback | Revert the task commit. If reverted, the privilege-escalation path reopens, so prefer a forward fix. |

### T-3B0-08 - Verify seed data supports the corrected scope behavior

| Field | Detail |
|---|---|
| Goal | Ensure the local/demo stack can demonstrate the scoped, zero-assignment, and global-super_admin cases. |
| Files / areas | `packages/db/seed/rbac.sql`, `packages/db/seed/dev.sql`, `packages/db/seed/demo.sql` (seed only, no migration) |
| Dependencies | T-3B0-07, T-3B0-04, T-3B0-05, T-3B0-06 (runs after the scope contract and the three notification fixes it feeds) |
| Acceptance criteria | Three seeded identities: (1) at least one admin identity with a facility-scoped `user_roles` row (sees data; plan Section 13.2 CASE C, consumer CASE B); (2) at least one **non-super_admin** identity with zero active facility assignments (empty-scope fail-closed state reachable; plan Section 13.2 CASE D, consumer CASE C); (3) at least one identity with an active global `super_admin` assignment (`user_roles.role_id` = the `super_admin` role, `facility_id IS NULL`, `status = 'active'`, `deleted_at IS NULL`) so the explicit unrestricted path is reachable (plan Section 13.2 CASE B, consumer CASE A); `make db-seed` still refuses to run unless `SIGAP_ENV=local`. |
| Tests | Run `make db-seed` locally and exercise all three identities; covered indirectly by T-3B0-07's matrix tests |
| Risk | Low. Seed-only change. |
| Rollback | Revert the task commit and re-seed. |

### T-3B0-09 - GATE 1 verification

| Field | Detail |
|---|---|
| Goal | Prove 3B0 is complete and safe before any visual work begins. Passing this task (GATE 1) is the release condition for starting 3B1. |
| Files / areas | None (verification task) |
| Dependencies | T-3B0-01 .. T-3B0-08 |
| Acceptance criteria | `GOTEST` green; `GOVET` green; `ROUTERCOV` >= 90%; `RUSTTEST` green; `WEBCHECK` green; `WEBTEST` green; all P0 regression tests present and executing; the super_admin security matrix (plan Section 13.2, cases A-G) present and green; no migration introduced; `git diff --stat packages/db/migrations` is empty. Only after this passes may 3B1 begin. |
| Tests | The commands above |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B1 - Test Tooling Foundation, Shared Primitives, Tokens, Helpers

Phase 3B1 order is fixed and renumbered (Phase 3A.3): the test tooling foundation is **first**, so no
task inside this phase requires a runner that does not exist yet. All internal parallelism in 3B1 is
allowed only after T-3B1-01 has landed.

### T-3B1-01 - Add unit/component/E2E test tooling foundation

| Field | Detail |
|---|---|
| Goal | Establish the complete test tooling foundation - unit, component, **and** the Playwright E2E runner - before any other 3B1 task, so that every later task and every gate can run its tests. Every other T-3B1 task depends on this one. |
| Files / areas | `apps/web/package.json`, `apps/web/vitest.config.ts`, test setup file, `apps/web/playwright.config.ts`, repo E2E script/command, baseline `apps/web/e2e/` structure |
| Dependencies | **T-3B0-09 (GATE 1).** No 3B1 task may start before GATE 1. |
| Acceptance criteria | `WEBTEST` runs Vitest; `@testing-library/svelte` and jsdom configured; `@playwright/test` installed with `playwright.config.ts`; a repo E2E script/command exists (e.g. `pnpm --filter sigap-web e2e`); baseline `apps/web/e2e/` scaffold with at least one smoke scenario proving the runner works; the config targets the local seeded stack only; a **hard safety guard refuses a production target** (any base URL matching the production origin is rejected at config level - the production URL must never be accepted as the E2E target). This task does **not** implement the full case set: 3B2-3B5 add E2E incrementally on this foundation, and 3B6 completes and integrates it. Existing `tests/build-verification.test.js` and `tests/auth-actions.test.js` still run in the same `test` script chain; CI's `web` job remains green. |
| Tests | `WEBTEST` and a single passing E2E smoke scenario against the local seeded stack |
| Risk | Low-medium. The `test` script is a chain; the existing two scripts must keep running. |
| Rollback | Revert the task commit. |

### T-3B1-02 - Add design tokens

| Field | Detail |
|---|---|
| Goal | Encode the frozen design system as the single source of colour, radius, and control height. |
| Files / areas | `apps/web/src/lib/design/tokens.css`, `apps/web/src/lib/design/tokens.ts`, `apps/web/src/app.css`, `apps/web/tailwind.config.ts` |
| Dependencies | T-3B1-01 |
| Acceptance criteria | Values exact per `DESIGN.md`: primary `#0F766E`, hover `#0B6B63`, active `#084F49`, canvas `#F7F6F3`, surface `#FFFFFF`, foreground `#1C1B1A`, muted `#57534E`, border `#E0DDD8`, success `#2F7D32`, warning `#B45309`, danger `#C4322A`, info `#1D6BB5`; radii 6 (control), 8 (panel), 12 (dialog); control heights 44 (citizen) and 36/40 (admin); spacing on an 8px base; the legacy `--accent: #059669` is removed. |
| Tests | A Vitest unit test asserting the token map matches the frozen values exactly |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B1-03 - Build the shared UI primitives

| Field | Detail |
|---|---|
| Goal | Provide the design-system component layer. |
| Files / areas | `apps/web/src/lib/ui/{Button,IconButton,Field,Input,Select,Textarea,Alert,StatusBadge,Dialog,EmptyState,ErrorState,LoadingState,Skeleton,ForbiddenPanel,UnauthPanel,CodeDisplay,Toast}.svelte` |
| Dependencies | T-3B1-02 |
| Acceptance criteria | Every primitive reads semantic tokens only (no literal hex, no `slate-*`/`emerald-*`); `Dialog` uses native `<dialog>` with `showModal()` and implements trap, Escape, return focus, and `aria-labelledby`; `Field` associates label, helper, error, and `aria-invalid`; `StatusBadge` always renders label text plus colour plus optional icon; `Button` has visible `:focus-visible`; all decorative icons are `aria-hidden`. |
| Tests | Component tests via `@testing-library/svelte`: dialog trap/Escape/return-focus; field error association; badge label+colour+icon; button contrast. |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B1-04 - Add the icon dependency

| Field | Detail |
|---|---|
| Goal | One icon family, one stroke width, matching the frozen glyphs. |
| Files / areas | `apps/web/package.json`, an icon wrapper in `lib/ui/` |
| Dependencies | T-3B1-02 |
| Acceptance criteria | `lucide-svelte` installed (the frozen HTML uses Lucide glyph names via `data-lucide`); a single stroke width standardised in one wrapper; no second icon family introduced; no hand-rolled SVG icon paths. |
| Tests | `WEBCHECK`; a Vitest unit test asserting the wrapper applies the standard stroke width |
| Risk | Low. |
| Rollback | Revert the task commit and remove the dependency. |

### T-3B1-05 - Build the API client and error normalisation

| Field | Detail |
|---|---|
| Goal | One place for fetch, headers, abort, and error mapping. |
| Files / areas | `apps/web/src/lib/api/client.ts`, `apps/web/src/lib/api/errors.ts`, `apps/web/src/lib/api/types/api.ts`, `apps/web/src/lib/api/endpoints/{public,citizen,admin}.ts` |
| Dependencies | T-3B1-02 |
| Acceptance criteria | `apiFetch` centralises base URL, JSON handling, and `AbortController`; `errors.ts` returns a discriminated union covering 400/401/403/404/409/429/500 plus network failure and abort; 403 maps to forbidden-or-unauth using session presence; bodies with a trailing newline parse; wire types mirror the Go response structs exactly (Appendix A of the plan). |
| Tests | Vitest unit tests for every status mapping, the 403 branch, abort as a no-op, and trailing-newline parsing |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B1-06 - Build the domain helpers

| Field | Detail |
|---|---|
| Goal | Centralise status maps, joins, formatting, and polling. |
| Files / areas | `apps/web/src/lib/domain/{status,joins,format,polling}.ts` |
| Dependencies | T-3B1-05 |
| Acceptance criteria | `status.ts` encodes exactly the two verified transition maps (plan Sections 4.2/4.3) and label maps for every enum value; `joins.ts` resolves id -> name with `-` for missing/out-of-scope/null ids; `format.ts` fixes the `relativeTime` minutes-vs-hours ordering bug and formats `id-ID`; `polling.ts` exports `createPolling` with interval, overlap guard, `visibilitychange` pause/resume, manual refresh, abort, and cleanup. |
| Tests | Vitest unit tests for transition maps, join fallbacks, `relativeTime` ordering, and polling lifecycle with fake timers |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B1-07 - Add the session store

| Field | Detail |
|---|---|
| Goal | Let the Admin shell distinguish unauthenticated from unauthorized without a role source. |
| Files / areas | `apps/web/src/lib/stores/session.ts`, `apps/web/src/routes/+layout.server.ts`, `apps/web/src/app.d.ts` |
| Dependencies | T-3B1-02 |
| Acceptance criteria | `+layout.server.ts` exposes a boolean `hasSession` (never the token); the store exposes it to the Admin shell; no role or permission data is exposed or inferred client-side (D5). |
| Tests | A Vitest unit test asserting `hasSession` is a boolean and that no token field is present in the layout data |
| Risk | Low. Must not widen the exposed data beyond a boolean. |
| Rollback | Revert the task commit. |

### T-3B1-08 - GATE 2 verification

| Field | Detail |
|---|---|
| Goal | Prove the primitives match the frozen tokens and that the complete test tooling foundation is working before any route consumes them. |
| Files / areas | None (verification task) |
| Dependencies | T-3B1-01 .. T-3B1-07 |
| Acceptance criteria | Shared primitives and tokens: every `DESIGN.md` token present and exact; radii 6/8/12 and control heights correct; one icon family at one stroke width; no gradients, glassmorphism, or decorative shadows; no literal hex outside `tokens.css`. Test tooling foundation: the **Vitest** runner exists and runs; the **Playwright** runner and `playwright.config.ts` exist; the **baseline E2E smoke scenario passes against the local seeded stack**; the **production-target safety guard is explicitly tested and passes**, i.e. the production URL is **rejected** as an E2E target. Commands: `WEBTEST` green; `WEBCHECK` green. This GATE 2 is identical in substance to the implementation plan's GATE 2 (plan Section 17). |
| Tests | `WEBTEST`; `WEBCHECK`; the baseline E2E smoke scenario against the local seeded stack; an explicit guard test asserting the production URL is rejected as an E2E target |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B2 - Citizen Shell and Public Catalog Foundation

### T-3B2-01 - Build the citizen shell

| Field | Detail |
|---|---|
| Goal | Replace the single hybrid header with a citizen shell (mobile bottom nav + desktop nav). |
| Files / areas | `apps/web/src/routes/+layout.svelte`, `lib/citizen/{CitizenHeader,CitizenDesktopNav,CitizenBottomNav,AccountMenu}.svelte` |
| Dependencies | T-3B1-08 |
| Acceptance criteria | Bottom nav has exactly four tabs (Beranda, Faskes, Check-In, Status) and is present at 390px; desktop nav is one line and replaces the bottom nav at 1440px; `aria-current="page"` on exactly one destination; account menu is neutral (anonymous -> Masuk/Daftar; logged in -> email + Keluar) with no role-conditional links; safe-area clearance reserved for the fixed bottom nav; `/wallet` is not linked. |
| Tests | Rendering + responsive assertions at 390px and 1440px; `aria-current` assertion |
| Risk | Medium. This is the first shell change and touches every citizen route. |
| Rollback | Revert the task commit. |

### T-3B2-02 - Replace Beranda

| Field | Detail |
|---|---|
| Goal | Remove the demo stage from the public entry point. |
| Files / areas | `apps/web/src/routes/+page.svelte`, `lib/citizen/QuickActions.svelte` |
| Dependencies | T-3B2-01 |
| Acceptance criteria | No `BedAvailabilityDashboard`; no hardcoded facilities; no fake bed availability; no Chaos Mode; no gamified log; no emoji UI icons; no geolocation mock; no referral map; content is quick actions plus a real catalog summary (or an honest empty state) plus a 3-step "how it works"; catalog failure shows a retry state. |
| Tests | Route rendering; empty-catalog state; catalog-failure retry state; a test asserting no demo strings remain on the page |
| Risk | Medium. `tests/build-verification.test.js` asserts `types.ts` exports; keep those until 3B6. |
| Rollback | Revert the task commit; the old root page returns. |

### T-3B2-03 - Build `/faskes`

| Field | Detail |
|---|---|
| Goal | Ship facility discovery on the approved new route. |
| Files / areas | `apps/web/src/routes/faskes/+page.svelte`, `lib/citizen/{FacilitySearch,FacilityResultRow}.svelte` |
| Dependencies | T-3B2-01 |
| Acceptance criteria | Renders normal, loading (skeleton), empty ("Belum ada data faskes"), no-result (with reset), and error (retry) states; type filter group (Semua / Puskesmas / Rumah Sakit) with `aria-pressed`, matching the frozen `faskes.html`; each row shows name, type, and short code only; no address, distance, opening hours, availability, or bed claims; selecting a facility leads to booking with the facility preselected. |
| Tests | Route rendering for all five states; filter behaviour; a test asserting no unsupported fields are rendered |
| Risk | Low. New route. |
| Rollback | Revert the task commit; `/faskes` becomes 404. |

### T-3B2-04 - Remove `/wallet` from navigation

| Field | Detail |
|---|---|
| Goal | Apply D7 without deleting code. |
| Files / areas | Any nav/route references to `/wallet` |
| Dependencies | T-3B2-01 |
| Acceptance criteria | `/wallet` is unreachable from navigation; the file `apps/web/src/routes/wallet/+page.svelte` still exists and is unmodified. |
| Tests | A test asserting no navigation element links to `/wallet` |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B2-05 - Citizen shell and catalog checkpoint

| Field | Detail |
|---|---|
| Goal | Confirm the shell and catalog foundation before transactional work. This is a **Phase 3B2 checkpoint, not a gate**; the only GATE 3 verification is T-3B3-06. |
| Files / areas | None (verification task) |
| Dependencies | T-3B2-01 .. T-3B2-04 |
| Acceptance criteria | No demo content anywhere in the citizen shell; bottom nav exactly four tabs; `/faskes` fully functional; `/wallet` unreachable; `WEBCHECK`, `WEBTEST` green. |
| Tests | The commands above |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B3 - Citizen Transactional Flows

### T-3B3-01 - Rebuild `/appointments/new`

| Field | Detail |
|---|---|
| Goal | Redesign booking as a three-step flow while preserving the backend contract. |
| Files / areas | `apps/web/src/routes/appointments/new/+page.svelte`, `lib/citizen/{BookingStepper,BookingSummary}.svelte` |
| Dependencies | T-3B2-05 |
| Acceptance criteria | Three steps (Fasilitas -> Layanan -> Waktu + Data Diri) with a visible stepper; service options filtered client-side by the selected `facility_id`; two-column form + summary at desktop, single column at 390px; the `checkin_code` and `result.id` markers that `build-verification.test.js` asserts are preserved; success shows the check-in code with a copy action and a deep link carrying `appointment_id` + `checkin_code`; validation for phone 10-15 digits, future time, and required fields; 429 and 409 render the backend copy verbatim; a generic 500 renders a generic retry state with no duplicate-specific copy (D4). |
| Tests | Form validation tests; error-state tests for 400/429/409/500; route rendering; E2E booking flow |
| Risk | Medium-high. Asserted markers and error copy must not regress. |
| Rollback | Revert the task commit. |

### T-3B3-02 - Rebuild `/appointments/check-in`

| Field | Detail |
|---|---|
| Goal | Redesign appointment check-in with a prominent queue ticket. |
| Files / areas | `apps/web/src/routes/appointments/check-in/+page.svelte`, `lib/citizen/{CheckinForm,QueueTicket}.svelte` |
| Dependencies | T-3B3-01 |
| Acceptance criteria | Deep-link prefill from `appointment_id`/`id` and `checkin_code`/`code` preserved; the `appointment_id`, `queue_ticket_id`, and `formatted_number` markers asserted by `build-verification.test.js` are preserved; success shows a large queue number, `estimated_wait_minutes` with estimate language (never presented as a live calculation), and a next-step CTA to Status; the 401 wrong-code case renders a wrong-code state, **not** an auth state; 404, 409, and 429 render their own states; a cross-link to `/queues/new` is present. |
| Tests | Deep-link prefill test; error-state tests for 401/404/409/429; route rendering; E2E prefilled check-in |
| Risk | Medium-high. The 401-means-wrong-code mapping is easy to get wrong. |
| Rollback | Revert the task commit. |

### T-3B3-03 - Build `/queues/new` (walk-in)

| Field | Detail |
|---|---|
| Goal | Ship the walk-in flow on the canonical route, contractually separate from check-in. |
| Files / areas | `apps/web/src/routes/queues/new/+page.svelte`, `lib/citizen/WalkInForm.svelte` |
| Dependencies | T-3B3-01 |
| Acceptance criteria | Route is `/queues/new`; the page calls `POST /api/v1/queues/generate` and **never** the check-in endpoint; request body is `{facilityId, patient:{fullName, phone}}`; facility options come from the public catalog; validation for required name and 10-15 digit phone; success shows `formatted_number` plus the wait estimate and a Status CTA; 429 renders the walk-in daily-limit copy verbatim; the response reader tolerates both `formatted_number` and `FormattedNumber` (the Go tag gap noted in the plan); a cross-link back to Check-In is present. |
| Tests | Form validation; a test asserting the page never references the check-in path; 429 state; E2E walk-in flow |
| Risk | Medium. Must not merge contracts with check-in (R3-F2). |
| Rollback | Revert the task commit; `/queues/new` becomes 404. |

### T-3B3-04 - Rebuild `/patient/status`

| Field | Detail |
|---|---|
| Goal | Redesign the status lookup with a current-state progress indicator. |
| Files / areas | `apps/web/src/routes/patient/status/+page.svelte`, `lib/citizen/VisitProgress.svelte` |
| Dependencies | T-3B2-05 |
| Acceptance criteria | Single code input; `GET /api/v1/patient/status?code=`; the `checkin_status` and `queue_status` mappings match `mapCheckinStatus` (`patient.go:153-170`); progress renders Check-In -> Antre -> Dilayani -> Selesai as a **current-state indicator**, not a fabricated history, with no invented timestamps (D6); found-by-code-not-found renders the no-result state with guidance; 429 renders the backend copy. |
| Tests | Status mapping test; no-result state; 429 state; route rendering |
| Risk | Medium. Mapping drift would misreport a citizen's position. |
| Rollback | Revert the task commit. |

### T-3B3-05 - Restyle the auth pages

| Field | Detail |
|---|---|
| Goal | Apply the design system to login/register/logout presentation only. |
| Files / areas | `apps/web/src/routes/auth/{login,register,logout}/+page.svelte`, `lib/citizen/AuthForms.svelte` |
| Dependencies | T-3B2-05 |
| Acceptance criteria | `+page.server.ts` files are **untouched**; field names `email`, `password`, `confirm` unchanged; the `?registered=1` banner preserved; `tests/auth-actions.test.js` passes unchanged; validation, 401, and 503 states render correctly. |
| Tests | `WEBTEST` (must include the untouched auth-actions suite); error-state rendering |
| Risk | **High if the actions are touched.** Presentation-only. |
| Rollback | Revert the task commit. |

### T-3B3-06 - GATE 3 verification

| Field | Detail |
|---|---|
| Goal | Prove the citizen experience is complete. |
| Files / areas | None (verification task) |
| Dependencies | T-3B3-01 .. T-3B3-05 |
| Acceptance criteria | Citizen critical-path E2E green; all five transactional flows render every required state; no demo content; bottom nav exactly four tabs; `tests/auth-actions.test.js` unchanged and passing; `WEBCHECK`, `WEBTEST` green. |
| Tests | `E2E`, `WEBCHECK`, `WEBTEST` |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B4 - Admin Shell and Read-Only Operational Pages

### T-3B4-01 - Build the admin shell

| Field | Detail |
|---|---|
| Goal | Give the Admin product its own shell, separate from Citizen. |
| Files / areas | `lib/admin/{AdminShell,AdminSidebar,AdminPageHeader,AdminToolbar}.svelte`, layout wiring |
| Dependencies | **T-3B3-06 (citizen stage complete, GATE 3 passed), T-3B1-08.** Strict post-GATE-2 order: the admin stage starts only after the citizen stage is proven; it does not run alongside 3B2/3B3. |
| Acceptance criteria | Six destinations (Ringkasan, Antrean, Janji Temu, Jadwal, Fasilitas, Notifikasi) with `aria-current="page"` on the active one; full sidebar at 1440px and a 56px icon rail at ~1024px with accessible names on the rail items; the account area links to Beranda as a static link with no role detection; no admin mobile layout is created; `UnauthPanel` is chosen from session presence, not status code. |
| Tests | `aria-current` assertion; responsive assertions at 1440px and ~1024px; accessible-name assertion on the collapsed rail |
| Risk | Medium. Shell change touching all six admin routes. |
| Rollback | Revert the task commit. |

### T-3B4-02 - Build the admin table layer

| Field | Detail |
|---|---|
| Goal | Provide the dense, scan-first table pattern. |
| Files / areas | `lib/ui/DataTable.svelte`, `lib/admin/{AdminTable,AdminResponsiveTablePattern,FacilityFilter}.svelte` |
| Dependencies | T-3B4-01 |
| Acceptance criteria | Semantic table with `caption` where useful and `scope` on column headers; admin density (36-40px rows, 11px uppercase headers); lower-priority columns hidden below 1280px while status and actions remain; horizontal overflow scoped to the table region; `FacilityFilter` is a client-side narrowing control over the loaded dataset, labelled "Menampilkan: X dari Y" (never server-filter language). |
| Tests | Table semantics assertion; column-hiding assertion; filter label assertion |
| Risk | Medium. Accessibility of tables is easy to regress. |
| Rollback | Revert the task commit. |

### T-3B4-03 - Build `/admin` Ringkasan

| Field | Detail |
|---|---|
| Goal | Ship the derived-counts overview. |
| Files / areas | `apps/web/src/routes/admin/+page.svelte` |
| Dependencies | T-3B4-02 |
| Acceptance criteria | Counts are derived only from already-loaded scoped endpoints; each figure is labelled with its source and a "Diperbarui pukul HH:MM" timestamp; cross-links reach the correct modules; no invented KPI, chart, or notification fallback; the empty-scope case renders as a class-1 state ("Anda belum memiliki fasilitas dalam cakupan"). |
| Tests | Rendering test; derived-count test against mocked scoped data; empty-scope state test; a test asserting no fabricated metric strings |
| Risk | Medium. Deriving counts client-side invites accidental global sourcing; must use only scoped lists. |
| Rollback | Revert the task commit; `/admin` becomes 404. |

### T-3B4-04 - Rebuild `/admin/queues` read path with polling

| Field | Detail |
|---|---|
| Goal | Ship the queue board with 30s polling and correct transitions offered. |
| Files / areas | `apps/web/src/routes/admin/queues/+page.svelte`, `lib/admin/{QueueBoard,QueueBoardRow}.svelte` |
| Dependencies | T-3B4-02 |
| Acceptance criteria | Board groups Dipanggil / Menunggu / Selesai hari ini; 30s polling with pause on `visibilitychange`, immediate refresh on becoming visible, overlap guard, manual refresh, and abort on destroy; "Diperbarui pukul HH:MM" always present, including in the stale state; refresh failure keeps the last rows and shows a stale warning instead of blanking the table; first-load failure shows an ErrorState; the raw-UUID filter input is gone, replaced by `FacilityFilter`; the `updated_at` marker asserted by `build-verification.test.js` is preserved; the word "realtime" appears nowhere. |
| Tests | Polling lifecycle tests; stale-state test; error-state test; rendering test; `updated_at` marker assertion |
| Risk | High. This page's mutation path was broken end to end before 3B0; polling adds lifecycle complexity. |
| Rollback | Revert the task commit. |

### T-3B4-05 - Rebuild `/admin/appointments` read path

| Field | Detail |
|---|---|
| Goal | Ship the appointment table with correct action affordances. |
| Files / areas | `apps/web/src/routes/admin/appointments/+page.svelte`, `lib/admin/AppointmentRow.svelte` |
| Dependencies | T-3B4-02 |
| Acceptance criteria | Table renders facility name and service name via joins (or `-`); filters are client-side narrowing controls; per-row actions offer **only** the verified transitions (plan Section 4.2) and no longer offer `checked_in -> completed`; the `updated_at` marker asserted by `build-verification.test.js` is preserved; `practitioner_id` is never displayed. |
| Tests | Transition-offer test against the verified map; join fallback test; rendering test |
| Risk | High. Correcting the button set changes observable behavior, which is the intent. |
| Rollback | Revert the task commit. |

### T-3B4-06 - Rebuild `/admin/schedules` read path

| Field | Detail |
|---|---|
| Goal | Ship the schedule table with scoped selects and a permission-aware read view. |
| Files / areas | `apps/web/src/routes/admin/schedules/+page.svelte` |
| Dependencies | T-3B4-02 |
| Acceptance criteria | Facility and service names resolve via joins or show `-`; raw UUID inputs are gone from the read view; when `schedule.manage` is absent the mutation controls are hidden or blocked with the ForbiddenPanel; `practitioner_id` is never displayed, never fabricated as a name, and never exposed as a raw-UUID or free-text field. |
| Tests | Join fallback test; forbidden-state test; rendering test |
| Risk | Medium-high. Dev identity lacks `schedule.manage`, so this path is exercised in every local run. |
| Rollback | Revert the task commit. |

### T-3B4-07 - Rebuild `/admin/facilities` read path

| Field | Detail |
|---|---|
| Goal | Ship the facility table with correct rendering of the inactive state. |
| Files / areas | `apps/web/src/routes/admin/facilities/+page.svelte` |
| Dependencies | T-3B4-02 |
| Acceptance criteria | Table renders facility fields; inactive facilities are clearly marked with label plus colour, not colour alone; the one-way deactivate semantics are stated in the UI (a deactivated facility cannot be reactivated); the string `"false"` response shape is handled without boolean coercion. |
| Tests | Rendering test; inactive-state assertion |
| Risk | Medium. The `is_active` string/bool inconsistency is a real trap. |
| Rollback | Revert the task commit. |

### T-3B4-08 - Rebuild `/admin/notifications` read path

| Field | Detail |
|---|---|
| Goal | Preserve the best existing page while fixing its defects. |
| Files / areas | `apps/web/src/routes/admin/notifications/+page.svelte`, `lib/admin/{NotificationRow,OutboxSummary}.svelte` |
| Dependencies | T-3B4-02, T-3B0-06 |
| Acceptance criteria | Separate list and summary skeletons and separate error states preserved; URL-synced shareable filters preserved; masked recipient preserved and the raw contact or hash never rendered; `limit` handling respects the verified default 100 / max 500 with **no** server-pagination controls (R3-F1); the `relativeTime` minutes-vs-hours ordering bug is fixed; summary cards reflect the corrected (non-leaking) counts. |
| Tests | Masking test; relativeTime ordering test; no-pagination-control assertion; separate-error-state test; rendering test |
| Risk | Medium. The page depends on three P0-corrected endpoints. |
| Rollback | Revert the task commit. |

### T-3B4-09 - GATE 4 verification

| Field | Detail |
|---|---|
| Goal | Prove all Admin read flows are correct. |
| Files / areas | None (verification task) |
| Dependencies | T-3B4-01 .. T-3B4-08 |
| Acceptance criteria | Six destinations reachable with `aria-current`; Ringkasan derived-only; polling with pause-on-hidden and last-updated label; joins resolve or show `-`; empty-scope class-1 state everywhere; no admin rate-limit state; icon rail accessible at ~1024px; `WEBCHECK`, `WEBTEST`, `E2E` (read flows) green. |
| Tests | The commands above |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B5 - Admin Mutations

### T-3B5-01 - Wire queue status mutations

| Field | Detail |
|---|---|
| Goal | Make queue transitions work against the corrected proxy and contract. |
| Files / areas | `apps/web/src/routes/admin/queues/+page.svelte`, `lib/admin/QueueBoardRow.svelte`, `lib/ui/Toast.svelte` |
| Dependencies | T-3B4-04 |
| Acceptance criteria | Actions offer exactly the verified queue map (plan Section 4.3); an invalid transition surfaces the 400 message verbatim; success updates the row inline and announces via the polite live region; a toast appears for ~4s; no raw UUID panel is shown. |
| Tests | Transition-offer test; invalid-transition message test; live-region assertion; E2E queue action |
| Risk | Medium-high. |
| Rollback | Revert the task commit. |

### T-3B5-02 - Wire appointment status mutations

| Field | Detail |
|---|---|
| Goal | Make appointment transitions work against the corrected proxy and contract. |
| Files / areas | `apps/web/src/routes/admin/appointments/+page.svelte`, `lib/admin/AppointmentRow.svelte` |
| Dependencies | T-3B4-05 |
| Acceptance criteria | Actions offer exactly the verified appointment map (plan Section 4.2); the confirmation step is required only for `cancelled`; an invalid transition surfaces the 400 message verbatim and is **not** presented as a conflict; success updates inline and announces. |
| Tests | Transition-offer test; confirmation-scope test; invalid-transition message test; E2E appointment action |
| Risk | Medium-high. |
| Rollback | Revert the task commit. |

### T-3B5-03 - Build the dialog-based schedule editor

| Field | Detail |
|---|---|
| Goal | Replace the schedule modal with the accessible dialog and scoped selects. |
| Files / areas | `lib/admin/ScheduleEditor.svelte` |
| Dependencies | T-3B4-06 |
| Acceptance criteria | Facility and service come from scoped lists, not raw UUID inputs. **Practitioner (Phase 3A.1 rule):** no practitioner field of any kind in the editor. No fabricated names, no raw-UUID input, no free-text field pretending to represent `practitioner_id`, no fake practitioner catalog. Verified backend semantics: CREATE omits `practitioner_id` (the column is nullable, `0005_appointments.sql`; `CreateScheduleRequest.PractitionerID` is `omitempty`, so absence is legal). UPDATE sends a body that **omits** `practitioner_id` (`UpdateScheduleRequest` fields are pointers with `omitempty`, so an omitted field is never re-sent or cleared - the existing value is preserved internally and is not displayed or edited). Validation covers slot 5-180 minutes dividing the range, capacity 1-100, and end > start; create and update both work through the corrected proxy; a missing `schedule.manage` renders the ForbiddenPanel rather than a red error; the dialog satisfies trap, Escape, return focus, and dialog semantics. |
| Tests | Validation tests; forbidden-state test; dialog accessibility test; E2E schedule create/update |
| Risk | High. Combines a broken proxy, a permission gap, and dialog accessibility. |
| Rollback | Revert the task commit. |

### T-3B5-04 - Build the dialog-based facility editor and deactivate confirmation

| Field | Detail |
|---|---|
| Goal | Replace the facility modal and native `confirm()` with accessible dialogs. |
| Files / areas | `lib/admin/FacilityEditor.svelte`, `apps/web/src/routes/admin/facilities/+page.svelte` |
| Dependencies | T-3B4-07 |
| Acceptance criteria | Create and update work through the corrected proxy; deactivate is dialog-confirmed, calls `PATCH`, and is one-way; the dialog states the public-catalog impact ("faskes ini tidak lagi muncul untuk warga"); the string `"false"` response is handled; focus returns to the trigger after both confirm and cancel; validation covers required fields and the phone character rules. |
| Tests | Validation tests; deactivate method test; focus-return test; E2E facility deactivate |
| Risk | High. This is the flow that was completely broken before 3B0. |
| Rollback | Revert the task commit. |

### T-3B5-05 - Wire notification retry/cancel with permission awareness

| Field | Detail |
|---|---|
| Goal | Expose retry/cancel only where permitted, against the corrected endpoints. |
| Files / areas | `apps/web/src/routes/admin/notifications/+page.svelte`, `lib/admin/NotificationRow.svelte` |
| Dependencies | T-3B4-08, T-3B0-04, T-3B0-05 |
| Acceptance criteria | Retry/cancel are hidden when `notification.manage` is absent; an invalid-state action surfaces the 409 message verbatim; success reloads the row and announces; the out-of-scope path is not reachable from the UI (the corrected backend returns 404 regardless); the masked recipient is never replaced by an unmasked value after an action. |
| Tests | Permission-gating test; 409 message test; masking-after-action test; E2E notification actions |
| Risk | Medium. |
| Rollback | Revert the task commit. |

### T-3B5-06 - GATE 5 verification

| Field | Detail |
|---|---|
| Goal | Prove all Admin mutations are correct and accessible. |
| Files / areas | None (verification task) |
| Dependencies | T-3B5-01 .. T-3B5-05 |
| Acceptance criteria | Appointment and queue actions offer exactly the verified transitions; invalid transitions surface the 400 message; deactivate is dialog-confirmed and one-way; `schedule.manage` absence shows ForbiddenPanel; dialogs satisfy trap/Escape/return-focus; the live region announces mutations; `WEBCHECK`, `WEBTEST`, `E2E` (mutation flows) green. |
| Tests | The commands above |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B6 - Cross-Product Integration and Automated E2E

### T-3B6-01 - Complete full E2E coverage and CI integration

| Field | Detail |
|---|---|
| Goal | Complete and consolidate the E2E coverage on the Playwright foundation established by T-3B1-01. This task does **not** introduce Playwright for the first time; it fills remaining cross-product scenarios, consolidates the complete suite, and wires it into CI (or a documented manual trigger). |
| Files / areas | `apps/web/e2e/**` (case set + consolidation), `package.json` scripts, CI workflow (the Playwright config itself originates from T-3B1-01) |
| Dependencies | T-3B3-06, T-3B5-06 |
| Acceptance criteria | E2E covers the citizen critical path, booking, prefilled check-in, walk-in, status lookup, auth, admin queue actions, admin appointment actions, schedule management, facility management, and notification actions; every E2E scenario that 3B2-3B5 promised (as part of their test lines) exists and is green; the suite is consolidated into one entry point; it runs against a local seeded stack and never against production (the T-3B1-01 guard is intact); CI gains an E2E job, or a documented manual trigger if runtime cost is prohibitive. |
| Tests | `E2E` |
| Risk | Medium. New CI surface. |
| Rollback | Revert the task commit; CI returns to the previous jobs. |

### T-3B6-02 - Remove the legacy demo components

| Field | Detail |
|---|---|
| Goal | Delete the demo stage now that real surfaces replace it. |
| Files / areas | `apps/web/src/lib/components/dashboard/BedAvailabilityDashboard.svelte`, `apps/web/src/lib/components/ReferralMap.svelte`, `maplibre-gl` in `package.json`, `apps/web/src/lib/types.ts` |
| Dependencies | T-3B6-01 |
| Acceptance criteria | No import of either component remains; no `maplibre-gl` import remains; the dependency is removed and the lockfile updated; `lib/types.ts` is reconciled into `lib/api/types/api.ts` plus domain types; `tests/build-verification.test.js` is updated to reflect the new type locations (its assertions about the removed exports must be revised deliberately, not deleted silently); `WEBCHECK`, `WEBTEST`, `E2E` green. |
| Tests | `WEBCHECK`, `WEBTEST`, `E2E` |
| Risk | Medium. `build-verification.test.js` asserts `types.ts` exports; changing it is a deliberate test update. |
| Rollback | Revert the task commit; deleted files return from git history. |

### T-3B6-03 - Cross-product consistency pass

| Field | Detail |
|---|---|
| Goal | Verify one brand, two densities. |
| Files / areas | All replaced routes and components |
| Dependencies | T-3B6-02 |
| Acceptance criteria | Citizen and Admin share the exact token set; Citizen controls >= 44px and Admin 36-40px; one accent used identically everywhere; one radius system; no gradients, glassmorphism, emoji UI icons, or decorative illustrations; no orphaned routes; no dead links; no raw UUID in a name position. |
| Tests | A token-usage assertion test; manual visual sweep recorded in the PR |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B6-04 - GATE 6 verification

| Field | Detail |
|---|---|
| Goal | Prove full regression and production readiness. |
| Files / areas | None (verification task) |
| Dependencies | T-3B6-01 .. T-3B6-03 |
| Acceptance criteria | Full E2E suite green; `GOTEST` green with the router coverage gate; `RUSTTEST` green; `WEBCHECK`, `WEBTEST` green; legacy components removed; no orphaned routes; rollback points documented per phase; `SIGAP_DEV_IDENTITY` confirmed off in production config; `git diff --stat packages/db/migrations` empty. |
| Tests | The commands above |
| Risk | Low. |
| Rollback | n/a |

---

## Phase 3B7 - Production Migration and Deployment Readiness

### T-3B7-01 - Document the deployment and rollback procedure

| Field | Detail |
|---|---|
| Goal | Make the cutover repeatable and reversible. |
| Files / areas | `docs/operations/**`, deployment runbook |
| Dependencies | T-3B6-04 |
| Acceptance criteria | Per-phase deploy order documented (plan Section 15.6); per-phase rollback point documented (plan Section 18.2); environment variables enumerated (`SIGAP_API_INTERNAL`, `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY`, `SIGAP_DEV_IDENTITY`, `SIGAP_ENV`); the statement that no migration exists and therefore no DB restore is ever required. |
| Tests | Documentation review |
| Risk | Low. |
| Rollback | n/a |

### T-3B7-02 - Extend the smoke checks

| Field | Detail |
|---|---|
| Goal | Provide post-deploy verification aligned with the existing smoke scripts. |
| Files / areas | `scripts/smoke/**` |
| Dependencies | T-3B7-01 |
| Acceptance criteria | Smoke checks cover the plan's Section 15.8 list, including the two P0 proofs: an out-of-scope notification retry returns 404 **and** leaves the row unchanged, and a **zero-assignment non-super_admin** actor's notification summary is all-zero (while a DB-resolved active global `super_admin` still sees global counts, per plan Section 4.5); new routes (`/`, `/faskes`, `/queues/new`, `/admin`) are checked for 200. |
| Tests | Run the smoke script against a production-equivalent stack |
| Risk | Low. |
| Rollback | Revert the task commit. |

### T-3B7-03 - Confirm production configuration safety

| Field | Detail |
|---|---|
| Goal | Ensure no development affordance reaches production. |
| Files / areas | Production environment configuration (not the repo) |
| Dependencies | T-3B7-02 |
| Acceptance criteria | `SIGAP_DEV_IDENTITY` is unset or `false` in production; `SIGAP_ENV` is not `local`; the dev-header path is inactive; `make db-seed` would refuse to run. |
| Tests | Configuration inspection recorded in the release checklist |
| Risk | Medium if missed: a dev identity in production grants a synthetic permission set. |
| Rollback | n/a (verification task) |

---

## Task dependency summary

| Phase | Tasks | May start only when |
|---|---|---|
| 3B0 | T-3B0-01 .. T-3B0-09 | GATE 0 passed. Within the backend security chain: **T-3B0-07 first**, then T-3B0-04/05/06, then T-3B0-08, then T-3B0-09; the proxy repairs T-3B0-02/03 are independent of that chain |
| 3B1 | T-3B1-01 .. T-3B1-08 | **GATE 1 passed (3B0 complete) - mandatory; every 3B1 task is downstream of GATE 1, directly or transitively; T-3B1-01 (test tooling foundation) precedes every test-dependent 3B1 task** |
| 3B2 | T-3B2-01 .. T-3B2-05 | GATE 2 passed (3B1 complete); T-3B2-05 is an internal checkpoint, not a gate |
| 3B3 | T-3B3-01 .. T-3B3-06 | 3B2 complete (including the T-3B2-05 checkpoint); T-3B3-06 is the ONLY GATE 3 verification |
| 3B4 | T-3B4-01 .. T-3B4-09 | **GATE 3 passed (citizen stage 3B2/3B3 complete)** |
| 3B5 | T-3B5-01 .. T-3B5-06 | GATE 4 passed (3B4 complete) |
| 3B6 | T-3B6-01 .. T-3B6-04 | GATE 5 passed (3B5 complete) |
| 3B7 | T-3B7-01 .. T-3B7-03 | GATE 6 passed (full regression and production readiness) |

**STRICT canonical order (Phase 3A.2): GATE 0 -> 3B0 -> GATE 1 -> 3B1 -> GATE 2 -> 3B2 -> 3B3 -> GATE 3 ->
3B4 -> GATE 4 -> 3B5 -> GATE 5 -> 3B6 -> GATE 6 -> 3B7.** One reviewable implementation stream: each
stage proves the shared-component foundation and the previous stage before the next begins. 3B1 MUST NOT
begin before GATE 1: the redesign builds on the corrected authorization/proxy contract rather than
developing against knowingly broken security behavior. 3B4 MUST NOT start before GATE 3: the citizen stage
demonstrates the shared component foundation before the admin conversion. No stage runs alongside another.

## Standing constraints for every task

1. No existing URL is renamed.
2. No database migration is created. `git diff --stat packages/db/migrations` must stay empty.
3. No production deployment is performed as part of a task.
4. No new dependency is added without a documented reason (Vitest, `@testing-library/svelte`, jsdom, and Playwright in T-3B1-01; Lucide in T-3B1-04 are the only anticipated additions; T-3B6-01 does not add the Playwright runner - it is established by T-3B1-01).
5. The frozen visuals are not reinterpreted. Any necessary deviation is documented before implementation.
6. The security mechanisms in the current build are preserved: Supabase SSR cookies, Bearer forwarding, `SIGAP_DEV_IDENTITY` plus dev header, SvelteKit action CSRF protection, DB-resolved RBAC with token claims ignored, security headers in hooks, and notification recipient masking.
7. No client-side role or scope inference (Phase 3A.1). The browser must never infer `email == admin`, `role == super_admin`, or empty scope == global. Unrestricted facility scope is server-side, DB-resolved authorization state only (plan Section 4.5).
8. Practitioner is out of the schedule UX entirely (Phase 3A.1): no raw UUID, no free-text field, no invented names, no fake catalog (T-3B5-03).

## Execution status

**PHASE 3B0.2 IMPLEMENTED. GATE 1 RE-VERIFIED WITH READ AND MUTATION PROVENANCE.**

T-3B0-01 through T-3B0-09 are implemented on branch `design/ui-ux-overhaul` and verified with a fresh
GATE 1 run from HEAD `34a3e56`. Phase 3A.4 documentation decisions above are unchanged.

Implemented and verified:

- T-3B0-01 approved design/plan artifacts under version control; `design/generated/**` and
  `design/tmp/**` remain ignored; no generated HTML export staged.
- T-3B0-02 all 23 API proxies audited; 9 previously broken dynamic-ID proxies repaired to
  interpolate `encodeURIComponent(event.params.id)`; no literal `/ID` or `/STATUS` remains.
- T-3B0-03 facility deactivation is PATCH end to end (UI, SvelteKit proxy, Go route, backend
  target `PATCH /api/v1/admin/facilities/{id}/deactivate`); one-way soft deactivation preserved.
- T-3B0-04/05/06 notification retry and cancel authorize facility scope BEFORE any write;
  summary carries explicit facility provenance with all five keys always present.
- T-3B0-07 centralized `FacilityScopeResult` (IDs / Unrestricted / Err) is the single scope
  decision consumed by every admin and notification handler.
- T-3B0-08 local-only seed identities for the active global super_admin, facility-scoped admin,
  and zero-assignment non-super_admin cases; `SIGAP_ENV=local` guard preserved.
- T-3B0-09 GATE 1: Go tests, `go vet`, router coverage, Rust test, web check, and web test all green.

Phase 3B0.1 (facility-aware RBAC closure) additionally resolved a confirmed cross-facility
privilege escalation: `user_roles` is keyed `(user_id, role_id)`, so one user may hold different
roles at different facilities, and the former flat permission union made a permission granted at
facility B authorize mutations at facility A. Authorization now resolves permission provenance per
facility (`identity.FacilityGrant`, `auth.AuthorizeFacilityMutation`), with the stored row as the
anchor and supplied `facility_id` authorized at its own target. No database migration was created.

Phase 3B0.2 (facility-aware READ authorization closure) extends the same provenance rule to every
facility-scoped read. The previous residual "read paths intentionally remain scope-based only" is
no longer true and has been closed. Every facility-scoped list, detail, and summary now requires
BOTH (a) the target facility inside `FacilityScopeResult` AND (b) the route's required permission
granted AT that facility, through the same DB-resolved `identity.FacilityGrant` provenance the
mutation path uses since 3B0.1.

- Read authorization is centralized in `auth.AuthorizeFacilityRead` (single known facility) and
  `auth.AuthorizedFacilityIDsForPermission` (list intersection), so handlers never re-derive the
  scope/provenance rule. `AdminHandler.beginFacilityRead` and `AdminHandler.listFacilityReadSet`
  are the only two entry points.
- List endpoints filter on the INTERSECTION of scope and permission provenance, never on
  `scope.IDs` alone. That union was the read-side twin of the 3B0.1 mutation defect: an actor
  scoped to facilities A and B holding `notification.read` only at B previously received A's rows.
  Zero authorized facilities render as HTTP 200 with an empty list so row existence never leaks.
- Detail endpoints authorize the STORED resource facility and return 404 when unauthorized or
  out of provenance. Phase one runs BEFORE any resource query, which preserves the fail-closed
  contract that an actor with no authorized facility never reaches the row at all and keeps the
  endpoint from becoming an existence oracle.
- Permission provenance is resolved LIVE per request (`auth.ActorWithLiveGrants`,
  `auth.FacilityGrantResolver`, `rbacResolver.ResolveByAppUserID`) so a grant or revocation takes
  effect on the next request rather than at the next re-authentication. Facility scope was already
  live; provenance is now equally live.
- `notification.read` governs `ListNotifications`, `GetNotification`, and
  `GetNotificationSummary`. The summary aggregates only authorized facilities, post-fetch row
  filtering uses the authorized read set rather than raw scope, a global `super_admin` remains
  global, and a zero-assignment non-super_admin gets the all-zero summary.
- The route-level `RequirePermission` check is retained unchanged as the COARSE first gate ("does
  this actor hold the key anywhere?"). It is a necessary but never sufficient condition for a
  facility-scoped resource; existing 403 semantics are preserved while resource-level
  authorization returns 404.
- Regression matrix R1-R9 is DB-backed and uses only permission keys present in
  `packages/db/seed/rbac.sql`. Repository-wide, flat `HasPermission(...)` now has exactly one
  production call site: the coarse route gate in `internal/identity/authz.go`. Zero
  facility-resource decisions rely on it. No database migration was created.

Open residuals carried into Phase 3B1 planning (recorded, not fixed here):

1. Collection proxies that do not forward `event.url.search`, so UI filters are dropped:
   `admin/appointments`, `admin/facilities`, `admin/notifications`, `admin/notifications/summary`,
   `admin/queues`, `admin/schedules`, `admin/service-units` (plus the dynamic `[id]` variants and
   the public/patient read proxies). Classified P1 FUNCTIONAL; no authorization consequence,
   because facility filtering is enforced server-side rather than by query-string filters.
2. CI does not block on `go vet ./...` and does not run `pnpm --filter sigap-web test` (which
   contains the proxy-contract suite). The web `test` script exists but is not invoked by any CI
   job. Classified P1 PROCESS; the later task should add both as blocking steps.
