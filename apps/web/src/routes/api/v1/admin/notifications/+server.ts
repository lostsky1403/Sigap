import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { proxyHeaders, apiBase, authHeaders } from '$lib/server/auth';

async function proxy(request: Request, path: string, event: RequestEvent, method?: string): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: method ?? request.method,
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json',
			...proxyHeaders(),
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

// The Go ListNotifications handler accepts limit, facility_id, status, channel,
// template_key, created_from, and created_to. The admin notifications page sends
// all of them, so the query string must be forwarded or every filter is a silent
// no-op. facility_id remains defense-in-depth only: the API honors it solely
// after it passes the scope and notification.read provenance intersection.
export const GET: RequestHandler = async (event) => {
	const query = new URL(event.request.url).search;
	return proxy(event.request, `/api/v1/admin/notifications${query}`, event);
};

export const POST: RequestHandler = async (event) => {
	return proxy(event.request, '/api/v1/admin/notifications', event);
};
