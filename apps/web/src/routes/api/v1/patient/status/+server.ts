import type { RequestEvent, RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

async function proxy(request: Request, path: string, event: RequestEvent): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: request.method,
		headers: {
			'Content-Type': 'application/json',
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
	const url = new URL(event.request.url);
	const query = url.search;
	return proxy(event.request, `/api/v1/patient/status${query}`, event);
};
