# Supabase Runtime Retirement — Re-verification of the 49 Touchpoints

Scope: the 49 touchpoints enumerated in
`design/supabase-dependency-audit.md` (the PRIOR milestone's read-only
enumeration), **re-verified against the current tree** after the Better Auth
swap landed on branch `feat/sigap-selfhosted-auth` (HEAD `60de7d6`,
`test(auth): reconcile stale harnesses to the Better Auth contract`).

This document does NOT redo the audit. It answers one question per touchpoint:
*does this still couple the running system to Supabase?*

Prepared 2026-11-11. Documentation only — no code was modified.

---

## 0. Classification key

| Class | Meaning |
|---|---|
| **(a) REMOVED** | The swap already decoupled it. The Supabase coupling is gone; whatever replaced it (Better Auth, or a Supabase-agnostic seam) is what runs today. |
| **(b) LEGACY-ADAPTER** | Code/deps kept **only as rollback reference**. Nothing in the running system loads it. Confined to `apps/web/src/lib/supabase/*`, the `@supabase/*` dependencies, and the gitignored local `supabase/` stack. |
| **(c) RUNTIME-DEPENDENCY** | Still required for the system to run. **Must be zero** to satisfy the acceptance target. |
| **(d) DOC** | Prose/comments/config-comments that still name Supabase. No runtime effect; stale wording. |
| **(e) TEST** | Test/fixture code. Not runtime; mostly still valid guards or identity fixtures. |

Raw counts over the 49:

| Class | Count |
|---|---|
| (a) REMOVED | **33** |
| (b) LEGACY-ADAPTER | **5** |
| (c) RUNTIME-DEPENDENCY | **0** |
| (d) DOC | **4** |
| (e) TEST | **7** |
| **Total** | **49** |

---

## 1. Web tier (`apps/web`) — items 1–25

| # | Prior touchpoint | Status today | Class |
|---|---|---|---|
| 1 | `hooks.server.ts:2` `import { createSupabaseServerClient }` | Gone. `hooks.server.ts:3` now imports `{ auth, betterAuthConfigured, type AuthSession }` from `$lib/server/auth.better`; `git grep '\$lib/supabase'` → no hits in `apps/`. | **(a)** |
| 2 | `hooks.server.ts:3` `import { supabaseConfigured }` | Gone. `betterAuthConfigured()` lives in `auth.better.ts:32`; `git grep 'supabaseConfigured' apps/` → no hits. | **(a)** |
| 3 | `hooks.server.ts:10-18` `client.auth.getSession()` as the session authority | Replaced. `hooks.server.ts` resolves the session via `auth.api.getSession({ headers })`, guarded so an auth-store outage degrades to signed-out, then mints the ES256 JWT via `auth.api.getToken` into `locals.jwt`. | **(a)** |
| 4 | `hooks.server.ts:20-22` `applyPendingHeaders(response)` | Gone from `hooks.server.ts` (`@supabase/ssr` anti-CDN-cache headers). Cookie writes now go through Better Auth's `sveltekitCookies` plugin; `svelteKitHandler` returns the final `Response`. | **(a)** |
| 5 | `app.d.ts:1` `import type { SupabaseClient } from '@supabase/ssr'` | Gone. `app.d.ts` imports `{ auth as BetterAuth, AuthSession }` from `$lib/server/auth.better`. | **(a)** |
| 6 | `app.d.ts:7-8` `Locals.supabase` / `Locals.session: AuthSessionLike` | Gone. `Locals` is now `{ auth: BetterAuth \| null; session: AuthSession \| null; jwt: string \| null }`. No `supabase` key. | **(a)** |
| 7 | `lib/supabase/server.ts:1-2` `@supabase/ssr` + `@supabase/supabase-js` imports | **File still exists, still imports `@supabase/*` — but nothing imports the file.** `git grep '\$lib/supabase' apps/` → 0 hits; `git grep 'createSupabaseServerClient' apps/ ':!apps/web/src/lib/supabase'` → 0 hits. Orphaned rollback adapter. | **(b)** |
| 8 | `lib/supabase/server.ts:22-45` `createServerClient(URL, ANON_KEY, {cookies})` | Same file, same dead code path. The `PUBLIC_SUPABASE_*` reads at `server.ts:26-27` are reachable only if someone re-imports the adapter. | **(b)** |
| 9 | `lib/server/auth.ts:19-23` `AuthSessionLike` | Deleted. `git grep 'AuthSessionLike' apps/` → 0 hits. The session type is now `AuthSession` (`auth.better.ts:127`). | **(a)** |
| 10 | `lib/server/auth.ts:25-28` `supabaseConfigured()` | Deleted. Alarms now use `betterAuthConfigured()` (`auth.better.ts:32`) / `Boolean(locals.auth)`. | **(a)** |
| 11 | `lib/server/auth.ts:32-41` `authHeaders()` reading `locals.session.access_token` | Re-pointed: `authHeaders()` now reads `locals.jwt` (the Better Auth jwt-plugin ES256 token) and still emits `Authorization: Bearer <token>`. The Go contract is unchanged; **no Supabase type or field remains**. | **(a)** |
| 12 | `routes/auth/login/+page.server.ts` `locals.supabase.auth.signInWithPassword` | Replaced by `auth.api.signInEmail({ body: { email, password } })` (rate-limited, 401-vs-503 distinguished, same-origin `redirectTo`). `signInWithPassword` → 0 hits in `apps/`. | **(a)** |
| 13 | `routes/auth/register/+page.server.ts` `locals.supabase.auth.signUp` | Replaced by `auth.api.signUpEmail({ body: { email, password, name } })`, `authConfigured: Boolean(locals.auth)`. | **(a)** |
| 14 | `routes/auth/logout/+page.server.ts` `locals.supabase.auth.signOut()` | Replaced by `locals.auth.api.signOut({ headers })`. | **(a)** |
| 15 | `routes/auth/login/+page.svelte:11-12` comment "the 503 when Supabase is unconfigured" | **Still says "Supabase".** The code below it is Better Auth. Stale comment only. | **(d)** |
| 16 | `routes/auth/register/+page.svelte` mirror comment | No `supabase` string remains (absent from `git grep -i supabase apps/`). | **(a)** |
| 17 | `routes/auth/logout/+page.svelte` comment | No `supabase` string remains. | **(a)** |
| 18 | `routes/+layout.server.ts:34-41` `locals.session?.user?.email` / `?.access_token` | Still `userEmail`, but `hasSession` is now `Boolean(locals.session?.session?.token)`. Same two-field, token-free boundary (browser never receives token/role/scope); the Supabase-shaped `access_token` field is gone. | **(a)** |
| 19 | `lib/citizen/AuthForms.svelte:26,53` comments naming "a configured Supabase" | **Still says "Supabase"** in two comments. The enforced invariant ("presentation layer must not import supabase") still holds. | **(d)** |
| 20 | `lib/citizen/auth-presentation.test.ts:378-389` "must not import supabase" firewall | **Still present and still passing as written** — `expect(code).not.toMatch(/supabase/i)` over the five client-side auth files. This is a live, meaningful client-side firewall, not a stale reference. | **(e)** |
| 21 | `apps/web/package.json` `@supabase/supabase-js` + `@supabase/ssr` | **Both still declared** (`package.json:51-52`), reachable only from the orphaned adapter. Rollback dependency surface. | **(b)** |
| 22 | `.env` / `.env.example:41-43` `PUBLIC_SUPABASE_URL` / `PUBLIC_SUPABASE_ANON_KEY` | `.env` (untracked, local) **still sets both** to the old local stack (`http://127.0.0.1:54321`); `.env.example` (tracked) has them commented with a "Publishable values ONLY" note. **Nothing in `apps/` reads them** except the orphaned adapter. Rollback residue in config; the `.env.example` block is pure doc. | **(b)** |
| 23 | `e2e/admin-facility-mutations.spec.ts:42` `X-Sigap-Local-Test-Subject` | Still present and still correct — it routes identity through the Go local RBAC test selector, which is Supabase-independent. No `supabase` string in the file. | **(e)** |
| 24 | `e2e/smoke.spec.ts:46-59` comment "With Supabase configured …" | The **assertion** (form *or* `auth-unavailable` notice is visible, honest either way) is still valid and Better Auth-shaped. The **comment** still says "Supabase". | **(e)** (assertion) / **(d)** (wording) |
| 25 | `e2e/smoke.spec.ts:60-68` `auth-unavailable` notice | Still valid: the notice is driven by `authConfigured: betterAuthConfigured()`, not Supabase. | **(e)** |

### Negative findings, re-checked

- **Still no `auth/callback` route.** `apps/web/src/routes/auth/` contains only
  `login/`, `register/`, `logout/`. Better Auth serves its own handler at
  `/api/auth/*` via `svelteKitHandler`, so no callback route was needed.
- **Still no client-side Supabase wrapper** (`$lib/supabase/client*` does not
  exist); the only Supabase file is the orphaned server adapter.
- **Cookie `sb-*-auth-token` is gone.** The session cookie is now
  `better-auth.session_token`, written via Better Auth's `sveltekitCookies`
  plugin — no `getAll/setAll` Supabase seam remains in `hooks.server.ts`.

---

## 2. Go API (`apps/api`) — items 26–37

None of these ever depended on the Supabase **product**; they are the generic
JWT/OIDC trust boundary and they consume only issuer/audience/JWKS **values**.
After the swap those values come from Better Auth, so the Supabase coupling is
**removed** in every case while the code itself is retained unchanged.

| # | Prior touchpoint | Status today | Class |
|---|---|---|---|
| 26 | `cmd/server/main.go:160` `authMode := os.Getenv("SIGAP_AUTH_MODE")` banner | Unchanged code; the mode switch is `dev`/`jwt`/`disabled`, issuer-agnostic. | **(a)** |
| 27 | `main.go:258-276` `LoadConfigFromEnv()` → `NewJWTProviderWithResolver` \| `NewProvider` | Unchanged. Supabase was only ever the *value* behind `SIGAP_AUTH_ISSUER`/`JWKS_URL`. | **(a)** |
| 28 | `main.go:293-320` `SIGAP_LOCAL_RBAC_TEST_IDENTITY` selector | Unchanged and still local-gated. Never Supabase-coupled. | **(a)** |
| 29 | `internal/auth/config.go:44-50` `AuthConfig{Mode,Issuer,Audience,JWKSURL}` | Unchanged; provider-neutral. | **(a)** |
| 30 | `config.go:63-69` `LoadConfigFromEnv` reads `SIGAP_AUTH_*` | Unchanged; now fed Better Auth values. | **(a)** |
| 31 | `config.go:82-92` `Validate()` requires Issuer + Audience for jwt | Unchanged. | **(a)** |
| 32 | `jwt_provider.go:338` `validJWTAlgs()` = RS/ES 256/384/512 | Unchanged — and deliberately sufficient: Better Auth's jwt plugin is pinned to **ES256** in `auth.better.ts`, so no allowlist change was needed (`parseJWK` still accepts only `RSA`/`EC`). | **(a)** |
| 33 | `jwt_provider.go` `keyFunc` — `alg=none` rejected, `kid` required with a JWKS | Unchanged. Better Auth's JWKS emits `kid` (covered by `internal/auth/better_auth_compat_test.go`). | **(a)** |
| 34 | `jwt_provider.go:232-248` `Authenticate` `WithValidMethods`/`WithIssuer`/`WithAudience` | Unchanged; `WithIssuer` is now the Better Auth issuer. | **(a)** |
| 35 | `jwt_provider.go:272-295` `sub = claims.Subject` → `resolver.Resolve(ctx, sub)` | Unchanged — the Supabase `sub` becomes the Better Auth `user.id` in `app_users.subject`; the resolver join key is identical. | **(a)** |
| 36 | `jwt_provider.go:299-307` resolver-less jwt → zero permissions, fail closed | Unchanged. | **(a)** |
| 37 | `internal/config/envguard.go` `GuardDevCapabilities` | Unchanged; still refuses dev-only selectors outside `SIGAP_ENV=local`. | **(a)** |

New sibling (not one of the 49, evidence the re-point landed):
`apps/api/internal/auth/better_auth_compat_test.go` pins acceptance of Better
Auth's ES256 `{"kty":"EC","crv":"P-256",...,"kid":...}` JWKS.

---

## 3. Seeds / migrations / fixtures writing `subject` — items 38–46

These are **identity records and mappings**, not Supabase couplings. They store
an **opaque** subject string; Supabase's `auth.users.id` and Better Auth's
`user.id` both slot into the same column with no schema change. They are
classified **(a)** because no Supabase dependency remains — and they are called
out separately below because **they must NOT be deleted**.

| # | Prior touchpoint | Status today | Class |
|---|---|---|---|
| 38 | `packages/db/migrations/0003_identity_rbac.sql:7-24` `app_users` | Unchanged. Forward-only migration; per the prior audit §5.3 the ledger `0001–0014` is immutable. Holds the identity records. | **(a)** — RETENTION |
| 39 | `packages/db/migrations/0008_identity_subject.sql:12-19` `subject` + `uq_app_users_subject_active` | Unchanged. **This is the mapping seam** where a Better Auth `sub` lands. | **(a)** — RETENTION |
| 40 | `packages/db/seed/dev.sql:39-48` `subject` for `local-global-super-admin` (d990) etc. | Unchanged. Synthetic local subjects, Supabase-independent. | **(a)** — RETENTION |
| 41 | `packages/db/seed/dev.sql:114-118` `e2e-schedule-*` (d993-d995) subjects | Unchanged. Drives the local E2E actor selector. | **(a)** — RETENTION |
| 42 | `packages/db/seed/regional_tenancy.sql:147-157` org/regional/facility actors (c301-c306) + `local-patient-p1` (c307) | Unchanged. | **(a)** — RETENTION |
| 43 | `apps/api/cmd/bootstrap/main.go:12,29` synthetic `admin@sigap.local` | Unchanged (subject not set here). Supabase-independent. | **(a)** |
| 44 | `internal/auth/local_seed_actors_test.go:39-48` deterministic UUID↔subject pins | Unchanged. Test fixture, not Supabase. | **(e)** |
| 45 | `internal/handler/cross_facility_test.go` citizen subjects `local-patient-p1/p2/p2x` | Unchanged. Test fixture. | **(e)** |
| 46 | `internal/identity/actor_test.go` subject ≠ app-user-id trust-boundary test | Unchanged; guards the `UserID` (external subject) vs `AppUserID` split. | **(e)** |

---

## 4. Deploy / docs / config naming Supabase — items 47–49

| # | Prior touchpoint | Status today | Class |
|---|---|---|---|
| 47 | `supabase/` directory (local stack + `.temp` secrets) | Present on disk, **untracked**: `git ls-files supabase` → 0 files; `git check-ignore -v supabase/config.toml` → `.gitignore:52:supabase/`. Kept purely as a rollback reference; nothing in `apps/` reaches a Supabase service. | **(b)** |
| 48 | `docs/operations/DEPLOYMENT_RUNBOOK.md` (provisioning) | **Stale.** Still lists `PUBLIC_SUPABASE_URL`/`ANON_KEY` as required web env (`:271-272`), an operator gate "Supabase public vars present" (`:1201`), and cites `lib/server/auth.ts:27` / `lib/supabase/server.ts:26` — both wrong now (that code moved to `auth.better.ts`, and the adapter is orphaned). `docs/operations/RELEASE_CHECKLIST.md:40-41,496` mirrors it. Doc-only. | **(d)** |
| 49 | `design/*` docs encoding the Supabase contract (`auth-integration-inspection.md`, `jwt-env-config.md`, `patient-identity-interop.md`, `supabase-auth-verdict.md`) | Historical/handover docs describing the Supabase era. Superseded by the new Better Auth set. `design/prototypes/sigap-civic-v2.pen` also still labels the auth flows "Supabase signInWithPassword/signUp/signOut". Doc-only. | **(d)** |

Residual stale doc/config mentions outside the 49 (same class **(d)**, listed
for completeness): `scripts/dev/Start-V22Preview.ps1:56`,
`scripts/ops/evidence-env-presence.sh:34`,
`scripts/ops/test-operator-evidence-bundle.sh:43-44,169-178`,
`docker-compose.yml:164-166` (passes `PUBLIC_SUPABASE_*` into the web
container — harmless because nothing reads them), `review-shots/product-final/CAPABILITY-MATRIX.md:83`.

---

## 5. Acceptance check

> **Better Auth is the sole active authority; Go RBAC remains authoritative; no
> Supabase Auth service is needed during normal operation.**

Read against the tree:

1. **Better Auth is the sole active authority (web tier).** The single session
   authority is `hooks.server.ts` → `auth.api.getSession` on the Better Auth
   `auth`-schema instance (`auth.better.ts`); the JWT the Go API sees is minted
   by Better Auth's jwt plugin (ES256 + JWKS at `${baseURL}/api/auth/jwks`,
   surfaced as `locals.jwt`). No Supabase client is constructed anywhere
   reachable: `git grep '\$lib/supabase' apps/` → 0 hits.
2. **Go RBAC remains authoritative.** The Go API still resolves permissions,
   grants and `AppUserID` server-side from `app_users`/`user_roles`/
   `role_permissions` via `rbac_resolver.go::Resolve(ctx, sub)`; token claims
   never authorise (AUDIT-101). Only the *issuer/JWKS/audience values* changed.
3. **No Supabase Auth service is needed during normal operation.** Runtime
   dependencies on Supabase: **0** (class (c) empty). The only Supabase artifacts
   are the 5 class-(b) rollback items — an orphaned server adapter, two unused
   package.json deps, the untracked local stack, and the `PUBLIC_SUPABASE_*`
   values nothing reads.

### What must stay

The local `supabase/` directory is **gitignored** (`.gitignore:52`) and retained
as a **rollback reference only** — it is not part of the build, and deleting it
is a deliberate, separate decision, not a retirement step.

**Identity records and mappings must NOT be deleted.** Specifically:

- `app_users` (`0003`) and its `subject` column + `uq_app_users_subject_active`
  (`0008`) — the mapping seam a Better Auth `sub` lands in.
- The `subject` seeds (`dev.sql`, `regional_tenancy.sql`).
- `apps/web/scripts/backfill-auth-subjects.mjs` — the one real-data step that
  writes each account's Better Auth id into `app_users.subject`.
- The `patient_account` FK (`appointments.patient_account_id` → `app_users.id`).

Retiring Supabase removes a *vendor*; it must never remove identities.

---

## 6. Proof — no runtime Supabase dependency remains

Run from the repo root (`F:\sigap-design-v2`) on branch
`feat/sigap-selfhosted-auth`. All commands below are read-only.

### P1 — nothing imports the legacy adapter (expect: no output)

```bash
git grep -nI '\$lib/supabase' -- 'apps/'
git grep -nI -e 'createSupabaseServerClient' -e 'supabaseConfigured' -e 'AuthSessionLike' \
  -- 'apps/' ':!apps/web/src/lib/supabase'
```

### P2 — no runtime Supabase code outside the rollback adapter (expect: no output)

GNU grep (Linux/macOS/Git-Bash). Excludes `node_modules`, build output, the
legacy adapter directory, E2E/tests, and the package manifest:

```bash
grep -rniI \
  --exclude-dir=node_modules --exclude-dir=.svelte-kit --exclude-dir=build \
  --exclude-dir=supabase --exclude-dir=e2e --exclude-dir=tests \
  --exclude="*.test.ts" --exclude="*.test.js" --exclude="*.spec.ts" \
  --exclude=package.json \
  -e "@supabase" -e "signInWithPassword" -e "locals.supabase" \
  -e "PUBLIC_SUPABASE" -e "createServerClient" \
  apps/
```

Equivalent with ripgrep:

```bash
rg -ni --glob '!**/node_modules/**' --glob '!**/.svelte-kit/**' --glob '!**/build/**' \
   --glob '!apps/web/src/lib/supabase/**' --glob '!**/*.test.*' --glob '!**/*.spec.*' \
   --glob '!apps/web/tests/**' --glob '!apps/web/e2e/**' --glob '!**/package.json' \
   -e '@supabase' -e 'signInWithPassword' -e 'locals\.supabase' -e 'PUBLIC_SUPABASE' \
   apps/
```

### P3 — the only surviving Supabase references, for inspection

Anything still matching `supabase` under `apps/` (comments, tests, the
orphaned adapter, the dependency declaration) — i.e. the class-(b)/(d)/(e)
residue, never runtime code:

```bash
git grep -nI -i supabase -- 'apps/'
```

Expected residue: `apps/web/src/lib/supabase/server.ts` (class b),
`apps/web/package.json:51-52` (class b), comment-only hits in
`AuthForms.svelte`, `auth.better.ts`, `login/+page.svelte`,
`auth-presentation.test.ts`, `smoke.spec.ts`, `auth-actions.test.js`
(class d/e), and prose in Go doc-comments
(`local_test_identity_provider*.go`, `identity/actor*.go`) describing the
Supabase→Better Auth history (class d).

### P4 — the retired web env vars are read nowhere (expect: no output)

```bash
grep -rniI --exclude-dir=node_modules --exclude-dir=.svelte-kit --exclude-dir=build \
  --exclude-dir=supabase --exclude-dir=tests \
  'PUBLIC_SUPABASE' apps/
```

### Pass criteria

P1, P2 and P4 produce **no output**; P3 shows only class-(b)/(d)/(e) residue —
no `apps/web/src/hooks.server.ts`, no `app.d.ts`, no auth route, no `+layout.server.ts`,
no Go handler. Class (c) is empty ⇒ **no runtime Supabase dependency remains**.

---

## 7. One-line verdict

The Supabase **runtime** dependency is retired: 33 touchpoints decoupled, 5
retained as rollback-only adapters/deps/stack, 4 stale docs and 7 tests — and
**0 runtime dependencies**. The remaining work is cosmetic (stale doc wording)
plus the one deliberate retention decision: the gitignored `supabase/` stack and
every identity record/mapping stay.
