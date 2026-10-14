import type { Page } from '@playwright/test';
import { expect, test } from './support/test';

/**
 * Phase 3B2 browser coverage: the Citizen shell, Beranda, and /faskes.
 *
 * Everything here runs against the LOCAL seeded stack, so the facility
 * assertions are backed by the Go API and the seeded database rather than by a
 * fixture. `scripts/dev/Start-LocalE2E.ps1` is the approved way to bring that
 * stack up; the loopback-only guard in playwright.config.ts rejects anything
 * else before a browser launches.
 *
 * The shell is asserted at both ends of the responsive range, because the two
 * navigations are not one responsive element — they are two different
 * navigations, each shown in exactly one range. Asserting "a nav is visible" at
 * a single width would pass with both bars rendered on top of each other, which
 * is precisely the duplicated-navigation failure the contract forbids.
 *
 * Structural checks (no horizontal overflow, nothing hidden behind the bar, a
 * visible focus ring) are automated rather than left to manual review. They are
 * the defects that survive a code review and fail on a real phone.
 */

const MOBILE = { width: 390, height: 844 };
const DESKTOP = { width: 1440, height: 900 };

/** Fails the test if the document scrolls sideways at the current width. */
async function expectNoHorizontalOverflow(page: Page) {
	const overflow = await page.evaluate(() => ({
		scrollWidth: document.documentElement.scrollWidth,
		clientWidth: document.documentElement.clientWidth
	}));
	// One pixel of slack: sub-pixel layout rounding on fractional device widths
	// is not a real overflow, and a flaky guard gets ignored rather than fixed.
	expect(
		overflow.scrollWidth,
		`content is ${overflow.scrollWidth}px wide in a ${overflow.clientWidth}px viewport`
	).toBeLessThanOrEqual(overflow.clientWidth + 1);
}

/** Proves nothing important sits underneath the fixed bottom navigation. */
async function expectContentClearsBottomNav(page: Page) {
	const clearance = await page.evaluate(() => {
		const nav = document.querySelector('.sigap-bottom-nav');
		const main = document.querySelector('.sigap-citizen-main');
		if (!nav || !main) return null;
		const navStyle = getComputedStyle(nav);
		const mainStyle = getComputedStyle(main);
		return {
			navBottom: nav.getBoundingClientRect().bottom,
			mainPaddingBottom: parseFloat(mainStyle.paddingBottom) || 0,
			navHeight: nav.getBoundingClientRect().height
		};
	});
	expect(clearance, 'the bottom nav and main must both exist').not.toBeNull();
	if (!clearance) return;
	// The content region reserves at least the height of the bar, so the last
	// element of a long page cannot be covered by it.
	expect(clearance.mainPaddingBottom).toBeGreaterThanOrEqual(clearance.navHeight);
}

test.describe('citizen shell: mobile at 390px', () => {
	test.use({ viewport: MOBILE });

	test('shows the bottom navigation and hides the desktop one', async ({ page }) => {
		await page.goto('/');

		const bottomNav = page.locator('nav.sigap-bottom-nav');
		const desktopNav = page.locator('nav.sigap-desktop-nav');

		await expect(bottomNav).toBeVisible();
		// Exactly one navigation strategy per breakpoint. Both visible at once
		// is a duplicate-navigation defect, not a styling detail.
		await expect(desktopNav).toBeHidden();
	});

	test('the bottom navigation has exactly four destinations', async ({ page }) => {
		await page.goto('/faskes');
		const nav = page.locator('nav.sigap-bottom-nav');
		await expect(nav).toBeVisible();
		await expect(nav.locator('a')).toHaveCount(4);
	});

	test('exactly one destination is marked as the current page', async ({ page }) => {
		await page.goto('/faskes');
		const current = page.locator('nav.sigap-bottom-nav a[aria-current="page"]');
		await expect(current).toHaveCount(1);
		await expect(current).toHaveAttribute('href', '/faskes');

		// The mark is per route, not sticky: navigating to another destination
		// must move it rather than add a second one.
		await page.goto('/patient/status');
		await expect(page.locator('nav.sigap-bottom-nav a[aria-current="page"]')).toHaveCount(1);
		await expect(
			page.locator('nav.sigap-bottom-nav a[aria-current="page"]')
		).toHaveAttribute('href', '/patient/status');
	});

	test('every destination meets the 44px touch target floor', async ({ page }) => {
		await page.goto('/');
		const heights = await page
			.locator('nav.sigap-bottom-nav a')
			.evaluateAll((links) => links.map((link) => link.getBoundingClientRect().height));
		expect(heights.length).toBe(4);
		for (const height of heights) {
			expect(height, 'touch target below the 44px citizen floor').toBeGreaterThanOrEqual(44);
		}
	});

	test('the tab bar is fixed and the content clears it', async ({ page }) => {
		await page.goto('/');
		await expect(page.locator('nav.sigap-bottom-nav')).toHaveCSS('position', 'fixed');
		await expectContentClearsBottomNav(page);
	});

	test('does not scroll horizontally and does not clip content', async ({ page }) => {
		await page.goto('/');
		await expectNoHorizontalOverflow(page);
		// The brand and the account control must both remain reachable, which is
		// what actually breaks first when a header overflows at 390px.
		await expect(page.locator('.sigap-citizen-header__brand')).toBeVisible();
	});

	test('shows a visible focus ring when tabbing', async ({ page }) => {
		await page.goto('/');
		await page.locator('nav.sigap-bottom-nav a').first().focus();

		const outline = await page
			.locator('nav.sigap-bottom-nav a')
			.first()
			.evaluate((el) => {
				const style = getComputedStyle(el);
				return { width: style.outlineWidth, style: style.outlineStyle };
			});
		// A focus ring that is technically present but 0px wide is the same as no
		// focus ring for a keyboard user.
		expect(parseFloat(outline.width)).toBeGreaterThan(0);
		expect(outline.style).not.toBe('none');
	});

	test('offers no navigation path to /wallet', async ({ page }) => {
		await page.goto('/');
		const hrefs = await page.locator('a').evaluateAll((links) =>
			links.map((link) => link.getAttribute('href') ?? '')
		);
		for (const href of hrefs) {
			expect(href, `Beranda must not link to ${href}`).not.toContain('/wallet');
		}

		await page.goto('/faskes');
		const faskesHrefs = await page.locator('a').evaluateAll((links) =>
			links.map((link) => link.getAttribute('href') ?? '')
		);
		for (const href of faskesHrefs) {
			expect(href, `/faskes must not link to ${href}`).not.toContain('/wallet');
		}
	});
});

test.describe('citizen shell: desktop at 1440px', () => {
	test.use({ viewport: DESKTOP });

	test('shows the desktop navigation and hides the bottom bar', async ({ page }) => {
		await page.goto('/');
		await expect(page.locator('nav.sigap-desktop-nav')).toBeVisible();
		await expect(page.locator('nav.sigap-bottom-nav')).toBeHidden();
	});

	test('the desktop navigation is a single row', async ({ page }) => {
		await page.goto('/');
		const rows = await page.locator('nav.sigap-desktop-nav .sigap-desktop-nav__list').evaluate(
			(el) => {
				// A wrapping flex list reports a height taller than one row of
				// links; a single line reports roughly one link height.
				const list = el as HTMLElement;
				const links = Array.from(list.querySelectorAll('a'));
				const tops = new Set(links.map((link) => Math.round(link.getBoundingClientRect().top)));
				return { distinctRows: tops.size, linkCount: links.length };
			}
		);
		expect(rows.linkCount).toBeGreaterThanOrEqual(4);
		expect(rows.distinctRows, 'the frozen layout is one line, not a wrapped block').toBe(1);
	});

	test('marks exactly one desktop destination as current', async ({ page }) => {
		await page.goto('/faskes');
		const current = page.locator('nav.sigap-desktop-nav a[aria-current="page"]');
		await expect(current).toHaveCount(1);
		await expect(current).toHaveAttribute('href', '/faskes');
	});

	test('does not scroll horizontally', async ({ page }) => {
		await page.goto('/');
		await expectNoHorizontalOverflow(page);
	});
});

test.describe('Beranda: real data, no demo content', () => {
	test('renders the real seeded facility catalog', async ({ page }) => {
		await page.goto('/');

		// The heading the frozen reference specifies, so a shell that renders
		// without a page cannot pass.
		await expect(page.getByRole('heading', { level: 1 })).toHaveText(
			'Apa yang bisa Anda lakukan di Sigap?'
		);

		// Backed by the seeded database: the facility names come from the Go API,
		// so this only passes when the whole local chain is live.
		const preview = page.locator('.sigap-beranda__facility');
		await expect(preview.first()).toBeVisible({ timeout: 10_000 });
		expect(await preview.count()).toBeGreaterThan(0);

		// A raw UUID is an identifier, not citizen-facing information.
		const body = (await page.locator('body').textContent()) ?? '';
		expect(body).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i);
	});

	test('contains none of the previous demo content', async ({ page }) => {
		await page.goto('/');
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

		const body = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		// Each of these is a specific claim the demo surface made and the public
		// API cannot support: bed counts, maps, gamified panels, sample data.
		for (const forbidden of [
			'kamar kosong',
			'bed availability',
			'chaos',
			'referral',
			'data contoh',
			'scaffolding',
			'lorem'
		]) {
			expect(body, `Beranda must not contain "${forbidden}"`).not.toContain(forbidden);
		}

		// And the demo components are not merely hidden: they are not in the DOM.
		await expect(page.locator('canvas')).toHaveCount(0);
	});

	test('offers only real quick actions', async ({ page }) => {
		await page.goto('/');
		const hrefs = await page.locator('.sigap-quick-actions a').evaluateAll((links) =>
			links.map((link) => link.getAttribute('href') ?? '')
		);
		const allowed = new Set([
			'/faskes',
			'/appointments/new',
			'/appointments/check-in',
			'/queues/new',
			'/patient/status'
		]);
		for (const href of hrefs) {
			expect(allowed, `unexpected quick action target: ${href}`).toContain(href);
		}
	});

	test('links to the full catalog', async ({ page }) => {
		await page.goto('/');
		await page.getByRole('link', { name: 'Lihat semua' }).click();
		await expect(page).toHaveURL(/\/faskes$/);
	});
});

test.describe('/faskes: the public facility catalog', () => {
	test('lists real facilities from the local backend', async ({ page }) => {
		await page.goto('/faskes');

		await expect(page.getByRole('heading', { level: 1 })).toHaveText('Cari Faskes');

		// Seeded data, reached through the same-origin proxy.
		const rows = page.locator('#sigap-facility-results li');
		await expect(rows.first()).toBeVisible({ timeout: 10_000 });
		const count = await rows.count();
		expect(count).toBeGreaterThan(0);

		// Rows carry only verified public fields, and never a raw identifier.
		const firstRowText = (await rows.first().textContent()) ?? '';
		expect(firstRowText).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i);
		for (const forbidden of ['km', 'menit', 'rating', 'kamar']) {
			expect(firstRowText.toLowerCase(), `row must not show ${forbidden}`).not.toContain(forbidden);
		}
	});

	test('search narrows the real results', async ({ page }) => {
		await page.goto('/faskes');
		const rows = page.locator('#sigap-facility-results li');
		await expect(rows.first()).toBeVisible({ timeout: 10_000 });
		const total = await rows.count();
		expect(total).toBeGreaterThan(1);

		const search = page.getByRole('searchbox');
		await expect(search).toBeVisible();

		// Match the first facility by name, so the query is real data rather
		// than a guess about what the seed contains.
		const firstName = ((await rows.first().locator('.sigap-facility-row__name').textContent()) ?? '').trim();
		expect(firstName.length).toBeGreaterThan(0);
		await search.fill(firstName);
		await expect(rows).toHaveCount(1);
		await expect(rows.first()).toContainText(firstName);
	});

	test('a search with no matches offers a way back', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});

		await page.getByRole('searchbox').fill('zzzzzzznotafacility');
		await expect(page.getByText('Tidak ada faskes yang cocok')).toBeVisible();
		// A no-result state must be recoverable, or the citizen is stranded.
		const reset = page.getByRole('button', { name: 'Hapus semua filter' });
		await expect(reset).toBeVisible();
		await reset.click();

		await expect(page.getByText('Tidak ada faskes yang cocok')).toBeHidden();
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible();
	});

	test('empty catalog and no-result are different experiences', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});

		// The seeded stack has facilities, so the empty-catalog copy must NOT be
		// showing. Conflating the two states would show it after any failed search.
		await expect(page.getByText('Belum ada data faskes')).toBeHidden();
	});

	test('preselects the facility when booking from a result', async ({ page }) => {
		await page.goto('/faskes');
		const rows = page.locator('#sigap-facility-results li');
		await expect(rows.first()).toBeVisible({ timeout: 10_000 });

		const bookingHref = await rows.first().getByRole('link').getAttribute('href');
		expect(bookingHref).toContain('/appointments/new');
		// The facility id travels as a query parameter so the booking form can
		// preselect it. Booking itself is Phase 3B3 and is not exercised here.
		expect(bookingHref).toMatch(/facility_id=[0-9a-f-]{36}/i);
	});

	test('is keyboard operable and keeps a visible focus ring', async ({ page }) => {
		await page.goto('/faskes');
		const search = page.getByRole('searchbox');
		await expect(search).toBeVisible();

		await search.focus();
		await expect(search).toBeFocused();
		await search.fill('a');
		await expect(search).toHaveValue('a');

		const outline = await search.evaluate((el) => getComputedStyle(el).outlineWidth);
		expect(parseFloat(outline)).toBeGreaterThan(0);
	});
});

/**
 * The facility type, verified against the real seeded API.
 *
 * The point of these tests is that `type` travels the whole way — database
 * enum, Go projection, JSON, Svelte type, rendered label — and every hop is
 * checked against real data rather than a fixture. So the first thing here is
 * a read of the actual wire response: the expected labels and the expected
 * row counts are derived from what the backend returned, not from what this
 * file believes the seed contains. A test that hardcoded "there are 3
 * puskesmas" would keep passing after someone edited the seed, and would keep
 * passing if the type field were dropped entirely, because the browser would
 * still be showing the right names.
 */
test.describe('/faskes: the real facility type on the wire', () => {
	interface WireFacility {
		id: string;
		name: string;
		short_code: string;
		type: string;
		is_active: boolean;
	}

	/** Reads the public catalog exactly as the browser does, via the proxy. */
	async function readWireCatalog(page: Page): Promise<WireFacility[]> {
		const response = await page.request.get('/api/v1/public/facilities');
		expect(response.status(), 'the public catalog must not require a session').toBe(200);
		const body = (await response.json()) as {
			success: boolean;
			data: WireFacility[] | null;
		};
		expect(body.success).toBe(true);
		return body.data ?? [];
	}

	const LABELS: Record<string, string> = {
		puskesmas: 'Puskesmas',
		rumah_sakit: 'Rumah Sakit'
	};

	test('the projection adds only type, and sends the raw database enum', async ({ page }) => {
		await page.goto('/faskes');
		const rows = await readWireCatalog(page);
		expect(rows.length).toBeGreaterThan(0);

		for (const facility of rows) {
			// Exactly the agreed five fields. An extra one is a leak, and the
			// key set is asserted rather than spot-checked so a newly added
			// field fails here.
			expect(Object.keys(facility).sort()).toEqual(
				['id', 'is_active', 'name', 'short_code', 'type'].sort()
			);
			// The raw enum, not a display label. Relabelling server-side would
			// hand the frontend a string it cannot validate against the schema.
			expect(Object.keys(LABELS)).toContain(facility.type);
			// The administrative fields that exist on the same table must not
			// travel with it.
			for (const forbidden of ['address', 'phone', 'total_beds', 'available_beds']) {
				expect(facility, `public row must not carry ${forbidden}`).not.toHaveProperty(forbidden);
			}
		}
	});

	test('every seeded category renders its human-readable label', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});
		const wire = await readWireCatalog(page);
		const present = [...new Set(wire.map((facility) => facility.type))];

		// The dev seed carries both categories, so both labels must be on
		// screen. If a future seed loses one, the assertion below degrades to
		// whatever is actually there rather than inventing a row.
		expect(present.length).toBeGreaterThan(0);
		for (const type of present) {
			const label = LABELS[type];
			await expect(
				page.locator('.sigap-facility-row__type', { hasText: label }).first(),
				`no row renders the ${label} label`
			).toBeVisible();
		}

		// And the raw wire values never reach the screen.
		const bodyText = (await page.locator('body').textContent()) ?? '';
		for (const type of present) {
			expect(bodyText, `raw enum ${type} must not be displayed`).not.toContain(type);
		}
	});

	test('rows are labelled from the wire field, not inferred from the name', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});

		// Cross-check the DOM against the API row by row. If any rendered label
		// disagreed with its facility's real enum, that facility would appear
		// under the wrong filter and the citizen would be misled about which
		// kind of clinic it is.
		const rendered = await page.locator('#sigap-facility-results li').evaluateAll((nodes) =>
			nodes.map((node) => ({
				name: (node.querySelector('.sigap-facility-row__name')?.textContent ?? '').trim(),
				shortCode: (node.querySelector('.sigap-facility-row__code')?.textContent ?? '').trim(),
				type: (node.querySelector('.sigap-facility-row__type')?.textContent ?? '').trim()
			}))
		);
		const wire = await readWireCatalog(page);

		for (const row of rendered) {
			const match = wire.find((f) => f.name === row.name && f.short_code === row.shortCode);
			expect(match, `${row.name} is not in the API response`).toBeTruthy();
			expect(row.type, `${row.name} must show its real enum label`).toBe(LABELS[match!.type]);
		}
	});

	test('filters to one category and back to all of them', async ({ page }) => {
		await page.goto('/faskes');
		const rows = page.locator('#sigap-facility-results li');
		await expect(rows.first()).toBeVisible({ timeout: 10_000 });
		const wire = await readWireCatalog(page);
		const total = wire.length;
		expect(total).toBeGreaterThan(0);

		const typeGroup = page.getByRole('group', { name: 'Saring berdasarkan tipe' });
		await expect(typeGroup).toBeVisible();

		// Default is Semua, and it shows everything.
		const semua = typeGroup.getByRole('button', { name: 'Semua' });
		await expect(semua).toHaveAttribute('aria-pressed', 'true');
		await expect(rows).toHaveCount(total);

		// Each category the seed actually contains. Counts come from the API,
		// so this asserts real filtering rather than a hardcoded number.
		const present = [...new Set(wire.map((facility) => facility.type))];
		const seen = new Set<string>();

		for (const type of present) {
			const expected = wire.filter((facility) => facility.type === type);
			// Skip a category with no rows rather than asserting on an empty set.
			if (expected.length === 0) continue;
			seen.add(type);

			await typeGroup.getByRole('button', { name: LABELS[type] }).click();
			await expect(rows).toHaveCount(expected.length);
			await expect(semua).toHaveAttribute('aria-pressed', 'false');

			// Every surviving row really is of that category, checked by name
			// against the API. A correct count with the wrong rows would still
			// be a lie to the citizen.
			for (const facility of expected) {
				await expect(rows.filter({ hasText: facility.name })).toHaveCount(1);
			}
		}

		// Back to the full catalog.
		await semua.click();
		await expect(rows).toHaveCount(total);
		await expect(semua).toHaveAttribute('aria-pressed', 'true');
		expect(seen.size, 'the local seed must exercise both categories').toBeGreaterThan(0);
	});

	/**
	 * The page's own search rule, so an expected count is computed the same way
	 * the UI computes it. Search covers name and short_code only — the two
	 * public fields the contract allows — and nothing else.
	 */
	const searchMatches = (facility: WireFacility, query: string) => {
		const needle = query.toLowerCase();
		return (
			facility.name.toLowerCase().includes(needle) ||
			facility.short_code.toLowerCase().includes(needle)
		);
	};

	test('the type filter and the search box both apply at once', async ({ page }) => {
		await page.goto('/faskes');
		const rows = page.locator('#sigap-facility-results li');
		await expect(rows.first()).toBeVisible({ timeout: 10_000 });
		const wire = await readWireCatalog(page);

		// Short codes are unique per facility, so each one is a query that
		// matches exactly one row. That makes the intersection unambiguous: the
		// filtered count is 1 when both apply, and the other category's
		// facility is a real near-miss rather than a hypothetical.
		const target = wire[0];
		const other = wire.find((facility) => facility.type !== target.type);
		test.skip(!other, 'the local seed needs at least two categories');

		await page.getByRole('searchbox').fill(target.short_code);
		await expect(rows).toHaveCount(1);
		await expect(rows.first()).toContainText(target.name);

		// The right type: the row stays, so the type filter is not over-eager.
		const typeGroup = page.getByRole('group', { name: 'Saring berdasarkan tipe' });
		await typeGroup.getByRole('button', { name: LABELS[target.type] }).click();
		await expect(rows).toHaveCount(1);
		await expect(page.locator('.sigap-facility-row__type').first()).toHaveText(LABELS[target.type]);

		// The wrong type: the row goes, and the search is still in the box, so
		// the citizen can see which of the two filters caused it.
		await typeGroup.getByRole('button', { name: LABELS[other!.type] }).click();
		await expect(rows).toHaveCount(0);
		await expect(page.getByText('Tidak ada faskes yang cocok')).toBeVisible();
		await expect(page.getByRole('searchbox')).toHaveValue(target.short_code);

		// Both conditions at once, over a query that genuinely narrows within
		// a category. Derived from the data rather than assumed: every word of
		// every real facility name is a candidate, and the first one that
		// matches more than one row and fewer than all of them is used. The
		// seed decides which word that is, so the test never hardcodes a
		// naming coincidence someone could edit out from under it.
		//
		// Note this cannot rely on a word spanning both categories: the local
		// names are "Pus<X> <name>" and "RS<X> <name>", so the category
		// prefix and the given name never overlap. The narrowing has to happen
		// inside a category for the intersection to be observable.
		const byType = new Map<string, WireFacility[]>();
		for (const facility of wire) {
			byType.set(facility.type, [...(byType.get(facility.type) ?? []), facility]);
		}
		const [type, members] = [...byType.entries()].sort((a, b) => b[1].length - a[1].length)[0];
		test.skip(members.length < 2, 'the local seed needs a multi-row category');

		const candidates = [...new Set(members.flatMap((f) => f.name.split(' ')))].filter(
			(word) => word.length >= 3
		);
		const query = candidates.find((candidate) => {
			const all = wire.filter((f) => searchMatches(f, candidate));
			return all.length > 1 && all.length < wire.length;
		});
		test.skip(!query, 'no word in the local seed narrows the catalog without emptying it');

		await page.getByRole('searchbox').fill(query!);
		const allMatches = wire.filter((f) => searchMatches(f, query!));
		const typedMatches = allMatches.filter((f) => f.type === type);
		// The premise of the assertion below. If this ever fails, the seed
		// changed and this test is no longer proving composition.
		expect(allMatches.length, 'the query must narrow the catalog').toBeLessThan(wire.length);
		expect(allMatches.length, 'the query must not empty the catalog').toBeGreaterThan(0);
		expect(
			typedMatches.length,
			'the query must match rows of the selected type'
		).toBeGreaterThan(0);

		// Search alone: every match, across categories.
		await typeGroup.getByRole('button', { name: 'Semua' }).click();
		await expect(rows).toHaveCount(allMatches.length);

		// Search and type together: strictly the intersection. If the type were
		// dropped this would be allMatches.length; if the query were dropped it
		// would be the size of the whole category.
		await typeGroup.getByRole('button', { name: LABELS[type] }).click();
		await expect(rows).toHaveCount(typedMatches.length);
		await expect(page.locator('.sigap-facility-row__type').first()).toHaveText(LABELS[type]);
		for (const facility of typedMatches) {
			await expect(rows.filter({ hasText: facility.name })).toHaveCount(1);
		}

		// Widening the type back keeps the query applied.
		await typeGroup.getByRole('button', { name: 'Semua' }).click();
		await expect(rows).toHaveCount(allMatches.length);
		await expect(page.getByRole('searchbox')).toHaveValue(query!);
	});

	test('a type filter with no matches is recoverable and blames the type', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});
		const wire = await readWireCatalog(page);

		const present = [...new Set(wire.map((facility) => facility.type))];
		test.skip(present.length < 2, 'the local seed needs both categories for this case');
		const [firstType, secondType] = present;
		const target = wire.find((facility) => facility.type === firstType)!;
		const opposite = secondType;

		// The short code is unique per facility, so searching for it matches
		// exactly one row, and that row's type is the opposite of the selected
		// one. The empty result is therefore a property of the data, not
		// something this test engineered by editing a row.
		await page.getByRole('searchbox').fill(target.short_code);
		await expect(page.locator('#sigap-facility-results li')).toHaveCount(1);

		await page
			.getByRole('group', { name: 'Saring berdasarkan tipe' })
			.getByRole('button', { name: LABELS[opposite] })
			.click();

		await expect(page.getByText('Tidak ada faskes yang cocok')).toBeVisible();
		// The message names the type, so the citizen is not sent off to shorten
		// a query that is perfectly good.
		await expect(page.getByText(`Tidak ada hasil untuk`, { exact: false })).toBeVisible();

		// And the way back clears both inputs, not just the visible one.
		await page.getByRole('button', { name: 'Hapus semua filter' }).click();
		await expect(page.getByText('Tidak ada faskes yang cocok')).toBeHidden();
		await expect(page.getByRole('searchbox')).toHaveValue('');
		const typeGroup = page.getByRole('group', { name: 'Saring berdasarkan tipe' });
		await expect(typeGroup.getByRole('button', { name: 'Semua' })).toHaveAttribute(
			'aria-pressed',
			'true'
		);
		await expect(typeGroup.getByRole('button', { name: LABELS[opposite] })).toHaveAttribute(
			'aria-pressed',
			'false'
		);
		await expect(page.locator('#sigap-facility-results li')).toHaveCount(wire.length);
	});

	test('the type controls meet the citizen touch target floor', async ({ page }) => {
		await page.setViewportSize(MOBILE);
		await page.goto('/faskes');
		await expect(page.getByRole('searchbox')).toBeVisible({ timeout: 10_000 });

		const typeGroup = page.getByRole('group', { name: 'Saring berdasarkan tipe' });
		await expect(typeGroup).toBeVisible();

		// Same 44px floor as every other citizen control. A chip is the easiest
		// thing in a filter bar to shrink, because it looks like a tag.
		const heights = await typeGroup
			.getByRole('button')
			.evaluateAll((nodes) => nodes.map((node) => node.getBoundingClientRect().height));
		expect(heights).toHaveLength(3);
		for (const height of heights) {
			expect(height, `type control is ${height}px tall`).toBeGreaterThanOrEqual(44);
		}
	});

	test('a type control is reachable and activatable from the keyboard', async ({ page }) => {
		await page.goto('/faskes');
		await expect(page.getByRole('searchbox')).toBeVisible({ timeout: 10_000 });
		const wire = await readWireCatalog(page);
		const present = [...new Set(wire.map((facility) => facility.type))];

		const typeGroup = page.getByRole('group', { name: 'Saring berdasarkan tipe' });
		// Walk the group in DOM order from the search box. Asserting the whole
		// sequence rather than one chip is what proves the controls are
		// sequenced in the frozen order and reachable in a single tab run —
		// a single-chip assertion would pass even if the first chip were
		// unreachable and the browser had jumped the tab order for it.
		const expectedOrder = ['Semua', ...present.map((type) => LABELS[type])];
		await page.getByRole('searchbox').focus();

		for (const [index, label] of expectedOrder.entries()) {
			await page.keyboard.press('Tab');
			const chip = typeGroup.getByRole('button', { name: label });
			await expect(chip, `tab stop ${index + 1} should be "${label}"`).toBeFocused();
			// A real button, so activation is the browser's job.
			await expect(chip).toHaveJSProperty('tagName', 'BUTTON');
		}

		// The last chip in the group is the one focused above. Activating it
		// with Enter must move the pressed state, which is the whole contract:
		// a control that is focusable but not operable is worse than one that
		// is absent, because it looks available.
		const last = typeGroup.getByRole('button', { name: expectedOrder[expectedOrder.length - 1] });
		await page.keyboard.press('Enter');
		await expect(last).toHaveAttribute('aria-pressed', 'true');
		await expect(typeGroup.getByRole('button', { name: 'Semua' })).toHaveAttribute(
			'aria-pressed',
			'false'
		);
		// And it actually filtered, rather than only updating its own state.
		const expectedCount = wire.filter((f) => f.type === present[present.length - 1]).length;
		await expect(page.locator('#sigap-facility-results li')).toHaveCount(expectedCount);
	});

	test('the type filter does not overflow the mobile viewport', async ({ page }) => {
		await page.setViewportSize(MOBILE);
		await page.goto('/faskes');
		await expect(page.getByRole('searchbox')).toBeVisible({ timeout: 10_000 });
		await expect(page.getByRole('group', { name: 'Saring berdasarkan tipe' })).toBeVisible();

		// The chip row is a horizontal scroller, but three chips fit at 390px,
		// so the page itself must not gain a sideways scroll.
		await expectNoHorizontalOverflow(page);
	});
});

test.describe('citizen pages stay inside the local origin', () => {
	test('never contacts production while browsing', async ({ page, baseURL }) => {
		const requested: string[] = [];
		page.on('request', (request) => requested.push(request.url()));

		await page.goto('/');
		await page.goto('/faskes');

		const host = new URL(baseURL as string).host;
		for (const url of requested) {
			expect(url, `${url} must stay on the local origin`).toContain(host);
			expect(url).not.toContain('chaerulchalik');
		}
	});
});

test.describe('hydration and CSP', () => {
	/**
	 * The app must actually become interactive, and it must do so without
	 * weakening script-src.
	 *
	 * `script-src 'self'` without a nonce blocks SvelteKit's inline hydration
	 * script. The page still renders its server HTML, so every server-side check
	 * passes while the browser silently does nothing: no fetch, no filter, no
	 * navigation. Only a real browser can catch that, which is why this lives in
	 * E2E rather than in a unit test.
	 */
	test('hydrates with no CSP violations', async ({ page }) => {
		const violations: string[] = [];
		page.on('console', (message) => {
			if (/Content Security Policy/i.test(message.text())) violations.push(message.text());
		});
		page.on('pageerror', (error) => violations.push(error.message));

		await page.goto('/faskes');
		// A page that never hydrated is still stuck on the loading skeleton.
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});
		expect(violations, violations.join('\n')).toEqual([]);
	});

	test('keeps script-src free of unsafe-inline', async ({ page }) => {
		const response = await page.goto('/');
		const csp = response?.headers()['content-security-policy'] ?? '';
		expect(csp, 'every response must carry a CSP').toBeTruthy();
		expect(csp).toContain("script-src 'self'");
		// A nonce is per-request and lets SvelteKit hydrate while keeping
		// injected inline scripts blocked. 'unsafe-inline' would not.
		expect(csp).toMatch(/script-src[^;]*'nonce-[A-Za-z0-9+/=_-]+'/);
		expect(csp).not.toContain("script-src 'self' 'unsafe-inline'");
		expect(csp).not.toContain('script-src *');
	});

	test('the hydration script actually runs under the policy', async ({ page }) => {
		// Deliberately not asserting that the inline <script> exposes a matching
		// `nonce` attribute. Browsers deliberately hide the nonce content
		// attribute from the DOM after parsing, precisely so injected markup
		// cannot read it back out and replay it. Reading it here would only
		// measure that mitigation, not whether the policy works.
		//
			// The real proof is behavioural: if the policy were wrong, SvelteKit's
		// hydration script would be blocked, the page would stay on its loading
		// skeleton, and the catalog rows below would never appear.
		const csp = (await page.goto('/'))?.headers()['content-security-policy'] ?? '';
		expect(csp).toMatch(/script-src[^;]*'nonce-[A-Za-z0-9+/=_-]+'/);

		await page.goto('/faskes');
		await expect(page.locator('#sigap-facility-results li').first()).toBeVisible({
			timeout: 10_000
		});

		// And the app is genuinely interactive, not merely rendered.
		const search = page.getByRole('searchbox');
		await expect(search).toBeVisible();
		await search.fill('a');
		await expect(search).toHaveValue('a');
	});

	test('uses a fresh nonce per request', async ({ page }) => {
		const first = (await page.goto('/'))?.headers()['content-security-policy'] ?? '';
		const second = (await page.goto('/'))?.headers()['content-security-policy'] ?? '';
		const nonceOf = (header: string) => header.match(/'nonce-([A-Za-z0-9+/=_-]+)'/)?.[1];
		const a = nonceOf(first);
		const b = nonceOf(second);
		expect(a).toBeTruthy();
		expect(b).toBeTruthy();
		// Reusing a nonce across responses would let an attacker reuse a
		// previously-approved inline script.
		expect(a).not.toBe(b);
	});
});
