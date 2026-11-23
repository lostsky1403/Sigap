import type { auth as BetterAuth, AuthSession } from '$lib/server/auth.better';

declare global {
	namespace App {
		interface Locals {
			auth: BetterAuth | null;
			/** The Better Auth session envelope, exactly as `auth.api.getSession` returns it. */
			session: AuthSession | null;
			/** The ES256 JWT minted by the Better Auth jwt plugin for the Go API. */
			jwt: string | null;
		}
	}
}

export {};