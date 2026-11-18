# SIGAP auth gap matrix v2 — self-hosted Better Auth identity tier

**Status:** living matrix. Correct as of 2026-11-11, branch
`feat/sigap-selfhosted-auth` @ `a5423ec` (worktree `F:\sigap-design-v2`), with
the B1/B2 hardening applied in this change (JWKS rotation pinned, session
security negatives tested, Go trust-boundary negatives extended, envguard
boundary tests extended).
**Verified state of `apps/web/tests/session-security.test.js`:** GREEN and
SELF-CONTAINED — `Summary: 61 passed, 0 failed, 0 skipped (DB-gated)` against
the dev Postgres on `127.0.0.1:5433`, twice in a row, with no state carried
between runs: every case signs in its own device on a per-run account
(timestamp + random suffix) and the `finally` deletes `auth.user`,
`auth.session` and `app_users` for the ids it created, then ASSERTS the three
tables are empty for them. Two consequences of the product's
`emailAndPassword.autoSignIn: false` shape the suite: sign-up does not mint a
session (asserted), and the session cookie is SIGNED, so a session is only
presentable through the cookie jar the server itself filled.
**Purpose:** reconstruct the REAL gap matrix for the self-hosted auth deployment,
verbatim from the three source documents, then classify every gap so an owner
can pick each one up without re-deriving it.
**Classification keys (mutually exclusive, applied per gap):**
- **CODE-FIXABLE** — fixable in this repository's web/API code or tests.
- **TEST-FIXABLE** — code is correct; what is missing is a test/assertion that
  proves it (once added, the gap is closed).
- **INFRA-BLOCKED** — needs a deployment/host/DB change (TLS, roles, SMTP, a
  shared store, a live drill) that no code edit can substitute.
- **OWNER-BLOCKED** — needs a product/security-owner decision before work can
  proceed; the mechanism is known, the choice is not made.

---

## 1. The nine GAPs from `design/auth-deployment-readiness.md` (verdict table)

Table verbatim (1 READY / 9 GAP):

| # | Item | Status | One-line reason |
|---|---|---|---|
| 1 | HTTPS termination, proxy trust, HSTS | **GAP** | API half is implemented and tested; nothing asserts the web tier's public origin is `https`, and the compose comment claiming `SIGAP_TLS_TERMINATED` drives web HSTS is false. |
| 2 | Cookie security (`Secure`/HttpOnly/SameSite) | **READY** | Library-enforced HttpOnly + SameSite=Lax; `Secure` + `__Secure-` prefix derive from `BETTER_AUTH_URL` being `https`. |
| 3 | CSRF / origin (`trustedOrigins`) | **GAP** | `trustedOrigins` is unset and `BETTER_AUTH_URL` is not even passed by compose → the allow-list is a single fallback origin. |
| 4 | JWT/JWKS key rotation | **GAP** | `jwks.rotationInterval` unset → the ES256 signing key never expires and is never rotated; Go cache bounds retirement pickup at 15 min. |
| 5 | Secrets (`BETTER_AUTH_SECRET`) | **GAP** | A public build placeholder is a silent fallback; compose/web env supply no real secret. |
| 6 | DB backup / restore of the `auth` schema | **GAP** | Whole-DB `pg_dump` covers `auth`, but no restore verification queries `auth.*` — identity-store loss would pass the drill. |
| 7 | Separate `auth`-schema DB privileges | **GAP** | No least-privilege role exists anywhere; Better Auth runs as the same `sigap` role that owns `public` and the ledger. |
| 8 | Rate limiting | **GAP** | Three layers exist (Better Auth native, web form limiter, Go limiter) but all are per-process in-memory. |
| 9 | Audit logs for sign-in / sign-out | **GAP** | Better Auth writes only `auth.session` rows; nothing reaches the Go `audit_events` table. |
| 10 | Production config assertions | **GAP** | `preflight-production-env.sh` asserts the Go/API envelope only; no check covers any auth variable. |

## 2. The two skipped checks from `design/session-lifecycle-policy.md`

1. **No `auth.user.banned` column (identity-boundary deactivation gate).**
   §3 verbatim: better-auth 1.7.20 base config has no ban/status field on
   `auth.user` — `banned`/`banReason`/`banExpires` arrive only with the admin
   plugin, which is not installed, and `sign-in/email` performs no such check
   in the base. `user.validateUserInfo` is skipped for email/password
   sign-ins. Today a deactivated account can still obtain a session and a
   JWT; the Go API refuses to authorize it (RBAC resolved per request on
   `public.app_users.status = 'active' AND deleted_at IS NULL`).
2. **The 1h `freshAge` window cannot be exercised live.** §5: the refresh at
   `updateAge` writes `expiresAt`/`updatedAt` only — `createdAt` is never
   rolled, so freshness is time since sign-in, not activity. The
   `freshSessionMiddleware` gate on `list-sessions`/`delete-user` is asserted
   statically and by source reference only; waiting out 3600 s is not worth a
   test.

## 3. The twelve unverified flow steps from `design/email-mfa-flows-verification.md` §6

1. Real mail delivery (SMTP) — `SIGAP_SMTP_URL` unset; no transport exists;
   capture-only delivery exercised.
2. `requireEmailVerification: true` branch — env gate unset; unverified
   sign-in refusal and verification-mail precondition never observed.
3. Trusted device — `trustDevice: true` never sent; 30-day cookie and
   challenge-skip behaviour untested.
4. Account lockout — `maxFailedAttempts: 10` never driven to ceiling; the
   15-minute lock and per-challenge 5-attempt cap not reached.
5. Verification-link expiry — both MAX_AGE clocks 3600 s; an expired token
   never presented; JSON-body replay variant unobserved.
6. TOTP drift window — adjacent-step codes computed but never submitted.
7. `POST /api/auth/send-verification-email` (resend) not called.
8. `POST /api/auth/two-factor/disable` and `generate-backup-codes` not called.
9. The Go API trust boundary — SvelteKit `getToken` → ES256 JWT → JWKS →
   `app_users.subject` path not exercised end-to-end; MFA-enrolled subject
   mapping unchecked.
10. Concurrency — simultaneous replays of one reset token never fired
    (single-use verified sequentially only).
11. Backup-code exhaustion and encryption strength — AES-GCM-at-rest claim
    only weakly supported; plaintext remaining-count never recovered.
12. Anti-enumeration — known/unknown address pair never submitted back to
    back; byte-identical response and the ≥500 ms floor unmeasured.

---

## 4. Classification

### Gap 1 — HTTPS/proxy/HSTS (web tier) — **INFRA-BLOCKED + CODE-FIXABLE (assertion)**
The API half is READY and envguard-tested (`GuardTLS`, proxy header trust).
The web tier reads no TLS flag: HSTS keys off `event.url.protocol`, cookie
`Secure` keys off `BETTER_AUTH_URL` being `https`, and
`SIGAP_WEB_ORIGIN`/`BETTER_AUTH_URL` default to `http://` in compose. The
assertion part is CODE-FIXABLE (preflight script must require `https://` for
`SIGAP_WEB_ORIGIN` + `BETTER_AUTH_URL` when `SIGAP_ENV` is non-local — the fix
is one line in `scripts/ops/preflight-production-env.sh`, outside this change's
file scope). The actual TLS termination at the edge is INFRA. Restoring the
false compose comment is INFRA/doc.

### Gap 3 — CSRF / origin — **CODE-FIXABLE (web config) + INFRA-BLOCKED (compose env)**
`trustedOrigins` is unset → the allow-list is exactly one fallback origin
(`dist/context/helpers.mjs:60-77`). CODE-FIXABLE: set `trustedOrigins` from a
`BETTER_AUTH_TRUSTED_ORIGINS` env (or accept the default) and add the variable
to `.env.example`. INFRA-BLOCKED: compose's web service env block forwards no
`BETTER_AUTH_*` variable, so a deployed origin list cannot exist until compose
passes it.

### Gap 4 — JWKS rotation — **CODE-FIXABLE (CLOSED in this change)**
`jwks.rotationInterval` was unset (key permanent until manual deletion). Now
pinned to 30 days in `auth.better.ts` with the Go verifier's 15-minute cache
bound documented next to it. Go-side negatives added: rotated-key pickup is
immediate for a new kid; a re-minted kid's old material is refused once the
cache refreshes; the cache TTL constant is asserted to be exactly 15 minutes.
The cross-language half lives in `session-security.test.js` phase 1 and passes
green: the `jwks` block is extracted from its 3-tab nesting depth,
`rotationInterval` evaluates to `60 * 60 * 24 * 30` s, and a window around the
option must name `jwt_provider.go` and "15 minutes" (the retired-key pickup
bound). Remaining (INFRA/ops): the operator rotate procedure
(delete/regenerate the `auth.jwks` row) is a runbook entry, not code.

### Gap 5 — Secrets — **CODE-FIXABLE (CLOSED) + INFRA-BLOCKED (deploy env)**
The code half is already fail-closed: `resolveSecret()` throws at module
construction on a production server without `BETTER_AUTH_SECRET` (asserted by
`auth-config-assertions.test.js`), so the verbatim build-placeholder silent
fallback no longer exists in HEAD. What remains is INFRA-BLOCKED: the compose
web service env block forwards no `BETTER_AUTH_SECRET` and `.env.example` names
none — a production deploy still has no real secret to supply, and nothing
asserts the placeholder is absent on the host (OPERATOR VERIFICATION REQUIRED).

### Gap 6 — `auth`-schema backup/restore verification — **TEST-FIXABLE + INFRA-BLOCKED (drill)**
Whole-DB `pg_dump` covers `auth`, but every critical-table restore check
enumerates `public` tables only — an identity-store loss passes the drill. The
script change (add `auth.*` queries: `auth.user`, `auth.session`, `auth.account`,
`auth.jwks`, `auth.verification`, `auth.twoFactor`) is CODE/TEST-FIXABLE, but
the only proof is a live restore drill, which is INFRA-BLOCKED (needs a
restore target + the dump).

### Gap 7 — `auth`-schema DB privileges — **INFRA-BLOCKED**
Only namespacing exists today; the web service authenticates as the same
`sigap` role that owns `public` and the migration ledger. A least-privilege
role for the `auth` schema (CREATE ROLE + GRANTs, DDL vs DML split) is a
Postgres/DB-admin change; no code edit can substitute. (Do NOT touch auth
schema privileges in code — infra.)

### Gap 8 — Rate limiting durability — **INFRA-BLOCKED + OWNER-BLOCKED (store choice)**
All three layers are per-process in-memory by deliberate design
(`rate-limit.ts:9-12`, `limiter.go:13-16`); a multi-replica deployment will not
share buckets. The fix is a durable store (Redis/Postgres) — infra, and the
store choice is an owner decision. Better Auth's native storage option
(`rateLimit.storage`) is a config change once the store exists.

### Gap 9 — Sign-in/out audit — **OWNER-BLOCKED (schema/chain decision)**
Better Auth emits no event that the Go `audit_events` table can see. Mechanism
is known (A: `databaseHooks.session.create.after` / sign-out hook in the web
tier writing audit rows; B: an internal API audit endpoint the web calls; C: a
log shipper). The decision is which owner writes to the hash-chained Go audit
service and with what identity — an owner call, not a code gap. Until then,
`auth.session` rows remain the only record.

### Gap 10 — Production config assertions — **CODE-FIXABLE (preflight) + TEST-FIXABLE (Go side, tripwire added)**
`preflight-production-env.sh` asserts the Go/API envelope only. The required
assertion set (MUST be set non-empty in non-local: `BETTER_AUTH_SECRET`,
`BETTER_AUTH_URL` https, `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`,
`SIGAP_AUTH_JWKS_URL`) is CODE-FIXABLE in that script. Go-side, `envguard_test.go`
now carries a tripwire asserting the auth/web env vars are deliberately NOT yet
guarded by `GuardDevCapabilities`/`GuardTLS`, so a future guard flip is
noticed. The script itself remains the unedited owner item.

### session-lifecycle-policy skipped check 1 — `auth.user.banned` — **OWNER-BLOCKED + INFRA-BLOCKED (migration)**
The product question (session-lifecycle-policy.md §3) is unanswered: should a
deactivated account see an authentication error (revealing provisioning state)
or a successful sign-in that authorizes nothing? Today is the latter, and the
compensating control (Go RBAC per request) is tested and documented. If a hard
identity-boundary gate is wanted, the two supported mechanisms are the
`databaseHooks.session.create.before` SQL gate (CODE-FIXABLE) or the admin
plugin + `banned` (INFRA: schema migration). The choice is OWNER-BLOCKED; the
gap itself is now characterized live and PASSING in
`session-security.test.js` phase 2 — two labelled `GAP CHARACTERIZATION`
assertions: a disabled `app_users` row still signs in, and the resulting
session resolves to that same user at the identity layer. The characterization
is CLOSED; the product gap is not.

### session-lifecycle-policy skipped check 2 — 1h freshAge window — **TEST-FIXABLE (static, done)**
Exercising the gate live requires waiting out `freshAge` (3600 s); the
`freshSessionMiddleware` gate is asserted statically (`session-lifecycle.test.js`
phase 1) and by source reference. Nothing more is actionable — treating it as
closed by static assertion.

### email-mfa-flows-verification §6 items — **per-item classes in §7.3 (1–3 INFRA/OWNER, 4–8 + 9–12 TEST-FIXABLE)**
- 1 (SMTP): INFRA-BLOCKED — needs a transport (decision: which provider) — do
  NOT invent SMTP in code.
- 2 (requireEmailVerification): OWNER-BLOCKED — the env gate exists
  (`SIGAP_REQUIRE_EMAIL_VERIFICATION`); flipping it before a transport exists
  locks every account out. Decision first.
- 3 (trusted device): INFRA-BLOCKED/ops (cookie + flow) — needs a device and a
  decision that staff may use trusted devices.
- 4 (lockout): TEST-FIXABLE — the config is set (`accountLockout`); driving 10
  failed attempts against a live account would close it.
- 5 (verification-link expiry): TEST-FIXABLE — present a >1h token (need a
  way to time-travel or wait); low priority.
- 6 (TOTP drift window): TEST-FIXABLE — submit adjacent-step codes against the
  live verifier; low priority.
- 7 (resend endpoint): TEST-FIXABLE — call `send-verification-email` live.
- 8 (disable / generate-backup-codes): TEST-FIXABLE — call both live with a
  fresh session + password.
- 9 (Go trust boundary for MFA subjects): TEST-FIXABLE — extend the selfhosted
  e2e or the Go `better_auth_compat_test.go` path with an MFA-enrolled subject
  (Go-side negatives for issuer/audience/expiry/rotation are now covered).
- 10 (concurrency): TEST-FIXABLE — fire two simultaneous replay requests of one
  reset token.
- 11 (backup-code exhaustion/encryption): TEST-FIXABLE — spend codes down to
  zero and assert the envelope/column cannot recover plaintext; the encryption
  claim is already partially evidenced.
- 12 (anti-enumeration): TEST-FIXABLE — submit a known/unknown pair back to
  back and assert byte-identical responses + the response-time floor.

---

## 5. Session security — what is now tested

Both files were green on 2026-11-11 against :5433;
`session-security.test.js` reported `Summary: 61 passed, 0 failed, 0 skipped
(DB-gated)` twice in a row, with zero fixture residue (asserted).

| Negative | Where | State |
|---|---|---|
| Logout (sign-out deletes the row; the SIGNED pre-sign-out cookie is replayed and rejected — `get-session` 200 null, `/token` 401) | `session-lifecycle.test.js` phase 3 + `session-security.test.js` phase 2 | tested |
| Expiration (past `expiresAt` ⇒ get-session null, `/token` 401; liveness asserted first, so the null is not a never-worked session) | `session-security.test.js` phase 2 | tested |
| Refresh (`updateAge` roll-forward write on stale sessions) | `session-security.test.js` phase 2 | tested |
| Revocation (self single-session revoke deletes the row and kills its cookie; all; other-sessions; no new mint after) | `session-lifecycle.test.js` + `session-security.test.js` phase 2 | tested |
| Cross-user revocation isolation (a foreign token is a silent no-op; the target session survives, presented through its own jar) | `session-security.test.js` phase 2 | tested |
| Session switching (per-device sessions; revoke-other-sessions kills only the other device) | `session-lifecycle.test.js` + `session-security.test.js` phase 2 | tested |
| Password-change invalidation (current password mandatory; revokeOtherSessions; old password dead AND new password live as a positive control; default leaves other devices live) | `session-lifecycle.test.js` + `session-security.test.js` phase 2 | tested |
| Privileged-staff deactivation (identity gap characterized live by two labelled assertions; Go RBAC enforcement asserted statically) | `session-security.test.js` phase 2 + `rbac_resolver_test.go` (existing) | tested (characterization) |
| Short-lived-JWT policy (access JWT TTL exactly 3600 s; zero leeway in Go) | `session-lifecycle.test.js` + `better_auth_compat_test.go` (expired test) | tested |
| JWKS rotation (interval pinned at 30 d; Go pickup/retirement contract; cache TTL = 15 min) | `auth.better.ts` (code) + `session-security.test.js` phase 1 + `better_auth_compat_test.go` | tested |
| Wrong issuer / wrong audience (Go negatives) | `better_auth_compat_test.go` (existing) | tested |
| Suite self-containment (per-run accounts; `auth.user` / `auth.session` / `app_users` deleted and the empty result asserted) | `session-security.test.js` phase 2 `finally` | tested |
| `autoSignIn: false` consequence (sign-up mints no session — the anti-enumeration shape the fixture depends on) | `session-security.test.js` phase 2 | tested |

## 6. What remains infra/owner-blocked (summary)

- INFRA-BLOCKED: TLS at the edge + the https-origin assertion (gap 1); compose
  `BETTER_AUTH_*` env forwarding (gaps 3, 5); real SMTP (MFA 1); auth-schema
  least-privilege role (gap 7); durable rate-limit store (gap 8); live restore
  drill for `auth.*` (gap 6); operator JWKS rotate procedure (gap 4, runbook).
- OWNER-BLOCKED: audit-event owner/chain decision (gap 9); deactivation-at-
  identity gate choice (policy skip 1); `requireEmailVerification` flip (MFA
  2); trusted-device policy (MFA 3).
- TEST-FIXABLE (open, not yet added): MFA §6 items 4–12 (live, cheap, in scope
  of a future run).

## 7. Closure ledger — every item, one line, with its class

Class keys: **CODE** = CODE-FIXABLE, **TEST** = TEST-FIXABLE,
**INFRA** = INFRA-BLOCKED, **OWNER** = OWNER-BLOCKED. CLOSED = no further work
is needed on that item as classified.

### 7.1 The nine GAPs of `auth-deployment-readiness.md` (row 2, cookie security, is READY — not a gap)

| # | Item | Class | State |
|---|---|---|---|
| 1 | HTTPS termination / proxy trust / HSTS (web tier) | INFRA (edge TLS) + CODE (https-origin assertion in `preflight-production-env.sh`, outside this change's file scope) | OPEN |
| 3 | CSRF / origin (`trustedOrigins`) | CODE (web config) + INFRA (compose must forward `BETTER_AUTH_*`) | OPEN |
| 4 | JWT/JWKS key rotation | CODE | **CLOSED** (30 d interval + comment; phase-1 assertions; Go rotation/TTL negatives) |
| 5 | Secrets (`BETTER_AUTH_SECRET`) | CODE + INFRA (deploy env) | CODE half **CLOSED** (fail-closed `resolveSecret()`, `auth-config-assertions.test.js`); deploy-env half OPEN |
| 6 | `auth`-schema backup / restore verification | TEST (script) + INFRA (live drill) | OPEN |
| 7 | Separate `auth`-schema DB privileges | INFRA | OPEN |
| 8 | Rate limiting durability | INFRA + OWNER (store choice) | OPEN |
| 9 | Audit logs for sign-in / sign-out | OWNER (owner/chain decision) | OPEN |
| 10 | Production config assertions | CODE (preflight script) + TEST (Go tripwire) | Go tripwire **CLOSED** (`envguard_test.go`); preflight script OPEN |

### 7.2 The two skipped checks of `session-lifecycle-policy.md`

| # | Item | Class | State |
|---|---|---|---|
| 1 | No `auth.user.banned` identity-boundary deactivation gate | OWNER (choice) + INFRA (admin-plugin migration if chosen) | product gap OPEN; the live characterization is **CLOSED** (two passing `GAP CHARACTERIZATION` assertions) |
| 2 | The 1 h `freshAge` window cannot be exercised live | TEST (static only) | **CLOSED** (asserted statically; waiting 3600 s is not worth a test) |

### 7.3 The twelve unverified flow steps of `email-mfa-flows-verification.md` §6

| # | Item | Class | State |
|---|---|---|---|
| 1 | Real mail delivery (SMTP) | INFRA (provider decision) | OPEN |
| 2 | `requireEmailVerification: true` branch | OWNER | OPEN |
| 3 | Trusted device | INFRA / ops | OPEN |
| 4 | Account lockout | TEST | OPEN |
| 5 | Verification-link expiry | TEST | OPEN |
| 6 | TOTP drift window | TEST | OPEN |
| 7 | `send-verification-email` (resend) | TEST | OPEN |
| 8 | `two-factor/disable` + `generate-backup-codes` | TEST | OPEN |
| 9 | Go API trust boundary (SvelteKit → ES256 JWT → JWKS → `app_users.subject`) | TEST | Go JWT half **CLOSED** (issuer / audience / expiry / rotation negatives in `better_auth_compat_test.go`); MFA-enrolled subject mapping OPEN |
| 10 | Concurrency (simultaneous reset-token replays) | TEST | OPEN |
| 11 | Backup-code exhaustion / encryption strength | TEST | OPEN |
| 12 | Anti-enumeration (known/unknown pair) | TEST | OPEN |

### 7.4 Closed by this change

- Gap 4 (rotation pinned + asserted), Gap 5's code half, Gap 10's Go tripwire.
- Policy skip 1's characterization and skip 2 entirely.
- MFA §6 item 9's Go trust-boundary half.
- The ten session-security negatives of §5, now green and self-contained
  (`61 passed, 0 failed, 0 skipped`, twice, zero residue).

### 8. Post-integration E2E evidence (2026-11-18, integrated dev HEAD)
- **Real Better Auth schema provisioned** on a disposable local Postgres through the
  project's official `getMigrations(auth.options)` API (`better-auth/db/migration`): six
  `auth` tables (`user`, `session`, `account`, `verification`, `jwks`, `twoFactor`), no
  invented DDL. SIGAP migrations 0001–0014 were applied with the real Go runner first.
- **Real authenticated browser journey passed 4/4** (`selfhosted-auth-journey.spec.ts`,
  `-Jwt -NoAdminActor` preview, no local actor): patient A's two cross-facility cards +
  real cancel, patient B isolation + denied cancel, org-A admin seeing exactly one org
  with a sibling org id refused, super admin seeing both orgs. Real form sign-in →
  Better Auth session → ES256 JWT/JWKS → Go authorization on every case.
- **Live session suites on the migrated auth schema:**
  `session-security.test.js` 61 passed / 0 failed / 0 skipped;
  `session-lifecycle.test.js` 61 passed / 0 failed / 2 documented skips (no `auth.user`
  banned column; the 1 h `freshAge` window is not waited out). Both suites' drift was
  corrected in the tests, not by weakening a security assertion: lifecycle now signs in
  explicitly (`autoSignIn: false`) and selects the "other device" by the sign-in token.
- **Product status unchanged:** passing negative tests and real E2E journeys do NOT make
  the overall session lifecycle or staff MFA READY — the owner/infra gaps above (SMTP,
  MFA recovery, TLS edge, secrets, least-privilege DB role, audit sink) remain open.
