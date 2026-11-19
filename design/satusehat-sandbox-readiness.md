# SATUSEHAT Sandbox Readiness — SIGAP

**Status:** READINESS ASSESSMENT (design). This document is the **exact minimum prerequisite list**
for a *future* SATUSEHAT sandbox PoC. It does **not** attempt to obtain or use any SATUSEHAT credential,
does **not** make any sandbox call, and does **not** bypass onboarding.

**Connection state (authoritative).** A real sandbox request is made **only** with legitimate authorized
credentials already configured. **NONE are configured here.**

> ## SATUSEHAT_SANDBOX = NOT_CONNECTED
>
> No `client_id` / `client_secret` pair exists in this environment. No OAuth token has been minted. No
> SATUSEHAT endpoint has been contacted — sandbox or production. Nothing in this repository can authenticate
> to SATUSEHAT. The sandbox becomes reachable **only** after every gate in §1 is met (partner registration →
> SSP verification → `client_id`/`client_secret` issuance → fasyankes data update), and even then only from a
> dedicated, credentialed runner — never from the default working environment.

**Sources (read; do not re-research).** Interop plan `design/satusehat-interoperability-plan.md` (§3, §4,
§5, §6, §7, §11, §13, §14, §15 — evidence S1–S9) and `design/regional-interop-contracts.md` (§0, §1,
§2.1–§2.5, §3). All citations below use those documents' own labels; S# = primary portal source in the
interop plan.

---

## 1. Gates for the sandbox PoC

Each gate is a **prerequisite in its own right** — a certified condition, not a self-serve task. Tag meanings:

- **READY** — satisfied today; nothing blocks this dimension.
- **BLOCKED** — not satisfied; a hard gate that the team cannot self-serve (external authority or credential).
- **NEEDS-OWNER** — a decision/state that an owner must resolve before the PoC can proceed.

| # | Prerequisite | Evidence | Tag | Blocked by / needed from |
|---|---|---|---|---|
| G1 | Partner registration & RME-system verification on SATUSEHAT Platform (SSP) | interop §14.1 (S7) | **BLOCKED** | Kementerian Kesehatan / SSP onboarding process |
| G2 | Kemenkes issuance of `client_id`/`client_secret`, bound to exactly one Organization ID | interop §7, §14.2 (S7) | **BLOCKED** | G1; credential pair is organization-bound; sending any other Organization ID is rejected with `resource cannot be accessed due to business rule` |
| G3 | Target fasyankes current in DFO/REGFASYANKES/RS Online | interop §14.3 (S7) | **BLOCKED** | Facility master-data update by deployment owner; never sourced from this repo |
| G4 | Sandbox endpoints only (staging), never production | interop §14.4 (S7) | **READY** | `https://api-satusehat-stg.dto.kemkes.go.id/oauth2/v1` is the documented sandbox token endpoint; production `https://api-satusehat.kemkes.go.id/…` is forbidden for the PoC. Exact FHIR R4 base must be read from the api-catalogue at integration time (D2 — never hard-coded). |
| G5 | Onboarding run: §7 five ordered steps for a test Organization/Location/Practitioner | interop §14.5 (S4) | **BLOCKED** | G2 (auth) first; then `POST Organization` → `POST Location` → `GET Practitioner` (IHS) → `GET Patient` (IHS via MPI) |
| G6 | Sandbox-only dummy patients (published table; no real NIK/clinical data) | interop §14.6 (S8) | **READY** | Published sandbox fixtures exist (e.g. `NIK 9271060312000001 → IHS P02478375538`); usable **only** under `FUTURE-AUTHORIZED-SATUSEHAT`, never in synthetic contract tests |
| G7 | Official SATUSEHAT Postman collection for reference payloads | interop §14.7 (S6) | **READY** | Public artifact; normative reference only |
| G8 | Owner sign-off: NIK collection/retention (D1) + clinical-field retention (D4) | interop §14.8, §15 D1/D4 | **NEEDS-OWNER** | NIK is collected **only** for IHS resolution (§13), and is **not stored in this pilot** (see G11); field-level KMS encryption decision outstanding |

**Summary.** 6 of 8 gates are either BLOCKED (G1–G3, G5) or NEEDS-OWNER (G8). Only G4/G6/G7 are READY,
and G6/G7 are only usable **after** G1–G3. The PoC therefore cannot begin.

---

## 2. Verifier: resource support (documented catalog, not live)

Method (interop §3): "Supported" = resource appears in the canonical interoperability resource list (S5)
**and** has a dedicated profile page + ReST api-catalogue entry. Documentation-level verification only; no
live sandbox call was or can be made (no credentials). "No" = absent from S5 / no profile page — unsupported
or at best unverified.

| FHIR resource | Support | Evidence | PoC role |
|---|---|---|---|
| `Patient` | ✅ Yes (GET search; MPI demographics; IHS + NIK identifier systems) | S4/S8 | Patient identity resolution; sandbox dummy patients only |
| `Encounter` | ✅ Yes | S5 | Visit lifecycle (below) |
| `Condition`, `AllergyIntolerance`, `MedicationStatement`, `MedicationRequest`, `Observation`, `DiagnosticReport`, `DocumentReference` | ✅ Yes | S5 | Target clinical use-case modules (D5 — owner decision) |
| `Organization` | ✅ Yes (onboarding prerequisite) | S4 | Facility master-data sync |
| `Location` | ✅ Yes (onboarding prerequisite) | S4 | Service-unit sync |
| `Practitioner` | ✅ Yes (onboarding prerequisite; GET IHS lookup) | S4 | Health-worker IHS mapping |
| `Provenance` | ❌ **Absent from catalog** | S5 (absence) | Do not design against it; SIGAP audit table is authoritative |
| `AuditEvent` | ❌ **Absent from catalog** | S5 (absence) | SIGAP `audit_events` remains authoritative; SATUSEHAT-side access trail is surfaced to the patient via SATUSEHAT Mobile, not to SIGAP |
| `Consent` | ❌ **Absent from catalog** | S5 (absence); S9 | Authorization is a platform-layer 6-digit-code / emergency check in SATUSEHAT Mobile — SIGAP authors no `Consent` resource and never logs the code |

---

## 3. Minimum prerequisites per integration dimension

### 3.1 Authentication — OAuth 2.0 `client_credentials`
- Grant type `client_credentials`, server-to-server, no user login (S7).
- `client_id` + `client_secret`, issued only after SSP verification + fasyankes data update (G1–G3).
- Credential pair bound to **exactly one Organization ID**; other orgs rejected (business-rule error).
- Token endpoint (sandbox): `https://api-satusehat-stg.dto.kemkes.go.id/oauth2/v1` (never production).
- Secrets at rest: KMS-backed secret store; never in repo/binary; rotate on schedule/suspected disclosure
  (interop §13). **Tag: BLOCKED** (G2) — no credential exists.

### 3.2 Sandbox access method
- Use only the staging token endpoint + sandbox FHIR R4 base from the api-catalogue (G4; D2).
- Dummy patients from the published sandbox table only (G6).
- **Tag: BLOCKED** at the credential gate (G1–G3); access method itself is documented and READY.

### 3.3 Resource support
- Verified-supported (S5): Patient, Encounter, Condition, AllergyIntolerance, MedicationStatement,
  MedicationRequest, Observation, DiagnosticReport, DocumentReference, Organization, Location, Practitioner.
- Absent (S5): **Provenance, AuditEvent, Consent** — do not design against them.
- **Tag: READY** (documentation-level verification complete); live verification impossible until G1–G3.

### 3.4 Facility identifier mapping (Organization / Location)
- One SATUSEHAT Organization per SIGAP facility; org id returned at onboarding, stored on the facility row.
- Each service unit → a Location under that Organization; `Location.identifier`/name from the SATUSEHAT
  catalogue (S4, interop §5).
- Onboarding-time sync, not per-visit. **Tag: BLOCKED** (G5 — onboarding run requires G2).

### 3.5 Patient identifier mapping (IHS / NIK)
- National identity = **IHS number** (`https://fhir.kemkes.go.id/id/patient-ihs-number`), resolved via the
  MPI; NIK lives under `https://fhir.kemkes.go.id/id/nik` (S8).
- Resolution: search SATUSEHAT Patient by NIK → read IHS → persist mapping in `patient_identity_mappings`
  (PROPOSED table). Never derive IHS from phone. Demographic-only fallback routes to a human link, never
  silent auto-merge (interop §4).
- **NIK must NOT be stored in this pilot.** NIK collection is a POLICY-DECISION-REQUIRED precondition (D1);
  the pilot's synthetic fixtures carry a fictional NIK as a *test input*, not a stored PII record. If NIK
  collection is adopted later, it is for IHS resolution only, retained minimally, under patient consent/lawful
  basis (interop §12/§13).
- **Tag: BLOCKED** (G2) + **NEEDS-OWNER** (D1 NIK decision).

### 3.6 Encounter lifecycle
- `appointments`/`queue_tickets` → `Encounter`: `status` `planned` → `in-progress` → `finished` |
  `cancelled`; `subject` = Patient (IHS), `serviceProvider` = Organization, `participant` = Practitioner,
  `location` = Location (S5, interop §5).
- Timestamps UTC+00 on the wire (WIB − 7); dates ≥ 2014-06-03 (S8).
- **Tag: READY** (contract shape defined in `FHIRContractValidator`); live create blocked on G2.

### 3.7 Error handling
- Idempotency key per outbound write (or reuse SATUSEHAT resource `id`); outbox `satusehat_outbox`
  `pending → submitted → acknowledged | failed | dead_letter` (interop §10).
- Transient (network/5xx/timeout) → exponential backoff + jitter, bounded. Permanent business-rule 4xx →
  no blind retry; move to `failed`, operator correction.
- `401` → single token refresh + one replay, then backoff.
- Correction = PUT same `id` (new `meta.versionId`), never a new id, never fabricating local truth as synced.
- **Tag: READY** (pattern reused from existing outbox; interop §10); not exercisable end-to-end until G2.

### 3.8 Consent / privacy limitations
- No FHIR `Consent` resource; authorization = 6-digit code in SATUSEHAT Mobile (non-emergency) or emergency
  mechanism, platform-enforced (S9). SIGAP authors no `Consent` resource and **never** logs the code.
- SIGAP `user_roles.facility_id` scoping stays the boundary for SIGAP-owned data; SATUSEHAT reads are
  additionally gated by the patient's Mobile grant. `super_admin` must not become a global medical-records
  reader (F5 hard restriction).
- Patient-visible access history is owed by the platform layer (SATUSEHAT Mobile); SIGAP does not design a
  direct access-history endpoint (re-opens the enumeration vector).
- Safe default for consent UX: **deep-link out** to SATUSEHAT Mobile — never collect the code inside SIGAP
  (D3 — owner decision).
- **Tag: READY** for the boundary rules; **NEEDS-OWNER** for D3 (consent UX).

---

## 4. Synthetic contract test map (from regional-interop-contracts.md §2)

The bounded adapter interfaces in `regional-interop-contracts.md` are DESIGN-ONLY today. Every contract can
and should be exercised **now**, on synthetic data only, with fake/in-memory implementations. **No contract
test may touch a real SATUSEHAT endpoint or a real credential.**

| Adapter interface (§2) | Synthetic contract test list | Data | Status |
|---|---|---|---|
| `PatientIdentityMapper` (§2.1) | (a) `Resolve` returns fixture row; asserts **phone is never a resolver key**; (b) demographic-only hint routes to `NeedsHumanLink` (never auto-merge); (c) `Store` **versions** rows and never overwrites an existing IHS assignment; (d) `LookupLocal` never performs outbound I/O | Fictional `Pasien Demo 01`, NIK `3201010101000001`, IHS `P-PILOTSARI-0001` | **LOCAL-SYNTHETIC-CONTRACT** |
| `FacilityIdentityMapper` (§2.2) | (a) `Onboard` records the five-step order (auth → Organization → Location → Practitioner IHS → Patient IHS) with **no network I/O**; (b) `StoreMapping` writes synthetic org/location/practitioner ids; (c) `Lookup` returns cached refs, never outbound | `Organization/Org-PILOTSARI-DINKES-0001`, `Location/Loc-PILOTSARI-F1-POLI-UMUM-0001`, `Practitioner P1000000001` | **LOCAL-SYNTHETIC-CONTRACT** |
| `FHIRContractValidator` (§2.3) | (a) `ValidatePatient` — IHS/NIK identifier systems present; no fabricated content; (b) `ValidateEncounter` — mandatory refs (subject/serviceProvider/participant/location) present; status ∈ {planned, in-progress, finished, cancelled}; (c) `NormalizeTime` — WIB→UTC+00 out, UTC preserved in; (d) `Version` — correction re-submits original `id` with new version, never a new id | Synthetic `FhirPatient` + `FhirEncounter` JSON (R4 wire shape) | **LOCAL-SYNTHETIC-CONTRACT** |
| `ConsentGate` (§2.4) | (a) `Check` returns a pure decision over (actor scope × purpose) — no code ever enters; (b) `RecordEvidence` logs outcome only (authorized/denied, basis, patient IHS, facility) via `audit.Service`; (c) invariant asserted: the 6-digit code appears **nowhere** in decision or audit `metadata` (PII/PHI forbid-list) | `ConsentDecision`: authorized=false, basis=`no-platform-grant`; authorized=true, basis=`patient-grant-return` | **LOCAL-SYNTHETIC-CONTRACT** |
| `AuditProvenanceRecorder` (§2.5) | (a) `Emit` appends a synthetic interop event to `audit_events` with **FacilityID set** (P5 invariant), PII/PHI sanitized, hash-chain fields populated by existing `audit.Service`; (b) `Snapshot` returns SATUSEHAT id + versionId for a pushed resource so corrections reuse the original id | Synthetic row: action=`satusehat.patient_ihs_resolve`, resource_type=`patient_identity_mapping`, resource_id=`P-PILOTSARI-0001`, facility_id=F-1 | **LOCAL-SYNTHETIC-CONTRACT** |

**Blocked, never instantiated:** `FHIRClient` (CreatePatient/SearchPatient/CreateEncounter) and the
credentialed `Resolve` (NIK MPI search over the wire) are `FUTURE-AUTHORIZED-SATUSEHAT` — blocked on P6/P7
(G1–G3, G8). They are stubbed and return a "not authorized / credential missing" error by construction.

---

## 5. Overall readiness

| Dimension | Tag |
|---|---|
| Authentication (OAuth client_credentials) | **BLOCKED** — no credential (G2) |
| Sandbox access method | **BLOCKED** at credential gate; method itself READY (G4) |
| Resource support | **READY** — documentation-level, complete (S5) |
| Facility mapping (Organization/Location) | **BLOCKED** — onboarding requires G2 (G5) |
| Patient mapping (IHS/NIK) | **BLOCKED** + **NEEDS-OWNER** — G2; NIK not stored in pilot (D1) |
| Encounter lifecycle | **READY** — contract shape defined; live create blocked on G2 |
| Error handling | **READY** — outbox/retry/idempotency pattern defined |
| Consent/privacy | **READY** — boundary rules; **NEEDS-OWNER** for D3 UX decision |

**Non-goals (unchanged).** No production credential, no real patient data (including real NIK), no
fabrication of clinical content, no bypass of SATUSEHAT authorization/onboarding, no weakening of SIGAP's
existing facility-scoped RBAC.
