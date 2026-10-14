/**
 * SIGAP frozen design tokens — canonical TypeScript source.
 *
 * Every value here is fixed by design/DESIGN.md and must not drift. The token
 * test asserts this object EXACTLY, so adding, removing, or editing a value is
 * a deliberate, visible act rather than an accident.
 *
 * tokens.css mirrors this file for the CSS custom properties consumed by
 * Tailwind and by component styles. The two must stay in sync; the token test
 * verifies the CSS file against this object.
 */

/** Brand color. One brand only; no secondary accent. */
export const BRAND = {
	primary: '#0F766E',
	primaryHover: '#0B6B63',
	primaryActive: '#084F49'
} as const;

/** Surface and text colors. */
export const SURFACE = {
	canvas: '#F7F6F3',
	surface: '#FFFFFF',
	foreground: '#1C1B1A',
	muted: '#57534E',
	border: '#E0DDD8'
} as const;

/** Semantic status colors. */
export const STATUS = {
	success: '#2F7D32',
	warning: '#B45309',
	danger: '#C4322A',
	info: '#1D6BB5'
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

/** 8px spacing rhythm. */
export const SPACE = {
	1: '8px',
	2: '16px',
	3: '24px',
	4: '32px',
	5: '40px',
	6: '48px'
} as const;

/** Icon stroke width, standardized across the single Lucide family. */
export const ICON_STROKE_WIDTH = 1.75;

/** The complete frozen token set, keyed by the CSS custom property name. */
export const TOKENS = {
	'--sigap-primary': BRAND.primary,
	'--sigap-primary-hover': BRAND.primaryHover,
	'--sigap-primary-active': BRAND.primaryActive,
	'--sigap-canvas': SURFACE.canvas,
	'--sigap-surface': SURFACE.surface,
	'--sigap-foreground': SURFACE.foreground,
	'--sigap-muted': SURFACE.muted,
	'--sigap-border': SURFACE.border,
	'--sigap-success': STATUS.success,
	'--sigap-warning': STATUS.warning,
	'--sigap-danger': STATUS.danger,
	'--sigap-info': STATUS.info,
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
	'--sigap-icon-stroke': `${ICON_STROKE_WIDTH}`
} as const;

export type SigapTokenName = keyof typeof TOKENS;

/**
 * Maps Tailwind v3 semantic color names onto the CSS variables above, so
 * `bg-surface`, `text-muted`, and `border-default` resolve to the frozen
 * values instead of drifting into the default Tailwind palette. Tailwind v3 is
 * retained deliberately; this is a semantic extension, not a migration.
 */
export const SEMANTIC_COLOR_MAP = {
	brand: {
		DEFAULT: 'var(--sigap-primary)',
		hover: 'var(--sigap-primary-hover)',
		active: 'var(--sigap-primary-active)'
	},
	canvas: 'var(--sigap-canvas)',
	surface: 'var(--sigap-surface)',
	foreground: 'var(--sigap-foreground)',
	muted: 'var(--sigap-muted)',
	line: 'var(--sigap-border)',
	success: 'var(--sigap-success)',
	warning: 'var(--sigap-warning)',
	danger: 'var(--sigap-danger)',
	info: 'var(--sigap-info)'
} as const;

export const SEMANTIC_RADIUS_MAP = {
	control: 'var(--sigap-radius-control)',
	panel: 'var(--sigap-radius-panel)',
	dialog: 'var(--sigap-radius-dialog)'
} as const;
