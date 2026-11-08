# Regional Pilot Proposal — One District, Few Facilities, Zero Record-Open Scope

> Status: **Design document**. Every section is tagged **IMPLEMENTED**,
> **PROPOSED**, or **POLICY-DECISION-REQUIRED**. PROPOSED and
> POLICY-DECISION-REQUIRED content does not exist in the codebase or database
> today and is directional until an owner approves it. Nothing here is a promise
> to ship.
>
> **No fabricated data.** Every facility, region, and person named in this
> document is a **placeholder** — a made-up name used only to make the pilot
> concrete. None of them is a real district, facility, or person. Real facility
> master data would come from the deployment owner at onboarding time and, for
> SATUSEHAT interop, from the national registry (DFO/REGFASYANKES/RS Online).

Evidence is drawn from `design/regional-architecture-rfc.md` (RFC),
`design/product-positioning.md`, `design/service-day-queue-policy.md`,
`design/sigap-v2-capability-matrix.md` (capability matrix),
`design/satusehat-interoperability-plan.md` (interop plan), and
`design/identity-access-consent-threat-model.md`. Real table/column/symbol names
are quoted verbatim from `packages/db/migrations/*.sql`,
`packages/db/seed/rbac.sql`, and `apps/api/internal/*`.

---

## 1. Purpose and one-line shape

SIGAP is positioned as the **regional civic-health access-and-operations layer**
(product positioning §1, §4): it should make one *region's* many facilities
discoverable and bookable behind one surface, without ever becoming a national
record registry. Today the schema is **flat and facility-first** with **no
`organizations` table** and **no region** (RFC §1.1–1.2); the deepest real
hierarchy is `facility → service_unit`. The regional pilot is how we prove the
first honest step on top of that: **one district, a handful of facilities, one
shared directory, and cross-facility booking only where an explicit org-level
grant authorizes it — with SATUSEHAT interop exercised in sandbox only, behind
available credentials.**

The pilot is deliberately small. It is not a national rollout and not a clinical
record project.

---

## 2. Pilot scope (concrete)

### 2.1 Region — placeholder

- **Region:** one kabupaten/kota. Placeholder name **"Kabupaten Pilotsari"**.
  *This is a made-up name; it is not a real district.*
- **Timezone assumption:** WIB (`Asia/Jakarta`, UTC+7). This matches the
  implemented single-zone default — `serviceDayWIB = time.FixedZone("WIB",
  7*60*60)` in `admin.go`, with `facilities` having **no timezone column**
  (service-day policy §2.1). A WIB-only pilot deliberately stays inside the
  implemented default so the per-facility timezone seam is a prerequisite to
  record, not a silent blocker (see §5, prerequisite P4).
- **Final site selection** is the deployment owner's decision
  (**POLICY-DECISION-REQUIRED**); this document fixes the *shape* (1 district, N
  facilities), not the literal district.

### 2.2 Facilities to onboard — placeholders

| # | Placeholder name | `facility_type` | Notes (all placeholder) |
|---|---|---|---|
| F-1 | **RSUD Pilotsari** | `rumah_sakit` | Regional public hospital; service units Poli Umum, Poli Gigi, IGD |
| F-2 | **Puskesmas Pilotsari Kota** | `puskesmas` | Urban puskesmas |
| F-3 | **Puskesmas Pilotsari Pesisir** | `puskesmas` | Rural puskesmas |
| F-4 | **Puskesmas Pilotsari Rawat Inap** | `puskesmas` | Rural puskesmas with beds (`total_beds`/`available_beds` populated) |

> The 1 hospital + 3 puskesmas mix is intended to exercise the one thing the
> current constant cannot model: a busy hospital and a quiet puskesmas sharing
> `DAILY_QUEUE_LIMIT = 300` (capability matrix N6; RFC §2.6). Whether the pilot
> also flips the per-facility capacity column is a **PROPOSED** sub-scope, not a
> pilot gate.

All four are onboarding targets. No patient names, no real persons, no real
addresses appear anywhere; `kecamatan`/`kabupaten_kota`/`provinsi` free-text
address attributes are placeholder strings.

### 2.3 Roles to onboard

**IMPLEMENTED (exist in `rbac.sql` seed, production equivalent):**

- `super_admin` — global, `facility_id IS NULL`; the *only* current global reach
  (RFC §1.3–1.4).
- `facility_admin` — most privileged facility-scoped role.
- `operator`, `viewer` — facility-scoped operating roles.

**PROPOSED (do not exist today; required by the RFC target hierarchy §2.1):**

- `regional_sysadmin` — regional, unscoped (`organization_id IS NULL AND
  facility_id IS NULL`); manages the regional directory, all organizations, and
  cross-org governance. **Administrative-only, NOT record-open** (RFC §3).
- `regional_auditor` — `audit.read` scoped to the pilot region; read-only audit
  log access, never medical records (RFC §2.7).
- An org-scoped referral grant shape (e.g. role `org_referral_coordinator`) that
  holds `appointment.manage`/`schedule.read` **with provenance at an
  organization**, enabling the cross-facility booking desk (RFC §2.8, Phase 5).

Citizen/patient remains **code-keyed and unauthenticated** for the booking/queue
surface, exactly as today; any citizen account is out of pilot scope (capability
matrix F1).

---

## 3. Guiding constraints carried over from the RFC

Three constraints bound every phase below. They are the pilot's safety rails.

1. **`regional_sysadmin` is not record-open.** It manages tenancy structure and
   directory, grants/revokes org and facility roles, and reads the *audit* log
   scoped to its region — **never** individual `medical_records`. Today
   `medical_records` has **no reader endpoint at all** (`GET
   /api/v1/medical-records` does not exist; capability matrix F5); the pilot
   does **not** introduce one (RFC §3). **IMPLEMENTED** (absence of the role) /
   **POLICY-DECISION-REQUIRED** (any future `.read`).
2. **Facility remains the row owner.** Segregation is derived, not re-stored: an
   actor's visible set stays `facilities WHERE organization_id ∈
   authorized_orgs AND facility_id ∈ facility_scope` (RFC §2.4). Org is an
   access-control grouping, not a second FK on every clinical row.
3. **Cross-facility booking is denied by default.** An actor may create an
   `appointments` row at facility B only when B's `organization_id` is in the
   actor's authorized org set **and** the actor holds the permission *with
   provenance at B* (RFC §2.8). Citizen self-service book/check-in stays
   facility-scoped and code-keyed.

---

## 4. Pilot phases, KPIs, and acceptance criteria

Dependencies are linear within a phase. No phase advances until the prior
phase's acceptance criteria are met. No phase touches the production database,
and no phase merges to `main` until a phase gate is green. SATUSEHAT interop is
**sandbox-only** and is a separate, non-blocking phase (Phase 5).

### Phase 0 — Prerequisites (blocking gates)

This phase ships no feature; it proves the blockers in §5 are either present or
explicitly waived.

- **Work:** confirm `0012_organizations.sql` (regions, organizations,
  `facilities.organization_id`) and `0013_regional_rbac.sql`
  (`user_roles.organization_id` + partial indexes) are merged (RFC §2.10); confirm
  the `regional_sysadmin` predicate and org-scope resolution are in the auth
  path (RFC Phase 2); confirm the audit facility-attribution fix (RFC Phase 3).
- **KPI:** 4/4 pilot facilities can be assigned an `organization_id`; 0 existing
  rows rewritten; pre-existing facility-scoped tests still pass byte-for-byte
  (`admin_scope_matrix_test.go`, `rbac_resolver_test.go`,
  `facility_scope_test.go`).
- **Acceptance criteria:**
  - Migrations are forward-only and additive; `facilities` rows without an org
    still function.
  - `facilityScopeQuery` behavior is unchanged for pre-existing grants.
  - `regional_sysadmin` assignment with `facility_id IS NULL AND
    organization_id IS NULL` resolves as regional; a facility- or org-scoped
    `regional_sysadmin` is **narrowed, not global** (mirrors the existing
    `super_admin` scoping rule, RFC §2.3).
  - SATUSEHAT sandbox credential gates (interop plan §14) are filed as blocker
    items; none are fabricated as resolved.

### Phase 1 — Organization onboarding

- **Work:** create `regions` (Pilotsari placeholder) and one `organizations`
  row (e.g. "Dinas Kesehatan Pilotsari — placeholder"); assign a
  `regional_sysadmin` scoped to the region; assign the audit facility-attribution
  fix so regional `audit.read` can scope by region.
- **KPI:** 1 region created; 1 org created; 1 `regional_sysadmin` granted; 100%
  of onboarding mutations written to `audit_events` with `FacilityID` (or
  `NULL` legitimately for region-scope actions) set correctly.
- **Acceptance criteria:**
  - Onboarding a region/org reads **no patient data** and **no
    `medical_records`**.
  - Regional `audit.read` returns only the pilot region's rows; a sibling
    region (if any) is invisible.
  - `audit.sanitizeEvent` PII/PHI-forbidden invariant holds on every new row.

### Phase 2 — Facility provisioning

- **Work:** onboard F-1…F-4 via the org-scoped admin path: create each
  `facilities` row, set `organization_id`, seed `service_units` and
  `practitioners`/`practitioner_schedules`, and grant the first
  facility-scoped `facility_admin` per facility (RFC §2.9).
- **KPI:** 4 facilities created with correct `type`/`organization_id`; ≥1
  `facility_admin` granted per facility; ≥1 service unit per facility; 0
  patient-data reads.
- **Acceptance criteria:**
  - Onboarding a facility creates the facility + scoped `facility_admin`
    atomically, with no hard delete of any clinical row.
  - `DeactivateFacility` soft-disables the facility subtree
    (`facilities.is_active=false` + service_units/schedules) rather than
    deleting (RFC §2.9).
  - Offboarding an operator soft-deletes the `user_roles` row
    (`status='inactive'`, `deleted_at=NOW()`) and the actor is excluded on the
    **next request** via `ActorWithLiveGrants` (RFC §2.9).

### Phase 3 — Regional directory

- **Work:** extend the public catalog (`internal/handler/catalog.go`,
  `GET /api/v1/public/facilities`) to return a non-sensitive
  `region`/`organization` grouping key, and give org/facility admins a
  scope-aware admin list (RFC §2.5).
- **KPI:** 100% of pilot facilities returned with correct
  region/org grouping; 0 PHI/PII fields in the directory payload; 0 leaked
  facilities outside an actor's scope.
- **Acceptance criteria:**
  - Public catalog lists F-1…F-4 with group keys; no `patient_phone`, no
    `patient_display_name`, no contact beyond what the catalog already exposes.
  - A facility-scoped actor with an empty read set receives `200 []`, not an
    error (row existence never leaks; RFC §1.4 / threat model §4.7).
  - Directory is a projection over `facilities` + `organizations` + `regions`;
    no new store is introduced.

### Phase 4 — Cross-facility booking where authorized

- **Work:** add the explicit org-referral grant shape; enforce B's-org-in-scope
  **and** provenance-at-B before any cross-facility `appointments` write (RFC
  §2.8, Phase 5).
- **KPI:** 1 referral desk operator can create a booking at a sibling facility
  inside the org; 0 cross-facility writes succeed for a facility-scoped
  operator; 0 citizen self-service writes cross facility boundaries.
- **Acceptance criteria:**
  - An `org_referral_coordinator` (org-scoped, `organization_id` set) creates an
    `appointments` row at F-2 from an F-1 context; the row carries
    `facility_id = F-2` and the correct org.
  - A facility-scoped `operator` at F-1 **cannot** create at F-2 even though
    both are in the same org (denied, fail-closed, `404` not `403` to avoid
    existence leaks).
  - Citizen self-service (`POST /api/v1/appointments`) remains facility-scoped
    and code-keyed; the check-in-code bearer model is unchanged.
  - Cross-facility writes that do succeed are audited with `FacilityID` set.

### Phase 5 — SATUSEHAT interop (sandbox only) — BLOCKED on credentials

- **Work:** build the FHIR R4 mapping façade (`Patient`/`Organization`/
  `Location`/`Appointment`/`Encounter`) behind an adapter, post to the
  SATUSEHAT **sandbox** endpoint only, using the published dummy-patient table
  (interop plan §3, §7, §14).
- **KPI:** 0 production endpoints contacted; 0 real NIK/clinical data used; 100%
  of outbound writes idempotent (idempotency key or reused SATUSEHAT `id`); 100%
  of writes re-queued on transient failure via an outbox pattern.
- **Acceptance criteria (not met until credentials exist):**
  - Credentials are the **sandbox** `client_id`/`client_secret` bound to one
    sandbox Organization ID, issued only after partner registration + SSP
    verification + fasyankes data update (interop plan §7/§14 gates).
  - Only `https://api-satusehat-stg.dto.kemkes.go.id/oauth2/v1` and the sandbox
    FHIR R4 base are used — **never** `https://api-satusehat.kemkes.go.id/…`.
  - Onboarding run completes the five ordered steps (auth → Organization →
    Location → Practitioner IHS → Patient IHS) against a **test** org, using
    SATUSEHAT's sandbox dummy patients only.
  - NIK collection and retention, and any clinical-field caching, do **not**
    proceed without the owner decisions D1/D4 (interop plan §15).

> **Explicit status:** Phase 5 cannot start until SATUSEHAT sandbox credentials
> exist. They are **not available today** and are **not** self-serviceable — the
> eight gates in interop plan §14 are external to this repository. The pilot's
> go/no-go (below) therefore does **not** depend on Phase 5; it remains a
> separately-gated roadmap item, exactly as product positioning §7 requires
> ("no live SATUSEHAT integration exists today").

---

## 5. Explicit prerequisites (before Phase 0 can close)

Each is a gate, not a self-serve task. None may be assumed resolved.

| # | Prerequisite | Status | Notes |
|---|---|---|---|
| P1 | **`organizations`/`regions` tables + `facilities.organization_id`** | PROPOSED (RFC Phase 1, migration `0012`) | No `organizations` table exists anywhere today (RFC §1.2). |
| P2 | **`regional_sysadmin` role + org-scope auth resolution** | PROPOSED (RFC Phase 2, migration `0013`) | The only current global reach is `super_admin`; org-scoped "every facility in this org" does not exist. |
| P3 | **`user_roles.organization_id` + partial indexes** | PROPOSED (RFC Phase 2) | Preserve existing `(user_id, role_id)` semantics. |
| P4 | **Per-facility timezone (`facilities.timezone`)** | PROPOSED (service-day policy §2.3) | `facilities` has no timezone column; WIB default suffices for this WIB-only pilot, but the seam must exist before any WITA/WIT or multi-zone expansion. |
| P5 | **Tenant-aware audit + facility attribution fix** | PROPOSED (RFC Phase 3; capability matrix N5) | Admin rows currently do not set `Event.FacilityID`, so regional audit scoping needs the data fix first. |
| P6 | **SATUSEHAT sandbox credentials** | **NOT available** (interop plan §14) | External gates: partner registration, SSP verification, client credential issuance, fasyankes data update, dummy-patient table, Postman collection, owner sign-off. |
| P7 | **Owner policy decisions** (NIK collection/retention, consent purpose/opted-in set, record-read scope) | POLICY-DECISION-REQUIRED | Blocks interop patient-identity mapping (interop plan D1–D5) and any `medical_record.read` (RFC §3, capability matrix F5). |

---

## 6. Risks and mitigations

| # | Risk | Class | Mitigation |
|---|---|---|---|
| R1 | **Scope-creep into medical records.** A pilot that "onboards a region" tempts someone to also expose `medical_records`. | Boundary | `regional_sysadmin` is administrative-only; no `medical_record.read` permission is added; `medical_records` keeps no reader endpoint (RFC §3). |
| R2 | **Cross-facility privilege widening.** Flat-permission union lets "operator at A" reach B. | Security | Provenance-at-B check (`AuthorizeFacilityMutation/Read`); org-scoped referral is explicit and denied by default (RFC §2.8; threat model §4.6). |
| R3 | **Timezone drift across facilities.** A WITA/WIT facility read against WIB yields wrong service-day. | Correctness | Pilot is single-WIB; per-facility `timezone` column is a recorded prerequisite (P4), never silently ignored. |
| R4 | **SATUSEHAT sandbox contact with production data.** Mixing real NIK/clinical data into the sandbox PoC. | Compliance/privacy | Sandbox endpoints only; dummy patients only; no production credential (interop §14 non-goals). |
| R5 | **Fabricated "integrated" claims.** Reporting sandbox or proposed work as live. | Integrity | Every section tags IMPLEMENTED/PROPOSED/POLICY; SATUSEHAT stays "sandbox-only, blocked on credentials." |
| R6 | **Audit rows unattributed.** Regional `audit.read` cannot scope without `FacilityID`. | Accountability | P5 fix lands in Phase 0; new rows always set `FacilityID`. |
| R7 | **Silent capacity model mis-sale.** A hospital and puskesmas sharing `DAILY_QUEUE_LIMIT = 300` reads as a bug. | Product | Optional per-facility limit sub-scope (RFC §2.6, capability N6); not a pilot gate. |
| R8 | **Row-existence oracle.** A denied read leaks that a sibling facility exists. | Security | `404`/`200 []` fail-closed, never `403` with detail (RFC §1.4). |

---

## 7. Deliberately OUT of pilot scope

- **No medical-record reader endpoint.** `medical_records` stays closed to every
  role, including `regional_sysadmin` (RFC §3; capability F5).
- **No citizen accounts or appointment hub.** Citizen surface stays code-keyed;
  Supabase/OIDC citizen identity is out (capability F1).
- **No national identifier (NIK/IHS) collection** in the pilot unless/until the
  owner decides (POLICY-DECISION-REQUIRED, interop D1).
- **No production SATUSEHAT call, no production credential, no real patient
  data** (interop §14 non-goals).
- **No national rollout**, no second region, no multi-FOM governance beyond the
  single placeholder org.
- **No notification provider** (SMS/WhatsApp/email are schema-valid but
  undeliverable — `notification_outbox` ships `DevProvider` only; capability F4).
- **No clinical content** (Condition/Observation/MedicationRequest) — the
  `Encounter`/`Appointment` operational mapping is the only interop target in
  scope, and only in sandbox.
- **No wait-time model** beyond what exists; `estimated_wait_minutes` remains a
  constant and is not surfaced as a fabricated number (capability F7).

---

## 8. Go / no-go — success criteria

The pilot is **go** only if **all three** hold after Phases 0–4. Phase 5
(sandbox interop) is excluded from go/no-go because its credentials are blocked.

1. **One region, four facilities, one directory.** The `regional_sysadmin` can
   onboard the Pandu placeholder org and F-1…F-4, each scoped with its own
   `facility_admin`, and the public catalog returns all four with correct
   region/org grouping and **zero** patient data — with no existing row
   rewritten and pre-existing facility-scope tests green.
2. **Cross-facility booking is denied by default and granted only explicitly.**
   An `org_referral_coordinator` inside the pilot org successfully books a slot
   at a sibling facility; a facility-scoped `operator` cannot; and citizen
   self-service stays facility-scoped and code-keyed — all audited with
   `FacilityID` set.
3. **Regional audit scoping is real.** A `regional_auditor` reads only the pilot
   region's `audit_events` (facility-attributed, PII-sanitized), and no subject
   holding any administrative scope can read `medical_records` because no reader
   endpoint exists.

If any criterion fails, the pilot is **no-go**: the failure is recorded against
the specific phase gate, the relevant prerequisite (P1–P7) is re-opened, and no
follow-on region is onboarded.