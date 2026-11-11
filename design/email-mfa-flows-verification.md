# Email verification, password reset & TOTP MFA — live end-to-end verification

**Status:** Evidence record. Measurements taken against a running preview, not
policy and not a design proposal. The normative documents remain
`design/email-recovery-mfa.md` (config of record) and
`design/selfhosted-auth-e2e.md` (the four-actor browser journey).
**Date:** 2026-11-11, 11:47:34–11:54:08 WIB (04:47:34–04:54:08 UTC).
**Subject:** the three public recovery/MFA flows of the self-hosted identity
layer, driven through their real HTTP endpoints on a live SvelteKit preview with
the local mail capture as the delivery channel — and an explicit list of what
was **not** exercised.
**Config under test:** `apps/web/src/lib/server/auth.better.ts` as loaded by the
preview (branch `feat/sigap-selfhosted-auth`, HEAD `9b369bd`).

---

## 0. Verdict

| # | Flow | Verdict | Evidence |
|---|---|---|---|
| 1 | **Email verification** — register → captured mail → open link → `emailVerified = true` | **PASSED end-to-end** | §2 (requests A–C, `auth.user.emailVerified` `f → t`) |
| 2 | **Password reset** — request → captured link → set new password → new signs in, old rejected | **PASSED end-to-end** (twice, on two different accounts; fixture restored) | §3 (requests 1–11 and 29–34) |
| 3 | **TOTP MFA** — enroll → sign in with TOTP → sign in with a backup code → wrong/replayed codes rejected | **PASSED end-to-end** | §4 (requests 12–26) |

Everything in those three flows that can be observed from outside the process —
HTTP status, response body, `Set-Cookie`, `auth.*` rows in Postgres, and the
captured mail file — was observed directly. Nothing here is inferred from the
configuration.

**Not verified:** §6. Short version — real SMTP delivery (there is no transport
in this build), the `requireEmailVerification: true` policy branch, the
trusted-device cookie, the account-lockout ceiling, and the Go-API JWT trust
boundary for an MFA-enrolled subject.

---

## 1. Method

### 1.1 The stack under test

| Piece | Where | State when the run started |
|---|---|---|
| SvelteKit (Better Auth handler + app) | `http://127.0.0.1:5173` | already running (vite 5.4.21, `web.log` written 11:36 WIB) |
| Go API | `http://127.0.0.1:18080` | already running |
| Postgres | `127.0.0.1:5433/sigap` (`sigap-postgres`) | already running |
| Rust queue engine | `127.0.0.1:50051` | already running |
| Mail capture | `F:\sigap-design-v2\.local-preview\mail.log` | **did not exist** before this run — every entry in it was produced by this run |

The preview was **not restarted**. A live stack was already serving (started at
11:36 WIB by `scripts/dev/Start-V22Preview.ps1`), and another agent had an
uncommitted edit to `apps/web/src/lib/server/auth.better.ts` in flight
(mtime 11:54:47 WIB, i.e. *after* the last request below). Restarting would have
torn down the instance this document describes and would have picked up a
half-finished config. Every request below was served by the pre-existing
process; the endpoints and behaviours verified here are the ones that process
exhibited.

### 1.2 The harness

Better Auth was driven directly over HTTP with a Python `urllib` opener carrying
a cookie jar (equivalent `curl` given per step in §2–§4), because the flows need
cookie continuity of both `better-auth.session_token` and
`better-auth.two_factor`.

Three things about the HTTP surface are load-bearing and were handled by the
harness:

1. **Media type.** `allowedMediaTypes: ["application/json"]` — every POST body
   is JSON with `Content-Type: application/json`.
2. **Origin.** `originCheckMiddleware` validates the `Origin` header against
   `trustedOrigins` (default: `baseURL`) **whenever the request carries a
   cookie**. Every POST below therefore sent `Origin: http://127.0.0.1:5173`.
   (Read out of `better-auth/dist/api/middlewares/origin-check.mjs`, not
   exercised here: a cookie-bearing POST from a foreign origin should answer
   `403 INVALID_ORIGIN`. This run never sent one.)
3. **Rate limits** — read from the installed library
   (`dist/api/rate-limiter/index.mjs` `getDefaultSpecialRules()` and the
   `twoFactor` plugin's own rule), not witnessed: no request in this run was
   answered `429`, so the buckets below were *respected*, not *observed*.
   `rateLimit { window: 60, max: 20, storage: memory }`; buckets are keyed by
   client IP + normalized path:

   | Path | Bucket |
   |---|---|
   | `/sign-in*`, `/sign-up*`, `/change-password`, `/change-email` | 3 / 10 s |
   | `/request-password-reset`, `/send-verification-email`, `/forget-password*` | 3 / 60 s |
   | `/two-factor/*` | 3 / 10 s |
   | everything else | 20 / 60 s |

   The memory store resets a bucket only after the **full window of inactivity**
   once the maximum is reached, so "3 per 10 s" means three requests and then a
   ≥10 s gap. The harness paces per path; no request in this run was answered
   `429`.

**TOTP generation** was a local RFC 6238 computation — `crypto.createHmac("sha1", …)`
over the base32-decoded secret, 30 s step, 6 digits, dynamic truncation — in a
throwaway helper at `.scratch/totp.cjs` (`.scratch/` is gitignored). No
dependency was added.

**Database reads** used `psql` (PostgreSQL 18.4) against the 5433 dev DSN from
`.env`:

```sh
psql "postgresql://sigap:<pw from .env>@127.0.0.1:5433/sigap?sslmode=require" \
     -XA -c 'SELECT id, email, "emailVerified" FROM auth."user";'
```

Port 5434 and `F:\Sehat-Hub` were never touched.

### 1.3 Accounts used

| Account | Why | State left behind |
|---|---|---|
| `verify-20261111114733@sigap.local` (id `wgyFEisstBSr6uMFAzj2RaEPyAm2UKyK`) | created by flow 1; reused for a second, independent flow-2 cycle | `emailVerified = t`, no MFA, password now `Verify54321!` (§3.4), 0 sessions |
| `smoke-b@sigap.local` (fixture) | flow 2's primary run — a **seeded** account with a known password | **restored**: password is `SmokeB12345!` again (proved by sign-in #10), 0 sessions |
| `mfa-20261111115023@sigap.local` (id `zQX6cLbNGghwFLZi0Dir74Hy0g6aBcdH`) | created by flow 3 for MFA enrolment | `twoFactorEnabled = t`, `emailVerified = f`, 0 sessions, 9 of 10 backup codes unspent |

Flow 3 deliberately does **not** enrol a seeded journey fixture: turning on
`twoFactorEnabled` for `smoke@sigap.local`, `smoke-b@sigap.local`,
`org-a-admin@sigap.local` or `global-admin@sigap.local` would make
`apps/web/e2e/selfhosted-auth-journey.spec.ts` fail — it signs those accounts in
with email + password only and would start receiving
`{"twoFactorRedirect": true}` (§4.3, observed). The account above plays the
fixture role for this verification instead.

---

## 2. Flow 1 — email verification

**Result: PASSED end-to-end.**

### 2.1 Steps

| # | Request | Status | Observed |
|---|---|---|---|
| A | `POST /api/auth/sign-up/email` `{"name":"Verify Probe","email":"verify-20261111114733@sigap.local","password":"Verify12345!"}` | **200** | `{"token":null,"user":{…,"emailVerified":false,"twoFactorEnabled":false,"id":"wgyFEisstBSr6uMFAzj2RaEPyAm2UKyK"}}` — **no `Set-Cookie`** (`autoSignIn: false`), so registration mints no session |
| B | `GET /api/auth/verify-email?token=<HS256 JWT>&callbackURL=%2F` — URL read out of `.local-preview/mail.log` | **302** | `Location: /` |
| C | same URL, replayed | **302** | idempotent: no error, no re-verification, `emailVerified` stays `t` |

```sh
# A
curl -sS -D- -X POST http://127.0.0.1:5173/api/auth/sign-up/email \
  -H 'Origin: http://127.0.0.1:5173' -H 'Content-Type: application/json' \
  -d '{"name":"Verify Probe","email":"verify-<ts>@sigap.local","password":"Verify12345!"}'
# B — the URL is printed by the sink on the web console and appended to the file
curl -sS -D- "$(grep -o 'http://127.0.0.1:5173/api/auth/verify-email?token=[^ ]*' \
  F:/sigap-design-v2/.local-preview/mail.log | tail -1)"
```

### 2.2 Mail capture

`.local-preview/mail.log` gained this block (token abbreviated; the full token
is in the gitignored file):

```
=== 2026-11-11T04:47:34.043Z | email-verification | to: verify-20261111114733@sigap.local | subject: Verifikasi alamat surel SIGAP ===
Halo,

Konfirmasi alamat surel ini dengan membuka tautan berikut:
http://127.0.0.1:5173/api/auth/verify-email?token=<HS256 JWT>&callbackURL=%2F

Tautan berlaku satu jam. Abaikan surel ini jika Anda tidak mendaftar.
```

The web console line for the same message was
`[sigap mail] captured email-verification to=verify-20261111114733@sigap.local subject=Verifikasi alamat surel SIGAP url=http://127.0.0.1:5173/api/auth/verify-email?token=…`.

### 2.3 Database evidence

```
id                              | email                            | emailVerified
wgyFEisstBSr6uMFAzj2RaEPyAm2UKyK| verify-20261111114733@sigap.local| t
```

Before request B the same row read `f`; the only write between the two reads was
request B. The flip is therefore attributable to the mailed link, not to the
sign-up.

---

## 3. Flow 2 — password reset

**Result: PASSED end-to-end, on two accounts, including single-use-token
evidence and a restore of the seeded fixture.**

### 3.1 The documented path is `/request-password-reset`

| Request | Status | Note |
|---|---|---|
| `POST /api/auth/forget-password` `{"email":"smoke-b@sigap.local"}` (#27) | **404** | there is **no** `forget-password` alias in `better-auth@1.7.20`; the path only appears in the rate-limit matcher (`dist/api/rate-limiter/index.mjs`). The endpoint of record is `POST /api/auth/request-password-reset`. |

### 3.2 Primary run (`smoke-b@sigap.local`, a seeded fixture)

| # | Request | Status | Observed |
|---|---|---|---|
| 1 | `POST /api/auth/request-password-reset` `{"email":"smoke-b@sigap.local"}` | **200** | `{"status":true,"message":"If this email exists in our system, check your email for the reset link"}` |
| 2 | `GET /api/auth/reset-password/<token>?callbackURL=` (link from `mail.log`) | **302** | hands the token to the app page (raw capture of the same redirect in §3.3: `location: http://127.0.0.1:5173/?token=<token>`) |
| 3 | `POST /api/auth/reset-password` `{"newPassword":"SmokeB54321!","token":"<token>"}` | **200** | `{"status":true}` |
| 4 | same `token`, replayed with a different password | **400** | `{"message":"Invalid token","code":"INVALID_TOKEN"}` — the row was consumed by #3 |
| 5 | `POST /api/auth/sign-in/email` `{"email":"smoke-b@sigap.local","password":"SmokeB12345!"}` | **401** | `{"message":"Invalid email or password","code":"INVALID_EMAIL_OR_PASSWORD"}` — the OLD password is rejected |
| 6 | `POST /api/auth/sign-in/email` `{"email":"smoke-b@sigap.local","password":"SmokeB54321!"}` | **200** | `Set-Cookie: better-auth.session_token=…`; a live session row appears |
| 7–9 | second cycle: request → `GET` link (**302**) → `POST /reset-password` back to `SmokeB12345!` | **200/302/200** | the restore; a fresh token was issued (tokens differ) |
| 10 | `POST /api/auth/sign-in/email` `{"…","password":"SmokeB12345!"}` | **200** | the **seeded fixture password works again** |
| 11 | `POST /api/auth/sign-out` | **200** | `{"success":true}` |

### 3.3 The redirect's raw headers (third cycle, `verify-…@sigap.local`)

`sec-fetch` / browser navigation is not required; the endpoint answers a plain
`GET`:

```
30 GET /api/auth/reset-password/ckrMdTjah9XakUJQ87u1zSnt?callbackURL=%2F -> 302
   location: http://127.0.0.1:5173/?token=ckrMdTjah9XakUJQ87u1zSnt
   content-type: application/json      access-control-allow-origin: *
   x-frame-options: DENY               x-content-type-options: nosniff
```

### 3.4 Second, independent cycle (`verify-20261111114733@sigap.local`)

Run on an account whose address had been **verified** in flow 1, to show the
flow does not depend on `emailVerified = false`:

| # | Request | Status | Observed |
|---|---|---|---|
| 29 | `POST /api/auth/request-password-reset` `{"email":"verify-…@sigap.local"}` | **200** | same generic body |
| 30 | `GET /api/auth/reset-password/<token>?callbackURL=%2F` | **302** | `location: http://127.0.0.1:5173/?token=…` |
| 31 | `POST /api/auth/reset-password` `{"newPassword":"Verify54321!","token":"<token>"}` | **200** | `{"status":true}` |
| 32 | sign-in with `Verify12345!` | **401** | old password rejected |
| 33 | sign-in with `Verify54321!` | **200** | new password accepted |
| 34 | `POST /api/auth/sign-out` | **200** | `{"success":true}` |

### 3.5 Captured reset mail

Three `password-reset` messages were captured (04:48:42.866Z and 04:49:15.599Z
to `smoke-b@sigap.local`, 04:54:08.257Z to `verify-…@sigap.local`):

```
=== 2026-11-11T04:48:42.866Z | password-reset | to: smoke-b@sigap.local | subject: Atur ulang kata sandi SIGAP ===
Halo,

Permintaan atur ulang kata sandi diterima untuk akun ini.
Tautan berlaku satu jam dan hanya dapat dipakai sekali: http://127.0.0.1:5173/api/auth/reset-password/<24-char token>?callbackURL=

Jika Anda tidak meminta ini, abaikan surel ini — kata sandi tidak berubah.
```

Every message was produced by a `request-password-reset` call in this run. The
`forget-password` alias probe (#27) answered 404 and wrote nothing, and no
message was written for request #4 — a replayed, already-spent token never
reaches the mail callback. **An unknown address was never submitted**, so this
run does not test the anti-enumeration claim (see §6.12).

### 3.6 Database evidence for single-use

`auth.verification` holds **no `reset-password:*` rows** after the run, although
three reset cycles were started and completed. The row was consumed at the
moment the new password was written, which is what makes the replay in #4 answer
`400 INVALID_TOKEN`.

---

## 4. Flow 3 — TOTP MFA

**Result: PASSED end-to-end**, on `mfa-20261111115023@sigap.local` (created for
this flow, §1.3).

### 4.1 Enrolment

| # | Request | Status | Observed |
|---|---|---|---|
| 12 | `POST /api/auth/sign-up/email` `{"name":"MFA Probe", …}` | **200** | account created, `token:null` |
| 13 | `POST /api/auth/sign-in/email` `{…, password:"Mfa123456!"}` | **200** | session cookie `better-auth.session_token` |
| 14 | `POST /api/auth/two-factor/enable` `{"password":"Mfa123456!","method":"totp"}` | **200** | `{"method":"totp","totpURI":"otpauth://totp/SIGAP:mfa-…%40sigap.local?secret=<32-char base32>&issuer=SIGAP&digits=6&period=30","backupCodes":[ … 10 codes … ]}` — e.g. `ByWL7-u6FnQ` |
| 15 | `POST /api/auth/two-factor/verify-totp` `{"code":""}` (harness artefact: the code generator printed nothing) | **401** | `{"message":"Invalid code","code":"INVALID_CODE"}` — an empty code is rejected, and enrolment is **not** completed by a failure |
| 16 | `POST /api/auth/two-factor/verify-totp` `{"code":"<local TOTP>"}` | **200** | enrolment completed; the **session cookie is rotated** (`Set-Cookie` carries a new `session_token`, the pre-enrolment token row is deleted). Note the response body's `token` field echoes the *pre-rotation* token while the cookie carries the new one — the body value is dead, the cookie is live. |
| 17 | `POST /api/auth/sign-out` | **200** | |

**Between 14 and 16 the account is un-enrolled**, exactly as the config
comments say: the `totpURI`/`backupCodes` exist but nothing is armed yet.

Post-enrolment rows:

```
auth.user:        email = mfa-20261111115023@sigap.local → twoFactorEnabled = t, emailVerified = f
auth."twoFactor": userId=zQX6cLbNGghwFLZi0Dir74Hy0g6aBcdH | verified = t | failedVerificationCount = 0 | lockedUntil = ∅
                  secret_len = 144 | backupCodes_len = 362
```

`secret` (144 chars) and `backupCodes` (362 chars at enrolment, **334** after one
code was spent) are the AES-GCM envelopes: the base32 secret from the `otpauth:`
URI does **not** appear in the column, and the plaintext backup code
`ByWL7-u6FnQ` does not appear in `backupCodes` either
(`position('ByWL7-u6FnQ' in "backupCodes") = 0`). The at-rest claim is therefore
true for the one plaintext value this run holds.

### 4.2 Sign-in with a TOTP code

| # | Request | Status | Observed |
|---|---|---|---|
| 18 | `POST /api/auth/sign-in/email` `{…,"password":"Mfa123456!"}` | **200** | `{"twoFactorRedirect":true,"twoFactorMethods":["totp"]}`; the credential handler's session **is deleted by the plugin** (`auth.session` count for this user = **0**), no `session_token` cookie is kept, and a signed `better-auth.two_factor` cookie (`2fa-…`) is set |
| 19 | `POST /api/auth/two-factor/verify-totp` `{"code":"000000"}` (verified locally not to be in the ±1-step window) | **401** | `{"message":"Invalid code","code":"INVALID_CODE"}`; the challenge cookie survives, and `failedVerificationCount` went `0 → 1` |
| 20 | `POST /api/auth/two-factor/verify-totp` `{"code":"220904"}` (correct) | **200** | `{"token":"…","user":{…}}`; `Set-Cookie: better-auth.session_token` set, `better-auth.two_factor` expired; `auth.session` for the user = **1**; `failedVerificationCount` reset to **0** |

### 4.3 Sign-in with a backup code

| # | Request | Status | Observed |
|---|---|---|---|
| 21 | `POST /api/auth/sign-out` | **200** | sessions back to 0 |
| 22 | `POST /api/auth/sign-in/email` (password) | **200** | `{"twoFactorRedirect":true,…}` — a new challenge |
| 23 | `POST /api/auth/two-factor/verify-backup-code` `{"code":"ByWL7-u6FnQ"}` | **200** | `{"token":"…","user":{…}}` + session cookie; `auth.session` for the user = **1**; the stored list is rewritten (`backupCodes_len 362 → 334`) |
| 24 | `POST /api/auth/sign-out` | **200** | |
| 25 | `POST /api/auth/sign-in/email` (password) | **200** | third challenge |
| 26 | **replay** `POST /api/auth/two-factor/verify-backup-code` `{"code":"ByWL7-u6FnQ"}` | **401** | `{"message":"Invalid backup code","code":"INVALID_BACKUP_CODE"}`; `auth.session` for the user = **0**; `failedVerificationCount` = **1** |

So: a backup code signs in once, is re-encrypted away, and a replay of the same
string is refused. Throughout, no session row exists for the account while a
challenge is pending — the password step alone never produces a usable session
once MFA is enabled.

### 4.4 Leftover challenge state (expected)

```
auth.verification:
  2fa-JBdLmAyyPM105yB0RBRw              | value zQX6cLbNGghwFLZi0Dir74Hy0g6aBcdH
  2fa-attempts-2fa-JBdLmAyyPM105yB0RBRw | value 1
```

Challenges 1 and 2 were consumed by requests 20 and 23; challenge 3 was answered
once, wrongly (26), so it is still open with one attempt recorded against it and
expires with `twoFactorCookieMaxAge` (600 s). This is the expected residue of an
abandoned challenge, not a leak: it references a user id and carries no
credential.

---

## 5. Complete request/response ledger

Every HTTP request this run made against the preview, in order. (`A`–`C` are
flow 1 and were issued before the numbered harness; `#27`/`#28` are the alias
and idempotency probes.)

| # | Method + path | Status |
|---|---|---|
| A | `POST /api/auth/sign-up/email` (verify account) | 200 |
| B | `GET /api/auth/verify-email?token=…` | 302 → `/` |
| 1 | `POST /api/auth/request-password-reset` (smoke-b) | 200 |
| 2 | `GET /api/auth/reset-password/<token>` | 302 |
| 3 | `POST /api/auth/reset-password` | 200 |
| 4 | `POST /api/auth/reset-password` (replay, spent token) | 400 |
| 5 | `POST /api/auth/sign-in/email` (old password) | 401 |
| 6 | `POST /api/auth/sign-in/email` (new password) | 200 |
| 7 | `POST /api/auth/request-password-reset` (restore) | 200 |
| 8 | `GET /api/auth/reset-password/<restore token>` | 302 |
| 9 | `POST /api/auth/reset-password` (back to original) | 200 |
| 10 | `POST /api/auth/sign-in/email` (restored password) | 200 |
| 11 | `POST /api/auth/sign-out` | 200 |
| 12 | `POST /api/auth/sign-up/email` (MFA account) | 200 |
| 13 | `POST /api/auth/sign-in/email` | 200 |
| 14 | `POST /api/auth/two-factor/enable` | 200 |
| 15 | `POST /api/auth/two-factor/verify-totp` (empty code) | 401 |
| 16 | `POST /api/auth/two-factor/verify-totp` (valid code) | 200 |
| 17 | `POST /api/auth/sign-out` | 200 |
| 18 | `POST /api/auth/sign-in/email` (→ challenge) | 200 |
| 19 | `POST /api/auth/two-factor/verify-totp` (wrong code) | 401 |
| 20 | `POST /api/auth/two-factor/verify-totp` (correct code) | 200 |
| 21 | `POST /api/auth/sign-out` | 200 |
| 22 | `POST /api/auth/sign-in/email` (→ challenge) | 200 |
| 23 | `POST /api/auth/two-factor/verify-backup-code` (first code) | 200 |
| 24 | `POST /api/auth/sign-out` | 200 |
| 25 | `POST /api/auth/sign-in/email` (→ challenge) | 200 |
| 26 | `POST /api/auth/two-factor/verify-backup-code` (replay) | 401 |
| 27 | `POST /api/auth/forget-password` (alias probe) | 404 |
| 28 | `GET /api/auth/verify-email?token=…` (replay) | 302 |
| 29 | `POST /api/auth/request-password-reset` (verify account) | 200 |
| 30 | `GET /api/auth/reset-password/<token>` | 302 |
| 31 | `POST /api/auth/reset-password` | 200 |
| 32 | `POST /api/auth/sign-in/email` (old password) | 401 |
| 33 | `POST /api/auth/sign-in/email` (new password) | 200 |
| 34 | `POST /api/auth/sign-out` | 200 |

No request was answered `429`; no request was answered with a 5xx.

---

## 6. What is NOT verified

Listed so that nobody reads §0 as a bigger claim than it is. Each line says what
is missing and why.

1. **Real mail delivery (SMTP).** `SIGAP_SMTP_URL` was **not** set and no
   transport exists in this build: a set `SIGAP_SMTP_URL` logs one
   `console.error` and drops the message without writing the link to disk. The
   only delivery channel exercised is the local capture file.
2. **`requireEmailVerification: true`.** The env gate
   (`SIGAP_REQUIRE_EMAIL_VERIFICATION`) was unset, so the policy branch was
   never taken. In particular *unverified sign-in being refused* and *a
   verification mail being the precondition for a session* were not observed.
   Notably `smoke-b@sigap.local` and the MFA account both signed in with
   `emailVerified = f` throughout this run.
3. **Trusted device.** No request set `trustDevice: true`, so the
   `trust-device-*` verification row, its 30-day cookie, and the "next sign-in
   skips the challenge" behaviour are untested.
4. **Account lockout.** `accountLockout { maxFailedAttempts: 10, durationSeconds: 900 }`
   was not driven to its ceiling (only two failed verifications in total, and
   both were reset by the following success). Neither the 15-minute lock nor the
   per-challenge 5-attempt cap (`TOO_MANY_ATTEMPTS_REQUEST_NEW_CODE`) was
   reached.
5. **Verification-link expiry.** Both MAX_AGE clocks are 3600 s; the run lasted
   ~7 minutes, so an expired token was never presented. The *replay* semantics
   were observed (302, idempotent), but the JSON body variant
   (`{status:true,user:null}`) was not: the observed replay was the redirect
   branch of a URL that carries `callbackURL`.
6. **TOTP drift window.** Codes were generated for the current step only; the
   adjacent-step codes that the verifier's window would also accept were
   computed (for the wrong-code check) but never submitted.
7. **`POST /api/auth/send-verification-email`** (resend for an unverified
   address) was not called.
8. **`POST /api/auth/two-factor/disable`** and
   `POST /api/auth/two-factor/generate-backup-codes` were not called, so
   "requires a fresh session + password" and re-issuing codes are unverified.
9. **The Go API trust boundary.** Nothing here exercises the SvelteKit
   `auth.api.getToken` → ES256 JWT → `SIGAP_AUTH_JWKS_URL` → `app_users.subject`
   path, and nothing checks that an MFA-enrolled subject maps to the right
   internal user. That is `design/selfhosted-auth-e2e.md`'s scope.
10. **Concurrency.** The single-use guarantee in #4 and #26 was observed
    sequentially; two simultaneous replays of one reset token were not fired.
11. **Backup-code exhaustion and encryption strength.** The claim that codes are
    AES-GCM-encrypted at rest is supported only in the weak sense that the one
    plaintext code this run holds is absent from the column and its length moved
    when one code was spent. The plaintext count of remaining codes was not
    recovered from the envelope.
12. **Anti-enumeration.** Every address submitted to `/request-password-reset`
    was a real account. The claim that an unknown address gets the byte-identical
    `{"status":true,"message":"If this email exists in our system, …"}` response
    — and the ≥500 ms floor on `/send-verification-email` — were **not** measured;
    they need a known/unknown pair submitted back to back, which the 3-per-60 s
    bucket on that path makes awkward to script but not impossible.

---

## 7. How to reproduce

`Start-V22Preview.ps1` was already running for this run, but a clean reproduction
is:

```pwsh
# 1. stack (Better Auth works in dev mode too; -Jwt additionally switches the Go
#    API to the real JWT trust boundary)
pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1

# 2. mail capture — the sink resolves to <repo-root>/.local-preview/mail.log
Remove-Item .local-preview/mail.log -ErrorAction SilentlyContinue

# 3. drive the flows (JSON bodies, cookie jar, Origin header — see §1.2), then
#    read the link out of the capture:
Select-String -Path .local-preview/mail.log -Pattern 'verify-email|reset-password'

# 4. assert against the auth schema (5433 dev DSN from .env)
psql "postgresql://sigap:<pw>@127.0.0.1:5433/sigap?sslmode=require" -XA \
  -c 'SELECT email, "emailVerified", "twoFactorEnabled" FROM auth."user";' \
  -c 'SELECT identifier FROM auth.verification;' \
  -c 'SELECT "userId", verified, "failedVerificationCount", "lockedUntil" FROM auth."twoFactor";'
```

The signals to check, in one line each: `emailVerified f → t` after the mailed
link; a reset token that answers `200` once and `400 INVALID_TOKEN` after; a
sign-in answering `{"twoFactorRedirect":true}` with **zero** session rows; and a
backup code that answers `200` once and `401 INVALID_BACKUP_CODE` on replay.
