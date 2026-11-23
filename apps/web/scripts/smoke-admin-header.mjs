/**
 * Smoke check: does each admin header actually render its freshness cluster?
 *
 * The freshness label is stamped by four separate page components, so "the
 * header has a label" is a per-page fact that a shared component cannot
 * guarantee on its own. This reads the rendered header on every admin route and
 * asserts the label, the control, and the subtitle are all present, then
 * exercises the control and confirms the label's time advances.
 *
 * Not part of the suite. Usage: node scripts/smoke-admin-header.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';

const ROUTES = [
	['/admin', 'Ringkasan'],
	['/admin/queues', 'Antrean'],
	['/admin/appointments', 'Janji Temu'],
	['/admin/schedules', 'Jadwal'],
	['/admin/facilities', 'Fasilitas'],
	['/admin/notifications', 'Notifikasi']
];

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await context.newPage();

let failures = 0;

for (const [route, title] of ROUTES) {
	await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
	await page.waitForTimeout(1200);

	const state = await page.evaluate(() => {
		const header = document.querySelector('.sigap-admin-header');
		if (!header) return { header: false };
		const text = (sel) => header.querySelector(sel)?.textContent?.trim() ?? '';
		const refresh = Array.from(header.querySelectorAll('button')).find((b) =>
			/perbarui/i.test(b.textContent ?? '')
		);
		return {
			header: true,
			title: text('.sigap-admin-header__title'),
			subtitle: text('.sigap-admin-header__subtitle'),
			freshness: text('.sigap-admin-freshness'),
			hasRefresh: Boolean(refresh),
			refreshDisabled: refresh?.disabled ?? null
		};
	});

	if (!state.header) {
		console.log(`FAIL ${route} — no admin header rendered`);
		failures++;
		continue;
	}

	const problems = [];
	if (state.title !== title) problems.push(`title "${state.title}" != "${title}"`);
	if (!state.subtitle) problems.push('no subtitle');
	if (!/^Diperbarui pukul \d{2}\.\d{2}$/.test(state.freshness)) {
		problems.push(`freshness "${state.freshness}" missing or malformed`);
	}
	if (!state.hasRefresh) problems.push('no Perbarui control');

	// The refresh must actually re-read.
	//
	// Neither the label nor the disabled state is a reliable signal: the label is
	// minute-resolution, so a refresh inside the same minute is correctly
	// identical, and a local read can settle inside a sampling window. The
	// observable fact is the request the control causes, so count them.
	let reRead = 'not-observed';
	if (state.hasRefresh) {
		const seen = [];
		const onRequest = (req) => {
			const url = req.url();
			if (url.includes('/api/')) seen.push(url.replace(/^https?:\/\/[^/]+/, ''));
		};
		page.on('request', onRequest);
		await page
			.getByRole('button', { name: /perbarui/i })
			.first()
			.click({ timeout: 5000 })
			.catch(() => {});
		await page.waitForTimeout(2500);
		page.off('request', onRequest);
		reRead = seen.length ? `${seen.length} request(s)` : 'NO REQUEST';
		if (seen.length === 0) problems.push('Perbarui issued no API request');
	}

	console.log(
		`${problems.length === 0 ? 'ok  ' : 'FAIL'} ${route.padEnd(24)} ` +
			`title="${state.title}" freshness="${state.freshness}" refresh=${state.hasRefresh} ` +
			`subtitle=${state.subtitle ? 'present' : 'MISSING'} re-read=${reRead}` +
			(problems.length ? `\n       ${problems.join('; ')}` : '')
	);
	if (problems.length) failures++;
}

await browser.close();
console.log(`\n${failures} admin page(s) with header problems`);
process.exitCode = failures > 0 ? 1 : 0;
