import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { authHeaders, proxyHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

// Proxy GET /api/v1/patient/appointments to the Go API.
//
// This is the ACCOUNT-scoped counterpart to the code-keyed /patient/status
// proxy. Both the session bearer token (authHeaders) and the local E2E actor
// selector (proxyHeaders — a no-op outside SIGAP_ENV=local) are forwarded, so
// the Go API can resolve the citizen's internal patient identity either from a
// real session or from the synthetic seeded citizen. Without one, the upstream
// answers 401 and the page renders its sign-in state rather than a list.
async function proxy(request: Request, path: string, event: RequestEvent): Promise<Response> {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const upstream = await fetch(`${resolved.base}${path}`, {
		method: request.method,
		headers: {
			'Content-Type': 'application/json',
			...proxyHeaders(),
			...authHeaders(event)
		}
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
	return proxy(event.request, '/api/v1/patient/appointments', event);
};