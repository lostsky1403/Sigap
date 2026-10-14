import { test as base, expect, type Page } from '@playwright/test';

/**
 * The E2E `test`, with every navigation gated on hydration.
 *
 * WHY THIS EXISTS
 *
 * The app is a SvelteKit SSR application, so a page has two lives: the server
 * HTML is painted immediately, and the client takes over a moment later. Every
 * assertion in this suite is about the SECOND life — a bound input, a form
 * submit, a prefill read from the deep link — and none of them are true until
 * hydration has finished.
 *
 * Playwright cannot tell the difference on its own. `page.goto()` resolves at
 * the load event, and its actionability checks (visible, enabled, editable,
 * stable) are all satisfied by the server HTML. So a `fill()` issued right
 * after `goto()` can land in the gap, where the input has no listener yet: the
 * event is discarded, the component's bound value stays empty, and the page
 * then behaves exactly as if the user had typed nothing. That is not a
 * hypothetical — it produced two intermittent failures that looked like
 * product bugs:
 *
 *   - `/patient/status`: the code input stayed empty, so submitting tripped
 *     the page's own "Kode wajib diisi." validation and NEITHER the found
 *     result NOR the not-found panel ever rendered. The assertion that timed
 *     out was `visit-steps`, three steps away from the real cause.
 *   - `/appointments/check-in`: the deep-link prefill is read in `onMount`, so
 *     `toHaveValue()` sat on an empty input until its timeout expired.
 *
 * Both are timing-dependent, which is why they appeared under parallel load
 * and vanished on a quiet machine. Retrying them, or widening their timeouts,
 * would have hidden a real property of the app rather than encoding it.
 *
 * HOW IT WORKS
 *
 * `routes/+layout.svelte` sets `data-sigap-hydrated="true"` on `<html>` from
 * its own `onMount`. Svelte runs a child's `onMount` before its parent's, so
 * the root layout is the LAST component to mount: the marker appearing proves
 * the entire tree is interactive, including any page that reads a query
 * parameter in `onMount`.
 *
 * This fixture wraps the methods that start a DOCUMENT LOAD — `page.goto`,
 * `page.reload`, `page.goBack`, `page.goForward` — so the wait happens for
 * every such navigation in every spec, without each test having to remember it.
 * That is deliberate: a per-test convention would be one forgotten call away
 * from the same flake returning, and the failure it produces points at the
 * wrong place.
 *
 * It is not a total guarantee, and it is worth being exact about what is left
 * outside. A full document load can still be triggered by a click on a link
 * that does not go through the SvelteKit router, or by `page.setContent`. Those
 * do not appear in this suite, and a spec that adds one should await
 * `waitForHydration` itself. What the fixture removes is the whole class that
 * DID appear — including the reload inside the quiescence loops, which would
 * otherwise have reintroduced the race in the very code written to avoid it.
 *
 * The timeout is generous because hydration latency is a property of the
 * machine, not of the assertion: under a full parallel matrix it is measured
 * in seconds, on an idle stack in tens of milliseconds. A test that fails here
 * is reporting that the app never became interactive, which is worth knowing.
 */
const HYDRATION_TIMEOUT = 30_000;

/**
 * Waits until the app has hydrated.
 *
 * Exported for a test that starts a document load by some other means — a
 * click on a full-page link, or `page.setContent` — rather than through the
 * wrapped navigation methods.
 */
export async function waitForHydration(page: Page, timeout = HYDRATION_TIMEOUT): Promise<void> {
	await expect(page.locator('html')).toHaveAttribute('data-sigap-hydrated', 'true', { timeout });
}

export const test = base.extend<{ page: Page }>({
	page: async ({ page }, use) => {
		const navigate = page.goto.bind(page);

		page.goto = async (url, options) => {
			const response = await navigate(url, options);
			await waitForHydration(page);
			return response;
		};

		/*
		 * reload/back/forward start a document load too, and they are wrapped
		 * for the same reason as `goto`.
		 *
		 * They are ALSO the reason the fixture cannot simply intercept `goto`
		 * and call the guarantee complete: the quiescence loops in
		 * admin-mutations.spec.ts and citizen-transactions.spec.ts reload to
		 * re-take a page against a stable server snapshot, and a reload that
		 * returned before hydration would reintroduce the exact race this
		 * fixture exists to remove — inside the very code written to avoid it.
		 * Wrapping them here means those call sites do not each have to
		 * remember, and a future one cannot forget.
		 */
		for (const method of ['reload', 'goBack', 'goForward'] as const) {
			const original = page[method].bind(page);
			page[method] = async (...args: Parameters<typeof original>) => {
				const response = await original(...args);
				await waitForHydration(page);
				return response;
			};
		}

		await use(page);
	}
});

export { expect };
