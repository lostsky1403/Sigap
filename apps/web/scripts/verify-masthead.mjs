/**
 * Verifies the citizen masthead composition against the frozen frames.
 *
 * Two claims, both read from the rendered DOM:
 *   1. The account control precedes the booking CTA (the frames draw "Masuk"
 *      to the LEFT of "Buat Janji Temu"), and DOM order equals visual order so
 *      tab order matches.
 *   2. Beranda draws a footer.
 *
 * Not a test. Usage: node scripts/verify-masthead.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const browser = await chromium.launch();

const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await context.newPage();

for (const route of ['/', '/faskes', '/auth/login', '/auth/register']) {
	await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
	await page.waitForTimeout(500);

	const r = await page.evaluate(() => {
		const actions = document.querySelector('.sigap-actions');
		const controls = actions
			? Array.from(actions.querySelectorAll('a, button'))
					.filter((el) => getComputedStyle(el).display !== 'none')
					.map((el) => ({
						text: el.textContent.trim().slice(0, 20),
						left: Math.round(el.getBoundingClientRect().left)
					}))
			: [];
		const footer = document.querySelector('footer, .sigap-footer');
		return {
			controls,
			footer: footer ? footer.textContent.trim().replace(/\s+/g, ' ').slice(0, 70) : null
		};
	});

	const byDom = r.controls.map((c) => c.text);
	const byVisual = [...r.controls].sort((a, b) => a.left - b.left).map((c) => c.text);
	const aligned = JSON.stringify(byDom) === JSON.stringify(byVisual);
	console.log(
		`${route.padEnd(16)} dom=[${byDom.join(' | ')}] visual=[${byVisual.join(' | ')}] ` +
			`${aligned ? '(aligned)' : '<-- MISMATCH'} footer=${r.footer ? 'yes' : 'NO'}`
	);
}

await context.close();
await browser.close();
