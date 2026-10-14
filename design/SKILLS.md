# How to use the Sigap Civic Health design system

*Layanan kesehatan warga yang cepat, tenang, dan tepercaya*

This package is a portable **Brands** design system for websites, landing pages, dashboards, decks, and product UI. Hand the unzipped folder to any AI coding agent — Claude Code, Codex, Cursor, Gemini, OpenCode, or Qwen — alongside `DESIGN.md`, and it will produce on-brand work without further art direction.

## What it is good for

- Landing pages & marketing sites
- Slide decks & pitch decks
- Dashboards & product UI
- Prototypes & component mockups

## How to apply it

1. Unzip this folder and open it in your AI coding tool.
2. Tell the agent: "Use `DESIGN.md` as the design system for everything you generate."
3. Ask for the artifact you want — e.g. "a pricing page" or "a 10-slide deck".
4. The agent reads `DESIGN.md` (identity, palette, typography, voice, layout) and the `system/` kit, then matches the brand.

`DESIGN.md` is the single source of truth. The `system/` directory (when present) ships the rendered kit and design tokens — keep them together so the agent can read both.

## Palette quick reference

| Role | Hex |
| --- | --- |
| Background | `#f7f6f3` |
| Foreground | `#1c1b1a` |
| Accent | `#0f766e` |
| Border | `#e0ddd8` |
| Muted | `#57534e` |

Note: `design/DESIGN.md` is the canonical design-token source; this quick reference must match it.
(Phase 3A.1 correction: Muted was drift-corrected here from `#706b65` to the canonical `#57534e`.)

## Tips for better results

- Reference `DESIGN.md` explicitly in every prompt so the agent stays on-brand.
- Ask the agent to pull exact hex values and font families from `DESIGN.md` rather than inventing its own.
- For multi-page or multi-slide work, ask it to reuse the same tokens across every page.
- Iterate by pointing at a specific section of `DESIGN.md` when something looks off.

---

Generated with **OpenDesign** — the open-source, local-first Claude Design alternative. Generate decks, landing pages, dashboards, and brand systems with your favourite AI coding agent.

https://github.com/nexu-io/open-design
