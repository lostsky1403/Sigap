import type { LayoutServerLoad } from './$types';

/**
 * Root layout data.
 *
 * Two fields, both safe to serialize into the client bundle:
 *
 * - `userEmail` — pre-existing, used by the header to render the signed-in
 *   identity versus the sign-in link.
 * - `hasSession` — a boolean, used to choose between the "please sign in" and
 *   "you may not" presentations when the admin auth layer answers 403 for both.
 *
 * What is deliberately absent is the security-relevant set: the bearer token,
 * the user id, the role, the permission list, the facility scope, and the
 * super_admin flag. The Go API resolves all of those from the database on every
 * request (Phase 3B0), and they must never reach the browser — a client that held
 * a facility scope or a super_admin flag would be an authorization authority,
 * and an authorization authority is something a user can edit.
 *
 * `hasSession` is included precisely because it is the one fact the client
 * cannot otherwise infer, and it is safe: it reveals that a cookie exists, and
 * nothing about what that cookie can do.
 */
export const load: LayoutServerLoad = ({ locals }) => {
	return {
		userEmail: locals.session?.user?.email ?? null,
		// Derived from the same session object the proxies use, so the header and
		// the 403 presentation can never disagree about whether anyone is signed in.
		hasSession: Boolean(locals.session?.access_token)
	};
};
