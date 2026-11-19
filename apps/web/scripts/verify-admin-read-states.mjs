/**
 * Verifies the admin read states with a CONTROLLED backend outcome.
 *
 * The live stack's database is currently unreachable, which makes the admin
 * pages hang rather than fail — so this intercepts the admin reads and returns a
 * chosen status, making each state deterministic and independent of the
 * database. It covers the contract the freshness change depends on: the
 * "Diperbarui" label is stamped ONLY after a successful read, so a failure must
 * render the error surface and NO freshness claim.
 *
 * Not a test. Usage: node scripts/verify-admin-read-states.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const ROUTES = ['/admin', '/admin/queues', '/admin/schedules', '/admin/facilities', '/admin/notifications'];

const browser = await chromium.launch();

async function probe(route, fulfill) {
	const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
	const page = await context.newPage();

	await page.route('**/api/v1/admin/**', (r) =>
		r.fulfill({
			status: fulfill.status,
			contentType: 'application/json; charset=utf-8',
			body: JSON.stringify(fulfill.body)
		})
	);

	await page.goto(BASE + route, { waitUntil: 'domcontentloaded', timeout: 30000 }).catch(() => {});
	await page.waitForTimeout(2500);

	const r = await page.evaluate(() => ({
		freshness: document.querySelector('.sigap-admin-freshness')?.textContent?.trim() ?? '',
		hasRefresh: Boolean(
			Array.from(document.querySelectorAll('.sigap-admin-header button')).find((b) =>
				/perbarui/i.test(b.textContent ?? '')
			)
		),
		alertText: (document.querySelector('[role="alert"]')?.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 80),
		loading: Boolean(document.querySelector('[aria-busy="true"]'))
	}));

	await context.close();
	return r;
}

// A 500 must fail closed: an error surface, and NO freshness claim.
console.log('--- 500 from every admin read: expect an alert and NO freshness ---');
for (const route of ROUTES) {
	const r = await probe(route, { status: 500, body: { success: false, error: 'Gagal memuat data.' } });
	const ok = r.alertText.length > 0 && r.freshness === '';
	console.log(
		`${ok ? 'ok  ' : 'FAIL'} ${route.padEnd(24)} freshness=${JSON.stringify(r.freshness)} ` +
			`alert=${JSON.stringify(r.alertText)} loading=${r.loading}`
	);
}

// A 200 with an empty list must ALSO stamp freshness: the read succeeded, it
// simply found nothing, and "no rows" is not "no data freshness".
console.log('\n--- 200 with empty lists: expect freshness AND the empty state ---');
for (const route of ROUTES) {
	const r = await probe(route, { status: 200, body: { success: true, data: [] } });
	console.log(
		`${r.freshness ? 'ok  ' : 'FAIL'} ${route.padEnd(24)} freshness=${JSON.stringify(r.freshness)} ` +
			`alert=${JSON.stringify(r.alertText)} loading=${r.loading}`
	);
}

await browser.close();
