# Service-Day Queue Policy — Date Scoping, Timezone, and Carry-Over for SIGAP v2

> Status: **Design document**. Every section is tagged **IMPLEMENTED**,
> **PROPOSED**, or **POLICY-DECISION-REQUIRED**. PROPOSED and
> POLICY-DECISION-REQUIRED content does not exist in the codebase or database
> today; it is directional until an owner approves it. A hard rule throughout:
> **reversible access only** — the service day is expressed as an *additive*
> query parameter plus a WIB-time range filter. No schema migration, no data
> rewrite, no silent mutation of tickets is proposed or implemented.

Evidence is drawn from `apps/api/internal/handler/admin.go`
(`ListQueueTickets`, `resolveServiceDay`, `serviceDayWIB`),
`packages/db/migrations/0001_init.sql` (`queue_tickets`, `facilities`),
`apps/web/src/lib/api/endpoints/admin.ts` (`listQueueTickets`), and
`apps/web/src/lib/admin/ringkasan.ts` (`activeQueueCount`, `attentionItems`).

---

## 1. What "service day" means — IMPLEMENTED (API) / PROPOSED (UI affordances)

A **facility-local service date** is the calendar day on which a queue ticket was
registered, as understood by the facility that operates the queue. The board an
operator works is always *one* such day: it is never "all tickets ever".

There are two consumers of this notion today:

1. **The queue board** — `GET /api/v1/admin/queues`.
2. **The admin overview (Ringkasan)** — `apps/web/src/routes/admin/+page.svelte`,
   which derives its headline queue figures from the exact same list endpoint.

Both must agree on "today". The policy therefore defines a single shared default
and implements it in one place, the handler, so the two consumers cannot drift.

---

## 2. Timezone assumption — IMPLEMENTED (WIB default) / PROPOSED (per-facility column)

### 2.1 What the schema actually is — IMPLEMENTED

The `facilities` table (`0001_init.sql`) has **no timezone column**. The columns
that exist are `id, name, type, address, kecamatan, kabupaten_kota, provinsi,
phone, total_beds, available_beds, last_updated, is_active, short_code,
created_at, updated_at` — none of which encodes an IANA zone.

Indonesia runs a single-span, abuse-resistant simplification here: `Asia/Jakarta`
(WIB, UTC+7) has **no daylight-saving transition**. The handler therefore models
the service day against a fixed-offset location — `serviceDayWIB` in
`admin.go` — rather than loading tzdata at boot. This is exact for WIB and
removes a deployment-time tzdata dependency.

```go
var serviceDayWIB = time.FixedZone("WIB", 7*60*60)
```

### 2.2 The single-zone default — IMPLEMENTED

`Asia/Jakarta` (WIB) is the **documented single-zone default** for every facility.
A ticket's service day is the WIB calendar day containing its `registered_at`
instant. The filter is a half-open range `[start, start+24h)` computed in WIB:

```go
func resolveServiceDay(raw string) (start, end time.Time, err error)
```

- `date` omitted → `start` = today 00:00 WIB, `end` = tomorrow 00:00 WIB.
- `date` present → parsed as `YYYY-MM-DD` **in WIB**; `start` = that midnight, `end` = midnight + 1 day.
- malformed → the caller returns HTTP 400 and never issues a query.

### 2.3 Per-facility timezone — PROPOSED (future additive column)

A facility in WITA (UTC+8) or WIT (UTC+9) would currently have its service day
resolved against WIB. The correct fix is a **future additive
`facilities.timezone` column** (nullable text holding an IANA name), defaulting to
`Asia/Jakarta` on the read path. It is intentionally **not** part of this change:
the table has no such column today, and adding one would require an irreversible
schema decision plus a backfill policy. This document records the assumption and
the future seam; nothing is migrated.

---

## 3. Default board scope = today — IMPLEMENTED

`ListQueueTickets` defaults to **today in WIB** when `date` is omitted. Before
this change the handler returned every ticket the actor could see, with no date
bound, so the live board implicitly accumulated all-time history. The default now
filters `queue_tickets.registered_at` to the current WIB calendar day:

```text
WHERE registered_at >= $1 AND registered_at < $2   -- [WIB midnight, WIB midnight + 1 day)
  [AND facility_id = ANY($3)]                       -- existing coarse facility scope, unchanged
ORDER BY registered_at DESC, id DESC                 -- existing stable ordering, unchanged
```

Stable ordering is **not** changed. Stable scope is **not** changed: when the
actor's read set is restricted, the facility predicate is appended as an extra
`AND`, exactly as before.

---

## 4. Historical queue access — IMPLEMENTED (read-only UI)

An **explicit past date** (`?date=YYYY-MM-DD` for a date earlier than today)
answers the same endpoint with that day's tickets. The policy constraint:

- History is **read-only**. The `date` parameter only changes the *selection
  window*; it grants no write path, and `UpdateQueueStatus`
  (`PATCH /api/v1/admin/queues/{id}/status`) is orthogonal and unchanged.
- Selection never filters by `status`, so a prior-day ticket still marked
  `waiting`/`called`/`in_service` is **visible under its own day** rather than
  silently hidden.

The UI wiring **ships**: the admin queue board has a "Hari layanan" date
selector (`apps/web/src/routes/admin/queues/+page.svelte`) defaulting to today
WIB and capped at today (`max`), so a past date re-reads that day's history. In
a non-today scope the board is **read-only**: `readOnly` is threaded through
`QueueBoard` → `QueueBand` and empties each row's transition set, so no
`PATCH` action button is offered for a historical row. The backend contract this
builds on is the additive `?date` parameter pinned above.

---

## 5. Unresolved prior-day tickets — POLICY-DECISION-REQUIRED

The schema does not force a resolution, and it should not. When a ticket
registered on day N is still `waiting`/`called`/`in_service` when day N+1 begins,
four behaviours are possible. Two are ruled out, two are left to a product owner:

**Ruled out (do NOT do):**

- **DO NOT silently close or carry them forward.** Auto-transitioning a prior-day
  open ticket (e.g. to `cancelled`/`no_show`, or re-stamping it into today) would
  rewrite operational truth and destroy the audit trail. No such mutation is
  implemented or proposed here.
- **DO NOT hide them.** Hiding an open ticket makes a clinic look idle while a
  real patient is still unserved. The `date` filter intentionally does not add a
  status predicate.

**Recommended (needs an owner, not implemented):**

- Keep them **visible under their own day's history**, flagged in the UI as an
  *unresolved carry-over* (an open ticket whose service day is before today).
  This preserves visibility without pretending the ticket belongs to today.

**Open decision — POLICY-DECISION-REQUIRED:**

- Whether an unresolved prior-day ticket may still be **acted on** (called /
  served) from a historical view, or whether terminal resolution on a stale day
  requires an explicit operator override. The API currently permits a status
  transition by ticket id regardless of the ticket's day. **The shipped UI takes
  the safe default**: historical (non-today) views are read-only, so no action
  is offered until the owner decides whether to re-enable a same-day/override
  action. Choosing the safe default here is exactly what "do not silently decide
  policy" requires — the UI refuses rather than invents.

---

## 6. Correct Admin Overview aggregation — IMPLEMENTED

Ringkasan's queue figures (`activeQueueCount`, `attentionItems`) are computed in
`apps/web/src/lib/admin/ringkasan.ts` from the list returned by
`listQueueTickets()` — which calls `GET /api/v1/admin/queues` **without a
`date`**. Because the handler now defaults the no-`date` read to *today in WIB*,
the overview inherits the same today scope automatically. No separate aggregation
endpoint or all-time counter remains, so the overview can no longer report
all-time backlog as if it were today's activity.

- `activeQueueCount` counts `waiting` + `called` + `in_service` — everything not
  yet closed — but now only within **today's** service day.
- `attentionItems` derives a "Perlu perhatian" waiting count from the same
  today-scoped list.

**Scope is now visible.** The overview headline metric reads "Antrean aktif hari
ini" and the attention section is titled "Perlu perhatian hari ini (WIB)", so an
operator is not misled into reading a today-only figure as an all-time one. A
prior-day ticket still `waiting`/`called`/`in_service` is therefore **not
counted** on Ringkasan; it is visible only by selecting its own day on the board
(§4). This is a deliberate consequence of day-scoping, not a hidden record —
the day boundary is stated on-screen.

The frontend computation is unchanged; the correction is entirely in the shared
backend default, which is the point of defining the service day in one place.

---

## 7. Reversibility and safety — IMPLEMENTED

- **Additive only.** The change introduces one optional query parameter (`date`)
  and a WIB range filter. No existing query parameter is removed or redefined.
- **No migration.** `resolveServiceDay` and `serviceDayWIB` are pure Go. There
  is no new column, no enum, no data rewrite, and no index change.
- **No data mutation.** The only touched handler is a `SELECT`. Tickets are never
  updated, closed, or carried by this feature.
- **Rollback** is a revert of `admin.go` plus `admin_facility_scope_test.go`;
  nothing persisted depends on the change.

---

## 8. Number-allocation boundary — OPEN DIVERGENCE (documented, not fixed)

**Scope boundary:** this change scopes the *read* path (board list + Ringkasan)
to a WIB service day. It does **not** yet change the *write* path that mints
`queue_number` or enforces `DAILY_QUEUE_LIMIT`. Those stay on a UTC day. The two
boundaries therefore differ, and the consequence is real.

**What is UTC today:**

- Rust queue engine keys `daily_queue_counters` and enforces
  `DAILY_QUEUE_LIMIT = 300` with `chrono::Utc::now()` (`queue.rs`), i.e. the
  counter and cap day is **UTC**.
- Go fallback allocation stamps/resets with `registered_at::date = CURRENT_DATE`
  (`admin.go`, `booking.go`). The database session `TimeZone` is `UTC`
  (confirmed `SHOW timezone`), so this is also **UTC**.
- The one-active-booking-per-day guard (`uq_active_booking_per_patient_day`)
  backs on `appointment_day(timestamptz) = ($1::date)` (`0007_checkin_constraints.sql`),
  which is session-timezone-dependent despite being declared `IMMUTABLE`.

**Consequence (00:00–07:00 WIB):** UTC midnight is 07:00 WIB, i.e. mid-service-day.
A WIB board can therefore carry **two number series in one local day** (the
sequence resets at 07:00 WIB), and the 300 cap is enforced per UTC day (≈ twice
per WIB day). This is **not** a silent data rewrite — it is a genuine boundary
mismatch between the day used to *read* a board and the day used to *mint*
numbers — and it predates this change.

**Proposed fix (single authority) — PROPOSED, not implemented:** compute the
service date once, in Go, against `Asia/Jakarta`, and pass that value to (a) the
number allocator, (b) `ListQueueTickets`, and (c) the booking-per-day guard —
redefining `appointment_day` as a genuinely immutable `($1 AT TIME ZONE
'Asia/Jakarta')::date` with a reindex, or pinning the DB session `TimeZone`.

**Do NOT silently change the allocator/guard now.** It changes when the 300 cap
resets and how the one-booking-per-day invariant is bounded — an operational
policy decision with outage consequences. It is recorded here and in the
implementation backlog as a dependency-ordered item, not bundled into the
read-scope change.