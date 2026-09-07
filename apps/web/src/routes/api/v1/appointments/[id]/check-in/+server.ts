import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

async function proxy(request: Request, path: string, event: RequestEvent): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
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
	return proxy(event.request, `/api/v1/appointments/${encodeURIComponent(event.params.id ?? '')}/check-in`, event);
};
