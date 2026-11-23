# Patient Identity Model — Authentication Subject vs Internal Patient Identity

**Status:** DESIGN document (English) + one RUNNING schema delta (`0014`) and a
RUNNING Go resolution contract (`identity.Actor.PatientAccountID`).
**Branch:** `feat/sigap-regional-pilot`.

This document separates the identity concepts SIGAP must never conflate, and
defines the **internal patient identity** now made concrete: which `app_users`
account may own a patient's appointments. It extends §1 of
`design/patient-identity-consent.md` (which separates the four identity layers)
with a fifth distinction — the *account* that owns appointments vs the
*subject* that authenticated vs the *person* who verified themselves.

Sources of truth (read, not duplicated):

- `design/patient-identity-consent.md` — the four identity layers, consent
  evaluator, hard negations (no NIK, no unrestricted patient search, no
  identity-mapping endpoint).
- `design/identity-access-consent-threat-model.md` — §4.1 verified identity,
  §4.4 facility-local record, §4.5 consent.
- `design/regional-interop-contracts.md` — `PatientIdentityMapper` contract and
  honesty markers.
- `packages/db/migrations/0003_identity_rbac.sql`, `0008_identity_subject.sql`,
  `0005_appointments.sql`, `0001_init.sql`, `0014_patient_account.sql`.
- `apps/api/internal/identity/patient_account.go` — the ownership resolver.
- `apps/api/internal/auth/rbac_resolver.go` — subject → app_users mapping.

---

## 0. Honesty markers (read first)

Every item below is tagged exactly one of:

- **RUNNING** — enforced by code/schema in this branch, with a cited file or
  migration. Only items so tagged are live today.
- **DESIGN-ONLY** — designed, not built. Sub-labels **PROPOSED** (needs
  backend/DB work), **POLICY-DECISION-REQUIRED** (needs an owner decision), and
  **FUTURE-AUTHORIZED-SATUSEHAT** (needs Kemenkes credentials + partner
  registration) are all DESIGN-ONLY.

**Hard negations** (identical to the consent contract, non-negotiable):

1. **No NIK storage.** The system never stores a national identifier (NIK).
2. **No unrestricted patient search.** Every record access is scoped to a
   subject the caller can already name and is authorized to reach.
3. **No identity-mapping endpoint.** No public or internal endpoint returns a
   phone→NIK, phone→IHS, or local→national correlation.
4. **Synthetic only.** Every fixture in this document is fictional.

---

## 1. The five concepts (separated, never conflated)

### 1.1 Authentication subject — `app_users.subject` — **RUNNING**

Who is this in the *application*? The external identity a validated token
proves: the JWT/OIDC `sub` (or a local test subject), mapped to a server-side
`app_users` row by the partial unique index `uq_app_users_subject_active`
(migration `0008`). The subject is never an authorization source; permissions
are resolved server-side. This layer answers **"which account is this?"** —
for staff *and* citizens alike.

- **RUNNING (IMPLEMENTED).** `apps/api/internal/auth/jwt_provider.go`,
  `rbac_resolver.go`, migrations `0003`/`0008`.

### 1.2 Internal patient identity — an `app_users` row that is a CITIZEN — **RUNNING (schema + resolver)**

Which account may **own a patient's appointments**? This is **not** a staff
role. The RBAC roles (`super_admin`, `facility_admin`, `operator`, `viewer`,
`organization_admin`, `regional_admin`) are all **staff** roles. A citizen is
an `app_users` row that holds **no staff role assignment at all** — it reuses
the existing `app_users` table, **no parallel identity system**, no new table,
no new role. Citizenship is the *absence* of staff authorization, which is
exactly what makes the fail-closed check cheap and safe.

- **RUNNING (MIGRATION `0014`).** `appointments.patient_account_id UUID
  REFERENCES app_users(id) ON DELETE SET NULL`, NULLABLE, indexed
  (`idx_appointments_patient_account_id`). Existing anonymous bookings stay
  NULL; NULL is the "anonymous" state, never a backfilled guess.
- **RUNNING (RESOLVER).** `identity.Actor.PatientAccountID()` (§2) returns
  the owning `app_users.id` for an authenticated citizen and `""` for everyone
  else.
- **RUNNING (SEED).** `packages/db/seed/regional_tenancy.sql` seeds one
  synthetic citizen app_user: subject `local-patient-p1`, id
  `00000000-…-c307`, no `user_roles` rows (self-verified by the seed).
- **Current booking behavior is NOT changed.** The public booking and check-in
  handlers (`apps/api/internal/handler/booking.go`) are untouched: they still
  write inline `patient_display_name`/`patient_phone` and *no* account link.
  `patient_account_id` is written only by a future signed-in booking flow. The
  column, resolver, and seed are the contract that flow will consume.

### 1.3 Verified patient identity — **NOT implemented (DESIGN-ONLY)**

Who is this *person*, in the real world, with proof? This is the record-bearing
identity the threat model §4.1 requires before a person gets any right to their
own records.

- **DESIGN-ONLY (PROPOSED / POLICY-DECISION-REQUIRED).** There is no NIK
  column and no verification step anywhere in the schema. The internal patient
  identity (§1.2) is an *account*, not a *proof*: it says "this session maps to
  this app_user", never "this human is who they claim". A verified identity is
  the only place a NIK could legally appear, and it is explicitly out of scope
  for this branch.

### 1.4 Facility-local medical-record identity — `patients` table — **schema RUNNING, appointment linkage DESIGN-ONLY**

Who is this patient *at a specific facility*, for clinical purposes? The
`patients` table (migration `0001`) exists and is referenced by
`queue_tickets.patient_id`; it is the facility-local record. Appointments
deliberately do **not** join it — they carry inline contact data (data
minimization, migration `0005`). The new `patient_account_id` FK points at
`app_users`, **not** at `patients`: the *owner* is an account; the *record* is
a facility-local clinical row. Never conflate the two.

- **RUNNING (TABLE).** `patients` per `0001`.
- **DESIGN-ONLY (PROPOSED).** Linking appointments to `patients` rows (a
  consent-gated facility membership) remains future work.

### 1.5 Future SATUSEHAT identity — **contract only (DESIGN-ONLY)**

Who is this patient in the *national* health platform (IHS / KYC)?

- **DESIGN-ONLY (FUTURE-AUTHORIZED-SATUSEHAT).** The `PatientIdentityMapper`
  contract in `design/regional-interop-contracts.md` §2.1 names the shape;
  no SATUSEHAT client is instantiated, no credential exists, and every such
  path returns "credential missing" by construction.

---

## 2. Account-ownership resolution contract

`identity.Actor.PatientAccountID()` (`apps/api/internal/identity/patient_account.go`)
returns the `app_users.id` that may own patient appointments, or `""` when no
such account exists. It is a pure function over the actor's resolved state —
no extra DB round trip — because the RBAC resolver already folded server-side
truth into the actor (`AppUserID`, `Permissions`, `FacilityGrants`).

Ownership exists exactly when **all** of:

1. `Actor.Type == ActorUser` (real authenticated user; system/dev/unauthenticated
   actors are excluded);
2. `AppUserID != ""` (the subject resolved to a live, active, non-deleted
   `app_users` row — only the DB resolver can produce this);
3. `len(Permissions) == 0 && len(FacilityGrants) == 0` (the account holds **no**
   staff authorization: no permission keys, no facility/org grants, including
   zero-scope assignments).

| Actor | Result |
|---|---|
| Authenticated citizen (subject → app_users, no staff grants) | the `app_users.id` |
| Unauthenticated / zero actor | `""` |
| Staff (any permission key *or* any facility/org grant) | `""` |
| Zero-scope assignment (grant with no facility provenance) | `""` |
| Dev identity (`ActorDev`) | `""` |
| System actor (`ActorSystem`) | `""` |
| Authenticated but subject unmapped (no `app_users` row) | `""` |

The result **fails closed** in every ambiguous case; there is no "safe default
to allow" branch. An empty result means the caller must treat the appointment
as anonymous (NULL) or reject. This mirrors the existing fail-closed
philosophy of `rbac_resolver.go` and `RequirePermission`.

---

## 3. Schema delta — migration `0014`

`packages/db/migrations/0014_patient_account.sql`:

- `ALTER TABLE appointments ADD COLUMN IF NOT EXISTS patient_account_id UUID
  REFERENCES app_users(id) ON DELETE SET NULL;` — nullable, so existing
  anonymous bookings stay NULL and the migration is a pure addition.
- `CREATE INDEX IF NOT EXISTS idx_appointments_patient_account_id ON
  appointments (patient_account_id);`
- `COMMENT` documenting NULL = anonymous and the delete semantics.

No backfill, no UPDATE, no edit of existing migrations or handlers.

---

## 4. Seed — synthetic citizen

`packages/db/seed/regional_tenancy.sql` (run after `0014`):

- app_user id `00000000-0000-0000-0000-00000000c307`, email
  `patient-p1@sigap.local`, display name `Citizen Patient P1 (synthetic)`,
  status `active`, subject `local-patient-p1`, `deleted_at NULL`.
- **No** `user_roles` row — deliberately. The seed self-verifies the citizen
  holds no active role assignment, so `PatientAccountID()` resolves it as an
  owner and the RBAC resolver grants it nothing.

---

## 5. DESIGN-ONLY / out of scope for this branch

- Verified identity and NIK (§1.3) — requires an owner policy decision.
- Writing `patient_account_id` from the current public booking endpoints —
  handler work is explicitly deferred; the public flow remains anonymous.
- Phone→account auto-linking or backfilling existing appointments.
- Linking appointments to `patients` facility-local records (§1.4).
- SATUSEHAT IHS integration (§1.5).
