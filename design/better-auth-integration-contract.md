# Better Auth Integration Contract (SIGAP self-hosted identity)

**Status:** Contract / research — pins the real Better Auth API (v1.7.x) and the
unchanged Go JWT trust boundary. **No code changes proposed here.**

**Date:** 2026-11-11

---

## 0. Executive summary — the ONE recommended Go-trust bridge

**Better Auth supports ES256 + JWKS natively — YES.**

Better Auth's `jwt` plugin is *already* asymmetric. It generates a keypair, signs its
external JWT with that keypair, and publishes the public key at a JWKS endpoint. It does
**not** sign its cross-service tokens with HS256 (HS256 appears only as the *internal*
`session_data` cookie-cache default, at `session.cookieCache.strategy: "jwt"`).

**Recommended bridge (no Go change):**

1. Configure Better Auth to sign **ES256** (ECDSA P-256) — the default is **EdDSA /
   Ed25519 (kty `OKP`)**, which the existing Go `parseJWK` does **not** accept (it only
   handles `RSA`/`EC`). Pinning `alg: "ES256"` makes the emitted key a normal `EC`/`P-256`
   JWK, which the existing Go `JWTProvider` already parses and which is already in
   `validJWTAlgs()`.
2. Pin `issuer` and `audience` in the plugin so they mirror `SIGAP_AUTH_ISSUER` /
   `SIGAP_AUTH_AUDIENCE`.
3. Point `SIGAP_AUTH_JWKS_URL` at `<sveltekit-base>/api/auth/jwks`.

The Go side retains the exact allowlist it has today (`alg ∈ {RS256,384,512, ES256,384,512}`;
`iss`; `aud`; `exp/nbf/iat`; `kid`-based JWKS lookup; `alg=none` rejected). This is
bridge **(b)** — *"a narrow internal access-token minted by Better Auth and verified by Go"*
— realized natively with asymmetric keys, which the reverted HS256 bridge was always a
stopgap for (see §5.4).

The HS256 shared-secret bridge **(a)** is **not needed** and stays reverted.

---

## 1. Trust boundary (unchanged)

`apps/api/internal/auth/jwt_provider.go` + `config.go` define the boundary that Better
Auth must satisfy. Read it as the *target contract*, not something to modify.

- `AuthConfig` env: `SIGAP_AUTH_MODE` (`dev|jwt|disabled`), `SIGAP_AUTH_ISSUER`,
  `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`.
- `validJWTAlgs()` = `RS256, RS384, RS512, ES256, ES384, ES512` (no HS*, no EdDSA).
- `keyFunc` rejects `alg=none`/`alg=""`, requires a `kid` header when JWKS is set, and
  looks the key up from the JWKS cache by `kid`.
- `parseJWK` accepts `kty` `RSA` (n/e) and `kty` `EC` (x/y/crv ∈ P-256/P-384/P-521); it
  **skips** any other kty (including `OKP`/Ed25519) and skips keys with `use` not empty
  and not `sig`.
- `Authenticate` verifies signature + `iss` + `aud` + registered claims, then resolves
  permissions **server-side** from DB RBAC (`Resolver`) keyed on the token `sub`
  (`app_users.subject`). Token `permissions` claim is ignored (AUDIT-101).

⇒ Every requirement on Better Auth below is derived from this boundary.

---

## 2. Package to install

| Package | Version line | Notes |
|---|---|---|
| `better-auth` | `better-auth@^1.7.20` | Latest at check time (registry `1.7.20`, tarball `better-auth-1.7.20.tgz`). Subpaths: `better-auth/svelte-kit`, `better-auth/svelte`, `better-auth/plugins`, `better-auth/client`, `better-auth/client/plugins`, `better-auth/db/migration`. |
| `pg` | `pg@^8` | Required only if wiring a `pg.Pool` (recommended). Not present in `apps/web/package.json` today. |
| `@types/pg` | `@types/pg@^8` | devDependency. |

The SvelteKit server integration is a **subpath** of the `better-auth` package
(`better-auth/svelte-kit`). The client is `better-auth/svelte`. There is **no** published
`@better-auth/svelte` package (npm returns 404 for `@better-auth/svelte`); the client
integration ships as the `./svelte` export of the main package.

> ⚠️ `apps/web/package.json` pins `@sveltejs/kit@^2.5.0`. The `sveltekitCookies` plugin
> requires `getRequestEvent` from `$app/server`, which only exists in **SvelteKit ≥ 2.20.0**.
> Pin `@sveltejs/kit` to `^2.20.0` (or better, latest 2.x) before wiring `sveltekitCookies`.

---

## 3. SvelteKit integration (`@sveltejs/kit@^2.5.0` + Svelte 5)

Source: <https://better-auth.com/docs/integrations/svelte-kit>; verified against
`packages/better-auth/src/integrations/svelte-kit.ts` (`svelteKitHandler`,
`toSvelteKitHandler`, `sveltekitCookies`, `isAuthPath`).

### 3.1 Server config (`betterAuth()`)

```ts
// apps/web/src/lib/auth.ts
import { betterAuth } from "better-auth";
import { jwt } from "better-auth/plugins";
import { sveltekitCookies } from "better-auth/svelte-kit";
import { getRequestEvent } from "$app/server"; // SvelteKit >= 2.20

export const auth = betterAuth({
  baseURL: process.env.BETTER_AUTH_BASE_URL, // app origin; required for non-auth.api
  secret: process.env.BETTER_AUTH_SECRET,    // or AUTO from BETTER_AUTH_SECRET/AUTH_SECRET
  database: /* see §4 — pg.Pool or { dialect, type:"postgres", schemaName:"auth" } */,
  emailAndPassword: { enabled: true /* + §6 */ },
  plugins: [
    jwt({
      jwks: { keyPairConfig: { alg: "ES256" } },     // CRITICAL: not the EdDSA default
      jwt: {
        issuer: process.env.SIGAP_AUTH_ISSUER,       // mirror Go
        audience: process.env.SIGAP_AUTH_AUDIENCE,   // mirror Go
        // expirationTime: "15m" (default) — the real key name (NOT "expiresIn")
      },
    }),
    sveltekitCookies(getRequestEvent), // MUST be the LAST plugin
  ],
});
```

### 3.2 Handler wiring (`hooks.server.ts`)

```ts
// apps/web/src/hooks.server.ts
import { auth } from "$lib/auth";
import { svelteKitHandler } from "better-auth/svelte-kit";
import { building } from "$app/environment"; // SvelteKit 2 (NOT "$app/env" = SK 3)

export async function handle({ event, resolve }) {
  return svelteKitHandler({ auth, event, resolve, building });
}
```

- `svelteKitHandler` inspects `options.basePath || "/api/auth"` (and `options.baseURL`
  for origin match) and forwards matching paths to `auth.handler`; other requests fall
  through to `resolve(event)`.
- `toSvelteKitHandler(auth)` is the lower-level `(event) => auth.handler(event.request)`
  helper if you want non-handler routing.
- No separate `Auth` class exists for SvelteKit (the `Auth` wrapper seen in Next.js is
  not exported here); `svelteKitHandler` **is** the mechanism.

### 3.3 `event.locals` is NOT populated for you

`event.locals.session` / `event.locals.user` are **not** set by `svelteKitHandler`. Populate
manually in `handle` (needed for `+layout.server.ts`, `+server.ts` proxies, actions):

```ts
export async function handle({ event, resolve }) {
  const session = await auth.api.getSession({
    headers: event.request.headers,
  });
  if (session) {
    event.locals.session = session.session;
    event.locals.user = session.user;
  }
  return svelteKitHandler({ auth, event, resolve, building });
}
```

`event.locals` must be typed in `apps/web/src/app.d.ts` (`App.Locals`): `session:
Session | undefined; user: User | undefined` (types imported from `better-auth`).

### 3.4 `sveltekitCookies` (server-action cookies)

`signOut` / mutations called from `+page.server.ts` / actions need cookies written back;
add `sveltekitCookies(getRequestEvent)` as the **last** plugin. It reads Better Auth
`set-cookie` response headers and writes them through `event.cookies.set`.

### 3.5 Client

```ts
// apps/web/src/lib/client.ts
import { createAuthClient } from "better-auth/svelte"; // NOT "better-auth/client" for Svelte

export const authClient = createAuthClient({ /* baseURL optional */ });
```

Reactive session: `const session = authClient.useSession();` (`nano-store` backed).

---

## 4. PostgreSQL adapter + isolated `auth.` schema

Source: <https://better-auth.com/docs/adapters/postgresql>,
<https://better-auth.com/docs/concepts/database>.

### 4.1 Adapter options (precise)

Three accepted shapes:

1. **`pg.Pool` directly** (keeps `public`/`search_path`):
   ```ts
   database: new Pool({ connectionString: import.meta.env /* DATABASE_URL */ })
   ```
2. **Kysely dialect + schemaName** (recommended for `auth.` isolation):
   ```ts
   import { PostgresDialect } from "kysely";
   database: {
     dialect: new PostgresDialect({ pool: new Pool({ connectionString }) }),
     type: "postgres",
     schemaName: "auth",
   }
   ```
3. **Kysely instance + schemaName**:
   ```ts
   database: { db, type: "postgres", schemaName: "auth" }
   ```

### 4.2 Isolated `auth.` schema — CRITICAL

Do **not** let Better Auth write into SIGAP's `public` schema (it would collide with the
`packages/db` domain tables). Use the `schemaName` option:

- `database.schemaName: "auth"` is applied to **runtime queries and CLI migrations**.
- `npx auth@latest migrate` creates it when needed (`create schema if not exists "auth";`)
  and emits schema-qualified names (`"auth"."user"` …), ignoring same-named tables in other
  schemas.
- `npx auth@latest generate` starts the SQL with `create schema if not exists "auth";`.
- The Postgres role Better Auth uses must be able to `CREATE` the schema/tables, or you
  pre-create the schema and `GRANT` to that role before migrating.
- `search_path` is the alternative (Pool `options: "-c search_path=auth"` or connection-
  string `?options=-c%20search_path%3Dauth`), but the CLI then expects the schema to
  already exist.

### 4.3 Does Better Auth auto-migrate?

**No runtime auto-migration by default.** Migration is explicit:

- CLI: `npx auth@latest migrate` (built-in Kysely adapter only — Postgres ✅) — prompts
  and applies missing tables/columns. `npx auth@latest generate` emits an SQL file.
- Programmatic: `getMigrations(auth.options)` → `{ toBeCreated, toBeAdded, runMigrations }`
  from `better-auth/db/migration` (Kysely adapter only; **not** Prisma/Drizzle).
- Runtime **schema validation** (on by default) only *reports* missing tables/columns via
  the logger and fails requests; it **does not create** them. Disable with
  `advanced.database.validateSchema: false`.

⇒ Contract: run `npx auth@latest migrate` against the SIGAP DB (`:5433`) as an explicit
step; do not rely on the app booting the tables.

### 4.4 Exact core schema

Authoritative columns from `packages/core/src/db/get-tables.ts` (main). Every table has an
`id` (text PK, generated by Better Auth unless `advanced.database.generateId` is changed).

**`user`** (default modelName `user`)
| column | type | notes |
|---|---|---|
| id | text PK | auto-generated |
| name | text | required |
| email | text | required, unique |
| emailVerified | boolean | required, default `false`, `input:false` |
| image | text | optional |
| createdAt | timestamptz | required, default now |
| updatedAt | timestamptz | required, on-update now |

**`session`** (default `session`)
| column | type | notes |
|---|---|---|
| id | text PK | |
| expiresAt | timestamptz | required |
| token | text | required, unique |
| createdAt | timestamptz | required |
| updatedAt | timestamptz | required |
| ipAddress | text | optional |
| userAgent | text | optional |
| userId | text | required, FK→`user.id` ON DELETE CASCADE, indexed |

**`account`** (default `account`; one row per auth method)
| column | type | notes |
|---|---|---|
| id | text PK | local account-row id |
| accountId | text | required (provider-side identity) |
| providerId | text | required (`"credential"` for email/password) |
| userId | text | required, FK→`user.id` CASCADE, indexed |
| accessToken | text | optional, `returned:false` |
| refreshToken | text | optional, `returned:false` |
| idToken | text | optional, `returned:false` |
| accessTokenExpiresAt | timestamptz | optional, `returned:false` |
| refreshTokenExpiresAt | timestamptz | optional, `returned:false` |
| scope | text | optional |
| password | text | optional, `returned:false` |
| createdAt / updatedAt | timestamptz | required |

Credential accounts use `providerId="credential"` and `accountId = user.id`.

**`verification`** (default `verification`): `id` PK, `identifier` (required, indexed),
`value` (required), `expiresAt` (required), `createdAt` (required), `updatedAt` (required).

Plugin tables: `jwt` plugin adds **`jwks`** (stores the keypair; private key AES-256-GCM
encrypted by default). If `rateLimit.storage === "database"`, Better Auth adds
**`rateLimit`** (`key`, `count`, `lastRequest`).

---

## 5. JWT for the Go API

Source: <https://better-auth.com/docs/plugins/jwt> (and `./plugins/jwt` source).

### 5.1 Native algorithm / JWKS — the answer

- **Not HS256.** The JWT plugin mints tokens with a locally-generated **asymmetric
  keypair**, and serves the public key(s) from a JWKS endpoint.
- **Default alg = EdDSA / Ed25519** (`kty: OKP`, `crv: Ed25519`). ❌ incompatible with the
  Go `parseJWK` (no `OKP` support) — this is why the contract must override it.
- **ES256 (ECDSA P-256) is a first-class option** → `kty: EC`, `crv: P-256`, which the Go
  boundary already accepts and which is already in `validJWTAlgs()`.
- JWKS endpoint: default path `/jwks` under `basePath` → effectively
  `/api/auth/jwks`; customizable via `jwt({ jwks: { jwksPath } })`. The token header
  carries a `kid` that matches the JWKS key `kid`.

### 5.2 Exact plugin config keys

`better-auth` `plugins: [ jwt({...}) ]`.

| Key path | Meaning | Default |
|---|---|---|
| `jwks.keyPairConfig.alg` | signing algorithm: `"EdDSA"`, `"ES256"`, `"ES512"`, `"RSA256"`, `"PS256"`, `"ECDH-ES"` | `"EdDSA"` |
| `jwks.keyPairConfig.crv` | curve (EdDSA: `Ed25519`/`Ed448`; ECDH-ES: `P-256`/`P-384`/`P-521`) | `Ed25519` |
| `jwks.keyPairConfig.modulusLength` | RSA256/PS256 modulus | `2048` |
| `jwks.remoteUrl` | external JWKS URL (disables local `/jwks`; requires `keyPairConfig.alg`) | — |
| `jwks.jwksPath` | custom JWKS path suffix (e.g. `"/.well-known/jwks.json"`) | `/jwks` |
| `jwks.disablePrivateKeyEncryption` | store private key in plaintext | `false` (encrypted) |
| `jwks.rotationInterval` | key rotation seconds | `undefined` (off) |
| `jwks.gracePeriod` | keep old key valid seconds after rotation | 30 days |
| `jwt.issuer` | token `iss` | `baseURL` |
| `jwt.audience` | token `aud` | `baseURL` |
| `jwt.expirationTime` | token TTL (string) — **the real key, NOT `expiresIn`** | `"15m"` |
| `jwt.getSubject` | custom `sub` (default = user id) | `session.user.id` |
| `jwt.definePayload` | custom claim builder | whole user object |
| `jwt.sign` | custom sign fn (KMS) | built-in |
| `sessionCookieCache` | sign the `session_data` cookie-cache JWT with plugin keys (requires `session.cookieCache.strategy:"jwt"`) | `false` |
| `disableSettingJwtHeader` | stop the `set-auth-jwt` header on `getSession` | `false` |
| `adapter` | custom JWKS storage (replace DB) | DB-backed |

> Naming note: the assignor's "`expiresIn`" maps to **`jwt.jwt.expirationTime`** (string,
> default 15 min) for the *plugin's* token, and separately to **`session.expiresIn`**
> (seconds, default 604800) for the *session cookie* (top level).

### 5.3 Recommended contract (the bridge to pin)

```ts
jwt({
  jwks: { keyPairConfig: { alg: "ES256" } },          // ECDSA P-256 ⇒ Go parseJWK OK
  jwt: {
    issuer:  process.env.SIGAP_AUTH_ISSUER,           // e.g. http://localhost:5173
    audience: process.env.SIGAP_AUTH_AUDIENCE,        // e.g. sigap-webapp
    // expirationTime: "15m" (default)
  },
})
```

Go env (unchanged shape, new values):

```
SIGAP_AUTH_MODE=jwt
SIGAP_AUTH_ISSUER=<the SvelteKit app origin / chosen issuer>
SIGAP_AUTH_AUDIENCE=<the audience chosen above, must match jwt.jwt.audience>
SIGAP_AUTH_JWKS_URL=<sveltekit origin>/api/auth/jwks
```

Subject backfill (same as the Supabase step): set `app_users.subject = <better-auth user.id>`
so `rbac_resolver.go` (`WHERE u.subject = $1`) resolves the actor. The Better Auth JWT `sub`
defaults to the `user.id` (a generated string id), so this is the `subject` value.

Contract invariants verified end-to-end by Go (no code change):
`alg` ∈ allowlist; `kid` present → JWKS `EC`/`P-256` key; `iss`/`aud` exact match;
`exp/nbf/iat` valid; `sub` → `Resolver` → `Actor`; token `permissions` claim ignored.

### 5.4 Fallback bridges (both superseded by §5.3)

**(a) Re-introduced Go HS256 shared-secret path — NOT needed, stays reverted.**
The reverted commit `d9e9dc8` (reverted by `3cd2125`) added `SIGAP_AUTH_JWT_SECRET` and
made it safe with a **per-key-source algorithm allowlist**:

- `HS256/384/512` were accepted **only** when a shared secret was configured
  (`SIGAP_AUTH_JWT_SECRET`); `RS*`/`ES*` were accepted **only** when JWKS was configured.
  This split closes the classic **algorithm-confusion** attack (an `RS256`/`ES256` token
  re-verified as `HS256` against the public key treated as an HMAC secret).
- Exactly **one** key source was allowed in `jwt` mode (secret XOR JWKS, never both).
- Shared-secret mode was gated to `SIGAP_ENV=local`; `iss`/`aud`/`exp` checks unchanged;
  the secret was never logged.
- Reverted because local Supabase already issues ES256+JWKS, making the JWKS-only provider
  the single verification path (see `design/jwt-env-config.md`).

Because Better Auth natively signs ES256 and publishes a JWKS, this path is unnecessary.

**(b) Narrow internal access-token minted by Better Auth + verified by Go** — this *is*
the recommended §5.3 path, upgraded to asymmetric keys. It is not a "second mode" to
build; it is the default JWT plugin behavior once pinned to ES256.

---

## 6. Email/password, session, refresh, logout, rate limit, verification, reset

Sources: <https://better-auth.com/docs/authentication/email-password>,
<https://better-auth.com/docs/concepts/rate-limit>, `/docs/reference/options`.

### 6.1 Exact config keys

**`emailAndPassword`** (top-level)
| key | default | notes |
|---|---|---|
| `enabled` | `false` | must be `true` |
| `disableSignUp` | `false` | |
| `requireEmailVerification` | `false` | blocks sign-in until verified (email/password only) |
| `minPasswordLength` | `8` | |
| `maxPasswordLength` | `128` | |
| `autoSignIn` | — | sign in right after sign up |
| `sendResetPassword` | — | `async ({ user, url, token }, request)` |
| `onPasswordReset` | — | post-reset callback |
| `revokeSessionsOnPasswordReset` | `false` | |
| `resetPasswordTokenExpiresIn` | `3600` | seconds |
| `onExistingUserSignUp` | — | enumeration-protection callback |
| `customSyntheticUser` | — | for plugins adding user fields |
| `password.hash` / `password.verify` | — | optional custom hashing |

**`emailVerification`** (top-level)
| key | default | notes |
|---|---|---|
| `sendVerificationEmail` | — | `async ({ user, url, token }, request)` |
| `sendOnSignUp` | `undefined` | `true` always sends, `false` never, `undefined` follows `requireEmailVerification` |
| `sendOnSignIn` | `false` | re-send on sign-in when unverified |
| `autoSignInAfterVerification` | — | |
| `expiresIn` | `3600` | verification-token TTL, seconds |

**`session`** (top-level)
| key | default | notes |
|---|---|---|
| `expiresIn` | `604800` (7 d) | cookie/session TTL, seconds |
| `updateAge` | `86400` (1 d) | refresh cadence; rolls `expiresAt` |
| `freshAge` | `86400` | "fresh session" threshold (`0` disables) |
| `disableSessionRefresh` | `false` | stop rolling refresh |
| `cookieCache.enabled` / `.maxAge` | `false` / 300 | verified-session cookie cache; `strategy:"jwt"` makes it HS256 (per `secret`) |
| `storeSessionInDatabase` / `preserveSessionInDatabase` | `false` / `false` | secondary-storage interplay |

**`rateLimit`** (top-level): default production `window: 10s`, `max: 100`; **disabled in
dev** unless `enabled: true`. Keys: `enabled`, `window`, `max`, `customRules` (per-path
override/fn, e.g. built-in `/sign-in/email` = 3/10s), `storage` (`"memory"` default,
`"database"` adds the `rateLimit` table), `modelName`. IP detection:
`advanced.ipAddress.ipAddressHeaders`, `trustedProxies`, `ipv6Subnet`,
`disableIpTracking`.

### 6.2 Flows (endpoints)

- Sign up: `authClient.signUp.email({ name, email, password })` → `POST /sign-up/email`.
- Sign in: `authClient.signIn.email({ email, password, rememberMe })` → `POST /sign-in/email`.
- Sign out: `authClient.signOut()` → `POST /sign-out`; server-side `auth.api.signOut({ headers })`.
- Refresh: cookie session auto-rolls on `updateAge` (no manual refresh token flow for the
  session cookie; the `jwt` plugin token is re-minted via `authClient.token()` /
  `POST /api/auth/token` / `set-auth-jwt` header).
- Reset: `authClient.requestPasswordReset({ email })` → `POST /request-password-reset`
  (invokes `sendResetPassword`).

### 6.3 Local dev — no SMTP, don't let verification block sign-in

The only thing that turns on a verification gate is `emailAndPassword.requireEmailVerification`
(or `emailVerification.sendOnSignUp: true`). Default (`false`) → **sign-in works with no
SMTP**. For a local run:

```ts
emailAndPassword: {
  enabled: true,
  // do NOT set requireEmailVerification (leave default false)
},
emailVerification: {
  // If you DO flip requireEmailVerification later, a no-op/logging sender prevents
  // the "verify your email" wall — print the link and click it:
  sendVerificationEmail: async ({ user, url }) => {
    console.log(`[local] verify ${user.email}: ${url}`);
  },
},
```

If you keep `requireEmailVerification: false`, **no `sendVerificationEmail`** is needed and
verification is skipped entirely for sign-in.

```ts
// password reset, no SMTP: log the reset URL/token instead of emailing
emailAndPassword: {
  enabled: true,
  sendResetPassword: async ({ user, url }) => {
    console.log(`[local] reset ${user.email}: ${url}`);
  },
},
```

---

## 7. Migration from Supabase (pointers)

Better Auth has a first-class guide:
<https://better-auth.com/docs/guides/supabase-migration-guide>. Key mapping for SIGAP:

- `auth.users.id` (Supabase) → Better Auth `user.id` **as** `app_users.subject`. Use the
  guide's copy script (from `auth.users` → `public.user`) to preserve `sub` stability, or
  accept new ids and backfill `app_users.subject` from the mapping table.
- Supabase's ES256 `authenticated` JWT → replaced by Better Auth's ES256 JWT with
  `jwt.jwt.issuer`/`audience` set to the values in §5.3; `SIGAP_AUTH_AUDIENCE` currently
  `authenticated` would become the chosen audience.
- No RLS/2FA migration path needed for SIGAP's use (RLS lives on the SIGAP side already in
  `packages/db`; Better Auth is only the identity/credential store).
- Sessions are invalidated on migration (expected — staff re-sign-in).

---

## 8. Acceptance checklist (for the follow-up implementation)

- [ ] Install `better-auth@^1.7.20` (+ `pg`, `@types/pg`), bump `@sveltejs/kit` to `^2.20.0`.
- [ ] `auth.ts`: `baseURL`, `secret`, Postgres `{ dialect, type:"postgres", schemaName:"auth" }`,
  `emailAndPassword.enabled`, `jwt({ jwks.keyPairConfig.alg:"ES256", jwt.issuer, jwt.audience })`,
  `sveltekitCookies(getRequestEvent)` last.
- [ ] `hooks.server.ts`: `svelteKitHandler({ auth, event, resolve, building })` + manual
  `event.locals` population; `app.d.ts` `App.Locals` typed.
- [ ] `npx auth@latest migrate` — verify tables land under `"auth"` schema, not `public`.
- [ ] Go env: `SIGAP_AUTH_MODE=jwt`, `ISSUER`/`AUDIENCE` matching `jwt.jwt.*`,
  `JWKS_URL=<origin>/api/auth/jwks`.
- [ ] Backfill `app_users.subject` = Better Auth `user.id`.
- [ ] E2E: sign-in → `authClient.token()` → `Authorization: Bearer` → Go 200 with
  DB-resolved permissions; anon 401; token `permissions` claim ignored (AUDIT-101).
- [ ] Confirm `kid` present and JWKS `kty:"EC"`, `crv:"P-256"` (never `OKP`).