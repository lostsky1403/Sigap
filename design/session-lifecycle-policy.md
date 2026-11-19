# Session lifecycle & authorization freshness policy (SIGAP self-hosted identity)

**Status:** Normative for `apps/web/src/lib/server/auth.better.ts`.
**Date:** 2026-11-11
**Subject:** Better Auth session lifetime, the fresh-session window, and — the part
that has to be stated out loud — **how long a revoked session keeps working**.

---

## 1. The three clocks (configured, not inherited)

`auth.better.ts` pins the `session` block; the values are asserted by
`apps/web/tests/session-lifecycle.test.js` (static phase) so this document and the
code cannot drift.

| Clock | Value | Configured as | Meaning |
|---|---|---|---|
| Idle ceiling | **7 days** | `session.expiresIn = 60 * 60 * 24 * 7` (604800 s) | `expiresAt` is stamped `now + 7d` at sign-in and at each refresh; past it the session row is expired and `getSession` returns `null`. |
| Refresh cadence | **1 day** | `session.updateAge = 60 * 60 * 24` (86400 s) | A session in use at least once per day has `expiresAt` rolled forward. A continuously-used browser session therefore survives indefinitely, an abandoned one dies ≤7 days after the last refresh. |
| Fresh window | **1 hour** | `session.freshAge = 60 * 60` (3600 s) | Window for "sensitive" endpoints. Measured from `session.createdAt`. |

Two of the three restate better-auth 1.7.20 defaults (`expiresIn` 604800,
`updateAge` 86400) — they are written down anyway because this policy is the
normative source. `freshAge` is **tightened 24x** (default 86400 → 3600).

### 1.1 What `freshAge = 1h` actually gates

Verified against the installed `better-auth@1.7.20`
(`apps/web/node_modules/better-auth/dist/api/routes/…`):

| Endpoint | Gate | Consequence after the window lapses |
|---|---|---|
| `GET /api/auth/list-sessions` | `freshSessionMiddleware` (`session.mjs:347-350, 336-344`) | **403 `SESSION_NOT_FRESH`** — the device list is not readable from a stale session. |
| `POST /api/auth/delete-user` with no `password` in the body | freshness check (`update-user.mjs:325-328`) | **400 `SESSION_EXPIRED`** — account deletion then requires the password (or a fresh session). |
| `POST /api/auth/change-password` | `sensitiveSessionMiddleware` only (`update-user.mjs:77-97`) | **Not freshness-bound.** The `currentPassword` it demands *is* the re-authentication factor. |
| `POST /api/auth/revoke-session`, `/revoke-sessions`, `/revoke-other-sessions` | `sensitiveSessionMiddleware` only (`session.mjs:377-380, 420-422, 450-453`) | Authoritative session required, freshness not required. |
| `GET /api/auth/get-session`, `GET /api/auth/token` | session lookup only | Unaffected. |

**Consequence to plan for:** the refresh at `updateAge` writes `expiresAt`/`updatedAt`
only (`session.mjs:181-201`) — `createdAt` is never rolled. So freshness is *time since
sign-in*, not *time since last activity*: after one hour in a single continuous browser
session, `GET /list-sessions` stops answering and the operator must sign in again to
manage devices. That is the intended posture for privileged staff (a long-lived tab must
not be able to enumerate or revoke devices), and it is why `change-password` is
deliberately *not* freshness-gated: it carries its own proof of identity.

---

## 2. Revocation semantics — the honest part

A **Better Auth session revocation is not a JWT revocation.**

```
sign-out / revoke-session / revoke-sessions / change-password(revokeOtherSessions)
        │  deletes the `auth.session` row
        ▼
apps/web (hooks.server.ts)
        │  auth.api.getSession() → null  ⇒ NO new Bearer is minted
        │  (locals.jwt stays null, proxies forward no Authorization header)
        ▼
apps/api  ← an ALREADY-MINTED Bearer keeps verifying: ES256 signature, iss, aud, exp
        │  (`apps/api/internal/auth/jwt_provider.go:259-264`, golang-jwt default
        │   leeway = 0 — `jwt.WithLeeway` is not set)
        ▼
        authorization permissions are re-resolved from the DB on EVERY request
```

So there are two different latencies, and they must not be conflated:

1. **Permission / privilege revocation: immediate (0 s).** The Go API ignores the token's
   `permissions` claim (AUDIT-101) and resolves RBAC from the database per request, with
   `u.status = 'active' AND u.deleted_at IS NULL AND ur.status = 'active' AND
   ur.deleted_at IS NULL` filters (`apps/api/internal/auth/rbac_resolver.go:101-131`).
   Removing a role, disabling an assignment, deactivating or soft-deleting an
   `app_users` row takes effect on the **next** API request, whatever the token says.
2. **Identity acceptance of an already-issued token: bounded by the access-token TTL.**
   A revoked session's JWT still authenticates the same `sub` until `exp`.

### 2.1 Maximum revocation delay — chosen bound

> **Maximum authorization freshness delay for a revoked session = the `jwt` plugin
> `expirationTime` = 1 hour.** Zero leeway (no `jwt.WithLeeway` in the Go validator, no
> token caching in front of it).

For **privileged staff** this 1 hour is the **accepted bound**, on the strength of the
compensating control in (1) above: privilege loss is immediate, and only *identity*
(being recognized as the same subject) can lag, for at most 1 h, on a token that was
already handed out before the revocation. The web tier stops minting immediately, so the
window only covers tokens a client already holds (a browser tab, a script, a cached
`Authorization` header).

**Levers, if a shorter bound is ever demanded** (pick one, in this order):

| Lever | Effect | Cost |
|---|---|---|
| Lower `jwt.jwt.expirationTime` (e.g. `"15m"`) | Bounds everything, all users | Every client must re-fetch `/api/auth/token` within the TTL. Minting is already per-request in `hooks.server.ts`, so a *browser* pays nothing extra; a script holding a token must refresh. |
| A second Better Auth instance / issuer for staff with its own shorter TTL | Short bound for staff only | Second JWKS + `SIGAP_AUTH_*` wiring for the Go API; two identity stores to operate. Not implemented. |
| Server-side session introspection in the Go API (check the session is still live) | Zero-lag revocation | New coupling from the API to the web tier's session store; not implemented, and it would put the web tier in the request path of every API call. |

This 1 h is therefore **a decision, not an oversight**, and it is the number to quote when
asked "how long does a fired employee's token work?".

---

## 3. Account deactivation (`INACTIVE` / banned)

**Enforcement point: the Go API's RBAC resolver, on every request — not session creation.**
SIGAP's deactivation truth is `public.app_users.status = 'active'` (+ `deleted_at IS NULL`),
consumed by `rbac_resolver.go` (see §2.1(1)). A deactivated staff member's `sub` resolves to
**zero permissions on their next API call**, well inside the 1 h identity bound.

**Stated gap (deliberate, and recorded here rather than left implicit):** the Better Auth
layer does **not** refuse to *create* a session for a deactivated account:

- better-auth 1.7.20 base config has no ban/status field on `auth.user` — `banned` /
  `banReason` / `banExpires` arrive only with the **admin plugin**, which is not installed,
  and `sign-in/email` performs no such check in the base (`dist/plugins/admin/*` is the
  only place `banned` is read).
- `user.validateUserInfo` is documented to be skipped for email/password sign-ins, so it
  cannot serve as the gate either.

Therefore, with today's configuration, a deactivated account can still obtain a Better
Auth session and a JWT; the API refuses to authorize it. If a hard gate is required later
(enforcement at the identity boundary), the supported mechanisms are, in order of
precision:

1. `databaseHooks.session.create.before` in `auth.better.ts` — reject when
   `public.app_users` has a row for the user's id (as `subject`) that is not active:
   ```sql
   SELECT 1 FROM public.app_users
   WHERE subject = $1 AND (status <> 'active' OR deleted_at IS NOT NULL);
   ```
   Fail-open for identities with no `app_users` row (citizen/patient accounts created
   outside staff provisioning), fail-closed for provisioned staff. This tightens
   *identity* to match *authorization*; it does not change the 1 h bound in §2.1.
2. Install the admin plugin and use `banned` + `banExpires` (queues a schema migration
   for `auth.user`).

Whichever is chosen, the policy question to answer first is a product one: should a
deactivated account see an authentication *error* (revealing provisioning state), or a
technically-successful sign-in that authorizes nothing? Today's answer is the latter.

---

## 4. Lifecycle operations (contract as implemented)

| Operation | Endpoint | Effect on sessions | Notes |
|---|---|---|---|
| Sign-up | `POST /api/auth/sign-up/email` | Creates the user + one session (auto sign-in) | `emailAndPassword.enabled: true`, `autoSignIn` default true. |
| Sign-in | `POST /api/auth/sign-in/email` | Creates one session per call (one row per device) | Sessions are independent; nothing is deduplicated. |
| Sign-out | `POST /api/auth/sign-out` | Deletes *this* session row, clears the cookie | Other devices unaffected. |
| Cross-device revocation | `POST /api/auth/revoke-session` `{token}` | Deletes the given session | Requires a live (authoritative) caller session; the target `token` comes from `GET /list-sessions`. |
| Revoke everything | `POST /api/auth/revoke-sessions` | Deletes all of the user's sessions | Caller's own session included. |
| Password change | `POST /api/auth/change-password` `{newPassword, currentPassword, revokeOtherSessions?}` | With `revokeOtherSessions: true`: deletes **all** sessions, then mints a fresh one for the caller (returned as `token` + set as cookie) | `currentPassword` is mandatory (400 `INVALID_PASSWORD` otherwise); declared intent documented by `user.changePassword.enabled: true`. |
| Idle expiry | — | `getSession` → `null` after `expiresAt` | 7 days without refresh. |

`hooks.server.ts` mints the ES256 JWT per request and only when a session resolves, so
every row above is also a "no new token minted from here on" statement for that session.

---

## 5. Test coverage map

`apps/web/tests/session-lifecycle.test.js` (plain node, like the other `tests/*.test.js`;
**not** collected by vitest, whose `include` is `src/**`). Run it directly, or add
`&& node tests/session-lifecycle.test.js` to the `test` script in `apps/web/package.json`
after the existing `tests/auth-actions.test.js` step.

| Phase | DB needed | Covers |
|---|---|---|
| 1 — static | no | `session` values (semantics-evaluated, not string-matched), `user.changePassword.enabled`, jwt plugin untouched (`ES256`, issuer, audience, `1h`), no `disableSessionRefresh`, and this document's numbers matching the code. |
| 2 — built app, no DB | no | The built server boots with the auth routes mounted: unauthenticated `get-session` is a clean `null` (degrade, never 500) and `sign-out` without a cookie is a no-op success. |
| 3 — live lifecycle | **yes** (`SIGAP_DATABASE_URL`) | sign-up → session; sign-in on a second device → 2 sessions; `list-sessions` exposes per-device tokens; cross-device `revoke-session` → that device's session gone; sign-out → own session gone; `change-password` wrong current password → 400; correct password + `revokeOtherSessions` → other sessions gone and old password rejected; **access JWT `exp − iat = 3600 s` (the §2.1 bound, measured on a real token)**. |

Skipped-when-DB-absent cases are printed as `[GATE] SKIP …` with the reason and the exact
command to run them (`SIGAP_DATABASE_URL=… node tests/session-lifecycle.test.js`). Set
`SIGAP_SESSION_LIFECYCLE_REQUIRE_DB=1` to turn a skip into a failure (for a CI job that
always has Postgres). Phase 3 also records, as documentation rather than assertion:

- the **fresh-window gate cannot be exercised without waiting out `freshAge`** (1 h), so
  it is verified statically in phase 1 and by source reference in §1.1;
- the **deactivation gate of §3** — asserted only to the extent that the `auth` schema has
  no ban/status column to drive it; the behavioural claim (a deactivated `sub` resolves to
  zero permissions) belongs to the Go API's resolver tests
  (`apps/api/internal/auth/*_test.go`), not to the web tier.

---

## 6. Operational notes

- Table ownership: everything lives in the `auth` schema (see
  `design/better-auth-schema.md`); `public` and the migration ledger are untouched.
- Rollback of Better Auth entirely: `DROP SCHEMA auth CASCADE;`.
- Re-running the tests needs the dev Postgres on `127.0.0.1:5433` (SSLMODE handling is in
  `apps/web/src/lib/server/db.ts`) with `auth` migrated
  (`npx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts`).
