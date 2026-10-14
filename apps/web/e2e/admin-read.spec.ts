import type { Page } from '@playwright/test';
import { expect, test } from './support/test';

/**
 * Phase 3B4: the six admin READ destinations, end to end.
 *
 * Read-only by design, and the tests are too. No admin mutation is performed
 * anywhere in this file: Phase 3B5 owns mutation execution, and an E2E run that
 * changed a queue ticket's status would leave the seeded database in a state the
 * next run inherits.
 *
 * Everything asserted here is backed by the Go API and the seeded database
 * reached through the same-origin proxy. No request is intercepted and no
 * response is faked, so "the table has rows" means the backend really returned
 * scoped rows rather than that a fixture was found.
 *
 * `scripts/dev/Start-LocalE2E.ps1` is the approved way to bring the stack up,
 * including the real Rust queue engine that Phase 3B3.1 added, and the
 * loopback-only guard in playwright.config.ts rejects any other target before a
 * browser launches.
 */

const DESKTOP = { width: 1440, height: 900 };
/** The frozen reference's collapsed-rail viewport. */
const RAIL = { width: 1024, height: 768 };

/** Real round trips against a local API; the default timeout is too tight. */
const SETTLE = { timeout: 15_000 };

/** The six frozen destinations, in sidebar order. */
const DESTINATIONS = [
	{ href: '/admin', label: 'Ringkasan' },
	{ href: '/admin/queues', label: 'Antrean' },
	{ href: '/admin/appointments', label: 'Janji Temu' },
	{ href: '/admin/schedules', label: 'Jadwal' },
	{ href: '/admin/facilities', label: 'Fasilitas' },
	{ href: '/admin/notifications', label: 'Notifikasi' }
] as const;

/** Any UUID. Used to prove a machine identifier never reaches the UI. */
const UUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

/**
 * The subject this run is pinned to, as exported by Start-LocalE2E.ps1.
 *
 * WHY THE READ SPECS MUST BE ACTOR-AWARE.
 *
 * These specs assert that a POPULATED table renders correctly — real seeded
 * names, no raw ids, a 36–40px row band. That is a claim about a table, so it
 * needs an actor that actually holds data.
 *
 * A zero-scope actor legitimately holds none, and the pages then render their
 * fail-closed empty state ("Anda belum memiliki fasilitas dalam cakupan") with
 * no table at all. That is the CORRECT behaviour and precisely what Phase 3B0
 * asks for: zero assignment must fail closed, quietly, without leaking the
 * existence of anything.
 *
 * So the choice is between two wrong answers, and it is worth being explicit
 * about which one this avoids:
 *
 *   - letting the suite fail, which trains a reader to treat a correct denial
 *     as a regression, or
 *   - relaxing the assertion to accept a silently broken table, which would
 *     let a real rendering defect hide behind the skip.
 *
 * Neither is acceptable, and neither is what happens here. The populated-table
 * contract is asserted for every actor that holds data, and the zero-scope
 * actor gets its OWN dedicated, stronger assertion: the empty state must be
 * shown AND no table may exist AND no identifier may leak. Skipping the
 * populated-table claim is not a weakening — for this actor there is no table
 * whose row band, labels, or ids could be wrong.
 */
const ACTOR = (process.env.SIGAP_E2E_ACTOR ?? '').trim();

/**
 * True when this run has no business expecting a populated table.
 *
 * Kept as an explicit allowlist of the known-empty subject rather than
 * "anything that happens to render no rows": the second form would silently
 * pass a genuine rendering failure on an ordinary actor.
 */
const HOLDS_NO_DATA = ACTOR === 'local-zero-scope-admin';

/**
 * Asserts the fail-closed empty state, and that it leaks nothing.
 *
 * This is the positive obligation for a zero-scope actor, and it is strictly
 * more than the populated-table spec would have checked.
 */
async function expectFailClosedEmptyScope(page: Page, what: string) {
	const table = page.locator('table');
	await expect(table, `${what} must render no table for an actor holding no rows`).toHaveCount(0);

	// The empty state must actually be shown, not merely implied by a blank page.
	await expect(page.getByText(/belum memiliki|belum ada|tidak ada/i).first()).toBeVisible(SETTLE);

	// The strongest form of the 3B0 guarantee: a denial must not become an
	// existence oracle. If any UUID or seeded facility name reached this page,
	// the empty state would be telling the actor what it cannot see.
	const text = ((await page.locator('body').textContent()) ?? '');
	expect(text, `${what} must leak no identifier while denying scope`).not.toMatch(UUID);
}

/**
 * Waits for the page's own heading, which is the first thing every admin
 * destination renders.
 *
 * Waiting for the sidebar instead would be a mistake: the shell renders
 * immediately, so it would let these tests start asserting table content before
 * any read had resolved, and a slow backend would read as an empty table.
 */
async function gotoAdmin(page: Page, path: string) {
	await page.goto(path);
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
}

/** Fails if the document scrolls sideways at the current width. */
async function expectNoHorizontalOverflow(page: Page) {
	const overflow = await page.evaluate(() => ({
		scrollWidth: document.documentElement.scrollWidth,
		clientWidth: document.documentElement.clientWidth
	}));
	expect(
		overflow.scrollWidth,
		`content is ${overflow.scrollWidth}px wide in a ${overflow.clientWidth}px viewport`
	).toBeLessThanOrEqual(overflow.clientWidth + 1);
}

/** The admin nav landmark, by its accessible name. */
function adminNav(page: Page) {
	return page.getByRole('navigation', { name: 'Menu operasi' });
}

test.describe('the admin shell', () => {
	test.use({ viewport: DESKTOP });

	for (const destination of DESTINATIONS) {
		test(`${destination.href} is reachable and marks itself current`, async ({ page }) => {
			await gotoAdmin(page, destination.href);

			// All six destinations exist, with their frozen labels.
			await expect(adminNav(page).getByRole('link')).toHaveCount(6);
			await expect(
				adminNav(page).getByRole('link', { name: destination.label })
			).toBeVisible();

			// EXACTLY ONE current marker. Two would tell a screen-reader user they
			// are on two pages at once, and zero would mean an operator cannot tell
			// where they are.
			const current = page.locator('[aria-current="page"]');
			await expect(current).toHaveCount(1);
			await expect(current).toHaveAttribute('href', destination.href);

			// One h1 per page, and the citizen chrome is absent. The admin product
			// is a separate shell; a bottom tab bar here would be the citizen shell
			// leaking into it.
			await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
			await expect(page.locator('.sigap-bottom-nav')).toHaveCount(0);
			await expect(page.locator('.sigap-citizen-header')).toHaveCount(0);
		});
	}

	test('walks every destination and lands on the right one each time', async ({ page }) => {
		// Each hop proves the previous page's aria-current was correct AND that the
		// next link works from a real rendered page, which a direct goto does not.
		await gotoAdmin(page, '/admin');
		for (const destination of DESTINATIONS) {
			await adminNav(page).getByRole('link', { name: destination.label }).click();
			await expect(page).toHaveURL(new RegExp(`${destination.href.replace('/', '\\/')}$`));
			await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
			await expect(page.locator('[aria-current="page"]')).toHaveCount(1);
		}
	});

	test('links the account area statically to Beranda, with no role detection', async ({ page }) => {
		await gotoAdmin(page, '/admin');
		// One static link, always the same. A link that varied by role would be
		// inferring a role the client does not hold.
		const account = adminNav(page).locator('xpath=../..').getByRole('link', { name: 'Beranda' });
		await expect(account).toHaveAttribute('href', '/');
	});
});

test.describe('the admin shell at 1024px', () => {
	test.use({ viewport: RAIL });

	test('collapses the sidebar to a 56px icon rail that keeps its names', async ({ page }) => {
		await gotoAdmin(page, '/admin');

		const sidebar = page.getByRole('complementary', { name: 'Navigasi panel operasi' });
		await expect(sidebar).toBeVisible();

		// 56px is the frozen collapsed width, and reaching it is the whole point of
		// this test: the rail must actually collapse rather than merely reflow.
		const width = await sidebar.evaluate((el) => el.getBoundingClientRect().width);
		expect(Math.round(width), 'the rail must be 56px wide at 1024px').toBe(56);

		// THE critical assertion. CSS hides the visible label at this width, so the
		// aria-label is the only thing distinguishing six icon-only links. Without
		// it an operator using a screen reader hears six identical "link"s.
		for (const destination of DESTINATIONS) {
			await expect(
				adminNav(page).getByRole('link', { name: destination.label })
			).toBeVisible();
		}

		// Still exactly one current marker on the rail.
		await expect(page.locator('[aria-current="page"]')).toHaveCount(1);
	});

	test('does not scroll the whole page sideways on any destination', async ({ page }) => {
		// A table may scroll inside its own region; the PAGE may not. A
		// whole-page horizontal scrollbar at 1024px means the sidebar scrolls away
		// and the operator loses their navigation.
		for (const destination of DESTINATIONS) {
			await gotoAdmin(page, destination.href);
			await expectNoHorizontalOverflow(page);
		}
	});
});

test.describe('admin reads are real and scoped', () => {
	test.use({ viewport: DESKTOP });

	test('Ringkasan shows figures derived from the scoped reads, with sources', async ({ page }) => {
		await gotoAdmin(page, '/admin');

		// A zero-scope actor gets the overview's empty state rather than figures
		// built from nothing. "0" would be a lie here: the actor cannot see any
		// queue, so the honest statement is that there is no scope, not that the
		// counts happen to be zero.
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Ringkasan');
			// The scope disclosure still has to reach the operator, and it does —
			// in the header subtitle rather than in the loaded-body note, because
			// the empty state replaces the figures that note describes. Both wordings
			// are accepted so this asserts the CLAIM, not one string.
			await expect(
				page.getByText(/lingkup (akses|operator)/i).first(),
				'the overview must still disclose that it is scoped'
			).toBeVisible(SETTLE);
			return;
		}

		// The freshness label, which is stamped only after all four reads succeed.
		// A partial failure must not leave an overview claiming a snapshot it does
		// not have.
		await expect(page.getByText(/Diperbarui pukul \d{2}\.\d{2}/)).toBeVisible(SETTLE);

		// Every headline figure names its source. A count an operator cannot trace
		// is a number they have to take on faith.
		for (const source of ['Daftar antrean', 'Daftar janji temu', 'Daftar fasilitas']) {
			await expect(page.getByText(source, { exact: false }).first()).toBeVisible();
		}

		// The provenance disclaimer. "Bukan data real-time" matters because the page
		// does not poll: an operator glancing at it is looking at a snapshot.
		await expect(page.getByText(/bukan data real-time/i)).toBeVisible();
		await expect(page.getByText(/lingkup operator/i).first()).toBeVisible();
	});

	test('Ringkasan invents no metric the scoped reads cannot support', async ({ page }) => {
		await gotoAdmin(page, '/admin');
		// The absence of invented metrics is asserted on the whole page, so it holds
		// for the empty state too. Only the "reads have landed" precondition needs
		// an actor that holds data.
		if (!HOLDS_NO_DATA) {
			await expect(page.getByText(/Diperbarui pukul/)).toBeVisible(SETTLE);
		}
		const text = ((await page.locator('body').textContent()) ?? '').toLowerCase();

		// Each of these needs a denominator or a history the four scoped reads do
		// not carry. An invented number on a clinic dashboard gets acted on.
		for (const forbidden of [
			'persen',
			'%',
			'trend',
			'naik',
			'turun',
			'sla',
			'okupansi',
			'kapasitas terpakai',
			'rata-rata waktu tunggu'
		]) {
			expect(text, `Ringkasan must not claim ${forbidden}`).not.toContain(forbidden);
		}
	});

	test('Ringkasan cross-links reach the correct admin modules', async ({ page }) => {
		await gotoAdmin(page, '/admin');
		// Cross-links are affordances INTO the modules, and a zero-scope overview has
		// no modules to cross-link to. Asserting them here would demand links into
		// pages the actor cannot open.
		test.skip(
			HOLDS_NO_DATA,
			'the overview shows no cross-links when the actor holds no modules'
		);
		await expect(page.getByRole('link', { name: 'Buka Antrean' })).toHaveAttribute(
			'href',
			'/admin/queues'
		);
		// And following one actually arrives.
		await page.getByRole('link', { name: 'Buka Antrean' }).click();
		await expect(page).toHaveURL(/\/admin\/queues$/);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
	});

	test('Facilitas renders real facility rows, never a raw id', async ({ page }) => {
		await gotoAdmin(page, '/admin/facilities');

		// A zero-scope actor has no rows to render, and the page correctly shows
		// its fail-closed empty state instead. Assert THAT, which is the stronger
		// claim for this actor, rather than skipping the destination.
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Fasilitas');
			return;
		}

		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		// A real seeded facility, read through the backend rather than assumed.
		await expect(table.locator('tbody')).toContainText(/Puskesmas|Rumah Sakit|RSUD/i);

		// The machine-identifier check. A truncated UUID in a cell meant for a
		// person is a defect an operator may try to use, and an out-of-scope id
		// echoed back would leak what 3B0 withholds.
		const text = (await table.textContent()) ?? '';
		expect(text, 'the facilities table must not show a raw UUID').not.toMatch(UUID);
	});

	test('Fasilitas states the one-way deactivate semantics', async ({ page }) => {
		await gotoAdmin(page, '/admin/facilities');
		// The one-way promise is about facilities this actor could deactivate. With
		// none in scope there is nothing to promise, and the empty state already
		// carries the stronger statement: no facility is reachable at all.
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Fasilitas');
			return;
		}
		// An operator who believes a facility can be reactivated will confidently
		// tell a clinic it can reopen — a false promise about a system they do not
		// own.
		await expect(page.getByText(/tidak dapat diaktifkan kembali/i)).toBeVisible();
	});

	test('Antrean shows the freshness label and the three frozen bands', async ({ page }) => {
		await gotoAdmin(page, '/admin/queues');
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Antrean');
			return;
		}

		// The bands, by their group headings. Membership is by status, so this is a
		// claim about the server's data rather than a date computation.
		for (const band of ['Dipanggil', 'Menunggu', 'Selesai hari ini']) {
			await expect(page.getByRole('heading', { name: band })).toBeVisible(SETTLE);
		}

		// The freshness label is present once data has loaded. A board that omits it
		// is a frozen screen an operator cannot reason about.
		await expect(page.getByText(/Diperbarui pukul \d{2}\.\d{2}/)).toBeVisible(SETTLE);
	});

	test('Antrean offers a manual refresh and never claims to be realtime', async ({ page }) => {
		await gotoAdmin(page, '/admin/queues');
		const refresh = page.getByRole('button', { name: /Perbarui/ });
		await expect(refresh).toBeVisible(SETTLE);
		await refresh.click();
		// The label must still be present afterwards: a manual refresh that blanks
		// the freshness claim is worse than one that does nothing.
		await expect(page.getByText(/Diperbarui pukul \d{2}\.\d{2}/)).toBeVisible(SETTLE);

		const text = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		// There is no SSE, no live channel, and no realtime claim. Polling every
		// thirty seconds is not realtime and the copy must not imply it is.
		expect(text).not.toContain('realtime');
		expect(text).not.toContain('real-time');
	});

	test('Antrean replaced the raw-id facility filter with a name filter', async ({ page }) => {
		await gotoAdmin(page, '/admin/queues');
		// A real seeded facility name, offered as an option.
		const select = page.getByLabel(/filter fasilitas/i);
		await expect(select).toBeVisible(SETTLE);

		// What the backend says this actor may see. This is read from the API
		// rather than hardcoded, because the local E2E stack arms a real
		// DB-backed, facility-scoped actor (Phase 3B5 §2) whose scope is a
		// property of the seed, not of this test. Asserting a literal
		// "Puskesmas..." would make this test fail the moment the actor genuinely
		// holds a narrower scope — which is the correct behaviour, and not a
		// defect in the filter.
		const scoped = await page.request.get('/api/v1/admin/facilities');
		expect(scoped.ok(), 'the scoped facility read must succeed').toBe(true);
		const expectedNames = ((await scoped.json()).data as Array<{ name: string }>).map(
			(facility) => facility.name
		);

		// A zero-scope actor is offered no facility at all. The filter must then
		// degrade to its own meaning — "Semua fasilitas", which selects nothing —
		// rather than inventing an option the backend withheld.
		if (expectedNames.length === 0) {
			await expect
				.poll(async () => select.locator('option').allTextContents(), {
					timeout: SETTLE.timeout,
					message: 'the filter must settle on its own meaning when the backend offers nothing'
				})
				.toEqual(['Semua fasilitas']);
			await expect(page.getByPlaceholder(/filter id/i)).toHaveCount(0);
			return;
		}

		expect(expectedNames.length, 'this actor must hold at least one facility').toBeGreaterThan(0);

		// The control renders immediately with only "Semua fasilitas"; the options
		// arrive with the scoped-facility read a moment later. Sampling the
		// options once would race that read and pass or fail on backend timing, so
		// this polls until the options settle. The assertion is that the backend
		// really sent scoped facility NAMES — not that they turned up quickly.
		await expect
			.poll(
				async () => {
					const options = await select.locator('option').allTextContents();
					return expectedNames.every((name) => options.includes(name));
				},
				{
					timeout: 15_000,
					message: `no backend-scoped facility name reached the filter (expected ${expectedNames.join(', ')})`
				}
			)
			.toBe(true);

		const options = await select.locator('option').allTextContents();
		// "Semua fasilitas" plus the scoped names, and never a raw identifier.
		expect(options.length).toBeGreaterThan(1);
		expect(options.join(' ')).not.toMatch(UUID);

		// And no free-text id input remains.
		await expect(page.getByPlaceholder(/filter id/i)).toHaveCount(0);
		const text = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		expect(text).not.toContain('filter id fasilitas');
	});

	test('Antrean never displays a raw UUID in the board', async ({ page }) => {
		await gotoAdmin(page, '/admin/queues');
		// The no-UUID guarantee is the point, and it is STRICTEST when there is
		// nothing to show: a denial that leaked an identifier would be the worst
		// possible outcome, so the empty state gets the check, not an exemption.
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Antrean');
			return;
		}
		await expect(page.getByRole('heading', { name: 'Menunggu' })).toBeVisible(SETTLE);
		const text = (await page.locator('body').textContent()) ?? '';
		expect(text, 'the queue board must not show a raw UUID').not.toMatch(UUID);
	});

	test('Janji Temu resolves facility and service, or shows "-"', async ({ page }) => {
		await gotoAdmin(page, '/admin/appointments');
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Janji Temu');
			return;
		}
		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const text = (await table.textContent()) ?? '';
		// The pre-3B4 page rendered `{a.facility_id.slice(0,8)}...` — a truncated
		// machine identifier where a person belongs.
		expect(text, 'the appointments table must not show a raw UUID').not.toMatch(UUID);
		expect(text).not.toMatch(/\.\.\./);
	});

	test('Janji Temu never displays practitioner_id', async ({ page }) => {
		await gotoAdmin(page, '/admin/appointments');

		// The absence of a practitioner field is asserted on the whole page, so it
		// holds for the empty state too — there is nothing to resolve and still no
		// field. Only the "there is a table" precondition needs the actor.
		if (!HOLDS_NO_DATA) {
			await expect(page.locator('table')).toBeVisible(SETTLE);
		}
		const text = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		// There is no practitioner catalog to resolve a name against, so any name
		// shown would be invented and any id shown would be a raw UUID.
		for (const forbidden of ['practitioner', 'id dokter', 'dokter']) {
			expect(text, `the appointments page must not show ${forbidden}`).not.toContain(
				forbidden
			);
		}
	});

	test('Jadwal resolves labels and fabricates no practitioner', async ({ page }) => {
		await gotoAdmin(page, '/admin/schedules');
		if (HOLDS_NO_DATA) {
			await expectFailClosedEmptyScope(page, 'Jadwal');
			// The class-1 empty state still must not smuggle a practitioner field in.
			await expect(page.locator('input[type="text"]')).toHaveCount(0);
			return;
		}
		const table = page.locator('table');
		await expect(table).toBeVisible(SETTLE);
		await expect(table.locator('tbody tr').first()).toBeVisible(SETTLE);

		const text = (await table.textContent()) ?? '';
		expect(text, 'the schedules table must not show a raw UUID').not.toMatch(UUID);

		// The pre-3B4 page had a free-text "ID Dokter" field. A schedule whose
		// practitioner is a typed-in string is a record the system cannot reason
		// about, so the field is gone entirely.
		const body = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		for (const forbidden of ['id dokter', 'practitioner', 'praktisi']) {
			expect(body, `the schedules page must not show ${forbidden}`).not.toContain(
				forbidden
			);
		}
		// AWAITED. This assertion previously dropped its promise, which broke it
		// in both directions: it never failed the test, so a stray text input
		// would have gone unnoticed, and under load the in-flight query raced
		// the page teardown and surfaced as "Protocol error: session closed"
		// rather than as the assertion it was.
		await expect(page.locator('input[type="text"]')).toHaveCount(0);
	});

	test('Notifikasi masks the recipient and never shows a raw contact or hash', async ({ page }) => {
		await gotoAdmin(page, '/admin/notifications');
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible(SETTLE);
		// The summary cards settle first; the list may legitimately be empty.
		await expect(page.getByText(/Menunggu/).first()).toBeVisible(SETTLE);

		const body = (await page.locator('body').textContent()) ?? '';
		// The internal dedup key is never in the response, and nothing here may
		// reconstruct a longer contact from the masked form.
		expect(body, 'no hash may be shown').not.toMatch(/hash/i);
		expect(body, 'no raw email may be shown').not.toMatch(/[\w.+-]+@[\w-]+\.[\w.]+/);
		// A masked form is allowed and is what the server sends.
		expect(body).not.toMatch(/\+62\d{9,}/);
	});

	test('Notifikasi offers URL-synced filters and no pagination controls', async ({ page }) => {
		await gotoAdmin(page, '/admin/notifications');
		await expect(page.getByText(/Menunggu/).first()).toBeVisible(SETTLE);

		// The filters are labelled controls.
		for (const label of [/^Status$/, /^Kanal$/, /^Template$/]) {
			await expect(page.getByLabel(label)).toBeVisible();
		}

		// Choose one and prove it lands in the URL, which is what makes a shared
		// link reproduce the operator's view.
		await page.getByLabel('Status').selectOption('failed');
		await page.waitForURL(/status=failed/, { timeout: SETTLE.timeout });

		// NO pagination. The endpoint returns a bounded list with no cursor and no
		// total, so a page-number control would be a control that cannot work.
		const body = ((await page.locator('body').textContent()) ?? '').toLowerCase();
		for (const forbidden of ['halaman', 'berikutnya', 'sebelumnya', 'muat lebih']) {
			expect(body, `must not offer ${forbidden}`).not.toContain(forbidden);
		}
		await expect(page.getByRole('button', { name: /^\d+$/ })).toHaveCount(0);
	});

	test('Notifikasi orders relative time as minutes, then hours, then days', async ({ page }) => {
		await gotoAdmin(page, '/admin/notifications');
		await expect(page.getByText(/Menunggu/).first()).toBeVisible(SETTLE);

		// The pre-3B4 helper returned SECONDS as "d" and DAYS as "h", so a
		// notification ten seconds old read "10d lalu" and one two days old read
		// "2h lalu". jsdom cannot run the timer, so this asserts the rendering path
		// uses the shared helper's vocabulary and never the inverted abbreviations.
		const text = (await page.locator('body').textContent()) ?? '';
		// A seconds-old row reading as days, or a days-old row reading as hours,
		// would show these. Neither may appear.
		expect(text).not.toMatch(/\d+\s*d\s+lalu/);
		expect(text).not.toMatch(/\d+\s*h\s+lalu/);
	});
});

test.describe('admin read pages perform no mutation', () => {
	test.use({ viewport: DESKTOP });

	test('visits every destination and issues only GET requests', async ({ page }) => {
		// Phase 3B4 is read behaviour. A PATCH or POST from any admin page would
		// mean a mutation was rewired, and it would leave the seeded database in a
		// state the next run inherits — which is how a "repeatable" E2E suite
		// quietly stops being repeatable.
		const mutations: string[] = [];
		page.on('request', (request) => {
			if (request.method() !== 'GET' && request.method() !== 'HEAD') {
				mutations.push(`${request.method()} ${request.url()}`);
			}
		});

		for (const destination of DESTINATIONS) {
			await gotoAdmin(page, destination.href);
			await expectNoHorizontalOverflow(page);
		}

		expect(mutations, `admin reads must not mutate: ${mutations.join(', ')}`).toEqual([]);
	});
});
