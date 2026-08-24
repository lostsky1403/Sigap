import type { RequestHandler } from '@sveltejs/kit';
import { apiBase } from '$lib/server/auth';

export const GET: RequestHandler = async ({ request }) => {
	const upstream = await fetch(`${apiBase()}/api/v1/public/service-units`, {
		method: 'GET',
		headers: {
			'Content-Type': request.headers.get('content-type') || 'application/json'
		}
	});
	const text = await upstream.text();
	return new Response(text, {
		status: upstream.status,
		headers: {
			'Content-Type': upstream.headers.get('content-type') || 'application/json'
		}
	});
};
