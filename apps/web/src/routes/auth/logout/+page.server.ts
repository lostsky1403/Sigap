import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = () => {};

export const actions: Actions = {
	async logout({ locals }) {
		if (locals.supabase) {
			await locals.supabase.auth.signOut();
		}
		redirect(303, '/');
	}
};
