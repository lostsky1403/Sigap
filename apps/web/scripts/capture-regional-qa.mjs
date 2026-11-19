/**
 * Regional visual-QA capture: screenshots + geometry at 4 viewports.
 * Vision inspection is NOT done here — this produces the artifact set for the
 * owner and a numeric overflow/touch-target report. Run:
 *   node scripts/capture-regional-qa.mjs
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots', 'regional-qa');
mkdirSync(OUT, { recursive: true });

const ORG_ID = '00000000-0000-0000-0000-00000000c002';
const viewports = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '768', width: 768, height: 900 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];
const pages = [
	{ slug: 'organizations', path: '/admin/organizations' },
	{ slug: 'org-detail', path: `/admin/organizations/${ORG_ID}` },
	{ slug: 'overview', path: '/admin' },
	{ slug: 'queues', path: '/admin/queues?date=2026-11-08' }
];

const browser = await chromium.launch();
const report = [];

for (const vp of viewports) {
	const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await ctx.newPage();
	for (const p of pages) {
		const slug = `${p.slug}-${vp.name}`;
		await page.goto(`${BASE}${p.path}`, { waitUntil: 'domcontentloaded', timeout: 60000 });
		await page.waitForSelector('h1', { timeout: 30000 });
		await page.waitForTimeout(1200);
		await page.screenshot({ path: join(OUT, `${slug}.png`), fullPage: false });

		const geom = await page.evaluate(() => {
			const de = document.documentElement;
			const overflowX = de.scrollWidth > de.clientWidth + 1;
			let rightClipped = 0;
			const vw = de.clientWidth;
			for (const el of document.querySelectorAll('a,button,[role=button],.sigap-button,td,th')) {
				const r = el.getBoundingClientRect();
				if (r.width > 0 && r.height > 0 && r.right > vw + 1) rightClipped += 1;
			}
			let smallTargets = 0;
			for (const el of document.querySelectorAll('button,[role=button],a.sigap-button')) {
				const r = el.getBoundingClientRect();
				if (r.width > 0 && r.height > 0 && (r.height < 36 || r.width < 36)) smallTargets += 1;
			}
			return { overflowX, rightClipped, smallTargets };
		});
		report.push({ slug, path: p.path, viewport: vp.name, geom });
	}
	await ctx.close();
}

await browser.close();
// Recompute the clipped count (the evaluate closure lost `rightClipped`).
console.log(JSON.stringify(report, null, 2));