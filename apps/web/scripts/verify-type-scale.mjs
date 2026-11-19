/**
 * Verifies the citizen type scale against the Pen's own numbers.
 *
 * The Pen draws two different page-title scales — a compact one on Beranda
 * (22px/600 phone, 24px/700 desktop) and a 32px/600 one on the task pages — plus
 * 24px/700 section headings on Beranda. This reads the computed values from the
 * rendered page so the claim is a measurement rather than an impression.
 *
 * Not a test. Usage: node scripts/verify-type-scale.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const browser = await chromium.launch();

const ROUTES = ['/', '/faskes', '/patient/status', '/appointments/new', '/queues/new'];

for (const width of [1440, 390]) {
	const context = await browser.newContext({ viewport: { width, height: 900 } });
	const page = await context.newPage();

	for (const route of ROUTES) {
		await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
		await page.waitForTimeout(400);

		const r = await page.evaluate(() => {
			const read = (sel) => {
				const el = document.querySelector(sel);
				if (!el) return null;
				const cs = getComputedStyle(el);
				return `${cs.fontSize}/${cs.fontWeight}`;
			};
			// The smallest rendered text on the page, to confirm the 11px floor.
			let min = 999;
			let minEl = '';
			for (const el of document.querySelectorAll('body *')) {
				if (!(el.textContent ?? '').trim()) continue;
				if (el.children.length) continue;
				const cs = getComputedStyle(el);
				if (cs.display === 'none' || cs.visibility === 'hidden') continue;
				const px = parseFloat(cs.fontSize);
				if (px > 0 && px < min) {
					min = px;
					minEl = el.className || el.tagName;
				}
			}
			return { h1: read('h1'), h2: read('h2'), minText: `${min}px (${String(minEl).slice(0, 24)})` };
		});

		console.log(`${String(width).padStart(4)} ${route.padEnd(20)} h1=${r.h1} h2=${r.h2} min=${r.minText}`);
	}

	await context.close();
}

await browser.close();
