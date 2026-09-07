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

export const GET: RequestHandler = async (event) => {
	return proxy(event.request, '/api/v1/admin/service-units', event);
};

export const POST: RequestHandler = async (event) => {
	return proxy(event.request, '/api/v1/admin/service-units', event);
};
