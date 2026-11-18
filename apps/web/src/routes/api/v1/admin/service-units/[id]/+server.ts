import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { proxyHeaders, authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

async function proxy(request: Request, path: string, event: RequestEvent, method?: string): Promise<Response> {
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

export const GET: RequestHandler = async (event) => {
	return proxy(event.request, `/api/v1/admin/service-units/${encodeURIComponent(event.params.id ?? '')}`, event);
};

export const PATCH: RequestHandler = async (event) => {
	return proxy(event.request, `/api/v1/admin/service-units/${encodeURIComponent(event.params.id ?? '')}`, event);
};
