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

// The queue board's service-day scope rides on `?date=YYYY-MM-DD` (WIB). The
// query string MUST be forwarded: without it the API silently applies its today
// default and a historical date renders the wrong (all-today) rows — the same
// no-op the notifications and patient-status proxies explicitly avoid.
export const GET: RequestHandler = async (event) => {
	const query = new URL(event.request.url).search;
	return proxy(event.request, `/api/v1/admin/queues${query}`, event);
};
