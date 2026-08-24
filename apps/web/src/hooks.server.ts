import type { Handle } from '@sveltejs/kit';

// Security headers applied to every response from the SvelteKit web server.
// These protect against clickjacking, MIME sniffing, and contain XSS via CSP.
export const handle: Handle = async ({ event, resolve }) => {
	const response = await resolve(event);

	// Prevent clickjacking — no framing of the admin UI.
	response.headers.set('X-Frame-Options', 'DENY');

	// Content Security Policy:
	//   - default-src 'self': only load resources from same origin
	//   - script-src 'self': no inline scripts
	//   - style-src 'self' 'unsafe-inline': SvelteKit needs unsafe-inline for scoped styles
	//   - connect-src 'self': API calls and SSE go to same origin (proxied)
	//   - img-src 'self' data: inline images for Svelte components
	//   - frame-ancestors 'none': prevent framing entirely
	response.headers.set(
		'Content-Security-Policy',
		"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'"
	);

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
