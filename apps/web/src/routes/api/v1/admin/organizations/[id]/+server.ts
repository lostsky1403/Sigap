import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { proxyHeaders, apiBase, authHeaders } from '$lib/server/auth';

async function proxy(
	request: Request,
	path: string,
	event: RequestEvent,
	method?: string
): Promise<Response> {
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

/**
 * The organization overview read. `GetOrganizationOverview` re-resolves the
 * caller's organization scope and facility read-set on every request and
 * answers 404 for a forged or out-of-scope org id, so the 404 (not the client)
 * is what keeps siblings from seeing each other's organizations.
 */
export const GET: RequestHandler = async (event) => {
	return proxy(
		event.request,
		`/api/v1/admin/organizations/${encodeURIComponent(event.params.id ?? '')}`,
		event
	);
};