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
	use: {
		baseURL: resolveBaseURL(),
		trace: 'retain-on-failure',
		// Never carry a real session or token into a browser run.
		storageState: undefined
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
