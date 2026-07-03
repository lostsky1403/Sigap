import type { RequestHandler } from '@sveltejs/kit';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

async function proxy(request: Request, path: string): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: request.method,
		headers: {
			'Content-Type': 'application/json'
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

export const GET: RequestHandler = async ({ request }) => {
	const url = new URL(request.url);
	const query = url.search;
	return proxy(request, `/api/v1/patient/status${query}`);
};
