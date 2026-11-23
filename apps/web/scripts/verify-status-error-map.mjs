/**
 * Verifies how /patient/status maps each upstream failure onto the UI.
 *
 * The contract this asserts is narrow and important: an outage must NEVER be
 * reported as "Kode tidak ditemukan". A citizen who reads that sentence will
 * retype a correct code, walk to the facility, or give up — and the actual
 * cause (the database is down) stays invisible to everyone.
 *
 * The responses are intercepted here on purpose: this script tests the CLIENT's
 * mapping of status codes, so the status code is the input under test. It is
 * not evidence that the backend produces those codes — verify-real-integration
 * covers the real path.
 *
 * Each case gets a fresh browser context. Reusing one page across cases left
 * the form unhydrated in later iterations and produced failures that were
 * harness artefacts, not mapping bugs.
 *
 *   node scripts/verify-status-error-map.mjs http://127.0.0.1:5173
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] || 'http://127.0.0.1:5173';
const ROUTE = '**/api/v1/patient/status*';

const CASES = [
	{
		name: '404 unknown code',
		status: 404,
		body: { success: false, error: 'Kode tidak ditemukan.' },
		expect: 'not-found'
	},
	{
		name: '400 malformed code',
		status: 400,
		body: { success: false, error: 'Kode hanya boleh berisi huruf, angka, dan tanda hubung.' },
		expect: 'field'
	},
	{
		name: '401 unauthenticated upstream',
		status: 401,
		body: { success: false, error: 'Tidak terautentikasi.' },
		expect: 'error'
	},
	{
		name: '429 rate limited',
		status: 429,
		body: { success: false, error: 'Terlalu banyak permintaan. Coba lagi dalam 1 menit.' },
		expect: 'error',
		contains: 'Terlalu banyak permintaan'
	},
	{
		name: '500 server fault',
		status: 500,
		body: { success: false, error: 'Terjadi kesalahan pada server.' },
		expect: 'error'
	},
	{
		name: '503 database unreachable',
		status: 503,
		body: { success: false, error: 'Layanan sementara tidak tersedia.' },
		expect: 'error'
	},
	{
		name: '502 bad gateway (proxy down)',
		status: 502,
		body: '',
		contentType: 'text/html',
		expect: 'error'
	},
	{
		name: 'network abort (no response)',
		status: null,
		expect: 'error'
	}
];

const browser = await chromium.launch();

// Warm the dev server before measuring anything. Vite compiles a route's
// modules on first request, and a cold compile takes longer than the settle
// timeout below — which made the first cases report "nothing rendered" purely
// because the code had not finished compiling. Warming is not part of the test.
{
	const warm = await browser.newContext();
	const warmPage = await warm.newPage();
	await warmPage.route(ROUTE, (route) =>
		route.fulfill({ status: 404, contentType: 'application/json', body: '{"success":false}' })
	);
	await warmPage.goto(`${BASE}/patient/status`, { waitUntil: 'domcontentloaded' });
	await warmPage.waitForFunction(
		() => document.documentElement.dataset.sigapHydrated === 'true',
		null,
		{ timeout: 60_000 }
	);
	await warmPage.locator('#sigap-status-code').waitFor({ state: 'visible' });
	await warm.close();
}

let failures = 0;

for (const testCase of CASES) {
	const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
	const page = await context.newPage();

	await page.route(ROUTE, async (route) => {
		if (testCase.status === null) {
			await route.abort('connectionrefused');
			return;
		}
		// `body` must be a string. Passing an object makes Playwright throw
		// inside the handler, so the request is never fulfilled, the page waits
		// forever, and every case reports "nothing rendered" — a harness bug
		// that looks exactly like a mapping bug.
		await route.fulfill({
			status: testCase.status,
			contentType: testCase.contentType || 'application/json',
			body: typeof testCase.body === 'string' ? testCase.body : JSON.stringify(testCase.body)
		});
	});

	await page.goto(`${BASE}/patient/status`, { waitUntil: 'domcontentloaded' });
	// A click before hydration performs a NATIVE submit: the form has no action,
	// so the browser re-GETs the same URL, the page reloads, and the result
	// region never renders. The layout sets this flag once Svelte has taken over.
	await page.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, {
		timeout: 20_000
	});
	await page.locator('#sigap-status-code').waitFor({ state: 'visible' });
	await page.locator('#sigap-status-code').fill('AB24CD');
	await page.getByRole('button', { name: /Cek Status/i }).click();

	await page
		.waitForFunction(
			() =>
				document.querySelector('[data-testid="status-not-found"]') !== null ||
				document.querySelector('[data-testid="status-error"]') !== null ||
				document.querySelector('#sigap-status-code-error') !== null,
			null,
			{ timeout: 30_000 }
		)
		.catch(() => {});

	const notFound = await page.getByTestId('status-not-found').count();
	const errorPanel = await page.getByTestId('status-error').count();
	const fieldError = await page.locator('#sigap-status-code-error').count();
	const errorText = errorPanel ? (await page.getByTestId('status-error').innerText()).trim() : '';

	let actual;
	if (notFound) actual = 'not-found';
	else if (errorPanel) actual = 'error';
	else if (fieldError) actual = 'field';
	else actual = 'none';

	const textOk =
		!testCase.contains || errorText.toLowerCase().includes(testCase.contains.toLowerCase());
	const ok = actual === testCase.expect && textOk;
	if (!ok) failures++;

	console.log(
		`${ok ? 'ok  ' : 'FAIL'} ${testCase.name.padEnd(32)} -> ${actual.padEnd(9)}` +
			`${errorText ? `  "${errorText.slice(0, 60)}"` : ''}`
	);

	await context.close();
}

await browser.close();
console.log(`\n${failures === 0 ? 'all status mappings correct' : `${failures} mapping failure(s)`}`);
process.exit(failures === 0 ? 0 : 1);
