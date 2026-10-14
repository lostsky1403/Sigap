/**
 * Centralized dev-identity header injection for SvelteKit server-side proxies.
 *
 * AUDIT-1802: Previously, every proxy module independently checked
 * `SIGAP_DEV_IDENTITY` and injected `X-Sigap-Dev-User-ID`. This module
 * centralizes that pattern and adds a production guard.
 *
 * Usage in +server.ts:
 *   import { proxyHeaders, apiBase } from '$lib/server/auth';
 *   const headers = { 'Content-Type': 'application/json', ...proxyHeaders() };
 */

import type { RequestEvent } from '@sveltejs/kit';

/**
 * Minimal view of a Supabase session used by server-side proxies.
 * Mirrors the subset of `@supabase/supabase-js` `Session` we forward.
 */
export interface AuthSessionLike {
	access_token?: string;
	user_id?: string;
	user?: { id?: string; email?: string };
}

/** True when the public Supabase configuration is present at runtime. */
export function supabaseConfigured(): boolean {
	return Boolean(process.env.PUBLIC_SUPABASE_URL && process.env.PUBLIC_SUPABASE_ANON_KEY);
}

/**
 * Returns headers to attach to upstream API requests from a browser session.
 *
 * When the request carries a valid Supabase session (cookie-based, refreshed
 * in hooks.server.ts), forwards `Authorization: Bearer <access_token>` so the
 * Go API validates the JWT and resolves permissions server-side (DB RBAC).
 * Never trusts token permission claims; the API resolves roles from the DB.
 * Returns empty headers when there is no session or Supabase is unconfigured.
 */
export function authHeaders(event: RequestEvent): Record<string, string> {
	const token = event.locals.session?.access_token;
	if (!token) {
		return {};
	}
	return { Authorization: `Bearer ${token}` };
}

/**
 * Returns headers to attach to upstream API requests.
 *
 * SECURITY: This function NEVER injects the X-Sigap-Dev-User-ID header.
 * The client is always untrusted — any header it sets is fully
 * client-controlled and must never carry authentication meaning
 * (upstream main, vuln-0002 follow-up).
 *
 * The ONE header it may emit is `X-Sigap-Local-Test-Subject`, and only when
 * the process is provably local: `localE2eActorHeader()` returns `{}` unless
 * `SIGAP_ENV=local` AND `SIGAP_LOCAL_E2E_ACTOR` is a non-empty subject. The Go
 * API additionally refuses to arm that selector outside `SIGAP_ENV=local`, so
 * the two tiers agree.
 *
 * - When a local E2E actor is configured (and only then): injects
 *   `X-Sigap-Local-Test-Subject`.
 * - Otherwise: returns empty headers.
 */
export function proxyHeaders(): Record<string, string> {
	return localE2eActorHeader();
}

/**
 * T-3B5 §2 — the LOCAL E2E actor selector.
 *
 * WHAT THIS IS FOR
 *
 * The schedule and notification mutation contracts are only meaningful if they
 * are proven against actors whose database grants genuinely differ. The seeded
 * `e2e-schedule-manager` (d993) holds schedule.manage at one facility;
 * `e2e-schedule-mixed` (d994) holds schedule.read at one facility and
 * schedule.manage at a DIFFERENT one. That divergence is the regression the
 * affordance contract exists to catch, and it can only be observed by running
 * as each actor in turn.
 *
 * The Go API already accepts `X-Sigap-Local-Test-Subject` and resolves it
 * through the real RBAC resolver. What was missing was a way for the E2E stack
 * to use it without making identity a browser-controlled input.
 *
 * WHY THE VALUE IS NOT A REQUEST HEADER
 *
 * The obvious implementation — forward `request.headers.get('x-sigap-...')` —
 * is exactly the vulnerability this phase is told to avoid, and it fails twice
 * over:
 *
 *   1. It would make identity a client-controlled input on the web tier, so
 *      ANY browser (and any attacker who can reach the web origin) could
 *      select a different database-backed actor simply by setting a header.
 *      §1 of the spec forbids the browser being an authorization authority, and
 *      this is the precise shape of that failure.
 *   2. It would need the browser to be able to SET the header, which it cannot
 *      do anyway: the Go CORS allowlist covers only Content-Type, Authorization
 *      and X-Requested-With, so a cross-origin attempt fails preflight. The
 *      existing proxies are same-origin, so a browser-issued X-Sigap-* header
 *      would actually reach the Go API, and then the API would honour it. That
 *      is a working impersonation path, not a theoretical one.
 *
 * So the value below comes from the PROCESS environment, which is set by
 * `scripts/dev/Start-LocalE2E.ps1` before the server starts. The browser can
 * read it only in the sense that the resulting page is rendered as that actor —
 * which is the entire point of the E2E — and cannot set it, forge it, or
 * influence it with a request.
 *
 * WHY THE RESULT IS STILL SERVER-AUTHORITATIVE
 *
 * This header names an identity; it grants nothing. The Go API resolves that
 * subject's permissions and facility grants from the database on every
 * request, and each mutation endpoint re-authorizes independently. A subject
 * with no seeded roles resolves to zero permissions and every mutation is
 * refused. Deleting the seed row removes the access with no code change.
 *
 * FOUR INDEPENDENT GATES, any one of which is sufficient to disallow it:
 *
 *   1. `SIGAP_ENV=local` must be set, or the selector returns no header at all.
 *   2. `SIGAP_LOCAL_E2E_ACTOR` must be set to a non-empty subject.
 *   3. The value is fixed at server start; no request can change it.
 *   4. The Go side is separately disarmed outside `SIGAP_ENV=local`
 *      (`config.GuardDevCapabilities` refuses to start such a process), so even
 *      if a header were forged, an outside-local API would not honour it.
 *
 * Gates 1 and 4 are deliberately redundant with the Go-side gates rather than
 * relying on them: the web tier must not be the only thing standing between a
 * test-only header and production.
 */
export function localE2eActorHeader(): Record<string, string> {
	if ((process.env.SIGAP_ENV || '').toLowerCase() !== 'local') {
		return {};
	}
	const subject = (process.env.SIGAP_LOCAL_E2E_ACTOR || '').trim();
	if (subject === '') {
		return {};
	}
	return { 'X-Sigap-Local-Test-Subject': subject };
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
