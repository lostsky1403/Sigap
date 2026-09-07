import { createServerClient } from '@supabase/ssr';
import type { SupabaseClient } from '@supabase/supabase-js';
import type { RequestEvent } from '@sveltejs/kit';

export interface SupabaseServerSession {
	client: SupabaseClient;
	applyPendingHeaders: (response: Response) => void;
}

/**
 * Server-side Supabase client backed by SvelteKit cookies (official
 * @supabase/ssr getAll/setAll pattern).
 *
 * The Supabase project URL and ANON (public) key are publishable values only —
 * the service-role key is never used in the web app. Configuration is read at
 * runtime so the same image works in dev and production.
 *
 * Token refreshes and sign-in/sign-out write cookies plus anti-CDN-cache
 * headers. Cookie writes go to `event.cookies`; the cache headers are applied
 * to the outgoing response after `resolve` via `applyPendingHeaders`.
 */
export function createSupabaseServerClient(event: RequestEvent): SupabaseServerSession {
	const pendingHeaders: Record<string, string> = {};

	const client = createServerClient(
		process.env.PUBLIC_SUPABASE_URL as string,
		process.env.PUBLIC_SUPABASE_ANON_KEY as string,
		{
			cookies: {
				getAll: () => event.cookies.getAll(),
				setAll: (cookies, headers) => {
					for (const [name, value] of Object.entries(headers)) {
						pendingHeaders[name] = value;
					}
					for (const { name, value, options } of cookies) {
						event.cookies.set(name, value, { path: '/', ...options });
					}
				}
			}
		}
	);

	return {
		client,
		applyPendingHeaders: (response: Response) => {
			for (const [name, value] of Object.entries(pendingHeaders)) {
				if (!response.headers.has(name)) {
					response.headers.set(name, value);
				}
			}
		}
	};
}
