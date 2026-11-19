# Patient Identity & Consent — Access-Control Contract

**Status:** DESIGN document (English) + a RUNNING, testable decision evaluator.
**Branch:** `feat/sigap-regional-pilot`.

This document is the consent / access-control contract for patient data: what a
grant *contains*, who may read a record, under what purpose and scope, for how
long, how it is revoked, how emergencies are handled, what gets audited, and how
a patient sees who looked at them. It reuses the honesty markers and the hard
negations of the existing design set verbatim; it does not re-derive them.

Sources of truth (read, not duplicated):

- `design/identity-access-consent-threat-model.md` — the security reasoning
  behind identity, access, and consent (§4.1 verified identity, §4.5 consent,
  §4.6 clinician authorization, §4.8 break-glass, §4.10 patient-visible logs).
- `design/regional-interop-contracts.md` — adapter contracts, honesty markers
  (`PRODUCTION-INTEGRATED` / `LOCAL-SYNTHETIC-CONTRACT` /
  `FUTURE-AUTHORIZED-SATUSEHAT`), and the negative invariants.
- `design/regional-architecture-rfc.md` — tenancy / scoping.

---

## 0. Honesty markers (read first)

Every item below is tagged exactly one of:

- **RUNNING** — enforced by code/schema in this branch, with a cited file or
  migration. Only items so tagged are live today.
- **DESIGN-ONLY** — designed, not built. Sub-labels **PROPOSED** (needs
  backend/DB work), **POLICY-DECISION-REQUIRED** (needs an owner decision), and
  **FUTURE-AUTHORIZED-SATUSEHAT** (needs Kemenkes credentials + partner
  registration) are all DESIGN-ONLY.

**Hard negations** (identical to the interop contracts §0, non-negotiable):

1. **No NIK storage.** The system never stores a national identifier (NIK),
   never derives one from a phone, and never exposes any identity mapping.
2. **No unrestricted patient search.** There is no free-text "find any patient"
   surface. Every record access is scoped to a subject the caller can already
   name and is authorized to reach.
3. **No identity-mapping endpoint.** No public or internal endpoint returns a
   phone→NIK, phone→IHS, or local→national correlation.
4. **Synthetic only.** Every fixture in this document and in the `consent`
   package is fictional, on the fictional `.pilotsari.example` /
   "Kabupaten Pilotsari" district.

---

## 1. The four identity layers (separated, never conflated)

Identity in SIGAP is **not** one field; it is four distinct layers with four
different readiness states. They are listed from most-ready to least-ready. The
invariant that governs all four is the threat model §1: **a visit token is never
a medical identity**, and each layer that reaches a record does so through its
own, strictly-separated path.

### Layer 1 — Authentication identity (`app_users`) — **RUNNING**

Who is this in the *application*? A user session, backed by the staff/admin
RBAC tables.

- **RUNNING (IMPLEMENTED).** JWT/OIDC `sub` is mapped to a server-side
  `app_users` row via the partial unique index `uq_app_users_subject_active`;
  the token proves only `sub`, never permissions. Authorization is resolved
  server-side from `app_users → user_roles → role_permissions → permissions`.
  (`packages/db/migrations/0003_identity_rbac.sql`,
  `packages/db/migrations/0008_identity_subject.sql`,
  `apps/api/internal/auth/rbac_resolver.go`, `apps/api/internal/auth/jwt_provider.go`.)
- This layer answers **"which staff member / admin is this?"** It is *not* a
  patient identity, and it must never be used as one.

### Layer 2 — Verified real-world patient identity — **NOT implemented (DESIGN-ONLY)**

Who is this *person*, in the real world, with proof? This is the **record-bearing
patient identity** that §4.1 of the threat model requires before a person gets
any right to their own records.

- **DESIGN-ONLY (PROPOSED / POLICY-DECISION-REQUIRED).** There is **no
  `patients` table, no NIK column, no verified-identity step** anywhere in the
  schema. Appointments carry only inline `patient_phone` + `patient_display_name`
  (data minimization), and that phone is a contact, **not** an identity.
  (`packages/db/migrations/0005_appointments.sql`.)
- The canonical record key, the proof level, and remote-vs-in-person enrollment
  are all owner decisions (threat model §4.1 "POLICY-DECISION-REQUIRED").
- **This layer is the only place a NIK *could* legally appear, and it is
  explicitly out of scope for this branch.** No code here reads, writes, or
  stores a NIK.

### Layer 3 — Facility-local medical-record identity — **NOT implemented (DESIGN-ONLY)**

Who is this patient *at a specific facility*, for clinical purposes? The
explicit patient↔facility relationship that §4.4 of the threat model says must
be created by a consent-gated action, never inferred from "once booked here".

- **DESIGN-ONLY (PROPOSED).** There is no `patients` table and no
  `patient_facility` membership row. Appointment rows do **not** join a patient
  identity, so there is no accidental cross-facility merge today — at the cost of
  no cross-visit linkage. (`packages/db/migrations/0005_appointments.sql`.)
- Any future `patients` record must require an explicit, consent-gated
  membership, and must be separate from appointment rows.

### Layer 4 — Future SATUSEHAT identity — **contract only (DESIGN-ONLY)**

Who is this patient in the *national* health platform (IHS / KYC)?

- **DESIGN-ONLY (FUTURE-AUTHORIZED-SATUSEHAT).** The `PatientIdentityMapper`
  contract in `design/regional-interop-contracts.md` §2.1 names the shape
  (`PatientIdentityMapping`, `Resolve`/`LookupLocal`/`Store`), but no SATUSEHAT
  client is instantiated, no credential exists, and every such path returns
  "credential missing" by construction. Blocked on interop plan §14 gate P6/P7.
- Its `LOCAL-SYNTHETIC-CONTRACT` tier is a fake that (a) versions rows instead
  of overwriting, (b) rejects phone-as-IHS-key inputs, (c) routes
  demographic-only hints to a human link — never an auto-merge. Exercisable
  against synthetic fixtures only.

**Net readiness, as of this branch:** Layer 1 is RUNNING; Layers 2, 3, and 4 are
DESIGN-ONLY. A 6-character check-in code remains a **visit bearer token** only
— it authenticates a visit, never any of these layers.

---

## 2. The consent record (contract)

A consent grant is the unit the evaluator in `apps/api/internal/consent` reasons
over. It is a purpose-bound, scoped, revocable, time-boxed record. The fields
below map 1:1 to the `Grant`/`Request`/`Decision` types in `policy.go`; the
ledger that *stores* them is DESIGN-ONLY (no table, column, or migration), so
`Grant` is a fully-populated in-memory value supplied by a caller.

| Field | Meaning | In `consent` | Status |
|---|---|---|---|
| **Patient** | record subject (synthetic local id) | `Scope.PatientID` | DESIGN-ONLY (no verified identity yet) |
| **Authorized actor** | the single professional allowed to act on the grant (synthetic local id) | `Grant.ActorID` ↔ `Request.ActorID` | DESIGN-ONLY (consent-level binding; still requires the upstream professional-authorization gate) |
| **Authorized facility** | where the grant applies | `Scope.FacilityID` | DESIGN-ONLY (no `patient_facility` row) |
| **Authorized organization** | the organization under which the grant applies | `Scope.OrganizationID` | DESIGN-ONLY |
| **Purpose** | the consented use | `Purpose` / `Request.ReadPurpose` | DESIGN-ONLY (no ledger yet) |
| **Resource category** | which class of record content the grant permits (e.g. `clinical_record`, `medication_history`, `billing`) | `Grant.ResourceCategory` ↔ `Request.ResourceCategory` | DESIGN-ONLY (empty = uncategorized = denies) |
| **Scope** | subject + facility + organization bounds | `Scope` | DESIGN-ONLY |
| **Validity period** | active window | `Validity.{NotBefore,NotAfter}` | DESIGN-ONLY |
| **Revocation** | withdrawn? when? | `Revocation` | DESIGN-ONLY |
| **Emergency policy** | may break-glass? mandatory review? | `EmergencyPolicy` | DESIGN-ONLY |
| **Audit event** | immutably records who read what | `Decision.Audit` (`AuditEvent`) → `audit_events` | RUNNING (append-only hash chain); consent decisions populate it (see §4) |
| **Patient-visible history** | the patient sees their own access log | *future* read path off `audit_events` | DESIGN-ONLY |

> **The clinician axis is the one RUNNING piece of the consent contract.** Staff
> authorization, including facility provenance (`FacilityGrant` +
> `AuthorizeFacilityRead`, which fails closed on any empty-scope or cross-facility
> grant), is already live. Consent is the *additional* patient-side gate layered
> on top of that clinician gate: a clinician who is authorized at a facility still
> needs a purpose-bound, non-revoked, in-scope grant to read a specific record.
> (`apps/api/internal/auth/facility_scope.go`, `apps/api/internal/identity/actor.go`.)

---

## 3. The access-decision evaluator (RUNNING, synthetic-only)

`apps/api/internal/consent/policy.go` is the RUNNING, testable slice of this
contract. It is pure Go — no database, no HTTP, no I/O — and it is
deny-by-default.

**Decision inputs** (what `Evaluate(req, grant, at)` considers, in order):

1. **Subject/purpose/resource presence** — an empty `grant.Purpose`, an empty
   `grant.Scope.PatientID`, or an empty `grant.ResourceCategory` is not a
   grant; it denies immediately, so a zero/corrupt record can never authorize.
2. **Revocation** — a revoked grant denies even under emergency access.
3. **Validity** — before `NotBefore` denies (not yet active); after `NotAfter`
   denies (expired).
4. **Actor binding** — `req.ActorID` must equal `grant.ActorID`; otherwise
   deny (`actor not authorized`). This is the *consent-level* actor check, in
   addition to — never instead of — the upstream professional-authorization
   gate (`identity.Actor` + `auth.AuthorizeFacilityRead`). Consent therefore
   never bypasses professional authorization.
5. **Emergency** — if `req.Emergency`, break-glass is governed by
   `EmergencyPolicy`:
   - `Allowed == false` → deny (`emergency access not permitted by policy`);
   - `Allowed == true`, a non-revoked, in-window grant, and a matching
     resource category → allow, relaxing the purpose/facility/organization/
     subject checks a normal read requires (the actor binding above still
     applies, and the mandatory-review flag rides to the audit path).
6. **Normal access** — requires exact match on resource category, purpose,
   facility, organization, and subject; any mismatch denies with a specific
   reason.

**Output** — a `Decision{Allowed bool, Reason string, Audit AuditEvent}` whose
`Reason` is one of the stable constants (`allowed`, `actor not authorized`,
`revoked`, `expired`, `grant not yet active`, `resource category not
permitted`, `purpose not consented`, `facility not in scope`, `organization
not in scope`, `patient mismatch`, `emergency access not permitted by policy`)
so audit logs can record a structured, comparable reason. Every decision —
allow or deny — carries the full `AuditEvent` (request as stated: actor,
subject, facility, organization, purpose, resource category; evaluation
instant; outcome), so denials are audited exactly like allows.

Because it takes a `Grant` parameter and never resolves storage, the evaluator
itself stores **no NIK**, performs **no search**, and exposes **no mapping**. It
is a pure function over synthetic identifiers.

---

## 4. Audit and patient-visible access history

- **RUNNING (IMPLEMENTED).** `audit_events` is an append-only, hash-chained log
  (`previous_hash` / `event_hash`); authz denials already carry a structured
  reason to it. (`packages/db/migrations/0004_audit_events.sql`,
  `apps/api/internal/audit/`, `apps/api/internal/identity/authz.go`.)
- **RUNNING (evaluator).** `Evaluate` already returns a full `AuditEvent`
  with every decision — allow or deny — capturing the request as stated
  (actor, subject, facility, organization, purpose, resource category), the
  evaluation instant, and the outcome (see §3).
- **DESIGN-ONLY (PROPOSED).** Emitting that *consent* `AuditEvent` into the
  `audit_events` stream — per the `Grant` evaluated — is designed, not wired.
  Same for break-glass: every emergency allow must produce an audit event
  flagged for mandatory review (threat model §4.8 "break-glass review").
- **DESIGN-ONLY (PROPOSED).** A patient-visible access history is a *read path
  over the existing audit stream scoped to that patient's own records*
  (threat model §4.10). It is not a new store; it is a filtered, patient-scoped
  projection of events whose `patient_id` matches the authenticated subject.
  Until a verified patient identity (Layer 2/3) exists, there is no such subject
  to scope the projection to.

---

## 5. What is deliberately out of scope for this branch

1. A `patients` table, a NIK column, or any verified-identity onboarding flow
   (Layer 2/3 — DESIGN-ONLY).
2. A SATUSEHAT client or any credentialed outbound call (Layer 4 —
   DESIGN-ONLY, blocked).
3. Any unrestricted patient search or identity-mapping endpoint.
4. A consent ledger table/column; the evaluator reasons over a supplied `Grant`
   only. Wiring a real ledger is a PROPOSED follow-up.