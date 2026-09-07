import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';

export const actions: Actions = {
	async signIn({ request, locals }) {
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
