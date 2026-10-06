import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { ICON_STROKE_WIDTH } from '$lib/design/tokens';
import { Check } from 'lucide-svelte';
import Icon from './Icon.svelte';

/**
 * Icon foundation contract (Phase 3B1 T-3B1-04).
 *
 * The point of these assertions is that the icon language cannot quietly fork.
 * A second icon package, a hand-written SVG, or a call site that overrides the
 * stroke width would each look harmless in review and produce a visibly
 * inconsistent product.
 */

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, '..');

/** Walks the src tree, returning every .svelte file path relative to srcRoot. */
function svelteFiles(dir: string = srcRoot): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir)) {
		const full = join(dir, entry);
		if (statSync(full).isDirectory()) {
			out.push(...svelteFiles(full));
		} else if (entry.endsWith('.svelte')) {
			out.push(full);
		}
	}
	return out;
}

describe('icon foundation', () => {
	it('renders a Lucide glyph with the standardized stroke width', () => {
		const { container } = render(Icon, { props: { icon: Check } });
		const svg = container.querySelector('svg');
		expect(svg).not.toBeNull();
		// 1.75 is the frozen SIGAP weight; Lucide's own default is 2, so this
		// assertion fails if the wrapper ever stops pinning it.
		expect(svg?.getAttribute('stroke-width')).toBe(String(ICON_STROKE_WIDTH));
	});

	it('hides decorative icons from assistive technology', () => {
		const { container } = render(Icon, { props: { icon: Check } });
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('aria-hidden')).toBe('true');
		expect(svg?.getAttribute('role')).toBe('presentation');
	});

	it('gives a meaningful icon an accessible name', () => {
		const { container } = render(Icon, {
			props: { icon: Check, decorative: false, label: 'Selesai' }
		});
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('role')).toBe('img');
		expect(svg?.getAttribute('aria-label')).toBe('Selesai');
		expect(svg?.getAttribute('aria-hidden')).toBeNull();
	});

	it('renders nothing when a meaningful icon is given no label', () => {
		// Fail closed: an unlabelled meaningful icon would ship as an unlabeled
		// control, so the wrapper refuses rather than guessing.
		const { container } = render(Icon, { props: { icon: Check, decorative: false } });
		expect(container.querySelector('svg')).toBeNull();
	});

	it('inherits the surrounding text color by default', () => {
		const { container } = render(Icon, { props: { icon: Check } });
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('stroke')).toBe('currentColor');
	});

	it('uses Lucide as the only icon source across the app', () => {
		const offenders: string[] = [];
		for (const file of svelteFiles()) {
			const source = readFileSync(file, 'utf8');
			// Other icon libraries or bespoke inline SVG both bypass the wrapper
			// and therefore bypass the stroke-width pin.
			if (/from\s+['"](?:@?sveltejs\/)?(?:iconify|heroicons|feather-icons)/.test(source)) {
				offenders.push(`${file}: alternative icon package`);
			}
			if (/<svg[\s>]/i.test(source)) {
				offenders.push(`${file}: inline <svg>`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('never overrides the icon stroke width at a call site', () => {
		const offenders: string[] = [];
		for (const file of svelteFiles()) {
			if (file.endsWith(join('ui', 'Icon.svelte'))) continue;
			if (/stroke-?[Ww]idth=/.test(readFileSync(file, 'utf8'))) {
				offenders.push(file);
			}
		}
		expect(offenders).toEqual([]);
	});
});
