import type { Config } from 'tailwindcss';
import { SEMANTIC_COLOR_MAP, SEMANTIC_RADIUS_MAP } from './src/lib/design/tokens';

/**
 * Tailwind v3 configuration.
 *
 * The semantic color and radius scales are derived from the frozen token source
 * (src/lib/design/tokens.ts) so `bg-surface`, `text-muted`, `rounded-control`
 * resolve to the frozen values. Tailwind v3 is retained deliberately; this maps
 * semantic names onto CSS variables rather than migrating to v4.
 */
const config: Config = {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			fontFamily: {
				sans: ['Inter', 'system-ui', 'sans-serif']
			},
			colors: {
				brand: SEMANTIC_COLOR_MAP.brand,
				canvas: SEMANTIC_COLOR_MAP.canvas,
				surface: SEMANTIC_COLOR_MAP.surface,
				foreground: SEMANTIC_COLOR_MAP.foreground,
				muted: SEMANTIC_COLOR_MAP.muted,
				line: SEMANTIC_COLOR_MAP.line,
				success: SEMANTIC_COLOR_MAP.success,
				warning: SEMANTIC_COLOR_MAP.warning,
				danger: SEMANTIC_COLOR_MAP.danger,
				info: SEMANTIC_COLOR_MAP.info
			},
			borderRadius: {
				control: SEMANTIC_RADIUS_MAP.control,
				panel: SEMANTIC_RADIUS_MAP.panel,
				dialog: SEMANTIC_RADIUS_MAP.dialog
			},
			spacing: {
				control: 'var(--sigap-space-1)',
				'control-lg': 'var(--sigap-space-2)',
				'control-xl': 'var(--sigap-space-3)'
			},
			height: {
				'control-citizen': 'var(--sigap-control-citizen)',
				'control-admin': 'var(--sigap-control-admin-compact)',
				'control-admin-lg': 'var(--sigap-control-admin-comfortable)'
			}
		}
	},
	plugins: []
};

export default config;
