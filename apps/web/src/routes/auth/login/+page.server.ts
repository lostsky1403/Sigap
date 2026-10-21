import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

/**
 * Tells the page, before the citizen types anything, whether signing in can
 * work at all.
 *
 * Without this the form renders normally and the failure only appears after a
 * submit, as "Masuk gagal karena terjadi gangguan pada layanan. Silakan coba
 * lagi dalam beberapa saat." — which describes a transient outage. It is not
 * transient: Supabase is simply not configured in this environment, and
 * retrying will never work. Telling someone to try again later when the action
 * cannot succeed is worse than telling them up front.
 */
export const load: PageServerLoad = ({ locals }) => ({
	authConfigured: Boolean(locals.supabase)
});

export const actions: Actions = {
	async default({ request, locals }) {
		if (!locals.supabase) {
			return fail(503, { message: 'Autentikasi belum dikonfigurasi.' });
		}
		const data = await request.formData();
		const email = String(data.get('email') ?? '').trim();
		const password = String(data.get('password') ?? '');
		if (!email || !password) {
			return fail(400, { message: 'Email dan kata sandi wajib diisi.' });
		}
		const { error } = await locals.supabase.auth.signInWithPassword({ email, password });
		if (error) {
			return fail(401, { message: 'Email atau kata sandi salah.' });
		}
		redirect(303, '/');
	}
};
