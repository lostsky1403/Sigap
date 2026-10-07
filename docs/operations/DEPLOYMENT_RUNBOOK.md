# Sigap — Deployment & Rollback Runbook

**Phase:** 3B7 (production migration and deployment readiness)
**Applies to:** the Sigap redesign stream on branch `design/ui-ux-overhaul`
**Status:** readiness documentation only. **This runbook has not been executed. No deployment, merge, or production access has occurred.**

This runbook is the canonical cutover procedure for the redesign. It is written against
the repository as it actually is, not against a hypothetical platform. Where a fact lives
only outside the repository (a secret, a DNS record, a platform toggle), it is marked
**OPERATOR VERIFICATION REQUIRED** with the exact check to perform — never asserted.

---

## 1. Scope and non-negotiable rules

The redesign is a **code-only** change. It introduces:

- **no database migration** — `packages/db/migrations/0001`–`0010` are byte-identical across
  the entire redesign (`git diff 6d7f940..HEAD -- packages/db/migrations` is empty);
- **no new required service** — the compose topology is unchanged;
- **no URL rename** — every pre-existing route keeps its path.

Therefore: **no rollback in this stream ever requires a database restore.** Section 6
proves this claim rather than assuming it.

Rules that must hold at every step:

1. Deploy only from a clean tree at the exact release commit.
2. Never enable a dev-only capability outside `SIGAP_ENV=local`.
3. Never point a smoke run at production unless that deployment is separately and
   explicitly authorized.
4. Roll back by redeploying the previous image — never by editing an applied migration.

---

## 2. Pre-deploy

| # | Check | How | Required result |
|---|---|---|---|
| 2.1 | Release commit selected | `git rev-parse HEAD` | `git rev-parse HEAD` is the branch tip; the **deployable release commit is `c19c0a6`** (3B6, the last commit carrying a deployable artefact — 3B7 and later doc commits add docs/smoke only) |
| 2.2 | Branch | `git rev-parse --abbrev-ref HEAD` | `design/ui-ux-overhaul` |
| 2.3 | Working tree clean | `git status --porcelain` | Empty |
| 2.4 | Migration diff empty | `git diff --stat <pre-3B-anchor>..HEAD -- packages/db/migrations` | Empty (anchor: `6d7f940`) |
| 2.5 | Gate 6 evidence accepted | `RELEASE_CHECKLIST.md` §B | Full E2E + Go + Rust + web gates green |
| 2.6 | Build prerequisites present | `go version`, `cargo --version`, `protoc --version`, `node --version`, `pnpm --version`, **`docker compose version`** | All resolve; **Compose ≥ 2.24.4** (the overlays use `!override`, which older Compose cannot parse) |
| 2.7 | Production builds produce | `make build` (Go + Rust release + web) | All succeed |
| 2.8 | Environment configured | §5 matrix, every variable set + `sh scripts/ops/preflight-production-env.sh` | Script exits 0; no missing required variable. `SIGAP_ENV` must be explicit — the production overlay refuses unset/empty, and the script rejects `local` (any case) |
| 2.9 | Rollback target captured (OPERATOR) | §6 contract: record `CURRENT_PRODUCTION_VERSION` (commit or `/_app/version.json`), image digest/tag per service, and `SIGAP_DEPLOY_DIR` | All three recorded; **none is "unknown" and none equals the release commit**. If unavailable: **ABORT** — never treat the historical anchor `6d7f940` as current production |
| 2.10 | Deployment directory proven | `cd "$SIGAP_DEPLOY_DIR"` then `git rev-parse --show-toplevel` + `git status --porcelain` | Inside the compose repo, clean tree, expected branch/commit |
| 2.11 | Backup/rollback assumption | §6 | Previous images identified in 2.9 (not merely "pullable") |

**Backup assumption.** Because no migration ships, the rollback path is image-only. The
existing Postgres backup job (`deploy/systemd/sigap-postgres-backup.*`,
`docs/operations/BACKUP_RESTORE.md`) continues to run on its normal schedule; it is **not**
a prerequisite of this cutover, and it must not be treated as one.

---

## 3. Deploy order

Two distinct orders matter, and both are recorded here: the **phase** order (which redesign
phase may be deployed, and when) and the **service** order (the compose DAG within one deploy).

### 3a. Phase deployment sequence (plan §15.6)

The redesign was delivered as seven phases on one branch. They are **not** deployed as a
single cutover. Deploy in this order, verifying between phases:

| # | Phase | Deploy | Gate before proceeding |
|---|---|---|---|
| 1 | **3B0** | Deploy 3B0 **alone first** — it is the independent security release (backend authorization + proxies). Redeploy both API and web images. | Smoke-check admin reads and the citizen flows. This phase closes a P0 privilege-escalation path; deploy it on its own so a regression is attributable. |
| 2 | **3B1** | Additive only (web: 58 files; no `apps/api` change). May be bundled with 3B2 or deployed alone; **rebuild web only**. | Web gate green. |
| 3 | **3B2** | Citizen surface (Beranda, `/faskes` catalog). `apps/api` changes (catalog handler) → **rebuild api + web**. | `/` and `/faskes` render. |
| 4 | **3B3** | Citizen transactional surfaces (booking, check-in, `/queues/new`, patient status). Web-only (no `apps/api` change) → **rebuild web only**. Deploy **after** 3B2. | Citizen transactions pass against the seeded stack. |
| 5 | **3B4** | Admin shell and read surfaces (`apps/api`: 15 files — router, config guard, handlers) → **rebuild api + web**. Deploy **after** 3B3. | `/admin` renders; the six read destinations load. |
| 6 | **3B5** | Admin mutation surfaces (`apps/api`: 4 files — `cmd/server/main.go`, handlers) → **rebuild api + web**. Deploy **after** 3B4. | Admin mutations pass the actor-matrix E2E. |
| 7 | **3B6** | Dead-code removal. Web-only → **rebuild web only**. Deploy **only after the E2E suite is green in production-equivalent staging.** | Full E2E green on the staging stack. |
| 8 | **3B7** | **Documents the above; it adds no deployment.** | — |

**Prefer bundling the pairs** (3B1+3B2, and within a phase 3B2→3B3, 3B4→3B5) only when the
earlier phase's gate has already passed on the same commit range; never skip a phase's gate
to save a deploy. Each phase's rollback point is in §6.

### 3b. Service deploy order (the compose DAG)

Within any single deploy, the order is dictated by `docker-compose.yml`'s `depends_on` +
healthcheck graph. It is a strict DAG; do not reorder.

```
pg-cert-init  (one-shot, restart:"no")
      │  service_completed_successfully
      ▼
  postgres    (healthcheck: pg_isready -U sigap -d sigap)
      │  service_healthy
      ▼
rust-engine   (healthcheck: nc -z localhost 50051)
      │  service_started
      ▼
    api       (healthcheck: wget -qO- http://localhost:8080/health)
      │  depends_on (plain)
      ▼
    web       (no healthcheck)
      │
      ▼
edge / reverse proxy (Traefik labels on the web service; enabled by ENABLE_EDGE_ROUTING)
```

| Step | Service | Action | Verify |
|---|---|---|---|
| 3.0 | release identity (pre-deploy) | Record the current production version identifier now: `curl -fsS https://<host>/_app/version.json` (and `docker compose images api web` digests, once host access exists). This is the no-op/deploy detector for 3.8. | Value recorded; §2.9 captured |
| 3.1 | database / migrations | **Nothing to run.** No migration exists. If the target database has never been initialised, apply `packages/db/migrations/*.sql` in filename order once, as a separate, one-off operator action — **not** as part of this cutover. | `git diff` empty; existing DB already at `0010` |
| 3.2 | build images | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml build api web` (**`--build` is mandatory** — a bare `up -d` reuses the previously built images and deploys nothing, while still passing health checks) | build exits 0 for both services |
| 3.3 | postgres | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d postgres` (waits on `pg-cert-init`) | `pg_isready -h 127.0.0.1 -p 5433 -U sigap -d sigap` |
| 3.4 | queue engine | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d --build rust-engine` (rebuild only if the release diff touches `apps/queue-engine`; this stream never does) | `nc -z localhost 50051` |
| 3.5 | API | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d api` (image built in 3.2) | `GET /health` → 200; `GET /readyz` → ready |
| 3.6 | web | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml up -d web` (image built in 3.2) with `ENABLE_EDGE_ROUTING=true`, `EDGE_NETWORK`, `SIGAP_PUBLIC_HOST`, `EDGE_CERT_RESOLVER` set. **The edge overlay is required here**, not only in 3.6-old: without it, web republishes on plaintext `0.0.0.0:3005` and detaches from Traefik. | `GET /` → 200; Traefik router `sigap-web` + `sigap-web-secure` registered; certificate issued |
| 3.7 | reverse proxy / edge | No separate step — the edge network join happens in 3.6 via the overlay. Confirm routing and certificate per 3.6. | same as 3.6 |
| 3.8 | release identity (post-deploy) | `curl -fsS https://<host>/_app/version.json` must differ from the 3.0 value and match the deployed release; `docker compose images api web` digests must match the just-built images. **If the version is unchanged, the deploy was a no-op: STOP and investigate (§9).** | version changed to the expected release |
| 3.9 | post-deploy smoke | §4 | All checks pass |

> **Always pass BOTH overlays (steps 3.2–3.7).** Without `docker-compose.prod-ports.yml`, steps 3.3–3.5 publish
> Postgres, the engine and the API on every interface (5433, 50051, 8080) instead of loopback —
> the exact exposure that overlay exists to prevent. Without `docker-compose.prod-edge.yml`,
> step 3.6 republishes web on plaintext `0.0.0.0:3005` and detaches it from the Traefik network.
> Rollback (§7) uses the **same file set** — never a subset. `8080` is commonly taken by
> unrelated apps on this host.

> **Rebuild every service the release diff touches.** Compute from the diff, not the phase label:
> `git diff --name-only <rollback-target>..HEAD -- apps/api apps/web apps/queue-engine` — any path
> with changes means that service's image must be built in 3.2 (or in §7 for rollback).
> For this stream: 3B1/3B3/3B6 are web-only; 3B2/3B4/3B5 are api+web; the engine never changes
> (`0` `apps/queue-engine` files in every phase range). Never rebuild Postgres for a code change.

For a bare-metal / non-compose deployment, the same **logical** order applies: database →
engine → API → web → proxy. The API must not be started before the engine is listening
(`apps/api/cmd/server/main.go` builds its gRPC channel at startup and does not usefully
reconnect an early `Generate`).

`docker-compose.prod-ports.yml` rebinds postgres/engine/api to loopback
(`127.0.0.1:5433`, `127.0.0.1:50051`, `127.0.0.1:18080`); the edge overlay binds the web
container to `127.0.0.1:3005` and attaches the edge network. Use both overlays together for
an edge-fronted production deployment.

---

## 4. Post-deploy smoke verification

There are **two distinct** verification activities, and they are not interchangeable:

**4a. Pre-production rehearsal (this is what the smoke scripts run).**
Run `scripts/smoke/sigap-full-local-demo.ps1` (or the individual suites) against a
**production-equivalent LOCAL/staging stack** before the cutover. This is where the web-route
checks, the admin-mutation checks and the two P0 proofs execute. They **cannot** run against
a real production deployment: steps 9–16 need the local test identity selector, which the API
arms only under `SIGAP_ENV=local` + `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`, and
`GuardDevCapabilities` refuses to start the API with that flag outside `local`. They also
create and remove disposable probe rows/facilities. Section 9 of the abort list therefore
forbids this configuration in production — the rehearsal is a **pre**-production gate, by
design.

**4b. Production post-deploy verification (after the real cutover).**
The canonical list is plan §15.9 plus the operator checks in §11 of this runbook:

- no 5xx-rate increase attributable to the phase;
- the router's deny-by-default 401 rate unchanged (a spike indicates a proxy regression);
- no new 403s for scoped admin identities (an increase indicates an over-tightened scope fix);
- the Admin Antrean last-updated timestamp advances during polling;
- the operator config checks O1–O9 (§11).

For a real deployment, run only the individual unauthenticated probes by hand, e.g.
`curl -fsS https://<host>/` and `curl -fsS https://<host>/api/v1/public/facilities`.
`sigap-production-readiness-smoke.ps1` **cannot** be used against a real deployment: it
refuses any non-loopback target (exit 2) and it writes/removes probe rows.
See `scripts/smoke/README.md` for parameters and exit codes.

---

## 5. Environment variable matrix

Names, purpose, expected **production** state, and how it is validated. Values are never
recorded here — only names and expected state. `where_read` is the source of truth for
whether a variable is live.

### 5.1 Required — API (`docker-compose.yml` → `api`)

| Variable | Purpose | Expected production state | Read at |
|---|---|---|---|
| `SIGAP_ENV` | environment marker; drives all fail-fast guards | **NOT `local`** (recommend exactly `production`). The guard matches `local` CASE-INSENSITIVELY, so `Local`/`LOCAL` also count as local. | `cmd/server/main.go:146`, `internal/config/envguard.go:70,106` |
| `SIGAP_AUTH_MODE` | auth strategy | `jwt` (never `dev`) | `cmd/server/main.go:147`, `internal/auth/config.go:56` |
| `SIGAP_AUTH_ISSUER` | OIDC issuer (jwt mode) | set | `internal/auth/config.go:63` |
| `SIGAP_AUTH_AUDIENCE` | OIDC audience (jwt mode) | set | `internal/auth/config.go:64` |
| `SIGAP_AUTH_JWKS_URL` | JWKS endpoint (jwt mode) | set | `internal/auth/config.go:65` |
| `SIGAP_DATABASE_URL` | server DB connection | set | `cmd/server/main.go:200` |
| `SIGAP_ENGINE_ADDR` | gRPC engine target | `rust-engine:50051` | `cmd/server/main.go:170` |
| `SIGAP_API_PORT` | API listen port | `8080` | `cmd/server/main.go:159` |
| `SIGAP_WEB_ORIGIN` | CORS allow-list origin | the real public origin | `cmd/server/main.go:37` |
| `SIGAP_TLS_TERMINATED` | confirms TLS upstream | `true` when behind TLS | `internal/config/envguard.go:112` |
| `SIGAP_TRUSTED_PROXIES` | trusted proxy hops | `2` (edge + SvelteKit proxy) | `internal/router/proxy.go:48` |
| `SIGAP_GRPC_TLS` | TLS to the engine | **unset** in the compose topology — the engine serves plaintext on the internal network and the compose `api` service does not forward this variable. Set `true` only for a separately provisioned mTLS deployment with the CA mounted at `/etc/sigap/certs/ca.crt`; with it `true` and no CA, the API exits 1. | `internal/grpc/client.go:33` |

### 5.2 Required — Web (`docker-compose.yml` → `web`)

| Variable | Purpose | Expected production state | Read at |
|---|---|---|---|
| `SIGAP_ENV` | environment marker for web guards | **NOT `local`** | `apps/web/src/lib/server/auth.ts:93,96,167` |
| `SIGAP_API_INTERNAL` | server-side proxy target | `http://api:8080` | `apps/web/src/lib/server/auth.ts:182` + 5 proxy routes |
| `PUBLIC_SUPABASE_URL` | Supabase project URL (publishable) | set | `lib/server/auth.ts:27`, `lib/supabase/server.ts:26` |
| `PUBLIC_SUPABASE_ANON_KEY` | Supabase anon key (publishable) | set | `lib/server/auth.ts:27`, `lib/supabase/server.ts:27` |
| `SIGAP_TLS_TERMINATED` | no web-tier consumer; HSTS is emitted from the request protocol (`hooks.server.ts:49-51`), not this flag | set for consistency; the edge must terminate TLS for HSTS to apply | compose env |
| `ORIGIN` | public origin (CSRF/absolute URLs) | the real public origin | adapter-node runtime |

> **Never set** `PUBLIC_SUPABASE_SERVICE_ROLE_KEY` or any service-role key here. Only
> publishable values belong in the web environment.

### 5.3 Dev-only — must be ABSENT or false in production

| Variable | Dangerous value | Enforced by | Expected production state |
|---|---|---|---|
| `SIGAP_DEV_IDENTITY` | `true` | `internal/config/envguard.go:31` (API); `apps/web/src/lib/server/auth.ts:48,93` (web) | **unset or `false`** |
| `SIGAP_AUTH_MODE` | `dev` | `envguard.go:24` | `jwt` |
| `SIGAP_ENGINE_FALLBACK` | `dev` | `envguard.go:38` | unset |
| `SIGAP_LOCAL_RBAC_TEST_IDENTITY` | `true` | `envguard.go:51`; `internal/auth/local_test_identity_provider.go:67` | **unset** |
| `SIGAP_LOCAL_E2E_ACTOR` | any | `apps/web/src/lib/server/auth.ts:170` | **unset** |

The API refuses to start if any dev-only flag is set to its dangerous value while
`SIGAP_ENV` is not `local` (`GuardDevCapabilities`). The web tier refuses to inject a dev
header under the same condition.

> **Safety today rests on variable ABSENCE, not on the guard firing.** No compose file
> (`docker-compose.yml`, `.prod-ports`, `.prod-edge`) forwards `SIGAP_DEV_IDENTITY`,
> `SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR` or `SIGAP_ENGINE_FALLBACK` into
> any container, so in the compose topology these guards cannot even see a value. If a future
> change adds a pass-through line (e.g. `SIGAP_DEV_IDENTITY: ${SIGAP_DEV_IDENTITY:-}`), the
> guard becomes the only control — and `SIGAP_ENV` must be non-local for it to hold. The
> production overlay now forces an explicit `SIGAP_ENV` (§2.8) so that control stays live.

### 5.4 Optional / operational

| Variable | Purpose | Expected production state |
|---|---|---|
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | compose database credentials | set; password strong, never committed |
| `SIGAP_AUTO_MIGRATE` | run tracked migrations on startup | `false` (no migration ships); leave unset |
| `SIGAP_PUBLIC_HOST` | edge hostname (Traefik label only) | the real hostname |
| `ENABLE_EDGE_ROUTING`, `EDGE_NETWORK`, `EDGE_ENTRYPOINT_WEB`, `EDGE_ENTRYPOINT_SECURE`, `EDGE_CERT_RESOLVER` | edge routing | set for edge deployments |
| `SIGAP_BACKUP_*` | backup job | per `docs/operations/BACKUP_RESTORE.md` |
| `SIGAP_NOTIFICATION_WORKER_*` | outbox worker | per deployment need; manual run only |

> **Dead variables (recorded, not removed).** `PUBLIC_SIGAP_API_BASE` (set by compose `web` and
> `.env.example`) and `SIGAP_PUBLIC_HOST`-as-app-reader have **no consumer in `apps/`** — the
> former is a leftover compose/env value, the latter appears only in Traefik labels. They are
> harmless but misleading; see §10 for the disposition.

---

## 6. Rollback map

Rollback points use **real** commit hashes from this branch. No history was rewritten and
nothing was squashed.

**Anchor:** `6d7f940` — *"deploy: make production origin env-driven and route Sigap through
the shared edge (#83)"*. Verified as `git merge-base HEAD origin/main`; it is the last
known-good commit before any 3B work.

| Phase | Phase end commit | What changed | Rollback target (commit BEFORE the phase) | Expected behaviour after rollback | DB restore? |
|---|---|---|---|---|---|
| 3B0 | `6c80ba5` (GATE 1 record) | backend authorization (facility-scoped read/mutation provenance), proxy repairs, local seed identities | `6d7f940` | previous API **and** web images; P0 privilege-escalation path reopens — **prefer a forward fix** | **No** |
| 3B0.2 | `16268de` | read-authorization closure (docs + follow-up fixes) | `6c80ba5` | as 3B0 | **No** |
| 3B1 | `5b25e2c` | shared UI primitives + test tooling; additive, **web-only** | `16268de` | previous web image; no user-visible change | **No** |
| 3B2 | `80e285d` | citizen shell, Beranda, `/faskes` catalog, facility-type filter (`apps/api` touched → rebuild api **and** web) | `5b25e2c` | api **and** web images; `/faskes` disappears; Beranda reverts to the demo dashboard | **No** |
| 3B3 | `0f98da9` | citizen transactional surfaces: booking, check-in, `/queues/new`, patient status (web-only) | `80e285d` | previous web image; `/queues/new` becomes 404; transactional surfaces revert | **No** |
| 3B4 | `e566dcb` | admin shell + six read destinations (`apps/api` 15 files → rebuild api + web) | `0f98da9` | previous api **and** web images; `/admin` disappears | **No** |
| 3B5 | `0270bbd` (GATE 5) | admin mutation surfaces (`apps/api` 4 files → rebuild api + web) | `e566dcb` | previous api **and** web images; mutation affordances revert | **No** |
| 3B6 | `c19c0a6` | dead-code removal (legacy demo components, `maplibre-gl`), E2E stabilization, guards (web-only) | `0270bbd` | previous web image; deleted components return from git history | **No** |
| 3B7 | `8568a8d` | **documentation + smoke + ops guards** — no deployable artefact | `c19c0a6` | nothing to roll back; reverting the docs has no runtime effect | **No** |

> **"Phase end commit" ≠ "rollback target".** The **end** column names the commit at which the
> phase was *recorded complete* (often a docs commit). The **rollback** column names the commit
> *before* that phase's deployable work — verified with
> `git merge-base --is-ancestor <end> <next-phase-rollback-target>`; e.g. `5b25e2c` is the true
> 3B1 tip (one commit after the GATE-2 record `b1303aa`), and `16268de` is the 3B0.2 tip. When in
> doubt, the rollback target is always the parent of the phase's first deployable commit.

**Canonical rule, verified:** because the redesign introduced **no** database migration,
**no rollback in this stream requires a database restore.** Verification:

```
git diff --stat 6d7f940..HEAD -- packages/db/migrations   # empty
git diff --stat 6d7f940..0cc3f89 -- packages/db/migrations # empty
```

Rollback **triggers** (roll back the most recent phase): a 5xx-rate increase attributable to
the phase; a broken citizen critical path; an uncovered admin read/mutation regression; an
unexpected 403 increase for scoped admins; a missing `/faskes`, `/queues/new`, or `/admin`
after the phase that introduced it.

**Do not** roll back for the intended fixes: zero-assignment non-super_admin 404s, cross-
facility notification retry/cancel failures, or zero summary counts for a zero-assignment
actor — those are the fix, not a regression. If a global `super_admin` **loses** access,
that is a regression in the fix and must be corrected **forward**.

---

## 7. Rollback commands

**Rollback target contract (BLOCKING).** Before deploying, the operator must have recorded:

| Required value | Source | Reject if |
|---|---|---|
| `CURRENT_PRODUCTION_VERSION` | `curl -fsS https://<host>/_app/version.json`, or the deployed commit if repo-based | unset / "unknown" / equal to the release being deployed |
| `CURRENT_IMAGE_DIGEST` (api + web, and engine if it ever changes) | `docker compose images api web` on the host, or `docker inspect --format '{{.Image}}' <container>` | unavailable |
| `SIGAP_DEPLOY_DIR` | operator-selected compose directory (see §7a) | unset / not a compose repo |

If **any** of the three is unavailable: **ABORT before `build`/`up`.** The historical anchor
`6d7f940` is **not** the current production version — the deployed bundle predates it
(2026-09-08 web build vs the 2026-09-12 anchor). Never substitute the anchor for the real
rollback target.

### 7a. Directory + environment proof (before any build or up)

```bash
cd "$SIGAP_DEPLOY_DIR" || { echo "SIGAP_DEPLOY_DIR not set"; exit 1; }
git rev-parse --show-toplevel            # must be the compose repo
git rev-parse --abbrev-ref HEAD          # expected branch
git rev-parse HEAD                       # expected release commit
git status --porcelain                   # must be empty
docker compose version                   # >= 2.24.4 (the overlays use !override)
sh scripts/ops/preflight-production-env.sh   # exit 0 required
```

### 7b. Rollback commands

Same overlay set as deploy — **`docker-compose.yml` + `docker-compose.prod-ports.yml` +
`docker-compose.prod-edge.yml`**, never a subset. Omitting the edge overlay detaches `web`
from the Traefik network (public HTTPS routing loses its server) while republishing it on
plaintext `0.0.0.0:3005`.

```bash
# 1. Code rollback — check out the §6 rollback target (or pin image tags if a registry exists).
git fetch --all --tags
git checkout <PHASE_ROLLBACK_TARGET_FROM_SECTION_6>

# 2. Rebuild + restart exactly the services the rollback diff touches
#    (git diff --name-only <rollback-target>..HEAD -- apps/api apps/web apps/queue-engine).
#    Same overlay set as deployment - BOTH overlays, edge included.
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  build api web
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  up -d postgres rust-engine api web

# 2b. Service rebuild scope by phase (mirror of §3a; never Postgres):
#   web-only: 3B1, 3B3, 3B6            -> build web
#   api+web: 3B0, 3B2, 3B4, 3B5        -> build api web
#   queue engine: never in this stream -> nothing to rebuild

# 3. Edge sanity — a rollback must NOT detach web from Traefik or expose plaintext 3005.
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  config | grep -E 'published: "3005"|host_ip: 127.0.0.1'   # web must be 127.0.0.1:3005 only
curl -fsS https://<SIGAP_PUBLIC_HOST>/ > /dev/null         # edge routing still serves web

# 4. Health verification
curl -fsS http://127.0.0.1:18080/health     # API
curl -fsS http://127.0.0.1:18080/readyz     # API readiness (engine reachable)
curl -fsS http://127.0.0.1:3005/            # web (loopback)
# AND through the edge (a loopback-only check would miss a broken Traefik route):
curl -fsS https://<host>/                   # public web
curl -fsS https://<host>/api/v1/public/facilities   # through the proxy

# 5. Release identity — /_app/version.json must now equal the ROLLBACK target's build.
curl -fsS https://<host>/_app/version.json   # must differ from the deployed release you rolled back FROM

# 6. Rehearsal smoke - REHEARSAL stack ONLY (the readiness script refuses non-loopback
#    targets and writes probe rows; see 4a). On a compose deployment the web port is 3005;
#    on the bare-metal E2E stack it is 4173. Requires the local selector (SIGAP_ENV=local).
$env:SIGAP_API_BASE = 'http://127.0.0.1:18080'
$env:SIGAP_WEB_BASE = 'http://127.0.0.1:3005'
$env:DATABASE_URL  = 'postgresql://sigap:<password>@127.0.0.1:5433/sigap?sslmode=require'
$env:SIGAP_ENV = 'local'
$env:SIGAP_LOCAL_RBAC_TEST_IDENTITY = 'true'   # arms the local selector the P0 steps need
pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1 -SkipSeed
```

**Database restore: NOT REQUIRED.** Do not run `scripts/ops/Restore-Postgres.*` as part of
a redesign rollback; that tooling exists for disaster recovery, not phase rollback. If a
restore is ever proposed, stop — the premise of this runbook (no migration) has been
violated and the change must be re-reviewed.

---

## 8. Smoke checks

`scripts/smoke/` provides the pre-production verification. Conventions (see
`scripts/smoke/README.md`): PowerShell 7+, no framework, exit `0` pass / `1` assertion or
transport failure / `2` parameter or precondition failure. Target via `-ApiBase` or
`$env:SIGAP_API_BASE` (default `http://127.0.0.1:8080`); web routes via `-WebBase` or
`$env:SIGAP_WEB_BASE` (default `http://127.0.0.1:4173`).

`sigap-production-readiness-smoke.ps1` covers the web routes, the public catalog, the two
admin mutations, and the two P0 proofs. Its assertions are proven non-vacuous by the negative
controls recorded in `RELEASE_CHECKLIST.md` §G (not §9 — §9 is the abort list).

### 8a. Read-only preflight policy (no side effects)

A read-only preflight must NOT issue any request that can write. Classify every operation:

| Class | Examples | May run in a read-only preflight? |
|---|---|---|
| **Pure observation** | `docker ps`/`inspect`, `docker compose config`/`images`, env presence, process health, image digest/tag, read-only config/log reads, `curl` of a **public, unauthenticated** route (`/`, `/api/v1/public/facilities`) | **Yes** |
| **Application request with a side effect via audit** | any **RBAC-denied protected-route probe** — e.g. `GET /api/v1/admin/facilities`, `/api/v1/admin/notifications/summary` — the Go API writes an `audit_events` row through `logAuthzDenied` (`internal/identity/authz.go` → `audit.Service.LogEvent`) even though the request is refused | **NO — never during read-only preflight** |
| **Mutating** | deploy, restart, migrate, seed, any POST/PATCH/DELETE | **No** |

Consequence: during a read-only preflight, inspect the API's identity safety from the
**environment and config** (class 1), not by probing admin routes (class 2). Probing those
routes writes append-only audit rows to production — the failure this policy exists to prevent.
Identity safety is instead established by reading the deployed env (via the operator evidence
bundle, §11a) and by the structural fact that no compose file forwards any dev-only flag.

**Business-data presence is a separate axis from service health.** The public catalog
returning zero facilities is a data-presence observation, not a health failure (see §9).

### 8b. P0 post-deploy smoke — side-effect policy

The two P0 proofs are **post-deploy controlled smoke tests**, not read-only preflight:

| Aspect | Policy |
|---|---|
| Setup | Runs on the **production-equivalent local/staging** stack (§4a) with `SIGAP_ENV=local` + `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`; it creates two `notification_outbox` probe rows, one probe facility, and one walk-in ticket |
| Expected side effects | Disposable probe rows/facility (removed in the script's `finally` block); `audit_events` rows for the denied-retry attempts (append-only, **expected**) |
| Cleanup policy | The script removes its own probe fixtures. **Never delete append-only `audit_events` evidence** to make a test look clean — audit rows are the record, not residue |
| Audit-event expectation | Denied retries legitimately produce audit rows; their presence is correct behaviour |
| Abort conditions | Any assertion failure (404 not returned, row changed, summary non-zero for a zero-assignment actor, or the positive control failing) |
| Where it may run | **Only** the rehearsal stack, and only after explicit deployment authorization for the production smoke variant |

---

## 9. Failure / abort conditions

**Pre-deploy aborts (must hold BEFORE any `build` or `up`):**

- `SIGAP_DEPLOY_DIR` unset, or the working directory is not the compose repo (§7a);
- the release commit is not the intended one, or the tree is dirty;
- `docker compose version` < 2.24.4 (the overlays use `!override`);
- the rollback target (§7 contract) is unknown, unset, or equals the release being deployed;
- a required environment variable (§5.1–5.2) is missing;
- `SIGAP_ENV` unset/empty, or `local` in the target environment (any case-variant — the guard matches `local` case-insensitively); the production overlay itself refuses unset/empty;
- `SIGAP_AUTH_MODE` is missing, `dev`, `disabled`, or any unknown value (canonical modes from `apps/api/internal/auth/config.go`: `dev`, `jwt`, `disabled`; production requires `jwt`. `disabled` boots but makes every admin route 403 — a dead deployment, indistinguishable from a broken one);
- `SIGAP_DEV_IDENTITY` is enabled, or `SIGAP_LOCAL_RBAC_TEST_IDENTITY` / `SIGAP_LOCAL_E2E_ACTOR` is set;
- the local identity selector is active (`SIGAP_ENV=local` + a subject);
- `make db-seed` would **not** refuse (the demo-seed guard is bypassed);
- a migration appears in the diff (the premise is broken);
- the required compose overlay set differs from the runbook's (both `prod-ports` AND `prod-edge` for an edge-fronted deploy);
- the Compose version is below the required minimum;
- the expected compose files are missing from the host deployment directory;
- the rollback target / image digest / deploy directory (§7 contract) is not recorded.

**Post-deploy rollback triggers (evaluate only after the cutover):**

- any production build fails;
- the release identity (`/_app/version.json`) did not change after the deploy (a no-op deploy) or does not match the release;
- any smoke check fails;
- an out-of-scope notification retry **writes** (row changed) instead of returning 404;
- a zero-assignment actor's notification summary leaks non-zero counts;
- a critical route is unavailable (`/`, `/faskes`, `/queues/new`, `/admin`);
- a 5xx-rate increase attributable to the phase;
- web is reachable on plaintext `0.0.0.0:3005` or the Traefik router is unregistered (edge overlay missing);
- the smoke target cannot be distinguished from production.

**Data presence is NOT a service-health abort.** An empty public catalog
(`/api/v1/public/facilities` → `data: []`/`null`) means the deployment is *serving* correctly
with no seeded business data. It is a **business-data presence** finding, recorded and raised
with the product owner — not a rollback trigger. Service health (routes 200, no 5xx spike,
authz failing closed) and data presence are verified separately; only service health aborts.

---

## 10. Known residuals (recorded, not silently fixed)

| Residual | Disposition | Why not fixed here |
|---|---|---|
| Dead public API: `/api/v1/facilities/nearby`, `/api/v1/events/beds` | post-redesign technical debt — see `docs/operations/DEAD_PUBLIC_API_ANALYSIS.md` | outside T-3B7 scope; changing the Go router is not required for readiness |
| `PUBLIC_SIGAP_API_BASE`, `SIGAP_PUBLIC_HOST` have no app reader | post-release config cleanup | cosmetic; removing compose keys is a separate change |
| CI `cargo clippy` non-blocking (`|| true`) | post-release CI hardening | clippy is clean locally; making it blocking is a CI policy change, not a readiness blocker |
| CI `web` job runs `check` only, not `test`/E2E | post-release CI hardening | WEBTEST passes locally; adding an E2E CI job needs a hosted Postgres + engine, a separate task |
| `/wallet` route retained, excluded from navigation (decision D7) | **intentional deferred route** — not an accidental orphan | canonical D7 defers wallet deletion beyond the redesign |
| `apps/api/internal/auth/local_test_identity_provider.go` cites a defence that does not exist | recorded; **not changed** (pre-3B7 Go comment) | the comment claims `proxy.go` drops `X-Sigap-*` headers when `SIGAP_ENV != local`; it does not. Safety is unaffected — the selector checks `Armed()` (exact `SIGAP_ENV=local`) before reading the header, and `GuardDevCapabilities` refuses the boot otherwise. Fix the comment when Go is next touched; keeping the Go diff empty is a property of this phase. |
| `SIGAP_ENV` guard is case-insensitive | documented in §5.1/§5.3 and the abort list | making the guard case-sensitive would change behaviour pinned by `envguard_test.go`; out of scope for a readiness phase |

---

## 11. Operator verification required

These cannot be established from the repository. Perform each during the deployment and
record the result in `docs/operations/RELEASE_CHECKLIST.md`.

| # | Check | Exact command / action | Pass condition |
|---|---|---|---|
| O1 | Production `SIGAP_DEV_IDENTITY` | Inspect the deployed API/web environment | unset or `false` |
| O2 | Production `SIGAP_ENV` | Inspect the deployed environment | not `local` |
| O3 | `SIGAP_AUTH_MODE` | Inspect the deployed API environment | `jwt` |
| O4 | Supabase public vars present | Inspect the deployed web environment | `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY` set; no service-role key |
| O5 | `SIGAP_API_INTERNAL` | Inspect the deployed web environment | resolves to the API |
| O6 | Local identity path inactive | Confirm no `SIGAP_LOCAL_E2E_ACTOR` / `SIGAP_LOCAL_RBAC_TEST_IDENTITY` | both unset |
| O7 | API boot did not print a dev-flag guard error | `docker compose logs api \| grep -i 'dev-only capabilities'` | no match |
| O8 | Edge certificate issued | Traefik dashboard / `openssl s_client` | valid cert for the public host |
| O9 | Smoke target is non-production | Confirm `SIGAP_API_BASE`/`SIGAP_WEB_BASE` during rehearsal | loopback only |
| O10 | Compose version on the host | `docker compose version` | ≥ 2.24.4 |
| O11 | Deployed release identity | `curl -fsS https://<host>/_app/version.json` (pre-deploy) | recorded as `CURRENT_PRODUCTION_VERSION`; not "unknown" |
| O12 | Deployment directory | `pwd` inside `$SIGAP_DEPLOY_DIR` | recorded; a compose repo |

### 11a. Operator evidence bundle (read-only host access)

The next preflight requires **one** safe read-only access path to the host. Either is
acceptable — do not build new remote-management tooling:

1. **SSH** (interactive or one-shot), or
2. **A documented operator-generated read-only dump**: the operator runs
   `docs/operations/OPERATOR_EVIDENCE_BUNDLE.md` on the host and returns the output.

The bundle must establish, without leaking secrets: deployment directory, compose version,
compose files in use, container status, image IDs/digests, `SIGAP_ENV` (exact non-secret
value), `SIGAP_DEV_IDENTITY` (safe boolean), `SIGAP_AUTH_MODE` (non-secret mode), relevant
port bindings, network membership, reverse-proxy labels, and the release identifier. Secret
variables are reported as PRESENT/ABSENT only.

> **Why this is required:** SSH from the prior workstation TCP-connected but never sent a
> banner, and the local Docker engine was down, so no host fact could be read. Production was
> observable only over HTTPS — which cannot read env, containers, image digests, or the
> deployment directory, and cannot probe admin routes without writing audit rows (§8a).
