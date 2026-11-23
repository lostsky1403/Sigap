/**
 * Authenticated browser pass: sign in as patient A, open Janji Saya, screenshot.
 * Run: node scripts/capture-auth-journey.mjs
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = 'http://127.0.0.1:5173';
const OUT = join(process.cwd(), '..', '..', 'review-shots');
mkdirSync(OUT, { recursive: true });

const viewports = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '768', width: 768, height: 900 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const browser = await chromium.launch();

for (const vp of viewports) {
	const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await ctx.newPage();

	await page.goto(`${BASE}/auth/login`, { waitUntil: 'domcontentloaded', timeout: 60000 });
	await page.fill('input[name="email"]', 'patient-a@sigap.local');
	await page.fill('input[name="password"]', 'SigapDev123!');
	await page.click('button[type="submit"]');
	await page.waitForTimeout(2500);

	await page.goto(`${BASE}/patient/appointments`, { waitUntil: 'domcontentloaded', timeout: 60000 });
	await page.waitForSelector('h1', { timeout: 30000 });
	await page.waitForTimeout(1500);

	const rows = await page.locator('a[href^="/patient/"], button').count().catch(() => 0);
	await page.screenshot({ path: join(OUT, `auth-janji-saya-${vp.name}.png`) });

	const h1 = await page.locator('h1').first().textContent().catch(() => '');
	console.log(`${vp.name}: h1="${(h1 || '').trim()}"`);
	await ctx.close();
}

await browser.close();
console.log('done');