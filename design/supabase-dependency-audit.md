# Supabase Dependency Audit — READ-ONLY Enumeration

Scope: everything in this worktree that touches Supabase today, the exact Go
JWT/env contract, the identity-mapping seam where a Better Auth `sub` must land,
the synthetic citizen subjects + dev/test selectors, and the change checklist
for replacing Supabase with Better Auth as the **single** identity system.

Prepared 2026-11-11 on branch `feat/sigap-selfhosted-auth`. No code was modified.

---

## 1. Every Supabase touchpoint

### 1.1 Web app (`apps/web`)

| # | File:line | Touchpoint |
|---|---|---|
| 1 | `apps/web/src/hooks.server.ts:2` | `import { createSupabaseServerClient } from '$lib/supabase/server'` |
| 2 | `apps/web/src/hooks.server.ts:3` | `import { supabaseConfigured } from '$lib/server/auth'` |
| 3 | `apps/web/src/hooks.server.ts:10-18` | `if (supabaseConfigured()) { createSupabaseServerClient(event); client.auth.getSession(); event.locals.supabase = client; event.locals.session = session; }` — the single web-tier session authority |
| 4 | `apps/web/src/hooks.server.ts:20-22` | `applyPendingHeaders(response)` — cookie/anti-CDN-cache header writes from `@supabase/ssr` |
| 5 | `apps/web/src/app.d.ts:1` | `import type { SupabaseClient } from '@supabase/ssr'` |
| 6 | `apps/web/src/app.d.ts:7-8` | `Locals.supabase: SupabaseClient \| null; Locals.session: AuthSessionLike \| null` |
| 7 | `apps/web/src/lib/supabase/server.ts:1-2` | `import { createServerClient } from '@supabase/ssr'; import SupabaseClient from '@supabase/supabase-js'` |
| 8 | `apps/web/src/lib/supabase/server.ts:22-45` | `createServerClient(PUBLIC_SUPABASE_URL, PUBLIC_SUPABASE_ANON_KEY, { cookies: getAll/setAll })` — the only Supabase **client** construction site |
| 9 | `apps/web/src/lib/server/auth.ts:19-23` | `AuthSessionLike` — "Minimal view of a Supabase session … mirrors the subset of `@supabase/supabase-js` `Session`" |
| 10 | `apps/web/src/lib/server/auth.ts:25-28` | `supabaseConfigured()` = `Boolean(PUBLIC_SUPABASE_URL && PUBLIC_SUPABASE_ANON_KEY)` — the web-tier alarm |
| 11 | `apps/web/src/lib/server/auth.ts:32-41` | `authHeaders(event)` — reads `event.locals.session?.access_token` → `Authorization: Bearer <token>` (the Supabase session → Go JWT bridge) |
| 12 | `apps/web/src/routes/auth/login/+page.server.ts:16-30` | `authConfigured: Boolean(locals.supabase)`; `locals.supabase.auth.signInWithPassword({ email, password })` |
| 13 | `apps/web/src/routes/auth/register/+page.server.ts:6-27` | `authConfigured: Boolean(locals.supabase)`; `locals.supabase.auth.signUp({ email, password })` |
| 14 | `apps/web/src/routes/auth/logout/+page.server.ts:8-9` | `locals.supabase.auth.signOut()` |
| 15 | `apps/web/src/routes/auth/login/+page.svelte:11-12` | frozen-action comment: "the 503 when Supabase is unconfigured" |
| 16 | `apps/web/src/routes/auth/register/+page.svelte` | frozen-action comment mirroring login (no Supabase API, pure `data?.authConfigured`) |
| 17 | `apps/web/src/routes/auth/logout/+page.svelte` | frozen-action comment (no Supabase API; posts to the action) |
| 18 | `apps/web/src/routes/+layout.server.ts:34-41` | `userEmail: locals.session?.user?.email; hasSession: Boolean(locals.session?.access_token)` — shape of `AuthSessionLike` |
| 19 | `apps/web/src/lib/citizen/AuthForms.svelte:26,53` | comment-only references to "a configured Supabase" (no client import — enforced by test) |
| 20 | `apps/web/src/lib/citizen/auth-presentation.test.ts:378-389` | asserts the presentation layer "must not import supabase" (the client-side firewall) |
| 21 | `apps/web/package.json` | declares `@supabase/supabase-js` + `@supabase/ssr` (+ peer Supabase libs) |
| 22 | `apps/web/.env` / `.env.example:41-43` | `PUBLIC_SUPABASE_URL` / `PUBLIC_SUPABASE_ANON_KEY` — the publishable-only values |
| 23 | `apps/web/e2e/admin-facility-mutations.spec.ts:42` | E2E comment routing identity through `X-Sigap-Local-Test-Subject` (Supabase-independent but auth-surface-adjacent) |
| 24 | `apps/web/e2e/smoke.spec.ts:46-59` | asserts the login page is honest about "With Supabase configured … Without it … the form is withheld" |
| 25 | `apps/web/e2e/smoke.spec.ts:60-68` | asserts the `auth-unavailable` notice / withheld form (the Supabase-unconfigured presentation) |

**Negative findings (absence is itself a touchpoint):**

- **No `auth/callback` route exists.** There is no OAuth/sign-in callback handler and no `signInWithOAuth`/`exchangeCodeForSession` anywhere in `apps/web/src`. The only auth surfacing is password sign-in/sign-up/sign-out (items 12–14). A Better Auth OAuth flow will introduce this route for the first time.
- **No client-side Supabase wrapper** (`$lib/supabase/client*` does not exist). Supabase is reached only server-side via `$lib/supabase/server.ts`.
- **Cookie name `sb-*-auth-token` is not hard-coded** in this repo. `@supabase/ssr` derives it from the project ref; `server.ts:29-38` round-trips all cookies through `event.cookies` with no name filter. Replacing Supabase removes this cookie; Better Auth's own cookie must be allowed through the same `getAll/setAll` seam.

### 1.2 Go API (`apps/api`)

| # | File:line | Touchpoint |
|---|---|---|
| 26 | `apps/api/cmd/server/main.go:160` | boots with `authMode := os.Getenv("SIGAP_AUTH_MODE")` (banner only) |
| 27 | `apps/api/cmd/server/main.go:258-276` | `LoadConfigFromEnv()`; `if authCfg.Mode == jwt { NewJWTProviderWithResolver(...) } else { NewProvider(...) }` — the mode gate |
| 28 | `apps/api/cmd/server/main.go:293-320` | `SIGAP_LOCAL_RBAC_TEST_IDENTITY` selector block (arms `LocalRBACTestIdentityProvider`) |
| 29 | `apps/api/internal/auth/config.go:44-50` | `AuthConfig{ Mode, Issuer, Audience, JWKSURL }` |
| 30 | `apps/api/internal/auth/config.go:63-69` | `LoadConfigFromEnv` reads `SIGAP_AUTH_MODE/ISSUER/AUDIENCE/JWKS_URL` |
| 31 | `apps/api/internal/auth/config.go:82-92` | `Validate()` — jwt mode requires Issuer + Audience |
| 32 | `apps/api/internal/auth/jwt_provider.go:334-338` | `validJWTAlgs()` = `RS256/384/512, ES256/384/512` (algorithm allowlist) |
| 33 | `apps/api/internal/auth/jwt_provider.go:316-331` | `keyFunc` — `alg=none` rejected; `kid` required when JWKS configured; key from `jwks.get(kid)` |
| 34 | `apps/api/internal/auth/jwt_provider.go:232-248` | `Authenticate` — `jwt.WithValidMethods`, `WithIssuer`, `WithAudience`; `sigapClaims`; fail-closed on any error |
| 35 | `apps/api/internal/auth/jwt_provider.go:272-295` | `sub = claims.Subject`; `userType = ActorUser` (or `ActorDev` when `claims.Issuer=="sigap-dev"`); resolver `Resolve(ctx, sub)` → actor permissions/grants/AppUserID |
| 36 | `apps/api/internal/auth/jwt_provider.go:299-307` | resolver-less jwt: zero permissions, fail closed |
| 37 | `apps/api/internal/config/envguard.go` + `envguard_test.go` | `GuardDevCapabilities` — rejects dev-only flags outside `SIGAP_ENV=local` (the local-only gate for selectors) |

### 1.3 Seeds / migrations / fixtures that write `subject`

| # | File:line | Touchpoint |
|---|---|---|
| 38 | `packages/db/migrations/0003_identity_rbac.sql:7-24` | `app_users` table (`id,email,display_name,status,created_at,updated_at,deleted_at`) — no `subject` yet |
| 39 | `packages/db/migrations/0008_identity_subject.sql:12-19` | `ALTER TABLE app_users ADD COLUMN subject TEXT` + `uq_app_users_subject_active` (subject→app_user map) |
| 40 | `packages/db/seed/dev.sql:39-48` | seeds `subject` for `local-global-super-admin`, `local-facility-admin`, `local-zero-scope-admin` (d990-d992) |
| 41 | `packages/db/seed/dev.sql:114-118` | seeds `subject` for `e2e-schedule-manager/mixed/reader` (d993-d995) |
| 42 | `packages/db/seed/regional_tenancy.sql:147-157` | seeds `subject` for org/regional/facility actors (c301-c306) and citizen `local-patient-p1` (c307) |
| 43 | `apps/api/cmd/bootstrap/main.go:12,29` | synthetic `admin@sigap.local` (bootstrap super_admin; subject not set here) |
| 44 | `apps/api/internal/auth/local_seed_actors_test.go:39-48` | pins deterministic UUID↔subject pairs for `e2e-schedule-*` |
| 45 | `apps/api/internal/handler/cross_facility_test.go:45-47,212,535` | `local-patient-p1`, `local-patient-p2`, `local-patient-p2x` citizen subjects |
| 46 | `apps/api/internal/identity/actor_test.go:82-83,153,166` | `local-facility-admin` and uuid-shaped JWT subject ≠ app user id (trust-boundary test) |

### 1.4 Deploy / docs / config that name Supabase

| # | File:line | Touchpoint |
|---|---|---|
| 47 | `supabase/` directory (config + `.temp/start-secrets/.../docker.env`) | the local Supabase stack itself (auth at `http://127.0.0.1:54321/auth/v1`) |
| 48 | `docs/operations/DEPLOYMENT_RUNBOOK.md` | references Supabase provisioning |
| 49 | `design/auth-integration-inspection.md`, `design/jwt-env-config.md`, `design/patient-identity-interop.md`, `design/supabase-auth-verdict.md` | design docs encoding the current Supabase contract |

---

## 2. The Go JWT / env contract (exact)

### 2.1 `AuthConfig` field names → env keys

`apps/api/internal/auth/config.go:44-50`

| `AuthConfig` field | env key | notes |
|---|---|---|
| `Mode` (`AuthMode`) | `SIGAP_AUTH_MODE` | `"dev"` \| `"jwt"` \| `"disabled"`; default when empty = `disabled` |
| `Issuer` (`string`) | `SIGAP_AUTH_ISSUER` | required when `Mode == jwt` |
| `Audience` (`string`) | `SIGAP_AUTH_AUDIENCE` | required when `Mode == jwt` |
| `JWKSURL` (`string`) | `SIGAP_AUTH_JWKS_URL` | optional; no JWKS URL ⇒ `keyFunc` rejects every token (`"no JWKS configured"`) |

Current local Supabase values (from `design/jwt-env-config.md`): issuer
`http://127.0.0.1:54321/auth/v1`, audience `authenticated`, JWKS
`.../auth/v1/.well-known/jwks.json`. These three MUST be repointed at Better Auth
(`SIGAP_AUTH_ISSUER`/`SIGAP_AUTH_JWKS_URL` → Better Auth's `/.well-known/jwks.json`,
`SIGAP_AUTH_AUDIENCE` → Better Auth's audience claim), or the Go API keeps
trusting a Supabase issuer that no longer mints tokens.

### 2.2 `validJWTAlgs()` — the algorithm allowlist

`apps/api/internal/auth/jwt_provider.go:334-338`

```
"RS256","RS384","RS512","ES256","ES384","ES512"
```

Better Auth defaults to `ES256` (or `RS256`). If the Better Auth signing key uses
an algorithm outside this list (e.g. `EdDSA`/`Ed25519`, or `PS256`), the Go
allowlist must be extended **and** `parseJWK` mirrored accordingly — otherwise
every token is rejected `alg`-first.

### 2.3 `keyFunc` kid handling

`apps/api/internal/auth/jwt_provider.go:316-331`

- `alg == "none" || alg == ""` → error (reject).
- `kid` missing **and** a JWKS URL is configured → error (`"missing kid in JWT header"`). So Better Auth tokens MUST carry a `kid`.
- No JWKS configured (`p.jwks == nil`) → every token rejected. `SIGAP_AUTH_JWKS_URL` MUST point at Better Auth's JWKS.
- Otherwise the key is fetched from a 15-min TTL cached JWKS (`newJWKSCache`, `ttl: 15*time.Minute`) via `pt.jwks.get(kid)`; `parseJWK` supports `RSA` and `EC` (P-256/384/521) only.

### 2.4 `Authenticate` claims checks

`apps/api/internal/auth/jwt_provider.go:232-295`

- `Authorization: Bearer <jwt>` only; empty/malformed → `identity.Actor{}` (unauthenticated, fail-closed, **not** an error).
- `jwt.ParseWithClaims(tokenStr, &sigapClaims{}, p.keyFunc, WithValidMethods(validJWTAlgs()), WithIssuer(cfg.Issuer), WithAudience(cfg.Audience))`.
- `sigapClaims` = `jwt.RegisteredClaims` + `Permissions []string` (the latter is **ignored** for authz — AUDIT-101).
- Enforced by the library: `exp`/`nbf`/`iat`, `iss`, `aud`, `alg` allowlist, signature. `alg=none` is rejected in `keyFunc` and by `WithValidMethods`.
- On success: `sub := claims.Subject`; then `resolver.Resolve(ctx, sub)` returns the DB RBAC result (`Permissions`, `Grants`, `AppUserID`); token claims never authorise.
- `claims.Issuer == "sigap-dev"` ⇒ `ActorDev` heuristic (test-only; never a production issuer match).

### 2.5 Provider construction — `main.go`

`apps/api/cmd/server/main.go:258-320`

- `authCfg, err := auth.LoadConfigFromEnv()`.
- `if authCfg.Mode == auth.AuthModeJWT {`:
  - `dbPool != nil` → `auth.NewRBACResolver(dbPool)` → `auth.NewJWTProviderWithResolver(*authCfg, resolver)` (server-side RBAC).
  - else → `auth.NewJWTProvider(*authCfg)` (fail closed, zero permissions).
- `else` (dev/disabled) → `auth.NewProvider(authCfg)` (which internally selects `DevIdentityProvider` / `Disabled`).
- Then the optional `SIGAP_LOCAL_RBAC_TEST_IDENTITY == "true"` block replaces the provider with `NewLocalRBACTestIdentityProvider(NewRBACResolver(dbPool))` **only** when it `Armed()` (i.e. `SIGAP_ENV=="local"`).

So `SIGAP_AUTH_MODE` is the single switch between dev (synthetic header) and jwt
(real token) — Better Auth replaces the jwt **issuer/JWKS/audience**, not this
switch.

---

## 3. Identity mapping — where a Better Auth `sub` must land

The chain, end to end:

1. **Storage** — `packages/db/migrations/0008_identity_subject.sql:12-19`
   `app_users.subject TEXT` + `uq_app_users_subject_active` (unique among non-deleted rows). The subject is "the stable identity the token proves; never itself an authorization source."

2. **Lookup** — `apps/api/internal/auth/rbac_resolver.go`
   - `ResolvedPermissions{ Permissions, Grants, AppUserID }` (line 31-34).
   - `Resolver.Resolve(ctx, subject)` interface (line 47).
   - `ResolveByAppUserID(ctx, appUserID)` extension (line 60, 142).
   - `permissionsQuery` ends `WHERE u.subject = $1 AND u.status='active' AND u.deleted_at IS NULL` (line 106) — **this is the exact join key**.
   - `Resolve` (line 158) returns `AppUserID` = the mapped `app_users.id` (line 260).
   - `NewRBACResolver(pool)` is the single production constructor (line ~70s).

3. **Actor** — `apps/api/internal/identity/actor.go:60-73`
   `Actor{ UserID, Type, Permissions, FacilityGrants, IsDev, AppUserID }`.
   - `UserID` = external subject (Better Auth `sub`) — identity only, never an authorization source.
   - `AppUserID` = `app_users.id` the resolver mapped that subject to.
   - `AuditUserID()` returns `AppUserID` (line 97-98) — the only value `audit_events.actor_user_id` (FK → `app_users.id`) accepts.

4. **The patient seam** — `apps/api/internal/identity/patient_account.go:17-39`
   `Actor.PatientAccountID()` returns `AppUserID` **iff** `Type==ActorUser`, `AppUserID != ""`, and `Permissions` **and** `FacilityGrants` are both empty (a citizen = an `app_user` with no staff role). Storage is `appointments.patient_account_id` (0014).

**The seam in one sentence:** a Better Auth `sub` must be written into
`app_users.subject` for every staff + citizen account (the same way today's
Supabase `sub` = `auth.users.id` would be backfilled), so that
`rbac_resolver.go::Resolve` (`WHERE u.subject = $1`) maps it to `app_users.id`,
which then flows `JWTProvider.Authenticate → Actor{UserID, AppUserID} →
ActorFromContext → Actor.PatientAccountID() → appointments.patient_account_id`.

A citizen must resolve **grant-free** (no `user_roles` rows) for
`PatientAccountID` to return their id; staff resolve with roles/grants and
`PatientAccountID` correctly returns `""`.

---

## 4. Current synthetic citizen subjects + dev/test selectors

### 4.1 Synthetic citizen subjects (DB-seeded `app_users.subject`)

| subject | app_users.id | email | source |
|---|---|---|---|
| `local-patient-p1` | `…c307` | `patient-p1@sigap.local` | `packages/db/seed/regional_tenancy.sql:153` |
| `local-patient-p1` (fixture) | — | — | `apps/api/internal/handler/cross_facility_test.go:45-47` |
| `local-patient-p2`, `local-patient-p2x` | — | — | `apps/api/internal/handler/cross_facility_test.go:212,535` |
| `local-global-super-admin` | `…d990` | `global-admin@sigap.local` | `packages/db/seed/dev.sql:41` |
| `local-facility-admin` | `…d991` | `facility-admin@sigap.local` | `packages/db/seed/dev.sql:42` |
| `local-zero-scope-admin` | `…d992` | `zero-scope-admin@sigap.local` | `packages/db/seed/dev.sql:43` |
| `e2e-schedule-manager` | `…d993` | `e2e-schedule-manager@sigap.local` | `packages/db/seed/dev.sql:114` |
| `e2e-schedule-mixed` | `…d994` | `e2e-schedule-mixed@sigap.local` | `packages/db/seed/dev.sql:115` |
| `e2e-schedule-reader` | `…d995` | `e2e-schedule-reader@sigap.local` | `packages/db/seed/dev.sql:116` |
| `local-org-admin-a/b`, `local-regional-admin`, `local-facility-admin-a1`, `local-operator-a1/b1` | `…c301-306` | `*@sigap.local` | `packages/db/seed/regional_tenancy.sql:147-152` |

> Note on `patient-a@/patient-b@sigap.local`: those exact tokens are **not** found
> anywhere in the worktree. The citizen concept is represented by
> `local-patient-p1` (+ `-p2`/`-p2x` in tests) backed by
> `patient-p1@sigap.local`. `patient-a`/`patient-b` appear to be an external
> label for the P1/P2 pair, not a literal subject string in source.

### 4.2 UUID-shaped subject backfills (test/fixture, never production-verified)

The **one genuine data gap** (from `design/auth-integration-inspection.md:131`) is
that **no production backfill writes `app_users.subject = auth.users.id`**. Real
Supabase/Better Auth accounts are un-mapped until a backfill runs. The only
uuid-shaped subjects are test fixtures:

- `apps/api/internal/auth/rbac_resolver_test.go:64,497` — `"lifecycle-" + uuid.NewString()`, uuid subject.
- `apps/api/internal/auth/facility_scope_test.go:305` — `"scope-" + uuid.NewString()`.
- `apps/api/internal/handler/cross_facility_test.go:61` — `suB = uuid.NewString()` (citizen subject in tests).
- `apps/api/internal/handler/facility_scope_test.go:803,850,1258` — uuid subjects.
- `apps/api/internal/identity/actor_test.go:162-166` — pins that "a jwt subject that is uuid-shaped is still not the app user id".

### 4.3 Dev / test selectors (env + header)

| Selector | Gate (env) | Header | Actor type | Permissions |
|---|---|---|---|---|
| Dev identity | `SIGAP_DEV_IDENTITY=true` | `X-Sigap-Dev-User-ID` | `ActorDev` | fixed read-only set (`facility.read, notification.read, appointment.read, queue.read, schedule.read, audit.read`) — `dev_provider.go:33-63`, `dev_middleware.go:9-27` |
| Local RBAC test identity | `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true` **and** `SIGAP_ENV==local` | `X-Sigap-Local-Test-Subject` | `ActorUser` (real resolver) | **none hard-coded** — resolved from DB RBAC (`local_test_identity_provider.go:64`, `main.go:297-320`) |
| Web-tier E2E actor | `SIGAP_ENV=local` **and** `SIGAP_LOCAL_E2E_ACTOR` non-empty | emits `X-Sigap-Local-Test-Subject` to Go | n/a (header only) | `auth.ts::localE2eActorHeader()` (`lib/server/auth.ts:~91-135`) |

Production gates (all four must hold or the selector is inert): `SIGAP_ENV=local`,
flag set, `config.GuardDevCapabilities` startup refusal outside local, and the
web tier independently empty outside `SIGAP_ENV=local`.

---

## 5. Change checklist — Better Auth REPLACES Supabase (no Supabase authority remains)

### 5.1 Remove / re-point Supabase (so no Supabase remains an authority)

1. **Web env**: drop `PUBLIC_SUPABASE_URL` / `PUBLIC_SUPABASE_ANON_KEY`; add
   `BETTER_AUTH_URL` / `BETTER_AUTH_SECRET` (`.env`, `.env.example:41-43`).
2. **`lib/supabase/server.ts`** → replace with a Better Auth server client (or
   delete + generate one in `$lib/server/auth`). The `cookies.getAll/setAll`
   round-trip seam stays; only the lib backing it changes.
3. **`hooks.server.ts:10-18`** → stop calling `client.auth.getSession()`; derive
   `event.locals.session` (an updated `AuthSessionLike`) from the Better Auth
   session. Keep `applyPendingHeaders` for cookie/cache-header writes.
4. **`app.d.ts:1,7-8`** → drop `SupabaseClient`; type `Locals.supabase` (or its
   Better Auth replacement) and `Locals.session` to the new shapes. Remove the
   `/auth/v1` mental model.
5. **`lib/server/auth.ts:25-28`** → `supabaseConfigured()` becomes
   `authConfigured()` gated on Better Auth env. **`authHeaders`/`authSessionLike`**
   (19-23, 32-41) keep shape but source from Better Auth's access token; the
   `Authorization: Bearer` → Go API contract is unchanged.
6. **Auth routes** `login/register/logout/+page.server.ts` → swap
   `locals.supabase.auth.signInWithPassword/signUp/signOut` for Better Auth
   `signIn.email` / `signUp.email` / `signOut`. `auth/callback` route must be
   **added** (OAuth), since none exists today.
7. **e2e specs** `smoke.spec.ts:46-68`, `admin-facility-mutations.spec.ts`,
   `admin-mutations.spec.ts`, `admin-org-isolation.spec.ts`, and the
   `AuthForms/auth-presentation` comments → update "Supabase" wording + any
   auth-surface assertions. `auth-presentation.test.ts:389` (`must not import
   supabase`) becomes `must not import better-auth` client-side.
8. **Cookie**: the `sb-<project-ref>-auth-token` cookie disappears; allow Better
   Auth's session cookie through the `hooks.server.ts` cookie round-trip and CSP.
9. **`supabase/` directory + compose/edge wiring** → remove once no local stack
   is needed; `docs/operations/DEPLOYMENT_RUNBOOK.md` and any compose labels must
   stop referencing Supabase.

### 5.2 Re-point the Go trust boundary (still jwt, now Better Auth issuer)

10. `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`
    (`config.go:44-69`) → Better Auth values (issuer + `/.well-known/jwks.json`
    + audience). Never point at `127.0.0.1:54321/auth/v1` again.
11. **Algorithm allowlist** `validJWTAlgs()` (`jwt_provider.go:334`) → add Better
    Auth's alg if outside `{RS,ES}{256,384,512}` (and extend `parseJWK` if
    `EdDSA`). Confirm Better Auth emits a `kid` (required by `keyFunc`).
12. **Subject backfill** — the one real data step: write every staff + citizen
    `app_users.subject = <Better Auth sub>`. Citizen `sub` MUST land in
    `app_users.subject` (0008) for `rbac_resolver.go:106` `WHERE u.subject = $1`
    to resolve → `AppUserID` → `PatientAccountID`.

### 5.3 MUST stay (do not touch)

- **`app_users`** + `0008_identity_subject.sql` (`subject` + `uq_app_users_subject_active`).
- **`patient_account` (0014)** — `appointments.patient_account_id UUID REFERENCES app_users(id) ON DELETE SET NULL` + index.
- **RBAC schema** — `roles`, `permissions`, `user_roles`, `role_permissions`
  (`0003_identity_rbac.sql:7-50`), the `NewRBACResolver` → `Resolve` →
  `ResolvedPermissions` chain, and facility/org provenance.
- **`apps/api/internal/identity/actor.go` + `patient_account.go`** — the
  `UserID` (external subject) vs `AppUserID` (app_users.id) split, `AuditUserID`,
  `PatientAccountID` fail-closed semantics, `ActorFromContext`.
- **`apps/api/internal/auth/jwt_provider.go` trust boundary** — `alg=none`
  rejection, `WithIssuer`/`WithAudience`/`WithValidMethods`, fail-closed on
  unknown subject / resolver error; permissions never from token claims (AUDIT-101).
- **`config.go` env surface** (`SIGAP_AUTH_MODE/ISSUER/AUDIENCE/JWKS_URL`) — only
  the *values* change; the contract stays.
- **Migration ledger `0001–0014`** — forward-only, immutable, tracked by
  `schema_migrations`; no mutation, no renumbering. Any subject population is a
  **data/seed/backfill step**, never an edit to a `000x` file.
- **`+layout.server.ts` `hasSession`/`userEmail`-only boundary** — the browser
  never receives a token/role/scope; the client is never an authorization authority.
- **Local dev/test selector production gates** (`GuardDevCapabilities`,
  `SIGAP_ENV=local`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR`) —
  keep the fail-safe so header-based selectors still cannot arm outside local.

---

## 6. Fragility ranking (single most fragile seam)

**The single most fragile seam is `hooks.server.ts:10-18` → `lib/server/auth.ts::authHeaders` → `app_users.subject`.**

`event.locals.session` is the one object the whole chain trusts: it feeds
`+layout.server.ts::hasSession`, every `Authorization: Bearer` proxy header
(`authHeaders`), and the login/register/logout actions. If Better Auth's session
shape or cookie name doesn't match what `hooks.server.ts` expects, the web tier
silently yields `locals.session = null` → `hasSession=false` and **no bearer
token** → the Go API sees every request as unauthenticated (fail-closed). That is
safe, but it is also a whole-system outage disguised as "not logged in". It is
fragile precisely because failure is silent and non-obvious: the pipeline still
renders, `supabaseConfigured()`/`authConfigured()` may still be true, and only the
`hasSession`/`401`/`403` path reveals it. The subject side is the second half of
the same seam: even with a valid token, an un-backfilled `app_users.subject` makes
the resolver return empty permissions/grants and `AppUserID=""`, so a citizen's
`PatientAccountID()` returns `""` and their appointments are treated as anonymous.
Both halves fail closed; both are easy to not notice.