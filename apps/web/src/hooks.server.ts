import type { Handle } from '@sveltejs/kit';
import { building } from '$app/environment';
import { auth, betterAuthConfigured, type AuthSession } from '$lib/server/auth.better';
import { svelteKitHandler } from 'better-auth/svelte-kit';

// Security headers applied to every response from the SvelteKit web server.
// These protect against clickjacking, MIME sniffing, and contain XSS via CSP.
export const handle: Handle = async ({ event, resolve }) => {
	// Resolve the Better Auth session from the request cookie. When Better Auth
	// is unconfigured (no Postgres connection string for its `auth` schema) the
	// instance cannot answer the session query. Crucially, a session-store
	// outage (auth schema unreachable) must DEGRADE to signed-out, not 500 every
	// route — the public facility catalog and citizen status page must survive
	// an auth-store hiccup. So the resolve is guarded: on failure we log once and
	// leave the session null, which downstream means unauthenticated (safe 401),
	// never a whole-site 500.
	let session: AuthSession | null = null;
	if (betterAuthConfigured()) {
		try {
			session = await auth.api.getSession({ headers: event.request.headers });
		} catch (err) {
			console.warn('[sigap] auth session resolve failed; degrading to signed-out', err);
			session = null;
		}
	}

	event.locals.auth = betterAuthConfigured() ? auth : null;
	event.locals.session = session;
	event.locals.jwt = null;

	// Mint the ES256 JWT the Go API validates. `session.session.token` is Better
	// Auth's opaque session token (NOT a JWT); the jwt plugin exposes the real
	// signed token at GET /api/auth/token via auth.api.getToken. Failure to mint
	// (or no session) leaves locals.jwt null and every proxy forwards no Bearer.
	if (session?.session?.token) {
		try {
			const { token } = await auth.api.getToken({ headers: event.request.headers });
			event.locals.jwt = token ?? null;
		} catch {
			event.locals.jwt = null;
		}
	}

	// `svelteKitHandler` serves Better Auth's own `/api/auth/*` routes through
	// `auth.handler` and lets every other request fall through to
	// `resolve(event)`. It returns the final Response for BOTH cases, so the
	// security headers are applied to its result once, below.
	const response = await svelteKitHandler({ event, resolve, auth, building });

	// Prevent clickjacking — no framing of the admin UI.
	response.headers.set('X-Frame-Options', 'DENY');

	// The Content-Security-Policy header itself is set by SvelteKit from
	// `kit.csp.directives` in svelte.config.js, together with the matching
	// per-request nonce on its inline hydration script.
	//
	// It used to be hardcoded here as `script-src 'self'`, which is a genuine
	// defect: SvelteKit emits its hydration payload as an inline <script>, so
	// that policy blocked the very script that makes the app interactive. Pages
	// still rendered their server HTML, and every server-side test passed, but
	// in a browser no fetch fired, no form submitted, and no navigation worked.
	// Two independent places now guard it — a CSP nonce in svelte.config.js, and
	// a hydration assertion in e2e/citizen-shell.spec.ts.
	//
	// That CSP + nonce is emitted by `resolve(event)` for SvelteKit-rendered
	// HTML. Better Auth's own `/api/auth/*` responses are produced directly by
	// `auth.handler` and carry no inline scripts or hydration payload, so there
	// is no nonce to inject on those paths — which is correct, because they are
	// JSON/redirect API responses, not script-bearing documents.

	// Prevent MIME sniffing.
	response.headers.set('X-Content-Type-Options', 'nosniff');

	// Referrer policy.
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');

	// HSTS: set when behind TLS (production).
	if (event.url.protocol === 'https:') {
		response.headers.set('Strict-Transport-Security', 'max-age=63072000; includeSubDomains');
	}

	return response;
};