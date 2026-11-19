# ADR — Identity deactivation & revocation policy for SIGAP self-hosted auth

**Status:** PROPOSED — owner decision required. Not implemented by this review.
**Date:** 2026-11-18 · reviewed/corrected 2026-11-19
**Scope:** `feat/sigap-selfhosted-auth` (Draft PR #97), head `cff2d35…`.
**Related:** `design/session-lifecycle-policy.md` (normative clocks/§3 gap),
`design/auth-gap-matrix-v2.md` (gap classification), `docs/operations/git-attribution.md`.

---

## 1. Verified current behaviour (the problem statement)

The following is confirmed from source, not assumed:

1. **Deactivation truth lives in the Go layer.** `public.app_users.status = 'active'`
   AND `deleted_at IS NULL`, plus active role assignments, are re-resolved from
   PostgreSQL on **every** API request by
   `apps/api/internal/auth/rbac_resolver.go`. Removing a role, disabling an
   assignment, deactivating or soft-deleting an `app_users` row therefore takes
   effect on the **next** request — privilege revocation latency is **0 s**.
2. **The Better Auth identity layer does not refuse session creation for a
   deactivated account.** better-auth 1.7.20 base config has no `banned`/
   `status` field on `auth.user` (those arrive only with the uninstalled admin
   plugin), and `user.validateUserInfo` is skipped for email/password sign-in.
   A disabled identity still signs in, receives a session and a JWT; Go then
   resolves zero permissions for it. This is the OPEN product-policy gap that
   keeps `SESSION_LIFECYCLE=PARTIAL`.
3. **A revoked Better Auth session is not a revoked JWT.** Sign-out /
   revoke deletes the `auth.session` row, so `hooks.server.ts` stops minting
   new Bearers immediately. An already-issued ES256 JWT keeps verifying at the
   Go API until `exp`, because the validator checks signature, `iss`, `aud`,
   `exp` only (`jwt_provider.go`, zero leeway) and ignores the token's
   `permissions` claim (AUDIT-101). The accepted bound is the 1 h JWT TTL.
4. **JWKS rotation is pinned at 30 days** with a 15-minute Go cache bound;
   issuer/audience/expiry/rotation negatives are covered in
   `better_auth_compat_test.go`.
5. Two historical Strix findings that intersect this area are already
   remediated in this head: the dev-identity header grants only read-only
   permissions and is production-blocked by envguard; patient status lookup is
   keyed only by the random 6-char `checkin_code`, never by sequential
   `formatted_number`.

**Consequence:** an offboarded staff member's *authority* ends immediately, but
their already-held token is still *accepted as an identity* for up to 1 hour,
and their account can still produce a fresh session/JWT at the identity layer
until a hard gate exists. Both facts must be stated in any security attestation.

---

## 2. Decision to be made by the owner

> Should a deactivated SIGAP identity see (A) a technically successful sign-in
> that authorizes nothing — today's behaviour — or (B) an explicit
> authentication error at sign-in / session creation?

Option B closes the identity-layer gap but reveals provisioning state (an
account-existence/deactivation oracle) and requires a policy choice about
error wording, enumeration resistance and audit. Option A preserves the
anti-enumeration posture and relies on the immediate Go RBAC control. This ADR
**recommends** a specific coherent Option B design below; it does not enact it.

---

## 3. Options compared

| # | Option | Boundary enforced | New JWTs for disabled identity | Existing JWT latency | Staff deactivation | Multi-facility role change | Migration | Verdict |
|---|--------|-------------------|-------------------------------|---------------------|--------------------|----------------------------|-----------|---------|
| 0 | Status quo (Go RBAC only) | Authorization only | Still minted | ≤1 h (identity), 0 s (privilege) | Immediate for authority; identity still accepted | Immediate (re-resolved) | none | Compensating control works; product gap stays OPEN. Recommended only if owner accepts the oracle trade-off. |
| 1 | `databaseHooks.session.create.before` SQL gate in `auth.better.ts` | Identity + session creation | Refused at creation | still ≤1 h for *already-issued* tokens | Immediate at creation for any subject with an `app_users` row | unchanged | none | **Recommended, with the corrected fail-closed semantics of §4.1.** Smallest change; refuses when a mapped identity is inactive/deleted **and** when the check itself cannot run; keeps all clocks. |
| 2 | Better Auth admin plugin (`banned` / `banReason` / `banExpires`) | Identity layer, native | Refused natively; expiry supported | still ≤1 h | Tied to `auth.user.banned`, must be synced with `app_users.status` | unchanged | **Yes** — `auth.user` schema change + dual-source-of-truth sync | Heavier; two deactivation sources to keep consistent. Reject unless native ban expiry is required. |
| 3 | Server-side session introspection in Go (every request checks `auth.session`) | Identity, zero-lag | n/a | 0 s (no JWT accepted after revoke) | Immediate | Immediate | none | Strongest, but couples the API to the web tier's session store and puts the web tier in every API request path. Reject for now; revisit if the 1 h bound is ever unacceptable. |
| 4 | Shorten JWT TTL to 15 m | Identity (bounded tighter) | Still minted | ≤15 m | 0 s privilege / ≤15 m identity | Immediate | none | Cheap complementary lever, not a deactivation gate. Can be layered on Option 1 if the owner wants a shorter identity bound. |

**Recommendation: Option 1, with Option 4 as an optional complementary lever;
explicitly reject Options 2 and 3 for this milestone.**

---

## 4. Recommended design (Option 1, owner-gated)

### 4.1 Identity-layer account disabling
Add a `databaseHooks.session.create.before` hook in
`apps/web/src/lib/server/auth.better.ts`. Before a session row is created,
query `public.app_users` by the auth user id as `subject`:

```sql
SELECT status, deleted_at FROM public.app_users WHERE subject = $1 LIMIT 1;
```

**Corrected model (review 2026-11-19).** The earlier draft said "fail-open for
citizens without an `app_users` row". That was inaccurate: the
`databaseHooks.user.create.after` hook at `auth.better.ts:148-190` provisions an
`app_users` row for **every** new `auth.user`, citizen included (status `active`).
So a subject with no row is the exception — a provisioning failure (the email-conflict
path at `:165-188`), not the citizen norm. Hence:

- If a row exists and is `status <> 'active'` OR `deleted_at IS NOT NULL` →
  **throw; no session is created** (explicitly disabled/deleted identity).
- If the query itself errors (DB unavailable) → **throw; fail closed.** A transient
  identity-check failure must never mint a session for an identity whose status
  could not be confirmed. This is the load-bearing correction: the security
  invariant forbids an explicitly disabled identity from regaining access
  *because a dependent check failed*.
- If no row exists → **throw; fail closed** as well. Such an account has no SIGAP
  identity (Go would resolve an empty `AppUserID` and 401 anyway), so allowing a
  session would only create a token that authorizes nothing. If the owner later
  wants self-healing provisioning, add an explicit create path — do not use a
  silent fail-open here.
- The check must run against the same DB the Go resolver reads; never a cached
  copy. Net effect: the hook only ever mints a session for a **confirmed-active**
  mapped identity.

### 4.2 Existing Better Auth session invalidation
On staff deactivation, the deactivation operation (whichever admin surface
owns it) must also delete the user's `auth.session` rows. There is no new
session to block after 4.1, but existing sessions must be revoked so
`hooks.server.ts` stops minting. Reuse the existing Better Auth
`revoke-sessions` mechanism; do not invent a parallel store.

### 4.3 Previously issued JWTs
Unchanged: they remain valid to `exp` (≤1 h). Enforced by signature/iss/aud/exp
only; `permissions` claim ignored; RBAC re-resolved from DB. The web tier
stops minting the moment the session row is gone (4.2). Documented as the
accepted bound — do not claim zero-lag identity revocation.

### 4.4 Maximum revocation latency
- Privilege / facility / role changes: **0 s** (next API request).
- Identity acceptance of a pre-revocation token: **≤1 h** (JWT TTL, zero
  leeway). Optional lever: lower `jwt.expirationTime` to 15 m (Option 4).

### 4.5 Staff deactivation
Deactivation remains a single authoritative act on `app_users.status` /
`deleted_at`. The 4.1 hook makes the identity tier agree; the Go resolver stays
the authorization authority. There must be **one** source of truth
(`app_users`), not a second `auth.user.banned` flag.

### 4.6 Multi-facility role changes
No code change: facility/org scope is resolved per request from `user_roles`
and its `facility_id`/`organization_id`, with deny-by-default. A role change or
assignment disable therefore also takes effect on the next request. Regression
tests must keep covering cross-facility denial and sibling-org 404s.

### 4.7 Audit events
Deactivation, session revocation and the hook's refusal must produce an audit
record. Note the existing OPEN owner decision (gap 9): Better Auth emits no
event the Go hash-chained `audit_events` table can see. Until an audit owner/
chain is chosen, the available record is the `auth.session` rows plus the
treatment in 4.1; this ADR does not pretend the Go audit sink is wired.

### 4.8 Recovery and reactivation
Reactivation = set `app_users.status='active'`, clear `deleted_at`, restore the
needed role assignments. The 4.1 hook then permits new sessions again. No
automatic re-issue of a previously revoked JWT — the user signs in afresh.
Record reactivation as an audit event (subject to 4.7).

### 4.9 MFA implications
`STAFF_MFA=PARTIAL`: enrol/verify/backup-code sign-in is proven, but
`/two-factor/disable`, `/generate-backup-codes`, lockout ceiling, token-expiry
clocks and MFA recovery remain unverified. A deactivation gate at session
creation does **not** by itself revoke TOTP secrets or backup codes; after
reactivation those may still be usable. Any MFA recovery policy must be decided
together with this ADR. Identity-layer deactivation does not weaken any MFA
assertion.

---

## 4A. Six-state invariant analysis (review 2026-11-19)

The governing invariant: **an explicitly disabled or deleted identity must not
regain protected application access because a dependent identity check fails.**
Applied to each distinct state:

| # | State | New session (web) | Existing session | Go RBAC per request | Pre-issued JWT | Invariant holds? |
|---|-------|-------------------|------------------|---------------------|----------------|------------------|
| 1 | Identity provider (Better Auth) unavailable | none can be minted (auth routes fail) | unchanged until exp/revoke | DB RBAC resolves independently | verifies to exp | **Yes** — outage cannot mint a new token; no fail-open mint path exists |
| 2 | Existing valid session | n/a | still authoritative for JWT minting | full permissions from DB | valid to exp | Yes |
| 3 | Application identity explicitly disabled (`status<>'active'`) | refused by §4.1 hook | should be revoked by §4.2; until then web still mints but Go denies | zero permissions (fail closed) | identity accepted to exp, no authority | **Yes after §4.1+§4.2** — authority always 0; today (no hook) gap is OPEN |
| 4 | Identity deleted / suspended (`deleted_at` set or auth account removed) | refused by §4.1 hook (row inactive) or no auth user to sign in | revoked by §4.2 | zero permissions | to exp, no authority | Yes after §4.1+§4.2 |
| 5 | Staff roles revoked (assignments disabled) | session still created (identity active) | unchanged | grants vanish next request → 403/404 | no authority | **Yes** — RBAC is re-resolved, roles are not cached in JWT |
| 6 | Already-minted JWT still valid | n/a | n/a | permissions re-resolved; claim ignored | identity accepted ≤1 h, zero authority | Yes — bounded, documented latency |

**What this changes vs the earlier draft:** the fail-open "no `app_users` row"
case is removed — it was the one path where a failed/missing identity check
could have produced a session. The hook now fails closed on error and on a
missing row, so state 1 (provider outage) and a transient DB error cannot be
confused with a confirmed-active identity. Note the distinction: *Go RBAC is the
authority for access* and already fails closed for states 3–6; the §4.1 hook
tightens *identity acceptance* so the two layers agree. It does **not** shorten
the ≤1 h JWT identity bound (state 6) — only a shorter TTL or server-side
introspection would do that, both owner-gated.

**Owner decisions still required:** (a) adopt §4.1 fail-closed semantics (this
review's correction); (b) error wording at sign-in (generic vs explicit — see §2
oracle trade-off); (c) who performs §4.2 session revocation on deactivation;
(d) audit/recovery treatment (§4.7–4.8); (e) MFA recovery alignment (§4.9).

---

## 5. Regression tests required before this could be accepted
1. Disabled `app_users` row (`status<>'active'`) → session creation refused by
   the §4.1 hook; assert no `auth.session` row and no JWT minted. Repeat for
   `deleted_at IS NOT NULL`.
2. **No `app_users` row → session refused (fail closed), not the earlier
   fail-open citizen assumption.** Add a negative-control case where the status
   query throws (simulated DB outage) → session creation refused, proving a
   failed dependency cannot mint a session (the §4A invariant).
3. Active mapped identity → session created (positive control, so the gate is
   not vacuously denying everyone).
4. Deactivation with an existing session → `revoke-sessions` removes the row;
   replayed signed cookie rejected (`get-session` null, `/token` 401).
5. Go RBAC still denies a disabled subject (existing `rbac_resolver_test.go`
   negatives remain green).
6. Cross-facility / sibling-org denial unchanged after role changes.
7. Reactivation → new session allowed; old revoked JWT not resurrected.

---

## 6. What this ADR does NOT do
- No code change, no commit, no push, no deployment.
- No change to `auth.user` schema (Option 2 rejected).
- No shortening of JWT TTL (Option 4 optional, owner-gated).
- No claim that `SESSION_LIFECYCLE` or `STAFF_MFA` is now READY — both stay
  **PARTIAL** until the owner decision and the tests above land.

**Owner decision needed:** choose 0 / 1 / 4 and approve error-wording and audit
treatment. Until then `IDENTITY_DEACTIVATION_POLICY=OPEN`.

---

## 9. Implementation record — 2026-11-19 (branch `feat/sigap-identity-deactivation`)

Owner **adopted Option 1** with the ten policy items in the task brief. Implemented
in an isolated worktree based on PR #97 head `2b1bb86`; PR #97 itself is untouched.

### 9.1 What was actually implemented
- `apps/web/src/lib/server/identity-status.ts` — `assertActiveIdentity(subject)`.
  Queries `public.app_users` by `subject` and **throws a single generic
  `APIError.from("UNAUTHORIZED", { message: "Email atau kata sandi salah.",
  code: "INVALID_EMAIL_OR_PASSWORD" })`** for every denial: blank/non-string
  subject, no DB pool, query error (DB outage), no row, `status <> 'active'`, or
  `deleted_at IS NOT NULL`. Returns void only for a confirmed-active identity.
  No SQL/status/existence signal is exposed — the login action maps that exact
  APIError shape to the same message as a wrong password (anti-enumeration).
- `apps/web/src/lib/server/auth.better.ts` — `databaseHooks.session.create.before`
  calls `assertActiveIdentity(session.userId)`. In Better Auth 1.7.20
  (`with-hooks.mjs:11`) a thrown error from `create.before` propagates and the
  session row is never written; no JWT can be minted for a refused identity.
  Coverage of creation paths (static source verification): every session write
  goes through `internalAdapter.createSession`, which is wrapped by
  `createWithHooks(data, "session", …)` (`internal-adapter.mjs:336`); that includes
  the ordinary `signInEmail` path (autoSignIn is false at sign-up) and the
  two-factor/MFA path where a verified TOTP mints a fresh session
  (`plugins/two-factor/index.mjs:174`). No alternative session-creation path that
  bypasses the adapter hook was found. This is a source-level confirmation, not a
  runtime proof (no disposable DB was available locally).
- `apps/web/scripts/provision-staff.mjs --deactivate` is the **real, existing**
  deactivation operation: it sets `app_users.status='disabled'` and calls
  `internalAdapter.deleteUserSessions` (verified `after === 0`, `fail()` on
  partial failure). Its stale "can still create a session" note was corrected to
  reflect the new gate. No new public admin endpoint was invented.

### 9.2 ADR statements kept true after implementation
- The ≤1 h JWT identity bound is **unchanged** — a previously issued JWT still
  verifies to `exp`; only new-session creation is gated. No zero-second claim.
- Go RBAC still re-resolves `app_users.status` per request (zero authority for a
  disabled subject) — unchanged, still the authorization authority.
- There is still **one** source of truth (`public.app_users`); no `auth.user.banned`.
- MFA behaviour is untouched (`STAFF_MFA=PARTIAL`).
- Audit: refusing the session is observable in logs/the generic error; there is
  no Go `audit_events` write (gap 9 unchanged, owner/infra-blocked).

### 9.3 Test evidence (honest)
- `apps/web/src/lib/server/identity-status.test.ts`: **6/6 PASS** with a FAKE pool
  (ACTIVE allowed; DISABLED/SOFT-DELETED/MISSING/DB-error/blank-subject all denied
  with the generic 401 shape). Runs without a DB.
- `apps/web/tests/identity-deactivation.integration.test.js`: DB-gated integration
  suite for the 16 mandated scenarios (real session rows, responses, JWT bound,
  reactivation). **NOT RUN locally** — no disposable Postgres credentials on this
  host; it SKIPs without DB and FAILs with `SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB=1`
  (no fake PASS). CI must run it with disposable Postgres.
- Frontend `pnpm test:unit`: **768/768 PASS**; `pnpm check` 0/0; production build PASS.
- `go build ./...` / `go test ./...`: PASS (DB-gated Go suites skip locally without
  `DATABASE_URL`).

### 9.4 Real-database verification — 2026-11-19 (disposable PostgreSQL 18)
A task-owned, loopback-only PostgreSQL 18 cluster (port 55432, db
`sigap_identity_test`, user `sigap_test`, self-signed SSL) was provisioned,
migrations 0001–0014 applied (14/14), and the Better Auth auth schema created
via `getMigrations(auth.options)` (6 tables). No shared/dev/production DB was
used. Verified with REAL runtime behaviour:

- **integration test (built app, HTTP API, real session rows):** 22/22 assertions
  PASS — active citizen/staff login 200 + session row; inactive/soft-deleted/
  missing-identity login 401 with generic `INVALID_EMAIL_OR_PASSWORD` and **no
  session row**; simulated DB outage fails closed; repeated attempts stay denied;
  reactivation allows a fresh session. The test was rewritten as a built-app HTTP
  runner (Node runtime) because the prior `.js` variant that imported `.ts`
  directly could not resolve SvelteKit's `$app/*` virtual modules.
- **real deactivation operation:** a live session (count 1) was created via the
  built app, `provision-staff.mjs --deactivate <email> --apply` ran against the
  disposable DB → `sessions_after=0`, `app_users.status=disabled`, subsequent
  login 401. `DEACTIVATION_RUNTIME=PASS`.
- **DB-backed regression suites:** session-security **62/0/0**, session-lifecycle
  **67/0/0**, auth-config **0/0/0** — all with `REQUIRE_DB=1` semantics, zero skips.
  Two stale gap-characterization assertions were updated from "disabled still
  signs in" to "disabled is refused at the identity layer" (strengthening, not
  weakening); the expiry test now uses a fresh post-reactivation device.
- strict attribution (`2b1bb86..HEAD`, mode strict) PASS; Gitleaks 0 leaks;
  `pnpm check` 0/0; `pnpm test:unit` 768/768; production build PASS;
  `go build ./...` / `go test ./...` PASS (DB-gated suites skip locally without
  `DATABASE_URL`; with the minimal disposable DB three schedule tests that need a
  demo facility fixture fail — an environment-fixture issue, not a regression;
  CI's full seed supplies that fixture).
- **MFA runtime path:** NOT exercised end-to-end locally. Hook coverage of the
  two-factor session mint was confirmed from library source in §9.2, but no TOTP
  verification test was driven against the disposable DB. `MFA_HOOK_RUNTIME=NOT_RUN`.
- **Go per-request RBAC / regional / patient isolation:** not re-driven against
  the disposable DB beyond the existing suites; covered by PR #97's 9/9 CI on
  head `2b1bb86` (unchanged by this feature branch).
- **deactivation/login race:** source review shows the gate queries live status
  at each sign-in and the deactivation operation revokes sessions then sets
  status; an in-flight login that passes the gate just before deactivation could
  still mint a session that deactivation then revokes (the revocation runs after
  and deletes all sessions). No atomic cross-system guarantee is claimed — the
  lifecycle contract's zero-privilege outcome still holds via Go RBAC. Status OPEN
  for an explicit race test.

### 9.5 CI / stacked-PR integration plan
`.github/workflows/ci.yml` triggers `pull_request` only for `main` and
`design/ui-ux-overhaul`. A stacked PR targeting `feat/sigap-selfhosted-auth`
will **not** auto-trigger CI. Least-disruptive options (owner-gated, none done
here): (a) run the PostgreSQL CI job locally/reproduce the nine checks before
opening the stacked PR — already done for the relevant suites above; (b) with
owner approval, add `feat/sigap-selfhosted-auth` to the workflow's PR target so
the stacked PR gets real CI; (c) after PR #97 merges to `main`, rebase this branch
onto the new `main` and open a normal PR (which triggers CI). Do NOT open a PR to
`main` now (would duplicate PR #97) and do NOT modify the protected parent PR.

### 9.7 Final security gates — 2026-11-19 (real MFA + race + fixture parity)
A second task-owned disposable PostgreSQL 18 cluster (loopback `127.0.0.1:55433`,
db `sigap_mfa_test`, user `sigap_test`, self-signed SSL) was used. Migrations
0001–0014 (14/14), Better Auth schema (6 tables), and the additive demo fixtures
(`dev.sql` + `rbac.sql` + `demo.sql`) were applied. No shared/production DB used.

- **MFA/TOTP runtime — PASS (18/18).** Real enrolled TOTP (RFC 6238 generated
  from the enable response's base32 secret) through the built app's HTTP API:
  (A) active TOTP sign-in mints a session; (B) disabling between credential
  sign-in and TOTP verification blocks verification and leaves **no** session;
  (C) deactivation revokes the MFA-authenticated session; (D) after reactivation
  a fresh MFA login succeeds and the old session is not resurrected. This closes
  the prior `MFA_HOOK_RUNTIME=NOT_RUN` gap with runtime evidence, not source
  inspection. (`tests/mfa-runtime.mjs`)
- **Deactivation/login race — PASS (bounded concurrency, 8 iterations).** A
  sign-in request and the real `--deactivate --apply` operation were interleaved
  repeatedly; after deactivation completed, **no usable session remained** and
  status was `disabled`. The final-state invariant holds because deactivation
  deletes all sessions after the status update. No atomic cross-system guarantee
  is claimed — only the verified post-deactivation invariant. (`tests/deactivation-race.mjs`)
- **Go fixture parity — PASS.** The three previously failing schedule tests pass
  once `demo.sql` (which seeds facility `...d000`) is applied after `dev.sql` and
  `rbac.sql`; the earlier failure was a missing demo fixture, not a defect. Full
  `go test ./...` passes with the seeded disposable DB.
- **Targeted regression:** identity integration 22/22; session-security 62/0/0;
  session-lifecycle 67/0/0; auth-config 0/0/0 (all DB-gated, zero skips); Vitest
  768/768; svelte-check 0/0; strict attribution PASS; Gitleaks 0 leaks.

### 9.8 Residual limitations (final)
- `SESSION_LIFECYCLE` stays **PARTIAL** only for the unresolved MFA **recovery /
  backup-code / lockout lifecycle** acceptance criteria — the core identity gate,
  TOTP session creation, deactivation, race invariant and session revocation are
  now runtime-verified. `STAFF_MFA=PARTIAL` for the same recovery/lockout gaps.
- No atomic cross-system revocation is claimed: session deletion + app_users status
  update are separate operations with observable partial failure, not one transaction.
- Race verification is bounded (8 iterations) final-state, not a proof of the
  absence of every transient timing window; the approved policy's zero-authority
  outcome is additionally enforced by Go's per-request RBAC.
- The intermittent 429 rate-limit observed during tests was a shared-bucket test
  artifact (no forwarded client IP); tests forward rotating loopback IPs. It is
  not an identity-gate behaviour and does not affect production semantics.
- Previous JWTs remain valid to their 1 h `exp` (documented bound); no zero-second
  JWT revocation is claimed. Race conditions in the previous_hash audit chain are
  unrelated and unchanged.
- CI: `.github/workflows/ci.yml` triggers PRs only for `main` and
  `design/ui-ux-overhaul`; a stacked PR to `feat/sigap-selfhosted-auth` will not
  auto-run CI. All nine relevant suites were reproduced locally on the feature
  head. Owner must approve either a workflow target addition or a post-merge PR.
