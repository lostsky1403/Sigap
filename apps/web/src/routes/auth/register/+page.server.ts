import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { auth } from '$lib/server/auth.better';
import { allowRegister } from '$lib/server/rate-limit';

/** See the note in auth/login/+page.server.ts: announce this before typing. */
export const load: PageServerLoad = ({ locals }) => ({
	authConfigured: Boolean(locals.auth)
});

export const actions: Actions = {
	async default(event) {
		const { request, locals } = event;
		if (!locals.auth) {
			return fail(503, { message: 'Autentikasi belum dikonfigurasi.' });
		}
		if (!allowRegister(event.getClientAddress())) {
			return fail(429, { message: 'Terlalu banyak percobaan. Coba lagi nanti.' });
		}
		const data = await request.formData();
		const email = String(data.get('email') ?? '').trim();
		const password = String(data.get('password') ?? '');
		const confirm = String(data.get('confirm') ?? '');
		if (!email || !password) {
			return fail(400, { message: 'Email dan kata sandi wajib diisi.' });
		}
		if (password.length < 8) {
			return fail(400, { message: 'Kata sandi minimal 8 karakter.' });
		}
		if (password !== confirm) {
			return fail(400, { message: 'Konfirmasi kata sandi tidak sama.' });
		}

		// Better Auth's signUpEmail requires a name; the SIGAP registration form
		// deliberately collects only email/password, so derive a stable display
		// name from the email local-part. This is a placeholder, not a verified
		// identity (see design/identity-mapping.md).
		const name = email.split('@')[0];
		try {
			await auth.api.signUpEmail({
				body: { email, password, name }
			});
		} catch (err) {
			// A race (already registered) or validation failure; keep the user
			// on the form rather than leaking which email is taken.
			return fail(400, { message: 'Pendaftaran gagal. Silakan coba lagi.' });
		}
		redirect(303, '/auth/login?registered=1');
	}
};