import type { RequestHandler } from '@sveltejs/kit';
import { proxyHeaders, apiBase } from '$lib/server/auth';

async function proxy(request: Request, path: string, method?: string): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: method ?? request.method,
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json',
			...proxyHeaders()
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

export const POST: RequestHandler = async ({ request }) => {
	return proxy(request, '/api/v1/admin/notifications/ID/cancel');
};
