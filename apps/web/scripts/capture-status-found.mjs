/**
 * Captures the /patient/status FOUND state.
 *
 * The approved Pen frames draw this page in its found state — the queue-number
 * pass is the whole point of the screen — but the generic capture script only
 * ever sees the idle form, because the found state requires a real check-in code
 * that resolves against the database. This intercepts the one status request and
 * returns the frame's own payload shape, so the pass can be compared against the
 * frame without depending on seeded data.
 *
 * LOCAL REVIEW TOOLING. It asserts nothing, is not part of the suite, and
 * intercepts a request ONLY in this throwaway browser context — the app and the
 * API are untouched.
 *
 * Usage: node scripts/capture-status-found.mjs [baseURL]
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots');
mkdirSync(OUT, { recursive: true });

/** Mirrors the Pen's own status frame: RSK-0042, mid-queue. */
const FOUND = {
	success: true,
	data: {
		found_by: 'checkin_code',
		facility_name: 'RSUD Kota Sehat',
		appointment_status: 'queued',
		appointment_time: '2026-10-20T02:30:00Z',
		checkin_status: 'in_queue',
		queue_number: 42,
		queue_status: 'waiting',
		queue_formatted_number: 'RSK-0042'
	}
};

const VIEWPORTS = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const browser = await chromium.launch();

for (const vp of VIEWPORTS) {
	const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await context.newPage();

	await page.route('**/api/v1/patient/status*', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json; charset=utf-8',
			body: JSON.stringify(FOUND)
		})
	);

	await page.goto(BASE + '/patient/status', { waitUntil: 'networkidle', timeout: 30000 });
	await page.locator('#sigap-status-code').fill('RSK0042');
	await page.getByRole('button', { name: /Cek Status/ }).click();
	await page.waitForSelector('[data-testid="visit-progress"]', { timeout: 10000 });

	// Measure the pass so the type size is a number rather than an impression.
	const m = await page.evaluate(() => {
		const num = document.querySelector('.sigap-visit__pass-number');
		const eyebrow = document.querySelector('.sigap-visit__pass-eyebrow');
		if (!num) return { note: 'NO PASS ELEMENT' };
		const cs = getComputedStyle(num);
		return {
			number: num.textContent.trim(),
			fontSize: cs.fontSize,
			fontFamily: cs.fontFamily.split(',')[0],
			fontWeight: cs.fontWeight,
			colour: cs.color,
			eyebrow: eyebrow?.textContent.trim() ?? null,
			eyebrowSize: eyebrow ? getComputedStyle(eyebrow).fontSize : null,
			docOverflow:
				document.documentElement.scrollWidth - document.documentElement.clientWidth
		};
	});

	const file = join(OUT, `status-found-${vp.name}.png`);
	await page.screenshot({ path: file, fullPage: true });
	console.log(`${vp.name}  ${JSON.stringify(m)}`);
	console.log(`       -> ${file}`);

	await context.close();
}

await browser.close();
