# Impersonation Production Guard — audit of every actor-selecting lever

Branch `feat/sigap-selfhosted-auth` @ `60de7d6`. Read-only audit, plus the
negative tests it cites. Every claim below cites a file + symbol; line numbers
are from this commit.

**Question this document answers.** Eight environment variables — the six that
name or arm an actor (`SIGAP_ENV`, `SIGAP_AUTH_MODE`, `SIGAP_DEV_IDENTITY`,
`SIGAP_ENGINE_FALLBACK`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`,
`SIGAP_LOCAL_E2E_ACTOR`) plus the two guard inputs (`SIGAP_API_HOST`,
`SIGAP_TLS_TERMINATED`) — and two header families can influence *who a request
is*. For each one: where is it read, what does it arm, what happens when it is
set outside `SIGAP_ENV=local`, and what concretely stops it in a deployment.

**Answer in one sentence.** Outside `SIGAP_ENV=local` every selector is inert
at request time, no header can name an actor or grant a permission, and a
process that asks for a dev capability refuses to start at all
(`config.GuardDevCapabilities`); the residual gaps are enumerated in §6, and one
of them (the loopback guard) is a real, narrow finding.

Test files added or extended for this audit:

| File | Role |
|---|---|
| `apps/api/internal/config/envguard_test.go` | extended: registry-driven refusal/allowance matrix for every registered dev flag |
| `apps/api/internal/auth/impersonation_negative_test.go` | new: forged-header and claim-escalation negatives, plus the boot-guard combinations |

---

## 1. The levers

| Env var | Read by | Arms | When armed | Outside `SIGAP_ENV=local` |
|---|---|---|---|---|
| `SIGAP_ENV` | `config` guards; web tier; providers | *(not a selector itself)* | Marked local as `TrimSpace`+`EqualFold("local")` (`envguard.go:69`) | The gate every other lever is measured against |
| `SIGAP_AUTH_MODE` | `auth.LoadConfigFromEnv`, `auth.NewProvider` (`factory.go:6`) | `dev` selects `DevIdentityProvider` | Header-driven synthetic actor | Boot refused (`envguard.go:24`); `jwt`/`disabled` unaffected |
| `SIGAP_DEV_IDENTITY` | `auth.NewDevIdentityProvider` (`dev_provider.go:34`) | `X-Sigap-Dev-User-ID` → `ActorDev`, read-only | Any request with the header, no DB, no token | Boot refused (`envguard.go:31`); provider otherwise reads no env but this one |
| `SIGAP_ENGINE_FALLBACK` | `cmd/server/main.go` (grpc client) | In-process `FakeQueueService` | No gRPC engine needed | Boot refused (`envguard.go:38`). Not an identity lever |
| `SIGAP_LOCAL_RBAC_TEST_IDENTITY` | `cmd/server/main.go:298` installer | `LocalRBACTestIdentityProvider` replaces the configured provider | `X-Sigap-Local-Test-Subject` → real DB RBAC resolution | Boot refused (`envguard.go:51`); provider additionally inert because it arms only on `SIGAP_ENV == "local"` (`local_test_identity_provider.go:90`) |
| `SIGAP_LOCAL_E2E_ACTOR` | **web tier only** — `apps/web/src/lib/server/auth.ts:125` | `X-Sigap-Local-Test-Subject` on upstream proxy calls | Local E2E runs as a seeded subject | Nothing in Go reads it. Blocked by the web gate (`auth.ts:121-130`) and by `scripts/ops/preflight-production-env.sh:75`. The header it emits is inert (§2) |
| `SIGAP_API_HOST` | `config.GuardDevNetworkBinding` (`envguard.go:111`) | — (a guard input) | With dev capabilities active, must be loopback | Only consulted in local; see finding F1 in §6 |
| `SIGAP_TLS_TERMINATED` | `config.GuardTLS` (`envguard.go:148`) | — (a guard input) | Required `true` outside local | Boot refused when missing outside local |

The set of variables the boot guard *itself* polices is `devOnlyFlags`
(`envguard.go:22`): `SIGAP_AUTH_MODE`, `SIGAP_DEV_IDENTITY`,
`SIGAP_ENGINE_FALLBACK`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`.
`TestGuardDevCapabilities_RegistryIsAudited` fails if that list and the audited
matrix in the test file ever diverge in either direction.

## 2. The two header families

| Header | Family | Read by | Meaning when its selector is armed | Meaning when it is not |
|---|---|---|---|---|
| `X-Sigap-Local-Test-Subject` | `X-Sigap-Local-Test-*` | `LocalRBACTestIdentityProvider.Authenticate` (`local_test_identity_provider.go:138`) | Names an `app_users.subject`; permissions and facility grants are resolved from the DB on every request | Nothing. The header is not read at all: the armed check precedes the header read (`:126-138`), so a disarmed selector never reaches the lookup |
| `X-Sigap-Dev-User-ID` | `X-Sigap-Dev-*` | `DevIdentityProvider.Authenticate` (`dev_provider.go:39-42`) | Names an actor with a fixed read-only, non-PHI permission set (`:64`) | Nothing. `NewDevIdentityProvider` snapshots `SIGAP_DEV_IDENTITY == "true"` once at construction (`:34`) |

Both header families are **identity selectors, never grants**:

- The local selector contributes identity only. It has no permission list of
  its own, and its actor is `identity.ActorUser` with `IsDev=false`
  (`:177-182`), so facility scope and permission-at-facility enforcement stay
  in force.
- The dev identity has no `AppUserID` and no facility grants, and its
  permission set deliberately omits every write key (`dev_provider.go:56-71`).

Headers in the `X-Sigap-Local-Test-*` namespace that **do not exist** —
`X-Sigap-Local-Test-Role`, `-Permissions`, `-Facility`, and the
`X-Sigap-Dev-Role` / `-Permissions` pair — are sent by the negative tests to
prove that no code path reads them. They are listed in
`impersonation_negative_test.go:forgedImpersonationRequest`.

**Who may set them.** Not a browser, cross-origin: the API's CORS allowlist is
`Content-Type, Authorization, X-Requested-With` (`main.go:62`), so a
cross-origin `X-Sigap-*` request fails preflight. Same-origin proxies *could*
forward a caller's header, so the web tier is asserted *by source* to read no
inbound header at all — `apps/web/src/lib/server/auth.test.ts:106-125` greps
`auth.ts` for `get('x-sigap…')` and for its `X-Sigap-*` literals, and requires
that the only literal emitted is `X-Sigap-Local-Test-Subject`.

## 3. Where each selector is armed, in order

`cmd/server/main.go`:

1. `GuardDevCapabilities()` (`:136`) — refuses to start if a registered dev
   flag is set outside local.
2. `GuardDevNetworkBinding()` (`:145`) — with dev capabilities active, requires
   a loopback `SIGAP_API_HOST`.
3. `GuardTLS()` (`:152`) — outside local, requires `SIGAP_TLS_TERMINATED=true`.
4. Provider construction (`:256-280`): `jwt` mode gets the JWT provider, wired
   with the DB RBAC resolver when a pool exists.
5. Local test selector installer (`:281-316`): only when
   `os.Getenv("SIGAP_LOCAL_RBAC_TEST_IDENTITY") == "true"` (`:298`) **and**
   `localProvider.Armed()` (`:307`) does it replace the configured provider.
6. Middleware chain (`:453`):
   `SecurityHeaders → TrustedProxy → DenyByDefault → auth.Middleware(provider) → Audit → RequirePermission → mux`.

Two layers therefore have to agree before the selector exists in a running
process: the flag gates *installation*, the environment gates *arming*. The
provider itself arms on `SIGAP_ENV == "local"` alone
(`local_test_identity_provider.go:90`, exact and case-sensitive) — pinned by
`TestImpersonationLocalSelectorArmsOnEnvironmentAlone` so a future change to
that split is a visible edit.

## 4. Production-block evidence

| # | Layer | Mechanism | Evidence |
|---|---|---|---|
| 1 | Boot | Any registered dev flag outside local ⇒ process exits 1 before any I/O | `main.go:136`, `envguard.go:69`; `TestImpersonationBootGuardRefusesArmableCombinations`, `TestGuardDevCapabilities_EachFlagRefusedOutsideLocal` |
| 2 | Boot | Registered dev flag in local ⇒ allowed (else local development is impossible) | `TestGuardDevCapabilities_EachFlagAllowedOnlyInLocal` |
| 3 | Boot | Case variants (`DEV`, `TRUE`) are dangerous to the guard even though every provider compares exactly | `TestGuardDevCapabilities_CaseVariantDangerousValueRefusedOutsideLocal`, `TestGuardDevCapabilities_FlagValueClassification` |
| 4 | Boot | In local with dev capabilities, non-loopback bind is refused | `main.go:145`, `envguard.go:111`; `TestImpersonationLoopbackGuardRequiresLoopbackForDevCapabilities` (see F1) |
| 5 | Boot | Outside local, TLS termination must be confirmed | `main.go:152`, `envguard.go:148`; `TestGuardTLS` |
| 6 | Request | Local selector arms only on exact `SIGAP_ENV == "local"`; every other spelling (including `Local`, `LOCAL`, ` local `, `localhost`, `local_test`) disarms it, and a disarmed selector never consults the resolver | `local_test_identity_provider.go:90,126-138`; `TestImpersonationForgedLocalTestSubjectIsInertOutsideLocal`, `TestLocalRBACTestIdentity_CannotArmOutsideLocal` |
| 7 | Request | Local selector contributes identity only: resolver result forwarded verbatim, `IsDev=false` | `local_test_identity_provider.go:177-182`; `TestImpersonationLocalSelectorNeverEscalatesViaHeaders`, `TestImpersonationLocalSelectorForgeYieldsNothingWithoutSeededRoles` |
| 8 | Request | Dev identity needs `SIGAP_DEV_IDENTITY == "true"` exactly, and grants no write permission even then | `dev_provider.go:34,40,64`; `TestImpersonationForgedDevUserIdIsInertWhenDevIdentityIsOff`, `TestImpersonationDevIdentityCannotBeEscalatedByHeader` |
| 9 | Request | JWT mode: token is authoritative for identity only; `permissions` claims are ignored; resolver error or absence fails closed | `jwt_provider.go:197,247,315`; `TestImpersonationJWTProviderIgnoresPermissionClaims`, `TestImpersonationJWTProviderFailsClosedWithoutClaimFallback`, `TestJWTProvider_ValidToken` |
| 10 | Request | JWT mode ignores both forged header families | `TestImpersonationJWTProviderIgnoresForgedHeaders` |
| 11 | Delivery | Web tier emits `X-Sigap-Local-Test-Subject` only when `SIGAP_ENV` (case-insensitively) is `local` **and** `SIGAP_LOCAL_E2E_ACTOR` is non-empty; never emits `X-Sigap-Dev-User-ID` | `auth.ts:121-130`, `proxyHeaders()`; `apps/web/src/lib/server/auth.test.ts` |
| 12 | Ops | `scripts/ops/preflight-production-env.sh` refuses a production deploy with any of `SIGAP_DEV_IDENTITY`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR`, `SIGAP_ENGINE_FALLBACK` set to a truthy value, and with any case/whitespace variant of `SIGAP_ENV=local` | `preflight-production-env.sh:75` |
| 13 | Ops | `scripts/ops/evidence-env-presence.sh` reports PRESENT/ABSENT for the three dev flags, and treats an uninspectable container as UNKNOWN (never ABSENT) | `evidence-env-presence.sh` |

The boot layers (#1-#5) are what make the request-time negatives the second
line rather than the only one: **a selector cannot be reached without the boot
guard having been satisfied first.** The one place that is not quite true is
F2 below, where a `SIGAP_ENV` spelling the guard accepts as local can arm the
dev identity while a deploy operator believes the environment is production.

## 5. Negative cases added by this audit

`apps/api/internal/auth/impersonation_negative_test.go` (all in package `auth`,
using the production header/env constants rather than literals):

| Test | Proves |
|---|---|
| `TestImpersonationForgedLocalTestSubjectIsInertOutsideLocal` | With the flag set to `true` and every forged header present, ten non-local `SIGAP_ENV` spellings leave the selector disarmed, return a zero actor, hold no privileged permission, and **never call the resolver** |
| `TestImpersonationLocalSelectorIsNotInstalledWhenItsFlagIsOff` | The flag is the installer: `SIGAP_ENV=local` plus `""`/`false`/`False`/`FALSE`/`TRUE`/`1`/`yes`/`on` leaves the configured provider in place, and forged headers produce a zero actor through the real middleware |
| `TestImpersonationLocalSelectorArmsOnEnvironmentAlone` | Pins the documented split: the provider arms on `SIGAP_ENV=local`; the flag is enforced by main.go + the boot guard |
| `TestImpersonationForgedDevUserIdIsInertWhenDevIdentityIsOff` | Eleven non-`"true"` spellings of `SIGAP_DEV_IDENTITY` (including `TRUE`, ` true `, `1`) leave the dev header inert |
| `TestImpersonationDevIdentityCannotBeEscalatedByHeader` | Armed dev identity: identity comes from the dev header, the fixed read-only set is exact, no facility grants, no `AppUserID`, no privileged permission from any forged header |
| `TestImpersonationLocalSelectorNeverEscalatesViaHeaders` | Armed local selector with a permissive resolver: permissions are exactly the resolver's (no `*`, `super_admin`, `admin` from headers), subject is the header's |
| `TestImpersonationLocalSelectorForgeYieldsNothingWithoutSeededRoles` | A `*-super-admin` subject with forged `X-Sigap-Local-Test-Permissions: *` against an unseeded subject yields zero permissions |
| `TestImpersonationJWTProviderIgnoresPermissionClaims` | A correctly signed token claiming `*`/`super_admin`/`schedule.manage` gets exactly the resolver's `[queue.read]` |
| `TestImpersonationJWTProviderIgnoresForgedHeaders` | Both forged header families are irrelevant on the JWT path: with a token the subject is the token's, without one the actor is zero and the resolver is not consulted |
| `TestImpersonationJWTProviderFailsClosedWithoutClaimFallback` | Resolver error ⇒ zero actor (no claim fallback); no resolver ⇒ identity with zero permissions |
| `TestImpersonationBootGuardRefusesArmableCombinations` | 13 boot combinations: refusal for every armable non-local case (naming the offending variable), allowance in `local`/`LOCAL`, no refusal for present-but-disarmed values, and no refusal for the web-side variable |
| `TestImpersonationLoopbackGuardRequiresLoopbackForDevCapabilities` | Loopback required in local for the dev capability, and pins F1 |

`apps/api/internal/config/envguard_test.go` (extended):

| Test | Proves |
|---|---|
| `TestGuardDevCapabilities_RegistryIsAudited` | `devOnlyFlags` ↔ audited matrix is a bijection; no entry lacks a predicate or label; `SIGAP_LOCAL_E2E_ACTOR` is deliberately not registered |
| `TestGuardDevCapabilities_FlagValueClassification` | Each predicate: dangerous value true, uppercase true, padded false (matching the providers' exact match), safe values false, empty false |
| `TestGuardDevCapabilities_EachFlagRefusedOutsideLocal` | 4 flags × 10 non-local spellings, each refused with a message naming the variable |
| `TestGuardDevCapabilities_EachFlagAllowedOnlyInLocal` | 4 flags × 5 local spellings, each allowed |
| `TestGuardDevCapabilities_EachFlagSafeValueNeverRefuses` | Present-but-safe values never refuse the boot in any environment |
| `TestGuardDevCapabilities_CaseVariantDangerousValueRefusedOutsideLocal` | `DEV`/`Dev`/`TRUE`/`True` all refuse the boot outside local |
| `TestGuardDevCapabilities_WebSideE2eActorIsNotAGoGate` | The Go guard's verdict is a function of `SIGAP_ENV` and the registered flags only, and does not change because of the web-side variable |
| `TestGuardDevCapabilities_MultipleViolations` (extended) | All four violations are reported in one error |

Together these cover the acceptance question directly: outside
`SIGAP_ENV=local` no header names an actor, no header grants a permission, the
JWT path ignores permission claims, and every dev flag is refused at boot while
being allowed in local.

## 6. Residual risk

**F1 — the loopback guard does not cover the local test selector.** (medium,
narrow)
`GuardDevNetworkBinding` (`envguard.go:111-142`) tests `SIGAP_DEV_IDENTITY`,
`SIGAP_AUTH_MODE` and `SIGAP_ENGINE_FALLBACK`. It does not test
`SIGAP_LOCAL_RBAC_TEST_IDENTITY`. Pinned by
`TestImpersonationLoopbackGuardRequiresLoopbackForDevCapabilities`.

Consequence: a process with `SIGAP_ENV=local` and
`SIGAP_LOCAL_RBAC_TEST_IDENTITY=true` binding `SIGAP_API_HOST=0.0.0.0` starts
and exposes header-driven, DB-backed identity selection to anything that can
reach the port. Both variables must be set, so it is an operator's explicit
configuration, and it is confined to an environment labelled local — but a
compose stack that runs `SIGAP_ENV=local` and publishes a port, which is a
common local topology, is exactly that configuration. Fixing it is a one-line
addition to the `devActive` expression; that file is outside this audit's edit
scope, so it is reported rather than changed.

**F2 — `DevIdentityProvider` does not consult `SIGAP_ENV`.** (low)
Its only gate is `SIGAP_DEV_IDENTITY == "true"` (`dev_provider.go:34`). The
production block is therefore entirely the boot guard (#1), the loopback guard
(#4) and the ops preflight (#12). This matters only for `SIGAP_ENV` values the
boot guard accepts as local but that a deploy operator would not consider
local: `LOCAL`, `Local`, ` local ` all satisfy `TrimSpace`+`EqualFold`
(`envguard.go:69-71`), and `GuardTLS` skips its check for them too. With
`SIGAP_ENV=LOCAL` and `SIGAP_DEV_IDENTITY=true` the API starts with the dev
header armed and no TLS confirmation required. The ops preflight is what
catches that combination (it lowercases and trims before comparing, and refuses
`local` for production), so the protection exists — but it is a layer that must
actually be run. Treating `SIGAP_ENV` as local only when it is exactly `local`
would remove the need for that discipline; that is a behaviour change to a
deliberately permissive guard and is out of scope here.

**F3 — the web gate and the Go provider disagree on case.** (low, fail-closed)
`localE2eActorHeader` lowercases `SIGAP_ENV` (`auth.ts:122`), so
`SIGAP_ENV=LOCAL` emits the header while the Go provider — exact match —
ignores it, producing an unauthenticated request rather than an impersonated
one. Already asserted in `auth.test.ts:176-181` as fail-closed, and the same
asymmetry is pinned on the Go side in
`TestImpersonationForgedLocalTestSubjectIsInertOutsideLocal`. No action needed;
recorded so the direction is a stated fact.

**F4 — `SIGAP_LOCAL_E2E_ACTOR` is not, and cannot be, a Go guard entry.**
(informational) The Go API never reads it (§1). Its block is the web gate plus
the ops preflight, and the header it produces is inert outside local. Registering
it in `devOnlyFlags` would refuse a boot for a variable the Go process does not
consume; `TestGuardDevCapabilities_RegistryIsAudited` fails if anyone does.

**F5 — CORS is not the control; the web tier's non-forwarding is.**
(informational) The allowlist (`main.go:62`) stops a cross-origin browser from
setting `X-Sigap-*`, but a same-origin proxy or a non-browser client can. The
property that actually holds is that the web tier never reads an inbound
`X-Sigap-*` header, asserted by source inspection in `auth.test.ts:106-125`.
Any new proxy that copies caller headers would silently break it.

## 7. Re-verifying this audit

```sh
cd apps/api
go test ./internal/config/ -run 'TestGuard' -v
go test ./internal/auth/  -run 'TestImpersonation|TestLocalRBACTestIdentity|TestDevIdentityProvider' -v
cd ../web && npx vitest run src/lib/server/auth.test.ts
sh scripts/ops/preflight-production-env.sh   # with the env of the deploy under review
```

(Not run as part of writing this document: the audit's change set is
edit-only by instruction; the commands above are for the reviewing pass.)
