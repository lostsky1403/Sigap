import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { auth, betterAuthConfigured } from '$lib/server/auth.better';
import { applySetCookie } from '$lib/server/auth';
import { allowLogin } from '$lib/server/rate-limit';

/**
 * Tells the page, before the citizen types anything, whether signing in can
 * work at all. When Better Auth (self-hosted) is unconfigured — no Postgres
 * connection string for its `auth` schema — trying to sign in can never
 * succeed, so announcing that up front is honest.
 */
export const load: PageServerLoad = () => ({
	authConfigured: betterAuthConfigured()
});

export const actions: Actions = {
	async default(event) {
		const { request, cookies, locals } = event;
		if (!locals.auth) {
			return fail(503, { message: 'Autentikasi belum dikonfigurasi.' });
		}
		if (!allowLogin(event.getClientAddress())) {
			return fail(429, { message: 'Terlalu banyak percobaan. Coba lagi nanti.' });
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
} catch (err) {
			// Distinguish a genuine credential rejection (401) from an auth-store
			// outage: telling someone their password is wrong while the service is
			// down is the exact misdiagnosis this action must avoid.
			let status: unknown;
			if (err && typeof err === 'object' && 'status' in err) {
				status = err.status;
			}
			if (status === 401) {
				return fail(401, { message: 'Email atau kata sandi salah.' });
			}
			return fail(503, { message: 'Layanan sedang bermasalah. Silakan coba lagi.' });
		}
		redirect(303, '/');
	}
};