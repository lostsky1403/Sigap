import type { LayoutServerLoad } from './$types';

// Expose the signed-in user's email (or null) to the layout so the header
// can render login/register versus the user's email + logout. The Supabase
// session is refreshed in hooks.server.ts; no token is surfaced to the client.
export const load: LayoutServerLoad = ({ locals }) => {
	return {
		userEmail: locals.session?.user?.email ?? null
	};
};
