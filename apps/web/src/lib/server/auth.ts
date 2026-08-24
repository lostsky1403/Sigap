/**
 * Centralized dev-identity header injection for SvelteKit server-side proxies.
 *
 * AUDIT-1802: Previously, every proxy module independently checked
 * `SIGAP_DEV_IDENTITY` and injected `X-Sigap-Dev-User-ID`. This module
 * centralizes that pattern and adds a production guard.
 *
 * Usage in +server.ts:
 *   import { proxyHeaders } from '$lib/server/auth';
 *   const headers = { 'Content-Type': 'application/json', ...proxyHeaders() };
 */

const isDevIdentityEnabled = (): boolean => {
	return process.env.SIGAP_DEV_IDENTITY === 'true';
};

/**
 * Returns headers to attach to upstream API requests.
 *
 * - When `SIGAP_DEV_IDENTITY=true`: injects `X-Sigap-Dev-User-ID: admin-ui`.
 * - When `SIGAP_ENV` is not `local` and dev identity is enabled:
 *   throws at startup to prevent accidental production use.
 * - Otherwise: returns empty headers.
 */
export function proxyHeaders(): Record<string, string> {
	if (!isDevIdentityEnabled()) {
		return {};
	}

	// Production guard: fail fast if dev identity is enabled outside local.
	const env = (process.env.SIGAP_ENV || '').toLowerCase();
	if (env && env !== 'local') {
		throw new Error(
			`SIGAP_DEV_IDENTITY=true is not allowed when SIGAP_ENV=${process.env.SIGAP_ENV}. ` +
			`Set SIGAP_ENV=local for development or disable dev identity.`
		);
	}

	return { 'X-Sigap-Dev-User-ID': 'admin-ui' };
}

/**
 * Returns the base URL for the upstream Go API.
 * Fails fast if not configured (AUDIT-1805).
 */
export function apiBase(): string {
	const base = process.env.SIGAP_API_INTERNAL;
	if (!base) {
		throw new Error(
			'SIGAP_API_INTERNAL is not set. ' +
			'Configure it to point to the Go API (e.g. http://127.0.0.1:18080).'
		);
	}
	return base;
}
