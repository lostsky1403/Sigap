import type { RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

// Proxy GET /api/v1/events/beds for Server-Sent Events (SSE) to Go API.
// Forwards the stream so client can do new EventSource('/api/v1/events/beds') (relative, same origin).
// The upstream base comes from SIGAP_API_INTERNAL via resolveApiBase, which refuses
// to guess a host: a wrong guess surfaces as an outage and hides the real cause.
export const GET: RequestHandler = async (event) => {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const upstream = await fetch(`${resolved.base}/api/v1/events/beds`, {
		headers: { ...authHeaders(event) }
	});

	// Forward key streaming headers + ensure SSE semantics
	const headers = new Headers();
	// Copy important headers from upstream
	upstream.headers.forEach((value, key) => {
		const lower = key.toLowerCase();
		if (lower === 'content-type' || lower === 'cache-control' || lower === 'connection') {
			headers.set(key, value);
		}
	});

	// Guarantee SSE headers for the client
	headers.set('Content-Type', 'text/event-stream');
	headers.set('Cache-Control', 'no-cache');
	headers.set('Connection', 'keep-alive');

	return new Response(upstream.body, {
		status: upstream.status,
		headers
	});
};
