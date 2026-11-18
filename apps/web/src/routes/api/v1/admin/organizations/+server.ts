import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { proxyHeaders, authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

async function proxy(
	request: Request,
	path: string,
	event: RequestEvent,
	method?: string
): Promise<Response> {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const upstream = await fetch(`${resolved.base}${path}`, {
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

/**
 * The organization directory read. The scope is resolved server-side by
 * `ListOrganizations`, so this proxy forwards the request and every bit of the
 * authorization decision stays upstream. It takes no query parameters in this
 * pilot, but the query string is forwarded anyway, symmetric with the queues
 * proxy, so a future scope/pagination parameter is never silently dropped.
 */
export const GET: RequestHandler = async (event) => {
	const query = new URL(event.request.url).search;
	return proxy(event.request, `/api/v1/admin/organizations${query}`, event);
};