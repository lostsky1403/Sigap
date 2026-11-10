// apps/web/src/lib/server/auth.better.ts
//
// Better Auth server instance (self-hosted identity). Replaces Supabase Auth as
// the web app's credential store + JWT issuer while reusing the SAME SIGAP
// Postgres. Better Auth's tables are isolated into the `auth` schema (NOT the
// SIGAP `public` schema or migration ledger).
//
// The `jwt` plugin is pinned to ES256 (ECDSA P-256) and serves a JWKS at
// `${baseURL}/api/auth/jwks`, so the existing Go JWT trust boundary
// (`apps/api/internal/auth/jwt_provider.go`) can verify these tokens with NO
// code change. Do NOT switch back to the EdDSA (`OKP`) default — the Go
// `parseJWK` only accepts `RSA`/`EC` keys.
//
// Notes:
// - No auto-migration: Better Auth does not create tables at runtime. Run the
//   migration explicitly (see `design/better-auth-schema.md`) before first use.
// - No hardcoded secret: Better Auth auto-reads `BETTER_AUTH_SECRET` from the
//   environment at construction time.

import { betterAuth } from "better-auth";
import { jwt } from "better-auth/plugins";
import { sveltekitCookies } from "better-auth/svelte-kit";
import { getRequestEvent } from "$app/server";
import { PostgresDialect } from "kysely";
import { databaseConfigured, getPool } from "./db";

/**
 * True when a Postgres connection string is configured, i.e. the Better Auth
 * instance can actually reach its schema. Pages and `+server.ts` proxies can
 * call this to announce unavailability instead of failing at request time.
 */
export function betterAuthConfigured(): boolean {
	return databaseConfigured();
}

export const auth = betterAuth({
	// BETTER_AUTH_SECRET is required. It is set by the runtime (Run-V22Service.ps1
	// reads .env before serving); `vite build` evaluates this module during SSR
	// prerender with no env, so a clearly-marked build placeholder avoids Better
	// Auth's default-secret error. The placeholder is never used to sign real
	// sessions — the server always supplies the real secret at runtime.
	secret: process.env.BETTER_AUTH_SECRET ?? "sigap-build-only-placeholder-secret",
	baseURL: process.env.BETTER_AUTH_URL || "http://127.0.0.1:5173",
	database: {
		dialect: new PostgresDialect({ pool: getPool()! }),
		type: "postgres",
		schemaName: "auth",
	},
	emailAndPassword: {
		enabled: true,
	},
	rateLimit: {
		enabled: true,
		window: 60,
		max: 20,
		storage: "memory"
	},
	plugins: [
		// Route Better Auth's session cookie through SvelteKit's `cookies` API
		// (via getRequestEvent), so sign-in/sign-up/sign-out set/clear the cookie
		// correctly with no hand-parsing and no double URL-encoding.
		sveltekitCookies(getRequestEvent),
		jwt({
			jwks: {
				keyPairConfig: { alg: "ES256" },
			},
			jwt: {
				issuer: "http://127.0.0.1:5173",
				audience: "authenticated",
				expirationTime: "1h",
			},
		}),
	],
});

/**
 * The session envelope `locals.session` carries, named here so consumers import
 * it rather than re-deriving `Awaited<ReturnType<...>>`.
 */
export type AuthSession = Awaited<ReturnType<typeof auth.api.getSession>>;