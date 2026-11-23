/**
 * SIGAP design tokens — canonical TypeScript source.
 *
 * V2.2 migration. Every value here is the definitive value taken from the
 * approved Pen.dev artifact (`design/prototypes/sigap.pen`, schema v2.21,
 * 36 variables, SHA-256
 * `4d079d340e3bb08e93ad7e5e66ced5692844bf2548b8850102c94781e4e943a5`).
 * Where the historical DESIGN.md disagreed, the Pen document wins.
 *
 * The token test asserts this object EXACTLY, so adding, removing, or editing a
 * value is a deliberate, visible act rather than an accident.
 *
 * tokens.css mirrors this file for the CSS custom properties consumed by
 * Tailwind and by component styles. The two must stay in sync; the token test
 * verifies the CSS file against this object.
 */

/**
 * Brand color. One brand only; no secondary accent.
 *
 * `primaryStrong` is the readable teal for text on light grounds and for the
 * monospace queue number. `primaryTint` is the queue-number ground.
 */
export const BRAND = {
	primary: '#0F766E',
	primaryStrong: '#115E59',
	primaryHover: '#0B6B63',
	primaryActive: '#084F49',
	primaryTint: '#E6F1EF',
	onPrimary: '#FFFFFF'
} as const;

/**
 * Surface and text colors.
 *
 * Two border tokens, and the distinction is load-bearing:
 *
 * - `border` (#767E7C) is for **essential control boundaries** — inputs,
 *   buttons, chips, table cells. It measures 4.16:1 on white, clearing the
 *   WCAG 2.2 AA 3:1 non-text threshold. It must NEVER be used as text: at
 *   4.16:1 it fails the 4.5:1 text threshold.
 * - `borderSubtle` (#E0E6E3) is for **decorative separators** only — rules,
 *   dividers, list hairlines. It measures 1.28:1 and would fail any control
 *   requirement, which is exactly why it is never wired to one.
 *
 * The previous single `border: #E0DDD8` (1.24:1) was used for both jobs, which
 * is the defect this split exists to fix.
 */
export const SURFACE = {
	canvas: '#F7F9F8',
	surface: '#FFFFFF',
	sunken: '#F1F4F3',
	foreground: '#17211F',
	muted: '#5F6F6B',
	border: '#767E7C',
	borderSubtle: '#E0E6E3'
} as const;

/**
 * Semantic status colors, each with the tint ground it is actually rendered on.
 *
 * A badge is tone text on its own tint, never on white. Contrast is therefore
 * asserted against the tint: success 4.81, warning 4.85, danger 5.76, info
 * 5.40. Checking these against white — which the earlier palette did — passes a
 * pair that never occurs on screen while missing the pair that does.
 */
export const STATUS = {
	success: '#147A3A',
	successTint: '#E9F4EC',
	warning: '#AC4F08',
	warningTint: '#FBF1E4',
	danger: '#B42318',
	dangerTint: '#FBEDEA',
	info: '#2563A6',
	infoTint: '#EAF1F8'
} as const;

/**
 * Corner radii. Exactly three steps: control, panel, dialog. No arbitrary
 * radii are permitted anywhere in the system.
 */
export const RADIUS = {
	control: '6px',
	panel: '8px',
	dialog: '12px'
} as const;

/**
 * Control heights. One brand, two densities:
 * Citizen surfaces are touch-first at 44px minimum; admin surfaces are denser
 * at 36px compact and 40px comfortable.
 */
export const CONTROL_HEIGHT = {
	citizen: 44,
	adminCompact: 36,
	adminComfortable: 40
} as const;

/**
 * The 4px-based spacing ladder.
 *
 * The V2.2 artifact uses 4/8/12/16/24/32/48/64. The previous 8px-only ladder
 * had no way to express the 12px gap the design uses between a label and its
 * input, so components invented raw values instead.
 */
export const SPACE = {
	1: '4px',
	2: '8px',
	3: '12px',
	4: '16px',
	5: '24px',
	6: '32px',
	7: '48px',
	8: '64px'
} as const;

/**
 * Type scale, in px. Two registers: a citizen reading register that is larger
 * and looser, and an admin data register that is smaller and tighter.
 *
 * The previous app.css scale (20/14/13) had no display step at all, which is
 * why no page could open with a 2px rule above a 32px title.
 */
export const TYPE = {
	display: 32,
	pageTitle: 20,
	subtitle: 15,
	body: 14,
	label: 13,
	caption: 12,
	micro: 11
} as const;

/** Icon stroke width, standardized across the single Lucide family. */
export const ICON_STROKE_WIDTH = 1.75;

/** The complete token set, keyed by the CSS custom property name. */
export const TOKENS = {
	'--sigap-primary': BRAND.primary,
	'--sigap-primary-strong': BRAND.primaryStrong,
	'--sigap-primary-hover': BRAND.primaryHover,
	'--sigap-primary-active': BRAND.primaryActive,
	'--sigap-primary-tint': BRAND.primaryTint,
	'--sigap-on-primary': BRAND.onPrimary,
	'--sigap-canvas': SURFACE.canvas,
	'--sigap-surface': SURFACE.surface,
	'--sigap-surface-sunken': SURFACE.sunken,
	'--sigap-foreground': SURFACE.foreground,
	'--sigap-muted': SURFACE.muted,
	'--sigap-border': SURFACE.border,
	'--sigap-border-subtle': SURFACE.borderSubtle,
	'--sigap-success': STATUS.success,
	'--sigap-success-tint': STATUS.successTint,
	'--sigap-warning': STATUS.warning,
	'--sigap-warning-tint': STATUS.warningTint,
	'--sigap-danger': STATUS.danger,
	'--sigap-danger-tint': STATUS.dangerTint,
	'--sigap-info': STATUS.info,
	'--sigap-info-tint': STATUS.infoTint,
	'--sigap-radius-control': RADIUS.control,
	'--sigap-radius-panel': RADIUS.panel,
	'--sigap-radius-dialog': RADIUS.dialog,
	'--sigap-control-citizen': `${CONTROL_HEIGHT.citizen}px`,
	'--sigap-control-admin-compact': `${CONTROL_HEIGHT.adminCompact}px`,
	'--sigap-control-admin-comfortable': `${CONTROL_HEIGHT.adminComfortable}px`,
	'--sigap-space-1': SPACE[1],
	'--sigap-space-2': SPACE[2],
	'--sigap-space-3': SPACE[3],
	'--sigap-space-4': SPACE[4],
	'--sigap-space-5': SPACE[5],
	'--sigap-space-6': SPACE[6],
	'--sigap-space-7': SPACE[7],
	'--sigap-space-8': SPACE[8],
	'--sigap-type-display': `${TYPE.display}px`,
	'--sigap-type-page-title': `${TYPE.pageTitle}px`,
	'--sigap-type-subtitle': `${TYPE.subtitle}px`,
	'--sigap-type-body': `${TYPE.body}px`,
	'--sigap-type-label': `${TYPE.label}px`,
	'--sigap-type-caption': `${TYPE.caption}px`,
	'--sigap-type-micro': `${TYPE.micro}px`,
	'--sigap-icon-stroke': `${ICON_STROKE_WIDTH}`
} as const;

export type SigapTokenName = keyof typeof TOKENS;

/**
 * Maps Tailwind v3 semantic color names onto the CSS variables above, so
 * `bg-surface`, `text-muted`, and `border-line` resolve to the frozen values
 * instead of drifting into the default Tailwind palette. Tailwind v3 is
 * retained deliberately; this is a semantic extension, not a migration.
 *
 * `line` is the essential control boundary and `line-subtle` the decorative
 * separator. Both names exist so a component author has to make the choice
 * explicitly rather than inherit whichever value happened to be wired.
 */
export const SEMANTIC_COLOR_MAP = {
	brand: {
		DEFAULT: 'var(--sigap-primary)',
		strong: 'var(--sigap-primary-strong)',
		hover: 'var(--sigap-primary-hover)',
		active: 'var(--sigap-primary-active)',
		tint: 'var(--sigap-primary-tint)',
		on: 'var(--sigap-on-primary)'
	},
	canvas: 'var(--sigap-canvas)',
	surface: 'var(--sigap-surface)',
	'surface-sunken': 'var(--sigap-surface-sunken)',
	foreground: 'var(--sigap-foreground)',
	muted: 'var(--sigap-muted)',
	line: 'var(--sigap-border)',
	'line-subtle': 'var(--sigap-border-subtle)',
	success: 'var(--sigap-success)',
	'success-tint': 'var(--sigap-success-tint)',
	warning: 'var(--sigap-warning)',
	'warning-tint': 'var(--sigap-warning-tint)',
	danger: 'var(--sigap-danger)',
	'danger-tint': 'var(--sigap-danger-tint)',
	info: 'var(--sigap-info)',
	'info-tint': 'var(--sigap-info-tint)'
} as const;

export const SEMANTIC_RADIUS_MAP = {
	control: 'var(--sigap-radius-control)',
	panel: 'var(--sigap-radius-panel)',
	dialog: 'var(--sigap-radius-dialog)'
} as const;
