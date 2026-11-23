/**
 * Verifies specific QA findings against the LIVE rendered DOM.
 *
 * Each check states the claim it is testing and prints the raw evidence, so a
 * finding is confirmed or refuted by the page rather than by a screenshot read.
 *
 * Not a test. Usage: node scripts/verify-findings.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const browser = await chromium.launch();

async function open(path, width = 1440) {
	const context = await browser.newContext({ viewport: { width, height: 1000 } });
	const page = await context.newPage();
	await page.goto(BASE + path, { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
	await page.waitForTimeout(1200);
	return { page, context };
}

// --- P1-2: Jadwal WAKTU column -------------------------------------------------
{
	const { page, context } = await open('/admin/schedules');
	const cells = await page.evaluate(() =>
		Array.from(document.querySelectorAll('table tbody tr')).slice(0, 5).map((tr) =>
			Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim())
		)
	);
	console.log('P1-2 Jadwal rows:', JSON.stringify(cells, null, 1));
	await context.close();
}

// --- P1-3: Antrean 768 unheaded cell ------------------------------------------
{
	const { page, context } = await open('/admin/queues', 768);
	const t = await page.evaluate(() => {
		const table = document.querySelector('table');
		if (!table) return { note: 'no table' };
		const headers = Array.from(table.querySelectorAll('thead th')).map((th) => th.textContent.trim());
		const firstRow = table.querySelector('tbody tr');
		const cells = firstRow
			? Array.from(firstRow.querySelectorAll('td')).map((td) => ({
					text: td.textContent.trim().slice(0, 24),
					hidden: getComputedStyle(td).display === 'none'
				}))
			: [];
		return { headers, cells };
	});
	console.log('P1-3 Antrean 768 headers:', JSON.stringify(t.headers));
	console.log('P1-3 Antrean 768 cells  :', JSON.stringify(t.cells, null, 1));
	await context.close();
}

// --- P1-1 / P2-5: Antrean action labels and column count ----------------------
{
	const { page, context } = await open('/admin/queues');
	const t = await page.evaluate(() => {
		const table = document.querySelector('table');
		const headers = table
			? Array.from(table.querySelectorAll('thead th')).map((th) => th.textContent.trim())
			: [];
		const buttons = Array.from(document.querySelectorAll('table button'))
			.map((b) => b.textContent.trim())
			.slice(0, 10);
		return { headers, buttons };
	});
	console.log('P2-5 Antrean 1440 headers:', JSON.stringify(t.headers));
	console.log('P1-1 Antrean action labels:', JSON.stringify(t.buttons));
	await context.close();
}

// --- P1-4: Notifikasi KANAL ---------------------------------------------------
{
	const { page, context } = await open('/admin/notifications');
	const t = await page.evaluate(() => {
		const table = document.querySelector('table');
		const headers = table
			? Array.from(table.querySelectorAll('thead th')).map((th) => th.textContent.trim())
			: [];
		const rows = table
			? Array.from(table.querySelectorAll('tbody tr')).slice(0, 5).map((tr) =>
					Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim().slice(0, 20))
				)
			: [];
		const placeholder = document.querySelector('input[placeholder]')?.placeholder ?? '';
		return { headers, rows, placeholder };
	});
	console.log('P1-4 Notifikasi headers:', JSON.stringify(t.headers));
	console.log('P1-4 Notifikasi rows   :', JSON.stringify(t.rows, null, 1));
	console.log('P2-7 placeholder       :', JSON.stringify(t.placeholder));
	await context.close();
}

// --- P1-1b: Janji Temu action labels ------------------------------------------
{
	const { page, context } = await open('/admin/appointments');
	const t = await page.evaluate(() =>
		Array.from(document.querySelectorAll('table tbody tr')).slice(0, 5).map((tr) => ({
			status: tr.querySelector('td')?.textContent.trim().slice(0, 14),
			actions: Array.from(tr.querySelectorAll('button')).map((b) => b.textContent.trim()),
			text: tr.querySelector('td:last-child')?.textContent.trim().slice(0, 40)
		}))
	);
	console.log('P1-1b Janji Temu rows:', JSON.stringify(t, null, 1));
	await context.close();
}

await browser.close();
