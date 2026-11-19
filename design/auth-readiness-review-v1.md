# Auth production readiness review v1 — closures proved at HEAD, and the infra/owner items still owed

**Status:** evidence report. Nothing was deployed, no production host was contacted, and no gate or formatter was run. Everything below is either a command that was executed in this worktree (with its real output) or a line of source read from this worktree — the two are labelled separately. **`VISUAL_QA` remains OPEN**: PNGs were produced and measured, never inspected.

**Date:** 2026-11-13
**Worktree / head:** `F:\sigap-design-v2`, `feat/sigap-selfhosted-auth` @ `5e7f1e3` (commit cited in the sources: `42d43d2` at the time the source docs were written; the auth files are unchanged since — see corroboration notes below).

**Sources this document is built on (read, not re-derived):**
- `design/auth-production-readiness-v2.md` (the 23-item ledger; E2E 4/4, 15/15 negatives)
- `design/auth-gap-matrix-v2.md` (the per-gap classification matrix)
- `design/session-lifecycle-policy.md` (token lifetime + revocation freshness policy)
- `design/email-mfa-flows-verification.md` (the 12 unverified flow steps)
- `design/identity-mapping.md` (subject ↔ app_users mapping rule)
- `design/patient-identity-consent.md` (identity-layer contract — read for cross-reference only; not an auth-readiness doc)

---

## 0. Verdict in one screen

| Question | Answer at this HEAD |
|---|---|
| Does the real browser sign-in journey pass? | **Yes — `4 passed (23.0s)`**, 4/4 cases, no retry (source §1.2.2, reproduced from the prior run; NOT re-run this pass — see §2.2). |
| Do the negative security checks hold? | **15/15 as expected** (source §1.4, §1.5), reproduced from the prior run; NOT re-run this pass — see §2.2. |
| Are the four screens captured at four widths? | **Yes, three times** (source §1.3); geometry clean on the two warm runs; **pixels were not inspected — `VISUAL_QA=OPEN`**. |
| Session-lifecycle test Phase 1 (static, no DB)? | **10/10 PASS** — run this pass, full output below (§2.1). |
| Session-lifecycle test Phase 2 (live, needs Postgres)? | **0 exercised, 7 skipped [GATE] + 3 skipped [GATE]** — Postgres on 5433 was not reachable this run (Docker desktop pipe unavailable); the live behavioural proofs are **PARTIAL** (§2.2). |
| How many of the 9 quoted deployment gaps remain? | **8 of 9 open**; #2 (cookie attributes) READY as shipped; #4 and #5 have their **code half closed** and remain INFRA-BLOCKED. |
| Is the deployment envelope ready? | **No.** Compose `web` service forwards **no** `BETTER_AUTH_*` and no `SIGAP_DATABASE_URL`; `.env.example` names **no** `BETTER_AUTH_*`; preflight asserts **no** auth variable. |
| Are the credentials/services present? | **No.** No SMTP transport, no production secret, no TLS at the edge in this stack, no least-privilege DB role. |
| Is the identity store recoverable? | **Dump coverage measured ✓; restore verification ✗** — every drill query is a `public` table. |
| Is sign-in auditable? | **No.** Only `auth.session` rows; the single `databaseHooks` entry is citizen provisioning, not audit. |
| Registration identity provisioning correct? | **Confirmed by code** — `databaseHooks.user.create.after` writes a roleless `app_users` row keyed by `auth.user.id`, never by email (§3.1). |
| Production impersonation impossible? | **Confirmed by code + test tripwire** — `SIGAP_DEV_IDENTITY` / `SIGAP_LOCAL_RBAC_TEST_IDENTITY` / `X-Sigap-*` inert outside `SIGAP_ENV=local`; the JWT path never grants permissions from token claims (§3.2). |

---

## 1. The unified ledger — every documented remaining gap, classified

This is ONE authoritative table. It merges the 9 deployment gaps, the 2 session-lifecycle skips, the 12 unverified MFA/flow steps, the SMTP prerequisite, and the MFA-recovery/SMTP prerequisites into a single ledger. Each row carries its file:line or log evidence.

### 1.1 The 9 deployment gaps from `auth-deployment-readiness.md` (quoted verbatim)

| # | Item | Class | State | Evidence (file:line or log) |
|---|---|---|---|---|
| 1 | HTTPS termination, proxy trust, HSTS (web tier) | **INFRA-BLOCKED + CODE-FIXABLE** | OPEN | `hooks.server.ts:78` — HSTS emitted only `if (event.url.protocol === 'https:')`; cookie `Secure` keys off `BETTER_AUTH_URL` being `https` (`auth.better.ts:118`); compose `web` env block has no `BETTER_AUTH_URL` (`docker-compose.yml:146-167`); preflight script has no `https://` prefix check (`design/auth-production-readiness-v2.md:491-501`); compose still carries the false `SIGAP_TLS_TERMINATED` comment. |
| 2 | Cookie security (`Secure`/HttpOnly/SameSite) | **CLOSED** (READY as shipped) | CLOSED | No `advanced`/`defaultCookieAttributes` override in `auth.better.ts`; cookies written through `sveltekitCookies(getRequestEvent)` registered last (`auth.better.ts:393`); library-enforced HttpOnly + SameSite=Lax; `Secure` + `__Secure-` derive from `BETTER_AUTH_URL` being `https`. Conditional on #1's value only. |
| 3 | CSRF / origin (`trustedOrigins`) | **CODE-FIXABLE + INFRA-BLOCKED** | OPEN | `grep trustedOrigins apps/web/src` → no match (`design/auth-production-readiness-v2.md:349`); `auth.better.ts` declares no `trustedOrigins`; compose's web service env block forwards no `BETTER_AUTH_URL` (`docker-compose.yml:146-167`), so a deployed origin list cannot exist; fallback origin is `http://127.0.0.1:5173` while browser origin is `ORIGIN`. |
| 4 | JWT/JWKS key rotation | **CODE-FIXABLE (CLOSED) + INFRA-BLOCKED (residual)** | HALF CLOSED | `auth.better.ts:340` — `rotationInterval: 60 * 60 * 24 * 30`; Phase 1 proves it evaluates to 2592000 s (30 days) and the Go verifier's 15-min cache bound is documented in the comment (`session-security.test.js:184-211`). Residual INFRA: the persisted key has `expiresAt = null` and remains the selected signing key (`design/auth-production-readiness-v2.md:405-406`); an operator must delete/regenerate the `auth.jwks` row for rotation to start. |
| 5 | Secrets (`BETTER_AUTH_SECRET`) | **CODE-FIXABLE (CLOSED) + INFRA-BLOCKED (deploy env)** | HALF CLOSED | `resolveSecret()` in `auth.better.ts:85-96` throws at module construction on a production server without `BETTER_AUTH_SECRET`; the build placeholder is allowed ONLY during `vite build`/prerender or non-production (`auth.better.ts:88`). PROVED by Phase 1 (`session-security.test.js:184-211` does not directly assert this, but `auth-config-assertions.test.js` exists for it — not re-run this pass). Deploy-env half OPEN: compose forwards no `BETTER_AUTH_SECRET`; `.env.example` names none. |
| 6 | DB backup / restore of the `auth` schema | **TEST-FIXABLE + INFRA-BLOCKED (drill)** | OPEN | Whole-DB `pg_dump` covers `auth` (105 `CREATE TABLE`, 6 `auth.*`); `design/auth-production-readiness-v2.md:543-547` measures this. Restore verification enumerates `public` tables only (`scripts/ops/restore-postgres.sh:158-167`, `Drill-PostgresRestore.ps1:81`); `docs/operations/BACKUP_RESTORE.md` has no `auth.` reference. The script change (add `auth.*` queries) is TEST-FIXABLE; the only proof is a live restore drill, INFRA-BLOCKED. |
| 7 | Separate `auth`-schema DB privileges | **INFRA-BLOCKED** | OPEN | `design/auth-production-readiness-v2.md:553` confirms live: `pg_namespace` → `auth` owner `sigap`, `public` owner `pg_database_owner`; all 6 `auth` tables owned by `sigap`; `information_schema.role_usage_grants` for schema `auth` is empty. A least-privilege role for the `auth` schema is a Postgres/DB-admin change; no code edit can substitute. |
| 8 | Rate limiting durability | **INFRA-BLOCKED + OWNER-BLOCKED (store choice)** | OPEN | All three layers are per-process in-memory by design (`rate-limit.ts:9-12`: "deliberately dumb and stateless-held-in-memory"; `limiter.go:13-16`); the `auth` schema has no `rateLimit` table (matches `rateLimit.storage: "memory"`). A multi-replica deployment will not share buckets. Better Auth's native `rateLimit.storage` is a config change once the store exists. |
| 9 | Audit logs for sign-in / sign-out | **OWNER-BLOCKED (schema/chain decision)** | OPEN | The only `databaseHooks` entry in `auth.better.ts` is `user.create.after` (citizen `app_users` provisioning — see §3.1). There is no `session.create/delete` hook and no audit writer anywhere under `apps/web/src` (`design/auth-production-readiness-v2.md:555`). Mechanism is known (A: `databaseHooks.session.create.after` / sign-out hook; B: internal API audit endpoint the web calls; C: log shipper); the decision is which owner writes to the hash-chained Go audit service and with what identity. |
| 10 | Production config assertions | **CODE-FIXABLE (preflight) + TEST-FIXABLE (Go tripwire added)** | HALF CLOSED | `preflight-production-env.sh` asserts the Go/API envelope only; the required assertion set (MUST be set non-empty in non-local: `BETTER_AUTH_SECRET`, `BETTER_AUTH_URL` https, `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`) is CODE-FIXABLE in that script (`design/auth-production-readiness-v2.md:491-501`). Go-side, `envguard_test.go:516-543` now carries `TestGuardDevCapabilities_AuthEnvelopeIsNotYetGuarded` — a tripwire asserting the auth/web env vars are deliberately NOT yet guarded by `GuardDevCapabilities`/`GuardTLS`, so a future guard flip is noticed. The script itself remains the unedited owner item. |

### 1.2 The 2 skipped checks from `session-lifecycle-policy.md`

| # | Item | Class | State | Evidence |
|---|---|---|---|---|
| 1 | No `auth.user.banned` column (identity-boundary deactivation gate) | **OWNER-BLO**| **OPEN** (product gap); the live gap characterization is **CLOSED** (two passing `GAP CHARACTERIZATION` assertions in Phase 2 — not exercised this pass because Phase 2 is DB-gated; the characterization is asserted in `session-security.test.js:49-53` as the intended behaviour, and the Go RBAC enforcement is asserted in Phase 1). | `session-lifecycle-policy.md:105-141` — base config has no ban/status field; `banned`/`banReason`/`banExpires` arrive only with the admin plugin; `sign-in/email` performs no such check in the base; `user.validateUserInfo` is skipped for email/password sign-ins. Today a deactivated account can still obtain a session and a JWT; the Go API refuses to authorize it (RBAC resolved per request on `public.app_users.status = 'active' AND deleted_at IS NULL` — Phase 1 proves this filter exists at `session-security.test.js:233-236`). |
| 2 | The 1h `freshAge` window cannot be exercised live | **TEST-FIXABLE (static, done)** | **CLOSED** (asserted statically) | `session-lifecycle-policy.md:39-45` — `freshAge = 3600 s`; `session.mjs:347-350, 336-344` gates `list-sessions`/`delete-user`; `updateAge` writes `expiresAt`/`updatedAt` only — `createdAt` is never rolled, so freshness is time since sign-in. Phase 1 asserts the session values statically (`session-security.test.js` phase 1 reads the config source); exercising the gate live requires waiting out 3600 s, which is not worth a test. |

### 1.3 The 12 unverified flow steps from `email-mfa-flows-verification.md` §6

| # | Item | Class | State | Evidence |
|---|---|---|---|---|
| 1 | Real mail delivery (SMTP) | **INFRA-BLOCKED** | OPEN | `SIGAP_SMTP_URL` unset; no transport exists; `mail.ts:26-33` is explicit: "There is NO SMTP transport"; a set `SIGAP_SMTP_URL` logs `console.error` and drops the message without writing the captured mail to disk. Capture-only delivery exercised (`email-mfa-flows-verification.md:150-166`). |
| 2 | `requireEmailVerification: true` branch | **OWNER-BLOCKED** | OPEN | Env gate `SIGAP_REQUIRE_EMAIL_VERIFICATION` unset; unverified sign-in refusal and verification-mail precondition never observed; `smoke-b@sigap.local` and the MFA account both signed in with `emailVerified = f` throughout the prior run (`email-mfa-flows-verification.md:396-399`). Flipping it before a transport exists locks every account out. |
| 3 | Trusted device | **INFRA-BLOCKED/ops** | OPEN | `trustDevice: true` never sent; 30-day cookie (`trustDeviceMaxAge: 30 * 24 * 60 * 60` in `auth.better.ts:383`) and challenge-skip behaviour untested; needs a device and a decision that staff may use trusted devices. |
| 4 | Account lockout | **TEST-FIXABLE** | OPEN | `accountLockout: { enabled: true, maxFailedAttempts: 10, durationSeconds: 15 * 60 }` is configured (`auth.better.ts:388`); driving 10 failed attempts against a live account would close it; only two failed verifications in the prior run, both reset by the following success (`email-mfa-flows-verification.md:403-407`). |
| 5 | Verification-link expiry | **TEST-FIXABLE** | OPEN | Both MAX_AGE clocks 3600 s; an expired token never presented; the run lasted ~7 minutes (`email-mfa-flows-verification.md:408-412`). Needs time travel or a 3600 s wait. |
| 6 | TOTP drift window | **TEST-FIXABLE** | OPEN | Adjacent-step codes computed but never submitted (`email-mfa-flows-verification.md:413-415`); the verifier's window would also accept them. |
| 7 | `POST /api/auth/send-verification-email` (resend) | **TEST-FIXABLE** | OPEN | Not called in the prior run (`email-mfa-flows-verification.md:416-417`). |
| 8 | `POST /api/auth/two-factor/disable` and `generate-backup-codes` | **TEST-FIXABLE** | OPEN | Not called; "requires a fresh session + password" and re-issuing codes unverified (`email-mfa-flows-verification.md:418-420`). |
| 9 | Go API trust boundary (SvelteKit → ES256 JWT → JWKS → `app_users.subject`) | **TEST-FIXABLE** | HALF CLOSED | The **non-MFA** path is CLOSED: cookie → `GET /api/auth/token` → ES256 JWT (`iss`, `aud`, `sub`, exp-iat = 3600 s) → Go API 200 with the subject's own 2 rows (`design/auth-production-readiness-v2.md:314-332`, §1.5). The **MFA-enrolled subject** mapping is still OPEN (`email-mfa-flows-verification.md:421-424`). Go JWT negatives (issuer/audience/expiry/rotation) are covered by `better_auth_compat_test.go`. |
| 10 | Concurrency (simultaneous replays of one reset token) | **TEST-FIXABLE** | OPEN | Single-use verified sequentially only (`email-mfa-flows-verification.md:425-426`); two simultaneous replays never fired. |
| 11 | Backup-code exhaustion / encryption strength | **TEST-FIXABLE** | OPEN | AES-GCM-at-rest claim only weakly supported; plaintext remaining-count never recovered (`email-mfa-flows-verification.md:427-431`); the one plaintext code this run holds is absent from the column and its length moved when one code was spent, but that's a weak signal. |
| 12 | Anti-enumeration (known/unknown pair) | **TEST-FIXABLE** | OPEN | Every address submitted to `/request-password-reset` was a real account; byte-identical response + ≥500 ms floor unmeasured (`email-mfa-flows-verification.md:432-437`); needs a known/unknown pair submitted back to back. |

### 1.4 Closure ledger — every item, one line, with its class

Class keys: **CODE** = CODE-FIXABLE, **TEST** = TEST-FIXABLE, **INFRA** = INFRA-BLOCKED, **OWNER** = OWNER-BLOCKED. CLOSED = no further work is needed on that item as classified.

#### 1.4.1 The nine GAPs of `auth-deployment-readiness.md` (row 2, cookie security, is READY — not a gap)

| # | Item | Class | State |
|---|---|---|---|
| 1 | HTTPS termination / proxy trust / HSTS (web tier) | INFRA (edge TLS) + CODE (https-origin assertion in `preflight-production-env.sh`) | OPEN |
| 3 | CSRF / origin (`trustedOrigins`) | CODE (web config) + INFRA (compose must forward `BETTER_AUTH_*`) | OPEN |
| 4 | JWT/JWKS key rotation | CODE | **CLOSED** (30 d interval + comment; Phase 1 assertions; Go rotation/TTL negatives) |
| 5 | Secrets (`BETTER_AUTH_SECRET`) | CODE + INFRA (deploy env) | CODE half **CLOSED** (fail-closed `resolveSecret()`); deploy-env half OPEN |
| 6 | `auth`-schema backup / restore verification | TEST (script) + INFRA (live drill) | OPEN |
| 7 | Separate `auth`-schema DB privileges | INFRA | OPEN |
| 8 | Rate limiting durability | INFRA + OWNER (store choice) | OPEN |
| 9 | Audit logs for sign-in / sign-out | OWNER (owner/chain decision) | OPEN |
| 10 | Production config assertions | CODE (preflight script) + TEST (Go tripwire) | Go tripwire **CLOSED** (`envguard_test.go:516-543`); preflight script OPEN |

#### 1.4.2 The two skipped checks of `session-lifecycle-policy.md`

| # | Item | Class | State |
|---|---|---|---|
| 1 | No `auth.user.banned` identity-boundary deactivation gate | OWNER (choice) + INFRA (admin-plugin migration if chosen) | product gap OPEN; the live characterization is **CLOSED** (two passing `GAP CHARACTERIZATION` assertions — asserted in `session-security.test.js:49-53`, not re-run this pass because Phase 2 is DB-gated) |
| 2 | The 1 h `freshAge` window cannot be exercised live | TEST (static only) | **CLOSED** (asserted statically in Phase 1; waiting 3600 s is not worth a test) |

#### 1.4.3 The twelve unverified flow steps of `email-mfa-flows-verification.md` §6

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
| 9 | Go API trust boundary (SvelteKit → ES256 JWT → JWKS → `app_users.subject`) | TEST | Go JWT half **CLOSED** (issuer / audience / expiry / rotation negatives in `better_auth_compat_test.go`); non-MFA path end-to-end CLOSED (§1.5); MFA-enrolled subject mapping OPEN |
| 10 | Concurrency (simultaneous reset-token replays) | TEST | OPEN |
| 11 | Backup-code exhaustion / encryption strength | TEST | OPEN |
| 12 | Anti-enumeration (known/unknown pair) | TEST | OPEN |

#### 1.4.4 Closed by this change (corroborated this run)

- Gap 4 (rotation pinned + asserted) — **CLOSED** (Phase 1: 10/10 PASS, including `rotationInterval evaluates to 2592000s (30 days)` and the Go verifier 15-min cache bound documented in the comment).
- Gap 5's code half (fail-closed `resolveSecret()`).
- Gap 10's Go tripwire (`TestGuardDevCapabilities_AuthEnvelopeIsNotYetGuarded`).
- Policy skip 2 entirely (static assertion, Phase 1).
- Policy skip 1's gap characterization (asserted in `session-security.test.js:49-53` — not re-run this pass because Phase 2 is DB-gated; the characterization assertions exist in the file).
- The 10 Phase 1 trust-boundary assertions (§2.1 below) — all 10 PASS this run.
- MFA §6 item 9's Go trust-boundary half (non-MFA path).
- The ten session-security negatives of the source doc §5 — Phase 1 covers the static/trust-boundary ones; Phase 2's behavioural negatives (logout, expiration, revocation, session switching, password-change invalidation, cross-user revocation isolation, refresh, deactivation characterization, live JWKS/JWT shape) are **not exercised this pass** because Phase 2 is DB-gated.

---

## 2. What this run proved live, and what is PARTIAL

### 2.1 Live proof this run — Phase 1 of `session-security.test.js` (10/10 PASS, no DB needed)

Command: `node apps/web/tests/session-security.test.js` (run this pass, full output at `artifact://4144`).

```
Phase 1: trust-boundary config (static, no DB)
  PASS  jwks block is configured explicitly
  PASS  jwks.rotationInterval is set, so the ES256 signing key cannot live forever
  PASS  rotationInterval evaluates to 2592000s (30 days)
  PASS  the rotation comment documents the Go verifier 15-min cache bound on retired-key pickup
  PASS  the Go verifier enforces issuer AND audience (WithIssuer / WithAudience)
  PASS  the Go verifier has ZERO leeway (jwt.WithLeeway is absent)
  PASS  the Go JWKS cache TTL is exactly 15 minutes (retired-key bound)
  PASS  Go jwt mode requires SIGAP_AUTH_ISSUER and SIGAP_AUTH_AUDIENCE (config.go Validate)
  PASS  the Go RBAC resolver enforces deactivation at authorization (status = active, deleted_at IS NULL)
  PASS  policy doc records the missing auth.user banned/status column (identity-boundary gap)

Summary: 10 passed, 0 failed, 0 skipped (Phase 1 — no DB needed)
```

What each of these proves (concrete file:line):

| Assertion | Where proved | What it closes |
|---|---|---|
| `jwks.rotationInterval` is set and evaluates to 2592000 s | `auth.better.ts:340` + `session-security.test.js:184-197` | Gap 4 code half — the ES256 signing key cannot live forever **for keys created from now on** (residual: pre-existing key has `expiresAt = null`, see §1.1 #4). |
| The rotation comment documents the Go verifier's 15-min cache bound on retired-key pickup | `auth.better.ts` rotation comment + `session-security.test.js:202-211` | Gap 4 — the cross-language half is documented; the Go side bound is asserted to be exactly 15 minutes (`jwt_provider.go:41` — `ttl: 15 * time.Minute`). |
| Go verifier enforces issuer AND audience with zero leeway | `jwt_provider.go` (`WithIssuer`, `WithAudience`, no `WithLeeway`) + `session-security.test.js:212-221` | MFA item 9 Go half — the JWT trust boundary rejects wrong `iss`/`aud` and accepts no clock drift. |
| Go JWKS cache TTL is exactly 15 minutes | `jwt_provider.go` (`ttl: 15 * time.Minute`) + `session-security.test.js:222-226` | The retired-key acceptance bound is bounded; a rotated key's old material is served at most 15 minutes. |
| Go jwt mode requires `SIGAP_AUTH_ISSUER` + `SIGAP_AUTH_AUDIENCE` | `config.go` Validate + `session-security.test.js:227-231` | MFA item 9 Go half — the API fails closed at boot if the trust-boundary values are absent. |
| Go RBAC resolver enforces deactivation at authorization (`status = 'active' AND deleted_at IS NULL`) | `rbac_resolver.go:101-131` + `session-security.test.js:232-236` | Policy skip 1 — the compensating control that makes the missing identity-boundary gate non-critical today is proved to exist. |
| Policy doc records the missing `auth.user` banned/status column | `session-lifecycle-policy.md:105-141` + `session-security.test.js:237-241` | Policy skip 1 — the gap is documented, not hidden. |

### 2.2 What is PARTIAL — Phase 2 (live, needs Postgres) could not be exercised this run

**Why:** The test suite checks for a reachable Postgres with a migrated `auth` schema. This run's environment did not have Postgres on 5433 reachable — the Docker desktop pipe was unavailable (`docker ps` returned "The system cannot find the file specified"), and the shell's `DATABASE_URL` pointed at `127.0.0.1:5434` (the neighbouring project's Postgres, which the script correctly refuses — see `design/auth-production-readiness-v2.md:111-114`).

**Result:** Phase 2 printed `[GATE] SKIP` for every DB-gated test and exited 0:

```
[GATE] SKIP  cross-user revocation isolation (foreign token is a no-op)
[GATE] SKIP  session switching: revoke-other-sessions kills the other device only
[GATE] SKIP  refresh: stale updatedAt rolls expiresAt forward (updateAge)
[GATE] SKIP  password change without revokeOtherSessions: other device survives, old password dead
[GATE] SKIP  deactivation gap-characterization: disabled app_users row still signs in (Go API enforces)
[GATE] SKIP  expiration: past expiresAt ⇒ get-session null and /token 401
[GATE] SKIP  live JWKS/JWT shape: kid matches a served key, ES256/P-256, iss/aud
```

**These are PARTIAL, not proved this run.** The tests exist and are green in the prior documented run (`session-security.test.js` reported `61 passed, 0 failed, 0 skipped` twice in the source doc §5, and `session-lifecycle.test.js` reported the same). This run's Phase 1 corroborates the static/config layer; the behavioural layer is carried by the prior run's evidence, NOT re-proven this pass.

#### 2.2.1 The three acceptance-criterion items and their current status

**(a) Logout** — sign in through the form → POST the logout action → assert the session cookie is gone AND that a request carrying the pre-logout signed cookie is rejected (401).

- **Status: PARTIAL.**
- **What is proved:** Phase 1 proves the config layer — the Go RBAC filter exists, the JWKS/JWT trust boundary is correct, the session cookie is signed (better-auth's `setSignedCookie`/`getSignedCookie`, documented in `session-security.test.js:55-58`). The source doc §1.2.3 reports the full logout flow passing in the prior E2E run (4/4 cases, each signing out through the UI and then observing an API-backed `Silakan masuk` refusal — `design/auth-production-readiness-v2.md:198-203`).
- **What is NOT exercised this run:** The live behavioural proof — `POST /api/auth/sign-out` deletes the session row, and the SIGNED pre-sign-out cookie is replayed and rejected (`get-session` 200 null, `/token` 401) — is in `session-security.test.js` phase 2, which is DB-gated this run. The test code that does this exists (`session-security.test.js:36-40` describes the phase 2 logout assertion; the actual test body is in the file's phase 2 section).
- **How to close:** Run Phase 2 with a reachable Postgres on 5433 (`SIGAP_DATABASE_URL=postgresql://sigap:…@127.0.0.1:5433/sigap node tests/session-security.test.js`). The test is self-contained: per-run accounts (timestamp + random suffix), sign-in on a per-run device, sign-out, then replay the signed cookie. The `finally` block deletes `auth.user`, `auth.session` and `app_users` for the ids it created, then asserts the three tables are empty for them.

**(b) Expiration** — assert a session past `expiresIn` is refused; if a real wait is impractical, use the harness's own clock/offset seam.

- **Status: PARTIAL.**
- **What is proved:** Phase 1 proves `session.expiresIn = 60 * 60 * 24 * 7` (604800 s, 7 days) is configured (`auth.better.ts` session block + `session-security.test.js` phase 1 reads the config source). The source doc §1.2 reports the full expiration behaviour passing in the prior run (`session-security.test.js` phase 2 — "a session row with `expiresAt` in the past answers `get-session` with 200 null and `/token` refuses to mint (401)" — see `session-security.test.js:43-44`).
- **What is NOT exercised this run:** The live behavioural proof — the test script manipulates `expiresAt` directly via SQL to simulate a past-expiry session without waiting 7 days, then asserts `get-session` returns 200 null and `/token` returns 401. This is in `session-security.test.js` phase 2 (`[GATE] SKIP expiration: past expiresAt ⇒ get-session null and /token 401`). The harness's clock/offset seam IS the SQL manipulation — it does not sleep; it writes `expiresAt = now() - interval` directly and asserts the session is refused.
- **How to close:** Run Phase 2 with a reachable Postgres. The test does NOT sleep; it uses the DB to back-date `expiresAt` and asserts the refusal. This is documented in `session-lifecycle.test.js` phase 3 (`design/session-lifecycle-policy.md:173`) and `session-security.test.js` phase 2 (`session-security.test.js:43-44`).

**(c) Revocation** — revoke a session server-side → assert the next request is refused.

- **Status: PARTIAL.**
- **What is proved:** Phase 1 proves the JWT trust boundary is correct (ES256, `iss`/`aud` verified, `sub` mapped to `app_users.subject` via the Go RBAC resolver). The source doc §1.2 reports revocation passing in the prior run: `POST /api/auth/revoke-session` with one's own token deletes that row; the cookie is dead afterwards; cross-user revocation is a silent no-op; the target session survives (`session-security.test.js` phase 2 — see `session-security.test.js:31-37`).
- **What is NOT exercised this run:** The live behavioural proof — revoke a session via `/revoke-session` with the caller's own token, then assert `get-session` returns 200 null and the cookie cannot be replayed for `/token` (401). Also the cross-user isolation proof (a foreign token is a silent no-op; the target session survives). These are in `session-security.test.js` phase 2 (`[GATE] SKIP` for both this run).
- **How to close:** Run Phase 2 with a reachable Postgres. The test is self-contained and exercises: own-session revocation, cross-user revocation isolation, session switching (`revoke-other-sessions` kills only the other device), and the password-change invalidation path (old password dead AND new password live as a positive control).
- **Important nuance (documented, not a defect):** A Better Auth session revocation is NOT a JWT revocation. The JWT `exp` = 1 h is the maximum identity-acceptance delay for an already-minted token (`session-lifecycle-policy.md:76-101`). The Go API ignores the token's `permissions` claim and re-resolves RBAC from the DB per request, so privilege loss is immediate (0 s); only identity (being recognized as the same `sub`) can lag for at most 1 h on a token already handed out before revocation (`session-lifecycle-policy.md:70-76`). The harness proves the session-row deletion is immediate; the JWT-layer bound is the 1 h TTL, which is a decision, not an oversight (`session-lifecycle-policy.md:100-101`).

### 2.3 The prior run's evidence that carries the behavioural proofs (not re-run this pass)

The source doc `design/auth-production-readiness-v2.md` §1.2.2 reports `4 passed (23.0s)` for the E2E journey (run 2, after a documented cold-dev-server flake in run 1 — §1.2.1). §1.4 reports 15/15 negative checks passing. §1.5 reports the full cookie→JWT→Go-API chain. These were produced against the `a5423ec` + working-tree stack; the auth files are byte-identical to HEAD at `42d43d2` where they are unchanged (`design/auth-production-readiness-v2.md:18-25`).

**This run does NOT re-run the E2E journey, the 15 HTTP probes, or the visual capture.** It runs `session-security.test.js` Phase 1 (10/10 PASS) and confirms Phase 2 is DB-gated. The behavioural proofs (logout, expiration, revocation, session switching, password-change invalidation, cross-user revocation isolation, refresh, deactivation characterization, live JWKS/JWT shape) are carried by the prior run's evidence, which is cited, not re-derived.

---

## 3. Registration identity provisioning — confirmed, not re-implemented

### 3.1 `databaseHooks.user.create.after` → roleless `app_users` row keyed by the auth user id, never email-linked

**Confirmed by code.** In `auth.better.ts`, the `databaseHooks` block contains exactly one entry — `user.create.after` — which writes a roleless `app_users` row:

```typescript
// auth.better.ts (databaseHooks section, referenced but the hook body is inline in the auth config)
// "Every new `auth.user` is provisioned with its own ROLELESS `app_users` row
//  by the `databaseHooks.user.create.after` hook below; without it a new
//  citizen's `sub` resolves to no `AppUserID`"
```

The rule is stated explicitly in `auth.better.ts:29-31`:
> "Every new `auth.user` is provisioned with its own ROLELESS `app_users` row by the `databaseHooks.user.create.after` hook below; without it a new citizen's `sub` resolves to no `AppUserID`"

**This is the fail-closed subject seam.** A new citizen's `auth.user.id` (the JWT `sub`) is written to `app_users.subject` by the hook. The Go RBAC resolver keys on `app_users.subject` (`rbac_resolver.go:106` — `WHERE u.subject = $1`), so a citizen with no `app_users` row resolves to zero permissions (`Resolve` returns `ErrClosed` / empty permissions, not an error). The hook is the only bridge between the auth provider's `user.id` and SIGAP's internal `app_users.id`.

**Email is never the join key.** The mapping rule is stated in `design/identity-mapping.md:56-80`:
> "The backfill must **not** derive an `app_user ↔ subject` link from email equality. It binds each `app_users.id` to a Better Auth `user.id` from a **reviewed, hard-coded list** and writes `subject` by `id`."

The backfill script `apps/web/scripts/backfill-auth-subjects.mjs` carries the explicit mapping (`MAPPING` array — `apps/web/scripts/backfill-auth-subjects.mjs:116-130`), listing `{ email, appUserId }` pairs explicitly and doing the `app_users` write strictly by `WHERE id`. The email is used **only** to locate/create the corresponding `auth.user` row (which is, by Better Auth's own schema, keyed on email); it is never the join key (`design/identity-mapping.md:77-80`).

**Why email equality is forbidden as a linking rule** (`design/identity-mapping.md:66-75`):
- Email is a self-asserted claim, not the token-proven identity. The verified token proves `sub`, never an email.
- Email ownership is transferable. Auto-linking by email would silently re-point `app_users.id` at whatever Better Auth account currently holds that address — a hostile account-takeover primitive and a violation of the fail-closed subject seam.
- A vendor can mint duplicate emails across providers. Only `sub` is unique and token-proven; the resolver keys on `subject`, so the mapping must be written at that exact key.

**Conclusion: registration identity provisioning is correct by construction.** The hook writes a roleless `app_users` row keyed by `auth.user.id` (the JWT `sub`), never by email. The Go RBAC resolver keys on `app_users.subject`. A citizen with no `app_users` row resolves to zero permissions (fail-closed). This is NOT re-implemented this run — it is confirmed by reading the source.

### 3.2 Production impersonation stays impossible — confirmed by code + test tripwire

**Three layers of proof, all read this run:**

**Layer 1 — The web tier's local actor selector is gated by `SIGAP_ENV=local`.** In `auth.ts:121-130`:

```typescript
export function localE2eActorHeader(): Record<string, string> {
    if ((process.env.SIGAP_ENV || '').toLowerCase() !== 'local') {
        return {};   // ← nothing emitted outside local
    }
    const subject = (process.env.SIGAP_LOCAL_E2E_ACTOR || '').trim();
    if (subject === '') {
        return {};   // ← nothing emitted when the actor var is empty
    }
    return { 'X-Sigap-Local-Test-Subject': subject };
}
```

So `X-Sigap-Local-Test-Subject` is emitted **only** when `SIGAP_ENV=local` AND `SIGAP_LOCAL_E2E_ACTOR` is non-empty. In any other environment (`staging`, `production`), the function returns `{}` — no header, no impersonation.

**Layer 2 — The Go API never reads the web-side actor variable.** `TestGuardDevCapabilities_WebSideE2eActorIsNotAGoGate` (`envguard_test.go:851-886`) documents this explicitly:

> "SIGAP_LOCAL_E2E_ACTOR is read by the web tier only (apps/web/src/lib/server/auth.ts: localE2eActorHeader), which emits X-Sigap-Local-Test-Subject and nothing else, and only when SIGAP_ENV is local. The Go API never reads the variable. The block for it therefore lives in three places this test does NOT replace: 1. apps/web/src/lib/server/auth.ts — no header unless SIGAP_ENV=local. 2. scripts/ops/preflight-production-env.sh — refuses a production deploy with the variable set. 3. The Go side refuses the HEADER it produces outside local (auth.LocalRBACTestIdentityProvider is disarmed, and this test's own registry assertions plus the auth impersonation tests prove it)."

**Layer 3 — The Go dev-flag registry does not include `SIGAP_LOCAL_E2E_ACTOR`.** The four registered dev flags are `SIGAP_DEV_IDENTITY`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_ENGINE_FALLBACK`, and one more (`envguard_test.go:559-588` — `auditedDevFlags`). `SIGAP_LOCAL_E2E_ACTOR` is NOT among them, which is correct: the Go process never reads it, so it should not be in the Go boot guard. The tripwire `TestGuardDevCapabilities_AuthEnvelopeIsNotYetGuarded` (`envguard_test.go:516-543`) documents the related gap: the auth env vars the web tier reads (`BETTER_AUTH_SECRET`, `BETTER_AUTH_URL`, `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`) are NOT registered dev/prod guards, so `GuardDevCapabilities` / `GuardTLS` deliberately say nothing about them. This is a tripwire, not a permission — if a future guard ever starts refusing one of these, the test fails and the matrix + preflight script must be updated in the same change.

**The JWT path never grants permissions from token claims.** The Go JWT provider verifies `iss`, `aud`, `exp`, `alg` (rejects `alg=none`), and selects the signing key by `kid` (`jwt_provider.go:64-96`). It does NOT read the token's `permissions` claim — `rbac_resolver.go` resolves permissions from the DB per request (`u.status = 'active' AND u.deleted_at IS NULL` filters). This is AUDIT-101, asserted in Phase 1 (`session-security.test.js:232-236` — "the Go RBAC resolver enforces deactivation at authorization").

**`SIGAP_DEV_IDENTITY` / `SIGAP_LOCAL_RBAC_TEST_IDENTITY` / `X-Sigap-*` are inert outside `SIGAP_ENV=local`.** The Go envguard (`envguard.go` `GuardDevCapabilities`) refuses to boot with these flags set when `SIGAP_ENV` is not `local` (`envguard_test.go:754-798` — `TestGuardDevCapabilities_EachFlagRefusedOutsideLocal`). The web tier's `localE2eActorHeader` returns `{}` outside `local` (`auth.ts:122-124`). The Go API's `LocalRBACTestIdentityProvider` is disarmed outside `local` (`envguard_test.go:851-886`).

**Conclusion: production impersonation is impossible by construction.** Three layers: (1) the web tier only emits `X-Sigap-Local-Test-Subject` when `SIGAP_ENV=local`; (2) the Go API never reads the web-side actor variable; (3) the Go dev-flag registry refuses to boot with dev flags outside `local`. The JWT path never grants permissions from token claims — the Go RBAC resolver re-resolves from the DB per request. This is NOT re-implemented this run — it is confirmed by reading the source and the tripwire test.

---

## 4. Visual QA — what can and cannot be concluded without pixel inspection

### 4.1 What exists

Three visual QA runs, each capturing 4 screens at 4 widths (16 PNGs per run, 48 PNGs total):

| Run | Stack | Output dir | Shots | Ready |
|---|---|---|---|---|
| A | `-Jwt -NoAdminActor` (warm) | `review-shots/auth-qa/` | 16 | 16/16 |
| B | `-Jwt` (admin actor armed), cold | `review-shots/auth-qa-armed/` | 16 | 14/16 (2 failed during Vite re-optimization) |
| C | same armed stack, warm | `review-shots/auth-qa-armed-warm/` | 16 | 16/16 |

The 16 PNG names (identical across all three runs):
- `auth-qa-login-{1440,768,390,360}.png`
- `auth-qa-register-{1440,768,390,360}.png`
- `auth-qa-appointments-signed-out-{1440,768,390,360}.png`
- `auth-qa-admin-{1440,768,390,360}.png`

Plus the legacy single shots in `review-shots/` root: `better-auth-janji-saya-populated.png`, `better-auth-janji-saya-1280.png`, and the `auth-janji-saya-*.png` / `admin-*-*.png` / `logout-*.png` / `register-*.png` / `login-*.png` set from an earlier capture.

### 4.2 What the geometry assertions proved (without pixel inspection)

The capture harness (`node apps/web/scripts/capture-auth-visual-qa.mjs`) is explicit about what it is: "a geometry + artifact producer … it never inspects the PNGs it writes" (`design/auth-production-readiness-v2.md:211-212`).

Geometry summary, run C (runs A and C are identical row-for-row — `design/auth-production-readiness-v2.md:228-245`):

| view | w | pageOverflow | clipped | smallCtl/total | primaryVisible |
|---|---|---|---|---|---|
| login | 1440 | no | 0 | 0/12 | true |
| register | 1440 | no | 0 | 0/13 | true |
| appointments-signed-out | 1440 | no | 0 | 0/9 | true |
| admin | 1440 | no | 0 | 0/10 | true |
| login | 768 | no | 0 | 0/11 | true |
| register | 768 | no | 0 | 0/12 | true |
| appointments-signed-out | 768 | no | 0 | 0/8 | true |
| admin | 768 | no | 0 | 0/10 | true |
| login | 390 | no | 0 | 0/11 | true |
| register | 390 | no | 0 | 0/12 | true |
| appointments-signed-out | 390 | no | 0 | 0/8 | true |
| admin | 390 | no | 0 | 0/10 | true |
| login | 360 | no | 0 | 0/11 | true |
| register | 360 | no | 0 | 0/12 | true |
| appointments-signed-out | 360 | no | 0 | 0/8 | true |
| admin | 360 | no | 0 | 0/10 | true |

Read as: no page-level horizontal overflow at any of the four widths, **zero** elements clipped outside a scroll region, **zero** interactive controls under the 36 px touch floor, and the primary affordance of every view inside the first viewport.

### 4.3 What CAN be concluded without pixel inspection

- **Layout integrity:** No horizontal overflow, no clipped elements, no controls below the 36 px touch floor. This is a real, measured assertion about the layout, not a pixel judgement.
- **Artifact completeness:** All 16 shots per run were produced (geometry reports `primaryVisible: true` for every view at every width, meaning the primary affordance was inside the first viewport — the screenshot captured the right part of the page).
- **Run-to-run consistency:** Runs A and C are identical row-for-row, which means the geometry is stable across warm boots.
- **Defect found in the harness itself:** `adminActorArmed` in `report.json` is computed as `document.body.innerText.includes('local-global-super-admin')`, and that exact string is static copy in `apps/web/src/routes/admin/+layout.svelte:69` (the non-dismissible dev banner). It read `true` on run A, whose stack was started with `-NoAdminActor` and therefore had no local actor. The field cannot be used as evidence that the local selector was armed; use `GET /admin` + `Silakan masuk`, or the API's 403, instead (`design/auth-production-readiness-v2.md:263-269`).

### 4.4 What CANNOT be concluded without pixel inspection (VISUAL_QA = OPEN)

- **Colour / contrast / hierarchy:** No colour, contrast, or visual hierarchy judgement was made. The geometry harness reports no colour data.
- **Spacing / alignment / visual weight:** No spacing or alignment judgement. Geometry reports "no overflow" and "no clipped elements" but not "the spacing between X and Y looks right".
- **Typography legibility / scale:** No typography judgement. The harness does not measure font sizes or line heights.
- **"Does it look right" / brand fit / polish:** No aesthetic judgement. The harness is a geometry producer, not a design reviewer.
- **The legacy single shots (`better-auth-janji-saya-populated.png`, `better-auth-janji-saya-1280.png`, the `auth-janji-saya-*.png` / `admin-*-*.png` / `logout-*.png` / `register-*.png` / `login-*.png` set):** These were produced by an earlier capture, not by the current harness. No geometry report accompanies them. Nothing about their layout, spacing, or visual quality can be concluded from the file names alone.
- **The `contact-sheet.png` and `sigap-product-final.zip` in `review-shots/`:** Not part of the auth QA harness; their provenance and content are not asserted here.

### 4.5 What would be needed to close VISUAL_QA

A human (or a vision model with access to the PNGs) must inspect the 48 PNGs from the three auth-qa runs and the legacy single shots. The geometry assertions are a necessary but not sufficient precondition — they prove the layout is sane, not that it is correct or polished. Closing `VISUAL_QA` requires a pixel-level review that this harness and this run do not perform.

**This run does NOT inspect pixels.** `VISUAL_QA` remains OPEN.

---

## 5. Per-gate status table

### 5.1 BETTER_AUTH_SELFHOSTED

| Sub-item | Status | Evidence |
|---|---|---|
| Real browser sign-in journey (4 actors) | **PASS** (prior run, not re-run this pass) | `4 passed (23.0s)`, 4/4 cases, no retry — `design/auth-production-readiness-v2.md:176-189` (run 2). Run 1 failed with a documented cold-dev-server flake (`design/auth-production-readiness-v2.md:132-172`). |
| Negative security checks (15/15) | **PASS** (prior run, not re-run this pass) | 15/15 as expected — `design/auth-production-readiness-v2.md:276-290`. |
| Cookie→JWT→Go-API chain (non-MFA) | **PASS** (prior run, not re-run this pass) | Cookie → `GET /api/auth/token` → ES256 JWT (`iss`, `aud`, `sub`, exp-iat = 3600 s) → Go API 200 with the subject's own 2 rows — `design/auth-production-readiness-v2.md:314-332`. |
| Session lifecycle Phase 1 (static, 10 assertions) | **PASS** (this run) | `session-security.test.js` Phase 1: 10/10 PASS — §2.1. |
| Session lifecycle Phase 2 (live, DB-gated) | **PARTIAL** (not exercised this run) | Postgres on 5433 not reachable; 7 live assertions skipped — §2.2. |
| JWKS rotation (code half) | **CLOSED** | `rotationInterval: 60 * 60 * 24 * 30` in `auth.better.ts:340`; Phase 1 asserts it evaluates to 2592000 s; the Go verifier's 15-min cache bound is documented in the comment. Residual: pre-existing key has `expiresAt = null` — operator must delete/regenerate the `auth.jwks` row. |
| Secrets (code half) | **CLOSED** | `resolveSecret()` throws at module construction on a production server without `BETTER_AUTH_SECRET` — `auth.better.ts:85-96`. Deploy-env half OPEN. |
| MFA flows (3 flows: email verification, password reset, TOTP MFA) | **PASS** (prior run, not re-run this pass) | All three PASS end-to-end — `email-mfa-flows-verification.md:17-28`. 12 sub-steps unverified — §1.3. |
| MFA item 9 (Go trust boundary) | **HALF CLOSED** | Non-MFA path end-to-end CLOSED; MFA-enrolled subject mapping OPEN. Go JWT negatives (issuer/audience/expiry/rotation) covered by `better_auth_compat_test.go`. |
| Registration identity provisioning | **CONFIRMED BY CODE** | `databaseHooks.user.create.after` writes roleless `app_users` row keyed by `auth.user.id`, never email — §3.1. |
| Production impersonation impossible | **CONFIRMED BY CODE + TEST TRIPWIRE** | Three layers — §3.2. |

### 5.2 SESSION_LIFECYCLE

| Sub-item | Status | Evidence |
|---|---|---|
| Logout (sign-out deletes row; signed cookie rejected) | **PARTIAL** | Phase 1 proves config; Phase 2 behavioural proof is DB-gated this run — §2.2(a). Prior run: E2E 4/4 each signs out through the UI and observes `Silakan masuk` refusal — `design/auth-production-readiness-v2.md:198-203`. |
| Expiration (past `expiresAt` ⇒ `get-session` null, `/token` 401) | **PARTIAL** | Phase 1 proves `expiresIn = 604800 s` configured; Phase 2 behavioural proof is DB-gated this run — §2.2(b). Harness seam = SQL back-date of `expiresAt`, no sleep. |
| Revocation (server-side revoke → next request refused) | **PARTIAL** | Phase 1 proves JWT trust boundary + RBAC filter; Phase 2 behavioural proof is DB-gated this run — §2.2(c). Prior run: `session-security.test.js` phase 2 exercised own-session revocation, cross-user isolation, session switching, password-change invalidation — `design/auth-gap-matrix-v2.md:231-245`. |
| Refresh (`updateAge` roll-forward) | **PARTIAL** | Phase 2 behavioural proof is DB-gated this run — `session-security.test.js:41-42`. |
| Session switching (per-device; revoke-other-kills-only-other) | **PARTIAL** | Phase 2 behavioural proof is DB-gated this run — `session-security.test.js:34-35`. |
| Password-change invalidation (old password dead, new password live) | **PARTIAL** | Phase 2 behavioural proof is DB-gated this run — `session-security.test.js:48-49`. |
| Cross-user revocation isolation (foreign token is silent no-op) | **PARTIAL** | Phase 2 behavioural proof is DB-gated this run — `session-security.test.js:31-32`. |
| Deactivation gap characterization (disabled `app_users` row still signs in; Go RBAC enforces) | **PARTIAL** | Phase 1 proves the Go RBAC filter exists (`session-security.test.js:232-236`); Phase 2 characterization assertions exist in the file (`session-security.test.js:49-53`) but are DB-gated this run. The characterization is asserted, not re-proven this pass. |
| 1h `freshAge` window | **CLOSED** (static) | Asserted in Phase 1; exercising requires 3600 s wait — §1.2 #2. |
| Short-lived JWT (access JWT TTL = 3600 s, zero leeway) | **CLOSED** | Phase 1 asserts `expirationTime: "1h"` in `auth.better.ts:357` and zero leeway in `jwt_provider.go` (`session-security.test.js:212-221`). |

**Session-lifecycle items exercised this run:** Phase 1's 10 static assertions (all PASS). **Not exercised this run:** Phase 2's 7 live behavioural assertions (logout, expiration, revocation, refresh, session switching, password-change invalidation, cross-user revocation isolation, deactivation gap characterization, live JWKS/JWT shape) — all DB-gated. **Count: 10 exercised (PASS), 9 not exercised (PARTIAL, carried by prior-run evidence).**

### 5.3 STAFF_MFA

| Sub-item | Status | Evidence |
|---|---|---|
| Email verification (register → captured mail → open link → `emailVerified = true`) | **PASS** (prior run) | `email-mfa-flows-verification.md:128-177`. |
| Password reset (request → captured link → set new password → new signs in, old rejected) | **PASS** (prior run, twice) | `email-mfa-flows-verification.md:181-260`. |
| TOTP MFA (enroll → sign in with TOTP → sign in with backup code → wrong/replayed codes rejected) | **PASS** (prior run) | `email-mfa-flows-verification.md:262-318`. |
| Real mail delivery (SMTP) | **INFRA-BLOCKED** | `SIGAP_SMTP_URL` unset; no transport; `mail.ts:26-33`. |
| `requireEmailVerification: true` branch | **OWNER-BLOCKED** | Env gate unset; flipping before transport exists locks every account out. |
| Trusted device | **INFRA-BLOCKED/ops** | `trustDevice: true` never sent; 30-day cookie; needs device + policy decision. |
| Account lockout to ceiling | **TEST-FIXABLE** | `accountLockout: { maxFailedAttempts: 10, durationSeconds: 900 }` configured (`auth.better.ts:388`); not driven to ceiling. |
| Verification-link expiry | **TEST-FIXABLE** | Both MAX_AGE clocks 3600 s; expired token never presented. |
| TOTP drift window | **TEST-FIXABLE** | Adjacent-step codes computed but never submitted. |
| Resend endpoint (`send-verification-email`) | **TEST-FIXABLE** | Not called. |
| `two-factor/disable` + `generate-backup-codes` | **TEST-FIXABLE** | Not called. |
| Go trust boundary for MFA-enrolled subject | **TEST-FIXABLE** | Non-MFA path closed; MFA subject mapping open. |
| Concurrency (simultaneous reset-token replays) | **TEST-FIXABLE** | Single-use verified sequentially only. |
| Backup-code exhaustion / encryption strength | **TEST-FIXABLE** | AES-GCM-at-rest claim weakly supported; plaintext count never recovered. |
| Anti-enumeration (known/unknown pair) | **TEST-FIXABLE** | Every address submitted was real; byte-identical response + ≥500 ms floor unmeasured. |

### 5.4 SUPABASE_RUNTIME_DECOUPLING

| Sub-item | Status | Evidence |
|---|---|---|
| Subject ↔ `app_users` mapping rule | **CONFIRMED BY CODE** | `app_users.subject` = `auth.user.id` (JWT `sub`); `app_users.id` is the stable internal id, never re-written — `design/identity-mapping.md:12-54`. |
| Explicit mapping (never email auto-link) | **CONFIRMED BY CODE** | Backfill script lists `{ email, appUserId }` pairs explicitly; writes `subject` by `id` — `apps/web/scripts/backfill-auth-subjects.mjs:116-130`; `design/identity-mapping.md:56-80`. |
| Go RBAC resolver keys on `subject` | **CONFIRMED BY CODE + PHASE 1** | `rbac_resolver.go:106` — `WHERE u.subject = $1`; Phase 1 asserts the filter exists — `session-security.test.js:232-236`. |
| Rollback is surgical | **CONFIRMED BY DESIGN** | Re-point `SIGAP_AUTH_ISSUER`/`AUDIENCE`/`JWKS_URL` at Supabase; `UPDATE app_users SET subject = <snapshotted old id>` — `design/identity-mapping.md:99-116`. |
| `PUBLIC_SUPABASE_URL` / `PUBLIC_SUPABASE_ANON_KEY` still read at build/serve | **CONFIRMED BY CODE** | `docker-compose.yml:164-166` — legacy Supabase seam still in compose; unused by the self-hosted path (the web app reads them but the auth path does not depend on them). |
| Supabase runtime fully decoupled for the auth path | **CONFIRMED BY CODE** | Better Auth replaces Supabase Auth; the only Supabase references are legacy publishable values (`docker-compose.yml:164-166`, `auth.better.ts` reads no Supabase vars). |

### 5.5 VISUAL_QA

| Sub-item | Status | Evidence |
|---|---|---|
| Geometry (no overflow, no clipping, no sub-36px controls) | **PASS** (3 runs, 48 shots) | `design/auth-production-readiness-v2.md:228-245` (run C; runs A and C identical). |
| Pixel-level review (colour, contrast, spacing, typography, "looks right") | **OPEN** | No vision model available; PNGs produced and measured, never inspected. |
| Legacy single shots (`better-auth-janji-saya-populated.png`, etc.) | **OPEN** | No geometry report; provenance not asserted. |
| Harness defect found (adminActorArmed field) | **DOCUMENTED** | `adminActorArmed` reads static copy, not the local selector — `design/auth-production-readiness-v2.md:263-269`. |

### 5.6 DEPLOYMENT_READINESS

| Sub-item | Status | Evidence |
|---|---|---|
| HTTPS termination + proxy trust + HSTS (web tier) | **INFRA-BLOCKED + CODE-FIXABLE** | `hooks.server.ts:78`; `auth.better.ts:118`; `docker-compose.yml:146-167`; preflight has no `https://` check — §1.1 #1. |
| Cookie security (`Secure`/HttpOnly/SameSite) | **CLOSED** (READY as shipped) | No override in `auth.better.ts`; library-enforced; `Secure` derives from `BETTER_AUTH_URL` being `https` — §1.1 #2. |
| CSRF / origin (`trustedOrigins`) | **CODE-FIXABLE + INFRA-BLOCKED** | No `trustedOrigins` in `auth.better.ts`; compose forwards no `BETTER_AUTH_URL` — §1.1 #3. |
| JWT/JWKS key rotation | **CODE CLOSED + INFRA-BLOCKED (residual)** | `rotationInterval: 30 d`; pre-existing key has `expiresAt = null` — §1.1 #4. |
| Secrets (`BETTER_AUTH_SECRET`) | **CODE CLOSED + INFRA-BLOCKED (deploy env)** | `resolveSecret()` throws on production without secret — `auth.better.ts:85-96`; compose forwards no secret — §1.1 #5. |
| DB backup/restore of `auth` schema | **TEST-FIXABLE + INFRA-BLOCKED (drill)** | Dump covers `auth` (105 `CREATE TABLE`, 6 `auth.*`); restore drill enumerates `public` only — §1.1 #6. |
| Separate `auth`-schema DB privileges | **INFRA-BLOCKED** | `sigap` owns `auth` and `public`; no least-privilege role — §1.1 #7. |
| Rate limiting durability | **INFRA-BLOCKED + OWNER-BLOCKED (store choice)** | Three per-process in-memory limiters; no `rateLimit` table — §1.1 #8. |
| Audit logs for sign-in / sign-out | **OWNER-BLOCKED** | Only `auth.session` rows; single `databaseHooks` entry is citizen provisioning — §1.1 #9. |
| Production config assertions | **CODE-FIXABLE (preflight) + TEST-FIXABLE (Go tripwire)** | Preflight asserts Go/API envelope only; Go tripwire `TestGuardDevCapabilities_AuthEnvelopeIsNotYetGuarded` added — §1.1 #10. |
| Compose `web` service forwards no `BETTER_AUTH_*` | **INFRA-BLOCKED** | `docker-compose.yml:146-167` — web env block has `PORT`, `HOST`, `ORIGIN`, `SIGAP_ENV`, `SIGAP_TLS_TERMINATED`, `SIGAP_API_INTERNAL`, `PUBLIC_SIGAP_API_BASE`, `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY` — no `BETTER_AUTH_SECRET`, no `BETTER_AUTH_URL`, no `SIGAP_DATABASE_URL`, no auth issuer/audience. |
| `.env.example` names no `BETTER_AUTH_*` | **CODE-FIXABLE** | Operator following `.env.example` cannot discover the requirement — `design/auth-production-readiness-v2.md:488-489`. |
| Preflight asserts no auth variable | **CODE-FIXABLE** | Preflight asserts `SIGAP_ENV`, `SIGAP_DEPLOY_DIR`, `SIGAP_AUTH_MODE`, and the four dev flags absent/false — nothing about auth — `design/auth-production-readiness-v2.md:491-501`. |

---

## 6. Infrastructure prerequisites still owed by the owner

These are the items that no code edit in this repository can substitute. Each is a person, an account, a certificate, a host, or a decision.

### 6.1 SMTP endpoint + creds (blocks MFA items 1, 2; gap 9's audit sink is separate)

| What | Blocks | Detail |
|---|---|---|
| A real SMTP provider with credentials | MFA item 1 (real mail delivery), MFA item 2 (`requireEmailVerification: true` branch) | `mail.ts:26-33` is explicit: "There is NO SMTP transport"; `SIGAP_SMTP_URL` is the documented seam; setting it today logs `console.error` and drops the message without writing the captured mail to disk. Required: a provider + credentials + deliverability work (SPF/DKIM/DMARC on the sending domain) **before** `SIGAP_REQUIRE_EMAIL_VERIFICATION=true` can be flipped — otherwise verification and password reset are unreachable and turning the gate on locks every account out (`design/auth-production-readiness-v2.md:518-519`). |

### 6.2 An email sender (blocks MFA item 1; related to SMTP above)

| What | Blocks | Detail |
|---|---|---|
| A sending domain with SPF/DKIM/DMARC | MFA item 1 | The sender envelope (From, return-path, DMARC policy) must be configured for the sending domain before any production email is sent. Without this, verification and password-reset emails to real addresses will be rejected or spam-filtered, and the `requireEmailVerification` gate cannot be safely enabled. |

### 6.3 MFA recovery operator path (blocks MFA items 7, 8, 11; related to SMTP)

| What | Blocks | Detail |
|---|---|---|
| An operator procedure for `POST /api/auth/send-verification-email` (resend) | MFA item 7 | The resend endpoint must be callable by an operator (or a user with a live session) to re-send a verification email when the original is lost. This is a TEST-FIXABLE item once SMTP exists; the operator path is the human procedure for triggering it. |
| An operator procedure for `POST /api/auth/two-factor/disable` + `generate-backup-codes` | MFA items 8, 11 | When a staff member loses their TOTP device, the operator (or the user with a fresh session + password) must be able to disable MFA and re-issue backup codes. This is TEST-FIXABLE once the endpoints are called live; the operator path is the human procedure + the authz gate for who may invoke it. |
| Backup-code rotation procedure (blocks MFA item 11's encryption-strength question) | MFA item 11 | Because the `BETTER_AUTH_SECRET` encrypts the 2FA backup codes (`design/auth-production-readiness-v2.md:418-423`), rotating the secret requires regenerating every enrollee's backup codes in the same window. This is an operator procedure, not a code change. |

### 6.4 A least-privilege DB role for the `auth` schema (blocks gap 7)

| What | Blocks | Detail |
|---|---|---|
| A dedicated Postgres login role with `USAGE, CREATE` on the `auth` schema and nothing on `public` | Gap 7 | Today `sigap` owns `auth` AND `public` (measured: 6/6 `auth` tables owned by `sigap`, zero explicit grants — `design/auth-production-readiness-v2.md:553`). The component that faces untrusted browsers holds write privileges over the clinical tables. Required: a dedicated login role with `USAGE, CREATE` on `auth` and nothing on `public`, its password in the secret store, and the web service pointed at it. This is a Postgres/DB-admin change; do NOT touch auth-schema privileges in code (`design/auth-gap-matrix-v2.md:144-149`). |

### 6.5 TLS termination at the edge (blocks gaps 1, 3; cookie `Secure`; HSTS; JWT `iss`; JWKS URL)

| What | Blocks | Detail |
|---|---|---|
| A TLS-terminating proxy (Traefik, `ENABLE_EDGE_ROUTING`) + a real certificate | Gaps 1, 3; cookie `Secure`; HSTS; the JWT `iss`; the JWKS URL | This stack is plain HTTP on `127.0.0.1:5173`; `BETTER_AUTH_URL` is `http://127.0.0.1:5173`, so cookies are non-`Secure` and un-prefixed here by design. Required: a TLS-terminating proxy, a real certificate, and `BETTER_AUTH_URL == ORIGIN == SIGAP_WEB_ORIGIN` at the `https` origin — the single lever that flips `Secure` cookies, the `__Secure-` prefix, the CSRF allow-list, the JWT `iss`, and the JWKS URL together (`design/auth-production-readiness-v2.md:520-526`). |
| `X-Forwarded-Proto: https` + `SIGAP_TRUSTED_PROXIES` matching the real hop count | Gap 1 header trust, `getClientAddress()` rate-limit keys | Whoever operates the edge must confirm `X-Forwarded-Proto: https` and that `SIGAP_TRUSTED_PROXIES` matches the real hop count (`design/auth-production-readiness-v2.md:624-625`). |

### 6.6 A real `BETTER_AUTH_SECRET` in the deploy secret store (blocks gap 5; any production boot at all)

| What | Blocks | Detail |
|---|---|---|
| A generated high-entropy secret (≥ 32 chars) in the deploy secret store, injected as the service environment, never in a compose file, never in `.env.example` | Gap 5 | The code half is closed (fail-closed `resolveSecret()`), but the deploy env supplies no secret (`docker-compose.yml:146-167`; `.env.example` names none). Required: the secret in the deploy secret store. Because it encrypts the JWKS private key and the 2FA backup codes (`design/auth-production-readiness-v2.md:418-423`), it must be handled as long-lived; a rotation procedure is part of this item, not a follow-up. |

### 6.7 A durable rate-limit store, or a ratified single-replica invariant (blocks gap 8)

| What | Blocks | Detail |
|---|---|---|
| Redis or the auth-schema store, or a written, reviewed "one web replica, one API replica" constraint | Gap 8 | All three limiters are per-process in-memory (`rate-limit.ts:9-12`; `limiter.go:13-16`); a multi-replica deployment will not share buckets. Better Auth's native `rateLimit.storage` is a config change once the store exists. The store choice is an OWNER decision (`design/auth-gap-matrix-v2.md:151-156`). |

### 6.8 A restore target + scheduled window for a drill that asserts `auth.*` (blocks gap 6)

| What | Blocks | Detail |
|---|---|---|
| A restore target (a Postgres instance that can receive a `pg_restore` of the `auth` schema) + a scheduled drill window | Gap 6 | Whole-DB `pg_dump` covers `auth` (105 `CREATE TABLE`, 6 `auth.*`); the restore drill currently enumerates `public` tables only. Required: a restore target and a drill that asserts `auth.user`, `auth.session`, `auth.account`, `auth.jwks`, `auth.verification`, `auth."twoFactor"` counts after restore — the script change (add `auth.*` queries) is TEST-FIXABLE, but the only proof is a live restore drill, which is INFRA-BLOCKED (`design/auth-production-readiness-v2.md:539-550`). |

### 6.9 Operator JWKS rotate procedure (blocks gap 4's residual)

| What | Blocks | Detail |
|---|---|---|
| A runbook entry for deleting/regenerating the `auth.jwks` row at a controlled moment | Gap 4 residual | The 30-day rotation is a real improvement for keys created from now on, but the key already in the database has `expiresAt = null` and keeps signing indefinitely until an operator deletes/regenerates that row (`design/auth-production-readiness-v2.md:405-416`). The cutover step is: `DELETE FROM auth.jwks` (or `npx auth@latest migrate`-adjacent regeneration) at a controlled moment, after which `GET /api/auth/jwks` mints a key whose `expiresAt` is set and the rotation cycle begins. The Go verifier's bounds are documented: unknown `kid` ⇒ immediate refresh; cached key served at most 15 minutes; stale key served on a refresh failure rather than failing closed. |

### 6.10 Owner decisions still owed (blocks gap 9, policy skip 1, MFA items 2, 3)

| Decision | Blocks | Detail |
|---|---|---|
| Where sign-in/out audit events are written, and with what identity | Gap 9 | Better Auth emits no event that the Go `audit_events` table can see. Mechanism is known (A: `databaseHooks.session.create.after` / sign-out hook in the web tier writing audit rows; B: an internal API audit endpoint the web calls; C: a log shipper). The decision is which owner writes to the hash-chained Go audit service and with what identity — an owner call, not a code gap (`design/auth-gap-matrix-v2.md:158-164`). |
| Deactivation-at-identity gate choice (admin plugin + `banned`, or the status quo) | Policy skip 1 | The product question: should a deactivated account see an authentication error (revealing provisioning state) or a successful sign-in that authorizes nothing? Today's answer is the latter. If a hard gate is wanted, the two supported mechanisms are the `databaseHooks.session.create.before` SQL gate (CODE-FIXABLE) or the admin plugin + `banned` (INFRA: schema migration). The choice is OWNER-BLOCKED (`design/session-lifecycle-policy.md:105-141`; `design/auth-gap-matrix-v2.md:175-187`). |
| `requireEmailVerification: true` flip | MFA item 2 | The env gate (`SIGAP_REQUIRE_EMAIL_VERIFICATION`) exists; flipping it before a transport exists locks every account out. Decision first (`design/auth-gap-matrix-v2.md:198-200`). |
| Whether staff may enrol trusted devices | MFA item 3 | `trustDevice: true` is the library default; the 30-day cookie is the longest-lived credential in the config. A device + a policy decision are needed (`design/auth-gap-matrix-v2.md:201-202`). |
| Replica counts for `web` and `api` | Gap 8, and any capacity plan | The three limiters assume one process each; the store choice (or a ratified single-replica invariant) must be accompanied by a capacity plan (`design/auth-production-readiness-v2.md:622-623`). |

---

## 7. Summary

### 7.1 Per-gate status after the closures proved this run

| Gate | Status |
|---|---|
| **BETTER_AUTH_SELFHOSTED** | Code half substantially closed (JWKS rotation, secrets fail-closed, Go tripwire, Phase 1 trust-boundary 10/10 PASS). Behavioural layer (E2E 4/4, 15/15 negatives, cookie→JWT→Go-API chain) carried by prior-run evidence; Phase 2 live session-security is PARTIAL (DB-gated this run). Registration provisioning + production impersonation confirmed by code. |
| **SESSION_LIFECYCLE** | Phase 1 (static, 10 assertions) **PASS this run**. Phase 2 (live, 7+ behavioural assertions) **PARTIAL** — Postgres on 5433 not reachable; the 7 live assertions (logout, expiration, revocation, refresh, session switching, password-change invalidation, cross-user revocation isolation, deactivation characterization, live JWKS/JWT shape) are skipped this run and carried by prior-run evidence. Items exercised this run: 10 (all PASS). Items not exercised: 9 (PARTIAL). |
| **STAFF_MFA** | All three flows PASS end-to-end in the prior run (not re-run this pass). 12 sub-steps unverified — 1 INFRA (SMTP), 2 OWNER (requireEmailVerification, trusted device), 9 TEST-FIXABLE (lockout, link expiry, TOTP drift, resend, disable/backup-codes, MFA subject mapping, concurrency, backup-code exhaustion, anti-enumeration). |
| **SUPABASE_RUNTIME_DECOUPLING** | **CONFIRMED BY CODE** — subject ↔ `app_users` mapping rule, explicit (never email) mapping, Go RBAC resolver keys on `subject`, rollback is surgical, legacy Supabase vars are unused by the self-hosted path. |
| **VISUAL_QA** | **OPEN** — geometry proved (3 runs, 48 shots, no overflow/clipping/sub-36px controls); pixels not inspected (no vision model). Legacy single shots have no geometry report. Harness defect documented (adminActorArmed reads static copy). |
| **DEPLOYMENT_READINESS** | **NOT READY** — 8 of 9 gaps open; 1 READY as shipped; 2 have code half closed (JWKS rotation, secrets) and remain INFRA-BLOCKED. Compose `web` forwards no `BETTER_AUTH_*`; `.env.example` names none; preflight asserts no auth variable. |

### 7.2 Count of remaining OWNER/INFRA-blocked items

From the 23-item ledger (9 gaps + 2 policy skips + 12 MFA/flow steps):

| Class | Items | Count |
|---|---|---|
| **CLOSED** | #2 (READY as shipped); #4 (code half); #5 (code half); #10 (Go tripwire); policy skip 2; MFA item 9 Go half + non-MFA path | 6 (1 whole + 4 halves + 1 static) |
| **CODE-FIXABLE (open)** | Gap 1 (https-origin assertion + compose comment); gap 3 (`trustedOrigins`); gap 5 (deploy env — compose + `.env.example` + preflight); gap 9 (once the owner decides); gap 10 (preflight script) | 5 |
| **TEST-FIXABLE (open)** | Gap 4 (assert the served JWKS key carries `expiresAt`); gap 6 (`auth.*` counts in the drill); gap 10 (Go tripwire — done); MFA items 4, 5, 6, 7, 8, 9 (MFA subject mapping), 10, 11, 12 | 10 |
| **INFRA-BLOCKED (open)** | Gap 1 (edge TLS + https-origin assertion); gap 3 (compose must forward `BETTER_AUTH_*`); gap 4 (regenerate `auth.jwks`); gap 5 (inject the secret); gap 6 (live drill); gap 7 (`auth` least-privilege role); gap 8 (shared store); MFA item 1 (SMTP); MFA item 3 (trusted device — needs device + policy) | 9 |
| **OWNER-BLOCKED (open)** | Gap 8 (store vs single replica); gap 9 (audit sink + identity); policy skip 1 (deactivation gate choice); MFA item 2 (`requireEmailVerification` flip); MFA item 3 (trusted-device policy); replica counts for `web` + `api` | 6 |

**Headline: 20 of 23 items open in whole or part — 9 INFRA-BLOCKED + 6 OWNER-BLOCKED + 5 CODE-FIXABLE + 10 TEST-FIXABLE (an item may need more than one class).**

### 7.3 Session-lifecycle items: what could and could not be exercised

**Exercised this run (10/10 PASS):**
1. `jwks` block is configured explicitly
2. `jwks.rotationInterval` is set (ES256 signing key cannot live forever)
3. `rotationInterval` evaluates to 2592000 s (30 days)
4. The rotation comment documents the Go verifier 15-min cache bound on retired-key pickup
5. The Go verifier enforces issuer AND audience (WithIssuer / WithAudience)
6. The Go verifier has ZERO leeway (jwt.WithLeeway is absent)
7. The Go JWKS cache TTL is exactly 15 minutes
8. Go jwt mode requires `SIGAP_AUTH_ISSUER` and `SIGAP_AUTH_AUDIENCE`
9. The Go RBAC resolver enforces deactivation at authorization (`status = 'active' AND deleted_at IS NULL`)
10. Policy doc records the missing `auth.user` banned/status column

**Not exercised this run (PARTIAL — DB-gated, 9 items):**
1. Logout (sign-out deletes row; signed cookie rejected — `get-session` 200 null, `/token` 401)
2. Expiration (past `expiresAt` ⇒ `get-session` null, `/token` 401)
3. Revocation (own-session revoke deletes row; cookie dead; cross-user is silent no-op)
4. Refresh (`updateAge` roll-forward on stale sessions)
5. Session switching (`revoke-other-sessions` kills only the other device)
6. Password-change invalidation (old password dead AND new password live as positive control; default leaves other devices live)
7. Cross-user revocation isolation (foreign token is silent no-op; target session survives)
8. Deactivation gap characterization (disabled `app_users` row still signs in; Go RBAC enforces)
9. Live JWKS/JWT shape (kid matches served key; ES256/P-256; iss/aud are pinned values)

**Why not exercised:** Postgres on 5433 was not reachable this run (Docker desktop pipe unavailable; shell's `DATABASE_URL` pointed at 5434, the neighbouring project's Postgres, which the script correctly refuses). The test suite printed `[GATE] SKIP` for every DB-gated test and exited 0. To exercise them: run with a reachable Postgres (`SIGAP_DATABASE_URL=postgresql://sigap:…@127.0.0.1:5433/sigap node tests/session-security.test.js`). The tests are self-contained (per-run accounts, sign-in on a per-run device, SQL-based clock manipulation for expiration — no sleep, no 7-day wait).

### 7.4 What this document does not claim

- No deployment, migration, rollback, restore, or production contact was performed.
- No pixel review: **`VISUAL_QA=OPEN`**.
- The E2E journey (4/4), the 15 HTTP probes, and the visual capture (3 runs) were NOT re-run this pass — they are carried by the prior-run evidence cited in §1 and §5.
- Phase 2 of `session-security.test.js` was NOT exercised this run (DB-gated) — the 9 behavioural assertions are PARTIAL and carried by prior-run evidence.
- No formatter, linter, build gate, or deployment gate was run (those are explicitly out of scope for this assignment; the one build that happened — `vite build` for the test suite — is part of the test, not a gate).
- Everything in the behavioural proofs was measured on one loopback dev stack with local configuration values (`http` origin, dev secret, single replica); none of it is evidence about a TLS-fronted, multi-replica, SMTP-connected deployment.

---

*Document end. Evidence artefacts: `artifact://4144` (full `session-security.test.js` Phase 1 + Phase 2 GATE output). Source docs read: `design/auth-production-readiness-v2.md`, `design/auth-gap-matrix-v2.md`, `design/session-lifecycle-policy.md`, `design/email-mfa-flows-verification.md`, `design/identity-mapping.md`, `design/patient-identity-consent.md`. Source files read: `apps/web/src/lib/server/auth.better.ts`, `apps/web/src/hooks.server.ts`, `apps/web/src/lib/server/auth.ts`, `apps/web/tests/session-security.test.js`, `apps/web/scripts/backfill-auth-subjects.mjs`, `apps/web/src/lib/server/rate-limit.ts`, `apps/web/src/lib/server/mail.ts`, `apps/api/internal/config/envguard_test.go`, `apps/api/internal/auth/rbac_resolver.go`, `docker-compose.yml`.*
