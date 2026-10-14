import type { RequestHandler } from '@sveltejs/kit';
import { apiBase, authHeaders } from '$lib/server/auth';

// ListPublicServiceUnits accepts facility_id to narrow the catalog. Forwarding
// the query string keeps the proxy contract aligned with the API so a future
// facility filter is not silently dropped. It is a public, unauthenticated
// catalog filter and carries no authorization meaning.
export const GET: RequestHandler = async (event) => {
	const query = new URL(event.request.url).search;
	const upstream = await fetch(`${apiBase()}/api/v1/public/service-units${query}`, {
		method: 'GET',
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
};
