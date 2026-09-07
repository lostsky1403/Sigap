import type { RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';

// Proxy GET /api/v1/events/beds for Server-Sent Events (SSE) to Go API.
// Forwards the stream so client can do new EventSource('/api/v1/events/beds') (relative, same origin).
// Uses internal Docker hostname from SIGAP_API_INTERNAL so it works inside compose network.
export const GET: RequestHandler = async (event) => {
	const apiBase = process.env.SIGAP_API_INTERNAL || 'http://api:8080';

	const upstream = await fetch(`${apiBase}/api/v1/events/beds`, {
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
