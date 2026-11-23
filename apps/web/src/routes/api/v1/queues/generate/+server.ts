import type { RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

// Proxy POST /api/v1/queues/generate to the Go API (internal URL).
// This allows the SvelteKit client to call relative /api/v1/... (same-origin, no CORS issues from browser).
// The upstream base comes from SIGAP_API_INTERNAL via resolveApiBase, which refuses
// to guess a host: a wrong guess surfaces as an outage and hides the real cause.
export const POST: RequestHandler = async (event) => {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const body = await event.request.text();

	const upstream = await fetch(`${resolved.base}/api/v1/queues/generate`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			...authHeaders(event)
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
