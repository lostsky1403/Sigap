import type { RequestHandler } from '@sveltejs/kit';

// Proxy POST /api/v1/queues/generate to the Go API (internal Docker URL).
// This allows the SvelteKit client to call relative /api/v1/... (same-origin, no CORS issues from browser).
// Server-side fetch uses SIGAP_API_INTERNAL (e.g. http://api:8080) to reach the api container.
export const POST: RequestHandler = async ({ request }) => {
	const apiBase = process.env.SIGAP_API_INTERNAL || 'http://api:8080';
	const body = await request.text();

	const upstream = await fetch(`${apiBase}/api/v1/queues/generate`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body
	});

	const text = await upstream.text();

	return new Response(text, {
		status: upstream.status,
		headers: {
			'Content-Type': upstream.headers.get('content-type') || 'application/json'
		}
	});
};
