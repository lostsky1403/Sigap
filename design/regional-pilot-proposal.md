# Regional Pilot Proposal — One District, Three Facilities, Zero Record-Open Scope

> Status: **DESIGN-ONLY**. This pilot has not been built and has not been run.
> Every item is tagged exactly one of:
> - **IMPLEMENTED** — the current code already runs it (file:line / symbol cited).
> - **PROPOSED** — engineered in the RFC but not built; needs backend/DB/UI work.
> - **POLICY-DECISION-REQUIRED** — engineered but blocked on an owner/legal decision.
>
> **PROPOSED** and **POLICY-DECISION-REQUIRED** together are **DESIGN-ONLY**: they
> do not exist in the codebase or database today. Nothing here is a promise to ship.

> **No fabricated data.** Every region, facility, administrator, and patient named
> in this document is a **placeholder** — a clearly fictional name used only to
> make the pilot concrete. None of them is a real district, facility, or person.
> Real facility master data would come from the deployment owner at onboarding time
> and, for SATUSEHAT interop, from the national registry (DFO/REGFASYANKES/RS Online).
> The synthetic patients/visits in §2.5 are sample data for contract tests only;
> they are **never** presented as real and are **never** written to a production database.

Evidence is drawn from `design/regional-architecture-rfc.md` (RFC),
`design/product-positioning.md`, `design/service-day-queue-policy.md`,
`design/sigap-v2-capability-matrix.md` (capability matrix),
`design/satusehat-interoperability-plan.md` (interop plan),
`design/sigap-implementation-backlog.md` (backlog), and
`design/identity-access-consent-threat-model.md`. Real table/column/symbol names
are quoted verbatim from `packages/db/migrations/*.sql`,
`packages/db/seed/rbac.sql`, `apps/api/internal/*`, and `apps/queue-engine/src/engine/queue.rs`.

---

## 1. Purpose and one-line shape

SIGAP is positioned as the **regional civic-health access-and-operations layer**
(product positioning §1, §4): it should make one *region's* many facilities
discoverable and bookable behind one surface, without ever becoming a national
record registry. Today the schema is **flat and facility-first** with **no
`organizations` table** and **no region** (RFC §1.1–1.2); the deepest real
hierarchy is `facility → service_unit`. The regional pilot proves the first
honest step on top of that: **one district, three facilities, one shared
directory, facility-scoped booking and queues, and cross-facility booking only
where an explicit org-level grant authorizes it** — with patient identity and
SATUSEHAT/FHIR interop a separately-gated roadmap (§6), not a pilot gate.

The pilot is deliberately small. It is not a national rollout and not a clinical
record project.

---

## 2. Pilot scope (concrete)

### 2.1 Region — placeholder

- **Region:** one kabupaten/kota. Placeholder name **"Kabupaten Pilotsari"**.
  *This is a made-up name; it is not a real district.*
- **Regional administration:** one placeholder org row
  **"Dinas Kesehatan Pilotsari — placeholder"** — an `organizations` row
  (**PROPOSED**, RFC §2.2) that groups the three pilot facilities. This is the
  "one regional administration" the pilot exercises; there is no second org and
  no multi-org governance in scope.
- **Timezone assumption:** WIB (`Asia/Jakarta`, UTC+7). This matches the
  implemented single-zone default — `serviceDayWIB = time.FixedZone("WIB",
  7*60*60)` in `admin.go`, with `facilities` having **no timezone column**
  (service-day policy §2.1). A WIB-only pilot deliberately stays inside the
  implemented default; the per-facility timezone seam is a recorded prerequisite,
  not a silent blocker (§7, P4).
- **Final site selection** is the deployment owner's decision
  (**POLICY-DECISION-REQUIRED**); this document fixes the *shape* (1 district, 3
  facilities), not the literal district.

### 2.2 Facilities to onboard — placeholders (all fictional)

| # | Placeholder name | `facility_type` | Notes (all placeholder) |
|---|---|---|---|
| F-1 | **RSUD Pilotsari** | `rumah_sakit` | Regional public hospital; service units Poli Umum, Poli Gigi, IGD |
| F-2 | **Puskesmas Pilotsari Kota** | `puskesmas` | Urban puskesmas; service units Poli Umum, KIA |
| F-3 | **Puskesmas Pilotsari Pesisir** | `puskesmas` | Rural puskesmas with beds (`total_beds`/`available_beds` populated); service units Poli Umum, Rawat Inap |

> The 1 hospital + 2 puskesmas mix is the smallest set that still exercises the
> one thing the current constant cannot model: a busy hospital and a quiet
> puskesmas sharing `DAILY_QUEUE_LIMIT = 300` (capability matrix N6; RFC §2.6).
> Whether the pilot also flips the per-facility capacity column is a **PROPOSED**
> sub-scope, not a pilot gate. All three are onboarding targets. No patient names,
> no real persons, no real addresses appear anywhere; the free-text
> `kecamatan`/`kabupaten_kota`/`provinsi` address attributes are placeholder strings.

### 2.3 Facility administrators — distinct, clearly fictional principals

Each facility gets its **own** distinct `facility_admin` — never one shared
principal — so authorization isolation can be observed, not asserted. The
principals are placeholder account labels on the fictional `.pilotsari.example`
domain, not real persons:

| Principal (placeholder) | Role | Scope | Can manage |
|---|---|---|---|
| `admin-f1@pilotsari.example` | `facility_admin` | F-1 only | F-1 service units, schedules, queues, appointments |
| `admin-f2@pilotsari.example` | `facility_admin` | F-2 only | F-2 service units, schedules, queues, appointments |
| `admin-f3@pilotsari.example` | `facility_admin` | F-3 only | F-3 service units, schedules, queues, appointments |
| `op-f1@pilotsari.example` | `operator` | F-1 only | F-1 queue operations (Panggil / Layani / Lewati), check-in |
| `reg-admin@pilotsari.example` | `regional_sysadmin` **(PROPOSED)** | region | directory, org/facility grants, regional audit read — **not** records |
| `reg-auditor@pilotsari.example` | `regional_auditor` **(PROPOSED)** | region | regional `audit.read` only — **not** records |
| `referral-f1@pilotsari.example` | `org_referral_coordinator` **(PROPOSED)** | the one org | cross-facility booking **with provenance at the target facility** |

`reg-admin`, `reg-auditor`, `referral-f1`, and the org-scoped + regional roles do
not exist today (**PROPOSED**, RFC §2.1–2.3 / §2.7). `facility_admin` / `operator`
/ `viewer` / `super_admin` already exist in `rbac.sql` (**IMPLEMENTED**, RFC §1.3).

### 2.4 Clinical vs operational permission scopes — distinct by design

The pilot must make **two scope classes** explicit and keep them non-overlapping:

| Class | What it covers | Permission keys | Status |
|---|---|---|---|
| **Operational** | queues, appointments, schedules, service units, facilities, notifications, the audit log | the 12 existing keys: `queue.generate/read/manage`, `facility.read/manage`, `audit.read`, `appointment.read/manage`, `schedule.read/manage`, `notification.read/manage` | **IMPLEMENTED** (RFC §1.3) |
| **Clinical** | any `medical_records` read, or any clinical content (`Condition`/`Observation`/`MedicationRequest`) | **none exist** — there is no `medical_record.read` key and no reader endpoint | **absent by design** (RFC §3; cap F5) |

The key demonstration is that the **operational** scope stops at the visit/queue
surface, and the **clinical** scope is *expressed as an absence*. Concretely:

- `operator`/`facility_admin` hold operational keys **only**; none of them is a
  clinical credential. Checking someone in, calling a queue number, or editing a
  schedule never grants record access.
- `regional_sysadmin` (the pilot's highest-scope role) is **administrative-only,
  not record-open**: it sees operational visibility across facilities and the
  region-scoped audit log, and **cannot** read `medical_records` — there is no
  reader endpoint and no permission key (RFC §3).
- A future **clinician**/practitioner identity (out of pilot scope) would be a
  *separate* scope with its own clinical keys, gated by the patient-identity +
  consent roadmap (§6). The pilot demonstrates the boundary, not the clinical read
  itself (**POLICY-DECISION-REQUIRED** for any `.read`, RFC §3).

Citizen/patient remains **code-keyed and unauthenticated** for the booking/queue
surface, exactly as today; any citizen account is out of pilot scope (cap F1).

### 2.5 Synthetic patients and visits — contract-test data only

The demonstrations below require patients and visits, but the pilot must never
touch a real person. All patient/visit rows are **synthetic**:

- Placeholder patient labels such as `demo-patient-01 … demo-patient-N`, with
  provider-side synthetic `patient_phone` values and placeholder `display_name`s
  (e.g. `"Pasien Demo 01"`). None resolves to a real number or name.
- Synthetic visits are created through the **same public API** as a real citizen
  would use (`POST /api/v1/appointments` → 6-char check-in code → check-in →
  gRPC queue ticket; and `POST /api/v1/queues/generate` for walk-ins). They are
  seeded into the *pilot* database only, labelled `[synthetic]`, and are excluded
  from any demo screenshot that could read as real.
- The service-day test (`§5.4`) drives a synthetic visit across the `23:59 WIB →
  00:01 WIB` boundary to prove day-bucketing without any real patient data.

This is the honesty guardrail: **synthetic data proves the contract; it is never
presented as a real citizen's visit.**

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

## 4. What the current code ALREADY runs vs what is DESIGN-ONLY

The pilot stands on an implemented floor. This table is the honest split —
anything not in the left column is **DESIGN-ONLY**.

| Capability | Status | Where |
|---|---|---|
| Flat facility schema; `facility → service_unit` | **IMPLEMENTED** | migrations `0001`–`0011` |
| RBAC: 4 roles, 12 permission keys, `user_roles.facility_id` scoping | **IMPLEMENTED** | `0003`/`0008`/`0009`; `seed/rbac.sql` |
| Facility-scoped auth: `FacilityGrant`, `HasPermissionAtFacility`, `AuthorizeFacilityMutation`/`Read`, live re-resolution, `404`-not-`403` | **IMPLEMENTED** | `internal/auth`; `handler/admin.go` |
| Append-only tamper-evident audit + PII/PHI forbid-list sanitizer | **IMPLEMENTED** | `0004`; `audit/service.go` |
| Public catalog + service units | **IMPLEMENTED** | `catalog.go` (`GET /api/v1/public/facilities`, `/service-units`) |
| Citizen booking → check-in code → queue ticket; walk-in generation | **IMPLEMENTED** | `booking.go`, `queue.go`; `queue.rs` |
| Service-day WIB default + additive `?date=` | **IMPLEMENTED** | `serviceDayWIB` / `resolveServiceDay` in `admin.go`; web proxy forwards `?date=` |
| Per-facility queue limit constant `DAILY_QUEUE_LIMIT = 300` | **IMPLEMENTED** | `apps/queue-engine/src/engine/queue.rs` |
| `regions`/`organizations` tables + `facilities.organization_id` | **DESIGN-ONLY (PROPOSED)** | RFC §2.2; no table exists today |
| Org/region-scoped authorization + `regional_sysadmin`/`regional_auditor`/`org_referral_coordinator` | **DESIGN-ONLY (PROPOSED)** | RFC §2.1–2.3, §2.7–2.8 |
| Admin audit `Event.FacilityID` attribution fix | **DESIGN-ONLY (PROPOSED)** | cap N5; RFC §2.7 |
| Cross-facility booking, patient-identity, FHIR/SATUSEHAT interop | **DESIGN-ONLY** | §6 roadmap; RFC §2.8; backlog Phases 7–9 |

---

## 5. Pilot phases, demonstrations, and acceptance criteria

Dependencies are linear within a phase. No phase advances until the prior
phase's acceptance criteria are met. No phase touches the production database,
and no phase merges to `main` until a phase gate is green. SATUSEHAT interop and
patient identity are a separate roadmap (§6), not a pilot phase.

### Phase 0 — Prerequisites (blocking gates)

This phase ships no feature; it proves the blockers in §7 are either present or
explicitly waived.

- **Work:** confirm `0012_organizations.sql` (regions, organizations,
  `facilities.organization_id`) and `0013_regional_rbac.sql`
  (`user_roles.organization_id` + partial indexes) are merged (RFC §2.10); confirm
  the `regional_sysadmin` predicate and org-scope resolution are in the auth
  path (RFC §2.3); confirm the audit facility-attribution fix (RFC §2.7).
- **KPI:** 3/3 pilot facilities can be assigned an `organization_id`; 0 existing
  rows rewritten; pre-existing facility-scoped tests still pass byte-for-byte
  (`admin_scope_matrix_test.go`, `rbac_resolver_test.go`,
  `facility_scope_test.go`).
- **Acceptance criteria:**
  - Migrations are forward-only and additive; `facilities` rows without an org
    still function.
  - `facilityScopeQuery` behavior is unchanged for pre-existing grants.
  - `regional_sysadmin` with `facility_id IS NULL AND organization_id IS NULL`
    resolves as regional; a facility- or org-scoped `regional_sysadmin` is
    **narrowed, not global** (mirrors the `super_admin` scoping rule, RFC §2.3).
  - SATUSEHAT sandbox credential gates (interop plan §14) are filed as blocker
    items; none are fabricated as resolved.

### Phase 1 — Organization onboarding

- **Work:** create the `regions` row (Kabupaten Pilotsari placeholder) and one
  `organizations` row ("Dinas Kesehatan Pilotsari — placeholder"); grant
  `reg-admin` as `regional_sysadmin` scoped to the region; land the audit
  facility-attribution fix so regional `audit.read` can scope by region.
- **KPI:** 1 region; 1 org; 1 `regional_sysadmin`; 100% of onboarding mutations
  written to `audit_events` with `FacilityID` (or `NULL` legitimately for
  region-scope actions) set correctly.
- **Acceptance criteria:**
  - Onboarding a region/org reads **no patient data** and **no `medical_records`**.
  - Regional `audit.read` returns only the pilot region's rows; a sibling region
    (if any) is invisible.
  - `audit.sanitizeEvent` PII/PHI-forbidden invariant holds on every new row.

### Phase 2 — Facility provisioning + distinct administrators

- **Work:** onboard F-1…F-3 via the org-scoped admin path: create each
  `facilities` row, set `organization_id`, seed `service_units` and
  `practitioners`/`practitioner_schedules`, and grant the **distinct**
  facility-scoped `facility_admin` per facility (§2.3) — a different principal
  for each, so isolation is observable.
- **KPI:** 3 facilities created with correct `type`/`organization_id`; 3 distinct
  `facility_admin` principals (one per facility); ≥1 service unit per facility;
  0 patient-data reads.
- **Acceptance criteria:**
  - Onboarding a facility creates the facility + its own scoped `facility_admin`
    atomically, with no hard delete of any clinical row.
  - `admin-f1` can manage F-1 and **cannot** manage F-2/F-3; same for `admin-f2`
    and `admin-f3` (§5.2 authorization isolation).
  - `DeactivateFacility` soft-disables the facility subtree
    (`facilities.is_active=false` + service_units/schedules) rather than deleting
    (RFC §2.9).
  - Offboarding an operator soft-deletes the `user_roles` row
    (`status='inactive'`, `deleted_at=NOW()`) and the actor is excluded on the
    **next request** via `ActorWithLiveGrants` (RFC §2.9).

### Phase 3 — Regional directory + cross-facility service discovery

- **Work:** extend the public catalog (`internal/handler/catalog.go`,
  `GET /api/v1/public/facilities`) to return a non-sensitive
  `region`/`organization` grouping key, and give org/facility admins a
  scope-aware admin list (RFC §2.5).
- **KPI:** 100% of pilot facilities returned with correct region/org grouping;
  0 PHI/PII fields in the directory payload; 0 leaked facilities outside an
  actor's scope.
- **Acceptance criteria:**
  - Public catalog lists F-1…F-3 with group keys; no `patient_phone`, no
    `patient_display_name`, no contact beyond what the catalog already exposes.
  - **Cross-facility discovery:** `referral-f1` (org-scoped) can list every
    pilot facility and its service units as candidates for referral, while
    `admin-f2` sees only F-2.
  - A facility-scoped actor with an empty read set receives `200 []`, not an
    error (row existence never leaks; RFC §1.4 / threat model §4.7).
  - Directory is a projection over `facilities` + `organizations` + `regions`;
    no new store is introduced.

### Phase 4 — Facility-scoped booking + queues, plus cross-facility where authorized

- **Work:** run synthetic bookings/queues at each facility through the existing
  public surface; then add the explicit org-referral grant shape and enforce
  B's-org-in-scope **and** provenance-at-B before any cross-facility
  `appointments` write (RFC §2.8).
- **KPI:** 1 referral desk operator can create a booking at a sibling facility
  inside the org; 0 cross-facility writes succeed for a facility-scoped
  operator; 0 citizen self-service writes cross facility boundaries.
- **Acceptance criteria (§5.1 booking/queues, §5.2 isolation):**
  - A synthetic citizen books at F-2 via `POST /api/v1/appointments`, checks in,
    and receives a queue ticket scoped to F-2; the F-2 board shows it, the F-1
    board does not.
  - `org_referral_coordinator` (org-scoped) creates an `appointments` row at F-3
    from an F-1 context; the row carries `facility_id = F-3` and the correct org.
  - A facility-scoped `operator` at F-1 **cannot** create at F-2 even though both
    are in the same org (denied, fail-closed, `404` not `403` to avoid existence
    leaks).
  - Citizen self-service (`POST /api/v1/appointments`) stays facility-scoped and
    code-keyed; the check-in-code bearer model is unchanged.
  - Cross-facility writes that succeed are audited with `FacilityID` set.

### Phase 5 — Service-day correctness + regional operational visibility

- **Work:** drive the synthetic `23:59 WIB → 00:01 WIB` boundary (§5.4) and
  confirm the `?date=` board and today-default are WIB-consistent; then assert the
  regional operational vs clinical boundary (§5.5).
- **KPI:** 0 tickets assigned to the wrong WIB service-day; 0 unauthorized
  clinical fields returned to any operational or regional principal.
- **Acceptance criteria (§5.4, §5.5).**

### Phase 6 — SATUSEHAT interop (sandbox only) — BLOCKED on credentials

Identical to the current sandbox-only phase; moved under §6 roadmap (ID-2) but
retained here as a separately-gated, non-pilot-gate workstream.

- **Work:** build the FHIR R4 mapping façade (`Patient`/`Organization`/
  `Location`/`Appointment`/`Encounter`) behind an adapter, post to the SATUSEHAT
  **sandbox** endpoint only, using the published dummy-patient table (interop plan
  §3, §7, §14).
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
    proceed without the owner decisions in §6.

---

### 5.1 Demonstration — facility-scoped booking and queues

What must hold, end-to-end, and be verifiable from the board API:

- A synthetic visit booked at F-2 produces an `appointments` row with
  `facility_id = F-2`; the board (`GET /api/v1/admin/queues`) scoped to F-2
  contains it, F-1's board does not. This is enforced by the **IMPLEMENTED**
  provenance machinery (`AuthorizeFacilityRead`; `404`/empty on out-of-scope).
- Queue-number minting stays per-facility (`daily_queue_counters.facility_id`);
  a walk-in at F-1 and a walk-in at F-2 can share a *formatted* number but are
  distinguishable only by their own facility's board — never cross-listed.
- `DAILY_QUEUE_LIMIT` remains a shared constant today (**IMPLEMENTED**); the
  per-facility limit (`facilities.daily_queue_limit`) is **PROPOSED** (RFC §2.6,
  capability N6) and is a sub-scope, not a gate.

### 5.2 Demonstration — authorization isolation

A read/write matrix the pilot must reproduce with the distinct principals, showing
fail-closed behavior as `404`/empty (never `403` with detail):

| Actor | See F-1 queue | See F-2 queue | Book at F-2 | Book at F-3 | Read `medical_records` |
|---|---|---|---|---|---|
| `admin-f1` | ✅ | ❌ | ❌ | ❌ | ❌ (no endpoint) |
| `admin-f2` | ❌ | ✅ | ❌ | ❌ | ❌ |
| `op-f1` | ✅ (`queue.read`) | ❌ | ❌ | ❌ | ❌ |
| `reg-admin` (PROPOSED) | ✅ (all facilities) | ✅ | ❌ (admin-only) | ❌ | ❌ |
| `referral-f1` (PROPOSED) | scoped org | scoped org | ✅ (provenance at target) | ✅ (provenance at target) | ❌ |
| unauthenticated citizen | ❌ | ❌ | facility-scoped only | ❌ | ❌ |

The "viewer at A + operator at B ⇒ operator at A" Cartesian-widening escalation
remains closed (RFC §1.4), now under the org axis too.

### 5.3 Demonstration — regional operational visibility WITHOUT clinical info

`reg-admin` sees **operations, not patients**:

- `reg-admin` can read every facility's queue board, appointment list, service
  units, schedules, and the region-scoped audit log (**PROPOSED**).
- `reg-admin` gets **no** `patient_phone`, no `patient_display_name` beyond the
  operational minimum the board already exposes, and no clinical content — the
  directory and board payloads are PII/PHI-sanitized (RFC §2.5; audit
  `sanitizeEvent`).
- `reg-admin` cannot read `medical_records` because **no reader endpoint and no
  permission key exist** (RFC §3). This is the single most important isolation
  assertion in the pilot.

### 5.4 Demonstration — accurate service-day handling

- The default board read with no `?date=` resolves to **today in WIB**
  (**IMPLEMENTED** — `resolveServiceDay` / `serviceDayWIB` in `admin.go`).
- A past `?date=YYYY-MM-DD` returns only that WIB calendar day's tickets; the web
  proxy forwards `?date=` unchanged (**IMPLEMENTED**).
- The synthetic boundary test: a ticket whose `registered_at` is `23:59 WIB` lands
  in that day's bucket, one at `00:01 WIB` lands in the next; no ticket is
  double- or mis-bucketed.
- Prior-day carry-over ("may an unresolved prior-day ticket still be acted on from
  the historical view?") remains **POLICY-DECISION-REQUIRED** (service-day §5;
  backlog PD5) and is out of the pilot's correctness scope.

---

## 6. Phased roadmap — patient identity + FHIR interoperability

**DESIGN-ONLY.** This roadmap is the deferred, dependency-ordered path for
patient identity and SATUSEHAT/FHIR interop. It is **not** a pilot gate (§8
go/no-go excludes it); it runs only after the operational pilot is green and each
policy gate is recorded. Mapping to the backlog is noted per phase.

### ID-1 — Verified patient identity + consent substrate (backlog Phase 7)

- **[PROPOSED]** Introduce `patients` + `patient_facility` membership; membership
  is created only by an explicit, consent-gated action — never inferred from
  "once booked here" (threat §4.4).
- **[PROPOSED]** A verified-identity proof step (NIK-backed) for any
  record-bearing identity, distinct from the phone-only visit path (threat §4.1).
- **[PROPOSED]** Purpose-bound consent ledger (patient id, purpose, granted/
  revoked at, grantor, evidence); revocation effective at the next processing step.
- **[POLICY-DECISION-REQUIRED]** Whether NIK is the canonical record key and the
  proof level for remote vs in-person enrollment (threat §4.1; backlog PD6).
- **[POLICY-DECISION-REQUIRED]** The approved consent purposes and opt-in/opt-out,
  plus retention after revocation (threat §4.5; backlog PD7).
- **Gate:** no record-bearing path accepts the 6-char code or walk-in token as an
  identity; a record read requires verified identity + consent + facility-bound
  care relationship, fail-closed.

### ID-2 — SATUSEHAT onboarding + master-data sync (backlog Phase 8)

- **[PROPOSED]** Bridge module (`apps/api/internal/satusehat/`): OAuth2
  `client_credentials` bound to exactly one Organization ID; refuse any request
  targeting a different org.
- **[PROPOSED]** Onboarding run (S4 order): authenticate → register
  `Organization` → `Location` → obtain `Practitioner` IHS → store ids back on
  `facilities`/`service_units`/`practitioners`.
- **[POLICY-DECISION-REQUIRED]** Partner registration + credential issuance
  (Kemenkes side; not self-serve) — interop §14 gates 1–3.
- **[POLICY-DECISION-REQUIRED]** Confirm the FHIR R4 base URL from the api-catalogue
  (interop D2).
- **Gate:** sandbox credentials exist; **no clinical data flows** in this phase;
  every onboarding mutation is audit-logged with `FacilityID` set.

### ID-3 — SATUSEHAT clinical FHIR exchange (backlog Phase 9)

- **[PROPOSED]** Patient identity mapping `patient_identity_mappings(patient_id,
  nik, ihs_number, satusehat_patient_ref, source, resolved_at)`; resolve IHS by
  NIK MPI search, never derive from phone alone (interop §4).
- **[PROPOSED]** `Encounter` anchored by `appointments`/`queue_tickets`, plus
  `Condition`/`Observation`/history resources mapped from a **new** clinical layer
  (all PROPOSED).
- **[PROPOSED]** `satusehat_outbox` (pending → submitted → acknowledged | failed |
  dead_letter) with idempotency keys and versioned correction (PUT with original
  `id`).
- **[PROPOSED]** Field-level encryption at rest (KMS) for cached NIK/clinical
  fields (interop §13).
- **[POLICY-DECISION-REQUIRED]** NIK collection purpose/retention (interop D1),
  consent UX (deep-link out vs 6-digit code; safe default = deep-link out),
  KMS ownership, and first use-case module (interop D3–D5).
- **Gate:** ID-1 + ID-2 complete; a SATUSEHAT fact is a fact only after an
  acknowledged push; rejected writes surface a human-correctable error, never a
  fabricated "synced" state.

---

## 7. Explicit prerequisites (before Phase 0 can close)

Each is a gate, not a self-serve task. None may be assumed resolved.

| # | Prerequisite | Status | Notes |
|---|---|---|---|
| P1 | **`organizations`/`regions` tables + `facilities.organization_id`** | PROPOSED (RFC Phase 1, migration `0012`) | No `organizations` table exists anywhere today (RFC §1.2). |
| P2 | **`regional_sysadmin` role + org-scope auth resolution** | PROPOSED (RFC Phase 2, migration `0013`) | The only current global reach is `super_admin`; org-scoped "every facility in this org" does not exist. |
| P3 | **`user_roles.organization_id` + partial indexes** | PROPOSED (RFC Phase 2) | Preserve existing `(user_id, role_id)` semantics. |
| P4 | **Per-facility timezone (`facilities.timezone`)** | PROPOSED (service-day policy §2.3) | `facilities` has no timezone column; WIB default suffices for this WIB-only pilot, but the seam must exist before any WITA/WIT or multi-zone expansion. |
| P5 | **Tenant-aware audit + facility attribution fix** | PROPOSED (RFC Phase 3; capability N5) | Admin rows currently do not set `Event.FacilityID`, so regional audit scoping needs the data fix first. |
| P6 | **SATUSEHAT sandbox credentials** | **NOT available** (interop plan §14) | External gates: partner registration, SSP verification, client credential issuance, fasyankes data update, dummy-patient table, Postman collection, owner sign-off. |
| P7 | **Owner policy decisions** (NIK collection/retention, consent purpose/opted-in set, record-read scope) | POLICY-DECISION-REQUIRED | Blocks interop patient-identity mapping (interop D1–D5) and any `medical_record.read` (RFC §3, capability F5). |

---

## 8. Risks and mitigations

| # | Risk | Class | Mitigation |
|---|---|---|---|
| R1 | **Scope-creep into medical records.** A pilot that "onboards a region" tempts someone to also expose `medical_records`. | Boundary | `regional_sysadmin` is administrative-only; no `medical_record.read` permission is added; `medical_records` keeps no reader endpoint (RFC §3). |
| R2 | **Cross-facility privilege widening.** Flat-permission union lets "operator at A" reach B. | Security | Provenance-at-B check (`AuthorizeFacilityMutation/Read`); org-scoped referral is explicit and denied by default (RFC §2.8; threat model §4.6). |
| R3 | **Timezone drift across facilities.** A WITA/WIT facility read against WIB yields wrong service-day. | Correctness | Pilot is single-WIB; per-facility `timezone` column is a recorded prerequisite (P4), never silently ignored. |
| R4 | **Synthetic data mistaken for real.** Demo visits/patients read as real citizens. | Integrity | §2.5 labels every synthetic patient/visit `[synthetic]`; none resolves to a real number; excluded from public-facing screenshots. |
| R5 | **SATUSEHAT sandbox contact with production data.** Mixing real NIK/clinical data into the sandbox PoC. | Compliance/privacy | Sandbox endpoints only; dummy patients only; no production credential (interop §14 non-goals). |
| R6 | **Fabricated "integrated" claims.** Reporting sandbox or proposed work as live. | Integrity | Every section tags IMPLEMENTED / DESIGN-ONLY (PROPOSED / POLICY-DECISION-REQUIRED); SATUSEHAT stays "sandbox-only, blocked on credentials." |
| R7 | **Audit rows unattributed.** Regional `audit.read` cannot scope without `FacilityID`. | Accountability | P5 fix lands in Phase 0; new rows always set `FacilityID`. |
| R8 | **Silent capacity model mis-sale.** A hospital and puskesmas sharing `DAILY_QUEUE_LIMIT = 300` reads as a bug. | Product | Optional per-facility limit sub-scope (RFC §2.6, capability N6); not a pilot gate. |
| R9 | **Row-existence oracle.** A denied read leaks that a sibling facility exists. | Security | `404`/`200 []` fail-closed, never `403` with detail (RFC §1.4). |

---

## 9. Deliberately OUT of pilot scope

- **No medical-record reader endpoint.** `medical_records` stays closed to every
  role, including `regional_sysadmin` (RFC §3; capability F5).
- **No citizen accounts or appointment hub.** Citizen surface stays code-keyed;
  Supabase/OIDC citizen identity is out (cap F1).
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
  scope, and only in sandbox (§6, ID-2/ID-3).
- **No wait-time model** beyond what exists; `estimated_wait_minutes` remains a
  constant and is not surfaced as a fabricated number (capability F7).

---

## 10. Go / no-go — success criteria

The pilot is **go** only if **all four** hold after Phases 0–5. The patient
identity + FHIR roadmap (§6) and sandbox interop are excluded from go/no-go
because their credentials and policy decisions are blocked.

1. **One region, three facilities, one directory.** `reg-admin`
   (`regional_sysadmin`) onboards the "Dinas Kesehatan Pilotsari" placeholder org
   and F-1…F-3, each with its own **distinct** `facility_admin` principal, and the
   public catalog returns all three with correct region/org grouping and **zero**
   patient data — with no existing row rewritten and pre-existing facility-scope
   tests green.
2. **Facility-scoped booking and queues are correct.** A synthetic visit booked at
   F-2 stays on F-2's queue and never appears on F-1/F-3; queue numbering is
   per-facility; the WIB service-day boundary (`23:59 → 00:01`) buckets tickets
   correctly.
3. **Authorization isolation holds.** `admin-f1` cannot read or book F-2/F-3; a
   facility-scoped `op-f1` cannot cross-book; `org_referral_coordinator` books a
   sibling facility only with provenance at the target; citizen self-service stays
   facility-scoped and code-keyed — all audited with `FacilityID` set.
4. **Regional operational visibility carries no clinical info.** `reg-admin`
   (`regional_sysadmin`) sees operations and the region-scoped audit log, and
   `reg-auditor` reads only the pilot region's `audit_events`; **no** administrative
   scope can read `medical_records`, because no reader endpoint and no clinical
   permission key exist.

If any criterion fails, the pilot is **no-go**: the failure is recorded against
the specific phase gate, the relevant prerequisite (P1–P7) is re-opened, and no
follow-on region is onboarded.