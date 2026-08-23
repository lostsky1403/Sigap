import type { RequestHandler } from '@sveltejs/kit';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

async function proxy(request: Request, path: string): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: request.method,
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json'
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

export const POST: RequestHandler = async ({ params, request }) => {
	return proxy(request, `/api/v1/appointments/${encodeURIComponent(params.id ?? '')}/check-in`);
};
