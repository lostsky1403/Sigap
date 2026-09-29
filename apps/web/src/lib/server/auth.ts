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

const isDevIdentityEnabled = (): boolean => {
	return process.env.SIGAP_DEV_IDENTITY === 'true';
};

/**
 * Returns headers to attach to upstream API requests.
 *
 * THE ORDER HERE IS A SECURITY DECISION, not a style choice.
 *
 * The Go API selects ONE provider at startup: when
 * `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true` and the pool is present, the local
 * DB-backed selector REPLACES the dev identity provider entirely
 * (`cmd/server/main.go`). So while that flag is armed:
 *
 *   - `X-Sigap-Dev-User-ID` is IGNORED. The local provider never reads it.
 *   - `X-Sigap-Local-Test-Subject` is what actually selects the actor.
 *   - Sending only the dev header would produce a ZERO actor, because the
 *     armed provider finds no subject and returns `identity.Actor{}` — which
 *     surfaces as a 401 on every admin read, not as a readable auth error.
 *
 * So the local actor header REPLACES the dev header rather than accompanying
 * it. This mirrors the Go-side precedence exactly, and it is the reason the
 * local actor is expressed here at all: the browser picks nothing, the process
 * does, and the two tiers agree on which one wins.
 *
 * - When a local E2E actor is configured: injects only
 *   `X-Sigap-Local-Test-Subject`.
 * - When `SIGAP_DEV_IDENTITY=true` and no local actor is configured: injects
 *   `X-Sigap-Dev-User-ID: admin-ui`.
 * - When `SIGAP_ENV` is not `local` and dev identity is enabled:
 *   throws at startup to prevent accidental production use.
 * - Otherwise: returns empty headers.
 */
export function proxyHeaders(): Record<string, string> {
	// Checked FIRST so the local actor wins over dev identity, matching the
	// Go-side provider precedence described above.
	const localActor = localE2eActorHeader();
	if (Object.keys(localActor).length > 0) {
		return localActor;
	}

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
