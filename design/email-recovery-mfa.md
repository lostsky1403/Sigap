# Email recovery, email verification & TOTP MFA (SIGAP self-hosted identity)

**Status:** Normative for the `emailAndPassword`, `emailVerification`, and `twoFactor()`
blocks of `apps/web/src/lib/server/auth.better.ts`; advisory for everything it marks
BLOCKED.
**Date:** 2026-11-11
**Subject:** the two recovery surfaces (email verification, password reset) that
better-auth now serves, the anti-enumeration contract they answer with, and the TOTP
second factor for privileged staff — plus exactly which parts are code-complete and
which are blocked on a real mail transport.
**Library of record:** `better-auth@1.7.20` (installed). Every behavioural claim below
was read out of `apps/web/node_modules/better-auth` / its pnpm-store
`@better-auth/core@1.7.20`; file references are to those installed sources.

---

## 1. What was added (config of record)

| Key | Value | Why it is written down |
|---|---|---|
| `emailAndPassword.autoSignIn` | `false` | Turns on better-auth's generic duplicate-signup response (see §3). Register already redirects to the sign-in form, so no session was being used anyway. |
| `emailAndPassword.requireEmailVerification` | `SIGAP_REQUIRE_EMAIL_VERIFICATION === "true"` | Env-gated, **default off**. Enforcement before a real transport exists would lock every account out. Flip only per §7. |
| `emailAndPassword.resetPasswordTokenExpiresIn` | `60 * 60` | 1-hour reset token (library default, pinned). |
| `emailAndPassword.revokeSessionsOnPasswordReset` | `true` | A reset ends every other session. |
| `emailAndPassword.sendResetPassword` | callback → `sendMail` | The only place a reset link is emitted. |
| `emailVerification.sendOnSignUp` | `true` | Every new account gets a verification mail. |
| `emailVerification.sendOnSignIn` | `false` | An unverified address must not be able to pump mail at will. |
| `emailVerification.autoSignInAfterVerification` | `false` | Possessing the link must not mint a session. |
| `emailVerification.expiresIn` | `60 * 60` | 1-hour verification token (library default, pinned). |
| `emailVerification.sendVerificationEmail` | callback → `sendMail` | Same sink as the reset link. |
| `twoFactor.issuer` | `"SIGAP"` | Label shown by the authenticator app. |
| `twoFactor.totpOptions` | `{ period: 30, digits: 6 }` | RFC 6238 defaults pinned; a future library default change must not invalidate enrolled staff. |
| `twoFactor.backupCodeOptions` | `{ amount: 10, length: 10, storeBackupCodes: "encrypted" }` | 10 single-use codes, AES-GCM at rest. |
| `twoFactor.twoFactorCookieMaxAge` | `10 * 60` | Seconds to finish the second factor after the password step. |
| `twoFactor.trustDeviceMaxAge` | `30 * 24 * 60 * 60` | Trusted-device cookie lifetime (library default). |
| `twoFactor.accountLockout` | `{ enabled: true, maxFailedAttempts: 10, durationSeconds: 15 * 60 }` | Account-level lockout across all factors. |

The `session`, `user`, `rateLimit`, cookies and `jwt` blocks are **unchanged**; the
`jwt` plugin stays first and `sveltekitCookies` stays last in `plugins`.

---

## 2. The flows

Paths are relative to `baseURL` (`BETTER_AUTH_URL`, `http://127.0.0.1:5173` locally)
plus better-auth's base path, i.e. `…/api/auth/…`. All are also reachable through
`hooks.server.ts` → `svelteKitHandler`.

### 2.1 Email verification

1. `POST /sign-up/email` (via the SvelteKit register action) creates the account, then
   — because `sendOnSignUp: true` — builds a token and calls
   `emailVerification.sendVerificationEmail({ user, url, token })`.
   `url = …/api/auth/verify-email?token=<jwt>&callbackURL=…`.
2. The sink captures the message (§5). Locally the link is read out of
   `.local-preview/mail.log`.
3. `GET /api/auth/verify-email?token=…` verifies the JWT (`HS256`, `exp` = 1 h),
   finds the user, sets `emailVerified = true`, and redirects to `callbackURL`.
4. `POST /api/auth/send-verification-email` re-sends on demand for an *unverified*
   address (constant-time handling in §3).

There is **no `verification` row** for this flow in 1.7.20: the token is a signed JWT,
so it cannot be revoked before `exp` — replay is handled by idempotency, not deletion
(§4).

### 2.2 Password reset (email recovery)

1. `POST /api/auth/request-password-reset { email }`.
2. Known address: a 24-char token is written to `auth.verification` under identifier
   `reset-password:<token>` with `expiresAt = now + 3600 s`, then
   `emailAndPassword.sendResetPassword({ user, url, token })` is called with
   `url = …/api/auth/reset-password/<token>?callbackURL=…`.
3. The user follows the link (captured locally) — `GET
   /api/auth/reset-password/<token>` is the redirect endpoint that hands the token on
   to `callbackURL`.
4. `POST /api/auth/reset-password { newPassword, token }` re-reads the row, rejects it
   when missing or past `expiresAt`, then spends it (§4) and writes the new password.
5. Every session of that user is deleted (§4).

The SvelteKit register action and the login action are the only callers of
`auth.api.signUpEmail` / `signInEmail`; nothing in the app calls
`requestPasswordReset` yet (see §7(5)).

### 2.3 TOTP MFA for privileged staff

Enrollment (authenticated, credential account, password required):

1. `POST /two-factor/enable { method: "totp", password }` — `method` defaults to
   `"totp"`. The plugin generates a 32-character secret, encrypts it, and returns
   `{ method: "totp", totpURI, backupCodes }`: the
   `otpauth://totp/SIGAP:user@example.com?secret=…&issuer=SIGAP` URI for the
   authenticator app plus the 10 plaintext backup codes. The row is stored with
   `verified = false` and `twoFactorEnabled` stays **false** — nothing is claimed yet.
2. `POST /two-factor/verify-totp { code }` during that session verifies the first code
   and then flips both the row's `verified = true` and `user.twoFactorEnabled = true`
   (the session cookie is rotated in the same call). This is the only way enrolment
   completes: `skipVerificationOnEnable` is left at its default `false`.
   `POST /two-factor/get-totp-uri` is the read-only companion for re-rendering the QR
   while enrolment is pending.

Sign-in with MFA enabled:

1. `POST /sign-in/email { email, password }` — credentials are checked first and a
   session IS created by the credential handler, but the plugin's `after` hook (matched
   on `/sign-in/*`) then **deletes that session**, clears the session cookie, and
   registers a pending challenge server-side: an `auth.verification` row `2fa-<20 chars>`
   plus a `2fa-attempts-<identifier>` counter, both expiring with
   `twoFactorCookieMaxAge`. The signed `two_factor` cookie only *references* that row.
   The response is `{ twoFactorRedirect: true, twoFactorMethods: ["totp"] }` and there
   is no live session for the user at this point.
2. `POST /two-factor/verify-totp { code, trustDevice? }` (or
   `POST /two-factor/verify-backup-code { code }`) — only on success is the session
   created and the session cookie set; the challenge row is spent.
3. `trustDevice: true` issues the 30-day trusted-device cookie backed by another
   `auth.verification` row; the next sign-in from that browser skips the challenge.

Disabling (`POST /two-factor/disable`) requires a **fresh** session
(`sensitiveSessionMiddleware`, i.e. within `session.freshAge = 1 h` — see
`design/session-lifecycle-policy.md`) *and* the account password.

The password step is the only thing that reveals anything about an account: an unknown
address and a wrong password both fail with the same credential error, before any
challenge exists (§3).

---

## 3. Anti-enumeration contract

| Surface | What better-auth 1.7.20 does | Where |
|---|---|---|
| `POST /request-password-reset` | Unknown address: generates a throwaway id **and runs a verification lookup against a dummy key** before answering `{status:true, message:"If this email exists in our system, check your email for the reset link"}`. A known address gets the same body. | `dist/api/routes/password.mjs` |
| `POST /send-verification-email` | Enforces a **≥500 ms floor** on the anonymous branch so "unknown / already verified" (a local JWT sign) cannot be told from "known and unverified" (an external send). Unknown or already-verified never sends and still answers `{status:true}`. | `dist/api/routes/email-verification.mjs` |
| `POST /sign-up/email` (duplicate) | With `autoSignIn: false` (now set) better-auth takes the generic branch: it **hashes the supplied password anyway** and returns a synthetic user with `token: null`, indistinguishable from a real sign-up response. Without it the endpoint throws `422 USER_ALREADY_EXISTS`. | `dist/api/routes/sign-up.mjs` |
| `POST /sign-in/email` | Unknown address and wrong password both fail with the same credential error. `twoFactorRedirect` is only ever computed **after** the password verified, so it cannot be used to test whether an address exists. | `dist/api/routes/sign-in.mjs`, `plugins/two-factor/index.mjs` |
| `GET /verify-email` | Invalid and expired tokens are distinguished (`INVALID_TOKEN` vs `TOKEN_EXPIRED`) — but a token can only exist if a mail was already sent to that address, so it leaks nothing an attacker does not already hold. | `dist/api/routes/email-verification.mjs` |

The **sink is part of this contract**: `/send-verification-email` re-throws a sender
failure, which would answer 500 for a real unverified address and 200 for an unknown
one — an oracle. `sendMail` therefore never rejects and never performs work that can
fail loudly on the request path (`apps/web/src/lib/server/mail.ts`).

---

## 4. Token expiry and replay prevention

| Flow | Token form | Lifetime | Replay behaviour |
|---|---|---|---|
| Email verification | HS256 JWT (signed with `BETTER_AUTH_SECRET`), payload carries `email`, `userId`, optional `updateTo`/`requestType` | `emailVerification.expiresIn` = **3600 s**, enforced by `exp` at `jwtVerify` | Idempotent, not "spent": a second visit to a link for an already-verified address returns `{status:true, user:null}` and does **not** re-verify or error. Because possession of the link never mints a session (`autoSignInAfterVerification: false`), a replayed verification link has no privilege to grant. |
| Password reset | 24-char id in `auth.verification`, identifier `reset-password:<token>` | `resetPasswordTokenExpiresIn` = **3600 s**, checked against the row's `expiresAt`; expired rows are rejected with `INVALID_TOKEN` | **Strictly single-use.** The row is consumed before the password is written, inside a per-identifier in-process lock and a transaction, so concurrent replays cannot both succeed; afterwards the row is gone and a replay fails `INVALID_TOKEN`. A failed reset leaves the token unspent (the user can retry). |
| MFA challenge | Signed `two_factor` cookie whose value is the identifier of an `auth.verification` row (`2fa-…`) | `twoFactorCookieMaxAge` = **600 s** on both the row and the cookie | The credential sign-in's session is deleted by the plugin before the response, and the challenge row is spent by a successful verification, so a challenge cannot be replayed into a second session. Replaying a used backup code fails because each successful use re-encrypts the *remaining* list. |
| Trusted device | Signed cookie | `trustDeviceMaxAge` = **30 days** | Opt-in per sign-in (`trustDevice: true`). This is the longest-lived credential in the config; it is the reason staff must not enable it on shared equipment. |

Session revocation on reset (all one sequence, sessions deleted **before** the password
write): `revokeSessionsOnPasswordReset: true`. This config does **not** enable
`database.transaction`, so the honest statement is "sequential, sessions first" — a
failure mid-sequence leaves the old password valid with every session signed out.

---

## 5. The local-capture dev path (the only path that works today)

`apps/web/src/lib/server/mail.ts` is the single sink for both callbacks.

- **Where:** `<repo-root>/.local-preview/mail.log`, append-only. The repo root is found
  by walking up from `process.cwd()` to the nearest ancestor containing
  `pnpm-workspace.yaml` or `.git` — necessary because
  `scripts/dev/Run-V22Service.ps1` does `Set-Location apps\web`, so a naive
  `cwd/.local-preview` would create a stray untracked directory inside the app.
  `.local-preview/` is already gitignored at the repo root and already holds
  `web.log` / `api.log`.
- **Override:** `SIGAP_MAIL_LOG=<path>` pins the file (absolute or relative).
- **Console:** one line per message —
  `[sigap mail] captured password-reset to=… subject=… url=… → <path>` — with the
  actionable URL inline so a developer never has to open the file to complete a flow.
- **File format:** `=== <ISO-8601> | <kind> | to: <address> | subject: <subject> ===`
  followed by the full body.
- **Failure mode:** every failure (`mkdir`, append) is logged as
  `[sigap mail] capture failed …` and swallowed. The function returns `Promise<void>`
  and is total by construction.

**Production branch (`SIGAP_SMTP_URL`) — a documented seam, NOT a configured
transport.** When it is set, `sendMail` emits one `console.error` per process stating
that no SMTP transport is implemented in this build, logs
`DROPPED …` for the message, and **returns without writing the link to disk** — a
deployment that believes it has SMTP must not leave live reset links in a plaintext
log file. Nothing in this repository sends mail over the network, and no dependency
that could was added: better-auth ships no SMTP client and this change adds none.
That is why `SIGAP_SMTP_URL` must not be described anywhere as "configured".

---

## 6. Backup-code lifecycle

- **Issued:** 10 codes, 10 characters each, rendered `XXXXX-XXXXX`; generated by
  better-auth with `crypto`-backed randomness at `/two-factor/enable` (and returned
  once, in that response only).
- **At rest:** stored **encrypted** (`storeBackupCodes: "encrypted"`, AES-GCM keyed
  off the auth secret) in `auth.twoFactor.backupCodes`. `twoFactor.backupCodes` and
  `twoFactor.secret` are declared `returned: false`, so they never appear in API
  responses or session payloads.
- **Use:** each code is consumed exactly once — a successful
  `/two-factor/verify-backup-code` re-encrypts the list *without* that code, so a
  captured code cannot be replayed. A used code is not recoverable from the server
  side; the user must draw from the remaining set.
- **Reissue:** `POST /two-factor/generate-backup-codes` (session + password, only when
  2FA is enabled) **replaces** the stored set — every previously issued code dies at
  that moment. This is the "I lost my list" self-service path.
- **Disable:** `POST /two-factor/disable` (fresh session + password) deletes the
  `auth.twoFactor` row outright — secret AND all remaining codes — clears
  `twoFactorEnabled`, rotates the session cookie, and expires any trusted-device cookie
  along with its verification row. Re-enabling therefore issues a brand-new secret and
  a brand-new code list. Consequence for support: "disable and re-enroll" is a
  **destructive** recovery, not a re-print, and it also removes the second factor in
  the interval — only use it as the last step of §6's lost-device path.
- **Recovery when the list AND the authenticator are lost:** a server-only call,
  `auth.api.viewBackupCodes({ userId })` — not exposed over HTTP, no client method —
  must be driven by trusted server code from an authenticated admin context. That
  tooling does not exist yet: `apps/web/scripts/provision-staff.mjs` is the account
  lifecycle CLI (it creates credentials and can revoke sessions or deactivate an
  account) and never touches `auth.twoFactor`. Until a reviewed recovery path is built
  (§7), the operator sequence is: confirm the person's identity out of band, clear the
  `auth.twoFactor` row for that user and let them re-enroll, and record the approval
  wherever staff changes are recorded. **Do not** disable MFA by hand to get someone
  back in without an approval trail: the second factor exists precisely to make that
  path expensive.
- **Lost-device policy to decide before rollout:** who may trigger recovery, what
  identity proof is required, and where the approval is recorded. This document does
  not yet name an owner.

---

## 7. Prerequisites for production (all BLOCKED items)

1. **SMTP transport.** A real sender must be written (provider SDK or a queued worker),
   wired behind `SIGAP_SMTP_URL`, with a verified `From` domain, SPF/DKIM/DMARC, TLS,
   bounce/complaint handling and a suppression list. Until then: no verification mail,
   no reset mail, and the local capture file is the only sink. Also note §5 — an SMTP
   URL with no transport means *dropped*, not *delivered*.
2. **A migration, before any MFA use.** The `twoFactor()` plugin requires the
   `auth.twoFactor` table and `auth.user.twoFactorEnabled`. **Verified against the local
   dev DB on 2026-11-11:** `auth` contains `account, jwks, session, user, verification`
   and `auth.user` has no `twoFactorEnabled` — i.e. the migration is outstanding. Reads
   are unaffected (the Kysely adapter does `selectAll()` and ignores absent columns, so
   sign-in/session resolution keep working), but `/two-factor/enable` will fail with
   `column "twoFactorEnabled" does not exist` until
   `npx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts` has run
   (command and schema notes: `design/better-auth-schema.md`). Note that
   `design/better-auth-integration-contract.md` §7's "no RLS/2FA migration path needed"
   refers to the Supabase → Better Auth *data* migration; it does not mean the MFA
   plugin's own schema is already present.
3. **SMS/OTP factor for staff without a TOTP app.** The plugin's OTP factor fires only
   when `otpOptions.sendOTP` is configured; it deliberately is **not**, because that
   needs a paid messaging provider and this mission forbids new paid services. So the
   only second factor today is TOTP, and a staff member without a capable device
   cannot sign in with MFA enabled — the backup codes are the designed answer, and (2)
   must be applied first.
4. **A mail-delivery-dependent policy switch.** The sign-in gate is
   `emailAndPassword.requireEmailVerification` alone (`sendOnSignUp: true` only *sends*);
   set `SIGAP_REQUIRE_EMAIL_VERIFICATION=true` only once (1) is real. Existing accounts
   carry `emailVerified = false`; flipping the switch before delivery exists locks them
   out.
5. **UI to consume the tokens.** The endpoints are live, but the app has no
   `/reset-password` or verification-result screen: the reset link lands on better-auth's
   redirect endpoint with `?token=…`, and nothing in `apps/web/src/routes` posts it to
   `POST /api/auth/reset-password` yet. Also the **login action ignores the return value
   of `signInEmail`**, so an MFA-enabled user would be redirected to `/` while holding no
   session — the action must branch on `twoFactorRedirect` and send the user to a
   challenge screen. Neither gap is closed by this change (the login action and the new
   screens are outside the files this change was scoped to).
6. **Shared multi-process deployment.** `rateLimit.storage: "memory"` and the local
   capture file are single-process constructs. A multi-replica deployment needs
   `rateLimit.storage: "database"` (or Redis) before these limits mean anything.

---

## 8. Rate limiting (reused, not reinvented)

`rateLimit` remains exactly `{ enabled: true, window: 60, max: 20, storage: "memory" }`.
The global block is the floor; better-auth 1.7.20 layers stricter rules on top
(`dist/api/rate-limiter/index.mjs`):

| Path | Window | Max |
|---|---|---|
| `/sign-in*`, `/sign-up*`, `/change-password*`, `/change-email*` | 10 s | 3 |
| `/request-password-reset`, `/send-verification-email`, `/forget-password*`, `/email-otp/*` | 60 s | 3 |
| `/two-factor/*` (added by the plugin) | 10 s | 3 |
| everything else (`/api/auth/*`) | 60 s | 20 |

Two complements, because a transport limit alone cannot stop a slow brute force:

- **Per challenge:** a sign-in challenge carries its own attempt counter
  (`auth.verification` row `2fa-attempts-<identifier>`); after **5** failed codes the
  challenge row is spent and the user must enter the password again. The counter is
  re-armed on each failure and cleared on success, so it measures *consecutive* failures
  against that one challenge.
- **Per account:** `twoFactor.accountLockout` above stops the "start a fresh challenge
  every 5 tries" loop — 10 consecutive failures across challenges and factors set
  `lockedUntil` on the `auth.twoFactor` row for 15 minutes and answer
  `ACCOUNT_TEMPORARILY_LOCKED` (429). A successful verification clears both counters.
- The SvelteKit form path's independent per-IP limiter in
  `apps/web/src/lib/server/rate-limit.ts` (`allowLogin` / `allowRegister`) closes the last
  gap: server actions calling `auth.api.*` bypass `auth.handler`, and therefore bypass
  everything in the table above.

---

## 9. What is NOT verified

Read this as the honest boundary of this change.

- **Nothing was executed.** No build, type-check, unit test, E2E run, or migration was
  performed as part of this change (the task explicitly excluded running gates). The
  configuration is therefore *unverified by execution*: it has not been shown to
  type-check under `svelte-check`, and no endpoint listed above has been called at
  runtime.
- **No real send has happened.** `.local-preview/mail.log` has not been created by this
  change; the capture path's behaviour under the real dev cwd (`apps/web`) is derived
  from `Run-V22Service.ps1`, not observed.
- **MFA is unenrolled.** No staff account has had 2FA enabled; the migration in §7(2)
  is outstanding, so `/two-factor/enable` would currently fail. No TOTP code, backup
  code, trusted-device cookie, or account lockout has been exercised.
- **No production delivery exists.** `SIGAP_SMTP_URL` is a seam, not a transport;
  production email delivery is unimplemented and unverified. No SMS/OTP factor is
  configured.
- **The forms are unwired.** Registration/login pages do not initiate verification or
  reset, and the login action does not handle `twoFactorRedirect` (§7(5)).
- **Behavioural claims are source-level.** Every statement about better-auth 1.7.20 was
  read from the installed `dist` sources and the live `auth` schema; they are not
  observations of a running request. A reviewer should re-read the cited files after any
  upgrade — `design/better-auth-integration-contract.md` is the place to record what an
  upgrade would have to re-prove.
