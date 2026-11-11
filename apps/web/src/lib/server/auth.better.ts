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
// - No hardcoded secret, and no silent fallback either. Better Auth auto-reads
//   `BETTER_AUTH_SECRET` from the environment; `resolveSecret()` below allows
//   the public build placeholder ONLY where it cannot sign a real session
//   (during `vite build`/prerender, or on a non-production launch) and THROWS
//   otherwise. A production server that misses the env fails closed at module
//   construction instead of signing real sessions with a public constant.
// - `jwt.issuer`/`jwt.audience` come from `SIGAP_AUTH_ISSUER` /
//   `SIGAP_AUTH_AUDIENCE` — the SAME names the Go JWT trust boundary reads —
//   so an issuer mismatch is a configuration change in one place, not a code
//   edit on one side only.
// - Every new `auth.user` is provisioned with its own ROLELESS `app_users` row
//   by the `databaseHooks.user.create.after` hook below; without it a new
//   citizen's `sub` resolves to no `AppUserID` (see the hook's comment).
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
import { building } from "$app/environment";
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

/**
 * The build-placeholder secret. It is a public constant: any deployment that
 * reaches runtime with it signs REAL sessions and encrypts TOTP backup codes
 * (the `twoFactor` plugin's `storeBackupCodes: "encrypted"`) with a value
 * printed in this repository. It exists for exactly one situation — the
 * SvelteKit build, which evaluates this module during SSR prerender with no
 * runtime env, where `betterAuth()` would otherwise refuse to construct.
 */
const BUILD_ONLY_SECRET = "sigap-build-only-placeholder-secret";

/**
 * The session-signing secret, resolved fail-closed.
 *
 * `BETTER_AUTH_SECRET` wins whenever it is set. Otherwise the build placeholder
 * is allowed ONLY where no real session can be signed with it:
 *
 *   - `building` — SvelteKit's `vite build` / prerender pass, where the module
 *     is evaluated before any runtime env exists (this is what the placeholder
 *     was always for); or
 *   - a non-production launch (`NODE_ENV !== "production"`) — `vite dev`, and
 *     scripted local probes.
 *
 * Any other launch — a container, `node build/index.js`, CI — that misses the
 * env THROWS here, at module construction, before a single request is served.
 * That is the only fail-closed point available: Better Auth reads the secret
 * once and every later "is it the placeholder?" check would be a race against
 * already-issued tokens.
 */
function resolveSecret(): string {
	const secret = process.env.BETTER_AUTH_SECRET;
	if (secret) return secret;
	if (building || process.env.NODE_ENV !== "production") return BUILD_ONLY_SECRET;
	throw new Error(
		"BETTER_AUTH_SECRET is not set: refusing to construct Better Auth on a production server " +
			"(NODE_ENV=production, outside the build) because it would sign real sessions and " +
			"encrypt 2FA backup codes with the public build placeholder secret. Set " +
			"BETTER_AUTH_SECRET in the server environment (.env, or the service unit that launches " +
			"build/index.js) and restart."
	);
}

/**
 * The `app_users.display_name` for a newly registered citizen.
 *
 * The SIGAP registration form collects only email + password, so the name
 * Better Auth stores is the email local-part (see `register/+page.server.ts`).
 * Prefer the auth user's own `name`, fall back to the local-part, and never
 * return an empty string — `display_name` is the only human label a citizen
 * gets until they edit their profile.
 */
function citizenDisplayName(user: { name?: unknown; email?: unknown }): string {
	const name = typeof user.name === "string" ? user.name.trim() : "";
	if (name) return name;
	const email = typeof user.email === "string" ? user.email : "";
	const localPart = email.split("@")[0]?.trim() ?? "";
	return localPart || email || "Pengguna SIGAP";
}

export const auth = betterAuth({
	// See `resolveSecret` above: real secret, or a hard failure.
	secret: resolveSecret(),
	baseURL: process.env.BETTER_AUTH_URL || "http://127.0.0.1:5173",
	database: {
		dialect: new PostgresDialect({ pool: getPool()! }),
		type: "postgres",
		schemaName: "auth",
	},
	// -------------------------------------------------------------------------
	// Citizen provisioning: every new `auth.user` gets its OWN SIGAP identity.
	//
	// `register/+page.server.ts` calls `signUpEmail`, which writes only
	// `auth.user`. With no `app_users` row, the token's `sub` resolves to no
	// `AppUserID` (`apps/api/internal/auth/rbac_resolver.go`: the lookup is
	// `WHERE u.subject = $1 AND u.status = 'active' AND u.deleted_at IS NULL`),
	// so `/patient/appointments` answers 401 and a booking would be written with
	// `patient_account_id = NULL`. This hook closes that gap: it inserts the
	// ROLELESS `app_users` row a citizen needs. Zero `user_roles` rows is what
	// makes `Actor.PatientAccountID()` return the new id (grants would demote the
	// actor to staff and return ""), so nothing is written to `user_roles` —
	// never, here.
	//
	// INSERT ONLY — never a match against an existing row, by email or otherwise
	// (`design/identity-mapping.md` §3: email is a self-asserted, transferable
	// claim while `sub` is the token-proven identity, so an email auto-link is an
	// account-takeover primitive). `subject` is the only join key and this is the
	// only place it is written for a citizen.
	//
	// Idempotent by construction: the partial unique index
	// `uq_app_users_subject_active` (0008) plus `ON CONFLICT DO NOTHING` means a
	// replay for the same auth user changes nothing and can never produce a
	// second active row for one subject.
	databaseHooks: {
		user: {
			create: {
				// Runs after the `auth.user` write (better-auth 1.7.20 queues it
				// after-commit when a transaction is configured; this config does not
				// set `database.transaction`, so it runs inline). A failure here
				// propagates to the sign-up caller on purpose: an account that cannot
				// resolve to a SIGAP identity is not a usable registration.
				after: async (user) => {
					const pool = getPool();
					if (!pool) return; // no database configured, so no auth.user was created
					const result = await pool.query(
						`INSERT INTO app_users (subject, email, display_name, status, deleted_at)
						 VALUES ($1, $2, $3, 'active', NULL)
						 ON CONFLICT DO NOTHING`,
						[user.id, user.email, citizenDisplayName(user)]
					);
					if (result.rowCount === 0) {
						// Nothing inserted. Two causes only, and they are told apart
						// by a read: (1) the row for this subject already exists —
						// the hook replayed, `uq_app_users_subject_active` did its
						// job, and there is nothing to say; or (2) no row for this
						// subject exists, so the conflict was the email: another
						// app_users row — active, disabled, or soft-deleted — already
						// owns that address. In case (2) the account is provisioned
						// WITHOUT a SIGAP identity rather than linked to that row; an
						// operator resolves the address conflict by id. Logged, never
						// thrown: the `auth.user` row is already committed, so failing
						// here would not undo it — it would only hide the state.
						const existing = await pool.query(
							"SELECT 1 FROM app_users WHERE subject = $1 LIMIT 1",
							[user.id]
						);
						if (existing.rowCount === 0) {
							console.warn(
								`[sigap auth] no app_users row created for auth user ${user.id}: ` +
									"its email is already owned by another app_users row — an email is " +
									"never a link between identities, so resolve that row by id"
							);
						}
					}
				}
			}
		}
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
				// Explicit rotation so the ES256 signing key cannot live forever:
				// 30 days (2592000 s). After this interval better-auth 1.7.20
				// stops signing with the key, and the JWKS display keeps serving it
				// for the default 30-day grace period so in-flight tokens still
				// verify. The Go verifier's JWKS cache
				// (apps/api/internal/auth/jwt_provider.go) holds a fetched key for
				// at most 15 minutes, so a deliberately retired key is accepted for
				// ≤15 min after the JWKS drops it — the bound documented in
				// design/auth-gap-matrix-v2.md item 4.
				rotationInterval: 60 * 60 * 24 * 30,
			},
			jwt: {
				// `iss` / `aud` are read by the Go JWT trust boundary, which verifies
				// them against SIGAP_AUTH_ISSUER / SIGAP_AUTH_AUDIENCE — the SAME
				// variable names, read here, so the two sides cannot be configured
				// independently and silently disagree (a mismatch makes every token
				// fail verification). Defaults are the local dev values.
				issuer: process.env.SIGAP_AUTH_ISSUER ?? "http://127.0.0.1:5173",
				audience: process.env.SIGAP_AUTH_AUDIENCE ?? "authenticated",
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