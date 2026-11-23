import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
	BRAND,
	CONTROL_HEIGHT,
	ICON_STROKE_WIDTH,
	RADIUS,
	SEMANTIC_COLOR_MAP,
	SEMANTIC_RADIUS_MAP,
	SPACE,
	STATUS,
	SURFACE,
	TOKENS,
	TYPE
} from './tokens';

/**
 * Frozen token contract (V2.2).
 *
 * These assertions are EXACT. There is no approximate color matching and no
 * tolerance: if a hex value drifts by one character the suite fails. That is
 * the point — a design system that can be "nearly right" is not frozen.
 *
 * The values come from the approved Pen.dev artifact. The CSS file is parsed
 * and compared against tokens.ts, so the stylesheet and the TypeScript source
 * cannot diverge.
 */

const here = dirname(fileURLToPath(import.meta.url));
const read = (...segments: string[]) => readFileSync(join(here, ...segments), 'utf8');

/**
 * Removes CSS comments so structural assertions inspect live declarations.
 *
 * Without this, prose that *names* a retired value (for example the removed
 * emerald accent) would read as a live value and fail the frozen-token
 * contract. Stripping comments keeps every assertion honest: it still fails on
 * any real declaration, it just stops failing on documentation.
 */
const stripComments = (source: string) => source.replace(/\/\*[\s\S]*?\*\//g, '');

/** Parses `--name: value;` declarations out of a :root block. */
function parseCssVariables(css: string): Record<string, string> {
	const rootMatch = /:root\s*\{([\s\S]*?)\}/.exec(css);
	if (!rootMatch) throw new Error('tokens.css must declare a :root block');
	const out: Record<string, string> = {};
	for (const line of rootMatch[1].split('\n')) {
		const declaration = /^\s*(--[a-z0-9-]+)\s*:\s*([^;]+);/i.exec(line);
		if (declaration) out[declaration[1]] = declaration[2].trim();
	}
	return out;
}

/** WCAG 2.x relative luminance for a `#RRGGBB` string. */
function luminance(hex: string): number {
	const value = hex.replace('#', '');
	const channel = (offset: number) => {
		const c = Number.parseInt(value.slice(offset, offset + 2), 16) / 255;
		return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	};
	return 0.2126 * channel(0) + 0.7152 * channel(2) + 0.0722 * channel(4);
}

/** WCAG 2.x contrast ratio between two `#RRGGBB` strings. */
function contrast(a: string, b: string): number {
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

describe('frozen design tokens', () => {
	it('pins the exact brand colors', () => {
		expect(BRAND).toEqual({
			primary: '#0F766E',
			primaryStrong: '#115E59',
			primaryHover: '#0B6B63',
			primaryActive: '#084F49',
			primaryTint: '#E6F1EF',
			onPrimary: '#FFFFFF'
		});
	});

	it('pins the exact surface and text colors', () => {
		expect(SURFACE).toEqual({
			canvas: '#F7F9F8',
			surface: '#FFFFFF',
			sunken: '#F1F4F3',
			foreground: '#17211F',
			muted: '#5F6F6B',
			border: '#767E7C',
			borderSubtle: '#E0E6E3'
		});
	});

	it('pins the exact semantic status colors and their tints', () => {
		expect(STATUS).toEqual({
			success: '#147A3A',
			successTint: '#E9F4EC',
			warning: '#AC4F08',
			warningTint: '#FBF1E4',
			danger: '#B42318',
			dangerTint: '#FBEDEA',
			info: '#2563A6',
			infoTint: '#EAF1F8'
		});
	});

	it('pins exactly three radii', () => {
		expect(RADIUS).toEqual({ control: '6px', panel: '8px', dialog: '12px' });
		expect(Object.values(RADIUS).every((value) => /^\d+px$/.test(value))).toBe(true);
	});

	it('pins citizen controls to 44px and admin controls to 36/40px', () => {
		expect(CONTROL_HEIGHT).toEqual({ citizen: 44, adminCompact: 36, adminComfortable: 40 });
		expect(CONTROL_HEIGHT.citizen).toBeGreaterThanOrEqual(44);
		expect(CONTROL_HEIGHT.adminCompact).toBeGreaterThanOrEqual(36);
		expect(CONTROL_HEIGHT.adminComfortable).toBeGreaterThanOrEqual(40);
	});

	it('keeps a 4px-based spacing ladder', () => {
		expect(Object.values(SPACE)).toEqual([
			'4px',
			'8px',
			'12px',
			'16px',
			'24px',
			'32px',
			'48px',
			'64px'
		]);
		for (const value of Object.values(SPACE)) {
			expect(Number.parseInt(value, 10) % 4).toBe(0);
		}
	});

	it('pins the type scale, with no step below 11px', () => {
		expect(TYPE).toEqual({
			display: 32,
			pageTitle: 20,
			subtitle: 15,
			body: 14,
			label: 13,
			caption: 12,
			micro: 11
		});
		for (const [name, size] of Object.entries(TYPE)) {
			expect(size, `type step ${name} must be at least 11px`).toBeGreaterThanOrEqual(11);
		}
	});

	it('standardizes a single icon stroke width', () => {
		expect(ICON_STROKE_WIDTH).toBe(1.75);
	});

	it('exposes no tokens beyond the frozen set', () => {
		expect(Object.keys(TOKENS).sort()).toEqual(
			[
				'--sigap-primary',
				'--sigap-primary-strong',
				'--sigap-primary-hover',
				'--sigap-primary-active',
				'--sigap-primary-tint',
				'--sigap-on-primary',
				'--sigap-canvas',
				'--sigap-surface',
				'--sigap-surface-sunken',
				'--sigap-foreground',
				'--sigap-muted',
				'--sigap-border',
				'--sigap-border-subtle',
				'--sigap-success',
				'--sigap-success-tint',
				'--sigap-warning',
				'--sigap-warning-tint',
				'--sigap-danger',
				'--sigap-danger-tint',
				'--sigap-info',
				'--sigap-info-tint',
				'--sigap-radius-control',
				'--sigap-radius-panel',
				'--sigap-radius-dialog',
				'--sigap-control-citizen',
				'--sigap-control-admin-compact',
				'--sigap-control-admin-comfortable',
				'--sigap-space-1',
				'--sigap-space-2',
				'--sigap-space-3',
				'--sigap-space-4',
				'--sigap-space-5',
				'--sigap-space-6',
				'--sigap-space-7',
				'--sigap-space-8',
				'--sigap-type-display',
				'--sigap-type-page-title',
				'--sigap-type-subtitle',
				'--sigap-type-body',
				'--sigap-type-label',
				'--sigap-type-caption',
				'--sigap-type-micro',
				'--sigap-icon-stroke'
			].sort()
		);
	});

	it('maps every semantic color name onto a CSS variable', () => {
		for (const [name, value] of Object.entries(SEMANTIC_COLOR_MAP)) {
			if (typeof value === 'string') {
				expect(value, `semantic color ${name} must be a CSS variable`).toMatch(/^var\(--sigap-/);
			} else {
				for (const [shade, shadeValue] of Object.entries(value)) {
					expect(shadeValue, `brand.${shade} must be a CSS variable`).toMatch(/^var\(--sigap-/);
				}
			}
		}
		for (const [name, value] of Object.entries(SEMANTIC_RADIUS_MAP)) {
			expect(value, `semantic radius ${name} must be a CSS variable`).toMatch(/^var\(--sigap-radius-/);
		}
	});

	describe('tokens.css mirrors tokens.ts', () => {
		const cssVariables = parseCssVariables(read('tokens.css'));

		it('declares exactly the frozen token set, with no extras', () => {
			expect(Object.keys(cssVariables).sort()).toEqual(Object.keys(TOKENS).sort());
		});

		it('agrees on every value, case-insensitively for hex', () => {
			for (const [name, value] of Object.entries(TOKENS)) {
				const declared = cssVariables[name];
				expect(declared, `${name} must be declared in tokens.css`).toBeDefined();
				const normalize = (input: string) =>
					input.startsWith('#') ? input.toLowerCase() : input;
				expect(normalize(declared as string), `${name} must match tokens.ts`).toBe(
					normalize(value as string)
				);
			}
		});
	});

	it('removes the legacy emerald accent token', () => {
		// Comments are stripped so the explanatory note that documents the
		// removal does not itself read as a live declaration.
		const appCss = stripComments(read('..', '..', 'app.css'));
		expect(appCss).not.toMatch(/--accent\s*:/);
		expect(appCss).not.toMatch(/#059669/i);
		const cssVariables = parseCssVariables(read('tokens.css'));
		expect(cssVariables['--accent']).toBeUndefined();
	});

	it('introduces no new visual values outside the token source', () => {
		// Every hex literal that survives in the design source must be one of
		// the frozen token values. Comments are stripped first so documentation
		// that NAMES a retired color (for example the removed emerald accent)
		// does not read as a live value.
		const allowed = new Set(
			[
				...Object.values(BRAND),
				...Object.values(SURFACE),
				...Object.values(STATUS)
			].map((hex) => hex.toLowerCase())
		);
		for (const file of ['tokens.css']) {
			const source = stripComments(read(file));
			for (const match of source.matchAll(/#[0-9a-f]{3,8}\b/gi)) {
				expect(allowed.has(match[0].toLowerCase()), `${file} contains unfrozen color ${match[0]}`).toBe(
					true
				);
			}
		}
	});

	/**
	 * The accessibility contract the token split exists to satisfy.
	 *
	 * These are not decorative assertions. The retired palette shipped a single
	 * `border: #E0DDD8` at 1.24:1 wired to every input and button outline, which
	 * fails the WCAG 2.2 AA 3:1 non-text requirement. These tests make that
	 * specific defect impossible to reintroduce.
	 */
	describe('contrast requirements', () => {
		it('keeps the control border at or above 3:1 on every ground it draws on', () => {
			for (const ground of [SURFACE.surface, SURFACE.canvas, SURFACE.sunken]) {
				const ratio = contrast(SURFACE.border, ground);
				expect(
					ratio,
					`control border ${SURFACE.border} on ${ground} must clear 3:1 for non-text contrast`
				).toBeGreaterThanOrEqual(3);
			}
		});

		it('never lets the decorative separator be mistaken for a control border', () => {
			const ratio = contrast(SURFACE.borderSubtle, SURFACE.surface);
			expect(ratio, 'the decorative separator must stay below the 3:1 control threshold').toBeLessThan(
				3
			);
		});

		it('keeps every status tone at or above 4.5:1 on its own tint ground', () => {
			const pairs: Array<[string, string, string]> = [
				[STATUS.success, STATUS.successTint, 'success'],
				[STATUS.warning, STATUS.warningTint, 'warning'],
				[STATUS.danger, STATUS.dangerTint, 'danger'],
				[STATUS.info, STATUS.infoTint, 'info']
			];
			for (const [tone, tint, name] of pairs) {
				const ratio = contrast(tone, tint);
				expect(ratio, `${name} ${tone} on its tint ${tint} must clear 4.5:1`).toBeGreaterThanOrEqual(
					4.5
				);
			}
		});

		it('keeps body text at or above 4.5:1 on every ground it draws on', () => {
			for (const ground of [SURFACE.surface, SURFACE.canvas, SURFACE.sunken]) {
				expect(contrast(SURFACE.foreground, ground)).toBeGreaterThanOrEqual(4.5);
			}
		});

		it('keeps muted supporting text at or above 4.5:1 on surface and canvas', () => {
			for (const ground of [SURFACE.surface, SURFACE.canvas]) {
				expect(
					contrast(SURFACE.muted, ground),
					`muted ${SURFACE.muted} on ${ground} must clear 4.5:1`
				).toBeGreaterThanOrEqual(4.5);
			}
		});

		it('keeps the primary action legible for white text on it', () => {
			expect(contrast(BRAND.onPrimary, BRAND.primary)).toBeGreaterThanOrEqual(4.5);
		});

		/**
		 * The border token is a control boundary, not a text colour. This test
		 * pins the reason so a future author cannot "tidy up" by using it for a
		 * caption and quietly ship a 4.16:1 failure.
		 */
		it('documents that the control border is unsuitable as small body text', () => {
			expect(contrast(SURFACE.border, SURFACE.surface)).toBeLessThan(4.5);
		});
	});
});
