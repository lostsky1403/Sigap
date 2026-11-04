import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

// Proxy POST /api/v1/appointments/cancel to the Go API.
//
// Same pattern as the check-in proxy: resolve the upstream once through
// resolveApiBase (which refuses to guess a host), forward the body unchanged,
// and preserve the upstream status/content-type so the client's error model
// can distinguish 404 (not found), 409 (past cancellable window) and 429.
async function proxy(request: Request, path: string, event: RequestEvent): Promise<Response> {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const upstream = await fetch(`${resolved.base}${path}`, {
		method: request.method,
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json',
			...authHeaders(event)
		},
		body: request.method !== 'GET' && request.method !== 'HEAD' ? await request.text() : undefined
	});
	const text = await upstream.text();
	return new Response(text, {
		status: upstream.status,
		headers: {
			'Content-Type': upstream.headers.get('content-type') || 'application/json'
		}
	});
}

export const POST: RequestHandler = async (event) => {
	return proxy(event.request, '/api/v1/appointments/cancel', event);
};