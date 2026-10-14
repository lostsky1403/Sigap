import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * Playwright assertion discipline (Phase 3B6, T-3B6-01).
 *
 * WHY THIS EXISTS
 *
 * Playwright's web-first assertions return a promise. Dropping it — writing
 * `expect(locator).toHaveCount(0)` without `await` — is silent, and it is wrong
 * in BOTH directions at once:
 *
 *   1. The assertion never fails the test. The value it checks may be wrong and
 *      the suite still reports green, which is worse than no assertion at all
 *      because it is counted as coverage.
 *   2. The promise settles after the test body returns, so the query can be
 *      in flight when the page is torn down and the run reports a transport
 *      error ("Protocol error: session closed") that names nothing about the
 *      real defect.
 *
 * That is not hypothetical. It is exactly how
 * `admin-read.spec.ts`'s "Jadwal resolves labels and fabricates no
 * practitioner" flaked, and the same line had been quietly unable to fail since
 * it was written.
 *
 * This suite is the structural fix. A per-file review cannot catch it, because
 * the line looks correct: the only missing character is `await`, and the
 * absence of an assertion failure reads as success.
 */

const here = dirname(fileURLToPath(import.meta.url));
const e2eDir = join(here, '..', 'e2e');

/**
 * The assertions that retry against a live page and therefore must be awaited.
 *
 * Deliberately NOT the whole `expect` surface: plain value matchers
 * (`toBe`, `toEqual`, `toContain`) on an already-resolved value are synchronous
 * and correctly written without `await`. Only the locator-retrying forms
 * return a promise that has to be waited on.
 *
 * The list is the WHOLE web-first surface, not just the matchers this suite
 * happens to use today. A guard that only knew about the currently-used subset
 * would go quiet exactly when someone reached for a new matcher — and
 * `toHaveURL` in particular is already used at five sites, so omitting it
 * would leave the same silent-failure hole the guard was written to close.
 *
 * The names are checked against `playwright@1.63.0`'s own
 * `types/test.d.ts` (`LocatorAssertions` + `PageAssertions`), so the list can be
 * re-derived rather than trusted. Three were missing until that check was run:
 * `toHaveRole`, `toContainClass` and `toHaveAccessibleErrorMessage`. None is
 * used in the suite yet, which is exactly why they had to be added — the guard's
 * value is that it covers the matcher someone reaches for NEXT.
 */
const RETRYING_ASSERTIONS = [
	'toBeAttached',
	'toBeChecked',
	'toBeDisabled',
	'toBeEditable',
	'toBeEmpty',
	'toBeEnabled',
	'toBeFocused',
	'toBeHidden',
	'toBeInViewport',
	'toBeOK',
	'toBeVisible',
	'toContainClass',
	'toContainText',
	'toHaveAccessibleDescription',
	'toHaveAccessibleErrorMessage',
	'toHaveAccessibleName',
	'toHaveAttribute',
	'toHaveClass',
	'toHaveCount',
	'toHaveCSS',
	'toHaveId',
	'toHaveJSProperty',
	'toHaveRole',
	'toHaveScreenshot',
	'toHaveText',
	'toHaveTitle',
	'toHaveURL',
	'toHaveValue',
	'toHaveValues',
	'toMatchAriaSnapshot',
	'toPass'
] as const;

/** Walks `e2e/`, returning every spec file (including nested support dirs). */
function specFiles(dir: string = e2eDir): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...specFiles(full));
		else if (entry.name.endsWith('.ts')) out.push(full);
	}
	return out;
}

/**
 * Finds every retrying assertion whose owning `expect` is not awaited.
 *
 * Works by locating the assertion method and then walking back to the `expect`
 * that owns it, rather than by scanning line by line: the chains are frequently
 * wrapped across several lines, so a line-oriented check would miss the
 * multi-line form and flag the single-line one for the wrong reason.
 *
 * WHY THE OWNER PATTERN IS NOT THE LITERAL `expect(`.
 *
 * Playwright's `expect` is also callable in qualified forms — `expect.soft(...)`,
 * `expect.poll(...)` and `expect.configure(...)`. Searching for the literal
 * `expect(` finds none of them, so a dropped `expect.soft(locator).toHaveCount(0)`
 * would be invisible to this guard while being exactly the defect it exists to
 * catch: a soft assertion that never runs never fails the test either.
 *
 * So the owner is matched as `expect` plus an optional `.soft`/`.poll`/`.configure`
 * qualifier before the `(`. The qualified forms are matched here even though the
 * suite does not use them today, for the same reason the matcher list is
 * complete: the guard has to cover the construct someone writes next, not only
 * the ones already present.
 */
function unawaitedAssertions(source: string): string[] {
	const offenders: string[] = [];
	const pattern = new RegExp(`\\.(${RETRYING_ASSERTIONS.join('|')})\\(`, 'g');
	const owner = /expect(?:\.(?:soft|poll|configure))?\(/g;

	for (const match of source.matchAll(pattern)) {
		// The nearest preceding owner of any accepted form.
		let expectStart = -1;
		for (const candidate of source.slice(0, match.index).matchAll(owner)) {
			expectStart = candidate.index;
		}
		if (expectStart === -1) continue;

		// The 40 characters before the owner tell us whether it was awaited.
		const before = source.slice(Math.max(0, expectStart - 40), expectStart);
		if (/await\s*$/.test(before)) continue;

		const line = source.slice(0, expectStart).split('\n').length;
		const statement = source.slice(expectStart, match.index + match[0].length);
		offenders.push(`${line}: ${statement.replace(/\s+/g, ' ').trim()}`);
	}
	return offenders;
}

describe('Playwright specs await their web-first assertions', () => {
	it('has no un-awaited retrying assertion', () => {
		// The WALK is proven non-vacuous, not just the detector. `toEqual([])` is
		// also satisfied by finding no files at all, so a broken `e2eDir` join or
		// an emptied `e2e/` would let this test pass while checking nothing.
		const files = specFiles();
		expect(
			files.length,
			'the guard must find the spec files it claims to check'
		).toBeGreaterThan(0);

		const offenders: string[] = [];
		for (const file of files) {
			// NOTE: support/test.ts is deliberately INCLUDED rather than exempted.
			// It is not a mere re-export — `waitForHydration` holds the
			// `toHaveAttribute` that every spec's hydration gate depends on, so a
			// dropped `await` there would silently turn the whole fixture into a
			// no-op, which is exactly the failure this guard exists to catch.
			const source = readFileSync(file, 'utf8');
			for (const offender of unawaitedAssertions(source)) {
				offenders.push(`${file.slice(e2eDir.length + 1)}:${offender}`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('detects a dropped assertion (the check itself is not vacuous)', () => {
		// A guard that cannot fail is the same class of defect it exists to
		// catch, so the detector is proven against a known-bad sample.
		const sample = [
			'\t\tawait expect(page.locator("a")).toBeVisible();',
			'\t\texpect(page.locator(\'input[type="text"]\')).toHaveCount(0);',
			'\t\tawait expect(',
			'\t\t\tpage.getByText("x")',
			'\t\t).toHaveAttribute("role", "alert");'
		].join('\n');

		const found = unawaitedAssertions(sample);
		expect(found).toHaveLength(1);
		expect(found[0]).toContain('toHaveCount');
	});

	it('detects a dropped qualified assertion (soft/poll/configure forms)', () => {
		// The literal-`expect(` detector missed all three qualified forms, so
		// each is proven here against a sample rather than assumed covered.
		const sample = [
			'\t\tawait expect.soft(page.locator("a")).toBeVisible();',
			'\t\texpect.soft(page.locator("b")).toHaveCount(0);',
			'\t\texpect.poll(() => page.locator("c").count()).toBeHidden();',
			'\t\tawait expect.configure({ timeout: 1000 })(',
			'\t\t\tpage.locator("d")',
			'\t\t).toBeEnabled();'
		].join('\n');

		const found = unawaitedAssertions(sample);
		expect(found).toHaveLength(2);
		expect(found.map((f) => f.replace(/^\d+: /, ''))).toEqual([
			'expect.soft(page.locator("b")).toHaveCount(',
			'expect.poll(() => page.locator("c").count()).toBeHidden('
		]);
	});
});
