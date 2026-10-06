import { defineConfig, devices } from '@playwright/test';

/**
 * Hard production E2E guard.
 *
 * SIGAP browser tests may only ever run against a LOCAL seeded stack. This
 * module is the single place that decides whether a target URL is allowed, and
 * it fails closed: anything that is not an explicit loopback host is rejected
 * before a browser is launched. Both the config and the guard test import it,
 * so the rule cannot drift between them.
 */

/** Production origin that must never be targeted by an E2E run. */
export const PRODUCTION_ORIGIN = 'https://sigap.chaerulchalik.web.id';

/** Hosts an E2E run is permitted to target. */
const ALLOWED_E2E_HOSTNAMES = new Set(['localhost', '127.0.0.1', '::1', '[::1]']);

export class ProductionTargetError extends Error {
	constructor(target: string) {
		super(
			`Refusing to run Playwright against a non-local target: ${target}. ` +
				`E2E runs are restricted to the local seeded stack (localhost). ` +
				`Production is ${PRODUCTION_ORIGIN} and must never be browser-tested.`
		);
		this.name = 'ProductionTargetError';
	}
}

/**
 * Throws unless `target` is a local loopback origin.
 *
 * A target without an explicit scheme is treated as http so that
 * `baseURL: 'localhost:3000'` in an env file is not silently accepted over
 * https. Non-loopback hostnames, bare IPs, and the production origin are all
 * rejected.
 */
export function assertLocalTarget(target: string): URL {
	let url: URL;
	try {
		url = new URL(target.includes('://') ? target : `http://${target}`);
	} catch {
		throw new ProductionTargetError(target);
	}

	if (url.protocol !== 'http:' && url.protocol !== 'https:') {
		throw new ProductionTargetError(target);
	}
	if (!ALLOWED_E2E_HOSTNAMES.has(url.hostname)) {
		throw new ProductionTargetError(target);
	}
	return url;
}

/** Resolves the E2E base URL, defaulting to the local preview server. */
export function resolveBaseURL(): string {
	const configured = process.env.SIGAP_E2E_BASE_URL?.trim();
	return assertLocalTarget(configured && configured.length > 0 ? configured : 'http://127.0.0.1:4173').origin;
}

export default defineConfig({
	testDir: './e2e',
	// Production must be rejected, not retried against.
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? 'list' : [['list']],
	/*
	 * Bounded worker concurrency, and it is not a performance knob.
	 *
	 * Playwright defaults to half the logical cores — 6 on a 12-core host — and
	 * on this host that produced an intermittent
	 * `net::ERR_NO_BUFFER_SPACE at http://127.0.0.1:4173/` at `page.goto`. That
	 * is an operating-system socket-buffer exhaustion error, not an application
	 * one: the connection never reached the preview server, which is why it hit
	 * a different page on each occurrence and why the page under test was never
	 * the thing that was wrong.
	 *
	 * The pressure comes from the whole stack sharing the machine — Postgres,
	 * the Rust engine, the Go API, the Vite preview, and six Chromium workers
	 * each holding a page and its subresource connections. Four workers keeps
	 * the suite well inside the host's socket budget while still running the
	 * file-parallel groups concurrently, so nothing about what the tests ASSERT
	 * changes: the same tests run, in the same order, against the same stack.
	 *
	 * Deliberately NOT solved by raising `retries`. The gate for this phase is
	 * zero failures and zero flakes, and a retry would convert a real signal
	 * into a green tick. It is also not solved by widening timeouts, which
	 * would not help: the error is a refusal to open a socket, not a slow
	 * response.
	 *
	 * Overridable for a host with a larger socket budget:
	 *   SIGAP_E2E_WORKERS=8 pnpm exec playwright test
	 */
	workers: Number(process.env.SIGAP_E2E_WORKERS ?? 4),
	use: {
		baseURL: resolveBaseURL(),
		trace: 'retain-on-failure',
		// Never carry a real session or token into a browser run.
		storageState: undefined
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
