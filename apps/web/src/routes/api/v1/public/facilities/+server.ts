import type { RequestHandler } from '@sveltejs/kit';
import { apiBase, authHeaders } from '$lib/server/auth';

export const GET: RequestHandler = async (event) => {
	const upstream = await fetch(`${apiBase()}/api/v1/public/facilities`, {
		method: 'GET',
		headers: {
			'Content-Type': event.request.headers.get('content-type') || 'application/json',
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
};
