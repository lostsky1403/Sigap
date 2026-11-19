# Auth deployment readiness — Better Auth (self-hosted) production assessment

**Status:** assessment only. **No deployment was performed and no gate was run.**
**Date:** 2026-11-11
**Branch / commit:** `feat-sigap-selfhosted-auth` @ `60de7d6` (worktree `F:\sigap-design-v2`)
**Scope:** the ten production-readiness questions for the self-hosted Better Auth identity
tier (web) plus the Go API's JWT trust boundary (api).

**Method.** Every claim below is read from the repository in this worktree. Where the
mechanism lives in the installed library, the citation is the installed package source
(`apps/web/node_modules/better-auth`, resolved version **1.7.20**), because that is the code
that actually executes. Anything that lives only on a host (a secret, a DNS record, a
TLS toggle) is marked **OPERATOR VERIFICATION REQUIRED** and is never asserted.

**Headline finding.** Better Auth itself is code-complete and its security defaults are
sound (HttpOnly, SameSite=Lax, HTTPS-derived `Secure`, an explicit ES256 keypair pinned for
the Go verifier). What is missing is the **deployment envelope around it**: the compose
topology forwards *no* `BETTER_AUTH_*` variable and no `SIGAP_DATABASE_URL` to the web
service, `.env.example` names none of them, the deployment runbook's web env matrix predates
Better Auth entirely, no Postgres role separates the identity schema from the domain schema,
and no auth event reaches the Go audit table. Treat the ten items below as *code READY /
deployment GAP*.

---

## Verdict table

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

**Counts: 1 READY / 9 GAP.**

---

## 1. HTTPS termination, proxy trust, HSTS — **GAP**

### Current mechanism

**API (ready).** `SIGAP_TLS_TERMINATED` is a hard fail-fast control, not a hint:

- `apps/api/internal/config/envguard.go:148-162` — `GuardTLS()` treats `SIGAP_ENV` unset or
  `local` as exempt (`:149-151`) and otherwise requires
  `SIGAP_TLS_TERMINATED` to equal `true` case-insensitively (`:155`), failing with
  *"the API listens on plain HTTP and requires a TLS-terminating reverse proxy in front"*.
- `apps/api/cmd/server/main.go:151-155` calls it before the listener binds and exits 1.
- HSTS on API responses is set from the *request*, not from the flag:
  `apps/api/internal/router/headers.go:33-34` —
  `if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"` →
  `Strict-Transport-Security: max-age=63072000; includeSubDomains`.
- Proxy-header trust is explicit and hop-counted:
  `apps/api/internal/router/proxy.go:27-76`. Outside local it peels
  `SIGAP_TRUSTED_PROXIES` entries off the right of `X-Forwarded-For` (`:46-51`), and in
  local mode it **strips** `X-Forwarded-For`, `X-Real-IP` **and** `X-Forwarded-Proto`
  (`:58-66`) so a local spoof cannot reach downstream code. Code default is 1 hop
  (`proxy.go:46`); `docker-compose.yml:125` sets the documented production value
  `SIGAP_TRUSTED_PROXIES: ${SIGAP_TRUSTED_PROXIES:-2}` (edge + SvelteKit proxy), matching
  `docs/operations/DEPLOYMENT_RUNBOOK.md` §5.1.

**Web tier.** `apps/web/src/hooks.server.ts:78-79` emits HSTS only when
`event.url.protocol === 'https:'`, with `max-age=63072000; includeSubDomains`. It also always
sets `X-Frame-Options: DENY` (`:51`), `X-Content-Type-Options: nosniff` (`:72`) and
`Referrer-Policy: strict-origin-when-cross-origin` (`:75`); CSP comes from
`kit.csp.directives` in `apps/web/svelte.config.js` (nonce mode, `frame-ancestors 'none'`).

### Gap

1. **The web tier reads no TLS flag at all.** `SIGAP_TLS_TERMINATED` is passed to the web
   container (`docker-compose.yml:159`) but has **zero consumers** in `apps/web/src` — HSTS
   keys off `event.url.protocol` (`hooks.server.ts:78`) and cookie `Secure` keys off
   `BETTER_AUTH_URL` (§2). The compose comment at `docker-compose.yml:157-159` ("relies on
   this to emit secure cookies/HSTS") is therefore inaccurate.
2. **Nothing asserts the public origin is `https`.** The web origin is
   `ORIGIN: ${SIGAP_WEB_ORIGIN:-http://localhost:3005}` (`docker-compose.yml:153`). If a
   production `.env` leaves `SIGAP_WEB_ORIGIN` at its `http://` default, the deploy comes up
   with **no HSTS** and **non-secure session cookies** while every Go-side guard still
   passes — a silent downgrade, because the Go guard only proves *the API* is behind TLS.

**Fix (one line):** in `scripts/ops/preflight-production-env.sh`, when `SIGAP_ENV` is
non-local require `SIGAP_WEB_ORIGIN` (and `BETTER_AUTH_URL`) to start with `https://`.

---

## 2. Cookie security — **READY**

Attrs are not hand-set anywhere; they come from the installed library defaults in
`apps/web/node_modules/better-auth/dist/cookies/index.mjs:25-43`:

| Attribute | Effective value | Evidence |
|---|---|---|
| `httpOnly` | `true` (hardcoded) | `dist/cookies/index.mjs:37` |
| `sameSite` | `"lax"` (hardcoded) | `dist/cookies/index.mjs:35` |
| `path` | `/` | `dist/cookies/index.mjs:36` |
| `secure` | `true` **iff** `BETTER_AUTH_URL` starts with `https://` | `dist/cookies/index.mjs:23,34` |
| name prefix | `__Secure-` when secure, else none; base `better-auth.` | `dist/cookies/index.mjs:23,28-32` |
| session cookie `maxAge` | `session.expiresIn` = 7 d (`auth.better.ts:74-78`) | `dist/cookies/index.mjs:49` |

`advanced` is not configured in `apps/web/src/lib/server/auth.better.ts`, so no
`useSecureCookies` / `defaultCookieAttributes` / `crossSubDomainCookies` override exists;
`session.lastLoginMethod`-style extras are absent too. Cookies are written through
SvelteKit's own `cookies` API via the `sveltekitCookies(getRequestEvent)` plugin
(`auth.better.ts:119`), which is registered **last** in `plugins` (`:98-119`) as the
integration contract requires.

### What changes when `BETTER_AUTH_URL` is `https`

`dist/cookies/index.mjs:19-23` flips `secureCookiePrefix` on, which simultaneously sets
`secure: true` and renames `/^better-auth\./` cookies to `__Secure-better-auth.*`
(`dist/cookies/index.mjs:23,28-32`). So pointing `BETTER_AUTH_URL` at the real `https` origin is
the single lever that makes session cookies `Secure` + `__Secure-`-prefixed. With the
current fallback (`auth.better.ts:43`, `http://127.0.0.1:5173`) a production deploy would
issue **non-Secure** cookies — the same missing assertion flagged in §1 and §10.

**Condition, not a gap in this item:** the mechanism is correct; it is the
`BETTER_AUTH_URL` value that is unasserted (see §1 gap 2, §3, §10).

---

## 3. CSRF / origin — **GAP**

`trustedOrigins` is **not set**. `apps/web/src/lib/server/auth.better.ts` configures only
`secret` (`:42`), `baseURL` (`:43`), `database` (`:44-48`), `emailAndPassword` (`:49-51`),
`session` (`:74-78`), `user` (`:90-92`), `rateLimit` (`:92-97`) and `plugins` (`:98-119`).
There is no `advanced` block, no `trustedOrigins`, and no `BETTER_AUTH_TRUSTED_ORIGINS` in
`.env.example` or compose.

### The default, exactly

`dist/context/helpers.mjs:60-77` (`getTrustedOrigins`) with a **static string** `baseURL`:

```js
const baseURL = getBaseURL(options.baseURL, options.basePath, request);
if (baseURL) trustedOrigins.push(new URL(baseURL).origin);        // :74      ← the whole list
if (options.trustedOrigins) { /* push ... */ }                     // :76-81   ← skipped, unset
const envTrustedOrigins = env.BETTER_AUTH_TRUSTED_ORIGINS;         // :83      ← unset
```

So the effective allow-list is **exactly one origin: the `BETTER_AUTH_URL` origin.** Two
consequences:

- It is *not* a wildcard and it is *not* `Host`-derived: proxy headers cannot widen it.
  `advanced.trustedProxyHeaders` defaults to `false`
  (`dist/context/helpers.mjs:108`), and this config never opts in.
- Correctness is entirely delegated to `BETTER_AUTH_URL`. Any state-changing Better Auth
  request whose `Origin` differs from that one value is rejected, and the failure mode looks
  like "sign-in does nothing" rather than a config error.

### Gap

`docker-compose.yml:148-170` (the `web` service env block) forwards `ORIGIN`, `SIGAP_ENV`,
`SIGAP_TLS_TERMINATED`, `SIGAP_API_INTERNAL`, `PUBLIC_SIGAP_API_BASE` and the two Supabase
values — but **not `BETTER_AUTH_URL`**. In the compose topology Better Auth therefore falls
back to `http://127.0.0.1:5173` (`auth.better.ts:43`) while the browser origin is
`ORIGIN` (`docker-compose.yml:153`, default `http://localhost:3005`). The derived allow-list
and the real origin disagree by construction, so the origin check refuses the request and
the whole sign-in/sign-out path is unreachable — the CSRF control is present and *too*
strict, because nothing wires its input.

SvelteKit's own origin check is a second, independent layer and is configured
(`ORIGIN`, `docker-compose.yml:149-153`, whose comment names "SvelteKit request-origin/CSRF
checks"), but it does not cover `/api/auth/*` semantics, which Better Auth enforces itself.

**Risk if left as-is:** the auth origin allow-list is undefined by configuration and
defaulted by accident. It fails closed today (denial of service on the login path), and it
would fail *open* in the opposite mistake — an operator "fixing" the 403 by setting
`BETTER_AUTH_URL` to a broad or wrong value, or by adding a permissive
`BETTER_AUTH_TRUSTED_ORIGINS`, with nothing reviewing that list.

**Fix (one line):** set `trustedOrigins: [process.env.BETTER_AUTH_URL ?? ""].filter(Boolean)`
explicitly in `auth.better.ts` (and pass `BETTER_AUTH_URL` into the web service) so the list
is declared, reviewable and single-valued rather than implicit.

---

## 4. JWT / JWKS rotation — **GAP**

### Current mechanism

**Issuer side (Better Auth, `jwt` plugin).** Products live at `${baseURL}/api/auth/jwks`
(`auth.better.ts:8-9`; `jwksPath` default `/jwks`, plugin `dist/plugins/jwt/index.mjs:26`),
with the keypair pinned to **ES256** (`auth.better.ts:100-102`) precisely because the Go
`parseJWK` accepts only `RSA`/`EC` (`apps/api/internal/auth/jwt_provider.go:135-177`). Tokens
are minted at `GET /api/auth/token` with `issuer: "http://127.0.0.1:5173"`
(`auth.better.ts:104`), `audience: "authenticated"` (`:105`) and `expirationTime: "1h"`
(`:113`).

Key lifecycle in the installed plugin:

- The key is **minted lazily on first use**, not at boot: `GET /jwks` calls `createJwk(...)`
  when the table is empty (`dist/plugins/jwt/index.mjs:118-121`), and the signing path does
  the same (`dist/plugins/jwt/sign.mjs:41,59`). It is persisted in the `jwks` table inside
  the `auth` schema (`dist/plugins/jwt/schema.mjs:2`; table list in
  `design/better-auth-schema.md:17-19`).
- Keys are filtered for liveness by `expiresAt`: `if (!key.expiresAt) return true`
  (`dist/plugins/jwt/index.mjs:125-128`), and signing picks the newest live key
  (`dist/plugins/jwt/adapter.mjs:24-25`).
- `expiresAt` is only ever stamped when `jwks.rotationInterval` is configured
  (`dist/plugins/jwt/utils.mjs:59`), and the JWKS display filter keeps an expired key live
  for a **default 30-day grace period** (`2592e3` s, `dist/plugins/jwt/index.mjs:125`).

**Verifier side (Go).** `apps/api/internal/auth/jwt_provider.go`:

- Cache TTL is **15 minutes**: `ttl: 15 * time.Minute` (`:41`), stamped on every successful
  refresh (`:130-131`).
- `get(kid)` (`:64-96`) returns a cached key while `now < expires`; an **unknown** `kid`
  falls through to `refreshLocked()` immediately (no negative caching, `:64-96`), and a
  transient refresh failure serves the **stale** key rather than failing closed (`:85-91`).
- `keyFunc` rejects `alg=none`/empty and rejects a token with no `kid` when JWKS is
  configured (`:321-333`); the allowed alg set is RS256/384/512 + ES256/384/512 (`:338-343`).
- `Validate()` requires `SIGAP_AUTH_ISSUER` **and** `SIGAP_AUTH_AUDIENCE` in jwt mode
  (`apps/api/internal/auth/config.go:78-90`); `SIGAP_AUTH_JWKS_URL` is technically optional
  there but the token path hard-fails without it (`jwt_provider.go:328-330`,
  *"no JWKS configured"*).

### Maximum delay to pick up a rotated key

| Event | Delay | Why |
|---|---|---|
| **New** key minted (new `kid` appears in JWKS) | **≈ 0** — next request | unknown `kid` forces an immediate refresh (`jwt_provider.go:64-96`) |
| **Retired/removed** key (or same `kid` re-minted with new material) | **≤ 15 min** | the cached key is served for the full TTL (`jwt_provider.go:41`, `:64-96`) |
| Token lifetime of an already-issued JWT | **≤ 1 h** (`auth.better.ts:113`) | Go verifies the JWT alone for authn; revocation bound is documented in `design/session-lifecycle-policy.md` |

### Gap

`jwks.rotationInterval` is **not set**, so `createJwk` stamps no `expiresAt`
(`dist/plugins/jwt/utils.mjs:59`), the liveness filter treats the key as permanently live
(`dist/plugins/jwt/index.mjs:125-128`), and the plugin has no scheduled rotation. The ES256
signing key is therefore **permanent until an operator manually deletes/regenerates the
`auth.jwks` row**. A key compromise has no bounded end, and the 15-minute Go cache is the
*only* bound on how long a deliberately retired key stays accepted.

**Fix (one line):** set `jwks: { keyPairConfig: { alg: "ES256" }, rotationInterval: 7776000 }`
(90 d) in `auth.better.ts` and record the manual rotate procedure + the 15-min verifier
bound in the runbook.

---

## 5. Secrets (`BETTER_AUTH_SECRET`) — **GAP**

### Current mechanism

- `apps/web/src/lib/server/auth.better.ts:42` —
  `secret: process.env.BETTER_AUTH_SECRET ?? "sigap-build-only-placeholder-secret"`.
  The comment (`:37-41`) explains the intent: `vite build` evaluates the module during SSR
  prerender with no env, so a clearly-marked placeholder avoids Better Auth's default-secret
  error.
- The build/runtime split is real: `scripts/dev/Run-V22Service.ps1:98-100` reads `.env` and
  sets `BETTER_AUTH_SECRET` / `BETTER_AUTH_URL` before serving.
- `.gitignore:2-3` ignore `.env` and `.env.*` (`!.env.example` re-admits the template); no
  secret is committed.
- Tests assert the secret is still environment-sourced
  (`apps/web/tests/session-lifecycle.test.js:189`).

### Gap

1. **The library's production guard cannot fire.** Better Auth throws
   *"You are using the default secret…"* only when `isDefaultSecret` is true
   (`apps/web/node_modules/better-auth/dist/context/create-context.mjs:42`) — i.e. when **no**
   secret was supplied and the built-in default is in play. Here a *non-default* literal is
   always passed (`auth.better.ts:42`), so the guard is bypassed and the app starts happily on
   a **publicly known** secret. It only checks length/entropy as a warning (`:44-45`).
2. **Nothing in the deployment supplies the real secret.** `grep BETTER_AUTH` across
   `docker-compose.yml`, `docker-compose.prod-ports.yml`, `docker-compose.prod-edge.yml`,
   `apps/web/Dockerfile` and `.env.example` returns **no match**; the web service env block is
   `docker-compose.yml:148-170`. So the compose path — the documented production path
   (`docs/operations/DEPLOYMENT_RUNBOOK.md` §3b) — runs the placeholder.
3. `.env.example` documents Supabase variables and the whole Go envelope, but names
   `BETTER_AUTH_SECRET` / `BETTER_AUTH_URL` nowhere, so an operator following it cannot
   discover the requirement.

**Blast radius:** the secret signs/encrypts session material and the cookie cache
(`dist/cookies/index.mjs:304-305,322-323,334-335` require it). A known secret plus a known
user identifier is a session-forgery primitive.

**Fix (one line):** add
`BETTER_AUTH_SECRET: ${BETTER_AUTH_SECRET:?set BETTER_AUTH_SECRET (>=32 chars) in .env}` and
`BETTER_AUTH_URL: ${SIGAP_WEB_ORIGIN:?}` to the `web` service in `docker-compose.yml`, and
document both in `.env.example`.

---

## 6. DB backup / restore of the identity store — **GAP (verification)**

### Current mechanism

`docs/operations/BACKUP_RESTORE.md` is a closed, production-proven control (AUDIT-701,
16/16 PASS on 2026-09-07, RPO ≤ 24 h, RTO ≤ 2 h):

- `pg_dump --format=custom` of the whole database (`BACKUP_RESTORE.md:6`), nightly at
  03:00 UTC via `sigap-postgres-backup.timer` (`:3`, `:11`).
- Local artifact validated with `pg_restore --list` (`:2`, §7-8), SHA-256 checksum sidecar
  (`:4`, §8), off-host upload to Cloudflare R2 with TLS + SSE (§2, §5).
- Retention 7 days over a narrow namespace (`:4`, §4).
- Destructive-restore refusal for URLs containing `prod` without `--allow-destructive`
  (§13), and a monthly drill (§12).

**Coverage:** because the dump is database-wide, the `auth` schema *is* included once it
exists (`auth.user`, `auth.session`, `auth.account`, `auth.verification`, `auth.jwks` —
`design/better-auth-schema.md:17-19`). There is no schema exclusion anywhere in the scripts
or the runbook. Core coverage is therefore **READY**.

### Gap

Nothing **verifies** the identity store survived a restore. Every critical-table check
enumerates `public` tables only:

- Remote drill: `SELECT count(*) FROM schema_migrations; … FROM facilities; … service_units;
  … appointments; … audit_events;` (§7b step G).
- Disaster-recovery verification: the same list (§10 step 3).
- The closure evidence records "6/6 tables match" (§6a, §6b) — all `public`.

A restore that silently dropped or emptied the `auth` schema would pass every existing
assertion while every user, session and signing key was gone: logins fail (or, worse with
§4's permanent key, the JWKS is empty and Go rejects every token). The drill also does not
assert the *pre-backup* `auth` schema exists — i.e. it cannot detect that the deploy tree
is backing up nothing but domain data either.

**Fix (one line):** add `auth."user"` / `auth.session` / `auth.jwks` counts to the drill's
comparison query list (§7b step G) and to §10 step 3.

---

## 7. Separate `auth`-schema DB privileges — **GAP**

### Current mechanism

Isolation today is **namespacing only**:

- `database.schemaName: "auth"` (`apps/web/src/lib/server/auth.better.ts:47`), honoured by
  the runtime Kysely dialect and the CLI (`design/better-auth-schema.md:14-16`).
- `apps/web/src/lib/server/db.ts:4-8` states the intent: Better Auth's tables are
  schema-qualified so they "never collide with the `public` domain tables or the
  `packages/db` migration ledger".
- The connection URL is the *same* one the Go API uses:
  `process.env.SIGAP_DATABASE_URL ?? process.env.DATABASE_URL` (`db.ts:25-36`), and in
  compose that string is built from `POSTGRES_USER` — the same `sigap` role for api and web
  (`docker-compose.yml:111`).
- The schema is not created by any tracked migration: `packages/db/migrations/` ends at
  `0014_patient_account.sql` and contains no `auth` DDL; the repo contains **no**
  `CREATE SCHEMA`/`GRANT` and no production role definition — the only role creation
  anywhere is the dev bootstrap `CREATE USER sigap WITH PASSWORD …`
  (`docs/DEV_SETUP.md:64-65`), and the only `CREATE SCHEMA` statements are test fixtures
  (e.g. `apps/api/internal/auth/rbac_resolver_test.go:46`, `handler/facility_scope_test.go:45`).
  The real `auth` schema arrives from a manual `npx auth@latest migrate`
  (`design/better-auth-schema.md:27-52`).

`design/better-auth-schema.md:21-22` states the requirement — *"The Postgres role Better Auth
uses must be able to `CREATE SCHEMA`/tables, or the `auth` schema must be pre-created and
`GRANT`ed before migrating"* — i.e. the plan expects privilege scoping, and the repo never
implements it.

### Gap

**No least-privilege role is defined.** In the compose topology the web tier — the only
component that ever handles untrusted browser input directly — authenticates as the database
**owner-level `sigap` role**, which owns `public` (all patient/appointment/audit tables) and
the `schema_migrations` ledger. A defect in the identity tier (or in `pg`/Kysely) therefore
reaches the whole clinical dataset, not just `auth.*`; and the reverse — a web-tier bug
writing `public` — is not blocked by any database-level control. The only guard is that the
application code keeps `schemaName: "auth"` correct.

**Fix (one line):** create a dedicated login role (e.g. `sigap_auth`) granted
`USAGE, CREATE` on schema `auth` and nothing on `public`, and point the web service's
`SIGAP_DATABASE_URL` at it.

---

## 8. Rate limiting — **GAP (durability)**

### Current mechanism — three independent layers

| Layer | Scope | Window / max | Storage | Evidence |
|---|---|---|---|---|
| Better Auth native | its own `/api/auth/*` endpoints via `auth.handler` | 60 s / **20** | `"memory"` | `apps/web/src/lib/server/auth.better.ts:92-97` |
| Web form limiter | SvelteKit server actions (`signInEmail`/`signUpEmail` bypass the native limiter) | 60 s / **10**, key `login:<ip>` / `register:<ip>` | in-process `Map` | `apps/web/src/lib/server/rate-limit.ts:1-7,18-19,38-47` |
| Go API limiter | public booking / walk-in / check-in / status / cancel | fixed window, per key | in-process `Map` | `apps/api/internal/limiter/limiter.go:1-60`; wired in `apps/api/internal/handler/booking.go:33-48,483-486` |

The web layers are hooked where they must be: `allowLogin(event.getClientAddress())`
(`apps/web/src/routes/auth/login/+page.server.ts:4,22-24`) and `allowRegister(...)`
(`apps/web/src/routes/auth/register/+page.server.ts:4,17-19`), both returning
`fail(429, …)`.

Note the client-IP input for keys: `getClientAddress()` on adapter-node derives from the
forwarded-for chain, so the hop count in §1 (`SIGAP_TRUSTED_PROXIES`, default 2 in compose)
is load-bearing for these limits.

### Gap

Every layer is **per-process, in-memory**, and the code says so deliberately
(`rate-limit.ts:9-12`, `limiter.go:13-16`). Consequences for a production deployment:

- N replicas ⇒ the effective limit is N × the configured value; a client can also shift
  buckets by reconnecting to a different replica.
- Any restart (deploy, crash, container recycle) resets all windows to zero, so a burst can
  be replayed immediately after a deploy.
- `storage: "memory"` was chosen explicitly over the database-backed store, so there is no
  `auth.rateLimit` table to persist against either.
- The web limiter has no cleanup for its `Map` beyond opportunistic filtering per key
  (`rate-limit.ts:23-32`); at scale it grows with distinct client IPs.

**Fix (one line):** move the shared buckets to a single-store limiter (Redis or the
`auth`-schema rate-limit storage) before running more than one web/API replica; until then,
document the single-replica constraint as a deployment invariant.

---

## 9. Audit logs for sign-in / sign-out — **GAP**

### Current mechanism

The Go API has a real, hash-chained audit service:
`apps/api/internal/audit/service.go:16-25` (append-only `Service`),
`:50-54` (`LogEvent`, best-effort, never breaks the request), `:97-100`
(`INSERT INTO audit_events (occurred_at, actor_type, action, resource_type, resource_id,
actor_user_id, facility_id, request_id, ip_hash, user_agent_hash, metadata, previous_hash,
event_hash)`), `:158-168` (`previousHash` chaining).

It is invoked **only by Go handlers**: `handler/admin.go:756-770`, `:1179-1194`,
`:1638-1646`; `handler/booking.go:1033-1046`; `handler/notifications.go:647-668`;
`handler/org_overview.go:67-90`. Severity is handled correctly there — metadata is sanitized
and PHI-free (`cross_facility_test.go:297-343` asserts actor + scope with no PHI).

### Gap

**Better Auth emits no audit event that this table can see.** Evidence:

- `apps/web/src/lib/server/auth.better.ts` has no `databaseHooks` (the option Better Auth
  exposes for exactly this) — the configured keys are exactly those listed in §3.
- No file under `apps/web/src` references `audit_events` or any audit sink (grep: the only
  matches are unrelated — a UI-only `visual-audit.test.ts` and `AUDIT-xxxx` ticket
  references in comments).
- The identity tables Better Auth writes are `auth.user`, `auth.session`, `auth.account`,
  `auth.verification`, `auth.jwks` (`design/better-auth-schema.md:17-19`) — **no audit
  table**, and `auth.rateLimit` exists only with database storage, which §8 shows is not set.

So sign-in, sign-out, failed sign-in, registration and password change leave **no durable
who/when/from-where record** beyond the mutable `auth.session` row (which the user's own
sign-out deletes). `audit_events.actor_user_id` is populated only for requests that go on to
hit a Go handler, so the *authentication* event — the one an auditor asks for first — is
absent. Note also that the Go audit `AppUserID` is resolved from `app_users.subject`
(backfilled by `apps/web/scripts/backfill-auth-subjects.mjs`), so the mapping needed to
correlate a Better Auth `sub` with the audit actor **already exists**.

**Fix (one line):** add a Better Auth `databaseHooks: { session: { create/delete } }` (or a
`svelteKitHandler`-adjacent hook) that appends `auth.sign_in` / `auth.sign_out` to the audit
sink using the same `app_users.subject` mapping the RBAC resolver already uses.

---

## 10. Production config assertions — **GAP**

### What is asserted today

`scripts/ops/preflight-production-env.sh` (read-only, exit 0 = safe) asserts the **Go/API**
envelope only:

- `SIGAP_ENV` set, non-empty, and not any whitespace/case variant of `local` — it trims and
  lowercases (`:22-32`) to stay faithful to the Go guard's `TrimSpace` + `EqualFold`
  (`apps/api/internal/config/envguard.go:70-71`).
- `SIGAP_DEPLOY_DIR` resolves to a directory containing `docker-compose.yml` (`:34-46`).
- `SIGAP_AUTH_MODE` must be exactly `jwt`; `dev`/`disabled`/unknown/unset all fail
  (`:48-67`).
- The four dev-only flags must be absent/false: `SIGAP_DEV_IDENTITY`,
  `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR`, `SIGAP_ENGINE_FALLBACK`
  (`:69-81`), matching the Go registry at `envguard.go:22-57`.

In-process fail-fast complements it: `GuardDevCapabilities` (`envguard.go:69-96`, called from
`cmd/server/main.go`), `GuardDevNetworkBinding` (`envguard.go:111-137`), `GuardTLS`
(§1), and web-side gates `localE2eActorHeader` / dev-identity refusal
(`apps/web/src/lib/server/auth.ts:44-133`, `SIGAP_ENV` compared case-insensitively at
`:121-123`).

### Gap

**No auth variable is asserted anywhere.** The script's `for v in …` list (`:69`) names no
`BETTER_AUTH_*`; `.env.example` names none; the runbook's web env matrix
(`docs/operations/DEPLOYMENT_RUNBOOK.md` §5.2) predates Better Auth and lists
`PUBLIC_SUPABASE_*`, `SIGAP_API_INTERNAL`, `SIGAP_TLS_TERMINATED`, `ORIGIN` — a grep for
`BETTER_AUTH` in that file returns nothing. There is likewise no assertion that
`SIGAP_AUTH_ISSUER` matches the **hardcoded** issuer, or that the JWKS URL resolves to the
public origin.

### Required production assertion set (add to the preflight script)

**MUST be set** (and non-empty):

| Variable | Consumer | Required value |
|---|---|---|
| `BETTER_AUTH_SECRET` | web (`auth.better.ts:42`) | real secret, ≥ 32 chars, high entropy (`dist/context/create-context.mjs:44-45`) — never the placeholder |
| `BETTER_AUTH_URL` | web (`auth.better.ts:43`) | the **exact public `https` origin** (also drives `Secure` cookies §2 and the CSRF allow-list §3) |
| `SIGAP_DATABASE_URL` | web (`db.ts:25`) | reachable Postgres URL for the `auth` schema |
| `SIGAP_WEB_ORIGIN` / `ORIGIN` | api CORS (`cmd/server/main.go:37`) / web (`docker-compose.yml:153`) | the real public origin, `https://…` |
| `SIGAP_AUTH_MODE` | api (`config.go:56-59`) | `jwt` (already asserted) |
| `SIGAP_AUTH_ISSUER` | api (`config.go:79-83`) | **exactly** `http://127.0.0.1:5173` — the literal hardcoded at `auth.better.ts:104`; any other value rejects every token |
| `SIGAP_AUTH_AUDIENCE` | api (`config.go:84-87`) | **exactly** `authenticated` (`auth.better.ts:105`) |
| `SIGAP_AUTH_JWKS_URL` | api (`jwt_provider.go:328-330`) | `${BETTER_AUTH_URL}/api/auth/jwks` — optional per `Validate()`, fatal in practice |
| `SIGAP_TLS_TERMINATED` | api (`envguard.go:155-161`) | `true` (already documented; make it asserted) |
| `SIGAP_TRUSTED_PROXIES` | api (`proxy.go:47-51`) | `2` for the edge + SvelteKit chain (compose default) |

**MUST be absent or false:** `SIGAP_DEV_IDENTITY`, `SIGAP_AUTH_MODE=dev`,
`SIGAP_ENGINE_FALLBACK=dev`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR`
(already asserted, `:69-81`), plus `PUBLIC_SUPABASE_SERVICE_ROLE_KEY` and any service-role
key (runbook §5.2), and **`BETTER_AUTH_TRUSTED_ORIGINS` unless it is deliberately reviewed**
— it silently widens the §3 allow-list (`dist/context/helpers.mjs:83`).

**Also assert the migration prerequisite:** the `auth` schema exists with its five tables
before the web container is allowed to serve traffic (`npx auth@latest migrate` is a manual
step: `design/better-auth-schema.md:27-52`), and `GET ${BETTER_AUTH_URL}/api/auth/jwks`
returns a `keys` array with one `ES256`/`P-256` entry (`auth.better.ts:100-102`;
Go-side decode of that entry: `jwt_provider.go:135-176`, `:179-193`).

**Fix (one line):** extend `preflight-production-env.sh` with the MUST-set/MUST-be-absent
lists above (and the `https://` prefix check from §1).

---

## Top gaps and one-line fixes

| Priority | Gap | One-line fix |
|---|---|---|
| **1** | **§5 — no real `BETTER_AUTH_SECRET` reaches the web tier**; a public placeholder is a silent, guard-bypassing fallback (`auth.better.ts:42`, `dist/context/create-context.mjs:42`) | Pass `BETTER_AUTH_SECRET` (≥32 chars) + `BETTER_AUTH_URL` into the `web` service in `docker-compose.yml` and document both in `.env.example`. |
| **2** | **§9 — sign-in/sign-out leave no audit trail**; `audit_events` is written only by Go handlers | Add a Better Auth `databaseHooks` session create/delete hook that appends `auth.sign_in`/`auth.sign_out` to the audit sink via the existing `app_users.subject` mapping. |
| **3** | **§3 — `trustedOrigins` is unset and `BETTER_AUTH_URL` is unwired**, so the CSRF allow-list is one accidental fallback origin (`dist/context/helpers.mjs:71-77`) | Declare `trustedOrigins` explicitly from `BETTER_AUTH_URL` and pass that variable into the web service. |

Runners-up, same class: §7 (no least-privilege role — web tier holds domain-table
privileges), §4 (permanent ES256 key, no rotation), §6 (restore drill never checks
`auth.*`), §8 (all limiters in-memory ⇒ per-replica), §10 (no auth variable asserted).

---

## OPERATOR VERIFICATION REQUIRED (cannot be read from the repository)

1. The real `BETTER_AUTH_SECRET` exists in the deploy `.env` and is **not** the placeholder
   (assert per §10; never print it).
2. `auth` schema migrated in production (`\dt auth.*` → `user`, `session`, `account`,
   `verification`, `jwks`) and `GET https://<host>/api/auth/jwks` returns an `ES256` key
   with `crv: "P-256"`.
3. Edge terminates TLS and forwards `X-Forwarded-Proto: https`; `ORIGIN` and
   `BETTER_AUTH_URL` equal the public `https` origin.
4. Whatever Postgres role the web service uses, and whether it is distinct from the Go
   API's role (§7).
5. Replica count of `web` and `api` (§8) — the three rate limiters assume a single process.

## Changelog

- 2026-11-11 — initial assessment at `60de7d6`. Read-only: no deployment, no migration, no
  gate, no production contact. Ten items assessed: 1 READY / 9 GAP.
