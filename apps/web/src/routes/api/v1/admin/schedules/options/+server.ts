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

/**
 * Schedule mutation options (Phase 3B5.0).
 *
 * This route exists so the browser can reach the options endpoint over the
 * same-origin proxy. That is the authorization model, not a convenience: the
 * proxy attaches the session cookie server-side, so no bearer token is ever
 * present in browser code and the client cannot be redirected into sending
 * credentials elsewhere. See $lib/server/auth.
 *
 * The route is a literal `options` segment, which SvelteKit matches in
 * preference to the sibling `[id]` route — so "options" can never be
 * mistaken for a schedule id on the way through, exactly as the Go router
 * orders its own dispatch.
 *
 * Only GET is exposed. The options endpoint is a read; the schedule mutations
 * live on the existing `/api/v1/admin/schedules` and `[id]` routes, which
 * keep their own method handlers and their own server-side authorization.
 */
export const GET: RequestHandler = async (event) => {
	return proxy(event.request, '/api/v1/admin/schedules/options', event);
};
