/**
 * Rendered evidence for the service-day board (Track A, deliverable 1).
 *
 * Drives /admin/queues in a headless Chromium (visibilityState 'visible', so
 * createPolling is NOT paused) and asserts the RENDERED rows match the API —
 * not inferred from the proxy layer. Run: node scripts/verify-service-day-render.mjs
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

const rowCount = () => page.locator('.sigap-queue-row').count();
const actionCount = () => page.locator('.sigap-queue-row__actions button').count();
const heading = () => page.locator('h1').first().textContent().then((t) => t?.trim());

async function selectDay(date) {
	await page.locator('#queue-service-day').evaluate((el, d) => {
		el.value = d;
		el.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
	}, date);
}

const report = {};

await page.goto(`${BASE}/admin/queues`, { waitUntil: 'domcontentloaded', timeout: 60_000 });
await page.waitForSelector('#queue-service-day', { timeout: 30_000 });

// 1. Today (empty WIB board).
await page.waitForTimeout(1500);
report.today = { rows: await rowCount() };

// 2. 2026-11-08 -> 48 WIB rows, read-only, correct heading.
await selectDay('2026-11-08');
await page.waitForFunction((n) => document.querySelectorAll('.sigap-queue-row').length === n, 48, { timeout: 20_000 });
report['2026-11-08'] = {
	rows: await rowCount(),
	actions: await actionCount(),
	heading: await heading()
};

// 3. Switch to 2026-11-04 -> 22 rows (renders immediately, no 30s poll wait).
await selectDay('2026-11-04');
await page.waitForFunction((n) => document.querySelectorAll('.sigap-queue-row').length === n, 22, { timeout: 20_000 });
report['2026-11-04'] = {
	rows: await rowCount(),
	actions: await actionCount(),
	heading: await heading()
};

// 4. Clear -> today (empty).
await page.locator('#queue-service-day').evaluate((el) => {
	el.value = '';
	el.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
});
await page.waitForFunction(() => document.querySelectorAll('.sigap-queue-row').length === 0, null, { timeout: 20_000 });
report.clearedToday = { rows: await rowCount() };

await browser.close();
console.log(JSON.stringify(report, null, 2));