import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { auth } from '$lib/server/auth.better';
import { applySetCookie } from '$lib/server/auth';

/**
 * Tells the page, before the citizen types anything, whether signing in can
 * work at all. Without this the form renders normally and the failure only
 * appears after a submit. When Better Auth (self-hosted) is unconfigured — no
 * Postgres connection string for its `auth` schema — trying to sign in can
 * never succeed, so announcing that up front is honest.
 */
export const load: PageServerLoad = ({ locals }) => ({
	authConfigured: Boolean(locals.auth)
});

export const actions: Actions = {
	async default({ request, cookies, locals }) {
		if (!locals.auth) {
			return fail(503, { message: 'Autentikasi belum dikonfigurasi.' });
		}
		const data = await request.formData();
		const email = String(data.get('email') ?? '').trim();
		const password = String(data.get('password') ?? '');
		if (!email || !password) {
			return fail(400, { message: 'Email dan kata sandi wajib diisi.' });
		}

		try {
			const { headers } = await auth.api.signInEmail({
				body: { email, password },
				returnHeaders: true
			});
			// Persist Better Auth's session cookie (HttpOnly/SameSite preserved)
			// so hooks.server.ts can resolve it on the next request and forward
			// the bearer JWT to the Go API.
			applySetCookie(headers, cookies);
		} catch {
			return fail(401, { message: 'Email atau kata sandi salah.' });
		}
		redirect(303, '/');
	}
};