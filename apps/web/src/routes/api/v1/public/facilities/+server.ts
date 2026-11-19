import type { RequestHandler } from '@sveltejs/kit';
import { authHeaders } from '$lib/server/auth';
import { resolveApiBase } from '$lib/server/upstream';

export const GET: RequestHandler = async (event) => {
	const resolved = resolveApiBase();
	if ('unavailable' in resolved) return resolved.unavailable;

	const upstream = await fetch(`${resolved.base}/api/v1/public/facilities`, {
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
