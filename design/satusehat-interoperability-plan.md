# SATUSEHAT Interoperability Plan — SIGAP

**Status:** PROPOSED (design). Nothing in this document is implemented in `F:\Sehat-Hub` / `F:\sigap-design-v2`. It is a
specification for **ROADMAP.md — Phase 8 "Live SATUSEHAT Integration"**. Where a paragraph is followed by
`[IMPLEMENTED]`, `[PROPOSED]`, or `[POLICY-DECISION-REQUIRED]`, that tag is authoritative and overrides any reading that
would imply the capability exists today.

**Scope.** How SIGAP (SvelteKit web + Go API + Rust queue engine + Postgres) would exchange health data with the national
SATUSEHAT platform operated by Kementerian Kesehatan RI. This is an **engineering/pre-integration design**, not a
live integration. It does **not** attempt to obtain or use any SATUSEHAT credential, does **not** touch production patient
data, and does **not** bypass SATUSEHAT onboarding (see §11 Sandbox PoC).

---

## 1. Evidence base (primary sources)

All support claims below are cited against the SATUSEHAT documentation portal (version observed **7.23**) or the
Kemenkes launch announcement. No claim in the support matrix is assumed; each is tied to a specific URL.

| # | Source | URL |
|---|---|---|
| S1 | SATUSEHAT Platform landing (integration overview, HL7 FHIR + HTTPS REST, ISO 27001/27799) | https://satusehat.kemkes.go.id/platform/ |
| S2 | Playbook — Introduction ("Apa itu SATUSEHAT"; HIE; single identifier / IHS number) | https://satusehat.kemkes.go.id/platform/docs/id/playbook/introduction/ |
| S3 | FHIR — overview (FHIR R4, resources, prerequisites vs interoperability framework) | https://satusehat.kemkes.go.id/platform/docs/id/fhir/ |
| S4 | FHIR — Prerequisites / Onboarding (auth → Organization → Location → Practitioner IHS → Patient IHS) | https://satusehat.kemkes.go.id/platform/docs/id/fhir/prerequisites/ |
| S5 | FHIR — Interoperability resource list (canonical list of 55 supported FHIR resources) | https://satusehat.kemkes.go.id/platform/docs/id/fhir/resources/ |
| S6 | Panduan Interoperabilitas — use-case modules (Modul Pelayanan + Modul Penerapan) | https://satusehat.kemkes.go.id/platform/docs/id/interoperability/ |
| S7 | ReST API — Autentikasi (OAuth 2.0 `client_credentials`; sandbox/production token endpoints) | https://satusehat.kemkes.go.id/platform/docs/id/api-catalogue/authentication/ |
| S8 | FHIR — Patient resource (IHS number, MPI, NIK identifier system, sandbox dummy patients) | https://satusehat.kemkes.go.id/platform/docs/id/fhir/resources/patient/ |
| S9 | Kemenkes — "Luncurkan SATUSEHAT RME" (national RME, patient consent, 6-digit code, emergency consent) | https://www.kemkes.go.id/id/kemenkes-luncurkan-satusehat-rme-rekam-medis-pasien-terintegrasi-secara-nasional |

SIGAP-side evidence (internal, live schema):

| # | Source | Path |
|---|---|---|
| I1 | Capability gap analysis (current citizen/admin surface, medical_records/audit gaps) | `design/sigap-v2-capability-matrix.md` |
| I2 | Product evolution (implemented vs backend-work vs policy-decision classes) | `design/sigap-v2-product-evolution.md` |
| I3 | Roadmap (Phase 8 names SATUSEHAT integration) | `ROADMAP.md` |
| I4 | DB schema (`patients`, `medical_records`, `facilities`, `practitioners`, `audit_events`) | `packages/db/migrations/*.sql` |

---

## 2. What the platform is (summary, per S1–S3)

- SATUSEHAT is the national **Health Information Exchange (HIE)** connecting health information systems (fasyankes,
  regulator, payer, digital-service providers) under the Indonesia Digital Health Transformation Blueprint 2024 (S2).
- Technical standard: **HL7 FHIR** data model **plus HTTPS REST API** (S3); FHIR "R4" profiles published on Simplifier
  (`SATUSEHAT-FHIR-R4-Implementation-Guide`, S8).
- It issues a **SATUSEHAT/IHS number** — the *single identifier* for a patient's health information — so care is
  continuous across facilities and is stored **individually, not per-facility** (S2, S9).
- Data exchange is **server-to-server** and credentialed; authentication is OAuth 2.0 `client_credentials` (S7, §7).

What SATUSEHAT is *not*: it is not a full medical-records editor for SIGAP, it does not expose a FHIR
`Consent`/`Provenance`/`AuditEvent` resource in its interoperability catalogue (§3), and patient access authorization is
enforced at the platform/RME layer via SATUSEHAT Mobile, not via a FHIR resource author by the integrating app (S9, §5).

---

## 3. VERIFIED FHIR support matrix

**Method.** "Supported" = the resource appears in the canonical interoperability resource list (S5) **and** has a dedicated
profile page + a ReST api-catalogue entry. This is **documentation-level verification**, performed against S5; we hold no
credentials and performed no live sandbox call (see §11). "No" = the resource does **not** appear in S5 and no profile page
exists for it; it is therefore **unsupported or, at best, unverified** for SIGAP. Resource slugs below resolve under
`https://satusehat.kemkes.go.id/platform/docs/id/fhir/resources/<slug>/`.

| FHIR resource | Supported | Doc cite (S# / page) | SIGAP use case that consumes it | SIGAP status |
|---|---|---|---|---|
| `Patient` | **Yes** (GET search; demographic record via MPI) | S4, S8 (`resources/patient/`) | Resolve national patient identity (IHS) at registration/booking; store IHS against local patient | PROPOSED — `patients` has no IHS/NIK column (I4) |
| `Encounter` | **Yes** | S5 (`resources/encounter/`) | Model a visit/service episode from an appointment or queue ticket (encounter status `planned` → `in-progress` → `finished`) | PROPOSED — no encounter table (I4) |
| `Condition` | **Yes** | S5 (`resources/condition/`) | Record diagnosis/problem from a clinical visit | PROPOSED — `medical_records` stores no diagnosis (I4, I1 F5) |
| `AllergyIntolerance` | **Yes** | S5 (`resources/allergy-intolerance/`) | Patient allergy list, surfaced before prescribing/administration | PROPOSED |
| `MedicationStatement` | **Yes** | S5 (`resources/medication-statement/`) | Current/known medication history for a patient | PROPOSED |
| `MedicationRequest` | **Yes** | S5 (`resources/medication-request/`) | Prescription ordering during a visit | PROPOSED |
| `Observation` | **Yes** | S5 (`resources/observation/`) | Vital signs, triage and point-of-care measurements | PROPOSED |
| `DiagnosticReport` | **Yes** | S5 (`resources/diagnostic-report/`) | Lab / radiology results display (S9 lists lab + radiology among integrated data) | PROPOSED |
| `DocumentReference` | **Yes** | S5 (`resources/document-reference/`) | Attach/share clinical documents and RME resume | PROPOSED |
| `Organization` | **Yes** (onboarding prerequisite) | S4 (`resources/organization/`) | Facility master-data sync (SIGAP `facilities` → SATUSEHAT org) | PROPOSED |
| `Location` | **Yes** (onboarding prerequisite) | S4 (`resources/location/`) | Service-unit / sub-location sync (SIGAP `service_units`) | PROPOSED |
| `Practitioner` | **Yes** (onboarding prerequisite; GET IHS lookup) | S4 (`resources/practitioner/`) | Health-worker IHS mapping (SIGAP `practitioners`) | PROPOSED |
| `Provenance` | **No** — not in documented resource list | S5 (absence); no profile page found | — do not design against it | Not applicable |
| `AuditEvent` | **No** — not in documented resource list | S5 (absence) | SIGAP uses its own `audit_events` table (I4) instead | Not applicable |
| `Consent` | **No** — no documented FHIR `Consent` resource | S5 (absence); consent enforced via SATUSEHAT Mobile 6-digit code + emergency path (S9) | Patient authorization is a platform-layer access check, not a resource SIGAP authors | Not applicable |

**Additional supported resources (context, not targeted SIGAP use cases today).** S5 also documents: `Composition`,
`ClinicalImpression`, `CarePlan`, `Procedure`, `Medication`/`MedicationAdministration`/`MedicationDispense`,
`Immunization`, `QuestionnaireResponse`, `Specimen`, `ImagingStudy`, `ServiceRequest`, `EpisodeOfCare`,
`FamilyMemberHistory`, billing/coverage/claim resources, and others (full list in S5). These are recorded here only to
bound the integration surface; they are **not** in SIGAP's Phase-8 target set.

**Terminology / time rules confirmed during verification (apply to every resource in this plan).** Mandatory elements are
per-use-case (S8); timestamps are transmitted **UTC+00** (WIB − 7, WITA − 8, WIT − 9); transmitted dates must be
**≥ 2014-06-03** (S8). The patient IHS number uses identifier system `https://fhir.kemkes.go.id/id/patient-ihs-number`,
with NIK under `https://fhir.kemkes.go.id/id/nik` (S8).

---

## 4. Patient identity mapping (IHS vs SIGAP id)

**SATUSEHAT side.** National identity is the **IHS number** (`patient-ihs-number`), obtained from the national **Master
Patient Index (MPI)** and stored under a stable Identifier system. Demographics live in the MPI (name, DOB, address,
government-issued IDs), not in each facility's system (S8). Partner systems are **encouraged to cache the IHS number
locally** and thereafter reference the patient by IHS without resending demographics on every transaction (S8).

**SIGAP side (current, I4).** `patients` key is a local UUID, with `full_name`, unique `phone`, `gender`, `date_of_birth`.
There is **no NIK and no IHS column**, and `medical_records` links to patients by `phone` (I4). SIGAP's *only* reliable
correlator today is the phone number — which SATUSEHAT does **not** treat as an identity key.

**Proposed mapping (PROPOSED / POLICY-DECISION-REQUIRED for NIK collection).**

1. Add a correlation table (or column) — e.g.
   `patient_identity_mappings(patient_id UUID, nik TEXT, ihs_number TEXT, satusehat_patient_ref TEXT, source TEXT, resolved_at TIMESTAMPTZ)`
   — to bind SIGAP local `patient_id` ↔ SATUSEHAT `ihs_number`.
2. Resolution path (server-side, credentialed, never exposed to the citizen UI): **search SATUSEHAT Patient by NIK**
   (via MPI/FHIR GET, S4/S8) → read the returned `ihs_number` → persist the mapping. Do **not** derive IHS from phone alone.
3. Because SIGAP currently captures no NIK, NIK collection at booking/registration is a **POLICY-DECISION-REQUIRED**
   precondition: it is a new PII field, needs a lawful basis, and must not be made enumerable (the same class of concern
   already documented in I1/I2 for phone and ticket tokens).
4. Fallback when NIK is unavailable at the facility: demographic MPI lookup (name + DOB + gender) — this is **less
   deterministic** (duplicate risk) and must fall back to a human "link patient" step, never to silent auto-merge.
5. **Source of truth for identity** = SATUSEHAT MPI. SIGAP holds a cached projection keyed by IHS and re-resolves on
   mismatch/expiry; it must never overwrite the MPI's IHS assignment.

`[IMPLEMENTED]` note: SIGAP's phone-based identity and check-in-code bearer model remain unchanged for the *booking/queue*
surface in the near term; the IHS mapping is layered on top for clinical exchange only.

---

## 5. Encounter + facility mapping (Organization / Location)

Per S4, onboarding requires registering the facility **Organization** and its **Location(s)** before any clinical
transaction. Per S6 the base service modules are: Resume Medis — Rawat Jalan, IGD, Rawat Inap, Kefarmasian.

| SIGAP entity (I4) | SATUSEHAT FHIR resource | Mapping detail |
|---|---|---|
| `facilities` (fasyankes) | `Organization` | One SATUSEHAT Organization per SIGAP facility. The org's SATUSEHAT id is returned at onboarding and stored on the facility row. |
| `service_units` (or facility sub-location) | `Location` | Each service unit maps to a Location under that Organization; `Location.identifier`/name from the SATUSEHAT catalogue. |
| `practitioners` (I4) | `Practitioner` | Each health worker maps to a Practitioner IHS number (GET lookup during onboarding, S4). |
| `appointments` / `queue_tickets` | `Encounter` | A visit becomes an `Encounter` with `subject` = `Patient` (IHS), `serviceProvider` = `Organization`, `participant` = `Practitioner`, `location` = `Location`. `planned` appointment ⇒ `Encounter.status = planned`; check-in ⇒ `in-progress`; completed/no-show ⇒ `finished`/`cancelled`. |
| `medical_records` (visit/queue history) | `Encounter` + clinical resources | `medical_records` today is a *signed attendance record*, **not** a clinical record (I4). It becomes the local anchor for the Encounter; the clinical content (Condition/Observation/etc.) is **new**, PROPOSED. |

`[IMPLEMENTED]` — the citizen-side booking/queue surface (appointments, check-in, walk-in) is live and unchanged.
`[PROPOSED]` — everything that emits an FHIR Encounter/Organization/Location on the SATUSEHAT wire.

Facility master-data sync (S4, I3 "Facility master data sync") is **onboarding-time**, not per-visit: register the
Organization and Locations once, store the returned SATUSEHAT ids, and reference them in every downstream resource.

---

## 6. Consent + authorization boundaries

- **SATUSEHAT consent is not a FHIR `Consent` resource.** The RME authorization model (S9) is: in non-emergency cases a
  health worker accesses a patient's RME only **after the patient grants access via a 6-digit code** generated in
  SATUSEHAT Mobile; in an **emergency** where the patient cannot consent, a health worker may use an **emergency-consent
  mechanism**, and that access remains visible to the patient in SATUSEHAT Mobile (S9).
- Consequently SIGAP **must not** author or persist a `Consent` resource, and **must not** build its own consent override
  that treats "SIGAP user is logged in" as sufficient to read SATUSEHAT data. The 6-digit code / emergency path is the
  only authorization runway for reading a patient's national RME.
- **Boundaries SIGAP must enforce (POLICY-DECISION-REQUIRED where marked):**
  1. SIGAP's existing `user_roles.facility_id` scoping (I1) stays the boundary for *SIGAP-owned* data; SATUSEHAT reads are
     additionally gated by the patient's SATUSEHAT-Mobile grant.
  2. SIGAP `super_admin` **must not** become a global medical-records reader (reaffirms the hard restriction in I1 F5 and
     the project's "no global unrestricted medical-records admin" rule).
  3. A patient's right to see their own record is mediated by SATUSEHAT Mobile / the "Resume Medis" feature (S9), not by a
     new SIGAP public endpoint that would re-open the enumeration vector documented in I1/I2.

`[This paragraph = POLICY-DECISION-REQUIRED]` Whether SIGAP ever surfaces the 6-digit consent prompt in its own UI, or
simply deep-links the patient out to SATUSEHAT Mobile, is an owner decision; the safe default is **deep-link out**, never
collect the code inside SIGAP.

---

## 7. Authentication + onboarding prerequisites

Per S7 (authentication) and S4 (onboarding order), integration is gated and server-to-server:

**Authentication (S7).** OAuth 2.0, grant type `client_credentials` (server-to-server, no user login). Requires
`client_id` + `client_secret`, issued only after the RME system is **verified on SATUSEHAT Platform (SSP)** and the
fasyankes has updated its data in **DFO/REGFASYANKES/RS Online**. A credential pair is bound to **exactly one
Organization ID**; sending a different Organization ID is rejected with the business-rule error
`resource cannot be accessed due to business rule`. Credentials are secret, unique, and must not be duplicated or
redistributed.

- Token endpoints (S7):
  - Sandbox: `https://api-satusehat-stg.dto.kemkes.go.id/oauth2/v1`
  - Production: `https://api-satusehat.kemkes.go.id/oauth2/v1`
- The FHIR REST base URL is specified under the api-catalogue (S7 refers to "ReST API SATUSEHAT"; the exact FHIR R4 base
  path must be read from the per-resource api-catalogue pages at integration time — do not hard-code an unverified path).

**Onboarding order (S4).** Five ordered steps before clinical data can flow:
1. Authenticate to SATUSEHAT.
2. Register the **Organization** structure (`POST Organization`).
3. Register the **Location** structure (`POST Location`).
4. Obtain/store the **Practitioner IHS number** (`GET Practitioner`).
5. Obtain the **Patient IHS number** (`GET Patient` via MPI).

`[IMPLEMENTED]` — none of these SATUSEHAT calls exist in SIGAP today. `[PROPOSED]` — a bridge module
(`apps/api/internal/satusehat/` or equivalent) that performs onboarding + token refresh; this is the "Bridge module for
Kemenkes SATUSEHAT API" named in I3.

---

## 8. Data ownership + source of truth

| Domain | Source of truth | SIGAP holds |
|---|---|---|
| Patient demographic identity / IHS | SATUSEHAT MPI | Cached projection (IHS + minimal fields) via `patient_identity_mappings` |
| Patient clinical record (diagnosis, meds, allergies, results) | SATUSEHAT (once pushed) | Local copy, with SATUSEHAT `id` + `versionId` for every resource |
| Appointments, queues, local scheduling, check-in codes | SIGAP | Authoritative |
| Audit trail of SIGAP actions | SIGAP `audit_events` | Authoritative (SATUSEHAT exposes no `AuditEvent`) |

**Rule.** A fact becomes a SATUSEHAT fact only after a successful, acknowledged push. Before that moment it is a SIGAP
fact. SIGAP never re-issues its own identity for a patient it has already mapped to an IHS number. Ownership of the
*exchange* is shared; ownership of each *service's operational data* is the respective service's.

---

## 9. Clinical correction + versioning rules

- FHIR resources are versioned: SATUSEHAT assigns a `meta.versionId` on write. **Correction = update (PUT) the existing
  resource**, which mints a new version; it is **not** a delete (SATUSEHAT does not expose destructive clinical deletes in
  the documented catalogue). [Applies from S5/S8's versioned-resource framing; delete semantics not documented.]
- SIGAP sync bookkeeping must store, per pushed resource: SATUSEHAT `id`, `meta.versionId`, `last_sync_at`, `sync_status`.
  A correction produces an **additional row/version**, never a silent in-place overwrite of the serialized payload.
- Local-to-SATUSEHAT timestamps are converted WIB→UTC+00 on the way out and preserved as UTC on the way in (S8).
- **Never re-submit a corrected record as a new id** — that would duplicate the patient's record in the MPI-indexed space.
  Re-submit with the original `id` so SATUSEHAT versions it.
- If SATUSEHAT rejects a write, SIGAP surfaces a human-correctable error and re-queues (§10); it does **not** fabricate a
  fallback "local truth" presented as if synced.

---

## 10. Sync retries + failure handling

- **Idempotency.** Every outbound write carries a client-generated idempotency key (or reuses the SATUSEHAT resource `id`),
  so a dropped response can be retried safely without duplication.
- **Outbox pattern.** Reuse SIGAP's existing outbox model (cf. `notification_outbox`, I1/I2) as a `satusehat_outbox` with
  states `pending → submitted → acknowledged | failed | dead_letter`, processed by the queue consumer (Rust queue engine /
  Go worker). `[IMPLEMENTED]` for the *pattern only*; `[PROPOSED]` for a SATUSEHAT-specific outbox.
- **Retry policy.** Transient (network, 5xx, timeout) → exponential backoff with jitter, bounded retries. Permanent
  business-rule rejections (4xx, e.g. the wrong-Organization error in S7, invalid mandatory element per S8) → **no blind
  retry**; move to `failed` and require operator correction.
- **Time zone.** All queueing/scheduling of retries uses UTC; facility-local calendar semantics (the "service-day" WIB
  contract already in the product) are kept separate from SATUSEHAT wire timestamps.
- **Backpressure.** Batch/bundle pushes per encounter; prefer one `Transaction`/`Batch` bundle per visit so a visit is
  atomic from SIGAP's perspective, and replay the bundle on retry.
- **No credential handling in this path:** tokens are refreshed from the OAuth endpoint (S7) before expiry; a
  `401` triggers a single token refresh + one replay, then backoff.

---

## 11. Audit logging

- **SIGAP's own audit trail is authoritative for SIGAP actions.** `audit_events` already records admin actions (I1); the
  bridge must append events for: patient-IHS resolution (with NIK→IHS correlation), each outbound FHIR write (resource
  type, SATUSEHAT id, versionId), consent/authorization-gated reads (outcome only — **never** the 6-digit code), and
  onboarding mutations (Organization/Location registration).
- Known gap to fix (from I1 N5): admin audit rows currently do not set `Event.FacilityID`, so facility-scoped audit reads
  cannot work on existing rows. Any interop audit must set `FacilityID` from day one and add the facility-scope reader
  before interop ships.
- **Do not attempt to use SATUSEHAT FHIR `AuditEvent`** — it is not in the documented resource list (§3). SATUSEHAT-side
  access records (the emergency-consent trail) are surfaced to the *patient* via SATUSEHAT Mobile (S9), not to SIGAP.

---

## 12. Data minimization

- After the `patient_identity_mappings` row exists, subsequent transactions reference the patient by **IHS number only**
  and omit resending demographics (this is the documented pattern, S8) — the minimal payload.
- Send only the fields the target use case marks mandatory (S8: mandatory elements are per-use-case); do not sweep the
  whole patient record into every push.
- Store locally only what SIGAP needs to operate the booking/queue product plus the sync bookkeeping (`id`, `versionId`,
  status). Do not cache the full national RME locally; read-through on demand, gated by consent (§6).
- **No fabricated data.** Reaffirms the release contract (I2 §4): never invent clinical content to satisfy a field, and
  never present unsynced local state as synced.
- NIK is collected **only** for the single purpose of IHS resolution, retained minimally and always with the patient's
  consent/lawful basis (see §4 item 3).

---

## 13. Encryption + key management

- **Transport.** All SATUSEHAT exchange is HTTPS REST (S1/S3/S7); never plaintext HTTP. The OAuth `client_secret` is a
  bearer secret and is transmitted only to the token endpoint over TLS.
- **Secrets at rest (PROPOSED).** `client_id`/`client_secret` must be stored in SIGAP's secret store (KMS-backed, e.g. the
  existing `.env`/secret mechanism upgraded to a KMS), **never** checked into the repo or embedded in the binary. Rotate
  on a schedule and on any suspected disclosure. Bind the credential to its one Organization ID (S7) and refuse any request
  that targets a different Organization.
- **Clinical data at rest (PROPOSED / POLICY-DECISION-REQUIRED).** Any clinical fields SIGAP caches locally must be
  encrypted at rest with KMS-managed keys; keys live in the KMS, not in the Postgres database. Encrypt the identity
  mappings (especially `nik`), not just the container.
- **Existing `medical_records.signature` is integrity, not confidentiality.** The SHA-256 signature (I4) proves the record
  has not changed; it is **not** encryption and must not be described as such. It continues to serve as a local integrity
  anchor and is independent of SATUSEHAT versioning (§9).
- **Key separation.** Distinct keys for (a) transport/TLS, (b) API client secret, (c) field-level data encryption. No
  single key of record.

---

## 14. Sandbox PoC — not pursued here

**Precise statement.** A proof-of-concept against SATUSEHAT requires **legitimate sandbox credentials and approved access**,
issued by Kementerian Kesehatan only after partner registration, RME-system verification, and fasyankes data update. This
plan **does not pursue that PoC**, does **not** attempt to obtain or use production (or sandbox) credentials, does **not**
use real patient data, and does **not** bypass or shortcut onboarding. All verification above is documentation-level
against the public portal (S1–S9).

Concrete prerequisites that must exist before a PoC could even begin (each is a gate, not a task we can self-serve):

1. **Partner registration** — SIGAP registered and verified as an RME partner on SATUSEHAT Platform (SSP) (S7).
2. **Organizational approval** — Kementerian Kesehatan issuance of a `client_id`/`client_secret`, bound to one
   Organization ID (S7).
3. **Facility data updated** — the target fasyankes current in DFO/REGFASYANKES/RS Online (S7).
4. **Sandbox endpoints** — use only `https://api-satusehat-stg.dto.kemkes.go.id/oauth2/v1` (S7) and the sandbox FHIR R4
   base from the api-catalogue; **never** the production `https://api-satusehat.kemkes.go.id/…` endpoints.
5. **Onboarding run** — complete §7's five steps for a test Organization/Location/Practitioner (S4).
6. **Sandbox-only dummy patients** — use SATUSEHAT's published sandbox patient table (NIK/name/gender/birthDate/IHS, S8),
   e.g. `NIK 9271060312000001 → IHS P02478375538`; **no real NIK or clinical data**.
7. **Postman collection** — the official SATUSEHAT Postman collection/workspace for reference payloads (S6).
8. **Sign-off** — owner approval for NIK collection (§4) and data-retention policy for any cached clinical fields (§12/§13).

Until all eight gates are met, any "integration" claim is out of scope; SIGAP continues to operate its live booking/queue
surface unchanged.

---

## 15. Open decisions (owner sign-off required before implementation)

| # | Decision | Reference |
|---|---|---|
| D1 | Whether/where SIGAP collects NIK for IHS resolution, and its retention rule | §4.3 |
| D2 | Confirm the exact FHIR R4 base URL from the api-catalogue (not assumed) | §7 |
| D3 | Consent UX: deep-link out to SATUSEHAT Mobile vs surfacing the 6-digit code in SIGAP UI | §6 |
| D4 | Field-level encryption key management (KMS) for cached NIK/clinical fields | §13 |
| D5 | Which Phase-8 use-case module(s) to target first (RME Rawat Jalan vs IGD vs Kefarmasian) | §3/§5, S6 |

**Non-goals (unchanged from project constraints).** No production credential, no real patient data, no fabrication of
clinical content, no bypass of SATUSEHAT authorization/onboarding, no weakening of SIGAP's existing auth or
facility-scoped RBAC.