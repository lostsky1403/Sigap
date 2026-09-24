# SIGAP Redesign Implementation Handoff

Status: Phase 2D complete. Citizen UI, Admin UI, and the shared design system are frozen.

Phase 3A addendum: the implementation-critical contracts in Sections 3, 5, 6, and 10 were re-verified
against current production source. Where the handoff conflicted with source, source won and this document
was corrected. Corrections are marked "Phase 3A correction" inline. No frozen visual or product decision
was changed. The full verification record is in `design/sigap-redesign-implementation-plan.md`.

Phase 3A.1 addendum: GATE 0 was closed with the owner decision on global `super_admin` facility scope
(explicit, DB-resolved, unrestricted - plan Section 4.5), the practitioner rule for the schedule UX, and
the frozen-token correction in `design/SKILLS.md`. Sections 5 and 10 were updated accordingly. No frozen
visual or product decision was changed.

This document is the implementation contract for SvelteKit work. It does not authorize production-code changes during the design phase and does not replace backend or security remediation work.

## 1. Freeze Status

- CITIZEN UI - FROZEN
- ADMIN UI - FROZEN
- DESIGN SYSTEM - FROZEN
- PHASE 2D - COMPLETE
- READY FOR IMPLEMENTATION PLANNING

No frozen HTML screen was changed during this Phase 2D audit. The only Phase 2D deliverable is this handoff document.

## 2. Design System Tokens

### Color

| Token | Value | Use |
|---|---|---|
| Primary | `#0F766E` | primary actions, links, active navigation |
| Primary hover | `#0B6B63` | hover state |
| Primary active | `#084F49` | pressed state |
| Canvas | `#F7F6F3` | page background |
| Surface | `#FFFFFF` | panels, dialogs, functional surfaces |
| Foreground | `#1C1B1A` | headings and body text |
| Muted | `#57534E` | secondary text and metadata |
| Border | `#E0DDD8` | rules, inputs, table separators |
| Success | `#2F7D32` | successful state |
| Warning | `#B45309` | attention state |
| Danger | `#C4322A` | destructive and failed state |
| Info | `#1D6BB5` | informational state |

Use semantic tokens rather than page-local colors. Status must communicate through label, color, and where useful an icon. Do not rely on color alone.

### Type, shape, and density

- Typeface: Inter with system fallbacks.
- Spacing base: 8px.
- Citizen controls: approximately 44px minimum target height.
- Admin operational controls: approximately 36-40px.
- Control radius: 6px.
- Panel radius: 8px.
- Dialog radius: maximum 12px.
- Border-first hierarchy. Use shadows only for semantic elevation.
- No gradients, glassmorphism, decorative blobs, oversized pills, fake charts, or decorative illustrations.
- Citizen is spacious and task-oriented. Admin is dense, scan-first, and desktop-first.

## 3. Information Architecture and Route Map

Labels are Indonesian UI labels. Preserve production URLs.

### Citizen

| Design surface | Production route | Notes |
|---|---|---|
| Beranda | `/` | Public entry point. Show only supported catalog and task actions. |
| Cari Faskes | `/faskes` | Approved route for facility discovery. Requires implementation route/page. |
| Buat Janji Temu | `/appointments/new` | Preserve existing route. Three-step flow. |
| Check-In Janji Temu | `/appointments/check-in` | Requires `appointment_id` and `checkin_code`. |
| Ambil Antrean Tanpa Janji | `/queues/new` | **Phase 3A correction.** Previously left undecided. Canonical route is `/queues/new`; see Section 3.1. Keep visually and contractually separate from appointment check-in. |
| Status Kunjungan | `/patient/status` | Lookup by check-in code or queue number according to the existing API contract. |
| Masuk | `/auth/login` | Preserve existing route. |
| Daftar | `/auth/register` | Preserve existing route. |
| Keluar | `/auth/logout` | Preserve existing route. |

Citizen mobile bottom navigation contains four destinations: Beranda, Faskes, Check-In, and Status. Buat Janji Temu remains a prominent action, not a permanent tab. Ambil Antrean Tanpa Janji is reached from the Check-In page and from Beranda quick actions; it is also not a permanent tab.

### 3.1 Walk-in route decision (Phase 3A correction)

```
WALK_IN_ROUTE = /queues/new
```

Rationale, verified against source and the frozen design:

- The frozen design already treats walk-in as a separate page. `check-in.html:251` (desktop) and
  `check-in.html:218` (mobile) link to `antrean.html` with "Datang tanpa janji? Ambil antrean tanpa janji.",
  and that page's title is "Ambil Antrean Tanpa Janji".
- The two flows have different backend contracts and must not be merged: check-in is
  `POST /api/v1/appointments/{id}/check-in` with `appointment_id` + `checkin_code`; walk-in is
  `POST /api/v1/queues/generate` with `fullName` + `phone`.
- `/queues/new` follows the existing production convention for citizen transactional routes
  (`/appointments/new` creates an appointment; `/queues/new` creates a queue ticket).
- No existing URL is renamed. `/appointments/check-in` is not overloaded.
- The route is not chosen from the prototype filename (`antrean.html`). Alternatives considered and rejected
  are recorded in `design/sigap-redesign-implementation-plan.md` Section 3.2.

### Admin

| Design surface | Production route |
|---|---|
| Ringkasan | `/admin` approved new overview route |
| Antrean | `/admin/queues` |
| Janji Temu | `/admin/appointments` |
| Jadwal | `/admin/schedules` |
| Fasilitas | `/admin/facilities` |
| Notifikasi | `/admin/notifications` |

Admin navigation must preserve the six destinations, active destination identification, accessible icon-rail labels at approximately 1024px, and cross-links from Ringkasan to the correct module.

## 4. Shared Component Inventory

### Shared

- `Button`: primary, secondary, ghost, text, danger; disabled and focus-visible states.
- `Field`: label-above-control wrapper with helper and field-level error.
- `Input`: text, date, time, readonly, invalid.
- `Select`: empty, selected, disabled, invalid.
- `Textarea`: label, helper, invalid.
- `Alert`: server error, authorization, informational, success.
- `StatusBadge`: semantic label plus optional icon; never color-only.
- `Dialog`: labelled, modal, focus trap, Escape close, focus return, keyboard-safe actions.
- `EmptyState`: title, reason, and appropriate next action.
- `ErrorState`: clear cause where known, retry action where safe.
- `LoadingState` / `Skeleton`: preserve final layout shape.
- `ForbiddenPanel` and `UnauthPanel`: separate authentication from authorization.

### Citizen

- `CitizenHeader`
- `CitizenDesktopNav`
- `CitizenBottomNav`
- `QuickActions`
- `FacilitySearch`
- `FacilityResultRow`
- `BookingStepper`
- `BookingSummary`
- `CheckinForm`
- `WalkInForm`
- `QueueTicket`
- `VisitProgress`
- `AuthForms`
- `AccountMenu`

### Admin

- `AdminShell`
- `AdminSidebar`
- `AdminPageHeader`
- `AdminToolbar`
- `FacilityFilter`
- `AdminTable`
- `QueueBoard`
- `QueueBoardRow`
- `AppointmentRow`
- `ScheduleEditor`
- `FacilityEditor`
- `NotificationRow`
- `OutboxSummary`
- `AdminResponsiveTablePattern`

Do not create separate component families for every state. Prefer shared primitives with explicit variants. Do not expose raw UUID inputs where the approved design uses a scoped list or human-readable value.

## 5. Data and Field Mapping

### Citizen

| Design field | API/backend field | Transformation and display |
|---|---|---|
| Facility options | public facilities catalog | Display only supported catalog fields: name, type, short code, active state where provided. No address, distance, opening hours, bed availability, or slot claim unless the endpoint supplies it. |
| Service options | public service-unit catalog | Filter by selected `facility_id`; display returned `name`. Never derive a service name from type, prefix, or code. |
| Booking reference | `response.id` | Display as ID janji temu. It is an operational identifier, not a marketing reference. |
| Check-in code | `response.checkin_code` | Display as the check-in code. Keep the approved copy and safe copy action. |
| Appointment check-in | `appointment_id` + `checkin_code` | Submit to `POST /api/v1/appointments/{id}/check-in`. Booking success links to the prefilled check-in frame. |
| Queue number | `formatted_number` | Display as the queue number after successful check-in or walk-in generation. |
| Wait estimate | `estimated_wait_minutes` | Display the backend value with clear estimate language. Current implementation evidence indicates the engine value is fixed at 25 minutes; do not present it as live calculation. |
| Visit state | status response | Render the approved current-state progress: Check-In, Antre, Dilayani, Selesai. Do not turn it into fabricated visit history. |

### Admin

| Design field | API/backend field | Transformation and display |
|---|---|---|
| Queue facility | `queue.facility_id` | Join against the scoped facilities result by exact ID, then display `facility.name`. |
| Appointment facility | `appointment.facility_id` | Join against the scoped facilities result by exact ID, then display `facility.name`. |
| Schedule facility | `schedule.facility_id` | Join against the scoped facilities result by exact ID, then display `facility.name`. |
| Appointment service | `appointment.service_unit_id` | Join against scoped `GET /admin/service-units`, matching both the ID and permitted facility scope. If unresolved in the static prototype, display `-`; production must resolve before populated display. |
| Schedule service | `schedule.service_unit_id` | Same scoped service-unit join. Never use facility type, prefix, hardcoded mapping, or assumption. |
| Practitioner | `practitioner_id` | Keep out of user-facing display until a supported practitioner read source exists. Do not invent practitioner names. **Phase 3A.1 rule:** no raw-UUID or free-text practitioner field in the schedule editor and no fake practitioner catalog. CREATE omits `practitioner_id` (backend accepts absence); UPDATE omits it from the request body so the backend pointer/`omitempty` PATCH preserves the existing value. |
| Queue status | `status` | Use the exact queue state machine: waiting -> called/cancelled; called -> in_service/cancelled/skipped; in_service -> completed. Terminal: completed, cancelled, skipped. |
| Appointment status | `status` | **Phase 3A correction.** Use the exact server-supported map: `scheduled -> checked_in/cancelled/no_show`; `checked_in -> queued/cancelled/no_show`; `queued -> completed/cancelled/no_show`; `completed`, `cancelled`, `no_show` are terminal. Note `checked_in -> completed` is NOT allowed; `completed` is reachable only from `queued`. Invalid transitions return **400**, not 409. See Section 6.1. |
| Schedule state | `is_active` | Create active schedules and support the existing PATCH state behavior. Do not imply facility reactivation where the facility API is one-way deactivate. |
| Notification recipient | masked notification row field | Preserve masking. Never display the original contact value. |
| Notification actions | notification status and permission | Retry/cancel only when status and permission allow. Treat authorization as a server-side requirement, not a UI-only check. |
| Notification scope | `facility_id` and actor scope | Apply server-authorized scope. A client filter is only a narrowing control, never an authorization boundary. |

## 6. State Matrix

Only states supported by the current contract are listed.

| Route or module | Normal | Loading | Empty / no result | Validation | Success | Error / authorization | Conflict / rate limit |
|---|---|---|---|---|---|---|---|
| Beranda | Supported catalog and task actions | Catalog loading when applicable | No active facilities | Not applicable | Navigation to next task | Catalog failure with retry | No unsupported realtime claim |
| Faskes | Scoped/public facility list | Skeleton list | No active facilities | Filter mismatch | Select facility and continue | Catalog error with retry | No fake opening-hour or availability state |
| Buat Janji Temu | Three steps | Catalog or submit loading | Catalog unavailable state | Phone, future time, required fields | Check-in code and next-step CTA | Generic server error where contract is generic; retry | 429 uses backend limit copy; 409 only where backend contract supports full-slot conflict |
| Check-In Janji Temu | ID and code form, with deep-link prefill | Submit loading | Empty initial form | Required ID/code | Queue number and status CTA | Wrong code, not found, invalid state, network/server error | Backend 429 copy for check-in limit |
| Walk-in | Name and phone form | Submit loading | Facility-full state where supported | Phone and required fields | Queue number and status CTA | Server/network error | Daily limit and capacity-full states only when returned |
| Status Kunjungan | Code lookup form | Lookup loading | Code not found | Required code | Current-state progress | Service error | Backend 429 copy |
| Login / Register | Form | Submit loading | Not applicable | Field-level validation | Registration redirects to login success state | Auth not configured or invalid credentials | No unsupported auth role claim |
| Admin Ringkasan | Derived counts from loaded scoped data | Loading skeleton | Empty facility scope | Not applicable | Cross-links to modules | Forbidden (403) / fetch error | No invented KPI or notification fallback |
| Admin Antrean | Queue board and actions | Table skeleton | No rows today | Filter input semantics | Inline status update | 403 (both unauthenticated and unauthorized; see Section 6.2), network/server error | Invalid transition returns **400 validation**, not conflict; polling is periodic, not realtime |
| Admin Janji Temu | Table, filters, supported actions | Table skeleton | Empty / filter no result | Filter validation if required | Inline status update | 403 (see Section 6.2), server error | Invalid transition returns **400 validation**, not conflict |
| Admin Jadwal | Schedule table and dialogs | Table skeleton | Empty / filter no result | Date/time/duration/capacity rules | Create or update response | 403 (see Section 6.2), server error | No schedule manage permission hides or blocks mutation controls |
| Admin Fasilitas | Facility table | Table skeleton | Empty / filter no result | Required fields and phone rules | Create/update/deactivate response | 403 (see Section 6.2), server error | Deactivate is one-way in the current contract |
| Admin Notifikasi | Server-filtered list and summary | Separate list/summary skeletons | Empty / filter no result | Filter input validation | Retry/cancel followed by reload | Separate list and summary errors; 403 forbidden action state | Retry/cancel invalid state returns 409; no server pagination claim |

Rate-limit frames apply only where the current endpoint exposes a verified limit. Do not manufacture a generic rate-limit state for Admin modules without one. **Phase 3A verification: no `/api/v1/admin/*` route has any rate limit.** Verified limits exist only on `POST /queues/generate` (2/25h per phone+facility), `POST /appointments` (2/25h per phone), `POST /appointments/{id}/check-in` (5/5min per IP+appointment), and `GET /patient/status` (30/min per IP).

## 6.1 Exact status transitions (Phase 3A correction)

The appointment and queue lifecycles are separate enums in separate tables. Do not merge them.

### Appointment - admin mutation flow

`PATCH /api/v1/admin/appointments/{id}/status`. Verified at `apps/api/internal/handler/admin.go:1950-1958`.

| source_status | allowed destination statuses |
|---|---|
| `scheduled` | `checked_in`, `cancelled`, `no_show` |
| `checked_in` | `queued`, `cancelled`, `no_show` |
| `queued` | `completed`, `cancelled`, `no_show` |
| `completed` | none (terminal) |
| `cancelled` | none (terminal) |
| `no_show` | none (terminal) |

Rejections: unknown status value -> 400 `Status janji temu tidak valid.`; transition not in the table -> 400
`Transisi status '<from>' -> '<to>' tidak diizinkan.`

### Appointment - public check-in flow (a different mechanism)

`POST /api/v1/appointments/{id}/check-in`. This flow does **not** use the transition table above. It performs
two guarded statements in one request: `scheduled -> checked_in` (atomic claim, requiring the matching
`checkin_code`) and then `checked_in -> queued` (finalisation, attaching the queue ticket). A successful public
check-in therefore reports `"status": "queued"`. Do not infer the admin map from this flow, or vice versa.

Public check-in status codes: 400 malformed, **401 wrong code**, 404 appointment absent, 409 wrong state or lost
race, 429 brute-force limit. The 401 here means "wrong code", not "not authenticated".

### Queue - admin mutation flow

`PATCH /api/v1/admin/queues/{id}/status`. Verified at `apps/api/internal/handler/admin.go:810-817`.

| source_status | allowed destination statuses |
|---|---|
| `waiting` | `called`, `cancelled` |
| `called` | `in_service`, `cancelled`, `skipped` |
| `in_service` | `completed` |
| `completed` | none (terminal) |
| `cancelled` | none (terminal) |
| `skipped` | none (terminal) |

Invalid transition -> 400 `Transisi status tidak valid: <from> -> <to>.`

## 6.2 Admin authentication vs authorization semantics (Phase 3A correction)

The handoff previously listed a generic "401/403". Verified runtime behavior:

| Runtime condition | Status | Body `error` |
|---|---|---|
| Undeclared method+path | **401** | `Akses ditolak: rute tidak dikenali atau memerlukan otorisasi.` |
| No credentials / invalid or expired token | **403** | `Akses ditolak: autentikasi diperlukan.` |
| Authenticated, missing required permission | **403** | `Akses ditolak: izin tidak mencukupi.` |
| Valid permission, empty facility assignment (list endpoints) | **200** | `{"success":true,"data":[]}` |
| Scope resolution failure (list endpoints) | **200** | `{"success":true,"data":[]}` |
| Scope resolution failure (detail/mutation endpoints) | **404** | `... tidak ditemukan.` |

**Implementation consequence:** the authorization layer never returns 401 for missing authentication, so the UI
cannot branch on status code. The Admin shell must choose `UnauthPanel` versus `ForbiddenPanel` from the presence
of a client-side session (available from `+layout.server.ts`), not from the status code. A 401 on an Admin call
indicates a proxy or method defect, not a logged-out user.

**Do not invent a 401 unauthenticated state for Admin screens.** It does not exist.

## 7. Responsive Rules

### Citizen

- Primary mobile target: 390px. Desktop target: 1440px.
- Mobile uses a compact header and bottom navigation with four destinations.
- Reserve content clearance for the fixed bottom navigation and safe-area inset.
- Forms remain single-column and readable on mobile.
- Desktop may use a two-column booking composition, with form and summary separated.
- Do not make transactional controls excessively wide.
- Maintain approximately 44px touch targets.
- Do not introduce a permanent appointment tab.

### Admin

- Primary desktop target: 1440px.
- Operational target: approximately 1024px.
- At approximately 1024px, collapse the sidebar to a 56px icon rail with accessible labels/tooltips.
- Hide lower-priority secondary table columns while preserving status and actions.
- Keep horizontal table overflow intentional and scoped to the table region.
- Dialogs must fit the viewport and retain keyboard access.
- Do not create an Admin mobile layout.
- Preserve the approved shell and responsive behavior; do not normalize Citizen and Admin density.

## 8. Accessibility Requirements

Target WCAG 2.2 AA. This is an implementation target, not a formal certification claim.

- Maintain one logical heading hierarchy per frame.
- Keep labels programmatically associated with controls.
- Use `aria-current` for active navigation.
- Use `aria-invalid` and an associated error message for invalid fields.
- Use links for navigation and buttons for actions.
- Use readonly for displayed identifiers that cannot be edited; do not use disabled when the value still needs to be copied or understood.
- Preserve visible `:focus-visible` rings with sufficient contrast.
- Keep status meaning available in text, not only color.
- Preserve table semantics with `caption`/headers where appropriate and `scope` on column headers.
- Dialogs require `role="dialog"`, `aria-modal="true"`, a labelled title, focus trap, Escape handling, and focus return.
- Dynamic refresh and mutation results should use an appropriate polite live region without announcing every table cell.
- Icon-only controls require accessible names. Decorative icons remain hidden from assistive technology.
- Citizen touch targets remain approximately 44px. Admin compact controls must still have an understandable name and visible focus.

## 9. Polling and Refresh

- Antrean uses periodic polling at 30 seconds when the implementation enables it.
- Manual refresh remains available.
- Pause or reduce polling when the tab is hidden via `visibilitychange`.
- Display the last updated time.
- Do not call this realtime and do not use SSE semantics for queue data.
- Loading, empty, error, and stale-data behavior must be explicit.

## 10. Security and Backend Blocker Register

These are implementation blockers, not design changes.

### P0 - Security

All five items below were re-verified against current source in Phase 3A. Full root-cause detail, exploit
paths, remediation, required tests, and migration verdicts are in
`design/sigap-redesign-implementation-plan.md` Section 5. None are fixed yet. **No database migration is
required for any of them.**

1. **Notification retry authorization ordering.** `apps/api/internal/handler/notifications.go:299` performs the
   `Retry` mutation, then `:314` performs the facility scope check. Authorization must be enforced before any
   mutation. An actor scoped to facility A can currently force a retry of facility B's outbox row (the response
   is 404, but the write has already committed).
2. **Notification cancel authorization ordering.** Same defect at `notifications.go:334` (mutation) versus
   `:349` (scope check). Because `Cancel` is idempotent, this is a denial-of-notification primitive across
   facility boundaries.
3. **Zero-assignment facility scope behavior is inconsistent across endpoint classes.** List endpoints correctly
   return an empty set, but detail and mutation endpoints **fail open**. See item 5.
4. **Notification summary empty-scope/global fallback leaks platform-wide counts.**
   `notifications.go:188-213` with `apps/api/internal/notification/service.go:227-251`: an empty-but-valid scope
   leaves `facilityFilter = uuid.Nil`, which the service treats as the "all facilities" sentinel, so the actor
   receives platform-wide notification counts. This is an authorization bypass, not merely an inconsistency.
5. **NEW - Detail and mutation endpoints fail open on empty scope (CRITICAL).** `admin.go` lines `190, 362, 409,
   658, 999, 1202, 1459, 1693` all use `if len(allowedFacilities) > 0 { <apply predicate> }`. When a non-dev
   actor has zero assignments the predicate is omitted and the statement runs unscoped. Affected:
   `GetFacility`, `UpdateFacility`, `DeactivateFacility`, `GetQueueTicket`, `GetServiceUnit`,
   `UpdateServiceUnit`, `GetSchedule`, `UpdateSchedule`. A zero-assignment **non-super_admin** actor can read
   any facility by id and can modify or deactivate another facility's record (after the Phase 3A.1 decision,
   a global `super_admin` is no longer an exploit actor: its unrestricted scope is explicit and structural).
   This is the highest-severity finding of Phase 3A and is not present in the original register.

Note: the zero-assignment case is reachable by a real seeded role. A global role has `facility_id = NULL`, and
`apps/api/internal/auth/facility_scope.go:48-57` excludes NULL, so a `super_admin` resolves to an empty set.
Per the Phase 3A.1 decision below, the `super_admin` empty set is granted unrestricted scope **explicitly**;
every other empty set (e.g. an inactive or soft-deleted assignment, or a non-super_admin global role) remains
fail-closed.

**Phase 3A.1 owner decision (GATE 0):** a genuine `super_admin` is intended to have unrestricted global
administration. Unrestricted access MUST be explicit: an authenticated production actor receives
`unrestrictedFacilityScope = true` ONLY when current DB-resolved RBAC proves an **active** global
`super_admin` assignment in the existing role model (role `super_admin`, `facility_id = NULL`,
`user_roles.status = 'active'`, `deleted_at IS NULL`). An empty facility set alone, an absent
`facility_id` by accident, token claims, or frontend role inference NEVER grant unrestricted scope.
Dev actors keep their separate development-only unrestricted path. Endpoint permission checks still run
for unrestricted actors. Zero-assignment **non-super_admin** actors fail closed (list: empty; detail: 404;
mutation: 404 with zero side effects; notification summary: all-zero). No DB migration is expected. The
implementation shape and the exact resolver contract are in plan Section 4.5, and the required test
matrix (cases A-G) is in plan Section 13.2.

### P0 - Functional implementation

1. SvelteKit `[id]` proxies forward literal `ID`/`STATUS` in nine routes. Phase 3A audited all 23 proxy files
   individually; the nine defective ones are exactly: `admin/appointments/[id]/status` (literal `STATUS`),
   `admin/facilities/[id]`, `admin/facilities/[id]/deactivate`, `admin/notifications/[id]/cancel`,
   `admin/notifications/[id]/retry`, `admin/queues/[id]`, `admin/queues/[id]/status`, `admin/schedules/[id]`,
   `admin/service-units/[id]` (literal `ID`). The remaining 14 are correct. Use
   `appointments/[id]/check-in/+server.ts` as the reference pattern; it is the only dynamic proxy that
   interpolates correctly.
2. Facility deactivate proxy and method contract must match the backend PATCH route. Verified detail: the UI
   calls `PATCH` (`admin/facilities/+page.svelte:138`) while the proxy exports only `POST`, so SvelteKit returns
   405. The proxy must export `PATCH` **and** interpolate the id. Preserve one-way deactivate semantics.
3. Facility deactivate response carries `"is_active"` as the JSON **string** `"false"`
   (`admin.go:426-429`), not a boolean. Do not coerce it into a boolean comparison.

### P1 - Product correctness and implementation

1. Resolve facility names through the scoped facilities source using exact `facility_id` joins.
2. Resolve service names through scoped `GET /admin/service-units` using exact `service_unit_id` and facility scope joins. Do not infer service names.
3. Add the approved `/faskes`, `/queues/new`, and `/admin` route implementations without renaming existing URLs. (`/queues/new` is the Phase 3A walk-in decision; see Section 3.1.)
4. Implement queue polling with hidden-tab handling and manual refresh.
5. Ensure schedule mutations respect `schedule.manage`; operator/dev identities without that permission must receive the approved forbidden state.
6. Keep notification list and summary errors independently visible.

### P2 - Scale and follow-up

1. List endpoints without pagination may require future server-side pagination for scale. The current design must not imply it exists.
2. Ringkasan requires client-side aggregation from loaded endpoint data unless a supported aggregate endpoint is added.
3. Queue and appointment facility filtering is client-side when the endpoint returns a scoped dataset. It is not an authorization boundary.
4. Notification pagination is not currently server pagination. Any client-side result slicing must be labelled as loaded-result management.
5. Wait estimates are currently not a live computed model. Keep estimate language until the backend contract changes.
6. Real facilities require real service-unit records before populated service names can replace the neutral static value.

## 11. Navigation QA Result

- Citizen desktop navigation: approved destinations are wired to Beranda, Faskes, Check-In, Status, and the prominent Buat Janji Temu action.
- Citizen mobile navigation: approved four-tab structure is present; Buat Janji Temu is not a permanent tab.
- Booking success to prefilled Check-In: approved deep-link structure is present with `appointment_id` and `checkin_code` query values.
- Check-In success to Status: approved next-step path is present.
- Admin navigation: six destinations are wired across the frozen Admin pages with `aria-current` on the active destination.
- Known standalone exported ZIP filename/link limitations remain exporter behavior and are not treated as product navigation defects.

## 12. Product-Copy QA Result

The frozen product frames contain no audited implementation terms such as demo, seed, fixture, mock, backend, frontend, UUID, phase labels, TODO, Lorem, workspace, context switch, realtime, or Konsole. The appointment/service placeholder correction is frozen as neutral `-` where representative service data is unresolved. Legitimate citizen appointment identifiers remain because the workflow requires them.

Internal source comments and design documents may retain phase or implementation annotations. They are not user-facing product copy.

## 13. Consistency and Defect Findings

- No concrete cross-product visual consistency defect justified a frozen-screen change during this audit.
- Citizen and Admin share the canonical color, type, radius, border, icon, status, focus, and form semantics while retaining intentionally different density and navigation shells.
- The previously documented Admin service-unit placeholder correction is preserved. Static rows use `-`; production uses the scoped service-unit join.
- No frozen design file was changed during Phase 2D. The current Admin user change to the schedule pages was retained and not reverted.
- Accessibility requirements that cannot be represented fully in static HTML, such as focus trapping and Escape handling, are recorded as production implementation requirements.

## 14. Design Source and Export Inventory

| Project | Canvas nodes | Generated source HTML | Exported HTML |
|---|---:|---:|---:|
| Citizen mobile | 37 | 37 | 37 where exported |
| Citizen desktop | 37 | 37 | 37 where exported |
| Admin desktop | 48 | 42 | 48 |

The Admin discrepancy is intentional: the six primary module pages have 1440 and approximately 1024 canvas nodes that reference the same source HTML. The exporter emits one page per canvas node, while the generated source directory contains one file per unique source page.

No duplicate viewport node was removed to force counts to match.

## 15. Validation

Required validators for all three design projects:

- `validate-design-workspace`
- `validate-finish-readiness --check=all`

Expected completion condition: success with zero blocking errors. The known non-blocking warning about a missing `runtime-orchestration-summary.json` must be reported honestly if it appears. No dummy summary should be created solely to suppress that warning.

## 16. Known Exporter Limitation

The standalone exported Admin ZIP may rename files to display titles while internal links continue to use source filenames. This is exporter behavior and does not represent the approved product navigation model. Validate navigation against the design source project and production route map.

## 17. Implementation Guardrails

- Do not modify frozen Citizen or Admin visuals for preference alone.
- Do not invent service names, practitioners, facilities, opening hours, availability, queue realtime behavior, pagination, roles, or notification delivery fields.
- Do not modify production frontend/backend code as part of this design handoff.
- Do not deploy.
- Do not begin Phase 3 until P0 blockers are triaged and assigned.
