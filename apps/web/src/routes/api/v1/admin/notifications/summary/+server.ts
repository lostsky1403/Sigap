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

// GetNotificationSummary accepts facility_id and narrows the aggregate to it.
// Other notification filters are not part of the summary contract, so
// forwarding the whole query string is safe: the API ignores what it does not
// define and still authorizes facility_id against the live read set.
export const GET: RequestHandler = async (event) => {
	const query = new URL(event.request.url).search;
	return proxy(event.request, `/api/v1/admin/notifications/summary${query}`, event);
};
