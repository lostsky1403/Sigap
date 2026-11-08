# Identity, Access & Consent — Threat Model

**Status:** Design / threat model (English). **Branch:** `feat/sigap-v2-implementation`.
**Scope:** how SIGAP v2 identifies a patient, proves their relationship to a facility,
authorizes a visit, controls access to health data, and records who looked at what.

This document is the security reasoning behind the identity/access/consent work. It is not a
shipping checklist. Where the system already enforces a property, the property is marked
**IMPLEMENTED** and the enforcement point is cited. Where a property is designed but not yet
built, it is marked **PROPOSED**. Where the property depends on an owner decision (not merely
engineering work), it is marked **POLICY-DECISION-REQUIRED**. Nothing PROPOSED or
POLICY-DECISION-REQUIRED is claimed as live.

Legend used throughout:

- **IMPLEMENTED** — live in this branch; enforcement point cited to a file/endpoint/schema.
- **PROPOSED** — designed, not built; requires backend/API/DB work.
- **POLICY-DECISION-REQUIRED** — designed, not built; requires an explicit owner decision first.

---

## 1. Invariant (authoritative, read first)

> **The 6-character check-in code is a TEMPORARY VISIT bearer token. It proves
> possession of a single appointment/visit, not a medical identity. It MUST NOT —
> and in the implemented system does not — grant access to medical records, to the
> patient's identity, or to any visit other than the one it was issued for.**

Corollaries that must hold at all times:

1. The check-in code authenticates **a visit**, never **a person**. It selects one
   `appointments` row and nothing else.
2. The check-in code is **revocable and expiring by nature of the visit**: once the
   appointment reaches a terminal state (`completed`, `cancelled`, `no_show`,
   `checked_in`), the code stops being usable for anything beyond a status read.
3. The check-in code is **not a substitute for a verified identity**. A person who holds
   a code is not thereby a "verified patient" to the system; they are a "holder of a
   visit token". Verified identity (when it exists) is a *separate* credential, resolved
   through a different path (see §4.1–§4.2).
4. Anything that wants to read **full medical records** must pass a **different,
   stronger** authorization path (verified identity + consent + facility-bound clinician
   authorization), never the visit-token path.

**Code-format contract (IMPLEMENTED).** Codes are exactly **6 characters** from a
**32-symbol alphabet** that excludes `I`, `O`, `0`, `1` (the glyphs a citizen cannot
reliably transcribe). Lookup is deliberately more permissive for legacy tolerance:
accepts alphanumerics plus `-` up to **64 characters**, and folds `0`↔`O` / `1`↔`I` on both
sides, exact match first, ambiguous fold refused. (`F:\sigap-design-v2\design\sigap-v2-product-evolution.md`,
"Check-in code format contract".)

---

## 2. Assets and trust boundaries

| Asset | Class | Boundary | Notes |
|---|---|---|---|
| `checkin_code` | Bearer token | Public | 6-char, 32-symbol. Visit-scoped bearer token (see invariant). |
| Walk-in `TicketID` (UUID) | Bearer token | Public (holder) | 122 bits of randomness, non-sequential; returned only to the creator. **PROPOSED** as a status key. |
| JWT / OIDC access token | Identity token | Trusted IdP | Proves `sub` only; never an authorization source. |
| `app_users` + `user_roles` + `role_permissions` + `permissions` | Authorization state | Server-only | Trusted DB RBAC; never mirrored into client-held tokens. |
| `medical_records` | PHI | Server-only | Immutable, SHA-256-signed visit history. No patient-facing read path today. |
| `appointments` (inline `patient_phone`, `patient_display_name`) | PII | Server-only | Patient data minimized inline; no `patients` table join in the current schema. |
| `notification_outbox` | Derived contact metadata | Server-only | Masked contact + hash only; raw contact is transient. |
| `audit_events` | Tamper-evident access log | Server-only | Append-only, hashed, PII-sanitized. |

---

## 3. Threat matrix (summary)

| # | Threat | Primary mitigation | Status |
|---|---|---|---|
| T1 | Check-in code escalated into a medical-identity credential | Invariant: visit token ≠ identity; separate stronger path for records | IMPLEMENTED (design + public surface) / PROPOSED (records path) |
| T2 | Check-in code brute-force / enumeration | 32-symbol code, `0/1/O/I` removed, per-IP + per-appointment limiting, non-enumerable lookup | IMPLEMENTED |
| T3 | Cross-facility privilege escalation (clinician at A reads B) | FacilityGrant provenance + `AuthorizeFacilityMutation/Read`; fail-closed | IMPLEMENTED |
| T4 | PHI / PII re-identification via public status & notification surfaces | PII-free status payload; timing removed; outbox masking/hash | IMPLEMENTED |
| T5 | Missing consent / emergency-break-glass governance, no patient-visible audit | Consent ledger, break-glass with mandatory review, patient access log | PROPOSED / POLICY-DECISION-REQUIRED |

Each area is expanded below with the specific threats and mitigations.

---

## 4. Threat model by area

### 4.1 Verified identity onboarding

**Principle.** A patient is a real person whose identity is established before they are
granted any right to *their own* records. A visit token is not the onboarding step.

**Threats.**

- **T1a — Identity substitution at onboarding.** An attacker registers a phone number they
  do not control or that is shared within a family, and later claims the real patient's
  records.
- **T1b — Claims-based self-enrollment.** A self-service "I am patient X" flow without
  proof, allowing any holder of a phone to bind themselves to X's history.
- **T1c — Incomplete onboarding leaking partial identity.** An onboarding flow that
  collects more identity than a visit needs, broadening the blast radius of a later leak.

**Mitigations.**

- **IMPLEMENTED** — The visit flow collects only the minimum: `patient_display_name` +
  `patient_phone`, stored inline on the `appointments` row (data minimization).
  (`F:\sigap-design-v2\packages\db\migrations\0005_appointments.sql`.)
- **PROPOSED** — For any *record-bearing* patient identity, require an explicit
  verified-identity step (e.g. genuine NIK-backed proof, not a self-asserted phone). No
  PROPOSED path may bind medical history to a phone without a proof step.
- **PROPOSED** — Separate the **visit identity** (phone + display name, bearer-token
  scoped) from the **record identity** (a `patients` record with a verified identifier).
  The current schema has no `patients` table; introducing one is a *new* identity boundary
  and must not be conflated with appointment rows.
- **POLICY-DECISION-REQUIRED** — Whether the NIK (or other national identifier) is the
  canonical record key, and what proof level is acceptable for remote vs. in-person
  enrollment.

### 4.2 SATUSEHAT patient identity + KYC compatibility

**Principle.** If SIGAP interoperates with SATUSEHAT (Indonesia's national health platform),
patient identity must come from a *trusted, verifiable* source, and the KYC step must be
non-bypassable. SATUSEHAT authorization/onboarding is **not** to be bypassed.

**Threats.**

- **T2a — Self-asserted SATUSEHAT identity.** Claiming to be a SATUSEHAT-registered
  patient without the platform verifying it.
- **T2b — Cross-provider identity confusion.** Two different IdPs resolving to the same
  local patient, or one external `sub` mapping to two patients, causing record mix-ups.
- **T2c — KYC bypass via the visit token.** Using the check-in code (or walk-in token) to
  reach SATUSEHAT-linked data without the KYC step.

**Mitigations.**

- **IMPLEMENTED (pattern to reuse)** — External identity is already handled defensively in
  the staff path: the JWT `sub` is mapped to a server-side `app_users` row via a partial
  unique index (`uq_app_users_subject_active`), and the token proves only `sub` — never
  permissions. (`F:\sigap-design-v2\packages\db\migrations\0008_identity_subject.sql`,
  `F:\sigap-design-v2\apps\api\internal\auth\jwt_provider.go`.)
- **PROPOSED** — Mirror that pattern for patients: a `patients` identity keyed by a verified
  SATUSEHAT/NIK-derived subject, with a **unique** subject→patient binding and the same
  "token proves identity, server resolves authorization" rule.
- **PROPOSED** — Enforce that any SATUSEHAT-linked read path requires the *identity*
  credential, and explicitly reject the visit token / walk-in token as input to it.
- **POLICY-DECISION-REQUIRED** — The KYC proof level and the exact SATUSEHAT onboarding
  flow are owner/partner decisions; this document must not specify a concrete bypassable
  flow. No weakening of SATUSEHAT authorization/onboarding is permitted.

### 4.3 Secure sessions

**Principle.** Anything authenticated by a *session* (staff/admin, and future patient
accounts) gets a properly validated, server-authorized session, not a homegrown password
store.

**Threats.**

- **T3a — Token forgery / alg confusion.** A forged or mis-signed JWT accepted as valid.
- **T3b — Trusting token claims for authorization.** A client-controlled `permissions`
  claim widening its own access.
- **T3c — Stale session outliving revocation.** A revoked role change not taking effect
  because authorization is cached.

**Mitigations.**

- **IMPLEMENTED** — `JWTProvider.Authenticate` enforces signature verification, rejects
  `alg = none`, validates `exp`/`nbf`/`iat`, `iss`, `aud`. Missing `kid` is rejected when
  JWKS is configured. (`F:\sigap-design-v2\apps\api\internal\auth\jwt_provider.go`.)
- **IMPLEMENTED** — **AUDIT-101**: authorization is resolved server-side from the DB RBAC
  resolver; the token's `permissions` claim is ignored. Unknown/disabled subject fails
  closed. (`F:\sigap-design-v2\apps\api\internal\auth\rbac_resolver.go`.)
- **IMPLEMENTED** — Live grant re-resolution for facility-scoped reads: `ActorWithLiveGrants`
  refreshes provenance from the DB on each request, so a revoked role stops authorizing on
  the next request rather than waiting for re-authentication.
  (`F:\sigap-design-v2\apps\api\internal\auth\facility_scope.go`.)
- **PROPOSED** — Patient-facing sessions reuse the same JWT/OIDC provider; patient identity
  must never fall back to the "dev" header identity, which is dev-only
  (`SIGAP_DEV_IDENTITY`) and must be unreachable in any record-bearing environment.

### 4.4 Patient ↔ facility relationship model

**Principle.** Data access is scoped by the *relationship* between a person and a facility,
and that relationship must be explicit — not implied by "someone once booked here".

**Threats.**

- **T4a — Implicit relationship.** A patient who once had an appointment at facility A is
  treated as "belonging to" A forever, exposing future cross-facility data.
- **T4b — Phone-collision merge.** Two people sharing a phone (common in family use) being
  merged into one implicit patient, leaking one person's visits to the other.
- **T4c — Phone re-use.** A recycled phone number inheriting the prior holder's history.

**Mitigations.**

- **IMPLEMENTED (current, explicit limitation)** — There is **no implicit patient record**:
  appointments carry inline `patient_phone`/`patient_display_name` and do **not** join a
  `patients` table. This avoids accidental cross-facility patient merging today, at the
  cost of no cross-visit linkage. (`F:\sigap-design-v2\packages\db\migrations\0005_appointments.sql`.)
- **PROPOSED** — Introduce an *explicit* patient↔facility relationship (a `patients` table
  with a `patient_facility` membership/authorization row) rather than deriving membership
  from any phone that ever booked. Membership must be created by an explicit, consent-gated
  action, not inferred.
- **POLICY-DECISION-REQUIRED** — Whether a national identifier (NIK) breaks the phone
  collision in practice, and how provider-side dedup must work for a family sharing a phone.

### 4.5 Patient consent management

**Principle.** A patient's data is used for a purpose they consented to, and consent is
**recorded** (not assumed) and **revocable**. No consent ledger exists today.

**Threats.**

- **T5a — Implied consent.** Using PII/PHI for a purpose (marketing, research, cross-facility
  sharing) the patient never approved.
- **T5b — Un-revocable consent.** A patient who withdraws consent cannot actually stop the
  use.
- **T5c — Consent without evidence.** A dispute about "did the patient consent?" that cannot
  be answered because nothing was recorded.

**Mitigations.**

- **IMPLEMENTED (minimal)** — The visit flow holds only the data the citizen themselves
  supplied for *that* booking, and the notification path stores only masked contact + hash,
  discarding the raw contact after consumption. No secondary use is wired anywhere.
  (`F:\sigap-design-v2\packages\db\migrations\0006_notifications.sql`.)
- **PROPOSED** — Add a purpose-bound **consent ledger** (patient id, purpose, granted at,
  revoked at, grantor, evidence). Any new secondary use (research, analytics beyond the
  visit, cross-facility sharing) requires a consent row *before* processing, and revocation
  must take effect at the next processing step, not "eventually".
- **POLICY-DECISION-REQUIRED** — The set of approved purposes and whether consent is opt-in
  or opt-out per purpose, plus the retention window after revocation.

### 4.6 Clinician authorization

**Principle.** A clinician may access only what their role — **at a specific facility** —
permits, and only for patients they legitimately treat.

**Threats.**

- **T6a — Flat-permission union.** A user holding "viewer at A" and "operator at B" getting
  the union `{viewer, operator}` applied at both facilities.
- **T6b — Role without facility provenance.** A non-super-admin role with no facility
  binding silently becoming global.
- **T6c — Over-broad clinical read.** A clinician able to read *any* patient's full record
  at their facility, regardless of care relationship.

**Mitigations.**

- **IMPLEMENTED** — **Facility provenance is first-class.** `FacilityGrant` carries
  `FacilityID` + `Unrestricted` per permission key, and `HasPermissionAtFacility` fails
  closed; a zero-scope (no facility) non-super-admin assignment grants nothing.
  (`F:\sigap-design-v2\apps\api\internal\identity\actor.go`.)
- **IMPLEMENTED** — `AuthorizeFacilityMutation` and `AuthorizeFacilityRead` require **both**
  (a) target facility inside the actor's scope **and** (b) the permission held *at* that
  facility. `AuthorizedFacilityIDsForPermission` returns the *intersection* of scope and
  provenance, specifically to kill the union bug. (`F:\sigap-design-v2\apps\api\internal\auth\facility_scope.go`.)
- **IMPLEMENTED** — Route gating via `RequirePermission` with an allow-list and a
  deny-by-default fallback; denials carry a structured reason to the audit log.
  (`F:\sigap-design-v2\apps\api\internal\identity\authz.go`.)
- **PROPOSED** — For full-record access, add a **care-relationship** predicate on top of
  facility scope: a clinician may read a patient's record only when a current care/treatment
  relationship exists, not merely because they share a facility. This is the gate that the
  visit token intentionally lacks.

### 4.7 Facility-bound access

**Principle.** A token or credential issued in the context of one facility cannot be replayed
to reach another facility's data. This is the same rule as §4.6, applied to every
facility-scoped resource, including reads.

**Threats.**

- **T7a — Cross-facility read leak.** A read endpoint returning facility-B rows to an actor
  authorized only at facility-A.
- **T7b — Row-existence oracle.** A denied read returning a distinguishable error that leaks
  *that a facility/row exists*.
- **T7c — Walk-in ticket cross-facility replay.** A walk-in `TicketID` from one facility
  being usable to query another.

**Mitigations.**

- **IMPLEMENTED** — `FacilityReadSet`: an actor without provenance for any in-scope facility
  gets an **empty set rendered as `200 []`**, not an error, so row existence never leaks.
  (`F:\sigap-design-v2\apps\api\internal\auth\facility_scope.go`.)
- **IMPLEMENTED** — Live `FacilityGrantResolver` keeps read provenance aligned with the DB,
  so scope and provenance cannot drift. (`F:\sigap-design-v2\apps\api\internal\auth\facility_scope.go`.)
- **PROPOSED** — Walk-in `TicketID` (UUID) lookup must be facility-scoped server-side: the
  token resolves to its own facility, and the response exposes only that visit's data
  (`facility_name`, `formatted_number`, `status`, `people_ahead` computed per facility per
  day) — never other rows. (`F:\sigap-design-v2\design\sigap-v2-product-evolution.md §2.2`.)

### 4.8 Emergency access ("break-glass")

**Principle.** Emergency access is a necessary escape hatch, but it must be a *controlled,
audited, reviewed* override — never a silent bypass of identity verification or consent.
It must respect applicable legal/clinical rules; this document must not weaken them.

**Threats.**

- **T8a — Unbounded break-glass.** An emergency flag that grants permanent or unscoped
  record access.
- **T8b — Unreviewed break-glass.** Emergency access used without any subsequent review,
  so unauthorized disclosures go undetected.
- **T8c — Break-glass as an auth bypass.** Using "emergency" to skip the patient identity
  and KYC steps entirely.

**Mitigations.**

- **PROPOSED** — Break-glass must: (1) require an explicit clinician action with a stated
  reason; (2) be **time-boxed**; (3) be **facility-scoped** (§4.6/§4.7 rules still apply);
  (4) write a **high-severity audit event** distinguishable from normal access; and
  (5) trigger **mandatory post-hoc review** by a second responsible person within a fixed
  period.
- **POLICY-DECISION-REQUIRED** — The exact grounds for break-glass, the review SLA, and how
  it interacts with the legal requirement to verify identity *as soon as practicable* must
  be specified by the owner/legal, not by this document. Break-glass is an **override, not a
  bypass** of identity verification and consent.

### 4.9 Revocation

**Principle.** Every credential can be revoked, and revocation takes effect **immediately**
— codes are revoked by visit-state transition, sessions by role disablement, and patient
consent by an explicit revocation.

**Threats.**

- **T9a — Long-lived visit token.** A check-in code that keeps working for the wrong visit
  or forever.
- **T9b — Stale authorization after role change.** A demoted/disabled user retaining access.
- **T9c — No cancellation path for the patient.** A patient who cannot stop a visit token
  they believe is leaked.

**Mitigations.**

- **IMPLEMENTED** — Check-in code stops being useful once the visit leaves `scheduled`; the
  atomic check-in transition (`UPDATE ... WHERE status='scheduled'`) means a second use of
  the same code cannot re-open or double-spend a visit.
  (`F:\sigap-design-v2\apps\api\internal\handler\booking.go`.)
- **IMPLEMENTED** — Patient-initiated cancellation via `POST /api/v1/appointments/cancel`
  with the code as the bearer — a leaked/lost slot can be abandoned by the code holder, and
  post-check-in cancellation is refused (facility then controls the ticket).
  (`F:\sigap-design-v2\design\sigap-v2-product-evolution.md §2.3`.)
- **IMPLEMENTED** — Role disablement + soft-delete honored in the RBAC query; live
  re-resolution makes revocation effective on the next request. (`F:\sigap-design-v2\apps\api\internal\auth\rbac_resolver.go`,
  `...\facility_scope.go`.)
- **PROPOSED** — Patient consent revocation must propagate to any scheduled secondary use and
  be recorded in the consent ledger (see §4.5).

### 4.10 Patient-visible access logs

**Principle.** The patient must be able to see *who accessed their data*, so unauthorized
access is both detectable and attributable. Today the audit log is append-only and
PII-sanitized, but there is **no patient-facing view** of it.

**Threats.**

- **T10a — Unattributable disclosure.** A record is read but no evidence exists of who/when,
  so a breach cannot be investigated or disclosed to the patient.
- **T10b — Invisible over-collection.** Staff reads beyond legitimate need go unnoticed
  because the patient cannot see them.
- **T10c — Log leakage as a new oracle.** A patient access-history endpoint that itself
  leaks other patients' data or becomes an enumeration surface.

**Mitigations.**

- **IMPLEMENTED** — `audit_events` is append-only, hashed (`event_hash`, `previous_hash`
  for a tamper-evident chain), and PII-sanitized: metadata runs through a forbid-list
  sanitizer, and IP/user-agent are stored hashed, never raw.
  (`F:\sigap-design-v2\packages\db\migrations\0004_audit_events.sql`,
  `F:\sigap-design-v2\apps\api\internal\audit\service.go`.)
- **PROPOSED** — Add a patient-visible access-log endpoint that lists access events for the
  *authenticated patient's own* record only, resolved through the same verified-identity +
  consent + facility-bound path as the record itself — never through the visit token. The
  endpoint must be rate-limited and non-enumerable (patient's own identity only).
- **PROPOSED** — Record PHI access events with an explicit `resource_type = 'medical_record'`
  and the acting clinician + facility, so the patient log can answer "who, when, where".

---

## 5. End-to-end journey (with implementation status)

> Statuses: **[IMPLEMENTED]** live · **[PROPOSED]** designed, not built ·
> **[POLICY]** needs owner/legal decision. The step column marks the *primary* component.

| # | Step | Status | Notes / enforcement |
|---|---|---|---|
| 1 | **Sign in** (staff/admin; future patient account) | **[IMPLEMENTED]** for staff; **[PROPOSED]** for patient accounts | JWT/OIDC validates signature/`iss`/`aud`/`exp`, `alg≠none`; authorization resolved server-side from DB RBAC. Patient accounts must reuse this, never the dev header. |
| 2 | **Find facility** | **[IMPLEMENTED]** | Public catalog/schedule surfaces; counts-only, no patient identifiers. |
| 3 | **Book** | **[IMPLEMENTED]** | `POST /api/v1/appointments` (no auth). Minimal PII inline; per-phone-per-facility-per-day limiting (max 2). 6-char code issued. |
| 4 | **Check in** | **[IMPLEMENTED]** | `POST /api/v1/appointments/{id}/check-in`: atomic status transition, per-(IP, appointment) limiting, non-enumerable error classification. Code is the bearer. |
| 5 | **Receive care** | **[IMPLEMENTED]** at queue/visit level; **[PROPOSED]** for record-level care | Queue ticket linked on check-in; `medical_records` written by the engine with SHA-256 signature. No patient-facing record read yet. |
| 6 | **View authorized health summary** | **[PROPOSED]** | Requires verified identity + consent + facility-bound clinician authorization (§4.1–§4.7). **Explicitly not reachable via the check-in or walk-in token.** |
| 7 | **Inspect access history** | **[PROPOSED]** | Patient-visible access log on the authenticated patient's own record (§4.10), non-enumerable, rate-limited. |

**Cross-cutting guarantee (requirement 4):** *No patient information may be exposed
without approved governance + access controls.* This is enforced today on the public
surface (PII-free status payload, masked/hashed contacts, sanitized audit) and must be
enforced on every PROPOSED record-bearing surface — a record read requires **verified
identity + an explicit consent/purpose record + facility-bound authorization**, and it
must fail closed when any element is missing.

---

## 6. Top threats (named)

1. **Visit-token → identity escalation** — treating the 6-char check-in code (or walk-in
   `TicketID`) as a medical identity and routing it to full medical records.
2. **Check-in code brute-force / enumeration** — guessing 6-char codes or probing status to
   hijack check-ins or enumerate appointments.
3. **Cross-facility privilege escalation** — a clinician's role at facility A authorizing
   reads/mutations at facility B (flat-permission union, missing provenance).
4. **PII / PHI re-identification on public surfaces** — appointment timing, phone, or name
   leaking through the status endpoint or the notification outbox.
5. **Missing consent & break-glass governance** — emergency/record access without patient
   consent or an approved, reviewed override, compounded by no patient-visible access audit.