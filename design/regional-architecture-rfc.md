# Regional Architecture RFC — Tenancy, Facility Scope, and Org Boundaries for SIGAP v2

> Status: **Design document**. Every section is explicitly tagged
> **IMPLEMENTED**, **PROPOSED**, or **POLICY-DECISION-REQUIRED**. PROPOSED and
> POLICY-DECISION-REQUIRED content does not exist in the codebase or database
> today. Nothing here is a promise to ship; all proposed schema, roles, and
> permissions are directional until an owner approves them.

The evidence below is drawn from the real source: `packages/db/migrations/*.sql`
(0001–0011), `packages/db/seed/rbac.sql`, `apps/api/internal/{identity,auth,audit,handler}`,
and `apps/web/src/lib/api/endpoints/admin.ts`. Real table, column, and Go symbol
names are quoted verbatim.

---

## 1. Current facility model — IMPLEMENTED

### 1.1 What the schema actually is

The current model is **flat and facility-first**. There is no organization and no
region. The authoritative table is `facilities` (migration `0001_init.sql`):

- `facilities(id, name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, last_updated, is_active, short_code, created_at, updated_at)`
- `type` is the enum `facility_type ('rumah_sakit', 'puskesmas')`. This is the
  *only* classification above a facility, and it is a type label — not an
  organizational boundary.

Everything hangs off `facilities.id`:

| Table (migration) | Relationship |
|---|---|
| `service_units` (`0005`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE CASCADE` |
| `practitioners` (`0005`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE CASCADE` |
| `practitioner_schedules` (`0005`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE CASCADE` |
| `appointments` (`0005`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE CASCADE` |
| `queue_tickets` (`0001`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT` |
| `daily_queue_counters` (`0001`) | `facility_id … ON DELETE CASCADE` |
| `medical_records` (`0002`) | `facility_id NOT NULL REFERENCES facilities(id) ON DELETE RESTRICT` |
| `notification_outbox` (`0006`) | `facility_id UUID REFERENCES facilities(id) ON DELETE SET NULL` |
| `audit_events` (`0004`) | `facility_id UUID REFERENCES facilities(id) ON DELETE SET NULL` |

So the deepest hierarchy that exists today is exactly two levels:
**facility → service_unit** (via `service_units.facility_id`, and transitively
`appointments.service_unit_id`, `practitioner_schedules.service_unit_id`). The
`kecamatan`/`kabupaten_kota`/`provinsi` columns on `facilities` are free-text
address attributes; they are **not** keys, are not constrained to a registry, and
no RBAC decision ever reads them.

### 1.2 The organization gap — CONFIRMED

There is **no `organizations` table anywhere** in `packages/db/migrations/`
(0001–0011 create only: `facilities`, `patients`, `queue_tickets`,
`daily_queue_counters`, `medical_records`, `app_users`, `roles`, `permissions`,
`user_roles`, `role_permissions`, `audit_events`, `service_units`,
`practitioners`, `practitioner_schedules`, `appointments`,
`notification_templates`, `notification_outbox`,
`notification_delivery_attempts`, plus ALTER-only migrations 0008, 0009, 0010,
0011). There is no `regions`, no `hospitals` vs `puskesmas` split table, and no
`org_id` / `region_id` column on any of the tables above.

The consequence is a real boundary gap: an operator "at the District Health Office"
cannot be expressed. The only two scoping dimensions that exist are:
1. the `user_roles.facility_id` column (facility-level), and
2. the role name `super_admin` when **and only when** its `user_roles.facility_id IS NULL` (global).

There is no "all clinics in one puskesmas network" or "one district's facilities"
bucket that is not either a single facility or the entire system.

### 1.3 RBAC schema — IMPLEMENTED

`0003_identity_rbac.sql` defines the identity tables:

- `app_users(id, email, display_name, status, created_at, updated_at, deleted_at)`
  — `status IN ('active','disabled')`, soft-delete via `deleted_at`.
- `roles(id, name UNIQUE, description, is_system)`.
- `permissions(id, key UNIQUE, description)`.
- `user_roles(user_id, role_id, facility_id NULL, created_at, PRIMARY KEY (user_id, role_id))`.
- `role_permissions(role_id, permission_id, PRIMARY KEY (role_id, permission_id))`.

`0008_identity_subject.sql` adds `app_users.subject` (the JWT `sub`) with a partial
unique index `uq_app_users_subject_active`, decoupling external identity from
authorization (comment references `AUDIT-101`). `0009_user_roles_lifecycle.sql`
adds `user_roles.status IN ('active','inactive')` and `user_roles.deleted_at` so
facility-scope resolution excludes disabled/soft-deleted assignments (`AUDIT-202`).

The production seed (`packages/db/seed/rbac.sql`, dev-only, "NEVER run in
production") defines **four roles**: `super_admin`, `facility_admin`, `operator`,
`viewer` — and **twelve permission keys**: `queue.generate`, `queue.read`,
`queue.manage`, `facility.read`, `facility.manage`, `audit.read`,
`appointment.read`, `appointment.manage`, `schedule.read`, `schedule.manage`,
`notification.read`, `notification.manage`.

`audit.read` is granted **only** to `super_admin` in `rbac.sql` (the capability
matrix confirms: "`audit.read` is granted to `super_admin` only"). The
`facility_admin` role is the most privileged *facility-scoped* role.

### 1.4 Facility-scoped authorization — IMPLEMENTED (the current security model)

The current model revolves around facility **provenance**, not flat permission
sets. The relevant Go symbols (package `internal/auth` unless noted):

- `auth.FacilityScope`, `auth.FacilityScopeResult`, `auth.NewDBFacilityScope`,
  and the private `facilityScopeQuery`. That query's defining filter is:

  ```sql
  SELECT DISTINCT ur.facility_id,
         (ur.facility_id IS NULL AND r.name = 'super_admin') AS global_super_admin
    FROM user_roles ur
    JOIN app_users u ON u.id = ur.user_id
    JOIN roles r ON r.id = ur.role_id
   WHERE u.id = $1
     AND u.status = 'active' AND u.deleted_at IS NULL
     AND ur.status = 'active' AND ur.deleted_at IS NULL
  ```

  Two consequences are load-bearing: (a) the *only* global reach is a role named
  `super_admin` whose assignment has `facility_id IS NULL`; (b) a
  `super_admin` assignment **scoped to a facility** is not global — it is narrowed
  to that facility (there is a test case `"facility scoped super admin role"` in
  `auth/facility_scope_test.go` that asserts `wantIDs: 1`).

- `identity.FacilityGrant{ Key, FacilityID, Unrestricted }` carries per-key
  facility provenance. `identity.Actor` carries `UserID`, `AppUserID`, `Type`,
  `Permissions`, `FacilityGrants`, `IsDev`. The comment on `UserID` is explicit:
  in JWT mode it "MUST NOT be used as an authorization source".

- `identity.Actor.HasPermission(key)` is documented as **unsafe for
  facility-scoped mutations** (it checks the flat union). The provenance-aware
  check is `identity.Actor.HasPermissionAtFacility(key, facilityID)`.

- The two sanctioned entry points for facility-scoped decisions are
  `auth.AuthorizeFacilityMutation` and `auth.AuthorizeFacilityRead` (and
  `auth.AuthorizedFacilityIDsForPermission` for list reads). Each requires
  **both** conditions: the target facility is inside the actor's scope, **and**
  the actor holds the permission **at** that facility via DB-resolved grant
  provenance. This is what stops the "viewer at A + operator at B ⇒ queue.manage
  at A" Cartesian-widening escalation (pinned by
  `auth/rbac_resolver_test.go` and `handler/admin_scope_matrix_test.go`).

- Live re-resolution: `auth.FacilityGrantResolver`,
  `auth.NewDBFacilityGrantResolver`, `auth.ResolveFacilityGrants`, and
  `auth.ActorWithLiveGrants` exist so a revoked grant takes effect on the next
  request rather than at the next re-authentication. `AdminHandler` wires these
  via `WithFacilityScopeResolver` / `WithFacilityGrantResolver`.

- Handlers in `internal/handler/admin.go` (`beginFacilityRead`,
  `listFacilityReadSet`, `ListFacilities`, `GetFacility`, `ListQueueTickets`,
  `GetQueueTicket`, `ListServiceUnits`, `ListSchedules`, `ListAppointments`, …)
  all follow the same discipline: a **coarse** route-level
  `identity.RequirePermission` gate, then a per-resource provenance check that
  returns `404` (never `403`) for an out-of-provenance row so its existence does
  not leak.

- `identity.RequirePermission` (in `identity/authz.go`, with
  `identity.ContextWithAudit` / `identity.DeniedReason`) is the route-level
  middleware; the dev identity is deliberately restricted to read-only, non-PHI
  keys (`facility.read`, `notification.read`, `appointment.read`, `queue.read`,
  `schedule.read`, `audit.read` — no `.manage`, see `auth/dev_provider.go`).

### 1.5 Audit — IMPLEMENTED (append-only, tamper-evident)

`audit_events` (`0004`) is append-only with `actor_user_id`, `actor_type`,
`action`, `resource_type`, `resource_id`, `facility_id`, `request_id`,
`ip_hash`, `user_agent_hash`, `metadata JSONB`, and a prepared tamper-evident
chain (`previous_hash`, `event_hash`). The Go service is `audit.Service`
(`audit.NewService`, `audit.LogEvent`) with `audit.sanitizeEvent` /
`audit.SanitizeMetadata` enforcing a `forbidList` so `metadata` never carries
PII/PHI. The comment on the writer is explicit: "Every operation that mutates
data or accesses PHI should leave a record here."

**Known limitation** (from the capability matrix, §N5): admin rows never set
`Event.FacilityID` (`admin.go` comment region around the audit helper), so
facility-filtered audit is not possible on existing rows — a data fix a
tenant-aware audit design must account for.

---

## 2. Target tenancy design — PROPOSED

### 2.1 Tenant hierarchy

```
Region  ──► Healthcare Organization ──► Facility ──► Service Unit ──► Practitioner/Schedule
 (regional sysadmin)   (org admin)      (facility_admin) (service-unit staff)  (clinician)
```

- **Regional sysadmin** — operates at the top (`region` scope), manages the
  regional directory, all organizations, and cross-FOM governance. Highest
  administrative scope; see §3 for why it is **not** record-open.
- **Healthcare organization** — a grouping of one or more facilities (e.g. a
  hospital group, a puskesmas network, a district clinic cooperative). New
  `organizations` entity.
- **Hospital / Puskesmas** — today's `facilities` rows, re-labeled as members of
  an organization. Type remains `facility_type`.
- **Service unit** — today's `service_units` rows (poli/clinic), already a child
  of `facilities`.
- **Staff/operator** — today's `operator` / `facility_admin` / `viewer` roles,
  additionally scoped by the org/region in which their facility sits.
- **Citizen/patient** — unauthenticated today (code-keyed); any citizen account
  is gated by the deferred identity-provider and data-retention decisions
  (capability matrix F1).

### 2.2 Org → facility → service-unit hierarchy

Add (additive only):

- `organizations(id, name, region_id, is_active, created_at, updated_at)`.
- `regions(id, code, name, created_at, updated_at)` — or, minimally, model the
  region as a top-level `organizations` row with a `parent_id` self-reference;
  the RFC leans toward an explicit **two-level `regions` + `organizations`** to
  keep the "shared regional directory" (§2.5) a first-class concept.
- `ALTER TABLE facilities ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE RESTRICT;`

The **existing** `service_units.facility_id` edge is preserved; no new edge is
added between service unit and organization because `facility → organization` is
transitively sufficient and avoids denormalization.

### 2.3 Facility-scoped vs regional permissions

Reuse the existing provenance machinery rather than invent a second one:

- Keep `user_roles.facility_id` exactly as-is for facility-scoped grants.
- Add `user_roles.organization_id` (nullable) for org-scoped grants, and
  interpret an assignment with `organization_id NOT NULL AND facility_id IS NULL`
  as "applies to every facility in that organization".
- Add a strictly narrower "unscoped + role-name" test that mirrors the existing
  `(ur.facility_id IS NULL AND r.name = 'super_admin')` predicate: the only
  **regional** global reach is a role named `regional_sysadmin` with
  `organization_id IS NULL AND facility_id IS NULL`. A `regional_sysadmin`
  scoped to an organization is org-scoped, not regional — the same
  "scoping narrows, never widens" rule already proven for `super_admin`.

Authorization for a facility-scoped action becomes the conjunction already
implemented in `AuthorizeFacilityMutation` / `AuthorizeFacilityRead`, extended to
"facility in scope via (org or facility) grant **and** permission held at that
org-or-facility". This is an extension of `FacilityGrant`, not a replacement: the
`Unrestricted` flag already models "applies everywhere"; the new axis only splits
"everywhere" from "everywhere under this org".

### 2.4 Data segregation

Polymorphic ownership is rejected. Proposal: **facility remains the row-owner**
for all clinical/operational tables (`queue_tickets`, `appointments`,
`medical_records`, `service_units`, `practitioner_schedules`,
`notification_outbox`, `audit_events`). Segregation is derived, not stored again:
an actor's visible set is `facilities WHERE organization_id ∈ authorized_orgs
AND facility_id ∈ facility_scope`. The org is an **access-control grouping**, not
a second FK on every row. This keeps the existing indexes
(`idx_queue_tickets_facility_registered`, `idx_appointments_facility_id`, …) as
the enforcement path and avoids a data rewrite.

### 2.5 Shared regional directory

A shared directory of every facility (and its org/region) is already partly
served by `GET /api/v1/public/facilities` / `catalog.go`. Proposal: extend the
public catalog to return a `region`/`organization` grouping key (non-sensitive
only) so citizens can browse by region, and give `facility_admin`/`organization
staff` a scope-aware admin list. The source of truth stays `facilities` +
`organizations` + `regions`; the directory is a projection, not a new store.

### 2.6 Per-facility schedules + queue policies

Today schedule data is per facility via `practitioner_schedules.facility_id`, but
queue capacity is a **global constant**: `DAILY_QUEUE_LIMIT = 300` lives in the
Rust engine (`apps/queue-engine/src/engine/queue.rs`, cited by the capability
matrix N6). Proposal:

- Add per-facility policy columns (e.g. `facilities.daily_queue_limit`,
  `facilities.checkin_opens_minutes`, `facilities.checkin_closes_minutes` — the
  check-in window columns are already proposed in
  `sigap-v2-product-evolution.md` §2.1 as `checkin_opens_minutes` /
  `checkin_closes_minutes` with `DEFAULT 120` / `DEFAULT 240`).
- Registry-backed per-organization defaults that individual facilities may
  override, so one busy hospital and one quiet puskesmas no longer share a
  constant (N6).

### 2.7 Tenant-aware audit

- Fix the existing `facility_id` gap first (admin rows must set
  `Event.FacilityID`), then add `organization_id`/`region_id` columns to
  `audit_events` (nullable, `SET NULL`) so a regional reader can scope the log
  without reading sibling orgs.
- `audit.read` remains granted to `super_admin` (and, under the proposal, to a
  new `regional_auditor` / `regional_sysadmin`) **only**; it never expands to
  facility staff. Every read of the log is itself an audit event.

### 2.8 Cross-facility booking — only where authorized

Cross-facility scheduling (e.g. an org-level referral desk booking a slot at a
sibling facility) is **not** a silent widening. The rule: an actor may create an
`appointments` row at facility B only when B's `organization_id` is in the
actor's authorized org set **and** the actor holds `appointment.manage` /
`schedule.read` *with provenance at B*. The default cross-facility grant is
**denied**; an explicit org-level reference/referral permission must be added
before any cross-facility booking becomes possible. Citizen self-service booking
stays facility-scoped and code-keyed (the current `POST /api/v1/appointments`
model).

### 2.9 Onboarding / offboarding — safe

- **Onboard a facility:** create the `facilities` row, set `organization_id`,
  seed its `service_units`/`practitioners`, and grant the first
  `facility_admin` scoped to that facility. Nothing here reads patient data.
- **Offboard an operator:** soft-delete the `user_roles` row
  (`user_roles.status='inactive'`, `user_roles.deleted_at=NOW()`), relying on the
  existing `0009` lifecycle filter to exclude it from `facilityScopeQuery`; the
  audit `actor_user_id` is `SET NULL`, so history is retained.
- **Deactivate a facility:** reuse the existing soft-delete path
  (`DeactivateFacility` sets `facilities.is_active=false`) rather than deleting;
  add an org-scoped "deactivate subtree" that soft-deactivates a facility's
  `service_units` and schedules. No hard deletes of clinical rows.

### 2.10 Migration path — ADDITIVE only, no data rewrite

Mirror the discipline already in `0008`/`0009` (ALTER + partial unique index,
never touching 0001–0007):

1. `0012_organizations.sql` — create `regions`, `organizations`, and
   `ALTER TABLE facilities ADD COLUMN organization_id` (nullable initially).
2. `0013_regional_rbac.sql` — `ALTER TABLE user_roles ADD COLUMN organization_id`
   (nullable), plus partial unique indexes preserving the existing
   `(user_id, role_id)` semantics.
3. `0014_tenant_audit.sql` — `ALTER TABLE audit_events ADD COLUMN organization_id
   … / region_id …` (nullable, `SET NULL`).
4. `0015_facility_policy_columns.sql` — `facilities.daily_queue_limit` etc., all
   with nullable-free defaults so existing rows are safe (the
   `checkin_opens_minutes` pattern already specified in the product-evolution
   doc).

No existing row is rewritten; every change is a new column/table with a default.

---

## 3. No single unrestricted global admin for medical records — POLICY-DECISION-REQUIRED

**Rule: there is exactly one highest-scope role, and it is administrative-only.
It is NOT record-open.**

Today the highest-scope role is `super_admin` (unscoped, `facility_id IS NULL`).
Even so, **`medical_records` has no reader endpoint at all** — `GET
/api/v1/medical-records` does not exist (capability matrix F5: "`medical_records`
exists with a SHA-256 signature and **no reader endpoint**"). `medical_records`
is linked by `patient_phone` (not `patients.id`), stores an immutable
`signature` (SHA-256), and exposes nothing to any role. So the current
`super_admin` is *already* not record-open.

Under the proposed tenancy the highest-scope role is **`regional_sysadmin`**
(regional, unscoped). It is **deliberately not record-open**, for three reasons
that must be enforced, not merely stated:

1. **Consent-first.** The `medical_records.signature` and `patient_phone` model
   is keyed to the patient, not the administrator. Clinical access requires
   patient consent and a treating-facility relationship; no org/regional
   principal can derive that from an administrative scope.
2. **Authorization-first.** Reading records requires a *new* permission key
   (e.g. `medical_record.read`), which today does not exist. It is **not** added
   to any role; adding it (and who may hold it — clinician, treating-unit staff,
   or facility admin under specific conditions) is a **POLICY-DECISION-REQUIRED**
   item, on par with F5 ("Exposing it is a clinical-data decision, not an
   engineering one").
3. **Audit-first.** Any eventual `.read` on records must be `audit_events`-logged
   with the patient acted upon, the consent basis, and the treating facility —
   and never carry record content in `metadata` (`audit` forbids PII/PHI).

`regional_sysadmin` therefore manages **tenancy structure and directory**, can
grant/revoke org and facility roles, and can read the *audit* log scoped to its
region — **not** individual medical records. A single unrestricted record-reading
global admin is explicitly ruled out.

Section verdict: the *meta*-question (whether any medical-record reader endpoint
exists and for whom) is **POLICY-DECISION-REQUIRED**; the *absence* of such a role
is the current, correct **IMPLEMENTED** state.

---

## 4. Phased implementation plan

Dependencies are linear within each phase; phases must not start until the prior
phase's acceptance criteria are met. No phase merges to `main` or touches the
production database.

**Phase 1 — Org/region schema (additive).**
- *Work:* migrations `0012` (regions, organizations, `facilities.organization_id`)
  and `0013` (`user_roles.organization_id` + partial indexes).
- *Deps:* none.
- *Acceptance:* migrations are forward-only; `facilities` rows without an org
  still function; `facilityScopeQuery` behavior is byte-for-byte unchanged for
  pre-existing grants; no existing row is rewritten.

**Phase 2 — Resolve org scope in auth.**
- *Work:* extend `FacilityGrant`/`FacilityScopeResult` with an org axis; extend
  `AuthorizeFacilityMutation`/`AuthorizeFacilityRead` to honor
  "facility in scope via org OR facility grant"; add a `regional_sysadmin`
  predicate mirroring the `super_admin` rule.
- *Deps:* Phase 1.
- *Acceptance:* existing facility-scoped tests (`admin_scope_matrix_test.go`,
  `rbac_resolver_test.go`, `facility_scope_test.go`) pass; new tests prove an
  org-scoped grant reaches every facility of that org but none outside it; a
  facility-scoped `regional_sysadmin` is narrowed, not global.

**Phase 3 — Tenant-aware audit + facility attribution fix.**
- *Work:* make admin write paths set `Event.FacilityID`; add
  `audit_events.organization_id`/`region_id` (migration `0014`).
- *Deps:* Phase 1 (Phase 2 optional but recommended).
- *Acceptance:* `audit_events` rows carry facility + org; a regional
  `audit.read` returns only the region's rows; PII-free invariant
  (`audit.sanitizeEvent`) holds.

**Phase 4 — Per-facility queue + check-in policy.**
- *Work:* `0015` policy columns; thread `facilities.daily_queue_limit`,
  `checkin_opens_minutes`, `checkin_closes_minutes` into the Rust queue engine
  and the check-in handler.
- *Deps:* Phase 1 (columns) and, for org defaults, Phase 2.
- *Acceptance:* two facilities with different limits produce different daily
  caps; check-in window honors the facility row defaults (`120`/`240` minutes);
  the `DAILY_QUEUE_LIMIT` constant is no longer the sole source of truth.

**Phase 5 — Cross-facility booking gated by org authorization.**
- *Work:* add an explicit referral/`appointment.manage`-at-org grant shape;
  enforce B's-org-in-scope **and** provenance-at-B before any cross-facility
  `appointments` write.
- *Deps:* Phase 2.
- *Acceptance:* an org referral role can create at a sibling facility; a
  facility-scoped operator cannot create at another facility even inside scope;
  citizen self-service remains facility-scoped.

**Phase 6 — Safe onboarding/offboarding + directory.**
- *Work:* org-scoped facility onboarding; soft-offboarding via
  `user_roles.status`/`deleted_at`; extend the public catalog with region/org
  grouping; `DeactivateFacility` subtree soft-deactivation.
- *Deps:* Phases 1–3.
- *Acceptance:* onboarding creates facility + scoped `facility_admin` with no
  patient-data read; offboarding excludes the actor the next request
  (`ActorWithLiveGrants`); deactivation soft-disables the subtree; directory
  grouping exposes no PHI.

**Phase 7 — Medical-record access design (blocked on policy).**
- *Work:* none until the owner decides whether any `medical_record.read` endpoint
  exists and who may hold it (§3).
- *Deps:* **POLICY-DECISION-REQUIRED** — patient-consent model, retention policy,
  and treating-facility authorization must be decided first.
- *Acceptance:* not applicable until the policy decision is recorded; the default
  remains "no reader endpoint, no record-open role".