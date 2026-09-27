import type { Handle } from '@sveltejs/kit';
import { createSupabaseServerClient } from '$lib/supabase/server';
import { supabaseConfigured } from '$lib/server/auth';

// Security headers applied to every response from the SvelteKit web server.
// These protect against clickjacking, MIME sniffing, and contain XSS via CSP.
export const handle: Handle = async ({ event, resolve }) => {
	let applyAuthHeaders: ((response: Response) => void) | null = null;

	if (supabaseConfigured()) {
		const { client, applyPendingHeaders } = createSupabaseServerClient(event);
		// Refresh the session when the access token is close to expiring so
		// server-side proxies always forward a valid bearer token.
		const {
			data: { session }
		} = await client.auth.getSession();
		event.locals.supabase = client;
		event.locals.session = session;
		applyAuthHeaders = applyPendingHeaders;
	}

	const response = await resolve(event);

	if (applyAuthHeaders) {
		applyAuthHeaders(response);
	}

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
