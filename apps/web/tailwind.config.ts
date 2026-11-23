import type { Config } from 'tailwindcss';
import { SEMANTIC_COLOR_MAP, SEMANTIC_RADIUS_MAP } from './src/lib/design/tokens';

/**
 * Tailwind v3 configuration.
 *
 * The semantic color and radius scales are derived from the token source
 * (src/lib/design/tokens.ts) so `bg-surface`, `text-muted`, `border-line` and
 * `rounded-control` resolve to the frozen values. Tailwind v3 is retained
 * deliberately; this maps semantic names onto CSS variables rather than
 * migrating to v4.
 *
 * Two border names exist on purpose. `border-line` is an essential control
 * boundary and `border-line-subtle` a decorative separator; they are not
 * interchangeable and the token test pins the contrast each must satisfy.
 */
const config: Config = {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			fontFamily: {
				sans: ['Inter', 'system-ui', 'sans-serif'],
				mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'monospace']
			},
			colors: SEMANTIC_COLOR_MAP,
			borderRadius: SEMANTIC_RADIUS_MAP,
			fontSize: {
				display: ['var(--sigap-type-display)', { lineHeight: '1.2', letterSpacing: '-0.02em' }],
				'page-title': ['var(--sigap-type-page-title)', { lineHeight: '1.35', letterSpacing: '-0.01em' }],
				subtitle: ['var(--sigap-type-subtitle)', { lineHeight: '1.5' }],
				body: ['var(--sigap-type-body)', { lineHeight: '1.5' }],
				label: ['var(--sigap-type-label)', { lineHeight: '1.45' }],
				caption: ['var(--sigap-type-caption)', { lineHeight: '1.5' }],
				micro: ['var(--sigap-type-micro)', { lineHeight: '1.45' }]
			},
			height: {
				'control-citizen': 'var(--sigap-control-citizen)',
				'control-admin': 'var(--sigap-control-admin-compact)',
				'control-admin-lg': 'var(--sigap-control-admin-comfortable)'
			},
			minHeight: {
				'control-citizen': 'var(--sigap-control-citizen)',
				'control-admin': 'var(--sigap-control-admin-compact)',
				'control-admin-lg': 'var(--sigap-control-admin-comfortable)'
			},
			spacing: {
				'1': 'var(--sigap-space-1)',
				'2': 'var(--sigap-space-2)',
				'3': 'var(--sigap-space-3)',
				'4': 'var(--sigap-space-4)',
				'5': 'var(--sigap-space-5)',
				'6': 'var(--sigap-space-6)',
				'7': 'var(--sigap-space-7)',
				'8': 'var(--sigap-space-8)',
				'control': 'var(--sigap-space-2)',
				'control-lg': 'var(--sigap-space-4)',
				'control-xl': 'var(--sigap-space-5)'
			}
		}
	},
	plugins: []
};

export default config;
