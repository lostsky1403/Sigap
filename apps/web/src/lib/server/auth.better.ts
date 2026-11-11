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
//   migration explicitly (see `design/better-auth-schema.md`) before first use —
//   and RE-RUN it after this change, because the `twoFactor()` plugin adds the
//   `auth.twoFactor` table and `auth.user.twoFactorEnabled`.
// - No hardcoded secret: Better Auth auto-reads `BETTER_AUTH_SECRET` from the
//   environment at construction time.
// - Transactional mail (verification + password reset) goes through
//   `./mail.ts`, which CAPTURES to `.local-preview/mail.log` for local dev.
//   There is no SMTP transport; see `design/email-recovery-mfa.md`.
// - `twoFactor()` adds TOTP + backup codes for privileged staff. The MFA
//   endpoints answer DB errors until the migration above is applied (reads are
//   unaffected: the adapter does `select *`, so existing sign-in keeps working).

import { betterAuth, type BetterAuthOptions } from "better-auth";
import { jwt, twoFactor } from "better-auth/plugins";
import { sveltekitCookies } from "better-auth/svelte-kit";
import { getRequestEvent } from "$app/server";
import { PostgresDialect } from "kysely";
import { databaseConfigured, getPool } from "./db";
import { sendMail } from "./mail";

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
	// -------------------------------------------------------------------------
	// Email + password: sign-up, email recovery, and the anti-enumeration
	// contract that both recovery surfaces answer with.
	//
	// `autoSignIn: false` is load-bearing beyond "make them sign in after
	// registering": better-auth 1.7.20 returns its GENERIC duplicate-signup
	// response (a synthetic user, with the password hashed anyway to equalize
	// timing) only when `requireEmailVerification` OR `autoSignIn === false`
	// holds — otherwise it throws 422 USER_ALREADY_EXISTS. `svelteKitHandler`
	// exposes `/api/auth/sign-up/email` directly, so without this the raw
	// endpoint would answer "this email is taken" to anyone who asks. The
	// SvelteKit register action already maps every failure to one generic
	// message; this closes the same hole at the API surface.
	//
	// `requireEmailVerification` is env-gated rather than hardcoded true:
	// enforcing verification before a transport exists would lock every account
	// out (mail is capture-only, see `./mail.ts`). The switch exists so the
	// policy can be turned on by config the moment real delivery does.
	emailAndPassword: {
		enabled: true,
		autoSignIn: false,
		requireEmailVerification: process.env.SIGAP_REQUIRE_EMAIL_VERIFICATION === "true",
		// The reset link carries a one-hour token (pinned; library default is the
		// same) and is single-use — better-auth consumes the `verification` row
		// before the new password is written, so a replay after a successful
		// reset fails with INVALID_TOKEN.
		resetPasswordTokenExpiresIn: 60 * 60,
		// A reset is the response to a compromised credential, so it must also end
		// every OTHER session. In better-auth 1.7.20 the sequence is: spend the
		// single-use token, delete the user's sessions, then write the new
		// password — one transaction when the adapter supports one. This config
		// does NOT set `database.transaction`, so the honest statement is
		// "sequential, sessions first": a failure after revocation leaves the old
		// password intact and every session signed out (never a new password
		// alongside still-valid sessions).
		revokeSessionsOnPasswordReset: true,
		// The one place a reset link is handed out. The request endpoint answers
		// `{status:true}` with the same message for known and unknown addresses
		// (it runs a dummy id + verification lookup first to flatten timing), so
		// this callback is the only difference between the two paths and must
		// never throw or leak — `sendMail` is total by construction.
		sendResetPassword: async ({ user, url }) => {
			await sendMail({
				kind: "password-reset",
				to: user.email,
				subject: "Atur ulang kata sandi SIGAP",
				text: [
					"Halo,",
					"",
					"Permintaan atur ulang kata sandi diterima untuk akun ini.",
					`Tautan berlaku satu jam dan hanya dapat dipakai sekali: ${url}`,
					"",
					"Jika Anda tidak meminta ini, abaikan surel ini — kata sandi tidak berubah."
				].join("\n")
			});
		},
	},
	// -------------------------------------------------------------------------
	// Email verification. `sendOnSignUp: true` means every new account gets a
	// verification mail (captured to `.local-preview/mail.log` locally). The
	// token is a signed HS256 JWT with a one-hour `exp` — no `verification` row —
	// and `GET /api/auth/verify-email` is idempotent: replaying a link after the
	// address is already verified returns `{status:true, user:null}` instead of
	// re-verifying or erroring. `autoSignInAfterVerification` stays false on
	// purpose: possessing the link must not mint a session, so a forwarded or
	// logged email cannot become a sign-in. `sendOnSignIn` stays false so an
	// unverified address cannot be used to pump mail at will.
	emailVerification: {
		sendOnSignUp: true,
		sendOnSignIn: false,
		autoSignInAfterVerification: false,
		expiresIn: 60 * 60,
		sendVerificationEmail: async ({ user, url }) => {
			await sendMail({
				kind: "email-verification",
				to: user.email,
				subject: "Verifikasi alamat surel SIGAP",
				text: [
					"Halo,",
					"",
					"Konfirmasi alamat surel ini dengan membuka tautan berikut:",
					url,
					"",
					"Tautan berlaku satu jam. Abaikan surel ini jika Anda tidak mendaftar."
				].join("\n")
			});
		},
	},
	// -------------------------------------------------------------------------
	// Session lifetime + authorization freshness.
	//
	// `design/session-lifecycle-policy.md` is normative for these three clocks;
	// they are pinned here explicitly (rather than inherited) so the policy and
	// the code cannot drift. The first two restate the library defaults; the
	// third TIGHTENS the default by 24x.
	//
	//   expiresIn  7 d — idle ceiling. `expiresAt` is stamped now + this on every
	//                    sign-in and on every refresh below; past it, the session
	//                    row is expired and `getSession` answers null.
	//   updateAge  1 d — refresh cadence. A session in use at least once a day has
	//                    `expiresAt` rolled forward (that write extends the idle
	//                    ceiling, not the absolute lifetime).
	//   freshAge   1 h — the "fresh session" window for sensitive endpoints. Better
	//                    Auth measures it from `session.createdAt`, and the refresh
	//                    above never rolls that column, so it means literally
	//                    "signed in within the last hour" — not "active within the
	//                    last hour". A stale session must re-authenticate.
	//                    In better-auth 1.7.20 the gate applies to
	//                    `GET /list-sessions` and to `POST /delete-user` when no
	//                    password is supplied; `POST /change-password` requires the
	//                    CURRENT password instead (its own re-authentication), so
	//                    it is deliberately not freshness-bound.
	session: {
		expiresIn: 60 * 60 * 24 * 7,
		updateAge: 60 * 60 * 24,
		freshAge: 60 * 60,
	},
	// Self-service password change is part of the account lifecycle and is
	// covered by tests/session-lifecycle.test.js. In better-auth 1.7.20 the
	// `POST /change-password` endpoint is mounted unconditionally and the
	// documented `user.changePassword.enabled` key is NOT read (absent from
	// `BetterAuthOptions` too), so this declaration is the intent of record: it
	// takes effect again if a later version reinstates the gate, and the
	// required-current-password contract is asserted in the test file. The
	// assertion is the narrowest form that keeps every other option exactly typed.
	user: {
		changePassword: { enabled: true },
	} as NonNullable<BetterAuthOptions["user"]>,
	rateLimit: {
		enabled: true,
		window: 60,
		max: 20,
		storage: "memory"
	},
	plugins: [
		jwt({
			jwks: {
				keyPairConfig: { alg: "ES256" },
			},
			jwt: {
				issuer: "http://127.0.0.1:5173",
				audience: "authenticated",
				// The access-token TTL is also the MAXIMUM authorization freshness
				// delay for a revoked session: the Go API verifies the JWT alone, so
				// a revoked session keeps being accepted (as `sub`) until this `exp`.
				// 1h is the accepted bound for privileged staff — see
				// `design/session-lifecycle-policy.md` §maximum revocation delay for
				// the reasoning, the compensating control (DB RBAC is re-resolved on
				// every API request), and the levers for shortening it.
				expirationTime: "1h",
			},
		}),
		// TOTP second factor for privileged staff. Chosen over the OTP factor
		// because OTP needs a delivery channel and SDS/SMS is NOT configured (no
		// paid service is allowed here), while TOTP needs nothing but the shared
		// secret in `auth.twoFactor`. The enrollment QR/URI is generated locally
		// (`/two-factor/get-totp-uri`), each code is a 30-second 6-digit TOTP, and
		// backup codes are 10 single-use codes stored ENCRYPTED (AES-GCM via
		// better-auth's secret config — the library default, pinned here so the
		// policy cannot drift). Lifecycle, recovery and what is unverified:
		// `design/email-recovery-mfa.md`.
		twoFactor({
			issuer: "SIGAP",
			// 30 s / 6 digits = RFC 6238 defaults; pinned so every enrolled
			// authenticator app agrees and a future default change cannot silently
			// invalidate already-enrolled staff.
			totpOptions: { period: 30, digits: 6 },
			backupCodeOptions: { amount: 10, length: 10, storeBackupCodes: "encrypted" },
			// 10 minutes to finish a second-factor challenge after the password step
			// (library default, pinned): long enough for a phone, short enough that a
			// stolen `two_factor` cookie expires before it is useful.
			twoFactorCookieMaxAge: 10 * 60,
			// Trusted-device opt-in stays at the library default of 30 days; note it
			// is the longest-lived credential in this config and staff should not use
			// it on shared equipment.
			trustDeviceMaxAge: 30 * 24 * 60 * 60,
			// Account-level lockout across all factors: 10 consecutive failed
			// second-factor attempts lock the account for 15 minutes. Complements the
			// transport rate limit (the plugin adds 3 requests / 10 s for
			// `/two-factor/*`), which alone cannot stop a slow brute force.
			accountLockout: { enabled: true, maxFailedAttempts: 10, durationSeconds: 15 * 60 },
		}),
		// LAST, per the Better Auth SvelteKit integration: it must observe every
		// other plugin's cookie writes to route them through SvelteKit's `cookies`
		// API (getRequestEvent) — no hand-parsing, no double URL-encoding.
		sveltekitCookies(getRequestEvent),
	],
});

/**
 * The session envelope `locals.session` carries, named here so consumers import
 * it rather than re-deriving `Awaited<ReturnType<...>>`.
 */
export type AuthSession = Awaited<ReturnType<typeof auth.api.getSession>>;