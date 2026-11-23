/**
 * Captures the /patient/status FOUND state from a REAL transaction.
 *
 * The approved Pen frame draws this page mid-queue, but the generic capture
 * script only sees the idle form. The previous version intercepted the status
 * request and substituted a hand-written payload that looked like the frame:
 * a `RSK0042` input (which reads as a queue number, contradicting the six-char
 * check-in alphabet) and an `appointment_time` the status endpoint deliberately
 * withholds as PHI-adjacent. A reviewer who opened that PNG could not tell the
 * fixture from the product, and reported both as contract defects.
 *
 * This version performs the real citizen round trip against the live dev API —
 * book an appointment, check it in, look it up by the REAL six-character code —
 * so the capture shows exactly what the product returns: a real facility, a real
 * code, a real queue number, and NO appointment_time.
 *
 * LOCAL REVIEW TOOLING. It asserts nothing and is not part of the suite.
 *
 * Usage: node scripts/capture-status-found.mjs [webBase] [apiBase]
 */
import { chromium, request } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const WEB = process.argv[2] ?? 'http://127.0.0.1:5173';
const API = process.argv[3] ?? 'http://127.0.0.1:18080';
const OUT = process.argv[4] ?? join(process.cwd(), '..', '..', 'review-shots');
mkdirSync(OUT, { recursive: true });

const FACILITY_ID = '00000000-0000-0000-0000-00000000e005';

/** Books an appointment, checks it in, and returns the real check-in code + queue number. */
async function realVisits() {
	const ctx = await request.newContext({ baseURL: API });

	const su = await (await ctx.get(`/api/v1/public/service-units?facility_id=${FACILITY_ID}`)).json();
	const unitId = su.data[0].id;

	const when = new Date(Date.now() + 2 * 86400e3).toISOString();
	const book = await (
		await ctx.post('/api/v1/appointments', {
			data: {
				facility_id: FACILITY_ID,
				service_unit_id: unitId,
				patient_display_name: 'Verifikasi Status',
				patient_phone: '0812' + String(Date.now()).slice(-8),
				appointment_time: when
			}
		})
	).json();

	await ctx.post(`/api/v1/appointments/${book.data.id}/check-in`, {
		data: { checkin_code: book.data.checkin_code }
	});

	await ctx.dispose();
	return book.data.checkin_code;
}

const code = await realVisits();
console.log('real check-in code:', code);

const VIEWPORTS = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const browser = await chromium.launch();

for (const vp of VIEWPORTS) {
	const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await context.newPage();

	await page.goto(WEB + '/patient/status', { waitUntil: 'networkidle', timeout: 30000 });
	await page.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, {
		timeout: 20_000
	});
	await page.locator('#sigap-status-code').fill(code);
	await page.getByRole('button', { name: /Cek Status/ }).click();
	await page.waitForSelector('[data-testid="visit-progress"]', { timeout: 10_000 });

	await page.screenshot({
		path: join(OUT, `status-found-${vp.name}.png`),
		fullPage: vp.name === '1440'
	});
	await context.close();
}

await browser.close();
console.log('status-found captures written (real transaction)');