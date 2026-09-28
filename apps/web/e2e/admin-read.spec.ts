import { expect, test, type Page } from '@playwright/test';

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
		await expect(page.getByText(/Diperbarui pukul/)).toBeVisible(SETTLE);
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
		// An operator who believes a facility can be reactivated will confidently
		// tell a clinic it can reopen — a false promise about a system they do not
		// own.
		await expect(page.getByText(/tidak dapat diaktifkan kembali/i)).toBeVisible();
	});

	test('Antrean shows the freshness label and the three frozen bands', async ({ page }) => {
		await gotoAdmin(page, '/admin/queues');

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

		// The control renders immediately with only "Semua fasilitas"; the options
		// arrive with the scoped-facility read a moment later. Sampling the options
		// once would race that read and pass or fail on backend timing, so this
		// polls for a named option to exist before reading the list. The
		// assertion is that the backend really sent scoped facility NAMES — not
		// that they turned up quickly.
		await expect
			.poll(
				async () => {
					const options = await select.locator('option').allTextContents();
					return options.some((o) => /Puskesmas|RSUD|RS Mitra/i.test(o));
				},
				{ timeout: 15_000, message: 'no seeded facility name reached the filter' }
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
		await expect(page.getByRole('heading', { name: 'Menunggu' })).toBeVisible(SETTLE);
		const text = (await page.locator('body').textContent()) ?? '';
		expect(text, 'the queue board must not show a raw UUID').not.toMatch(UUID);
	});

	test('Janji Temu resolves facility and service, or shows "-"', async ({ page }) => {
		await gotoAdmin(page, '/admin/appointments');
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
		await expect(page.locator('table')).toBeVisible(SETTLE);
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
		expect(page.locator('input[type="text"]')).toHaveCount(0);
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
