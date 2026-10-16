/**
 * Visual review capture.
 *
 * Walks the citizen and admin routes at the four review viewports and writes
 * one PNG per route/viewport pair. The output is compared against the approved
 * Pen frames in `design/prototypes/v22-shots/`, so the filenames are the route
 * with slashes replaced, which keeps the mapping readable without a manifest.
 *
 * Not a test. It asserts nothing and it is not part of the suite — it exists so
 * a human can look at the app, and so the comparison against the frozen frames
 * has evidence behind it rather than a recollection.
 *
 * Usage:
 *   node scripts/capture-review.mjs [baseURL]
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots');

const VIEWPORTS = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '768', width: 768, height: 1024 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const ROUTES = [
	['/', 'beranda'],
	['/faskes', 'faskes'],
	['/appointments/new', 'booking'],
	['/appointments/check-in', 'check-in'],
	['/queues/new', 'walk-in'],
	['/patient/status', 'status'],
	['/auth/login', 'login'],
	['/auth/register', 'register'],
	['/auth/logout', 'logout'],
	['/admin', 'admin-ringkasan'],
	['/admin/queues', 'admin-antrean'],
	['/admin/appointments', 'admin-janji-temu'],
	['/admin/schedules', 'admin-jadwal'],
	['/admin/facilities', 'admin-fasilitas'],
	['/admin/notifications', 'admin-notifikasi']
];

mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();

/*
 * Warm-up pass.
 *
 * Vite compiles a route on its first request, and on a cold server that compile
 * can outlast the navigation timeout for whichever viewport runs first — which
 * reported as `page.goto: Navigation ... is interrupted by another navigation`
 * for the six admin routes, on a build where they render perfectly. This pass
 * requests every route once and discards the result, so the measured captures
 * below never race a first compile.
 */
{
	const warm = await browser.newContext();
	const warmPage = await warm.newPage();
	for (const [route] of ROUTES) {
		await warmPage.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 90_000 }).catch(() => {});
	}
	await warm.close();
	console.log('warm-up complete');
}

for (const viewport of VIEWPORTS) {
	const context = await browser.newContext({
		viewport: { width: viewport.width, height: viewport.height },
		deviceScaleFactor: 2
	});
	const page = await context.newPage();

	for (const [route, slug] of ROUTES) {
		const file = join(OUT, `${slug}-${viewport.name}.png`);
		try {
			/*
			 * `domcontentloaded` rather than `networkidle`, then an explicit
			 * wait for the hydration marker. `networkidle` is the tempting
			 * choice and the wrong one here: the first request after a cold
			 * Vite start compiles the route on demand, and on a busy machine
			 * that exceeded the 30s budget and reported a timeout for a page
			 * that renders perfectly. The marker is the same contract the E2E
			 * fixture uses, and it is what actually has to be true before a
			 * screenshot means anything.
			 */
			const response = await page.goto(`${BASE}${route}`, {
				waitUntil: 'domcontentloaded',
				timeout: 60_000
			});
			await page.waitForFunction(
				() => document.documentElement.dataset.sigapHydrated === 'true',
				null,
				{ timeout: 30_000 }
			);
			/*
			 * One more frame for any in-flight fetch the page issues on mount.
			 * Beranda and Faskes both request the public catalog, and a
			 * screenshot taken mid-request shows skeletons — which is a real
			 * state, but not the one under review.
			 */
			await page.waitForLoadState('networkidle', { timeout: 15_000 }).catch(() => {});
			await page.waitForTimeout(250);
			await page.screenshot({ path: file, fullPage: true });
			console.log(`ok   ${viewport.name.padStart(4)} ${route} -> ${file} (${response?.status()})`);
		} catch (error) {
			console.log(`FAIL ${viewport.name.padStart(4)} ${route}: ${error.message}`);
		}
	}

	await context.close();
}

await browser.close();
