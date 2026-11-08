# SIGAP V2.2 — capability gap analysis

What the product can actually do today, what the backend already supports but the UI
does not surface, and what would need real work. Every claim below was checked against
the source; the file:line references are the evidence.

**Nothing here is a promise.** Items in *NEXT RELEASE* and *FUTURE* are proposals and
need owner approval before implementation.

---

## 1. What exists, as measured

### Citizen surface — the complete public API

| Method | Path | Handler | What it returns |
|---|---|---|---|
| POST | `/api/v1/appointments` | `booking.go:76` | appointment id, **check-in code**, time, status |
| POST | `/api/v1/appointments/{id}/check-in` | `booking.go:423` | queue ticket + formatted number |
| GET | `/api/v1/patient/status?code=` | `patient.go:63` | facility name, appointment status, queue number + status |
| GET | `/api/v1/public/facilities` | `catalog.go:59` | id, name, short code, type, active |
| GET | `/api/v1/public/service-units?facility_id=` | `catalog.go:94` | service units of one facility |
| POST | `/api/v1/queues/generate` | `queue.go:39` | walk-in queue ticket |
| GET | `/api/v1/facilities/nearby` | `main.go:476` | hardcoded list |
| GET | `/api/v1/events/beds` | `main.go:365` | SSE stream |

That is the whole citizen surface. There is **no** `GET /api/v1/appointments` — a citizen
cannot list their own appointments, because there is no identity to list them against.

### The honest summary

| | |
|---|---|
| Endpoints the citizen can reach | 8 |
| Endpoints behind admin permission | 26 |
| Tables with no reader endpoint | `medical_records`, `audit_events` |
| Dead client helpers pointing at undeclared routes | `myAppointments()` → `/api/v1/patient/appointments` (`citizen.ts:20`) |

---

## 2. Feature matrix

### QUICK WINS — the backend already supports it; only UI work

| # | Feature | Backend evidence | Work | Risk |
|---|---|---|---|---|
| Q1 | **Show the appointment time and queue number on the status page** | `PatientStatusLookup` returns `queue_formatted_number` and `queue_status`; the page renders both | Already shipped | none |
| Q2 | **Explain the check-in code on the booking confirmation** so a citizen can find it again | `booking.go:70` returns `checkin_code`; the confirmation renders it | Copy + hierarchy work | none |
| Q3 | **Link the booking confirmation directly into check-in** with id + code prefilled | `CheckinForm` already accepts both; the booking page already builds the deep link (`+page.svelte:360`) | Already shipped | none |
| Q4 | **Client-side facility search + type filter** | `GET /api/v1/public/facilities` returns the full catalog; the page already filters locally | Already shipped | none |
| Q5 | **Queue transition buttons that name the action** (Panggil / Layani / Lewati) | `admin.go:1057-1064` transition map; already shipped | Already shipped | none |
| Q6 | **Notification retry / cancel** | `notifications.go:432,490`; `service.go:276-330` | Already shipped | none |
| Q7 | **Schedule create + update** | `admin.go:1839,1915`; slot validation `admin.go:2197-2211` | Already shipped | none |
| Q8 | **Show *why* a lookup failed, distinctly from "not found"** | The API returns 400/404/429/5xx distinctly; `errors.ts` already models them | Mapping already correct — verified by `verify-status-error-map.mjs` | none |

### NEXT RELEASE — bounded backend work, no new security model

| # | Feature | What exists | What is missing | Value | Complexity |
|---|---|---|---|---|---|
| N1 | **Retrieve more of my appointment without an account** | `checkin_code` is a 6-char unguessable token (32^6 ≈ 1.07e9); the status lookup already resolves by code and, as of v2.3, returns `service_unit_name` | Bounded: the status response still deliberately withholds `appointment_time` (PHI-adjacent correlation) — do NOT re-add it; any retrieval stays code-only and non-enumerating | Medium | **S** — additive fields only, rate-limited |
| N2 | **Cancel my appointment — IMPLEMENTED (v2.3)** | `POST /api/v1/appointments/cancel` ships: scheduled-only, idempotent, audited, shared 30/min limiter, guarded transition | n/a | Shipped | — |
| N3 | **Reschedule my appointment** | `appointments.appointment_time` is mutable; the admin path already updates it | A citizen route keyed by code | High — today a citizen must phone the facility | **M** — needs slot validation against `practitioner_schedules`, and the same rule the admin editor enforces |
| N4 | **Staff check-in that issues a queue number — IMPLEMENTED (v2.3)** | `PATCH /api/v1/admin/appointments/{id}/status` to `checked_in` now mints and atomically attaches a queue ticket through the queue service, guarded by `queue_ticket_id IS NULL` + optimistic `AND status=$current`, rolling back to `scheduled` on failure; covered by a concurrency test | n/a | Shipped | — |
| N5 | **Read the audit trail** | `audit_events` exists and every admin action writes to it (`audit/service.go:97`) | A `GET /api/v1/admin/audit` scoped by facility; `audit.read` is granted to `super_admin` only | High for administrators; accountability for a civic system | **M** — plus a **data fix**: admin rows never set `Event.FacilityID` (`admin.go:733-742`), so facility filtering cannot work on existing rows |
| N6 | **Facility queue capacity as a setting** | `DAILY_QUEUE_LIMIT = 300` is a Rust constant (`queue.rs:10,113`) | A per-facility cap column + enforcement | Medium — one busy clinic and one quiet one cannot share a constant | **M** — schema change, engine change, migration |
| N7 | **Notification template management** | `notification_templates` table exists with `channel` CHECK (dev/sms/whatsapp/email) | No create/update route (`router.go:74-76`) | Medium | **M** — needs the template lifecycle and validation |
| N8 | **Server-side queue filtering by facility** | The handler's doc comment claims it (`admin.go:763-765`) but parses no query | Query parsing + scope-aware filter | Medium — the UI filters locally, which breaks past a page of rows | **S** |
| N9 | **Service-unit creation from the admin UI — IMPLEMENTED** | `createServiceUnit` posts to the registered `/api/v1/admin/service-units` with `facility_id` in the body | n/a | Shipped | — |

**Walk-in status retrieval — PROPOSED, not implemented.** A walk-in ticket has no
check-in code, so a walk-in citizen cannot retrieve their visit status today, and
lookup by `formatted_number` would re-introduce the enumeration vector that was
deliberately removed. Proposal: at walk-in issuance, mint an opaque, unguessable
token (crypto-random, stored beside the ticket, NEVER derived from the number),
accept it on the same code-only, rate-limited `/patient/status` path, and expire
it at end of service day. Must NOT expose patient name/phone through that path.
Deferred until the privacy design is written down; do not silently add it.

### FUTURE — needs a product and security decision first

| # | Feature | Why it is not a quick win |
|---|---|---|
| F1 | **Citizen accounts / appointment hub** | Requires an identity provider that actually works. Supabase is configured but not provisioned in this environment; every "my appointments" design depends on that decision, plus a data-retention policy for health data. |
| F2 | **Digital visit pass with QR** | A QR encodes the check-in code. Rendering it is trivial; deciding whether a screenshot of it should be sufficient at a counter is a security decision, and the answer changes the design. |
| F3 | **QR-assisted check-in** | Same token as F2, plus a scanner at the facility. Needs hardware and an offline-failure story. |
| F4 | **Citizen notifications / reminders** | The outbox exists but only `DevProvider` ships (`cmd/notification-worker/main.go:6-10`) — sms/whatsapp/email are schema-valid and **undeliverable**. Choosing a provider is a procurement and privacy decision. |
| F5 | **Medical record access** | `medical_records` exists with a SHA-256 signature and **no reader endpoint**. Exposing it is a clinical-data decision, not an engineering one. |
| F6 | **Practitioner catalogue and per-practitioner booking** | No practitioner endpoint exists at all; the schedule editor deliberately omits `practitioner_id`. Adds a new scope dimension (today the only scope is `user_roles.facility_id`). |
| F7 | **Wait-time estimates** | The engine returns `estimated_wait_minutes`, but no queue-depth model is exposed and the number is currently a constant. Showing it without a real model would be a fabricated claim. |

---

## 3. Where the product is thinner than it looks

Four gaps that matter more than the feature count suggests:

1. **A citizen cannot get back to their own appointment.** The check-in code is the only
   key, it is shown once on the confirmation, and there is no lookup that returns it.
   Every other gap is smaller than this one.
2. **The desk cannot check anyone in.** Admin check-in sets the status and stops. The
   queue number is only minted by the citizen's own check-in.
3. **Nobody can read the audit log**, and the rows are not facility-attributed, so even a
   future reader could not scope them.
4. **Nothing is configurable per facility** — not capacity, not templates, not the
   catalogue beyond what already exists.

---

## 4. What was deliberately not built

- **No appointment list for citizens.** There is no identity to scope it to. Building one
  against a phone number would put a patient lookup behind a guessable identifier.
- **No QR anywhere.** See F2/F4.
- **No wait estimate on the status page.** The value exists in the engine response but is
  a constant today; printing it would be inventing a number.
- **`/wallet` stays an honest empty state.** `GET /api/v1/medical-records` does not exist;
  the previous version fabricated a QR block and a signature claim.

---

## 5. Approval needed

| Decision | Why it needs the owner |
|---|---|
| Which of N1–N9 to build next | Each is a product choice, not a technical one |
| Whether citizens get accounts (F1) | Sets the shape of everything else in the citizen column |
| Which notification provider (F4) | Procurement + data-processing agreement |
| Whether a screenshot of a visit pass is acceptable at a counter (F2) | Security policy |
