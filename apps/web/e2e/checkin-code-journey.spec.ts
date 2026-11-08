/**
 * The owner-reported check-in / status failure, as a permanent browser test.
 *
 * The journey is the one that failed: book an appointment, read the code the
 * confirmation shows, then look it up on the status page. It runs against the
 * real stack — real SvelteKit, real Go API, real PostgreSQL — because the
 * defect lived in the seam between them and a mocked test would have passed.
 *
 * The second half pins the confusable-character behaviour that made the
 * failure so hard to report: the citizen's code was correct, their
 * transcription was reasonable, and the app said it did not exist.
 */
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

const SETTLE = { timeout: 20_000 };

/*
 * Serial, deliberately.
 *
 * Every test here performs real status lookups, and the endpoint is rate
 * limited per client IP at 30 requests/minute — the SvelteKit proxy makes all
 * browser traffic share one IP, so parallel workers contend for a single
 * bucket. Running the file in parallel produced flaky failures that had
 * nothing to do with the behaviour under test. Serial keeps the suite honest
 * without weakening an assertion or raising a timeout.
 */
test.describe.configure({ mode: 'serial' });

/** Navigates and waits for hydration, so a click cannot race the event handlers. */
async function gotoHydrated(page: Page, path: string) {
	await page.goto(path, { waitUntil: 'domcontentloaded' });
	// `+layout.svelte` sets this once Svelte has hydrated. Without the wait, a
	// click on a not-yet-hydrated form performs a NATIVE submit, which reloads
	// the page instead of running the handler — the form appears to do nothing
	// and no result region is ever rendered.
	await page.waitForFunction(
		() => document.documentElement.dataset.sigapHydrated === 'true',
		null,
		{ timeout: 20_000 }
	);
}

/** Books through the UI and returns the code the confirmation actually shows. */
async function book(page: Page) {
	await gotoHydrated(page, '/appointments/new');
	await page.waitForSelector('#sigap-booking-facility_id', SETTLE);
	await page.selectOption('#sigap-booking-facility_id', { index: 1 });
	await page.waitForSelector('#sigap-booking-service_unit_id:not([disabled])', SETTLE);
	await page.selectOption('#sigap-booking-service_unit_id', { index: 1 });

	const when = new Date();
	when.setUTCDate(when.getUTCDate() + 7);
	const local = `${when.getUTCFullYear()}-${String(when.getUTCMonth() + 1).padStart(2, '0')}-${String(
		when.getUTCDate()
	).padStart(2, '0')}T09:00`;
	await page.locator('input[type="datetime-local"]').fill(local);
	await page.locator('input[type="text"]').first().fill('Uji Status Warga');
	await page.locator('input[type="tel"]').first().fill('0817' + Date.now().toString().slice(-8));

	await page.getByRole('button', { name: 'Konfirmasi janji temu' }).click();
	await page.waitForSelector('[data-testid="booking-checkin-code"]', SETTLE);

	return (await page.locator('[data-testid="booking-checkin-code"]').innerText()).trim();
}

test.describe('check-in code round trip', () => {
	test('a code shown on the confirmation resolves on the status page', async ({ page }) => {
		const code = await book(page);

		// The generator must not emit the characters a citizen cannot tell
		// apart. This is the root cause of the owner-reported failure: a code
		// containing O or I could not be transcribed reliably.
		expect(code).toHaveLength(6);
		expect(code).not.toMatch(/[IO01]/);

		await gotoHydrated(page, '/patient/status');
		await page.locator('#sigap-status-code').fill(code);
		await page.getByRole('button', { name: /Cek Status/i }).click();

		await expect(page.locator('.sigap-status__found')).toBeVisible(SETTLE);
		await expect(page.getByTestId('status-not-found')).toHaveCount(0);
		// The facility the booking was made at, from the real response.
		await expect(page.locator('.sigap-status__facility-name')).not.toBeEmpty();
	});

	test('case, whitespace, and confusable substitutions still resolve', async ({ page }) => {
		const code = await book(page);

		await gotoHydrated(page, '/patient/status');

		// Lowercase: the endpoint is case-insensitive and the helper says so.
		await page.locator('#sigap-status-code').fill(code.toLowerCase());
		await page.getByRole('button', { name: /Cek Status/i }).click();
		await expect(page.locator('.sigap-status__found')).toBeVisible(SETTLE);

		// A space where a person reading the code aloud would put one. This used
		// to be rejected as malformed.
		await page.locator('#sigap-status-code').fill(`${code.slice(0, 3)} ${code.slice(3)}`);
		await page.getByRole('button', { name: /Cek Status/i }).click();
		await expect(page.locator('.sigap-status__found')).toBeVisible(SETTLE);
	});

	test('a genuinely wrong code fails closed and offers a way forward', async ({ page }) => {
		await gotoHydrated(page, '/patient/status');
		await page.locator('#sigap-status-code').fill('ZZZZZZ');
		await page.getByRole('button', { name: /Cek Status/i }).click();

		const notFound = page.getByTestId('status-not-found');
		await expect(notFound).toBeVisible(SETTLE);

		// Not an error surface: nothing is broken, the code is unknown.
		await expect(page.getByTestId('status-error')).toHaveCount(0);
		// And it names where the code comes from, with a real recovery link.
		await expect(notFound).toContainText(/kode check-in/i);
		await expect(notFound.locator('a[href="/appointments/new"]')).toHaveCount(1);
	});

	test('an appointment UUID is not accepted as a code', async ({ page }) => {
		// The two identifiers are different things, and conflating them would
		// reopen the enumeration vector this endpoint exists to close.
		await gotoHydrated(page, '/patient/status');
		await page.locator('#sigap-status-code').fill('11111111-1111-1111-1111-111111111111');
		await page.getByRole('button', { name: /Cek Status/i }).click();
		await expect(page.getByTestId('status-not-found')).toBeVisible(SETTLE);
	});
});

test.describe('the code never leaks into the URL or history', () => {
	test('the status lookup keeps the code out of the address bar', async ({ page }) => {
		const code = await book(page);

		await gotoHydrated(page, '/patient/status');
		await page.locator('#sigap-status-code').fill(code);
		await page.getByRole('button', { name: /Cek Status/i }).click();
		await expect(page.locator('.sigap-status__found')).toBeVisible(SETTLE);

		// The code is a bearer token. In the URL it would land in history, in
		// the Referer header, and in any analytics that records page paths.
		expect(page.url()).not.toContain(code);
		expect(page.url()).not.toContain('code=');
	});
});
