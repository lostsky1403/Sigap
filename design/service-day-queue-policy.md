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

## 8. Number-allocation boundary — RESOLVED (allocation + cap + guard + listing share WIB)

**Status: IMPLEMENTED.** The UTC-vs-WIB allocation divergence is now closed. The
service day is defined once per language as the `Asia/Jakarta` (WIB, UTC+7, no DST)
calendar day containing the instant, and the *write* path now uses the same day as
the *read* path. No stored instant is shifted: `registered_at` and `appointment_time`
remain the actual UTC `timestamptz` values.

**The three expressions (the single authority, per language):**

- **Rust** (`apps/queue-engine/src/engine/queue.rs`): the counter key and
  `DAILY_QUEUE_LIMIT` day is computed from a fixed offset —
  `chrono::FixedOffset::east_opt(7*3600)` — via `chrono::Utc::now()
  .with_timezone(&offset).date_naive()`. No new dependency; `FixedOffset` is already
  in `chrono`. `visit_time` (stored as `registered_at`) stays `chrono::Utc::now()` —
  the unmodified UTC instant.
- **Go fallback allocator** (`apps/api/internal/handler/admin.go` `recordAdminQueueTicket`,
  `booking.go` `recordQueueTicket`): the UTC-sensitive `registered_at::date = CURRENT_DATE`
  subquery is gone. It is replaced by `serviceDayRange(time.Now())`, which computes the
  WIB `[start, end)` bounds in Go (`time.Now().In(serviceDayWIB)`, floor to midnight, `+1
  day`) and passes them as bound parameters — exactly the same shape `ListQueueTickets`
  reads via `resolveServiceDay`. No session-tz-dependent SQL cast remains in the allocator.
- **SQL guard** (`packages/db/migrations/0012_service_day_wib.sql`, additive — does not edit
  `0007`): `appointment_day(timestamptz)` is redefined as
  `(($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date`. The `AT TIME ZONE 'UTC'` makes the
  cast session-timezone-independent, so the function is *genuinely* immutable. The
  `uq_active_booking_per_patient_day` unique index is then recreated on the new expression.

**Consequence closed.** Because allocation, the 300 cap, the one-booking-per-day guard, and
the board listing all key on the WIB calendar day, no facility can carry two number series in
one WIB day (there is no 07:00-WIB counter reset), and the cap is enforced once per WIB day.
Reached boundary tests prove UTC midnight (07:00 WIB) no longer shifts the service day, and
WIB midnight remains the only boundary.

**Safety — additive and collision-guarded.** `0012` first SELECTs for active
`(facility_id, patient_phone, WIB-day)` groups with `count(*) > 1` under the new expression.
If any exist, the migration raises a NOTICE and **changes nothing** (function and index left
as-is) rather than performing a destructive rewrite; an operator must resolve those rows by
hand first. No historical `registered_at`/`appointment_time` is rewritten. Rollback is a revert
of the migration file and the two handler changes.