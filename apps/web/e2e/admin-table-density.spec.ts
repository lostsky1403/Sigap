import { expect, test, type Page } from '@playwright/test';

/**
 * Phase 3B4.1: the admin table density contract, measured.
 *
 * WHY THIS IS AN E2E TEST AND NOT A COMPONENT TEST.
 *
 * T-3B4-02 requires rendered admin data rows between 36px and 40px. That is a
 * claim about LAYOUT, and jsdom does no layout: it injects no component
 * stylesheet (verified — a rendered DataTable leaves `document.querySelectorAll
 * ('style')` empty) and `getComputedStyle` returns empty strings for
 * component-scoped rules. A Vitest assertion on computed height would therefore
 * pass against any value at all, including a deleted stylesheet. That is the
 * same vacuous-test failure mode rejected in Phase 3B4, so this contract is
 * measured in a real browser instead.
 *
 * WHAT IS MEASURED, and why each part matters:
 *
 *  - `getBoundingClientRect().height` on a real `<tr>`, not a CSS declaration.
 *    The declaration is the intent; the rect is what an operator's eye actually
 *    gets, and they differ whenever padding, borders, or a tall child control
 *    pushes past the declared height.
 *  - EVERY data row on the page, not a sample. One over-tall row in a
 *    thousand-row table is still a broken density contract, and a `.first()`
 *    check would never see it.
 *  - The 11px uppercase header, read back from the rendered `th` rather than
 *    from the stylesheet.
 *  - That no action control was shrunk to hit the number: a row that is 40px
 *    because its buttons were crushed to 24px has not satisfied anything.
 */

const DESKTOP = { width: 1440, height: 900 };
const SETTLE = { timeout: 15_000 };

/** The frozen admin row band from T-3B4-02. */
const ROW_MIN = 36;
const ROW_MAX = 40;

/**
 * The subject this run is pinned to, as exported by Start-LocalE2E.ps1.
 *
 * A zero-scope actor holds no rows, so its pages render the fail-closed empty
 * state with no table. There is then no row whose height could be wrong, and
 * this spec has nothing to measure — it is skipped rather than reinterpreted,
 * because a "0 rows satisfies the density contract" assertion would pass on a
 * table that failed to render at all.
 *
 * The empty-state obligation itself is asserted in admin-read.spec.ts, where it
 * is a positive claim (no table, no leaked identifier) rather than an absence of
 * measurement.
 */
const HOLDS_NO_DATA = (process.env.SIGAP_E2E_ACTOR ?? '').trim() === 'local-zero-scope-admin';

/** The four representative read destinations required by T-3B4.1. */
const TABLES = [
	{ path: '/admin/appointments', label: 'appointments' },
	{ path: '/admin/schedules', label: 'schedules' },
	{ path: '/admin/facilities', label: 'facilities' },
	{ path: '/admin/notifications', label: 'notifications' }
] as const;

async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

test.describe('T-3B4-02: admin table density, measured in a real browser', () => {
	test.use({ viewport: DESKTOP });

	// The whole describe measures rendered rows. An actor that holds none has no
	// rows, and "no rows fell outside the band" is not a density result — it is
	// what a table that failed to render would also produce. Skipping is the
	// honest reading; asserting zero offenders here would be the vacuous one.
	test.skip(
		HOLDS_NO_DATA,
		'this actor holds no rows, so there is no density to measure (asserted in admin-read.spec.ts)'
	);

	for (const target of TABLES) {
		test(`${target.label} renders every data row between ${ROW_MIN}px and ${ROW_MAX}px`, async ({
			page
		}) => {
			await gotoAdmin(page, target.path);

			const table = page.locator('table.sigap-data-table').first();
			await expect(table).toBeVisible(SETTLE);

			// The seeded data must actually have rows, or this asserts nothing.
			const rows = table.locator('tbody tr');
			await expect
				.poll(async () => rows.count(), { timeout: SETTLE.timeout })
				.toBeGreaterThan(0);

			const measured = await rows.evaluateAll((els) =>
				els.map((el) => ({
					height: el.getBoundingClientRect().height,
					// A row whose cells are empty is an empty-state row, not a data
					// row, and it is allowed to be taller (it carries 24px padding).
					isDataRow: el.querySelectorAll('td').length > 0
				}))
			);

			const dataRows = measured.filter((row) => row.isDataRow);
			expect(dataRows.length, `${target.label} must render real data rows`).toBeGreaterThan(0);

			const offenders = dataRows
				.map((row, index) => ({ index, height: row.height }))
				.filter((row) => row.height < ROW_MIN || row.height > ROW_MAX);

			expect(
				offenders,
				`${target.label} rows outside ${ROW_MIN}-${ROW_MAX}px: ${JSON.stringify(
					dataRows.map((r) => r.height)
				)}`
			).toEqual([]);
		});
	}

	test('the header row is 11px uppercase on every admin table', async ({ page }) => {
		for (const target of TABLES) {
			await gotoAdmin(page, target.path);
			const headers = page.locator('table.sigap-data-table thead th');
			await expect(headers.first()).toBeVisible(SETTLE);

			const styles = await headers.evaluateAll((els) =>
				els.map((el) => {
					const computed = getComputedStyle(el);
					return { fontSize: computed.fontSize, textTransform: computed.textTransform };
				})
			);

			for (const style of styles) {
				expect(style.fontSize, `${target.label} header font-size`).toBe('11px');
				expect(style.textTransform, `${target.label} header transform`).toBe('uppercase');
			}
		}
	});

	test('the density is not bought by shrinking the row action controls', async ({ page }) => {
		// A 40px row full of 24px buttons satisfies the band and still fails the
		// operator. The admin action control is 36px compact; it must stay there.
		await gotoAdmin(page, '/admin/appointments');
		const table = page.locator('table.sigap-data-table').first();
		await expect(table).toBeVisible(SETTLE);
		await expect
			.poll(async () => table.locator('tbody tr').count(), { timeout: SETTLE.timeout })
			.toBeGreaterThan(0);

		const controlHeights = await table
			.locator('tbody tr td button')
			.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));

		// The seeded set may legitimately contain no buttons, in which case there
		// is nothing to shrink and the assertion is vacuous rather than failing.
		for (const height of controlHeights) {
			expect(height, 'row action controls must stay at the 36px admin density').toBeGreaterThanOrEqual(36);
		}
	});

	test('shrinking the row did not introduce whole-page horizontal overflow', async ({ page }) => {
		// Density and overflow are related: a narrower table is a wider-looking
		// table if `min-width: 0` was lost somewhere, and that regression is
		// invisible until the layout is measured at a real width.
		for (const target of TABLES) {
			await gotoAdmin(page, target.path);
			await expect(page.locator('table.sigap-data-table').first()).toBeVisible(SETTLE);
			const overflow = await page.evaluate(() => ({
				scrollWidth: document.documentElement.scrollWidth,
				clientWidth: document.documentElement.clientWidth
			}));
			expect(
				overflow.scrollWidth,
				`${target.label} must not scroll the whole page sideways`
			).toBeLessThanOrEqual(overflow.clientWidth + 1);
		}
	});
});
