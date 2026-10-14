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
	TOKENS
} from './tokens';

/**
 * Frozen token contract (Phase 3B1 section 5).
 *
 * These assertions are EXACT. There is no approximate color matching and no
 * tolerance: if a hex value drifts by one character the suite fails. That is
 * the point — a design system that can be "nearly right" is not frozen.
 *
 * The CSS file is parsed and compared against tokens.ts, so the stylesheet and
 * the TypeScript source cannot diverge.
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

describe('frozen design tokens', () => {
	it('pins the exact brand colors', () => {
		expect(BRAND).toEqual({
			primary: '#0F766E',
			primaryHover: '#0B6B63',
			primaryActive: '#084F49'
		});
	});

	it('pins the exact surface and text colors', () => {
		expect(SURFACE).toEqual({
			canvas: '#F7F6F3',
			surface: '#FFFFFF',
			foreground: '#1C1B1A',
			muted: '#57534E',
			border: '#E0DDD8'
		});
	});

	it('pins the exact semantic status colors', () => {
		expect(STATUS).toEqual({
			success: '#2F7D32',
			warning: '#B45309',
			danger: '#C4322A',
			info: '#1D6BB5'
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

	it('keeps the 8px spacing rhythm', () => {
		expect(Object.values(SPACE)).toEqual(['8px', '16px', '24px', '32px', '40px', '48px']);
		for (const value of Object.values(SPACE)) {
			expect(Number.parseInt(value, 10) % 8).toBe(0);
		}
	});

	it('standardizes a single icon stroke width', () => {
		expect(ICON_STROKE_WIDTH).toBe(1.75);
	});

	it('exposes no tokens beyond the frozen set', () => {
		expect(Object.keys(TOKENS).sort()).toEqual(
			[
				'--sigap-primary',
				'--sigap-primary-hover',
				'--sigap-primary-active',
				'--sigap-canvas',
				'--sigap-surface',
				'--sigap-foreground',
				'--sigap-muted',
				'--sigap-border',
				'--sigap-success',
				'--sigap-warning',
				'--sigap-danger',
				'--sigap-info',
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
			[...Object.values(BRAND), ...Object.values(SURFACE), ...Object.values(STATUS)].map((hex) =>
				hex.toLowerCase()
			)
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
});
