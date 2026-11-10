import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { applySetCookie } from '$lib/server/auth';

export const load: PageServerLoad = () => {};

export const actions: Actions = {
	async default({ request, cookies, locals }) {
		if (locals.auth) {
			try {
				const { headers } = await locals.auth.api.signOut({
					headers: request.headers,
					returnHeaders: true
				});
				// Apply Better Auth's session-expiry Set-Cookie so the browser
				// drops the cookie (rather than only invalidating it server-side).
				applySetCookie(headers, cookies);
			} catch {
				// Fall back to clearing the default session cookie defensively.
				cookies.delete('better-auth.session_token', { path: '/' });
			}
		}
		redirect(303, '/');
	}
};