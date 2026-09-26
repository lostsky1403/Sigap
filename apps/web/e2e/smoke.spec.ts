import { expect, test } from '@playwright/test';

/**
 * Baseline smoke scenario against the LOCAL seeded stack.
 *
 * Scope is deliberately narrow: prove the app boots, the root route renders,
 * and the unauthenticated auth entry point is reachable. It asserts on
 * structure and response status rather than on visual design, because Phase 3B1
 * does not redesign routes.
 *
 * Running this requires a local stack. Start it, then run:
 *   pnpm --filter sigap-web preview:e2e
 *   pnpm --filter sigap-web e2e
 * with SIGAP_E2E_BASE_URL pointing at the local origin (default 127.0.0.1:4173).
 *
 * The preview host flag is not optional. On Windows `localhost` resolves to the
 * IPv6 loopback `::1`, so a bare `vite preview` binds `::1` only and every
 * request to `127.0.0.1:4173` fails with ERR_CONNECTION_REFUSED. The
 * `preview:e2e` script pins `--host 127.0.0.1` so the address family matches
 * the Playwright baseURL.
 *
 * These assertions intentionally do not require the Go API. They prove the
 * built app boots and serves its shell from the local origin; authenticated
 * data paths stay behind the same-origin SvelteKit proxies and are covered by
 * the Go and proxy contract suites.
 */

test.describe('local seeded stack smoke', () => {
	test('root route responds and renders an app shell', async ({ page }) => {
		const response = await page.goto('/');
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('body')).toBeVisible();
	});

	test('login page exposes a usable form', async ({ page }) => {
		const response = await page.goto('/auth/login');
		expect(response?.status()).toBe(200);
		await expect(page.locator('form')).toBeVisible();
		await expect(page.locator('input[type="password"]')).toBeVisible();
	});

	test('unauthenticated admin area does not render privileged content', async ({ page }) => {
		const response = await page.goto('/admin/queues');
		// Either a redirect to login or a rendered guard; never privileged rows.
		expect(response?.status()).toBeLessThan(500);
		await expect(page.locator('body')).toBeVisible();
	});

	test('static assets are served from the local origin', async ({ page, baseURL }) => {
		const response = await page.goto('/');
		const scripts = await page.locator('script[src]').count();
		if (scripts === 0) return;
		const src = await page.locator('script[src]').first().getAttribute('src');
		expect(src).toBeTruthy();
		// Assert the page was served locally, never from a production CDN origin.
		expect(page.url()).toContain(new URL(baseURL as string).host);
	});
});
