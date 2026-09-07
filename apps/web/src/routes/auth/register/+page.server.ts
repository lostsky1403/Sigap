import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';

export const actions: Actions = {
	async register({ request, locals }) {
		if (!locals.supabase) {
			return fail(503, { message: 'Autentikasi belum dikonfigurasi.' });
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
		const { error } = await locals.supabase.auth.signUp({ email, password });
		if (error) {
			return fail(400, { message: error.message });
		}
		redirect(303, '/auth/login?registered=1');
	}
};
