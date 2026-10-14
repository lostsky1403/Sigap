---
name: "Sigap Civic Health"
category: Brands
surface: web
colors:
  background: "#f7f6f3"
  foreground: "#1c1b1a"
  accent: "#0f766e"
  surface: "#ffffff"
  muted: "#57534e"
  border: "#e0ddd8"
---

# Sigap Civic Health

> Category: Brands

> Surface: web

*Layanan kesehatan warga yang cepat, tenang, dan tepercaya*

Sigap is an Indonesian civic-health digital service for citizens, patients, and healthcare facility operators.

The product helps citizens find healthcare facilities, make appointments, check in, and track visit/queue status. Facility staff use it to manage queues, appointments, schedules, facilities, and operational notifications.

The redesign must feel trustworthy, calm, precise, accessible, modern, and suitable for a real public-health digital service.

Avoid generic SaaS aesthetics, AI-generated visual patterns, gradients, glassmorphism, excessive cards, excessive rounded corners, huge hero sections, fake metrics, decorative illustrations, and startup-style marketing UI.

Citizen-facing interfaces should be friendly and task-oriented. Admin interfaces should be operational, information-dense, fast to scan, and desktop-first.

All UI copy should use natural Indonesian.

This document is the canonical design-token source. Any quick-reference or generated copy (such as the
palette table in `design/SKILLS.md` or the kit files under `design/system/`) must match this document;
where they differ, this document wins.

## Color Palette

| Role | Name | Hex | Usage |
| --- | --- | --- | --- |
| background | Background | `#f7f6f3` | page canvas |
| foreground | Foreground | `#1c1b1a` | body text and headings |
| accent | Accent | `#0f766e` | brand accent, mirrors primary teal |
| surface | Surface | `#ffffff` | cards and panels |
| muted | Muted | `#57534e` | secondary text and metadata |
| border | Border | `#e0ddd8` | rules and dividers |

## Typography
- **Display:** Inter — weights 400, 700 — fallbacks: system-ui, -apple-system, Segoe UI, Helvetica Neue, Arial, sans-serif
- **Body:** Inter — weights 400, 700 — fallbacks: system-ui, -apple-system, Segoe UI, Helvetica Neue, Arial, sans-serif

## Voice & Tone

- **Adjectives:** trustworthy, calm, precise, accessible, modern, human
- **Tone:** Concise, respectful and useful Indonesian public-service tone. Calm and precise for health tasks, never marketing hype or startup enthusiasm.

### Messaging pillars
- Indonesian civic-health service for citizens, patients, and facility operators.
- Citizens find facilities, book appointments, check in, and track visit and queue status.
- Facility staff manage queues, appointments, schedules, facilities, and operational notifications.
- Trustworthy, calm, precise, accessible, and modern public-health service.

### Vocabulary
- **Use:** Beranda, Cari Faskes, Janji Temu, Check-In, Status Kunjungan, Antrean, Jadwal, Fasilitas, Notifikasi, Ringkasan
- **Avoid:** generic SaaS terms, marketing hype, startup slogans, exaggerated enthusiasm, technical jargon, lorem ipsum, fake metrics

## Imagery

- **Style:** Clean operational civic-health imagery, no decorative illustrations.
- **Subjects:** healthcare facility search and visit tasks, queue and appointment status, facility operations
- **Treatment:** Flat, legible, high-contrast informational visuals with 1px borders and restrained radius, no gradients or glassmorphism.
- **Avoid:** decorative illustrations, stock photography, giant hero sections, floating UI, decorative blobs, gradient text, emoji icons, fake charts and KPI cards

## Layout

- **Radius:** 8px
- **Border weight:** 1px
- **Spacing:** 8px baseline grid

### Posture rules
- Citizen UI is friendly, task-oriented, and spacious with restrained line lengths.
- Admin UI is denser, operational, scan-first, and desktop-first with left sidebar and facility context.
- CITIZEN/DEFAULT density is canonical: controlHeight 44, controlHeightLG 44; ADMIN/COMPACT only: controlHeight 36, controlHeightLG 40; never use compact as citizen default.
- Canvas is #F7F6F3; containers and elevated surfaces stay #FFFFFF; never derive every container from warm canvas.
- Primary #0F766E, hover #0B6B63, active #084F49; link mirrors primary; border #E0DDD8.
- Use 6px radius for inputs and buttons, 8px for panels, 12px for dialogs, never pills or oversized rounded containers.
- Structure with spacing, typography, borders, and surfaces; shadows only for semantic elevation.
- Tables over cards for operational data; cards only for distinct conceptual units.
- Status is never color-only: label plus color plus optional icon.
- No gradients, no glassmorphism, no giant heroes, no excessive cards, no floating UI.
