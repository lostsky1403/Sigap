# SIGAP — Product Positioning

> Status legend used throughout this document:
> - **IMPLEMENTED** — live in this repository today, with the code path named.
> - **PROPOSED** — planned/roadmap direction; not yet built.
> - **POLICY-DECISION-REQUIRED** — an open governance/legal decision the deployment owner must make; not asserted as resolved.

---

## 1. One-line positioning

> **SIGAP is the regional civic-health *access-and-operations* layer — multi-facility service discovery, appointment booking, check-in, and queue management — that interoperates with SATUSEHAT/FHIR instead of re-implementing the national medical-record infrastructure.**

In one sentence: **SIGAP owns the *citizen's journey into care and the facility's front door*; SATUSEHAT owns the *national record and the national data highway* — and SIGAP never claims the second.**

---

## 2. What SIGAP is — and is not

SIGAP is an open-source, civic-tech service for a **region** (a district/city health office, a cluster of puskesmas, or a hospital group). Its job is to make the **entry point into healthcare** digital, transparent, and calm: a citizen finds the right facility, books a slot, checks in, and follows their queue; a facility operator runs queues, schedules, and onboarding across facilities.

SIGAP is **not** an Electronic Medical Record (RME) system. It is not a national patient registry, not a national interoperability hub, and not a replacement for SATUSEHAT. The medical record that a clinician writes is created and held where the care happens; SIGAP's role ends once the citizen is "in the door and in the queue," and — PROPOSED — it reports the *operational encounter* to SATUSEHAT as a standard FHIR message, never owning the clinical record of truth.

This positioning is the product's sharpest constraint: **every feature must make access and operations better, without duplicating what the national platform already standardizes.**

---

## 3. Competitive and ecosystem positioning

### 3.1 SATUSEHAT Platform (Kementerian Kesehatan national integration hub)

- **What it is:** the national FHIR-based integration platform. It is the canonical endpoint for patient identity (IHS), facility master data, and health-data exchange at national scale.
- **SIGAP's relationship:** **consumer and contributor, not a competitor.** SIGAP is the *regional edge* that generates the encounters and appointments the national hub expects. PROPOSED: an `interop` adapter maps SIGAP's canonical model to **FHIR R4** (`Patient`, `Organization`, `Location`, `Appointment`, `Encounter`, `ServiceRequest`) and posts to SATUSEHAT behind an adapter so non-Indonesian deployments can disable it.
- **The boundary:** SIGAP never mints national identifiers, never serves as an alternative national endpoint, and never holds itself out as the canonical registry. It sends to SATUSEHAT; it does not mirror SATUSEHAT.

### 3.2 SATUSEHAT Mobile / RME (national citizen app + medical-record standard)

- **What it is:** the citizen-facing national app with a personal medical-record wallet, built on the RME record standard.
- **SIGAP's relationship:** **adjacent, non-overlapping.** The wallet and the longitudinal record are the citizen's *national* identity surface. SIGAP's citizen surface is *regional operations*: "which facility near me has a slot, and what is my queue number." A citizen may use both; SIGAP does not attempt to be a second record wallet.
- **The boundary:** SIGAP must not duplicate the record-wallet experience or store the longitudinal record. It holds only the operational minimum it needs to run a visit (phone as the booking handle, a transient check-in code, a queue ticket) and — IMPLEMENTED — audits and sanitizes that data carefully rather than amassing a shadow medical record.

### 3.3 Existing hospital information systems (HIS)

- **What they are:** the facility-internal systems many hospitals already run (billing, pharmacy, inpatient, clinician RME).
- **SIGAP's relationship:** **complementary front-door, not a replacement HIS.** A facility keeps its HIS for clinical and revenue work; SIGAP adds the *citizen-facing* and *regional-operation* layer the HIS often lacks: public service discovery, self-service booking/check-in, and a shared queue board a receptionist can operate alongside the HIS.
- **The boundary:** PROPOSED — interop with HIS/RME happens through **standard exchange (FHIR/SATUSEHAT) and import/export, not through re-implementing HIS internals.** SIGAP does not aim to be a billing, pharmacy, or laboratory system.

### 3.4 Regional health services (Dinas Kesehatan / puskesmas clusters)

- **What they are:** the regional public-health operators SIGAP is built for.
- **SIGAP's relationship:** **this is SIGAP's home.** It gives a region a single, standards-aware operational surface across many facilities — one discoverable catalog, one booking flow, one view of queues — where today a region often has per-facility silos or none at all.
- **The boundary:** SIGAP is a *service* for regions, not a mandate on them. Onboarding is per-facility and consent-driven (IMPLEMENTED facility activation/deactivation), and integration with the national platform is the region's gateway to compliance, not a replacement for it.

---

## 4. Ownership boundary — SIGAP vs SATUSEHAT

The rule that keeps SIGAP honest: **own the last mile, hand off the highway.**

| Capability | Owner | Status |
|---|---|---|
| Regional facility & service-unit **catalog / discovery** | **SIGAP** | IMPLEMENTED (`catalog`, `/faskes`) |
| **Appointment booking** with capacity + rate limiting | **SIGAP** | IMPLEMENTED (`booking.go`) |
| **Digital check-in** code → queue number | **SIGAP** | IMPLEMENTED (`check-in` → Rust engine) |
| **Queue generation & status** (`waiting→called→…`) | **SIGAP** | IMPLEMENTED (`queue.go`, Rust engine) |
| **Admin queue board & schedules** (facility-scoped) | **SIGAP** | IMPLEMENTED (`/admin/queues`, `/admin/schedules`) |
| **Facility onboarding** (create/update/deactivate, service units) | **SIGAP** | IMPLEMENTED (`admin.go`) |
| Identity, facility-scoped RBAC, audit | **SIGAP** | IMPLEMENTED (`auth`, `identity`, `audit`) |
| Regional consent capture & operational audit trail | **SIGAP** | IMPLEMENTED (audit) / partially PROPOSED (consent receipts) |
| **National patient index / IHS** (identifier registry) | **SATUSEHAT** | out of scope for SIGAP |
| **FHIR national endpoint / exchange hub** | **SATUSEHAT** | out of scope for SIGAP |
| **RME electronic medical record** (canonical clinical record) | **SATUSEHAT / facility RME** | out of scope for SIGAP |
| **Citizen longitudinal record wallet** | **SATUSEHAT Mobile** | out of scope for SIGAP |
| SIGAP → SATUSEHAT **encounter/appointment reporting (FHIR R4)** | **SIGAP adapter** (sends to SATUSEHAT) | **PROPOSED** (roadmap Phase 8) |
| Facility master-data sync to SATUSEHAT | **SIGAP adapter** | **PROPOSED** |

**Non-duplication rule (explicit):** SIGAP's canonical data model is SIGAP-native. It adopts **FHIR/RME-compatible identifiers and value sets** and emits standards-compliant messages **only at the integration boundary**. It does **not** build a parallel SATUSEHAT endpoint, a parallel patient registry, or a parallel record wallet. Anything the national platform or a facility RME already standardizes is **reported to, not reimplemented by, SIGAP.**

---

## 5. The differentiator, stated precisely

SIGAP's defensible position is the **regional citizen access + facility operations** layer, proven by a small, correct, already-shipping surface:

1. **Regional citizen access** — one discoverable catalog and one booking/check-in flow across *multiple* facilities, not a single-facility kiosk.
2. **Complete book → check-in → queue operations loop** — a citizen books (gets a check-in code), checks in (code validates into a queue number), and the ticket flows onto an operator's queue board. End to end.
3. **Multi-facility service discovery and onboarding** — facilities and service units are onboarded, scoped, and deactivated as data-driven records, not hardcoded UI.
4. **Standards-compliant integration, not record re-implementation** — the deliberate positioning: SIGAP speaks SATUSEHAT/FHIR at the boundary and stays out of the national record business.

---

## 6. Proof points — implemented surface today

These are the concrete, already-live capabilities that substantiate the positioning (all **IMPLEMENTED**, cited to code):

- **Multi-facility service discovery** — public catalog endpoint (`internal/handler/catalog.go`: `ListPublicFacilities`, `ListPublicServiceUnits`) exposes a minimal, non-sensitive facility/service-unit shape; the citizen-facing page (`/faskes`) searches and filters facilities by type.
- **Public appointment booking** — `POST /api/v1/appointments` (`internal/handler/booking.go`: `BookAppointment`) validates the facility/service unit/slot, enforces per-slot capacity, rate-limits **2 bookings/day per phone**, and returns a **6-character alphanumeric check-in code**.
- **Digital check-in code → queue number** — `POST /api/v1/appointments/{id}/check-in` (`CheckIn`) atomically transitions `scheduled→checked_in→queued` (single `UPDATE … WHERE status='scheduled'` to kill the TOCTOU race), then calls the Rust queue engine over gRPC to mint the ticket number.
- **Transactional queue generation** — the Rust engine (`apps/queue-engine/src/engine/queue.rs`) performs `SELECT … FOR UPDATE` on `daily_queue_counters` so concurrent bookings cannot double-issue a number; the Go gateway rate-limits and the engine owns the atomic write.
- **Admin queue board** — `GET/PATCH /api/v1/admin/queues…` (`internal/handler/admin.go`: `ListQueueTickets`, `UpdateQueueStatus`) with a facility-scoped state machine; the operator UI (`/admin/queues`) auto-refreshes and stays scoped to the actor's facilities.
- **Facility onboarding & schedules** — create/update/deactivate facilities, service units, and practitioner schedules (`admin.go`), each mutation guarded by RBAC and written to the audit log.
- **Security substrate that makes regional scoping credible** — deny-by-default routing, pluggable auth (dev/JWT), facility-scoped authorization with live permission provenance, and privacy-safe audit that redacts phone/NIK/name (`internal/auth`, `internal/identity`, `internal/audit`, `SanitizeMetadata`).

These proof points anchor the positioning in what is *already true*: the **book → check-in → queue** loop and the **multi-facility operational surface** are shipping. What is **not** yet shipping is the SATUSEHAT/FHIR bridge itself — that is the direction, and it must be labeled honestly as PROPOSED (below).

---

## 7. The standards-integration direction — PROPOSED, not live

The differentiator's fourth leg ("standards-compliant integration") is a **commitment to a direction, not a claimed capability**. Concretely:

- **PROPOSED — FHIR R4 façade + SATUSEHAT bridge** (roadmap Phase 8; master plan interop module): an `interop` mapping layer that translates SIGAP's canonical model to FHIR resources and posts `Appointment`/`Encounter`/`Patient` references to SATUSEHAT via OAuth2 client-credentials, behind an adapter so non-Indonesian deployments can disable it.
- **PROPOSED — FHIR-ready identifiers**: adopting NIK / BPJS / SATUSEHAT IHS as identifier systems so SIGAP can correlate a citizen without minting its own national identifier.
- **No live SATUSEHAT integration exists today.** No claim to the contrary belongs in any document, README, or CTA. The roadmap treats it as a later, separately-gated phase, not a shipping feature.

---

## 8. Policy decisions the deployment owner must make

These are governance choices, recorded here so the product is not silently over-claimed:

- **POLICY-DECISION-REQUIRED** — Whether a given region's SATUSEHAT integration is *mandatory* or *optional* for SIGAP deployments (the adapter design assumes optional/disableable).
- **POLICY-DECISION-REQUIRED** — Which identifier the region treats as the booking handle (phone today) and when/where a national identifier (NIK/IHS) is required, given UU PDP privacy obligations.
- **POLICY-DECISION-REQUIRED** — Consent scope: which operational events (encounter start, appointment) are authorized to flow to SATUSEHAT, captured under which consent purpose.
- **POLICY-DECISION-REQUIRED** — Division of labor with existing facility HIS/RME: what stays in the HIS and what SIGAP surfaces, so SIGAP is never operated as a shadow record system.

---

## 9. Summary

SIGAP wins by **owning the front door and the queue**, not the chart and not the highway: regional multi-facility discovery, self-service booking and check-in, and operator queue management — shipped today, secured by default — with SATUSEHAT/FHIR integration treated as a *standard hand-off at the boundary*, explicitly not as a reimplementation of Indonesia's national medical-record infrastructure.