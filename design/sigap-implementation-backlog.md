# SIGAP v2 — Implementation Backlog (single ordered plan)

> **Status:** Synthesis / planning document. This file merges eight design documents into
> **one dependency-ordered backlog** so engineering and product can plan from a single
> source. It introduces **no new capability**: every item and its status is inherited from
> the source documents listed below. Read a source before contesting an item.

**Sources consumed (read, not rewritten):**

| Doc | Subject |
|---|---|
| `regional-architecture-rfc.md` | Tenancy, facility scope, org boundaries, regional RBAC |
| `satusehat-interoperability-plan.md` | SATUSEHAT/FHIR integration (ROADMAP Phase 8) |
| `identity-access-consent-threat-model.md` | Identity, access, consent, break-glass |
| `blockchain-necessity-matrix.md` | Audit-log integrity; blockchain recommendation = **No** |
| `product-positioning.md` | SIGAP vs SATUSEHAT ownership boundary |
| `service-day-queue-policy.md` | Service-day date scoping, WIB, carry-over |
| `sigap-v2-capability-matrix.md` | Gap analysis Q1–Q8 / N1–N9 / F1–F7 |
| `sigap-v2-product-evolution.md` | Implemented vs backend-work vs policy-decision classes |

**Status legend (authoritative, inherited verbatim from the sources):**

- **IMPLEMENTED** — live in the current branch; enforcement point cited.
- **PROPOSED** — designed, not built; requires backend/DB/UI work.
- **POLICY-DECISION-REQUIRED** — designed, not built; a blocking owner/legal decision must be
  recorded first.

**Ordering rule.** Phases are ordered so every dependency is satisfied before its dependents
run (tenancy before regional permissions; SATUSEHAT onboarding before FHIR sync; service-day
policy before historical queue UI; verified patient identity before record access). A phase
may *start* before a later phase *finishes* only when its stated dependencies are already met.

---

## Phase 0 — Shipped baseline (status: IMPLEMENTED)

**Goal.** Establish the honest "as-is" floor the backlog builds on. Nothing here is forward work;
it exists today and must not regress.

**Work items (all `[IMPLEMENTED]`):**

- `[IMPLEMENTED]` Flat facility-first schema: `facilities`, `service_units`, `practitioners`,
  `practitioner_schedules`, `appointments`, `queue_tickets`, `daily_queue_counters`,
  `medical_records`, `app_users`, `roles`, `permissions`, `user_roles`, `role_permissions`,
  `audit_events`, `notification_*` (migrations `0001`–`0011`). Deepest hierarchy is
  `facility → service_unit`.
- `[IMPLEMENTED]` RBAC: four seeded roles (`super_admin`, `facility_admin`, `operator`,
  `viewer`), twelve permission keys, `user_roles.facility_id` facility scoping, `0008` JWT
  `sub` mapping, `0009` soft-delete role lifecycle.
- `[IMPLEMENTED]` Facility-scoped authorization: `FacilityGrant`, `HasPermissionAtFacility`,
  `AuthorizeFacilityMutation`/`AuthorizeFacilityRead`, `AuthorizedFacilityIDsForPermission`,
  live `FacilityGrantResolver`, deny-by-default routing, `404`-not-`403` row-oracle defence.
- `[IMPLEMENTED]` Append-only tamper-evident audit: SHA-256 hash chain, PII/PHI forbid-list
  sanitizer, hashed IP/user-agent.
- `[IMPLEMENTED]` Citizen loop end-to-end: public catalog (`GET /api/v1/public/facilities`,
  `/service-units`), booking (`POST /api/v1/appointments`), 6-char 32-symbol check-in code with
  confusable-tolerant lookup, check-in → gRPC queue ticket, walk-in queue generation.
- `[IMPLEMENTED]` v2.3 additions: citizen cancellation (`POST /api/v1/appointments/cancel`),
  staff check-in that mints a queue ticket atomically, `queue_formatted_number` on status.
- `[IMPLEMENTED]` Service-day WIB default and additive `?date=` query parameter in
  `resolveServiceDay`/`serviceDayWIB` (`admin.go`); Ringkasan inherits today-scope.
- `[IMPLEMENTED]` Notification outbox pattern (schema-valid channels; only `DevProvider`
  delivers today).

**Acceptance criteria.** Already met — the book → check-in → queue loop and multi-facility
operational surface described in `product-positioning.md` §6. Re-checked on every later change:
no regression to the facility-scope security model (`auth/*_test.go`,
`handler/admin_scope_matrix_test.go`) and no silent widening of the check-in-code invariant.

**Dependencies.** None.

---

## Phase 1 — Org/region tenancy schema (additive)

**Goal.** Introduce the `regions → organizations → facilities` hierarchy as **new tables and
nullable columns only**, with zero data rewrite, so later regional authorization and
tenant-aware audit have a home without disturbing today's facility-first model.

**Work items:**

- `[IMPLEMENTED]` Existing `facilities`, `service_units`, `user_roles` remain the row-owners;
  no `organizations`/`regions` table exists yet (RFC §1.2).
- `[PROPOSED]` Migration `0012_organizations.sql`: create `regions`, `organizations`, and
  `ALTER TABLE facilities ADD COLUMN organization_id` (nullable). RFC §2.2.
- `[PROPOSED]` Migration `0013_regional_rbac.sql`: `ALTER TABLE user_roles ADD COLUMN
  organization_id` (nullable) + partial indexes preserving `(user_id, role_id)` semantics. RFC §2.10.
- `[PROPOSED]` Keep `service_units.facility_id` as the only unit link (no service_unit→org edge).

**Dependencies.** None (mirrors `0008`/`0009` additive discipline; never touches `0001`–`0007`).

**Acceptance criteria.** Migrations are forward-only; `facilities` rows without an
`organization_id` still function; `facilityScopeQuery` output is byte-for-byte unchanged for all
pre-existing grants; no existing row rewritten; `service_units` edges unchanged.

**Blocks.** Phases 2, 3, 4, 8, 11, 12.

---

## Phase 2 — Regional & org-scoped authorization

**Goal.** Extend the *existing* provenance machinery (do not invent a second system) so an
actor may be scoped to an organization or a region, with the same "scoping narrows, never
widens" rule already proven for `super_admin`.

**Work items:**

- `[IMPLEMENTED]` `FacilityGrant`/`FacilityScopeResult`/`AuthorizeFacilityMutation`/
  `AuthorizeFacilityRead` and the `(facility_id IS NULL AND role='super_admin')` predicate are
  live (RFC §1.3–1.4).
- `[PROPOSED]` Extend `FacilityGrant` with an org axis; make
  `AuthorizeFacilityMutation`/`AuthorizeFacilityRead` honor "facility in scope via org **or**
  facility grant **and** permission held at that org-or-facility". RFC §2.3.
- `[PROPOSED]` Add a `regional_sysadmin` predicate mirroring `super_admin`
  (`organization_id IS NULL AND facility_id IS NULL`); an org-scoped `regional_sysadmin` is
  org-scoped, not regional. RFC §2.3.
- `[POLICY-DECISION-REQUIRED]` Whether a `regional_sysadmin` role is adopted at all, and its
  exact administrative scope (RFC §2.1) — the engineering is specified; the role's existence is
  an owner call.

**Dependencies.** Phase 1.

**Acceptance criteria.** Existing facility-scope tests pass unchanged; new tests prove an
org-scoped grant reaches every facility of that org and **none** outside; a facility-scoped
`regional_sysadmin` narrows (does not widen); the "viewer at A + operator at B" Cartesian-widening
escalation remains closed under the org axis.

**Blocks.** Phases 4 (org defaults), 10 (care-relationship on top of scope), 11 (cross-facility),
12 (directory scope).

---

## Phase 3 — Tenant-aware audit + facility-attribution fix + hash-chain hardening

**Goal.** Close the audit gaps that block accountability and later interop: admin rows must be
facility-attributed, the log must be readable scoped by org/region, and the hash chain must
survive concurrent writers.

**Work items:**

- `[IMPLEMENTED]` Append-only hash-chained `audit_events` with sanitized metadata (RFC §1.5;
  blockchain matrix §2).
- `[PROPOSED]` Data fix: make admin write paths set `Event.FacilityID` (today `admin.go` never
  sets it — cap matrix N5; RFC §2.7 / §1.5 "known limitation").
- `[PROPOSED]` Migration `0014_tenant_audit.sql`: add `audit_events.organization_id`/`region_id`
  (nullable, `SET NULL`). RFC §2.7 / §2.10.
- `[PROPOSED]` Facility-scoped `GET /api/v1/admin/audit` read (`audit.read` stays
  `super_admin`-only; proposal adds a `regional_auditor`/`regional_sysadmin` reader). Cap N5.
- `[PROPOSED]` Harden the hash chain (blockchain matrix, Option A): close the concurrent-write
  `previous_hash` race so the chain has no gaps.
- `[POLICY-DECISION-REQUIRED]` Audit retention period (durations + Indonesian regulatory
  minimum). Blockchain matrix §10.1.
- `[POLICY-DECISION-REQUIRED]` Correction/erasure framework for an audit row under a
  data-subject right, and how the chain is re-anchored after correction. Blockchain matrix §10.2.

**Dependencies.** Phase 1 (columns); Phase 2 recommended (org/region read scope).

**Acceptance criteria.** `audit_events` rows carry facility + org; a regional `audit.read`
returns only that region's rows; `audit.sanitizeEvent` PII-free invariant holds; the hash chain
verifies with no gaps under concurrent writers; every `audit.read` is itself an audit event.

**Escalation (gated, not a default).** `[PROPOSED]` Option B — WORM + periodic external
commitments — is pursued **only** if one of the blockchain matrix §6 triggers is cited as an
explicit requirement. The recommendation is **No blockchain**; adopt/harden Option A.
`[POLICY-DECISION-REQUIRED]` whether operator-independent tamper-evidence is a legal necessity
(blockchain matrix §10.3) is the single gate for Option B.

---

## Phase 4 — Per-facility queue + check-in policy

**Goal.** Replace the global `DAILY_QUEUE_LIMIT = 300` constant and the missing check-in window
with per-facility configuration, so one busy hospital and one quiet puskesmas no longer share
policy.

**Work items:**

- `[IMPLEMENTED]` `DAILY_QUEUE_LIMIT = 300` constant in `apps/queue-engine/src/engine/queue.rs`
  (cap matrix N6); check-in validates code + `status='scheduled'` only (product-evol §2.1).
- `[PROPOSED]` Migration `0015_facility_policy_columns.sql`: `facilities.daily_queue_limit`,
  `facilities.checkin_opens_minutes` (`DEFAULT 120`),
  `facilities.checkin_closes_minutes` (`DEFAULT 240`) — nullable-free defaults. RFC §2.6/§2.10;
  product-evol §2.1.
- `[PROPOSED]` Thread `daily_queue_limit` into the Rust queue engine and the check-in handler;
  enforce the check-in window (`409` "too early" / `409` "too late"). RFC §2.6/§2.10;
  product-evol §2.1.
- `[PROPOSED]` Registry-backed per-organization defaults that a facility may override (RFC §2.6).

**Dependencies.** Phase 1 (columns); Phase 2 (org-level defaults).

**Acceptance criteria.** Two facilities with different limits produce different daily caps;
a booking 3 days out is refused "too early" with a timestamp, 30 minutes out is accepted, 5
hours past is refused "too late"; `DAILY_QUEUE_LIMIT` is no longer the sole source of truth;
existing happy-path check-in tests keep passing.

---

## Phase 5 — Service-day scheduling correctness (policy → historical UI)

**Goal.** Make "today" unambiguous across the queue board and Ringkasan, and give operators a
read-only historical view — while forcing an owner decision only on the one genuinely open
question (carry-over).

**Work items (policy-first, then UI):**

- `[IMPLEMENTED]` WIB single-zone default and additive `?date=` param already resolve
  `[start, start+24h)` in `admin.go`; Ringkasan, board and overview share one "today". Service-day
  doc §1–§3, §7.
- `[IMPLEMENTED]` Number-allocation / cap / guard unification onto the WIB day (closes the
  UTC-vs-WIB divergence): Rust `service_day_wib` (`daily_queue_counters` key +
  `DAILY_QUEUE_LIMIT`), Go `serviceDayRange(time.Now())` fallback allocator range, and additive
  `0012_service_day_wib.sql` redefining `appointment_day` to a genuinely immutable WIB day with a
  collision-guarded reindex. No historical instant rewritten. Service-day doc §8.
- `[PROPOSED]` Historical queue UI: operator picks a past date (`?date=YYYY-MM-DD`); read-only,
  no status filter, stable ordering/scope unchanged. Service-day doc §4.
- `[POLICY-DECISION-REQUIRED]` Whether an unresolved prior-day ticket may still be acted on
  (called/served) from a historical view, or requires an explicit operator override. Service-day
  doc §5; also the per-facility `timezone` column remains a future additive seam (§2.3,
  `[PROPOSED]`).

**Dependencies.** None (builds on the implemented backend contract; UI wiring depends on the
implemented `date` param).

**Acceptance criteria.** No-`date` read = today in WIB everywhere; a past `date` returns only that
day's tickets; `PATCH /api/v1/admin/queues/{id}/status` is orthogonal and unchanged; a prior-day
open ticket is visible under its own day (never silently hidden, never silently closed).

---

## Phase 6 — Citizen front-door hardening (bounded backend)

**Goal.** Close the largest citizen-facing gaps with bounded, self-contained backend work that
does **not** introduce a new security model — walk-in status, reschedule, slot availability,
server-side queue filtering, and delivery visibility.

**Work items:**

- `[IMPLEMENTED]` Citizen cancellation (`POST /api/v1/appointments/cancel`) and staff check-in
  (v2.3) ship; `queue_formatted_number` returns on status.
- `[PROPOSED]` Walk-in status token: mint an opaque crypto-random token at walk-in issuance and
  accept it on the code-only rate-limited `/patient/status` path
  (`GET /api/v1/patient/queue-status?token=`); formatted number stays refused; expire at end of
  service day. Product-evol §2.2; cap matrix "walk-in status retrieval"; threat model §4.7 T7c.
- `[PROPOSED]` Citizen reschedule (`POST /api/v1/appointments/reschedule`) re-running
  slot-capacity and `uq_active_booking_per_patient_day` checks. Product-evol §2.3; cap N3.
- `[PROPOSED]` Slot availability endpoint (`GET /api/v1/public/schedules?facility_id=…&date=`)
  returning counts-only `{start, capacity, booked, available}`. Product-evol §2.4.
- `[PROPOSED]` Server-side queue filtering by facility (`admin.go` parses no query today — cap N8).
- `[PROPOSED]` Notification delivery visibility (`confirmation_sent: boolean`) on booking
  response, derived read-only from the outbox. Product-evol §2.5.
- `[PROPOSED]` Additive status fields (e.g. `service_unit_name` — cap N1) without re-adding
  `appointment_time` (deliberately withheld).

**Dependencies.** None hard; walk-in token and reschedule reuse the existing bearer-token and
rate-limiter model (30/min/IP).

**Acceptance criteria.** Fresh walk-in token resolves, completed token returns terminal state,
formatted number and random UUID return `404`, 30 req/min/IP; reschedule violating capacity or
daily-unique returns the same `409` as booking; slot at capacity reports `available: 0` and is
not offered while the booking path still enforces capacity server-side.

---

## Phase 7 — Verified patient identity + consent substrate

**Goal.** Introduce the *separate* record identity layer the threat model requires: a `patients`
table keyed by a verified subject, an explicit patient↔facility relationship, and a purpose-bound
consent ledger — none reachable via the visit token.

**Work items:**

- `[IMPLEMENTED]` Visit identity stays minimal: inline `patient_phone`/`patient_display_name` on
  `appointments`; no `patients` table today; check-in code is a visit token, not an identity.
  Threat model §4.1/§4.4.
- `[PROPOSED]` Introduce `patients` + `patient_facility` membership rows; membership created only
  by an explicit, consent-gated action (never inferred from "once booked here"). Threat §4.4.
- `[PROPOSED]` Verified-identity proof step for any record-bearing identity (NIK-backed), distinct
  from the phone-only visit path. Threat §4.1.
- `[PROPOSED]` Patient sessions reuse the existing JWT/OIDC provider (never the dev header). Threat §4.3.
- `[PROPOSED]` Purpose-bound **consent ledger** (patient id, purpose, granted/revoked at,
  grantor, evidence); revocation takes effect at the next processing step. Threat §4.5.
- `[POLICY-DECISION-REQUIRED]` Whether NIK (or other national identifier) is the canonical record
  key, and the acceptable proof level for remote vs in-person enrollment. Threat §4.1.
- `[POLICY-DECISION-REQUIRED]` The set of approved consent purposes, opt-in vs opt-out per
  purpose, and the retention window after revocation. Threat §4.5.
- `[POLICY-DECISION-REQUIRED]` Whether NIK breaks in-practice phone collision and how provider-side
  dedup works for a shared family phone. Threat §4.4.

**Dependencies.** Phase 2 (facility-bound auth substrate); Phase 6 (front-door stable first).

**Acceptance criteria.** No PROPOSED path binds medical history to a phone without a proof step;
the check-in/walk-in token are explicitly rejected as input to any record-bearing read; consent
rows are recorded (not assumed) before any secondary use; revocation is immediately effective.

**Blocks.** Phases 9 (SATUSEHAT patient IHS), 10 (record access).

---

## Phase 8 — SATUSEHAT onboarding + master-data sync

**Goal.** Stand up the credentialed bridge for the **onboarding** (not yet clinical) flow: token
auth, Organization/Location/Practitioner registration, and storing returned SATUSEHAT ids on
SIGAP rows. Gated by partner sign-off; no sandbox/production credential is used before the gates.

**Work items:**

- `[IMPLEMENTED]` None of the SATUSEHAT calls exist today; the live booking/queue surface is
  unchanged (satusehat plan §1/§7).
- `[PROPOSED]` Bridge module (`apps/api/internal/satusehat/`): OAuth2 `client_credentials` token
  refresh bound to exactly one Organization ID; refuse any request targeting a different org.
  Satusehat §7.
- `[PROPOSED]` Onboarding run (S4 order): authenticate → register `Organization` → register
  `Location` → obtain `Practitioner` IHS → store ids back on `facilities`/`service_units`/
  `practitioners`. Satusehat §5/§7.
- `[POLICY-DECISION-REQUIRED]` Partner registration + `client_id`/`client_secret` issuance
  (Kemenkes side; not self-serve). Satusehat §14, gates 1–3.
- `[POLICY-DECISION-REQUIRED]` Confirm the exact FHIR R4 base URL from the api-catalogue (do not
  hard-code an unverified path). Satusehat D2.
- `[POLICY-DECISION-REQUIRED]` Whether a region's SATUSEHAT integration is mandatory or optional
  (adapter assumes optional/disableable). Product-positioning §8.

**Dependencies.** Phase 1 (org/facility context to map); Phase 3 (facility-attributed audit so
interop audit is scoped); the eight PoC gates in satusehat §14 must be met.

**Acceptance criteria.** Token auth succeeds and refuses a mismatched Organization ID (business-rule
error); one SATUSEHAT Organization per SIGAP facility with the returned id stored on the row;
`Location`/`Practitioner` IHS obtained and stored; every onboarding mutation is audit-logged with
`FacilityID` set; **no clinical data flows in this phase**.

**Blocks.** Phase 9.

---

## Phase 9 — SATUSEHAT clinical FHIR exchange

**Goal.** Push operational encounters and their clinical content to SATUSEHAT as versioned FHIR
resources, with the patient resolved by IHS, an idempotent outbox, and KMS encryption — after
onboarding is live.

**Work items:**

- `[IMPLEMENTED]` Local `patients`/`appointments`/`queue_tickets` model; `medical_records` is a
  signed attendance record with **no** diagnosis/clinical content (cap F5; satusehat §5).
- `[PROPOSED]` Patient identity mapping `patient_identity_mappings(patient_id, nik, ihs_number,
  satusehat_patient_ref, source, resolved_at)`; resolve by NIK MPI search, cache IHS, never derive
  from phone alone; fall back to a human "link patient" step on duplicate risk. Satusehat §4.
- `[PROPOSED]` Encounter/clinical resource emission: `Encounter` anchored by
  `appointments`/`queue_tickets`, plus `Condition`/`Observation`/patient-history resources mapped
  from a new clinical layer (all new, PROPOSED). Satusehat §3/§5.
- `[PROPOSED]` `satusehat_outbox` (pending → submitted → acknowledged | failed | dead_letter),
  exponential backoff+retry for transient, no blind retry for 4xx, `Transaction`/`Batch` bundle per
  visit, idempotency key, versioned correction (PUT with original `id`). Satusehat §9/§10.
- `[PROPOSED]` Field-level encryption at rest (KMS) for cached NIK/clinical fields; key separation
  (TLS / client secret / field encryption). Satusehat §13.
- `[POLICY-DECISION-REQUIRED]` NIK collection purpose/retention for IHS resolution. Satusehat D1/§4.3.
- `[POLICY-DECISION-REQUIRED]` Consent UX: deep-link out to SATUSEHAT Mobile vs surfacing the
  6-digit code in SIGAP. Satusehat D3/§6; safe default = deep-link out.
- `[POLICY-DECISION-REQUIRED]` KMS key-management ownership. Satusehat D4/§13.
- `[POLICY-DECISION-REQUIRED]` Which Phase-8 use-case module to target first (RME Rawat Jalan vs
  IGD vs Kefarmasian). Satusehat D5.

**Dependencies.** Phase 7 (verified identity for NIK/IHS) and Phase 8 (onboarding + credential).

**Acceptance criteria.** A fact becomes a SATUSEHAT fact only after an acknowledged push; SIGAP
never re-issues an identity for a mapped IHS; corrections are PUT-with-original-id (never a new
id); rejected writes surface a human-correctable error and never a fabricated "synced" state.

---

## Phase 10 — Consent-gated clinical record access + patient access log

**Goal.** If the owner ever approves a medical-record read, it must require verified identity +
consent + facility-bound authorization (a care relationship), be audit-logged, and be visible to
the patient — never reachable via the visit token.

**Work items:**

- `[IMPLEMENTED]` `medical_records` has **no reader endpoint** (`GET /api/v1/medical-records`
  does not exist); `super_admin` is not record-open; SHA-256 `signature`. RFC §3; cap F5.
- `[PROPOSED]` Care-relationship predicate on top of facility scope for any full-record read.
  Threat §4.6.
- `[PROPOSED]` Break-glass: clinician action + reason, time-boxed, facility-scoped, high-severity
  audit event, mandatory post-hoc review. Threat §4.8.
- `[PROPOSED]` Patient-visible access log listing access events for the authenticated patient's
  own record (rate-limited, non-enumerable, via verified identity + consent, never the visit
  token). Threat §4.10.
- `[POLICY-DECISION-REQUIRED]` Whether any `medical_record.read` endpoint exists and who may hold
  it (clinician / treating-unit staff / facility admin — and under what conditions). RFC §3; cap F5.
- `[POLICY-DECISION-REQUIRED]` Break-glass grounds, review SLA, and interaction with the legal
  duty to verify identity as soon as practicable. Threat §4.8.

**Dependencies.** Phase 7 (verified identity + consent) and Phase 2 (scope substrate).

**Acceptance criteria.** A record read requires verified identity + consent/purpose row +
facility-bound care relationship and fails closed if any element is missing; the 6-char code /
walk-in token is rejected on this path; patient log answers "who, when, where" without leaking
other patients.

---

## Phase 11 — Cross-facility booking (org-gated)

**Goal.** Permit cross-facility scheduling only where an explicit org referral permission exists;
never as a silent widening of facility scope.

**Work items:**

- `[IMPLEMENTED]` Facility-scoped booking; citizen self-service stays facility-scoped
  (`POST /api/v1/appointments`). RFC §2.8.
- `[PROPOSED]` Add an org-level reference/referral permission shape; enforce "B's
  `organization_id` ∈ actor's authorized orgs **and** `appointment.manage`/`schedule.read` held
  with provenance at B" before any cross-facility `appointments` write. RFC §2.8.

**Dependencies.** Phase 2.

**Acceptance criteria.** An org referral role can create at a sibling facility; a facility-scoped
operator cannot (even inside the org); citizen self-service remains facility-scoped; default
cross-facility grant is **denied**.

---

## Phase 12 — Safe onboarding/offboarding + regional directory

**Goal.** Make adding/removing operators and facilities safe and org-aware, and expose a
region/org-grouped public directory without leaking PHI.

**Work items:**

- `[IMPLEMENTED]` Facility activation/deactivation (`DeactivateFacility`, `is_active=false`),
  create/update facilities/service-units/schedules; public catalog `catalog.go`. RFC §2.9; cap §1.
- `[PROPOSED]` Org-scoped facility onboarding: create facility + seed service units/practitioners
  + grant first scoped `facility_admin` (no patient-data read). RFC §2.9.
- `[PROPOSED]` Safe offboarding via `user_roles.status='inactive'`/`deleted_at`; org-scoped
  "deactivate subtree" soft-disabling a facility's service units/schedules (no hard deletes of
  clinical rows). RFC §2.9.
- `[PROPOSED]` Extend the public catalog to return a non-sensitive `region`/`organization`
  grouping key (projection, not a new store). RFC §2.5.

**Dependencies.** Phases 1–3.

**Acceptance criteria.** Onboarding never reads patient data; offboarding excludes the actor on
the next request (`ActorWithLiveGrants`); deactivation soft-disables the subtree; directory
grouping exposes no PHI; audit rows retain `actor_user_id` history via `SET NULL`.

---

## Phase 13 — Notification delivery, templates, and code recovery

**Goal.** Make the notification path *deliver* beyond `DevProvider`, let admins manage templates,
and give a citizen a recovery path for a lost code — each gated by a procurement/privacy decision.

**Work items:**

- `[IMPLEMENTED]` `notification_outbox` + `notification_templates` tables exist; only
  `DevProvider` ships, so sms/whatsapp/email are schema-valid but undeliverable. Cap F4.
- `[PROPOSED]` Notification template create/update route (lifecycle + validation). Cap N7.
- `[POLICY-DECISION-REQUIRED]` Choosing a real notification provider (procurement +
  data-processing agreement). Cap F4.
- `[POLICY-DECISION-REQUIRED]` Lost-code recovery (recommended: SMS re-send gated on exact phone
  match, 1/hour; creates a new enumeration surface). Product-evol §3.1.

**Dependencies.** Phase 6 (delivery visibility agreed); provider decision gates delivery work.

**Acceptance criteria.** A chosen provider delivers schema-valid channels; template CRUD is
audit-logged; any code-recovery path is rate-limited and never reveals whether a phone has an
appointment.

---

## Phase 14 — Audit-integrity escalation (Option B, gated horizon)

**Goal.** Hold a ready decision for operator-*independent* tamper-evidence; **do not build** until a
cited requirement triggers it.

**Work items:**

- `[IMPLEMENTED]` Option A hash-chained log (the default; see Phase 3 hardening).
- `[PROPOSED]` Option B — WORM write path + periodic external commitment/anchoring — as the only
  escalation, and only under the blockchain matrix §6 triggers.
- `[POLICY-DECISION-REQUIRED]` Operator-independent tamper-evidence as a legal necessity (the
  single gate; no evidence cited today → default **No blockchain**).

**Dependencies.** Phase 3 (hardened A) completed first.

**Acceptance criteria.** Not applicable until a trigger is cited; if triggered, anchor **batch
roots only** (never per-record commits), coarse the time domain, salt/key hashes, and never
commit NIK/name/phone/diagnosis/prescription/raw identifiers (§7 ceiling).

---

## Near-term executable slice (next two sprints)

Everything below is **unblocked** — no POLICY-DECISION-REQUIRED item gates any of it. Start these
three in parallel; they do not wait on each other.

| # | Item | Phase | Why now |
|---|---|---|---|
| 1 | **Tenancy migrations** `0012`+`0013` (regions, organizations, `facilities.organization_id`, `user_roles.organization_id`) | 1 | Additive, forward-only, zero data rewrite; dependency root for every regional capability. |
| 2 | **Admin audit facility-attribution fix** (`Event.FacilityID`) + **close the hash-chain concurrency race** | 3 | Unblocks facility-scoped audit read (cap N5) and is a precondition for SATUSEHAT interop audit. |
| 3 | **Walk-in status token + slot-availability endpoint** | 6 | Bounded, self-contained; closes two top citizen gaps with the already-written privacy contract. |

Suggested sprint split: **Sprint 1** = item 1 (schema only) + item 2 (attribution fix + race close
+ minimal `GET /api/v1/admin/audit` reader). **Sprint 2** = item 3 (walk-in token + slot
endpoint) + server-side queue filter (cap N8) + confirm Phase 5 historical-date UI wiring against
the implemented `date` contract.

## Long-horizon roadmap (dependency-ordered, post-slice)

1. **Phase 2** regional/org authorization (needs Phase 1).
2. **Phase 4** per-facility policy (needs Phases 1–2).
3. **Phase 5** historical UI + carry-over decision (policy first, UI after).
4. **Phase 6** remaining front-door hardening (reschedule, delivery visibility).
5. **Phase 7** verified identity + consent substrate (needs Phase 2/6).
6. **Phase 8** SATUSEHAT onboarding (needs Phases 1/3 + partner gates).
7. **Phase 9** SATUSEHAT clinical exchange (needs Phases 7/8).
8. **Phase 10** record access + patient log (needs Phases 7/2 + owner decision).
9. **Phase 11** cross-facility booking (needs Phase 2).
10. **Phase 12** onboarding/offboarding + directory (needs Phases 1–3).
11. **Phase 13** notification delivery + code recovery (provider decision).
12. **Phase 14** audit-integrity Option B (gated; not on a default path).

## Consolidated policy-decision log

The following `POLICY-DECISION-REQUIRED` items are blocking (or gating) and belong to an owner,
not to engineering. Each names its phase and the source.

| # | Decision | Phase | Source |
|---|---|---|---|
| PD1 | Adopt a `regional_sysadmin` role, and its exact administrative scope | 2 | regional RFC §2.1 |
| PD2 | Audit retention period (durations + Indonesian minimum) | 3 | blockchain §10.1 |
| PD3 | Audit correction/erasure framework + re-anchoring | 3 | blockchain §10.2 |
| PD4 | Operator-independent tamper-evidence as a legal necessity (Option B gate) | 3/14 | blockchain §10.3 |
| PD5 | Unresolved prior-day ticket: actionable from history, or operator override? | 5 | service-day §5 |
| PD6 | NIK as canonical record key + proof level (remote vs in-person) | 7 | threat §4.1 |
| PD7 | Approved consent purposes, opt-in/out, revocation retention | 7 | threat §4.5 |
| PD8 | NIK/phone dedup for shared family phone | 7 | threat §4.4 |
| PD9 | SATUSEHAT partner registration + credential issuance (Kemenkes) | 8 | satusehat §14 |
| PD10 | Confirm FHIR R4 base URL at integration time | 8 | satusehat D2 |
| PD11 | SATUSEHAT integration mandatory vs optional per region | 8 | positioning §8 |
| PD12 | NIK collection purpose/retention for IHS resolution | 9 | satusehat D1 |
| PD13 | Consent UX: deep-link out vs 6-digit code in SIGAP UI | 9 | satusehat D3 |
| PD14 | KMS key-management ownership for cached NIK/clinical fields | 9 | satusehat D4 |
| PD15 | First Phase-8 use-case module (Rawat Jalan / IGD / Kefarmasian) | 9 | satusehat D5 |
| PD16 | Whether any `medical_record.read` endpoint exists, and who holds it | 10 | regional RFC §3 / cap F5 |
| PD17 | Break-glass grounds, review SLA, identity-verification interplay | 10 | threat §4.8 |
| PD18 | Notification provider choice (procurement + DPA) | 13 | cap F4 |
| PD19 | Lost-code recovery (SMS re-send + enumeration tradeoff) | 13 | product-evol §3.1 |
| PD20 | Identity booking-handle (phone vs NIK) and when a national id is required | 7 | positioning §8 |

*End of backlog synthesis.*