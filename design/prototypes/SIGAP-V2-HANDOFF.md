# SIGAP V2.2 — Pen.dev Prototype Handoff

**Artifact (canonical):** `F:\Sehat-Hub\design\prototypes\sigap.pen`
**SHA-256:** `9b06902f4aa067cfa45612d0d9f92a0dc3a8879e0a169b672ce01b67bafb0bb7` — 1,457,099 bytes
**Byte-identical mirror:** `F:\sigap-design-v2\design\prototypes\sigap-civic-v2.pen` (branch `design/sigap-v2-prototype`)
**Superseded V2.1:** `d05d6f3f96ec426fa11925c5f17ecc5dfce993f8aa320d9abb8d5c80b1163fa5`, preserved at `design/prototypes/backup-v22/sigap-v2.1-20261016-1827.pen`

**Document:** Pen.dev schema **v2.21**, **45 top-level frames** (V2.1 had 37), 36 variables, 18 reusable components, 36 ref instances, 3,282 nodes, **0 broken references**, **0 top-level frame overlaps**, **0 internal child overflows** outside the `01 Components` gallery.

**Evidence:** `F:\Sehat-Hub\design\prototypes\v22-shots\` — 45 frame PNGs + `00-OVERVIEW.png` + `MANIFEST.md` + 6 `EVIDENCE-P0-*.png` before/after pairs.

**V2.1 → V2.2 delta:** all five P0 defects closed with structural corrections and re-rendered proof; three invented claims removed; one wrong weekday fixed across 7 nodes; mobile booking and mobile check-in rebuilt to match their desktop flows.

---

## 1. P0 corrections — before / after

### P0-1 — Desktop homepage: the CTA collided with the row beneath it

**Cause (structural, not cosmetic).** The primary action ref `Av6UJ` carried `x:40, y:40` — the *component's own gallery coordinates* (`XxbZA` is a reusable frame that sits at `40,40` inside `01 Components`). The ref inherited them, so inside `PrimaryWrap` (480×52) the 480×52 button was drawn at (40,40) and overflowed the wrapper by 40px on both axes. That 40px of bleed is what landed on top of `R/Check-In Janji Temu`.

**Correction.** `Av6UJ` repositioned to `0,0`; `PrimaryWrap` then measured 480×52 at y=66 inside `Start`, with `List` at y=134 — a 16px gap. No rectangle was layered over the collision to hide it.

**Evidence:** `EVIDENCE-P0-1-homepage-cta.png`. Vision on the after-crop: *"clear empty gap between the teal button and the list row: yes"*, *"overlaps or touches a list row: no"*.

**Also found and fixed by the same audit.** The generic overflow test found 12 more overflowing children across the document, all now at 0:
- `Yjzf3` Row in `40 States` (1360px content in a 1360px row, gap 0 → the 4th card bled 20px right). Gap set to 16.
- Four route-map cells in `60 Handoff` whose 12px text exceeded their columns by 5–15px. Route column 340→300, endpoint column 260→300.
- Three dialog caption frames in `41 States` (437px content in a 437px column, gap 0). Row gaps set to 20.

### P0-2 — Navigation semantics

**Confirmed failure.** `xRtxS` (mobile Check-In) highlighted **Faskes**. The V2.1 pass had set mobile nav states ad hoc and never covered the check-in screen.

**Correction — an explicit route-aware rule.** One table, applied to all 27 nav instances:

| Route | Artboards | Active tab |
|---|---|---|
| `/` | 10, 30 | Beranda |
| `/faskes` | 11, 32 | Faskes |
| `/appointments/new` | 12a–12e, 33a–33d | Faskes — the booking route is entered from the Faskes list and no tab maps to it |
| `/queues/new` | 13, 35–38 | Beranda — the citizen starts at Beranda and has no tab for walk-in |
| `/patient/status` | 14, 31, 34 | Status |
| `/appointments/check-in` | 15, 39, 39a, 39b, 46 | Check-In |
| `/wallet` | 16 | **none** — no tab maps; an intentional no-selection |
| `/auth/login`, `/auth/register` | 17, 18 | **none** — auth is not a tab destination |

**Rule:** a screen highlights a tab only when that tab's route is the screen's route. Where no tab maps, the nav renders fully muted rather than lighting a random destination.

**Verification.** Structural, not visual: every one of the 27 nav refs was read back and its `descendants` fill values asserted. Result: **27/27 have exactly one `$primary` tab, or zero where the table says none.** No instance has two.

**Evidence:** `EVIDENCE-P0-2-nav-active.png` — before/after bottom nav on the check-in screen. Vision: *"BEFORE: Faskes highlighted teal. AFTER: Check-In highlighted teal."*

### P0-3 — Check-In desktop/mobile parity

**Cause.** The mobile frame showed one field holding `RSK-0042-A17` — a string that is neither the appointment ID nor a valid check-in code. It was invented. The real contract (`CheckinForm.svelte`, `booking.go`) requires **two** inputs: `appointmentId` (UUID, sent as a path segment) and `checkinCode` (6 chars `[A-Z0-9]`, compared case-insensitively).

**Correction.** Both breakpoints rebuilt to the same two-field form, with monospace values and the real validation message:

- **Desktop `NFusA`:** `ID janji temu` = `3f2b9c41-7d8e-4a15-9b03-6c1e2f8a4d70`; `Kode check-in` = `4F8K2Q` (focused); helper *"6 karakter huruf dan angka, misalnya 4F8K2Q."*; submit `Check-In`.
- **Mobile `xRtxS`:** the same two fields at 390px, plus an info note linking to the walk-in path.
- **Five states, now separated and labelled** in `46 States / Check-In / 1440` and in the mobile frames `39` / `39a` / `39b`:

| State | Where | What it shows |
|---|---|---|
| Input | `NFusA`, `xRtxS` | both fields, code focused |
| Validation | `46` panel 2 | both fields red-bordered, *"ID janji temu wajib diisi."* / *"Kode check-in wajib diisi."*, request not sent |
| Submitting | `46` panel 3 | locked grey fields, disabled `Memproses…` |
| Success | `39a`, `46` panel 4 | queue number `RSK-0001`, progress list, *"Server tidak mengembalikan nama fasilitas pada respons ini."* |
| Error | `39b`, `46` panel 5 | red panel *"Kode check-in tidak cocok."*, red code field, `Coba lagi`, note that 401 here means wrong code, not a dead session |

**The question-mark icon is fixed.** `circle-check` resolves to a circle containing a **question mark** in this Pen build — proven by rendering an 11-icon probe sheet and reading it back (row 3 = `check-circle` = question mark; row 1 = `circle-check` = correct check). The success panels now use `circle-check`, which renders correctly. The probe frame was deleted after the test.

**Evidence:** `EVIDENCE-P0-3-checkin-desktop.png`, `EVIDENCE-P0-3-checkin-mobile.png`, and the `46 States` render.

### P0-4 — Booking: mobile had no transaction progression

**Cause.** `KXPTn` was a single scrolling form with all fields and no steps, while desktop had a four-step stepper. The four steps were also **wrong**: the real flow is one form with three steps (`BookingStepper.svelte` → `['Fasilitas', 'Layanan', 'Waktu + Data Diri']`), validated and submitted as a unit.

**Correction.**

- **Desktop corrected from four steps to the real three.** `12d Tinjau` was deleted; `12c` became `12c Booking / 3 Waktu + Data Diri` and gained the missing `Waktu` field; its submit label changed from `Lanjut ke tinjauan` to `Buat janji temu`. Steppers rebuilt on all three frames with the real labels, `check` glyphs for completed steps.
- **Mobile given the same journey** as four artboards, not a shrunken desktop form and not a one-page form:
  - `33a` step 1 Fasilitas — stepper current on 1, facility select, three facility options, `Kembali` / `Lanjut`
  - `33b` step 2 Layanan — current on 2, step 1 shows a green check
  - `33c` step 3 Waktu + Data Diri — current on 3, steps 1–2 checked, waktu + name + phone + optional note, single `Buat janji temu`
  - `33d` Sukses — 6-character check-in code in monospace, time, facility

**Evidence:** `EVIDENCE-P0-4-booking-mobile.png`. Vision on `afaSY`: *"Stepper labels: Fasilitas (completed, green check), Layanan (completed, green check), Waktu & data (current). Invented 4th step? No."*

### P0-5 — Walk-In: pre-submit and post-submit states were the same screen

**Cause.** `KNWMV` showed a generated queue number `A17` *above* the facility selector and a `Simpan nomor` button — a number on screen before anything was submitted, and a number in the wrong format (`A17` vs the real `<short_code>-<4 digits>`).

**Correction.** Split into four frames, and the number now appears **only after success**:

| Frame | State | Key content |
|---|---|---|
| `35` Isi data | before submit | facility + full name + phone; **no number**; note *"Nomor terbit setelah data terkirim."* |
| `36` Mengirim | in flight | fields locked grey, disabled `Memproses…`, note that button and fields stay locked so a double tap cannot spend two of the daily quota |
| `37` Terkirim | success | `RSK-0001` (correct format), success panel, `Estimasi sekitar 25 menit` **with** the caveat *"Estimasi ini dihitung saat pendaftaran dan bisa berubah karena kondisi antrean."*, `Lihat status kunjungan` / `Ambil nomor lagi` |
| `38` Gagal | failure | *"Nomor belum terbit"*, `Coba lagi` / `Ganti fasilitas`, and a causes list — the 2-per-facility-per-day limit and a dropped connection |

**Backend limits preserved:** no idempotency key exists, so the design keeps the submit locked while in flight and states that explicitly on both the in-flight and failure frames.

**Evidence:** `EVIDENCE-P0-5-walkin.png`.

---

## 2. P1 corrections

### Product language and data accuracy

1. **"Fasilitas terdekat" removed.** The public catalog returns only `id, name, short_code, type, is_active` ordered by `name` — no distance, no geolocation, no address, no city. The homepage heading is now **"Fasilitas dalam katalog"**, the description reads *"Empat faskes pertama menurut nama, dari katalog publik yang aktif."*, and the count reads *"Menampilkan 4 faskes pertama (diurutkan menurut nama)"*. (A separate `/api/v1/facilities/nearby` endpoint does compute `dist_km`, but it serves a hardcoded six-facility list and has no frontend caller — it is not a supportable basis for a claim.)

2. **Technical API text moved out of the citizen UI.** `/wallet` (`16`) no longer says `/api/v1/medical-records` or calls the route "legacy". It now reads:
   - title: *"Riwayat kunjungan"*
   - lead: *"Riwayat kunjungan Anda akan tampil di sini setelah fitur ini tersedia. Saat ini kami belum bisa menampilkan daftar kunjungan sebelumnya."*
   - panel: *"Belum ada riwayat yang bisa ditampilkan. Kami sedang menyiapkannya."*
   - two facts: status kunjungan still works; save your queue number and check-in code.
   The endpoint facts stay in this handoff and in frame `60`.

3. **Wrong weekday fixed.** 20 October 2026 is a **Tuesday**. Seven nodes said *"Sen, 20 Okt"*: `ybqz5` (Admin Ringkasan), `z8KP9` (mobile booking), `FHai6`, `Xv3mZ` (booking step 2), `r4GYx6`, `s0INd` (booking success), `C1AME` (frame 45). All now read **"Sel, 20 Okt"**. A document-wide regex re-scan for `Sen, 20 Okt` returns zero.

4. **Frame 70 motion table** referenced `#15803D`, the pre-V2.1 success colour. Corrected to `#147A3A`.

5. **Route map** updated for every new artboard; still exactly the 16 real SvelteKit routes.

### Visual craftsmanship

- **Decorative teal rules reduced on mobile.** The 390px Beranda had three 2px teal rules (hero + two section heads). The two section-head rules were replaced with 11px uppercase eyebrows matching the desktop pattern — the mobile screen now carries **one** rule, in the hero. Total across the document: 42 → 40, all of them section openers or the hero.
- **Homepage alignment.** The desktop hero now opens with the same 620px teal rule as every other citizen page; the eyebrow → title → description stack is identical between desktop and mobile.
- **Action hierarchy** unchanged in principle and re-verified after the edits: task screens keep one filled primary with an outlined header CTA; admin tables keep one filled primary per row.
- **Confirmation screens** clarified: booking success shows the 6-character check-in code at 36px monospace; walk-in success shows the queue number at 40px monospace; check-in success shows the queue number at 40px monospace with the three-step progress list.
- **Admin Antrean and the visit pass were not redesigned** — only their nav selection and the `Sen` → `Sel` label changed.

---

## 3. Screen inventory — 45 frames

**Meta:** `00` Cover · `01` Components · `02` Foundations

**Citizen desktop 1440:** `10` Beranda · `11` Faskes · `12a–12c` Booking steps 1–3 · `12d` Sukses · `12e` Galat · `13` Ambil Antrean · `14` Status · `15` Check-In · `16` Jejak Kunjungan · `17` Masuk · `18` Daftar

**Admin desktop 1440:** `20` Antrean · `21` Ringkasan · `22` Janji Temu · `23` Jadwal · `24` Fasilitas · `25` Notifikasi

**Responsive:** `26` Admin 768 · `27` Admin mobile 390

**Citizen mobile 390 / narrow 360:** `30` Beranda · `31` Status · `32` Faskes · `33a–33c` Booking steps 1–3 · `33d` Booking sukses · `34` Narrow status 360 · `35–38` Walk-In (isi / mengirim / terkirim / gagal) · `39` Check-In isi data · `39a` Check-In berhasil · `39b` Check-In kode salah

**States:** `40` Interaction & Read · `41` Dialog, Toast & Sesi · `45` Booking & Check-In · `46` Check-In (5-state matrix)

**Handoff & flow:** `60` Aksesibilitas & Pemetaan Rute · `70` Flow

---

## 4. Verification performed

**Structural (deterministic, on the live document):**

| Check | Result |
|---|---|
| Top-level frame overlap (AABB, all pairs) | **0** |
| Internal child overflow / clipping, every frame except the `01 Components` gallery | **0** |
| Broken component references | **0** |
| Text nodes below the 11px floor | **0** |
| Nav refs with exactly one active tab, or zero where no tab maps | **27 / 27** |
| Nodes named `Sen, 20 Okt` | **0** |
| Text containing `terdekat` | **0** |
| Text containing `#15803D` | **0** |
| Text containing a raw API path in a citizen frame | **0** |

**Visual (vision model on the exported PNGs):**

- `O1PIku` — CTA gap confirmed; exactly one filled teal primary; no clipping.
- `NFusA` — two fields with the right values; one filled primary; no clipping.
- `xRtxS` — two fields; no queue number; **Check-In** tab active; no clipping.
- `KXPTn` / `QTzc1` / `afaSY` — stepper advances 1 → 2 → 3, no fourth step, no clipping.
- `KNWMV` — no queue number before submit; `X6WNmi` — `RSK-0001` with caveated estimate; `lH16N` — disabled button and locked fields; `m3JirM` — no number, causes listed.
- `cJ2OE` — five labelled states; success icon is a check in a green circle, **not** a question mark.
- `M7Sgp` — one teal rule, eyebrow+heading pairs correct, no clipping, bottom nav present.

**Independently reviewed by subagents:** `ContractScout` (source contract extraction from `apps/web` and `apps/api`), `V22DataQA` (data-claim audit over the renders), `V22FlowQA` (flow-state audit over the renders). Their findings are folded into §1 and §2.

---

## 5. Remaining limitations

1. **Pen has no prototype interactions** (schema v2.21: no `onClick` / `transition` / `destination`). Every interaction is a labelled state artboard. Nothing in this document is clickable.
2. **`/wallet` has no data source.** `GET /api/v1/medical-records` does not exist and `/wallet` has no SvelteKit proxy. The frame is an honest empty state; it delivers nothing until the endpoint exists.
3. **No queue position, no live ETA.** The estimate is a registration-time constant; the design shows it only with its caveat.
4. **No idempotency key** on booking, walk-in or check-in. The design compensates with a locked submit, but a duplicate POST remains possible if a request is retried after a network partition.
5. **Rate-limit copy mismatch in the shipped app** — booking says "max 3 per day" while `main.go` passes `NewDailyLimiter(2)`. The prototype shows the 2-per-facility-per-day walk-in limit only; the booking message is not reproduced.
6. **Facility name is not returned by the check-in or walk-in success payloads.** Both confirmation screens say so rather than inventing it.
7. **`#767E7C` is a control-border colour only.** At 4.162:1 on white it passes the 3:1 non-text threshold and fails 4.5:1 as text; it is never used for text.
8. **The active nav item is distinguished by hue and weight only.** Documented in frame `40` rather than papered over.
9. **Sample data is sample data.** Facility names, dates and codes are design samples, stated on the cover.
10. **Admin Antrean, the visit pass and the 768px rail were left as V2.1 shipped them**, per the instruction not to redesign working components.

---

## 6. Verdict

| Gate | Result |
|---|---|
| `P0_DEFECTS_FIXED` | **5 / 5** — each with a structural correction, a re-render and a visual check |
| `DESKTOP_MOBILE_FLOW_PARITY` | **PASS** — check-in is two fields on both breakpoints with five separated states; booking is the same three steps on both; walk-in is four states on mobile and consistent with the desktop `13` |
| `VISUAL_QA` | **PASS** on 45 renders; 0 internal overflows, 0 frame overlaps, 0 sub-11px text |
| `DATA_CONTRACT_REVIEW` | **PASS** — 0 wrong weekdays, 0 proximity claims, 0 fabricated records, 0 invented routes, 0 technical API text in citizen UI |
| `READY_FOR_OWNER_APPROVAL` | **YES — awaiting independent owner review.** No approval is claimed on the owner's behalf. |
