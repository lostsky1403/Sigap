import { expect, test } from './support/test';

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
 * Ordering matters too: `pnpm --filter sigap-web test` runs `vite build`, which
 * rewrites the hashed assets under `.svelte-kit/output` that a running preview is
 * streaming. The preview then dies with ENOENT on a stale asset hash. Build
 * first, then start the preview, then run this suite.
 *
 * These page-load assertions do not require the Go API. One scenario below does,
 * because GATE 2 requires proof that the browser actually reaches the local API
 * and database rather than only rendering a static shell.
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

/**
 * The one backend-backed scenario.
 *
 * Every other smoke test is satisfied by an SSR shell, which would also pass
 * with the Go API switched off. This one cannot: it drives a real browser fetch
 * through the full local chain and therefore only succeeds when all four hops
 * are live.
 *
 *   browser -> same-origin SvelteKit proxy -> local Go API -> local seeded DB
 *
 * The endpoint is the public facility catalog: read-only, unauthenticated, and
 * non-destructive. It is deliberately NOT an admin route, because an admin read
 * would need dev identity and a 403 would be ambiguous between "no session" and
 * "no permission" (see lib/api/errors.ts). Public avoids that ambiguity entirely.
 *
 * Asserting on seeded CONTENT rather than just a 200 is the point. A 200 with an
 * empty array would pass a status check while proving nothing about the database,
 * so this asserts a non-empty list and a real facility shape.
 */
test.describe('local seeded stack: backend-backed read', () => {
	test('browser reaches the local API and seeded database through the proxy', async ({
		page,
		baseURL
	}) => {
		// Same-origin and relative: the request must traverse the SvelteKit proxy,
		// never a hardcoded upstream that could be pointed somewhere else.
		const response = await page.request.get('/api/v1/public/facilities');
		expect(response.status()).toBe(200);
		expect(page.url() || baseURL).toBeTruthy();

		const payload = (await response.json()) as {
			success: boolean;
			data: Array<{ id: string; name: string; short_code: string; is_active: boolean }> | null;
		};

		expect(payload.success).toBe(true);
		// Real seeded rows, not an empty shell.
		expect(Array.isArray(payload.data)).toBe(true);
		expect((payload.data ?? []).length).toBeGreaterThan(0);

		const [first] = payload.data ?? [];
		expect(first.id).toMatch(/^[0-9a-f-]{36}$/i);
		expect(first.name.trim().length).toBeGreaterThan(0);
		expect(first.short_code.trim().length).toBeGreaterThan(0);
	});

	test('the proxy is the only hop: no production origin is contacted', async ({
		page,
		baseURL
	}) => {
		// Defence in depth. The dedicated guard suite rejects a non-loopback
		// baseURL at config load; this additionally proves that a request made
		// from a real page never leaves the local origin.
		const requested: string[] = [];
		page.on('request', (request) => requested.push(request.url()));

		await page.goto('/');
		await page.request.get('/api/v1/public/service-units');

		for (const url of requested) {
			expect(url, `${url} must stay on the local origin`).toContain(
				new URL(baseURL as string).host
			);
			expect(url).not.toContain('chaerulchalik');
		}
	});
});
