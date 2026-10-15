# SIGAP V2.1 — Pen.dev Prototype Handoff

**Artifact (canonical):** `F:\Sehat-Hub\design\prototypes\sigap.pen`
**SHA-256:** `d05d6f3f96ec426fa11925c5f17ecc5dfce993f8aa320d9abb8d5c80b1163fa5` — 1,333,140 bytes
**Byte-identical mirror:** `F:\sigap-design-v2\design\prototypes\sigap-civic-v2.pen` (branch `design/sigap-v2-prototype`)
**Document:** Pen.dev schema **v2.21**, **37 top-level frames**, 36 variables, **18 reusable components**, 28 ref instances, 2,993 nodes, **0 broken references**, **0 top-level frame overlaps**.

**Source of truth read from:** `F:\sigap-design-v2`, branch `design/sigap-v2-prototype`. No Svelte component, API handler, migration, schema, auth code, or operational script was modified, and nothing was deployed.

**V2 → V2.1 delta:** 28 → 37 frames. All four P0 defects from the V2 review are closed, 9 sub-11px text nodes corrected, two semantic colours darkened to clear 4.5:1 with margin, one full screen (`16 Jejak Kunjungan`) rebuilt because it presented fabricated records, and 5 missing state/route artboards added.

---

## 1. Art direction — "Lembar Dinas" (Service Sheet)

Three directions were developed and compared:

- **A. Lembar Dinas** — the calm of a well-set printed public-service form: rules instead of boxes, one teal rule per section, numbers that line up.
- **B. Papan Informasi** — an all-caps noticeboard: heavy uppercase labels, the number as the largest element on its row, near-zero teal.
- **C. Loket** — a counter window: full-bleed teal band carrying one 48px monospace identifier, abutting zone grounds.

**A was selected** because it needs no new visual event (it re-specifies type and rhythm on primitives that already exist), it closes a real type-scale gap, and its failure mode is the cheapest to detect and reverse. C was rejected because a full-bleed saturated band is one scroll away from the hero the brief forbids — in the brand colour.

**Signature devices:** a 2px teal rule opening each section (content measure, never full-bleed); physically aligned tabular numeral columns; rule-separated lists rather than card-per-item.

**Teal budget:** ~2% of painted pixels — the primary action, inline links, the active nav item, section-opening rules, and the queue-number ground. **V2.1 correction:** the masthead CTA was demoted from filled to outlined so that **exactly one filled primary action exists per screen** (vision-verified on the homepage).

**Primary-action rule, stated precisely.** One filled primary per **action context**, not one per canvas:

- A screen with a page-level task has **exactly one** filled primary (submit, confirm, book). Its header CTA is outlined so the two never compete.
- An **admin data table** has no page-level primary. Each row carries its own filled action for the transition that row's state permits (`Panggil` for a waiting ticket, `Layani` for a called one). Four rows → four filled buttons, each scoped to one row, none competing for the same click. Demoting them to outlined would hide the operator's primary work behind a secondary affordance.

Both forms are applied consistently: every admin table (20, 22, 26, 27) uses row-level primaries; every task screen (12a–12f, 13, 15, 16, 17, 18) uses one page-level primary with an outlined header CTA.

---

## 2. Foundations

### Colour

| Token | Value | Usage | Contrast |
|---|---|---|---|
| `canvas` | `#F7F9F8` | page background | — |
| `surface` | `#FFFFFF` | interactive surfaces | — |
| `surface-sunken` | `#F1F4F3` | table headers, disabled | — |
| `fg` | `#17211F` | primary content | 16.49 on surface · 15.59 on canvas |
| `muted` | `#5F6F6B` | supporting content | 5.29 on surface · 5.00 on canvas |
| `border` | `#767E7C` | **control borders only** | 4.16 — passes 3:1 non-text; **fails 4.5:1 as text** |
| `border-subtle` | `#E0E6E3` | decorative separators only | 1.28 — never a control |
| `primary` | `#0F766E` | primary action, links, focus | 5.47 |
| `primary-strong` | `#115E59` | teal text, queue number | 7.58 |
| `primary-tint` | `#E6F1EF` | queue-number ground | — |
| `success` | `#147A3A` | successful operational status | 5.42 on surface · **4.805 on `success-tint`** |
| `warning` | `#AC4F08` | needs attention | 5.42 on surface · **4.851 on `warning-tint`** |
| `danger` | `#B42318` | failure, destructive actions | 6.57 · 5.761 on `danger-tint` |
| `info` | `#2563A6` | neutral information | 6.15 · 5.400 on `info-tint` |

**V2.1 accessibility corrections applied to the document:**

1. **`success` `#15803D` → `#147A3A`** — the V2 value measured **4.446:1** on its own `success-tint` ground, below the 4.5:1 AA threshold, across 18 badge uses. The V2 hand-off had only checked it against white.
2. **`warning` `#B45309` → `#AC4F08`** — measured **4.497:1** on `warning-tint`, across 17 uses. Corrected.
3. **`fWgPH` helper text** was drawing its text from the `$border` token (`#767E7C`, 4.162 on white — a border token used as text). Repointed to `$muted`.
4. **`qzfCB` Jadwal checkbox** was 16×16, below the WCAG 2.2 SC 2.5.8 24×24 target (or 20×20 with sufficient spacing). Raised to 20×20.
5. **Nine 10px text nodes** raised to 11px (see §3, typography).

**Still outstanding at implementation time — the same defect exists in the shipped app.** `apps/web/src/lib/design/tokens.ts` carries `#DDE5E2` for control borders, measuring **1.21:1** on canvas; it fails the WCAG 2.2 AA 3:1 non-text requirement for the input, button, chip and table-cell borders it draws. Correct to `#767E7C` and keep `#E0E6E3` for decorative separators only. The shipped `#E0DDD8` measures 1.24:1 and has the same defect.

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

**Minimum rendered size is 11px.** V2.1 raised 9 nodes that sat at 10px: `yU718`, `S5kOS`, `X2mB1` (30 Citizen Mobile / Beranda — facility short codes) and `Lt07i`, `fwSNi`, `F61Ks`, `SD9M4`, `fIFJU`, `qMtsK` (34 Citizen Narrow / Status — queue-number eyebrow, phase eyebrow, four step labels). Programmatic re-scan: **0 text nodes below 11px**.

**Numerals.** `tabular-nums` for any digit that can stack. Identifiers additionally use a monospace stack at display role — 48px/700/+0.12em for check-in codes, 44px/700 for the mobile queue number, 32px/700/+0.10em for desktop. Prose numerals stay proportional.

### Layout

8px base; ladder 4/8/12/16/24/32/48/64. Radius 6 (control) / 8 (panel) / 12 (dialog). Section rhythm 64px citizen, 32px admin. Measure 62ch body, 58ch lede. Control heights: citizen 44, admin 36 compact / 40 comfortable. Admin table rows 40px, 11px uppercase sticky headers, 12px cell padding.

---

## 3. Screen inventory — 37 frames

### Meta
| Frame | Purpose |
|---|---|
| 00 Cover / SIGAP V2 | Direction, source of truth, **16 route files** |
| 01 Components | 18 reusable components |
| 02 Foundations | Type scale, numerals, colour grid |

### Citizen — desktop 1440
| Frame | Route | Endpoint(s) |
|---|---|---|
| 10 Beranda | `/` | `GET /api/v1/public/facilities` |
| 11 Faskes | `/faskes` | `GET /api/v1/public/facilities` |
| 12a–12f Booking (6 steps) | `/appointments/new` | `GET /public/service-units` · `POST /appointments` |
| 13 Ambil Antrean | `/queues/new` | `POST /queues/generate` |
| 14 Status — visit pass | `/patient/status` | `GET /patient/status?code=` |
| 15 Check-In | `/appointments/check-in` | `POST /appointments/{id}/check-in` |
| 16 Jejak Kunjungan | `/wallet` | **endpoint absent — honest empty state** |
| 17 Masuk | `/auth/login` | Supabase `signInWithPassword` |
| 18 Daftar | `/auth/register` | Supabase `signUp` |

**Booking step series (new in V2.1, replaces the single V2 `12 Buat Janji`):** 12a Fasilitas → 12b Layanan → 12c Pasien → 12d Tinjau → 12e Sukses → 12f Galat. Each carries a 4-step stepper (done = green check, current = teal, todo = sunken). The step state is the only thing distinguishing them, which is exactly the prototype limitation described in §5.

**16 Jejak Kunjungan was rebuilt.** The V2 frame rendered two fabricated visit records (`RSUD Kota Sehat · 12 Okt 2026 · 09.30 · RSK-0042 · signature a3f9c1d4`). A caveat does not make a fabricated record acceptable, so the records were deleted. The frame is now an honest empty state: warning banner stating `/api/v1/medical-records` does not exist, a centred "Riwayat kunjungan belum tersedia" panel, two factual notes, and two escape actions. Vision-verified: **no fabricated records**.

### Citizen — mobile 390
| Frame | Notes |
|---|---|
| 30 Beranda | Masthead + hero + primary CTA + 3 facility rows + bottom nav |
| 31 Status | Visit pass at 390 |
| 32 Faskes | Facility list at 390 |
| 33 Buat Janji | Booking entry at 390 |
| 35 Walk-In **(new)** | Queue ticket + facility select + two actions; the no-account path on `/queues/new` |
| 36 Check-In **(new)** | Focused code field + success panel + 3-step progress + refresh |

### Citizen — narrow phone 360
34 Status (visit pass) — verifies the 44px monospace queue number and the 4-step progress row survive the narrowest supported width without overflow or label collision.

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
| 40 States / Interaction & Read States | 7 button states with value captions, 4 field states, 6-state admin read matrix, stale banner, row-level error |
| 41 States / Dialog, Toast & Sesi **(new)** | 3 confirmation dialogs (destructive / neutral / typed-confirm), scrim + focus rules, 4 toasts, logout dialog, session route note |
| 45 States / Booking & Check-In | Validation failure, success, 401/409/network, walk-in submit guard |
| 60 Handoff / Aksesibilitas & Pemetaan Rute | Contrast table, status encoding, **route map (16 routes)**, unsupported list |
| 70 Flow / Peta Alur & Gerak | Citizen + staff flows, motion spec table |

---

## 4. Status encoding — never colour alone

Every status renders **label text + semantic icon + tone**. Badges are never filled: 1px tone border, tone text, tinted ground. Shape and text carry the meaning; colour is support.

| Key | Label | Icon | Tone |
|---|---|---|---|
| `waiting` | Menunggu | `clock-3` | warning |
| `called` | Dipanggil | `megaphone` | info |
| `in_service` | Dalam Pelayanan | `stethoscope` | info |
| `completed` | Selesai | `circle-check` | success |
| `cancelled` | Dibatalkan | `circle-x` | danger |
| `skipped` | Dilewati | `skip-forward` | danger |
| `scheduled` | Terjadwal | `calendar-clock` | neutral |
| `checked_in` | Sudah Check-in | `clipboard-check` | info |
| `queued` | Dalam Antrean | `list-ordered` | warning |
| `no_show` | Tidak Datang | `user-x` | danger |

Legal transitions come from the server (`admin.go` queue + appointment maps). The UI shows only transitions the server accepts; an illegal transition returns **400**, not 409.

**Known weak spots, accepted for now:** the active nav item is distinguished by hue + font weight only, and the admin icon rail is icon-only with no visible labels. Both are documented in frame 40 rather than papered over.

---

## 5. Interaction — honest capability statement

**Pen.dev cannot express prototype interactions.** The installed build's schema (v2.21) has no `startingPoint` / `destination` / `action` / `onClick` / `transition` property, and no interaction tool is mounted. Verified against `pen-schema.md`, `execute.md`, and the full `skills/pen-dev` tree.

Therefore:
- **No button in this document is clickable.** Clicking will not navigate.
- Every interaction is communicated as a **labelled state artboard with explicit values** (frames 40, 41, 45) or as a **flow sequence of chips with arrows** (frame 70).
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

**Dialog rules (frame 41):** primary action right, cancel left; destructive actions use `$danger` and are never the default button; never more than one primary action; scrim click = cancel, same as the Batal button.

**Rejected for civic healthcare:** confetti, spring/bounce easing, shake on error, parallax, scroll-jacking, marquee, ambient motion, magnetic hover, count-up on fabricated metrics, animated gradients, transitions >220ms, and any motion that gates information.

---

## 6. Verification performed

Visual QA ran through a vision model on exported PNGs at every stage. **V2 defects found and fixed:**

1. **Collapsed primary button** — the CTA ref resolved to `width: fill_container(0)` inside a zero-height wrapper and never rendered. Fixed with an explicit wrapper height.
2. **Masthead floating over content** — the reusable masthead carried `layoutPosition: absolute`, inherited by every instance. Removed from the component.
3. **Clipped tablet sidebar** — at 768px the 240px sidebar was squeezed to 56px, clipping every nav label. Replaced with a dedicated icon-rail component.
4. **Indistinguishable button states** — hover/pressed/focus teal variants were visually identical. Added explicit value captions under each state.
5. **Duplicate helper text** on the check-in code field.
6. **Numeral specimens** all rendered the same value; corrected per label.
7. **Raw route paths** leaked as user-facing card subtitles on Ringkasan; replaced with plain Indonesian.
8. **Sample-data provenance** — added an explicit note that figures and facility names are design samples.
9. Invalid `alignItems: "baseline"` and missing lucide icon names (`clock`, `alert-triangle`) corrected.
10. **Overlapping frames** and **duplicate frames** from rebuild passes removed.
11. **Narrow-phone coverage gap** — added the 360px visit-pass artboard.
12. **Duplicate masthead components** — all instances repointed to one canonical component (20 → 18 components).

**V2.1 defects found and fixed:**

13. **Home masthead buttons missing (P0-A).** The `Account` frame was `fit_content(0)` and its two refs collapsed to zero width. `Masuk` and `Buat Janji Temu` did not render at all. Replaced both refs with sized plain frames (100×44, 170×44). Vision-verified present.
14. **Nav active state wrong on every screen (P0-B).** Root cause: the nav lives *inside* the shared components, so all instances shared one active item. Fixed with per-instance `descendants` overrides rather than duplicating mastheads — Masthead/Citizen (`Tg6xv`), Admin/Sidebar (`sExAw`), BottomNav/Citizen (`BAZFl`). Vision-verified: Faskes screen shows Faskes active, Ringkasan screen shows Ringkasan active.
15. **Overlapping artboards (P0-C).** Mobile frames were stacked at (0,8100)/(450,8100); admin mobile sat inside frame 40; the 12e/12f booking frames landed on top of `20 Antrean` and `21 Ringkasan`. All repositioned. **Programmatic AABB test over all 37 top-level frames: 0 overlaps.**
16. **Unsupported data presented as working (P0-D).** The visit pass showed `Estimasi sekitar 25 menit` in three places, and three screens promised a queue position. The ETA row was **removed, not caveated** — a caveated fabricated number is still a fabricated number. `RQTCJ/K7OCg` reworded to `Nomor antrean Anda`; "posisi antrean" reworded to "status kunjungan" in `TP9NP`, `pnhNV`, `c9wiV`, `L313Aj`.
17. **Fabricated visit records** in frame 16 — deleted; frame rebuilt as an honest empty state (see §3).
18. **Two semantic colours below AA on their own tints** — `success` and `warning` darkened (see §2).
19. **Nine 10px text nodes** raised to 11px (see §2).
20. **Collapsed and empty nodes** — `hWEr8` StatusCell collapsed to 140×40; empty spacers `xIuGV`, `exREi` and empty `CcXCF` deleted; 11 generic nodes renamed to `Hdr/*`, `Cell/*`, `Icon/*`.
21. **Decorative rules mistaken for structure** — 4 decorative teal `Rule` rectangles removed from the homepage; all 4 section heads rebuilt as uppercase eyebrow → title → description.
22. **Two filled primary actions on the homepage** — masthead CTA demoted to outlined. Vision-verified: exactly one filled teal CTA remains.
23. **Invented route** — frame 41's session panel listed `/patient/walk-in`, which is not a SvelteKit route. Corrected to `/queues/new`. The route map in frame 60 now matches the **16 real route files** exactly (was claimed as 21).
24. **Toast row overflow** — four 420px toasts in one 1360px row clipped the fourth. Restructured to two rows of two; vision-verified no clipping.
25. **Two filled primary actions on 11 screens.** The reusable masthead (`Tg6xv`) carries a filled `Buat Janji Temu` button, so every screen that also had its own filled submit rendered two filled teal primaries — contradicting the "exactly one" rule, which had only been enforced on the homepage. Fixed with per-instance `descendants` overrides demoting the masthead CTA to outlined on the 11 screens that own a primary: 12a–12f, 13 Ambil Antrean, 15 Check-In, 16 Jejak Kunjungan, 17 Masuk, 18 Daftar. Faskes and Status keep it filled — there it *is* the screen's primary. Vision-verified on 15, 17, 18, 13, 11: **exactly one filled teal button each.**
26. **Redundant auth labels.** On `/auth/login` the masthead offered "Masuk" beside a submit also labelled "Masuk". The masthead now offers the *alternate* action: "Daftar" on the login screen, "Masuk" on the register screen.
27. **Three empty input boxes** — the desktop Check-In code field was a genuinely empty frame (0 children), and the optional-note and error-state time fields held blank text nodes. Each given a muted placeholder. Two remaining empty frames are deliberate flex spacers inside `C/Admin/SidebarRail`.
28. **Collapsed badge inside the table-row component.** The DataRow component (`a913rh`) contains a badge ref whose target (`a7fsY`) carries `layoutPosition: absolute` — the same inheritance trap that broke the V2 masthead. The instance rendered at the *component's* gallery coordinates (40,288) instead of inside its StatusCell, so the gallery's STATUS column looked empty. Fixed by giving the ref explicit `width`/`height` and `layoutPosition: auto`; it now resolves to (0,7) inside the cell and is vertically centred. Vision-verified. Audit of all 28 refs confirms this was the only ref nested inside another component — the other 27 are direct children of top-level frames.
29. **Missing bottom nav on 33 Citizen Mobile / Buat Janji / 390.** Every other citizen mobile frame (30, 31, 32, 33's siblings, 34, 35, 36) carries the bottom nav; this one did not, leaving the screen as a dead end. Added with **Faskes** active — the booking route is entered from the Faskes list, and no tab maps to `/appointments/new` directly. Vision-verified: 4 labelled tabs, Faskes teal, no overlap with the form.
30. **1px column overflow in 24 Admin / Fasilitas.** The NAMA cell measured 128px while its longest value (`Puskesmas Sukajaya`) rendered at 129px, so the text ran to the cell edge with no gutter. TIPE reduced 140 → 130 to give NAMA 138px. A zoomed crop confirms a visible gap between NAMA and TIPE; the full-frame render at 1x was too small to judge and produced a false positive on first read.

**Final integrity check:** 37 top-level frames, 0 overlaps, 0 broken refs, 0 text nodes below 11px, 0 fabricated records, route map matches the app 1:1.

---

## 7. Unsupported by the backend — do NOT implement as if it exists

Documented in frame 60; must not appear in a production build.

1. **Queue position** ("you are 3rd") — never returned by any endpoint. **No longer shown anywhere in the design.**
2. **Live wait estimate** — a hardcoded `25` returned once at generation (`queue.rs`), never recomputed. **Removed entirely from the design**, including the caveated static form used in V2.
3. **Visit history / timeline** — `/patient/status` returns current state only. **Removed; frame 16 is now an empty state.**
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

**Pass 1 — tokens.** Update `design/DESIGN.md` (canonical), `apps/web/src/lib/design/tokens.ts` and `tokens.css` to the V2.1 values: the `#767E7C` border correction, a separate `--sigap-border-subtle`, and `success #147A3A` / `warning #AC4F08`. `tokens.test.ts` pins every declaration and must be updated in the same commit.

**Pass 2 — type and rhythm.** Replace the `app.css` scale (20/14/13) with 32/20/15/13, enforce an 11px floor, add the 2px section-opening rule as a shared primitive, and adopt `tabular-nums` as a rule for stackable digits.

**Pass 3 — citizen.** Rework `/patient/status` into the visit-pass composition first (signature screen, simplest win), then `/faskes` row anatomy, then the booking funnel as the 6-step series in frames 12a–12f. Keep `VisitProgress` — it already implements the correct current-state-only semantics. Replace `/wallet` with the honest empty state from frame 16 until the endpoint exists.

**Pass 4 — admin.** Apply the toolbar/header split (identity + freshness in the header; filters and actions in the toolbar), add sort affordance to `DataTable`, and give `/admin/appointments` a 400px detail panel. Do not introduce a Kanban board for queues.

**Out of scope until the data exists:** live ETA, queue position, appointment detail routes, and any wallet feature.

---

## 9. Open questions for the owner

1. **`/wallet`** — remove the route, or build the missing endpoint first? The design now shows an honest empty state, so the route can ship without lying, but it delivers nothing.
2. **Border correction** — accept `#767E7C` as the control border, or keep `#E0DDD8` and accept a known AA failure?
3. **Sample data** — frames use `RSUD Kota Sehat` / `Puskesmas Sukajaya` and dates around 12–20 Oct 2026. Replace with real (anonymised) facility names before any stakeholder demo?
4. **Pen interactivity** — if clickable review is required, the only honest path is a `Browser` node against the live app; Pen cannot wire it.
5. **Booking step series** — 6 artboards is a lot of canvas for one funnel. Keep all six, or collapse to 12a/12d/12e and describe 12b/12c in frame 70?
