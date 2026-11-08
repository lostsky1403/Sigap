/**
 * Queue mobile-card geometry: band-aware adjacency check + screenshots.
 *
 * Groups rows by `.sigap-queue-band` so "adjacent cards" means cards in the same
 * band, never across a band boundary (a boundary is separated by its heading,
 * not an overlap).
 *
 * Usage: node scripts/repro-queue-mobile.mjs [baseURL] [outDir] [suffix]
 *   suffix defaults to "after"; pass "before" when capturing the unfixed build.
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://localhost:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots');
const SUFFIX = process.argv[4] ?? 'after';
mkdirSync(OUT, { recursive: true });

const viewports = [
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const browser = await chromium.launch();
for (const vp of viewports) {
	const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await ctx.newPage();
	await page.goto(`${BASE}/admin/queues`, { waitUntil: 'domcontentloaded', timeout: 60000 });
	await page.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, { timeout: 30000 });
	await page.waitForSelector('.sigap-queue-row', { timeout: 15000 }).catch(() => {});
	await page.waitForTimeout(800);

	const data = await page.evaluate(() => {
		const doc = document.documentElement;
		const overlaps = [];
		const perRow = [];
		let checked = 0;
		for (const band of document.querySelectorAll('.sigap-queue-band')) {
			const bandKey = band.getAttribute('data-band-key');
			// Only rows in the band's live section — a closed <details> archive is not visible.
			const section = band.querySelector('section');
			if (!section) continue;
			const rows = Array.from(section.querySelectorAll('tr.sigap-queue-row'));
			const shaped = rows.map((row, i) => {
				const card = row.getBoundingClientRect();
				const actions = row.querySelector('.sigap-queue-row__actions');
				let actionsBottom = card.bottom;
				let lastLabel = null;
				if (actions) {
					const ar = actions.getBoundingClientRect();
					for (const b of actions.querySelectorAll('.sigap-button, button')) {
						const br = b.getBoundingClientRect();
						if (br.bottom > actionsBottom) actionsBottom = br.bottom;
						lastLabel = b.textContent?.trim() || lastLabel;
					}
					if (ar.bottom > actionsBottom) actionsBottom = ar.bottom;
				}
				return {
					band: bandKey,
					i,
					number: row.querySelector('.sigap-queue-row__number')?.textContent?.trim(),
					cardTop: Math.round(card.top),
					cardBottom: Math.round(card.bottom),
					actionsBottom: Math.round(actionsBottom),
					lastAction: lastLabel
				};
			});
			for (let i = 0; i < shaped.length; i++) {
				if (i < shaped.length - 1) {
					checked++;
					const cur = shaped[i];
					const next = shaped[i + 1];
					if (cur.actionsBottom > next.cardTop) {
						overlaps.push({ band: bandKey, from: cur.number, to: next.number, overlapPx: cur.actionsBottom - next.cardTop });
					}
				}
				if (perRow.length < 24) perRow.push(shaped[i]);
			}
		}
		return { pageOverflow: doc.scrollWidth - doc.clientWidth, checked, overlaps, perRow };
	});

	console.log(`\n=== ${vp.name} (${vp.width}w) ${SUFFIX.toUpperCase()} ===`);
	console.log(`pageOverflow: ${data.pageOverflow}px`);
	console.log(`adjacent-card pairs checked (within band): ${data.checked}`);
	console.log('OVERLAPS:', JSON.stringify(data.overlaps));
	for (const r of data.perRow.slice(0, 8)) {
		console.log(`  ${r.band} ${r.number}: cardTop=${r.cardTop} cardBottom=${r.cardBottom} actionsBottom=${r.actionsBottom} (last: ${r.lastAction})`);
	}
	await page.screenshot({ path: join(OUT, `admin-antrean-${vp.name}-${SUFFIX}.png`), fullPage: true });
	await ctx.close();
}
await browser.close();
console.log('\ndone');