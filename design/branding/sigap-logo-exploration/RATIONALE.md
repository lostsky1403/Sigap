# SIGAP — logo concept exploration

Three original directions, produced with the installed `logo-design` skill
(`~/.omp/agent/skills/logo-design/`, Design mode, Phases 1–6), stopping at the
three-concept checkpoint.

---

## 1. What was read before drawing

| Source | What it gave |
|---|---|
| `apps/web/src/lib/design/tokens.ts` | `BRAND.primary = #0F766E`, `primaryStrong = #115E59`, one brand colour, no secondary accent; radii 6/8/12; icon stroke 1.75 |
| `apps/web/src/lib/design/tokens.css` | the same values as CSS custom properties |
| `apps/web/src/lib/citizen/CitizenHeader.svelte` | the current mark: a 24 px rounded square in `--sigap-primary` holding a white letter "S" at 14 px/700, beside the wordmark "Sigap" at 17 px/600 and the tagline |
| `F:\Sehat-Hub\design\prototypes\sigap.pen` | the frozen frame: `Brand` = 24×24 `$primary` rectangle at `$r-ctl` + "Sigap" at 17 px/700, tracking −0.01 |
| `design/brand.json` | adjectives: trustworthy, calm, precise, accessible, modern, human; `logo.primary` is explicitly `null` |
| `docs/` (grep for `stands for`, `acronym`, `singkatan`, `artinya`, `kepanjangan`) | **nothing** — no expansion of SIGAP exists anywhere in the repo |
| `CONTRIBUTING.md`, `docs/LOCAL_DEMO_RUNBOOK.md`, `docs/DESIGN.md` | the product's own one-liners: "an open-source Civic-Tech framework for regional health information and queue services"; "a regional health information and queueing platform" |
| `design/brand.json` → `tagline` | "Layanan kesehatan warga yang cepat, tenang, dan tepercaya" — fast, calm, trustworthy |

**SIGAP is therefore treated as a word, not an acronym.** No meaning was invented for the
letters; `sigap` is simply the Indonesian word for prompt, alert, ready, which is consistent with
the product's promise of fast, calm service. Nothing in the three marks depends on an acronym.

### Brief

| | |
|---|---|
| Product | Digital access to public health care: find a facility, book an appointment, check in, track the visit and the queue number. Staff and administrators run the same system. |
| Audience | Citizens and patients first; facility staff and administrators second; public-sector stakeholders third. |
| Qualities | Trustworthy, calm, precise, accessible, modern, human. |
| Must work | 24 px in the header beside "Sigap"; one colour; light, canvas and dark grounds; at 16 px as a favicon. |
| Off-limits | Medical crosses, heartbeat lines, stethoscopes, hospital silhouettes, health shields, stock-icon shapes, AI logo effects, batik/wayang decoration. |

Category research (`search_library.py --industry health`): the reference library holds exactly
one health logo, so there is no category convention to copy or to break. The library was searched
for technique and a handful of exemplary marks (letterform and abstract) were read to study how
their geometry is built. Nothing was traced or adapted; all three marks are drawn from primitives
on a 256 grid.

---

## 2. Longlist — twelve one-liners, and why nine died

| # | Concept | One line | Outcome |
|---|---|---|---|
| 1 | **Route S** | The name as one unbroken stroke with two flat stops — the path from booking to being served. | **built — A** |
| 2 | **Threshold** | An open portal at the ground line: check-in is the moment you cross from waiting into care. | **built — B** |
| 3 | **Channel** | Two equal halves with one service channel between them: the citizen side, the facility side. | **built — C** |
| 4 | Two-part S | The S cut at its neck by a slot: two parties inside one letterform. | dropped — the slot closes at 16 px and reads as a printing fault at 64 px |
| 5 | Seated square | A ring with a square set into its opening: the appointment that completes the cycle. | dropped — reads as a keyhole / padlock |
| 6 | Ticket stub | A queue number with a squared notch. | dropped — literal product artefact |
| 7 | Serpentine | The rope-line queue folded three times. | dropped — the turns need a radius that turns the S into a coil |
| 8 | Counter pair | Two counters facing each other across an aisle. | dropped — ambiguous at 16 px |
| 9 | Tally | One bold marker standing on the queue line. | dropped — reads as a "T" or an exclamation mark |
| 10 | Stepped path | Three stations advancing diagonally. | dropped — reads as a signal-strength icon |
| 11 | Reserved slot | A solid tile with one clean gap. | dropped — same idea as 3, weaker |
| 12 | Folded ribbon | A single band turning 180°. | dropped — reads as a "U" / cup |

The three survivors are three different *kinds* of mark — letterform, architectural pictogram,
abstract — and each one still reads at 16 px.

---

## 3. The three concepts

### A — The Route S · letterform

Two circular bowls of equal radius, tangent at the centre, joined into a single 36-unit monoline
stroke whose terminals are cut flat on the radius. The S is not a typeface letter: it is drawn on
a 256 grid as two annular sectors (outer 67, inner 31, centres 98 apart), so the weight is
constant and the counters are 1.72 stem widths across — wide enough to stay open at 16 px.

The two flat stops are the idea: booking and being served. One continuous route, entered and left.

*Why it fits:* it **is** the name, so it needs no explanation and never competes with the wordmark;
it is a direct upgrade of the placeholder "S" in the 24 px header tile, so it inherits equity the
product already shows; and it is the strongest of the three at favicon size.

### B — The Threshold · architectural pictogram

A semicircular arch of constant 44-unit weight standing on the ground line, opening at the base.
Outer radius 96, inner 52, jambs 44 wide — one weight everywhere, no taper.

The opening is the idea: check-in is a threshold. You arrive as a person in a queue and cross into
care. It is the most human of the three and the least like anything else in the health category,
where the vocabulary is crosses, hearts and shields.

### C — The Channel · abstract

Two equal half-discs facing each other across a 32-unit channel. Each half is a segment of the same
100-radius disc, so the two sides are exactly congruent — neither is subordinate.

The channel is the idea: the service moment between the citizen's side and the facility's side.
It is the boldest silhouette of the three and the one that can carry pattern, motion and a
system (the channel can rotate, close or travel).

---

## 4. Recommendation

**A — The Route S.**

- It is the only concept that *is* the brand name, which makes it the cheapest to learn and the
  safest to live with for a decade.
- It replaces the existing placeholder honestly: the header already shows a white "S" in a 24 px
  teal tile, so A upgrades a mark the product already ships rather than introducing a second idea.
- It is the only one of the three that is still itself at 16 px. B's opening starts filling in
  there — its ink coverage *rises* from 27.6 % to 29.7 % between 32 px and 16 px, which is
  anti-aliasing closing the gap — while A's stays flat at 25.0 % across the whole ladder.

Honest risks:

- **A** is the least *ownable* of the three on sight. Ownership has to come from the proportions
  (condensed S, equal circular bowls, counters just under two stems, flat radial terminals) and
  from the lockup with the wordmark — kit work, not concept work.
- **B** reads as an arch before it reads as a threshold, and its opening is soft below 24 px.
- **C** is the most distinctive shape and the least specific meaning; at 16 px it flattens into a
  split dot, so it would need a simplified small-size cut for favicon use.

---

## 5. Evidence — what the skill's own tooling reported

All three files were audited with the skill's `svg_audit.py`; the output is in
`audit-output.txt` (and `audit-dark-bg.txt` for the dark-ground contrast pass).

```
=== concept-a.svg  (432 bytes)
viewBox 0 0 256 256 · aspect 1.0 (square)
colours 1: #0f766e · gradients 0 · anchors 10
margins L61.2 R61.2 T11.0 B13.0 · bbox fill 0.473
✔ no issues found
production-readiness score: 100/100 (heuristic)

=== concept-b.svg  (350 bytes)
viewBox 0 0 256 256 · aspect 1.0 (square)
colours 1: #0f766e · gradients 0 · anchors 9
margins L32.0 R32.0 T32.0 B32.0 · bbox fill 0.562
✔ no issues found
production-readiness score: 100/100 (heuristic)

=== concept-c.svg  (367 bytes)
viewBox 0 0 256 256 · aspect 1.0 (square)
colours 1: #0f766e · gradients 0 · anchors 6
margins L28.0 R28.0 T29.3 B29.3 · bbox fill 0.602
✔ no issues found
production-readiness score: 100/100 (heuristic)
```

No live text, no rasters, no filters, no masks, no gradients, no strokes left to expand, one
colour, 6–10 anchors — well inside the library's complexity distribution.

**Legibility, measured** (`legibility-test.png`, `legibility-report.txt`): each SVG rendered at
true 64 / 32 / 16 px on light `#FFFFFF`, canvas `#F7F6F3` and dark `#0B1220`, in brand teal, black
and white.

| Concept | 64 px | 32 px | 16 px | ink islands at 16 px |
|---|---|---|---|---|
| A Route S | 27.2 % ink | 25.8 % | 25.0 % | 1 (as drawn) |
| B Threshold | 28.4 % | 27.6 % | 29.7 % | 1 (as drawn) |
| C Channel | 38.4 % | 37.0 % | 39.1 % | 2 (channel survives) |

A holds its weight across the whole ladder. B's coverage *rises* at 16 px, which is the opening
filling in with anti-aliasing — the first sign of a mark losing its idea at favicon size. C keeps
its two islands even at 16 px, so the channel technically survives, but at that size each half is
about 5 px wide and the mark reads as a split dot rather than as two sides.

**Contrast** (WCAG 2.1 relative luminance, computed from the delivered hex):

| Ground | Ratio | Verdict |
|---|---|---|
| `#0F766E` on `#FFFFFF` | 5.47:1 | AA for text and non-text |
| `#0F766E` on `#F7F6F3` (app canvas) | 5.06:1 | AA for text and non-text |
| `#0F766E` on `#0B1220` (dark) | 3.42:1 | non-text only |

Carry-forward for the kit: on dark grounds the mark should use a lightened brand tint rather than
`#0F766E`, or it will sit under the AA graphic threshold.

**Test sheet** (`preview.html`, `preview.png`): size ladder from 16 px up, 1:1 pixel test, light /
canvas / dark / brand-colour / patterned grounds, one-colour black and white, squint blur, mirror
and 180° rotation, and the tab / app-icon / avatar / header / card contexts.

**Method note.** This session cannot display images inline. Every mark was therefore verified
numerically — rasterised at 256/128/64/32/16 px and read back as an ink mask (bounding box, margins,
coverage, ink islands, aperture behaviour) — and by the skill's audit. The delivered PNGs are the
human-checkable record; the final visual judgement is the reviewer's.

---

## 6. Files

| File | What it is |
|---|---|
| `concept-a.svg` | Route S — recommended |
| `concept-b.svg` | Threshold |
| `concept-c.svg` | Channel |
| `concepts.png` | Comparison sheet: large mark, true 64/32/16 px, name, one-line idea, recommendation |
| `concepts.svg` | Vector source of that sheet |
| `concepts-greyscale.png` / `.svg` | Greyscale round (colour is deferred until a direction is chosen) |
| `preview.html` / `preview.png` | Full test sheet |
| `legibility-test.png` | Size ladder on light / canvas / dark |
| `legibility-report.txt` | The measurements above, machine-generated |
| `audit-output.txt`, `audit-dark-bg.txt` | `svg_audit.py` output |
| `_legibility_test.py` | The script that produced the strip and the report (kept so the test is repeatable) |

Deliberately not built, per the checkpoint: the identity kit (wordmark, lockups, small-size cut,
favicon, app icons, presentation board, guidelines). Nothing in the running app was touched —
`CitizenHeader.svelte`, `static/` and the favicon are unchanged.

---

## 7. Next step

Pick a direction, or say what you like in each, and I will prepare the full logo kit for it:
colour and one-colour versions, reversed artwork, horizontal and stacked lockups, a small-size cut,
favicon + app-icon + web-icon set, a presentation board with public-health mockups, and a one-page
usage guide.
