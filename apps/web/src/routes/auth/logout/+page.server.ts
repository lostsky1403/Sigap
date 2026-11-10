import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = () => {};

export const actions: Actions = {
	async default({ request, locals }) {
		if (locals.auth) {
			try {
				// The sveltekitCookies plugin clears the session cookie via SvelteKit's
				// cookies API; no manual Set-Cookie handling needed.
				await locals.auth.api.signOut({ headers: request.headers });
			} catch {
				// Fall back to clearing the default session cookie defensively.
			}
		}
		redirect(303, '/');
	}
};