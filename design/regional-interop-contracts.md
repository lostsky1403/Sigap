# Regional Interop Contracts — Adapter Interfaces for the SIGAP Regional Pilot

> Status: **DESIGN document**. It defines interfaces and contracts; it is not a
> claim that any of them are live against SATUSEHAT.
>
> **Honesty markers (authoritative).** Every contract below is tagged exactly one of:
> - **PRODUCTION-INTEGRATED** — runs today, backed by real schema (`0001`–`0011`),
>   real Go symbols, or real audit rows. RUNNING.
> - **LOCAL-SYNTHETIC-CONTRACT** — a pure interface + value type that can be
>   exercised **today** against **synthetic data only**, with a fake/in-memory
>   implementation. No external dependency. DESIGN-ONLY (no backing table/column
>   yet), but self-contained and testable.
> - **FUTURE-AUTHORIZED-SATUSEHAT** — a contract bound to SATUSEHAT that can only
>   run **after** Kemenkes-issued credentials, partner registration, and owner
>   policy decisions exist. DESIGN-ONLY and **blocked**; none of it is instantiated.
>
> **PROPOSED** and **POLICY-DECISION-REQUIRED** are DESIGN-ONLY sub-labels and do not
> exist in the codebase or database today.

This document extracts the **actual prerequisites** for the regional pilot
(`design/regional-pilot-proposal.md` §7) and turns each into a named adapter
interface with a concrete **synthetic** record. It does not re-derive the
SATUSEHAT research or the tenancy design; it only names the contracts. Source of
truth for the research is `design/satusehat-interoperability-plan.md` (interop plan);
source of truth for tenancy/scoping is `design/regional-architecture-rfc.md` (RFC).

Sources (quoted verbatim where a table/column/symbol is real):
`packages/db/migrations/*.sql`, `packages/db/seed/rbac.sql`,
`apps/api/internal/{identity,auth,audit,handler}`, `apps/queue-engine/src/engine/queue.rs`.

---

## 0. Hard negations — read these first

These are invariants, not options.

1. **No fake live SATUSEHAT connection.** There is no SATUSEHAT client instantiated
   today — no sandbox call, no production call, no credentialed fetch, no OAuth
   `client_credentials` flow. The sandbox only becomes reachable after the interop
   plan §14 gates (partner registration → SSP verification → `client_id`/`client_secret`
   issuance → fasyankes data update) are met. Any code path named
   `FUTURE-AUTHORIZED-SATUSEHAT` in this document is **stubbed out and never wired**;
   it returns a "not authorized / credential missing" error by construction.
2. **No real patient records.** Every patient, NIK, IHS number, facility, org,
   practitioner, and visit in this document is a **fictional placeholder** on the
   fictional `.pilotsari.example` / "Kabupaten Pilotsari" district. No contract-test
   fixture resolves to a real person, real NIK, real phone number, or real facility.
   (Real facility master data comes from the deployment owner and, for interop, from
   DFO/REGFASYANKES/RS Online — never from this document.)
3. **No blockchain.** Audit tamper-evidence is the existing PostgreSQL append-only
   **hash chain** (`audit_events.previous_hash` / `event_hash`), which is *not* a
   distributed ledger, has no consensus, and no multi-organization dispute
   resolution. See `design/blockchain-necessity-matrix.md` (recommendation: **No**;
   Option A — append-only hash-chained audit — already live). No DLT node, no
   smart contract, no token appear anywhere.

---

## 1. Actual prerequisites (extracted, not re-researched)

Condensed from `regional-pilot-proposal.md` §7 (P1–P7) and interop plan §14/§7. Each
is a **gate**, not a self-serve task. The right-hand column names the contract
(§2) whose tag depends on it.

| # | Prerequisite | Status | Contract it gates |
|---|---|---|---|
| P1 | `regions`/`organizations` tables + `facilities.organization_id` | PROPOSED (`0012`) | `FacilityIdentityMapper` (local store path) |
| P2 | `regional_sysadmin` role + org-scope auth resolution | PROPOSED (`0013`, RFC Phase 2) | `ConsentGate` boundary (org axis) |
| P3 | `user_roles.organization_id` + partial indexes | PROPOSED (`0013`) | `ConsentGate` boundary (org axis) |
| P4 | Per-facility timezone `facilities.timezone` | PROPOSED (service-day policy §2.3) | `FhirEncounter` period conversion (multi-zone) |
| P5 | Tenant-aware audit + `Event.FacilityID` attribution fix | PROPOSED (`0014`, capability N5) | `AuditProvenanceRecorder` |
| P6 | SATUSEHAT sandbox credentials | **NOT available** (interop §14) | every `FUTURE-AUTHORIZED-SATUSEHAT` impl |
| P7 | Owner decisions: NIK collection/retention (D1), consent UX (D3), KMS keys (D4), first use-case module (D5) | **POLICY-DECISION-REQUIRED** | `PatientIdentityMapper` (NIK), `ConsentGate`, encryption at rest |

The **pilot go/no-go** (`regional-pilot-proposal.md` §10) explicitly excludes the
patient-identity + FHIR roadmap and SATUSEHAT sandbox interop; those are gated on
P6/P7. Therefore every contract in §2.1–§2.5 that touches SATUSEHAT is
**DESIGN-ONLY** for the pilot.

---

## 2. Adapter contracts

Package convention: the bridge module described in interop plan §7 / RFC ID-2 is
`apps/api/internal/satusehat/` (does **not** exist today; DESIGN-ONLY). The Go below
is *contract shape* — it names types and methods to align implementations; import
paths are illustrative. Reuse existing symbols where possible:
`identity.Actor` / `identity.FacilityGrant`, `auth.AuthorizeFacilityMutation` /
`auth.AuthorizeFacilityRead` / `auth.AuthorizedFacilityIDsForPermission`,
`audit.Service` / `audit.LogEvent` / `audit.SanitizeMetadata`.

### 2.1 Patient identity mapping adapter

**Synthetic record** (all fictional; table `patient_identity_mappings` is PROPOSED —
no such table exists):

```
patients (existing, PROPOSED extension only):
  id = 11111111-1111-4111-8111-111111111101
  full_name = "Pasien Demo 01"          [synthetic]
  phone     = "+6281200000001"          [synthetic, non-routable]
  gender = "F", date_of_birth = "1990-01-01"   [synthetic]

patient_identity_mappings (PROPOSED):
  patient_id           = 11111111-1111-4111-8111-111111111101
  nik                  = "3201010101000001"        [synthetic; structurally valid, fictional]
  ihs_number           = "P-PILOTSARI-0001"        [synthetic]
  satusehat_patient_ref= "Patient/P-PILOTSARI-0001" [synthetic]
  source               = "local-synthetic-fixture"  // NOT "nik-mpi-search"
  resolved_at          = 2026-11-09T00:00:00Z      [synthetic]
```

> The only real-world dummy pairs are SATUSEHAT's *published sandbox* table
> (interop §14 item 6, e.g. `NIK 9271060312000001 → IHS P02478375538`). Those are
> **sandbox fixtures**, not real people, and may be used only under
> `FUTURE-AUTHORIZED-SATUSEHAT`, never in a `LOCAL-SYNTHETIC-CONTRACT` test and never
> as real data. This document uses its own fictional pair instead.

```go
// apis/api/internal/satusehat — contract shape (illustrative)

// PatientIdentityMapping is the local correlation row (PROPOSED table).
type PatientIdentityMapping struct {
    PatientID          string    // local patients.id (uuid)
    NIK                *string   // national id; collected ONLY under owner decision D1
    IHSNumber          string    // SATUSEHAT patient-ihs-number (source of truth)
    SatuSehatPatientRef string   // "Patient/<ihs>"
    Source             string    // "nik-mpi-search" | "demographic-match" | "sandbox-dummy" | "local-synthetic-fixture"
    ResolvedAt         time.Time // UTC
}

type PatientIdentityQuery struct {
    // Exactly one key is required. NIK is deterministic and preferred;
    // phone is NEVER a resolver key.
    LocalPatientID *string
    NIK            *string
    Demographic    *DemographicHint // name+DOB+gender fallback → routes to human link, never auto-merge
}

type PatientIdentityMapper interface {
    // Resolve obtains the national IHS for a locally-known patient.
    // FUTURE-AUTHORIZED impl = NIK MPI search over the wire.
    // LOCAL-SYNTHETIC impl = returns a fixture row, asserts "never derive from phone".
    Resolve(ctx context.Context, q PatientIdentityQuery) (PatientIdentityMapping, error)

    // LookupLocal returns the cached mapping keyed by local patient_id.
    // Never calls outbound. Implemented today by a fake over synthetic fixtures.
    LookupLocal(ctx context.Context, patientID string) (PatientIdentityMapping, bool, error)

    // Store appends a mapping or versions the existing row; it NEVER overwrites
    // an existing IHS assignment (source of truth is the MPI, interop §4 item 5).
    Store(ctx context.Context, m PatientIdentityMapping) error
}
```

- **PRODUCTION-INTEGRATED** (floor): `patients` local UUID + unique `phone` +
  `full_name`/`gender`/`date_of_birth`; `medical_records` links by `patient_phone`.
  Phone-based identity is the *only* live correlator; it is **not** a SATUSEHAT key.
- **LOCAL-SYNTHETIC-CONTRACT**: the `PatientIdentityMapper` interface + a fake store
  that (a) versions rows instead of overwriting, (b) rejects phone-as-IHS-key inputs,
  (c) routes demographic-only hints to a `NeedsHumanLink` result. Exercisable now,
  table absent.
- **FUTURE-AUTHORIZED-SATUSEHAT**: `Resolve` via NIK MPI search (interop §4);
  identifier systems `https://fhir.kemkes.go.id/id/nik` and
  `https://fhir.kemkes.go.id/id/patient-ihs-number`. Blocked on P6/P7.

### 2.2 Facility / organization identity mapping adapter

**Synthetic record** (fictional):

```
regions [PROPOSED]:        code="PILOTSARI", name="Kabupaten Pilotsari"      [placeholder]
organizations [PROPOSED]:  id=org-1, name="Dinas Kesehatan Pilotsari — placeholder", region_id=…
facilities [existing]:     F-1 "RSUD Pilotsari" (rumah_sakit),
                           F-2 "Puskesmas Pilotsari Kota" (puskesmas),
                           F-3 "Puskesmas Pilotsari Pesisir" (puskesmas)

OrgLocationMapping [PROPOSED table, synthetic row]:
  facility_id            = F-1
  organization_id        = org-1
  satu_sehat_org_ref     = "Organization/Org-PILOTSARI-DINKES-0001"      [synthetic]
  satu_sehat_location_ref= "Location/Loc-PILOTSARI-F1-POLI-UMUM-0001"    [synthetic]
  service_unit_id        = su-poli-umum
  practitioner_ihs_number= "P1000000001"                                 [synthetic]
```

```go
type FacilityIdentityMapping struct {
    FacilityID          string // facilities.id
    OrganizationID      string // organizations.id (PROPOSED)
    SatuSehatOrgRef     string // "Organization/<id>"
    SatuSehatLocationRef string // "Location/<id>" per service unit
    ServiceUnitID       string
    PractitionerIHS     string // per practitioners row, obtained at onboarding
}

type FacilityIdentityMapper interface {
    // Onboard runs the five ordered steps (interop §7/S4): auth → Organization →
    // Location → Practitioner IHS → Patient IHS. FUTURE-AUTHORIZED only; the local
    // synthetic impl records the order but performs no network I/O.
    Onboard(ctx context.Context, f FacilityIdentityMapping) error

    // StoreMapping persists org/location/practitioner ids back onto facility rows.
    // LOCAL-SYNTHETIC impl writes fixtures; no real DFO/REGFASYANKES lookup.
    StoreMapping(ctx context.Context, m FacilityIdentityMapping) error

    // Lookup returns the cached org/location refs for a facility; never outbound.
    Lookup(ctx context.Context, facilityID string) (FacilityIdentityMapping, bool, error)
}
```

- **PRODUCTION-INTEGRATED** (floor): `facilities` with free-text
  `kecamatan`/`kabupaten_kota`/`provinsi` and `service_units`/`practitioners`
  child rows. No `organizations`/`regions` table, no `organization_id` column,
  no SATUSEHAT org/location id on any row.
- **LOCAL-SYNTHETIC-CONTRACT**: `FacilityIdentityMapper` + a fake that enforces the
  onboarding *order* and stores synthetic org/location/practitioner ids under the
  PROPOSED `organizations`/`regions` shape. Exercisable now.
- **FUTURE-AUTHORIZED-SATUSEHAT**: actual `POST Organization`/`POST Location`,
  `GET Practitioner` IHS lookup, OAuth `client_credentials` bound to **exactly one**
  Organization ID (reject any other org with "resource cannot be accessed due to
  business rule", interop §7). Blocked on P6.

### 2.3 FHIR Patient / Encounter contract

**Synthetic records** (fictional; R4 wire shape, times UTC+00, dates ≥ 2014-06-03):

```jsonc
// FhirPatient [synthetic]
{
  "resourceType": "Patient",
  "identifier": [
    { "system": "https://fhir.kemkes.go.id/id/nik", "value": "3201010101000001" },
    { "system": "https://fhir.kemkes.go.id/id/patient-ihs-number", "value": "P-PILOTSARI-0001" }
  ],
  "name": [ { "text": "Pasien Demo 01" } ],
  "gender": "female",
  "birthDate": "1990-01-01"
}

// FhirEncounter [synthetic] — appointment/queue_ticket → Encounter
{
  "resourceType": "Encounter",
  "identifier": [ { "system": "https://fhir.kemkes.go.id/id/encounter-id", "value": "enc-PILOTSARI-F1-0001" } ],
  "status": "planned",                         // planned → in-progress → finished | cancelled
  "subject":         { "reference": "Patient/P-PILOTSARI-0001" },
  "serviceProvider": { "reference": "Organization/Org-PILOTSARI-DINKES-0001" },
  "participant": [ { "individual": { "reference": "Practitioner/P1000000001" } } ],
  "location":  [ { "location": { "reference": "Location/Loc-PILOTSARI-F1-POLI-UMUM-0001" } } ],
  "period": { "start": "2026-11-09T01:00:00Z" } // converted from 08:00 WIB on the way out
}
```

```go
// Minimal FHIR R4 value shapes; contract, not a full IG.
type FHIRPatient struct {
    ResourceType string            `json:"resourceType"` // "Patient"
    Identifier   []FHIRIdentifier  `json:"identifier"`
    Name         []FHIRHumanName   `json:"name"`
    Gender       string            `json:"gender"`
    BirthDate    string            `json:"birthDate"`
}

type FHIREncounter struct {
    ResourceType    string                  `json:"resourceType"` // "Encounter"
    Identifier      []FHIRIdentifier        `json:"identifier"`
    Status          string                  `json:"status"`
    Subject         FHIRReference           `json:"subject"`
    ServiceProvider FHIRReference           `json:"serviceProvider"`
    Participant     []FHIRReference         `json:"participant"`
    Location        []FHIRReference         `json:"location"`
    Period          FHIRPeriod              `json:"period"`
}

// FUTURE-AUTHORIZED-SATUSEHAT: the credentialed wire client. NOT instantiated today.
type FHIRClient interface {
    CreatePatient(ctx context.Context, p FHIRPatient) (id, versionID string, err error)
    SearchPatient(ctx context.Context, q FHIHSearch) ([]FHIRPatient, error)
    CreateEncounter(ctx context.Context, e FHIREncounter) (id, versionID string, err error)
}

// LOCAL-SYNTHETIC-CONTRACT: validates + versions synthetic FHIR payloads, no network.
type FHIRContractValidator interface {
    ValidatePatient(p FHIRPatient) error     // IHS identifier system present; no fabricated content
    ValidateEncounter(e FHIREncounter) error // mandatory refs present; status ∈ {planned,in-progress,finished,cancelled}
    NormalizeTime(t time.Time, zone *time.Location) (utc time.Time, err error) // WIB→UTC+00 out, preserve UTC in
    Version(id, versionID string, payload []byte) error                        // correction = PUT same id, new version
}
```

- **PRODUCTION-INTEGRATED** (floor): `appointments`/`queue_tickets` anchored by
  `facility_id` with `service_unit_id`/`practitioner_schedule`; `medical_records` is
  a signed *attendance* record, **not** a clinical record. No FHIR, no `Encounter` table.
- **LOCAL-SYNTHETIC-CONTRACT**: `FHIRContractValidator` — asserts mandatory
  elements, IHS/NIK identifier systems, status machine, timezone conversion, and
  versioned correction (never "new id on correction", interop §9). Exercisable now
  with the synthetic JSON above and the synthetic visit fixture.
- **FUTURE-AUTHORIZED-SATUSEHAT**: `FHIRClient` over the api-catalogue FHIR R4 base
  (D2 — exact base URL confirmed at integration time, not hard-coded). Blocked on P6.

### 2.4 Consent / access contract

SATUSEHAT consent is **not** a FHIR `Consent` resource (interop §3 C7 / §6): the RME
authorization model is a **6-digit code** in SATUSEHAT Mobile, with an
**emergency** path, both platform-enforced. SIGAP therefore authors **no** `Consent`
resource and never logs the 6-digit code. The contract is a *gate* over the outcome.

**Synthetic record** (fictional; no code, no clinical content):

```
ConsentDecision [synthetic]:
  authorized = false,  basis = "no-platform-grant",
  patient_ihs = "P-PILOTSARI-0001", facility_id = F-1, decided_at = 2026-11-09T00:00:00Z

ConsentDecision [synthetic — only after a platform grant returns]:
  authorized = true,   basis = "patient-grant-return" (deep-link-out, code never enters SIGAP)
  patient_ihs = "P-PILOTSARI-0001", facility_id = F-1, decided_at = 2026-11-09T00:00:00Z
```

```go
type ConsentDecision struct {
    Authorized bool
    Basis      string // "patient-grant-return" | "emergency" | "denied-no-grant"
    PatientIHS string
    FacilityID string
    DecidedAt  time.Time // UTC
}

type ConsentRequest struct {
    Actor     identity.Actor // provenance-aware principal
    PatientIHS string
    FacilityID string
    Purpose   string         // purpose-bound consent ledger key (ID-1)
}

type ConsentGate interface {
    // Check returns the consent decision for reading a patient's national RME.
    // It NEVER receives or returns the 6-digit code; the platform is the authority.
    // LOCAL-SYNTHETIC impl: pure decision over (actor scope × purpose) with no code.
    Check(ctx context.Context, r ConsentRequest) (ConsentDecision, error)

    // RecordEvidence logs the outcome only (authorized/denied, basis, patient IHS,
    // facility) via audit. Never the code, never clinical content.
    RecordEvidence(ctx context.Context, d ConsentDecision) error
}
```

- **PRODUCTION-INTEGRATED** (floor): facility-scoped RBAC — `AuthorizeFacilityMutation`
  / `AuthorizeFacilityRead` / `HasPermissionAtFacility`; `super_admin` global *only*
  via `user_roles.facility_id IS NULL`; and `medical_records` has **no reader
  endpoint** and **no `medical_record.read` permission**. This is the running access
  boundary; it does not widen into any clinical read.
- **LOCAL-SYNTHETIC-CONTRACT**: `ConsentGate` as a pure decision + audit-outcome
  recorder. Enforced invariant: the 6-digit code never appears in the decision or in
  audit `metadata` (PII/PHI forbid-list). Safe default is **deep-link out** to
  SATUSEHAT Mobile (D3), so SIGAP never collects the code.
- **FUTURE-AUTHORIZED-SATUSEHAT**: the actual patient-RME read gated by the
  SATUSEHAT-Mobile grant/emergency path. Blocked on P6/P7.

### 2.5 Audit provenance contract

SATUSEHAT exposes **no** `Provenance`, **no** `AuditEvent`, **no** `Consent` resource
(interop §3 C6/C7/C8). SIGAP's own `audit_events` is therefore **authoritative** for
provenance. The contract is an extension of the existing `audit.Service`.

**Synthetic record** (fictional; appended to the real append-only table shape):

```
audit_events [existing table; synthetic interop row]:
  actor_user_id   = <principals from §2.5 pilot proposal>   # e.g. "admin-f1@pilotsari.example" [synthetic]
  action          = "satusehat.patient_ihs_resolve" | "fhir.encounter.write" | "fhir.onboard" | "consent.read.outcome"
  resource_type   = "patient_identity_mapping" | "encounter" | "organization" | "consent_decision"
  resource_id     = "P-PILOTSARI-0001"     [synthetic]
  facility_id     = F-1                     [synthetic; MUST be set — fixes N5 gap]
  organization_id = org-1                   [PROPOSED, migration 0014]
  region_id       = PILOTSARI               [PROPOSED, migration 0014]
  metadata        = { "satusehat_id": "<id>", "version_id": "<v>" }  // bookkeeping only, PII/PHI-free
```

```go
// audit.Service already exists (PRODUCTION-INTEGRATED). The interop contract is the
// event shape it writes, extended by PROPOSED columns.
type InteropAuditEvent struct {
    ActorUserID    string
    Action         string // one of: satusehat.patient_ihs_resolve | fhir.encounter.write | fhir.onboard | consent.read.outcome | …
    ResourceType   string
    ResourceID     string
    FacilityID     string          // MUST be set (P5); NULL only for legitimate region-scope actions
    OrganizationID string          // PROPOSED (0014)
    RegionID       string          // PROPOSED (0014)
    SatuSehatID    string          // SATUSEHAT resource id (bookkeeping, not PHI)
    VersionID      string          // SATUSEHAT meta.versionId
}

type AuditProvenanceRecorder interface {
    // Emit appends one interop event to audit_events with FacilityID set, PII/PHI
    // sanitized, and the hash-chain fields (previous_hash/event_hash) populated by
    // the existing audit.Service.
    Emit(ctx context.Context, e InteropAuditEvent) error

    // Snapshot returns the SATUSEHAT id + versionId for a pushed resource so a
    // correction can re-submit with the ORIGINAL id (interop §9), never a new id.
    Snapshot(ctx context.Context, resourceType, localID string) (satuSehatID, versionID string, ok bool, err error)
}
```

- **PRODUCTION-INTEGRATED**: `audit_events` (`0004`) append-only + hash chain;
  `audit.Service.NewService` / `LogEvent` / `SanitizeMetadata` / `sanitizeEvent`
  with the PII/PHI `forbidList`. RUNNING today.
- **LOCAL-SYNTHETIC-CONTRACT**: `AuditProvenanceRecorder.Emit` populating the
  existing columns plus the PROPOSED `organization_id`/`region_id`, against the
  synthetic record above; the `FacilityID`-set invariant is asserted in contract tests
  (P5 fix). Exercisable now for shape; the columns are DESIGN-ONLY.
- **FUTURE-AUTHORIZED-SATUSEHAT**: none — provenance is **not** delegated to a FHIR
  `Provenance`/`AuditEvent` (they do not exist in the catalogue). SATUSEHAT-side
  access records (emergency trail) are surfaced to the *patient* via SATUSEHAT Mobile,
  not to SIGAP (interop §11).

---

## 3. RUNNING vs DESIGN-ONLY split

**RUNNING (PRODUCTION-INTEGRATED) today:**

| Capability | Symbol / schema |
|---|---|
| Facility-scoped RBAC (mutation + read provenance gates) | `auth.AuthorizeFacilityMutation`, `auth.AuthorizeFacilityRead`, `identity.HasPermissionAtFacility` |
| `super_admin` global only when `user_roles.facility_id IS NULL` | `auth.facilityScopeQuery` / `seed/rbac.sql` |
| No medical-record reader endpoint; no `medical_record.read` key | capability F5; RFC §3 |
| Append-only hash-chained audit + PII/PHI sanitizer | `audit_events` (`0004`), `audit.Service` |
| Flat facility schema (`facilities → service_unit`), no `organizations` | `0001`–`0011` |
| Phone-keyed `patients` + `medical_records` by `patient_phone` | `0001`/`0002` |

**DESIGN-ONLY (both sub-labels below are not built):**

- **LOCAL-SYNTHETIC-CONTRACT** (self-contained, testable now on synthetic data):
  `PatientIdentityMapper` (+ fake store + versioning rule), `FacilityIdentityMapper`
  (ordering rule + fixtures), `FHIRContractValidator` (mandatory elements / status /
  timezone / versioned correction), `ConsentGate` (outcome-only, no code), and the
  `AuditProvenanceRecorder` extension shape. Backing tables/columns
  (`patient_identity_mappings`, `organizations`, `regions`,
  `facilities.organization_id`, `user_roles.organization_id`, tenant-audit columns)
  are PROPOSED, not present.
- **FUTURE-AUTHORIZED-SATUSEHAT** (blocked on P6/P7, never instantiated):
  `Resolve` via NIK MPI search, OAuth `client_credentials` token, Organization/
  Location/Practitioner onboarding over the wire, `FHIRClient.CreatePatient/
  SearchPatient/CreateEncounter`, and the patient-RME read gated by SATUSEHAT Mobile.

The single most important boundary is unchanged from the RFC/pilot: **no
administrative scope — including the PROPOSED `regional_sysadmin` — can read
`medical_records`**, because no reader endpoint and no clinical permission key exist
(RFC §3; pilot §9). Every contract above is *additive* on top of that floor and
never widens it.

---

## Non-goals (unchanged)

No production SATUSEHAT credential, no real patient data, no fabrication of clinical
content, no bypass of SATUSEHAT authorization/onboarding, no weakening of SIGAP's
existing facility-scoped RBAC, and **no blockchain** for audit integrity (the
existing hash chain suffices — `blockchain-necessity-matrix.md`).