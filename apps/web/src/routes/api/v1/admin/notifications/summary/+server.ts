import type { RequestHandler } from '@sveltejs/kit';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

const devHeaders = (): Record<string, string> => {
	if (process.env.SIGAP_DEV_IDENTITY === 'true') {
		return { 'X-Sigap-Dev-User-ID': 'admin-ui' };
	}
	return {};
};

async function proxy(request: Request, path: string, method?: string): Promise<Response> {
	const upstream = await fetch(`${apiBase()}${path}`, {
		method: method ?? request.method,
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json',
			...devHeaders()
		},
		body:
			request.method !== 'GET' && request.method !== 'HEAD'
				? await request.text()
				: undefined
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
	const path = `/api/v1/admin/notifications/summary${url.search}`;
	return proxy(request, path);
};
