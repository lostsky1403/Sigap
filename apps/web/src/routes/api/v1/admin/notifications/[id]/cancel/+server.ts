import type { RequestHandler } from '@sveltejs/kit';
import { error } from '@sveltejs/kit';

const apiBase = () => process.env.SIGAP_API_INTERNAL || 'http://api:8080';

const devHeaders = (): Record<string, string> => {
	if (process.env.SIGAP_DEV_IDENTITY === 'true') {
		return { 'X-Sigap-Dev-User-ID': 'admin-ui' };
	}
	return {};
};

// POST /api/v1/admin/notifications/[id]/cancel → POST /api/v1/admin/notifications/{id}/cancel
export const POST: RequestHandler = async ({ request, params }) => {
	const id = params.id;
	if (!id || !/^[0-9a-f-]{36}$/i.test(id)) {
		throw error(400, 'invalid notification id');
	}
	const upstream = await fetch(`${apiBase()}/api/v1/admin/notifications/${id}/cancel`, {
		method: 'POST',
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json',
			...devHeaders()
		},
		body: await request.text()
	});
	const text = await upstream.text();
	return new Response(text, {
		status: upstream.status,
		headers: {
			'Content-Type': upstream.headers.get('content-type') || 'application/json'
		}
	});
};