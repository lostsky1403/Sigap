# Auth Integration Inspection — dev → jwt (local Supabase)

Read-only inspection. Branch `feat/sigap-regional-pilot` @ `c98875f`.
No code changed. Every claim below cites a file + symbol.

---

## 1. Trust boundary today (how a dev actor is selected)

The authority that decides **who** a request is, is a compile/process-time environment
flag, never a cryptographically verified bearer token. Three mutually-exclusive
selectors exist, selected in order in `apps/api/cmd/server/main.go` (the "provider
wiring" block, ~L259–315):

| Selector | Process env gate | Request header it trusts | AppUser/provider | Actor shape |
|---|---|---|---|---|
| Dev identity | `SIGAP_DEV_IDENTITY == "true"` | `X-Sigap-Dev-User-ID` | `DevIdentityProvider` | `ActorDev`, fixed synthetic read-only perms, `IsDev=true`, NO `AppUserID` |
| Local RBAC test identity | `SIGAP_LOCAL_RBAC_TEST_IDENTITY == "true"` **and** `SIGAP_ENV == "local"` | `X-Sigap-Local-Test-Subject` | `LocalRBACTestIdentityProvider` | `ActorUser`, perms+grants resolved from real DB, `IsDev=false`, `AppUserID` set |
| JWT | `SIGAP_AUTH_MODE == "jwt"` | `Authorization: Bearer <jwt>` | `JWTProvider` | `ActorUser`, perms+grants from resolver, `AppUserID` set |

### 1a. Dev identity — `apps/api/internal/auth/dev_provider.go`

- `const devIdentityHeader = "X-Sigap-Dev-User-ID"`, `const devIdentityEnv = "SIGAP_DEV_IDENTITY"`.
- `NewDevIdentityProvider()` reads `SIGAP_DEV_IDENTITY` once; `Authenticate` returns a
  `zero` actor unless the env is exactly `"true"` and the header is non-empty.
- The resulting `identity.Actor` (see §3) carries a **hard-coded** permission set
  (`facility.read`, `notification.read`, `appointment.read`, `queue.read`,
  `schedule.read`, `audit.read`), `Type = ActorDev`, `IsDev = true`, and **no**
  `AppUserID`. It grants nothing from the database; it is a read-only demo identity.

### 1b. Local E2E / RBAC-test identity — `apps/api/internal/auth/local_test_identity_provider.go`

- `const localRBACSubjectHeader = "X-Sigap-Local-Test-Subject"`,
  `const localRBACIdentityEnv = "SIGAP_LOCAL_RBAC_TEST_IDENTITY"`.
- `NewLocalRBACTestIdentityProvider(resolver)` arms only when
  `localTestIdentityEnabled()` (i.e. `os.Getenv("SIGAP_ENV") == "local"`, exact,
  case-sensitive) is true; otherwise `Armed() == false` and `Authenticate` is a no-op.
- Armed `Authenticate` reads `X-Sigap-Local-Test-Subject`, then calls
  `p.resolver.Resolve(ctx, subject)` — the **same** `rbacResolver` the JWT path uses —
  and returns `Actor{Type: ActorUser, UserID: subject, Permissions, FacilityGrants, AppUserID}`.
- The safety argument (state in the file's header comment): the header names a subject;
  the database decides the permissions; the selector can **select** an identity the DB
  has already authorised, never manufacture access.

### 1c. Selection order — `apps/api/cmd/server/main.go`

```go
if authCfg.Mode == auth.AuthModeJWT {
    if dbPool != nil {
        resolver := auth.NewRBACResolver(dbPool)
        provider = auth.NewJWTProviderWithResolver(*authCfg, resolver)   // L271
    } else {
        provider = auth.NewJWTProvider(*authCfg)                         // L274 (fail-closed, zero perms)
    }
} else {
    provider = auth.NewProvider(authCfg)                                 // L278 (dev/disabled via factory)
}
// then, only if SIGAP_LOCAL_RBAC_TEST_IDENTITY=="true" and SIGAP_ENV=="local":
//   provider = auth.NewLocalRBACTestIdentityProvider(...)                // L306 — REPLACES the above
```

The local RBAC test selector is **not** a fallback and **not** a weakening of the JWT
provider: when armed it *replaces* the configured provider (dev or jwt) for every
request, so a local run can drive E2E as `e2e-schedule-manager` /
`e2e-schedule-mixed`. `SIGAP_DEV_IDENTITY` is not consulted when that selector arms.

### 1d. Web-tier mirror — `apps/web/src/lib/server/auth.ts`

- `proxyHeaders()` → `localE2eActorHeader()` emits `X-Sigap-Local-Test-Subject` **only**
  when `SIGAP_ENV.toLowerCase() === 'local'` and `SIGAP_LOCAL_E2E_ACTOR` (the web-side
  env) is a non-empty trimmed string. Otherwise `{}`.
- `authHeaders(event)` separately emits `Authorization: Bearer <access_token>` when
  `event.locals.session?.access_token` is present (see §2c). This is the only header
  that can carry a JWT; `proxyHeaders()` deliberately **never** injects
  `X-Sigap-Dev-User-ID` (that header is server-read but client-untrusted, AUDIT-1802).
- Every `+server.ts` proxy spreads both (`...proxyHeaders(), ...authHeaders(event)`),
  so in dev the browser gets the local test subject and in jwt mode it forwards the
  bearer token instead.

### 1e. Environment facts — `.env.example` (root)

- `SIGAP_AUTH_MODE=dev` is **commented out**; `config.go::LoadConfigFromEnv` therefore
  defaults to `AuthModeDisabled` unless the live (gitignored) `.env` explicitly sets a
  mode. The repo prompt states the running dev config sets `dev`.
- `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL` are **commented**.
- `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY` are **commented** ("publishable
  values only; the service-role key must never be set here").

---

## 2. The seams already present (what is wired vs commented)

### 2a. Go JWT verification — fully implemented, not commented

`apps/api/internal/auth/jwt_provider.go`:

- `type JWTProvider struct { cfg AuthConfig; jwks *jwksCache; resolver Resolver }` —
  the whole class is implemented, not stubbed.
- `NewJWTProvider(cfg)` builds a `jwksCache` when `cfg.JWKSURL != ""`;
  `NewJWTProviderWithResolver(cfg, resolver)` is the production constructor;
  `WithResolver(resolver)` returns a resolved copy.
- `Authenticate(r)` enforces `alg ≠ none`, signature verification via `keyFunc`,
  `exp/nbf/iat` (via `jwt.WithValidMethods`/registered claims), and
  `iss`/`aud` via `jwt.WithIssuer(p.cfg.Issuer)` / `jwt.WithAudience(p.cfg.Audience)`.
- `keyFunc` rejects a missing `kid` when JWKS is configured and fetches from the
  `jwksCache`; `validJWTAlgs()` allows `RS256/384/512` and `ES256/384/512`.
- `sigapClaims struct { jwt.RegisteredClaims; Permissions []string }` — the
  `Permissions` claim exists only in tests and is **ignored** by `Authenticate` (AUDIT-101).

What is **commented/absent** in the wiring:

- `factory.go::NewProvider` (used only for the non-jwt branch) maps
  `AuthModeJWT → NewJWTProvider(*cfg)` **without** a resolver. But `main.go` does not
  take that path for jwt mode: it calls `NewJWTProviderWithResolver` directly when
  `dbPool != nil`. So the *production* jwt path is already resolver-backed; the
  resolver-less factory branch is only exercised by `provider_test.go` and is
  fail-closed (zero permissions).

### 2b. Subject mapping — `apps/api/internal/auth/rbac_resolver.go`

- `type ResolvedPermissions struct { Permissions []string; Grants []identity.FacilityGrant; AppUserID string }`.
- `permisionsQuery` ends `WHERE u.subject = $1 AND u.status = 'active' AND u.deleted_at IS NULL` —
  the resolver keys **directly on `app_users.subject`**. `Resolve(ctx, subject)` returns
  `AppUserID` (the `app_users.id`) alongside permissions and facility grants.
- Column source: `packages/db/migrations/0008_identity_subject.sql`
  (`ALTER TABLE app_users ADD COLUMN IF NOT EXISTS subject TEXT;` +
  `CREATE UNIQUE INDEX ... uq_app_users_subject_active ON app_users(subject) WHERE subject IS NOT NULL AND deleted_at IS NULL`).
- **Gap**: the `subject` column is populated by seeds/tests
  (`packages/db/seed/…`, `local_seed_actors_test.go`, `rbac_resolver_test.go`,
  `handler/…_test.go` all `INSERT ... subject = $1`), but there is **no** observed
  production backfill that writes `app_users.subject = auth.users.id`. For a real
  Supabase user the `sub` must be written into `app_users.subject` or the resolver
  returns empty (fail-closed). This is the one genuinely *new* piece of data work.

### 2c. SvelteKit session — wired, cookie name is library-default

`apps/web/src/hooks.server.ts`:

- Guarded by `supabaseConfigured()` (`auth.ts`: true iff `PUBLIC_SUPABASE_URL` **and**
  `PUBLIC_SUPABASE_ANON_KEY` are set). When true:
  ```ts
  const { client, applyPendingHeaders } = createSupabaseServerClient(event);
  const { data: { session } } = await client.auth.getSession();
  event.locals.supabase = client;
  event.locals.session = session;   // AuthSessionLike | null (app.d.ts)
  applyAuthHeaders = applyPendingHeaders;
  ```
- `createSupabaseServerClient` (`apps/web/src/lib/supabase/server.ts`) wraps
  `@supabase/ssr` `createServerClient(URL, ANON_KEY, { cookies: getAll/setAll })`. The
  session **cookie name is not hard-coded** in this repo; `@supabase/ssr` derives it
  from the project ref as `sb-<project-ref>-auth-token`. `getAll()/setAll()` round-trip
  through `event.cookies`.
- Typing: `apps/web/src/app.d.ts` declares `Locals.supabase: SupabaseClient | null` and
  `Locals.session: AuthSessionLike | null`.

`apps/web/src/routes/+layout.server.ts`:

- `load({ locals })` serialises only `userEmail` (`locals.session?.user?.email ?? null`)
  and `hasSession` (`Boolean(locals.session?.access_token)`). No token/role/scope ever
  reaches the browser.

The forwarder: `auth.ts::authHeaders(event)` reads `event.locals.session?.access_token`
and returns `{ Authorization: "Bearer <token>" }`, consumed by every `routes/api/v1/…`
`+server.ts` proxy (`...authHeaders(event)`). This is the exact seam that turns a
Supabase browser session into a bearer JWT the Go API already validates.

### 2d. Patient identity is already a pure function of the resolved Actor

`apps/api/internal/identity/actor.go` + `patient_account.go`:

- `Actor{ UserID, Type, Permissions, FacilityGrants, IsDev, AppUserID }`.
- `Actor.PatientAccountID()` returns `AppUserID` iff `Type == ActorUser`, `AppUserID != ""`,
  **and** both `Permissions` and `FacilityGrants` are empty (a citizen = an app_user with
  no staff role). Otherwise `""` (fail-closed). The storage is
  `packages/db/migrations/0014_patient_account.sql`
  (`ALTER TABLE appointments ADD COLUMN patient_account_id UUID REFERENCES app_users(id) ON DELETE SET NULL`).

So once the jwt path populates `Actor.AppUserID`, patient ownership flows automatically;
nothing in the patient-account logic needs to know "Supabase".

---

## 3. Minimal change list to switch `dev` → `jwt` (local Supabase)

1. **Go env — arm jwt mode** (`config.go::LoadConfigFromEnv` / `Validate`):
   set `SIGAP_AUTH_MODE=jwt`; uncomment and set
   `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`.
   Local Supabase values: `SIGAP_AUTH_ISSUER=http://127.0.0.1:54321/auth/v1`,
   `SIGAP_AUTH_AUDIENCE=authenticated`,
   `SIGAP_AUTH_JWKS_URL=http://127.0.0.1:54321/auth/v1/.well-known/jwks.json`.
   *(No code change: `main.go` already calls `NewJWTProviderWithResolver` when
   `SIGAP_AUTH_MODE=jwt` and a pool exists.)*

2. **Web env — turn on the Supabase client** (`auth.ts::supabaseConfigured`):
   set `PUBLIC_SUPABASE_URL` and `PUBLIC_SUPABASE_ANON_KEY`. This alone makes
   `hooks.server.ts` populate `event.locals.session` and every `+server.ts` proxy
   forward `Authorization: Bearer <access_token>` via `authHeaders(event)`.

3. **Subject backfill** (the one real data step): write `app_users.subject = auth.users.id`
   (i.e. the verified JWT `sub`) for each real staff/citizen account so
   `rbac_resolver.go::permissionsQuery` (`WHERE u.subject = $1`) can resolve the actor.
   Without it, every valid token resolves to `AppUserID=""`, empty permissions/grants,
   and `Actor.PatientAccountID()` returns `""`.

4. **Verify the resolver → Actor → PatientAccountID chain** end-to-end (no new code):
   `JWTProvider.Authenticate` writes `UserID=sub, AppUserID=resolved.AppUserID` →
   handlers call `identity.ActorFromContext(ctx)` → `Actor.PatientAccountID()`
   (`patient_account.go`) → `appointments.patient_account_id` (0014). Citizens must
   resolve grant-free (no `user_roles` rows) or `PatientAccountID` deliberately fails closed.

5. **Disable the synthetic selectors** (env-level, not code): ensure
   `SIGAP_DEV_IDENTITY` is not `"true"`, and either `SIGAP_LOCAL_RBAC_TEST_IDENTITY`
   is not `"true"` or `SIGAP_ENV != "local"`, so no header-based selector can
   override the jwt provider (the `main.go` selector block is skipped and
   `DevIdentityProvider.enabled == false`).

6. **Guard the header surface** (already implemented, no change needed): confirm
   `auth.ts::proxyHeaders()`/`localE2eActorHeader()` emit nothing outside `SIGAP_ENV=local`,
   and that `authHeaders(event)` is the only bearer-token path (it is).

Net code delta: **zero production code changes are strictly required** for steps 1, 2, 5, 6;
step 3 is a data migration/seed (not a schema migration); step 4 is verification. The
only place a code edit *would* be needed is if the resolver is deliberately left
resolver-less (it is not, when `dbPool != nil`), or if the `subject` backfill is to be
automated rather than scripted.

---

## 4. Files/migrations that MUST remain untouched

- **`packages/db/migrations/0001…0014`** — the SIGAP forward-only migration set, tracked
  by `schema_migrations` (`SIGAP_AUTO_MIGRATE=true`). None of it needs editing for this
  switch:
  - `0008_identity_subject.sql` (`app_users.subject` + `uq_app_users_subject_active`) —
    already exists, already consumed by `rbac_resolver.go`.
  - `0014_patient_account.sql` (`appointments.patient_account_id`) — already exists;
    the column is populated by application code, not by migration.
- **`apps/api/internal/identity/actor.go` + `patient_account.go`** — the
  `Actor`/`PatientAccountID` trust model. No change: jwt-mode actors are already
  `ActorUser` with `AppUserID` set, which is exactly what `PatientAccountID` consumes.
- **`apps/api/internal/auth/`** (`provider.go`, `factory.go`, `config.go`,
  `jwt_provider.go`, `rbac_resolver.go`, `dev_provider.go`,
  `local_test_identity_provider.go`) — the seam set is complete for `dev→jwt`. Do **not**
  edit any of these as part of the switch; specifically keep the "jwt fails
  closed on unknown subject / no resolver" behaviour in `jwt_provider.go::Authenticate`.
- **`apps/web/src/hooks.server.ts`, `+layout.server.ts`, `lib/server/auth.ts`,
  `lib/supabase/server.ts`, `app.d.ts`** — already wired for the Supabase session and
  already enforce the "no bearer token in the browser" boundary.

Anything that changes must be *additive* (new env values, a new subject-populating seed
or a supabase-triggered subject backfill), never a mutation of `0001–0014`, and never a
widening of `PatientAccountID`.