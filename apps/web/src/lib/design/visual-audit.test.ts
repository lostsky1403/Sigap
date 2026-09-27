import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';
import { TOKENS, CONTROL_HEIGHT, RADIUS, ICON_STROKE_WIDTH } from './tokens';

/**
 * Repository-wide visual anti-pattern audit (GATE 2, Phase 3B1.1).
 *
 * `primitives.test.ts` already guards the components in lib/ui. This suite
 * widens the net to the whole NEW design-system surface named in the gate:
 *
 *   - apps/web/src/lib/ui/**      (shared primitives)
 *   - apps/web/src/lib/design/**  (token source)
 *   - apps/web/src/app.css
 *   - apps/web/tailwind.config.ts
 *
 * Scope is deliberately limited to those paths. Legacy route pages have not been
 * migrated yet, and failing GATE 2 because `+layout.svelte` still carries an
 * emerald Tailwind class would punish work that is scheduled for 3B2, not
 * measure whether the design system itself is sound. What must not happen is
 * anti-patterns entering the NEW shared layer, because that is what every
 * migrated route will inherit.
 *
 * Each rule states the exact construct it bans and why, so a future failure
 * explains itself instead of looking like an unexplained red test.
 */

const here = dirname(fileURLToPath(import.meta.url));
// This file lives at apps/web/src/lib/design/, so the web package root is three
// levels up. Getting this wrong silently scans a non-existent path.
const webRoot = join(here, '..', '..', '..');

/** The exact frozen palette. Anything else is drift. */
const FROZEN_HEX = new Set(
	Object.values(TOKENS)
		.filter((v) => /^#[0-9a-f]{6}$/i.test(v))
		.map((v) => v.toLowerCase())
);

/**
 * The only two files permitted to contain a colour literal.
 *
 * `tokens.ts` is the canonical TypeScript source and `tokens.css` is its
 * deliberate CSS mirror; tokens.test.ts asserts the two against each other, so
 * a literal in either IS the token definition, not a hardcode. Every other
 * design-system file must reach colour through `var(--sigap-*)` or a semantic
 * Tailwind class.
 */
const TOKEN_SOURCES = new Set([
	join(webRoot, 'src', 'lib', 'design', 'tokens.css'),
	join(webRoot, 'src', 'lib', 'design', 'tokens.ts')
]);

/** Recursively collects files under `dir` matching `ext`. */
function collect(dir: string, ext: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir)) {
		const full = join(dir, entry);
		if (statSync(full).isDirectory()) {
			out.push(...collect(full, ext));
		} else if (full.endsWith(ext)) {
			out.push(full);
		}
	}
	return out;
}

/**
 * The design-system files under audit, excluding tests.
 *
 * Tests are excluded because they legitimately name banned constructs in order
 * to assert their absence. The two token sources ARE included: they are the
 * files the colour rules are written to permit, and leaving them out would let
 * a stray hex hide in a file no rule ever reads.
 */
function auditedFiles(): string[] {
	const roots = [
		join(webRoot, 'src', 'lib', 'ui'),
		join(webRoot, 'src', 'lib', 'design'),
		// Phase 3B2 citizen shell and catalog. Added to the audited surface for
		// the same reason lib/ui is: these components are the new shared layer,
		// so anti-patterns entering here would propagate to every citizen page
		// rather than staying in one file.
		join(webRoot, 'src', 'lib', 'citizen')
	];
	// `.css` is included deliberately: tokens.css is the CSS mirror of tokens.ts
	// and is the file the colour rule names as permitted. A collector that
	// skipped it would let the token source hide from its own audit, and the
	// "literal confined to token sources" assertion would become unverifiable.
	const exts = ['.svelte', '.ts', '.css'];
	const files = roots
		.flatMap((dir) => exts.flatMap((ext) => collect(dir, ext)))
		.concat([
			join(webRoot, 'src', 'app.css'),
			join(webRoot, 'tailwind.config.ts'),
			// The two citizen routes. They are pages rather than shared components,
			// but they are new citizen surface written in this phase, so they are
			// held to the same rules as the components they compose.
			join(webRoot, 'src', 'routes', '+page.svelte'),
			join(webRoot, 'src', 'routes', 'faskes', '+page.svelte')
		]);

	return files.map((f) => join(f)).filter((f) => !f.endsWith('.test.ts'));
}

/**
 * Strips comments so documentation prose cannot satisfy or trip a rule.
 *
 * This matters more than it looks: a comment explaining that gradients were
 * removed would otherwise be flagged as a gradient, and a test asserting the
 * absence of the word would be satisfied by that same comment. Both directions
 * are false results, so comments are removed before any scan.
 */
function stripComments(source: string): string {
	return source
		.replace(/\/\*[\s\S]*?\*\//g, ' ')
		.replace(/(^|[^:])\/\/.*$/gm, '$1');
}

/**
 * Applies `rule` to every audited file and reports which ones matched.
 *
 * `rule.lastIndex` is reset before each test on purpose. A global regex
 * (`/…/g`) keeps a `lastIndex` cursor between `.test()` calls, so a rule that
 * matched near the end of one file would resume mid-string in the next and miss
 * an offender entirely. That failure mode is silent: the audit reports clean
 * while the construct it exists to ban is sitting in a file. Resetting makes
 * each file an independent test regardless of caller flags.
 */
function scan(rule: RegExp, label: string): string[] {
	const offenders: string[] = [];
	for (const file of auditedFiles()) {
		const source = stripComments(readFileSync(file, 'utf8'));
		rule.lastIndex = 0;
		if (rule.test(source)) offenders.push(`${relative(webRoot, file)}: ${label}`);
	}
	rule.lastIndex = 0;
	return offenders;
}

describe('visual anti-pattern audit: gradients', () => {
	it('uses no CSS gradient function', () => {
		// linear/radial/conic-gradient, in CSS or in a Tailwind arbitrary value.
		expect(scan(/(linear|radial|conic)-gradient/gi, 'CSS gradient')).toEqual([]);
	});

	it('uses no Tailwind gradient utility', () => {
		// `bg-gradient-*` plus the `from-*`/`via-*`/`to-*` stops. `to-*` is only
		// matched in its gradient-stop form, because `to-` also prefixes ordinary
		// utilities such as `topline` in prose and `top-0`-style spacing.
		const offenders = scan(/\bbg-gradient-to-[a-z]+\b/gi, 'bg-gradient-*');
		expect(offenders).toEqual([]);
		expect(scan(/(^|[\s"'`:])(from|via)-[a-z]+-\d{2,3}\b/g, 'gradient stop from-/via-')).toEqual(
			[]
		);
		expect(scan(/(^|[\s"'`:])to-[a-z]+-\d{2,3}\b/g, 'gradient stop to-')).toEqual([]);
	});
});

describe('visual anti-pattern audit: glassmorphism', () => {
	it('uses no backdrop blur or filter', () => {
		expect(scan(/backdrop-(filter|blur)/gi, 'backdrop-filter/blur')).toEqual([]);
		// The bare `backdrop` word is deliberately NOT banned on its own. The
		// native dialog scrim is written as `::backdrop`, which is the modal
		// dimming layer the platform provides, not a blurred glass panel.
		// What would be glassmorphism is a blur attached to it, and the assertion
		// above already rejects that.
	});

	it('uses no glass treatment', () => {
		expect(scan(/glass/gi, 'glass treatment')).toEqual([]);
	});
});

describe('visual anti-pattern audit: decorative shadows', () => {
	it('uses no box-shadow declaration', () => {
		// Focus indication in this system uses `outline`, so any box-shadow is
		// decoration by definition.
		expect(scan(/box-shadow/gi, 'box-shadow')).toEqual([]);
	});

	it('uses no Tailwind shadow utility', () => {
		// shadow-{sm,md,lg,xl,2xl,...}, shadow-none included: a system that
		// specifies no shadows should not even name the utility.
		expect(scan(/(^|[\s"'`:])shadow(-|\[)/g, 'shadow-* utility')).toEqual([]);
	});
});

describe('visual anti-pattern audit: colour literals', () => {
	it('hardcodes no hex colour outside the two token sources', () => {
		const offenders: string[] = [];
		for (const file of auditedFiles()) {
			// A literal inside tokens.ts / tokens.css is the definition itself.
			if (TOKEN_SOURCES.has(file)) continue;
			const source = stripComments(readFileSync(file, 'utf8'));
			// rgb()/hsl() is not scanned: the dialog scrim needs an alpha that a
			// CSS variable cannot express without inventing a new token.
			const withoutFunctions = source.replace(/rgba?\([^)]*\)/g, '').replace(/hsla?\([^)]*\)/g, '');
			for (const match of withoutFunctions.matchAll(/#[0-9a-f]{3,8}\b/gi)) {
				offenders.push(`${relative(webRoot, file)}: ${match[0]}`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('confines every colour literal to the token sources', () => {
		// The exact set of files allowed to contain a hex. Deliberately narrow:
		// the first rule permits a frozen value appearing in the wrong place, and
		// only this one pins WHERE a literal may live at all.
		const withHex = auditedFiles()
			.filter((file) => /#[0-9a-f]{6}\b/i.test(stripComments(readFileSync(file, 'utf8'))))
			.map((f) => relative(webRoot, f).replace(/\\/g, '/'))
			.sort();

		expect(withHex).toEqual(['src/lib/design/tokens.css', 'src/lib/design/tokens.ts']);
	});

	it('redefines no frozen colour outside the token sources', () => {
		// Guards the inverse direction: a hex equal to a frozen value but living
		// in a component would still be drift, because the component would not
		// follow the token if the token changed.
		const offenders: string[] = [];
		for (const file of auditedFiles()) {
			if (TOKEN_SOURCES.has(file)) continue;
			const source = stripComments(readFileSync(file, 'utf8'));
			for (const match of source.matchAll(/#[0-9a-f]{3,8}\b/gi)) {
				if (FROZEN_HEX.has(match[0].toLowerCase())) {
					offenders.push(`${relative(webRoot, file)}: ${match[0]}`);
				}
			}
		}
		expect(offenders).toEqual([]);
	});
});

describe('visual anti-pattern audit: icon family', () => {
	// webRoot is already the web package directory, so its manifest sits at the
	// root itself.
	const webPkg = JSON.parse(readFileSync(join(webRoot, 'package.json'), 'utf8')) as {
		dependencies?: Record<string, string>;
		devDependencies?: Record<string, string>;
	};
	const allDeps = { ...webPkg.dependencies, ...webPkg.devDependencies };

	it('depends on exactly one icon package', () => {
		const ICON_PACKAGES = [
			'lucide-svelte',
			'@lucide/svelte',
			'lucide-react',
			'heroicons',
			'@heroicons/react',
			'@phosphor-icons/svelte',
			'@tabler/icons-svelte',
			'@radix-ui/react-icons',
			'feather-icons',
			'@fortawesome/free-solid-svg-icons'
		];
		const present = ICON_PACKAGES.filter((name) => name in allDeps);
		expect(present).toEqual(['lucide-svelte']);
	});

	it('standardizes one stroke width in the wrapper', () => {
		// The wrapper must pin the width rather than accept it per call site,
		// because a per-instance prop is how a second width creeps in.
		const wrapper = readFileSync(join(webRoot, 'src', 'lib', 'ui', 'Icon.svelte'), 'utf8');
		expect(ICON_STROKE_WIDTH).toBe(1.75);
		expect(wrapper).toContain(String(ICON_STROKE_WIDTH));
		expect(stripComments(wrapper)).not.toMatch(/stroke-width:\s*\$/);
	});
});

describe('visual anti-pattern audit: shape system', () => {
	it('keeps radii to the three frozen steps', () => {
		// Any literal radius outside 6/8/12 (or 0, and 50% for the one spinner
		// circle) is an arbitrary radius, which is the classic shape drift.
		const offenders: string[] = [];
		const allowed = new Set(['0', ...Object.values(RADIUS), '50%']);
		for (const file of auditedFiles()) {
			const source = stripComments(readFileSync(file, 'utf8'));
			for (const match of source.matchAll(/border-radius:\s*([^;]+);/g)) {
				const value = match[1].trim();
				if (value.startsWith('var(--sigap-')) continue;
				if (!allowed.has(value)) offenders.push(`${relative(webRoot, file)}: ${value}`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('keeps control heights to the two frozen densities', () => {
		// The admin densities are exactly two: 36 compact and 40 comfortable.
		// The 44px citizen minimum and the larger touch targets are asserted
		// separately below, because they are floors rather than fixed values.
		expect(CONTROL_HEIGHT).toEqual({ citizen: 44, adminCompact: 36, adminComfortable: 40 });
		const allowedAdmin: number[] = [CONTROL_HEIGHT.adminCompact, CONTROL_HEIGHT.adminComfortable];
		const offenders: string[] = [];
		for (const file of auditedFiles()) {
			const source = stripComments(readFileSync(file, 'utf8'));
			// Scoped to control-ish selectors. A `height` on a header bar or a
			// list container is layout, not a control, and holding layout to the
			// control scale would be the wrong rule.
			for (const match of source.matchAll(
				/\.(sigap-[\w-]*(?:button|link|tab|action|trigger)[\w-]*)\s*\{([^}]*)\}/g
			)) {
				const [, selector, body] = match;
				for (const decl of body.matchAll(/(?:min-)?height:\s*(\d+)px/g)) {
					const value = Number(decl[1]);
					// At or above the citizen floor is a touch-first size, covered
					// by the citizen rule. Sub-36px is not a control at all.
					if (value >= CONTROL_HEIGHT.citizen) continue;
					if (value < CONTROL_HEIGHT.adminCompact) continue;
					if (!allowedAdmin.includes(value)) {
						offenders.push(`${relative(webRoot, file)}: ${selector} ${value}px`);
					}
				}
			}
		}
		expect(offenders).toEqual([]);
	});

	it('keeps every citizen touch target at or above 44px', () => {
		// The 36px admin density is legal in the admin shell and illegal in the
		// citizen shell, where the audience is often using one hand on a phone.
		// A citizen control at 36px would be a 36px target, below every
		// accessibility guideline for touch.
		//
		// The bound is a floor, not an exact match. 44px is the MINIMUM; a tab
		// bar row at 56px or a primary action at 48px is a deliberately more
		// generous target and is not drift. Only a value BELOW the floor is a
		// defect, which is why this is separate from the control-scale rule: same
		// files, different threshold, different reason.
		const offenders: string[] = [];
		for (const file of auditedFiles()) {
			if (!relative(webRoot, file).includes(join('lib', 'citizen'))) continue;
			const source = stripComments(readFileSync(file, 'utf8'));
			for (const match of source.matchAll(
				/\.(sigap-[\w-]*(?:button|link|tab|item|action|trigger|search__input)[\w-]*)\s*\{([^}]*)\}/g
			)) {
				const [, selector, body] = match;
				for (const decl of body.matchAll(/(?:min-)?height:\s*(\d+)px/g)) {
					const value = Number(decl[1]);
					if (value < CONTROL_HEIGHT.citizen) {
						offenders.push(`${relative(webRoot, file)}: ${selector} ${value}px`);
					}
				}
			}
		}
		expect(offenders).toEqual([]);
	});

	it('defines no control height that is neither a token nor a citizen target', () => {
		// Catches a genuinely invented density — a 52px control, say — while
		// allowing the 44/48/56px citizen targets above. Anything at or above the
		// citizen floor is a touch-first size; anything below it must be one of
		// the two frozen admin densities.
		const allowedSmall: number[] = [CONTROL_HEIGHT.adminCompact, CONTROL_HEIGHT.adminComfortable];
		const offenders: string[] = [];
		for (const file of auditedFiles()) {
			const source = stripComments(readFileSync(file, 'utf8'));
			for (const match of source.matchAll(
				/\.(sigap-[\w-]*(?:button|link|tab|item|action|trigger)[\w-]*)\s*\{([^}]*)\}/g
			)) {
				const [, selector, body] = match;
				for (const decl of body.matchAll(/(?:min-)?height:\s*(\d+)px/g)) {
					const value = Number(decl[1]);
					if (value >= CONTROL_HEIGHT.citizen) continue;
					if (value < CONTROL_HEIGHT.adminCompact) continue; // sub-control, e.g. a badge
					if (!allowedSmall.includes(value)) {
						offenders.push(`${relative(webRoot, file)}: ${selector} ${value}px`);
					}
				}
			}
		}
		expect(offenders).toEqual([]);
	});
});

describe('visual anti-pattern audit: scan integrity', () => {
	it('actually inspects the new design-system files', () => {
		// A scan that silently matched zero files would pass every rule above while
		// proving nothing. This pins a floor on coverage.
		const files = auditedFiles().map((f) => relative(webRoot, f).replace(/\\/g, '/'));
		expect(files.length).toBeGreaterThanOrEqual(20);
		expect(files).toContain('src/app.css');
		expect(files).toContain('tailwind.config.ts');
		expect(files).toContain('src/lib/ui/Button.svelte');
		expect(files).toContain('src/lib/design/tokens.ts');
		expect(files).toContain('src/lib/design/tokens.css');
	});

	it('excludes test files from the scan', () => {
		// Test files must be excluded: this file names every banned construct in
		// order to assert its absence, so including it would make the audit
		// contradict itself on the first run.
		const files = auditedFiles().map((f) => relative(webRoot, f).replace(/\\/g, '/'));
		expect(files.some((f) => f.endsWith('.test.ts'))).toBe(false);
	});

	it('audits the new citizen surface but not unmigrated routes', () => {
		// Pins the scope boundary in both directions, because both mistakes are
		// possible and opposite.
		//
		// Too wide: a legacy route still carrying emerald Tailwind classes would
		// fail the gate, punishing migration work scheduled for a later phase
		// rather than measuring whether the new design system is sound.
		//
		// Too narrow: dropping a new citizen page out of the audit would let an
		// anti-pattern ship in the one surface this phase is responsible for.
		//
		// So the two citizen pages written in Phase 3B2 are in, and the older
		// unmigrated routes are out.
		const files = auditedFiles().map((f) => relative(webRoot, f).replace(/\\/g, '/'));
		expect(files).toContain('src/lib/citizen/CitizenHeader.svelte');
		expect(files).toContain('src/lib/citizen/CitizenBottomNav.svelte');
		expect(files).toContain('src/lib/citizen/FacilityResultRow.svelte');
		expect(files).toContain('src/routes/+page.svelte');
		expect(files).toContain('src/routes/faskes/+page.svelte');

		// Unmigrated routes and the admin area stay out of scope.
		const routeFiles = files.filter((f) => f.startsWith('src/routes/'));
		expect(routeFiles.sort()).toEqual(['src/routes/+page.svelte', 'src/routes/faskes/+page.svelte']);
	});
});
