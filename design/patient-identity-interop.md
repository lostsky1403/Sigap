# Internal Patient Identity → Authorized National Identity — Boundary Design

> Status: **DESIGN document**. It defines the adapter/contract boundary only; it is
> not a claim that any of it is live. It performs **no wire calls** — no
> SATUSEHAT request, no Supabase request, no login claim.
>
> **Verdict (authoritative).** `AUTHENTICATED_PATIENT_OWNERSHIP = BLOCKED`.
> An isolated Supabase Auth dev env is **not configured** (see §5): every
> Supabase/JWT variable in `.env` is a commented placeholder and the API runs in
> `SIGAP_AUTH_MODE=dev`. Until external provisioning completes, no verified-login
> claim may be made — only synthetic identities.

Sources (read; do not re-research): `design/regional-interop-contracts.md`
(§0, §2.1, §2.3, §2.4), `design/satusehat-sandbox-readiness.md` (§1–§5),
`design/satusehat-interoperability-plan.md` (§4–§8, §11–§14),
`packages/db/migrations/0001_init.sql`, `0003_identity_rbac.sql`,
`0005_appointments.sql`, `0008_identity_subject.sql`,
`apps/api/internal/identity/actor.go`, `apps/api/internal/auth/config.go`, `.env`.

---

## 1. Two identities, one care subject

SIGAP has **two distinct identities**. They must never be conflated:

| Identity | What it is | Current state |
|---|---|---|
| `app_users` | **Auth principal** — who is logged in (staff). Carries an external JWT/OIDC `subject` (`0008`) mapped to `app_users.id`. All roles are staff: `super_admin`/`facility_admin`/`operator`/`viewer`/`organization_admin`/`regional_admin`. There is **no citizen role**. | Live (`0003` + `0008`); synthetic actors only under `dev` mode |
| `patients` | **Care subject** — the person receiving care. Local UUID + unique `phone` + `full_name`/`gender`/`date_of_birth`. No NIK, no IHS column. | Live (`0001`); referenced by `queue_tickets.patient_id`; `appointments` currently store `patient_display_name`/`patient_phone` **inline** with no patient FK |

**Future internal patient id** = `patients.id` (UUID). The mapping below assumes a
PROPOSED step where `appointments` gains `patient_id UUID REFERENCES patients(id)`
(replacing the inline name/phone correlator, which remains the only live
correlator today). A citizen's Supabase Auth account (when provisioned) links to a
`patients` row via a PROPOSED ownership mapping; this is what
`AUTHENTICATED_PATIENT_OWNERSHIP` means — and it is **BLOCKED** (§5).

## 2. Authorized national identity — the adapter/contract boundary

The national identity is the **IHS number** (`https://fhir.kemkes.go.id/id/patient-ihs-number`),
resolved from the national Master Patient Index (MPI); NIK lives under
`https://fhir.kemkes.go.id/id/nik` (interop §4, S8). The boundary is the
`PatientIdentityMapper` contract from `regional-interop-contracts.md` §2.1 —
**nothing else**. No wire calls exist today; the only exercisable impl is the
`LOCAL-SYNTHETIC-CONTRACT` fake over fictional fixtures.

```
patients.id (local UUID) ── PatientIdentityMapper ──> patient_identity_mappings (PROPOSED)
                                                         ├─ nik (POLICY-DECISION-REQUIRED, D1)
                                                         ├─ ihs_number      (source of truth = MPI)
                                                         └─ satusehat_patient_ref  "Patient/<ihs>"
```

Rules at this boundary (unchanged from the contracts doc, restated authoritatively):

1. **Exactly one key per resolve**: local patient id, NIK (deterministic, preferred),
   or demographic hint (name+DOB+gender) — which routes to a human `NeedsHumanLink`
   step, **never** silent auto-merge.
2. **Phone is never a resolver key** and never maps to an IHS number.
3. **Store versions, never overwrites** an existing IHS assignment; SIGAP holds a
   cached projection, re-resolves on mismatch, and never overwrites the MPI's
   IHS assignment (interop §4 item 5, §8).
4. **NIK is collected only under owner decision D1** (new PII field, lawful basis,
   non-enumerable) and is **not stored in this pilot**; synthetic fixtures carry a
   fictional NIK as a *test input* only (sandbox readiness §3.5, G8).
5. The credentialed `Resolve` (NIK MPI search over the wire) is
   `FUTURE-AUTHORIZED-SATUSEHAT`: blocked on Kemenkes gates P6 (credentials) and
   P7 (owner decisions). It returns "not authorized / credential missing" by
   construction. `LookupLocal` is cached and **never** performs outbound I/O.
6. A resolved mapping row is **metadata about the subject** — it establishes *who*
   a record belongs to. It confers no access on its own (§4).

## 3. FHIR Patient / Encounter boundary

Generated at the same adapter level via the `FHIRContractValidator` contract
(§2.3) — no clinical payload expansion, no live client:

- **Patient**: identifiers = IHS + NIK systems; name/gender/birthDate from the
  local `patients` row; no fabricated content; `ValidatePatient` enforces the
  IHS identifier system presence.
- **Encounter**: `appointments`/`queue_tickets` lifecycle → `status`
  `planned → in-progress → finished | cancelled`; `subject` = `Patient/<ihs>`,
  `serviceProvider` = `Organization`, `participant` = `Practitioner`,
  `location` = `Location` (onboarding-mapped refs). `medical_records` stays a
  *signed attendance record*, not a clinical record; clinical resources
  (Condition/Observation/…) are PROPOSED and gated by the §4 rules.
- Wire shape: UTC+00 timestamps (WIB −7), dates ≥ 2014-06-03; correction = PUT
  same `id` with new `meta.versionId` (never a new id); writes flow through the
  `satusehat_outbox` (PROPOSED) with idempotency keys.
- The credentialed `FHIRClient` is `FUTURE-AUTHORIZED-SATUSEHAT` — blocked on P6;
  the exact FHIR R4 base is read from the api-catalogue at integration time (D2),
  never hard-coded.

## 4. Identity match ≠ access — the hard rule

A successful identity match proves **who the patient is**. It grants **nothing**
to any staff actor. Access is a separate, deniable-by-default decision:

| Ask | Gate | Health |
|---|---|---|
| Staff operates on SIGAP-owned data (appointments, queue, facility) | Existing `user_roles.facility_id` scoped RBAC (`identity.HasPermissionAtFacility`) | Live (dev synthetic) |
| Staff reads **national** data (RME/clinical) | Facility-scoped RBAC **AND** the patient's SATUSEHAT-Mobile grant (6-digit code / emergency path, platform-enforced, S9) | Platform gate; SIGAP authors no `Consent` resource and never collects/logs the code (D3: deep-link out) |
| Citizen views own record | SATUSEHAT Mobile (Resume Medis) — **not** a new SIGAP public endpoint (re-opens enumeration vector) | Platform-mediated |
| `super_admin` | Must **never** become a global medical-records reader (F5 hard restriction) | Enforced rule |

Enforcement point: the `ConsentGate` boundary (§2.4) returns a pure decision over
(actor scope × purpose) and records evidence only (authorized/denied, basis,
patient IHS, facility) via `audit.Service` — with the 6-digit code on the
PII/PHI forbid-list. A match without a grant yields `authorized=false, basis=no-platform-grant`.
The `PatientIdentityMapper` emits mapping rows and **no permission decisions**;
an Encounter is created from a visit lifecycle event, never from an identity match.

## 5. Blockers + external provisioning

**SATUSEHAT interop (unchanged, authoritative):** `SATUSEHAT_SANDBOX = NOT_CONNECTED`.
6 of 8 gates are BLOCKED or NEEDS-OWNER (G1–G3, G5, G8); no `client_id`/`client_secret`
exists; the sandbox is reachable only after partner registration → SSP verification →
credential issuance → fasyankes data update, from a dedicated credentialed runner.
NIK collection (D1), consent UX (D3), KMS keys (D4), first use-case (D5) are owner decisions.

**Supabase Auth dev env — verdict: BLOCKED.** Evidence from `.env`/`.env.example`:

- `PUBLIC_SUPABASE_URL` and `PUBLIC_SUPABASE_ANON_KEY` are **commented placeholders**
  (lines 41–42) — no active values anywhere in the repo.
- `SIGAP_AUTH_MODE=dev` (line 32); `SIGAP_AUTH_ISSUER`/`SIGAP_AUTH_AUDIENCE`/
  `SIGAP_AUTH_JWKS_URL` are commented (lines 35–37). Go `auth.LoadConfigFromEnv`
  requires issuer + audience for `jwt` mode; `dev` mode = synthetic actor from
  `X-Sigap-Dev-User-ID` header.
- Therefore: no verified-login claim may be made; synthetic identities only.

**External provisioning items (before `AUTHENTICATED_PATIENT_OWNERSHIP` can move from
BLOCKED):**

1. Provision a Supabase project with Auth enabled (isolated dev env) and set
   `PUBLIC_SUPABASE_URL` / `PUBLIC_SUPABASE_ANON_KEY` (publishable values only —
   never the service-role key).
2. Configure the JWT/OIDC side: `SIGAP_AUTH_ISSUER` (Supabase Auth issuer,
   `https://<project-ref>.supabase.co/auth/v1`), `SIGAP_AUTH_AUDIENCE`,
   `SIGAP_AUTH_JWKS_URL`; switch `SIGAP_AUTH_MODE` from `dev` to `jwt` (production
   must be `jwt`; `dev` never in production).
3. Wire Supabase Auth identities to `app_users.subject` (`0008` already supports
   the mapping) — the `sub` claim becomes the external subject.
4. Provision **citizen** Supabase Auth accounts and a PROPOSED ownership link to
   `patients.id` (no citizen role exists today); define the enrollment flow and
   its lawful basis.
5. Keep SATUSEHAT credentials (G2) and owner decisions (D1/D3/D4/D5) as separate,
   still-blocked prerequisites; Supabase Auth alone does not unlock national-data
   access (§4).
