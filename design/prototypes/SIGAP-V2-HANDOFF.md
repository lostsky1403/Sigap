# SIGAP V2 — Pen.dev Prototype Handoff

**Artifact:** `design/prototypes/sigap-civic-v2.pen` — Pen.dev schema v2.21, 27 top-level frames, 36 variables, 20 reusable components, 21 component instances, 0 broken references.

**Source of truth read from:** `F:\sigap-design-v2`, branch `design/sigap-v2-prototype` (clean worktree created off `local/r2-drill` @ `704633c`). The main checkout `F:\Sehat-Hub` was left untouched — no Svelte component, API handler, migration, schema, auth code, or operational script was modified, and nothing was deployed.

---

## 1. Art direction — "Lembar Dinas" (Service Sheet)

Three directions were developed and compared:

- **A. Lembar Dinas** — the calm of a well-set printed public-service form: rules instead of boxes, one teal rule per section, numbers that line up.
- **B. Papan Informasi** — an all-caps noticeboard: heavy uppercase labels, the number as the largest element on its row, near-zero teal.
- **C. Loket** — a counter window: full-bleed teal band carrying one 48px monospace identifier, abutting zone grounds.

**A was selected** because it needs no new visual event (it re-specifies type and rhythm on primitives that already exist), it closes a real type-scale gap, and its failure mode is the cheapest to detect and reverse. C was rejected because a full-bleed saturated band is one scroll away from the hero the brief forbids — in the brand colour.

**Signature devices:** a 2px teal rule opening each section (content measure, never full-bleed); physically aligned tabular numeral columns; rule-separated lists rather than card-per-item.

**Teal budget:** ~2% of painted pixels — the primary action, inline links, the active nav item, section-opening rules, and the queue-number ground.

---

## 2. Foundations

### Colour

| Token | Value | Usage | Contrast on surface |
|---|---|---|---|
| `canvas` | `#F7F9F8` | page background | — |
| `surface` | `#FFFFFF` | interactive surfaces | — |
| `surface-sunken` | `#F1F4F3` | table headers, disabled | — |
| `fg` | `#17211F` | primary content | 16.49 |
| `muted` | `#5F6F6B` | supporting content | 5.29 |
| `border` | `#767E7C` | **control borders** | 4.16 |
| `border-subtle` | `#E0E6E3` | decorative separators only | 1.28 — never a control |
| `primary` | `#0F766E` | primary action, links, focus | 5.47 |
| `primary-strong` | `#115E59` | teal text, queue number | 7.58 |
| `primary-tint` | `#E6F1EF` | queue-number ground | — |
| `success` | `#15803D` | successful operational status | 5.02 |
| `warning` | `#B45309` | needs attention | 5.02 |
| `danger` | `#B42318` | failure, destructive actions | 6.57 |
| `info` | `#2563A6` | neutral information | 6.15 |

**Accessibility correction applied.** The candidate V2 `border: #DDE5E2` measures **1.21:1** on canvas — it fails the WCAG 2.2 AA 3:1 non-text requirement for the input, button, chip and table-cell borders it was meant to draw. Corrected to `#767E7C`. `#E0E6E3` is retained for decorative separators only and is never wired to a control. **The same defect exists in the shipped `#E0DDD8` (1.24:1)** and must be fixed in `apps/web/src/lib/design/tokens.ts` at implementation time.

### Typography — Inter, two registers

| Role | Size | Weight | Line-height | Tracking |
|---|---|---|---|---|
| H1 page | 32 | 600 | 1.25 | −0.02em |
| H2 section | 20 | 600 | 1.4 | −0.01em |
| H3 subsection | 15 | 600 | 1.47 | 0 |
| Body | 15 | 400 | 1.6 | 0 |
| Label (field) | 14 | 500 | 1.43 | 0 |
| Caption / meta | 12 | 400 | 1.5 | +0.01em |
| Table micro | 11 | 600 | 1.27 | +0.06em, uppercase |

**Numerals.** `tabular-nums` for any digit that can stack. Identifiers additionally use a monospace stack at display role — 48px/700/+0.12em for check-in codes, 32px/700/+0.10em for queue numbers. Prose numerals stay proportional.

### Layout

8px base; ladder 4/8/12/16/24/32/48/64. Radius 6 (control) / 8 (panel) / 12 (dialog). Section rhythm 64px citizen, 32px admin. Measure 62ch body, 58ch lede. Control heights: citizen 44, admin 36 compact / 40 comfortable. Admin table rows 40px, 11px uppercase sticky headers, 12px cell padding.

---

## 3. Screen inventory — 27 frames

### Citizen — desktop 1440
| Frame | Route | Endpoint(s) |
|---|---|---|
| 10 Beranda | `/` | `GET /api/v1/public/facilities` |
| 11 Faskes | `/faskes` | `GET /api/v1/public/facilities` |
| 12 Buat Janji | `/appointments/new` | `GET /public/service-units` · `POST /appointments` |
| 13 Ambil Antrean | `/queues/new` | `POST /queues/generate` |
| 14 Status — visit pass | `/patient/status` | `GET /patient/status?code=` |
| 15 Check-In | `/appointments/check-in` | `POST /appointments/{id}/check-in` |
| 16 Jejak Kunjungan | `/wallet` | **endpoint absent** |
| 17 Masuk | `/auth/login`, `/auth/register` | Supabase `signInWithPassword` / `signUp` |

### Citizen — mobile 390
30 Beranda · 31 Status (visit pass) · 32 Faskes · 33 Buat Janji

### Admin — desktop 1440
| Frame | Route | Notes |
|---|---|---|
| 20 Antrean | `/admin/queues` | Best-designed operational screen: 3 grouped bands, per-ticket transitions, stale banner, 30s visibility-aware polling |
| 21 Ringkasan | `/admin` | Count strip + attention list + next schedule. No fabricated KPIs, trends or revenue |
| 22 Janji Temu | `/admin/appointments` | 6-column table; transitions derived from the server state map |
| 23 Jadwal | `/admin/schedules` | 8-column table; editor gated by the server-supplied affordance list |
| 24 Fasilitas | `/admin/facilities` | 8-column table; one-way deactivate with confirmation |
| 25 Notifikasi | `/admin/notifications` | 5 summary cards + 7-column outbox; masked recipients; per-row retry/cancel |

### Responsive & system
| Frame | Purpose |
|---|---|
| 26 Admin Responsive / Antrean / 768 | Icon rail + 4-column table (secondary columns dropped, status and action never hidden) |
| 27 Admin Mobile / Antrean / 390 | Table → stacked-card adaptation |
| 40 States / Interaction & Read States | Button + field states, 6-state admin read matrix, stale banner, row-level error |
| 45 States / Booking & Check-In | Validation failure, success, 401/409/network, walk-in submit guard |
| 60 Handoff / Aksesibilitas & Pemetaan Rute | Contrast table, status encoding, route map, unsupported list |
| 70 Flow / Peta Alur & Gerak | Citizen + staff flows, motion spec table |

---

## 4. Status encoding — never colour alone

Every status renders **label text + semantic icon + tone**. Badges are never filled: 1px tone border, tone text, tinted ground. Shape and text carry the meaning; colour is support.

| Key | Label | Icon | Tone |
|---|---|---|---|
| `waiting` | Menunggu | `clock` | warning |
| `called` | Dipanggil | `megaphone` | info |
| `in_service` | Dalam Pelayanan | `stethoscope` | info |
| `completed` | Selesai | `circle-check` | success |
| `cancelled` | Dibatalkan | `circle-x` | danger |
| `skipped` | Dilewati | `skip-forward` | danger |
| `scheduled` | Terjadwal | `calendar-clock` | neutral |
| `checked_in` | Sudah Check-in | `clipboard-check` | info |
| `queued` | Dalam Antrean | `list-ordered` | warning |
| `no_show` | Tidak Datang | `user-x` | danger |

Legal transitions come from the server (`admin.go:1057` queue, `admin.go:2334` appointment). The UI shows only transitions the server accepts; an illegal transition returns **400**, not 409.

---

## 5. Interaction — honest capability statement

**Pen.dev cannot express prototype interactions.** The installed build's schema (v2.21) has no `startingPoint` / `destination` / `action` / `onClick` / `transition` property, and no interaction tool is mounted. Verified against `pen-schema.md`, `execute.md`, and the full `skills/pen-dev` tree.

Therefore:
- **No button in this document is clickable.** Clicking will not navigate.
- Every interaction is communicated as a **labelled state artboard with explicit values** (frames 40, 45) or as a **flow sequence of chips with arrows** (frame 70).
- The only place real hover/focus/transition behaviour exists is a Pen `Browser` node pointed at the live app.

Motion values — all ≤220ms, focus never animated:

| State | Duration | Easing | Change |
|---|---|---|---|
| Hover button | 120ms | ease-out | background → `#0B6B63` |
| Pressed button | 90–120ms | ease-out | background → `#084F49` |
| Focus-visible | 0ms | none | `outline: 2px solid primary; outline-offset: 2px` |
| Row / tab selected | 150ms | ease-in-out | background + border + text weight |
| Validation error | 120–160ms | ease-out | border → danger, helper text appears |
| Success confirmation | 160ms | ease-out | icon + colour → success |
| Spinner | 700ms loop | linear | 2400ms under reduced-motion |
| Skeleton | pulse | ease-in-out | opacity; disabled under reduced-motion |
| Dialog enter / exit | 180 / 140ms | ease-out / ease-in | opacity + 4px translate |
| Drawer enter (mobile) | 200ms | ease-out | translate from edge |
| Queue status change | 200ms | ease-in-out | ground + border + label |

**Rejected for civic healthcare:** confetti, spring/bounce easing, shake on error, parallax, scroll-jacking, marquee, ambient motion, magnetic hover, count-up on fabricated metrics, animated gradients, transitions >220ms, and any motion that gates information.

---

## 6. Verification performed

Visual QA ran through a vision model on exported PNGs at each stage. Defects found and fixed:

1. **Collapsed primary button** — the CTA ref resolved to `width: fill_container(0)` inside a zero-height wrapper and never rendered. Fixed with an explicit wrapper height; re-verified.
2. **Masthead floating over content** — the reusable masthead carried `layoutPosition: absolute`, inherited by every instance, so headers overlapped body content on every citizen screen. Removed from the component; re-verified.
3. **Clipped tablet sidebar** — at 768px the 240px sidebar was squeezed to 56px, clipping every nav label (`OPERASIONAL` rendered as `OPERA SIONA L`). Replaced with a dedicated icon-rail component; re-verified clean.
4. **Indistinguishable button states** — hover/pressed/focus teal variants were visually identical on a static canvas. Added explicit value captions under each state.
5. **Duplicate helper text** on the check-in code field.
6. **Numeral specimens** all rendered the same value; corrected per label.
7. **Raw route paths** (`/admin/queues`) leaked as user-facing card subtitles on Ringkasan; replaced with plain Indonesian.
8. **Sample-data provenance** — added an explicit note that figures and facility names are design samples, not production data.
9. Invalid `alignItems: "baseline"` and missing lucide icon names (`clock`, `alert-triangle`) corrected.
10. **Overlapping frames** and **duplicate frames** from rebuild passes removed.

Final integrity check: 0 placeholders remaining, 21 ref instances resolving to 9 component targets, 0 broken references.

---

## 7. Unsupported by the backend — do NOT implement as if it exists

Documented in frame 60; must not appear in a production build.

1. **Queue position** ("you are 3rd") — never returned by any endpoint.
2. **Live wait estimate** — a hardcoded `25` returned once at generation (`queue.rs:181`), never recomputed. Only truthful as a caveated static figure at registration.
3. **Visit history / timeline** — `/patient/status` returns current state only.
4. **Medical records** (`/wallet`) — `GET /api/v1/medical-records` does not exist in Go or in the proxies. The route is legacy and must be resolved before public use.
5. **Practitioner catalog** — schedules show no practitioner name; `practitioner_id` is deliberately omitted from the client contract.
6. **Queue reorder / prioritise** — no operation anywhere in the stack; `queue_tickets` has no ordering column.
7. **Facility reactivation** — deactivation is one-way.
8. **Server-side facility search** — current filtering is client-side only.
9. **Week-grid calendar** in admin — schedules are a table.
10. **Dark theme** — no dark token set is wired into the app.

Additionally: queue tickets carry **no service/poli** field and **no patient name**; notifications have **no pagination or total** and the facility column is unresolvable; walk-in submission has **no idempotency key**, so the submit button must be locked while in flight (the only server backstop inside the daily cap is a 2-per-day rate limiter); `/wallet` has an observed privacy concern (auto-loads a demo phone number, exposes signature data) that must be fixed rather than normalised.

---

## 8. Implementation recommendation

Four passes, each independently reviewable.

**Pass 1 — tokens.** Update `design/DESIGN.md` (canonical), `apps/web/src/lib/design/tokens.ts` and `tokens.css` to the V2 values, including the `#767E7C` border correction and a separate `--sigap-border-subtle`. `tokens.test.ts` pins every declaration and must be updated in the same commit.

**Pass 2 — type and rhythm.** Replace the `app.css` scale (20/14/13) with 32/20/15/13, add the 2px section-opening rule as a shared primitive, and adopt `tabular-nums` as a rule for stackable digits. This alone removes most of the current reliance on cards for separation.

**Pass 3 — citizen.** Rework `/patient/status` into the visit-pass composition first (signature screen, simplest win), then `/faskes` row anatomy, then the booking funnel. Keep `VisitProgress` — it already implements the correct current-state-only semantics.

**Pass 4 — admin.** Apply the toolbar/header split (identity + freshness in the header; filters and actions in the toolbar), add sort affordance to `DataTable`, and give `/admin/appointments` a 400px detail panel. Do not introduce a Kanban board for queues.

**Out of scope until the data exists:** live ETA, queue position, appointment detail routes, and any wallet feature.

---

## 9. Open questions for the owner

1. **`/wallet`** — remove the route, or build the missing endpoint first? It currently ships a page that calls a non-existent proxy.
2. **Border correction** — accept `#767E7C` as the control border, or keep `#E0DDD8` and accept a known AA failure?
3. **Sample data** — frames use `RSUD Kota Sehat` / `Puskesmas Sukajaya` and dates around 20 Oct 2026. Replace with real (anonymised) facility names before any stakeholder demo?
4. **Pen interactivity** — if clickable review is required, the only honest path is a `Browser` node against the live app; Pen cannot wire it.
5. **Document file** — Pen currently has the document open as `sigap.pen`. Save As `sigap-civic-v2.pen` to match the deliverable path.
