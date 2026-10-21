# SIGAP V2.2 — product evolution: what ships now, what needs backend work

This document separates three classes of change, as the brief requires:

1. **Supported by existing APIs** — implemented in this branch.
2. **Requires backend / API / database work** — specified here, not implemented.
3. **Requires a privacy or product-policy decision** — specified with the decision
   spelled out, not implemented.

Nothing below is implemented speculatively. Everything in class 2 and 3 is a
proposal awaiting owner approval.

---

## 1. Implemented in this branch (class 1)

| Change | Why it matters | Endpoint |
|---|---|---|
| Check-in code alphabet excludes `I O 0 1` | The owner's P0. A code read off a screen and typed into a phone was unreadable, and a faithful transcription failed. | `POST /api/v1/appointments` (generation) |
| Confusable-tolerant lookup, exact match first, ambiguity refused | Codes already issued keep working; a `0`/`O` or `1`/`I` substitution still resolves. | `GET /api/v1/patient/status` |
| Whitespace inside a code is stripped, not rejected | "AB 12 CD" is what a person types when reading a code aloud. | both layers |
| Status response carries `queue_formatted_number` | The page read a field the API never sent, so a checked-in citizen saw no ticket number. | `GET /api/v1/patient/status` |
| `in_queue` no longer renders as "Dilayani" | It means *waiting*. Every checked-in citizen was told they were already being seen. Now distinguished by the ticket's own status. | `queue_status` on the same response |
| Walk-in success screen no longer offers a dead-end status link | The status endpoint resolves check-in codes only; a queue number returns 404 by design. | — |
| Not-found state has a recovery path | Previously it restated the failure and stopped. | — |
| Booking validates only the visible step | Five errors, four of them for hidden fields. | — |
| Admin document titles | All six were empty. | — |
| Focus moves to the first invalid field | Check-in, walk-in, status. | — |
| Facility type error uses human labels | Printed `rumah_sakit, puskesmas` while the select said "Rumah sakit". | — |
| Walk-in name error matches its label | Said "Nama pasien" for a field labelled "Nama lengkap". | — |
| Auth availability announced before typing | The 503 only appeared after a submit, described as a transient outage. | — |
| Admin preview identity banner | States on screen that the admin screens act as a seeded test subject. | — |

---

## 2. Requires backend work (class 2) — NOT implemented

### 2.1 Check-in time window

**Defect it closes.** `POST /api/v1/appointments/{id}/check-in` validates the code
and `status = 'scheduled'` only. An appointment three days out can be checked in
today, which issues a live queue ticket for a future visit and makes
`estimated_wait_minutes` meaningless for everyone in the real queue.

**Proposed contract.**

```
POST /api/v1/appointments/{id}/check-in
  409 { "error": "Check-in dibuka pada <ISO8601>." }   # too early
  409 { "error": "Waktu check-in telah berakhir." }     # too late
```

**Rule.** Open `CHECKIN_OPENS_BEFORE` (default 2h) before `appointment_time`, and
close `CHECKIN_CLOSES_AFTER` (default 4h) after it. Both configurable, both read
from the facility row so a large hospital can differ from a puskesmas.

**Schema.** `facilities.checkin_opens_minutes int NOT NULL DEFAULT 120`,
`facilities.checkin_closes_minutes int NOT NULL DEFAULT 240`. Nullable-free
defaults keep the migration safe on existing rows.

**Acceptance.** A booking 3 days out is refused with the first error and a
timestamp; one 30 minutes out is accepted; one 5 hours past is refused with the
second. The existing happy-path tests keep passing.

**Why not implemented now.** The window length is a policy decision with a real
operational consequence (a patient who arrives early must be turned away, and a
clinic that runs late must still admit them). Choosing it silently would be
worse than the current behaviour.

### 2.2 Walk-in status lookup by ticket token

**Gap.** A walk-in patient has no appointment and therefore no check-in code.
`/patient/status` cannot resolve their ticket, and it must not start accepting
`PHB-0004` — that sequence is enumerable and the endpoint exists to close that
vector.

**Proposed contract.** `POST /api/v1/queues/generate` already returns `TicketID`
(a UUID). Add:

```
GET /api/v1/patient/queue-status?token=<TicketID>
  200 { facility_name, formatted_number, queue_number, status, people_ahead }
  404 for an unknown token
```

**Why a UUID is safe here.** It is 122 bits of randomness, non-sequential, and
already returned to the citizen who created it — the same bearer-token model as
the check-in code. It must be the ONLY accepted key; the formatted number stays
refused.

**Privacy risk.** `people_ahead` is a live count and is operationally
informative; it must be computed per facility per day and must not expose other
patients' rows. `facility_name` and the ticket number are already on the
citizen's own slip.

**Acceptance.** A freshly generated ticket resolves; the same token after
`completed` returns the terminal state; a formatted number returns 404; a random
UUID returns 404; 30 req/min per IP.

### 2.3 Appointment cancellation and rescheduling by the citizen

**Gap.** A citizen cannot cancel or move their own appointment. `/admin/*` can,
but that is an operator tool. The Pen draws no citizen cancellation frame, and
no endpoint exists.

**Proposed contract.**

```
POST /api/v1/appointments/{id}/cancel   { checkin_code }
POST /api/v1/appointments/{id}/reschedule { checkin_code, appointment_time }
```

**Authorization.** The check-in code is the bearer token, exactly as for
check-in. This is the same security model, not a new one.

**Constraint.** Rescheduling must re-run the slot-capacity and
`uq_active_booking_per_patient_day` checks; a reschedule that violates either
must fail with the same 409 the booking path returns.

**Policy decision required.** Whether cancellation is allowed after check-in
(the ticket already exists in the queue). Recommendation: no — once checked in,
the facility controls the ticket.

### 2.4 Slot availability

**Gap.** The booking form accepts any `datetime-local`, including 03:00 on a
Sunday. `practitioner_schedules` already stores `schedule_date`, `start_time`,
`end_time`, `slot_minutes`, `capacity_per_slot`, and `appointments` already
consumes capacity — the data is there, but no endpoint exposes the free slots.

**Proposed contract.**

```
GET /api/v1/public/schedules?facility_id=&service_unit_id=&date=
  200 { slots: [{ start, capacity, booked, available }] }
```

**Privacy.** Counts only. No patient identifiers, no names, no booking ids.

**Acceptance.** A slot at capacity reports `available: 0` and is not offered;
the booking endpoint still enforces capacity independently (the client is never
the authority).

### 2.5 Notification delivery visibility for the citizen

`notification_outbox` records delivery attempts per appointment, but nothing
lets a citizen know whether their confirmation was actually sent. A citizen who
never received the SMS has no way to tell whether the booking exists.

**Proposed.** Surface `confirmation_sent: boolean` on the booking response,
derived from the outbox row. Read-only; no new write path.

---

## 3. Requires a policy decision (class 3) — NOT implemented

### 3.1 How a citizen recovers a lost check-in code

Today the guidance is "contact the facility", which is honest but depends on a
phone line that may not exist. The alternatives each carry a real tradeoff:

| Option | Tradeoff |
|---|---|
| Send the code by SMS to the booking phone | Requires an SMS provider and a rate limit; the phone number is the only secret, and it is often shared within a family. |
| Email the code | Requires an email field the booking flow does not collect. |
| Re-issue a code from the appointment id | The UUID is on the confirmation screen only; anyone holding it can already check in. |
| Operator lookup at the desk | Current behaviour. Works, but only in person. |

**Recommendation.** SMS re-send, gated on the booking phone matching exactly and
rate-limited to one per appointment per hour. It reuses the notification outbox
that already exists. It needs an owner decision because it costs money per
message and creates a new enumeration surface (phone → appointment existence).

### 3.2 Whether a queue number may ever be shown to someone who does not hold
the ticket

Any change here reopens the enumeration vector that `vuln-0003` closed. The
recommendation is to keep the answer "no" and rely on §2.2's ticket token
instead.

---

## 4. Deliberately not done

- **No fake data.** Every figure on every screen comes from a response. Where a
  value does not exist, the screen says so rather than inventing one.
- **No new dependency.** Everything above uses the existing stack.
- **No silent behaviour change.** The two behavioural changes in class 1
  (whitespace absorption, confusable folding) are both additive: they accept
  more input and reject nothing that previously worked, except an ambiguous
  fold, which previously could not occur.
