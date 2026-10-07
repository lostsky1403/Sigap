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
| 2.1 | Release commit selected | `git rev-parse HEAD` | The intended commit (currently `c19c0a6`) |
| 2.2 | Branch | `git rev-parse --abbrev-ref HEAD` | `design/ui-ux-overhaul` |
| 2.3 | Working tree clean | `git status --porcelain` | Empty |
| 2.4 | Migration diff empty | `git diff --stat <pre-3B-anchor>..HEAD -- packages/db/migrations` | Empty (anchor: `6d7f940`) |
| 2.5 | Gate 6 evidence accepted | `RELEASE_CHECKLIST.md` §B | Full E2E + Go + Rust + web gates green |
| 2.6 | Build prerequisites present | `go version`, `cargo --version`, `protoc --version`, `node --version`, `pnpm --version` | All resolve |
| 2.7 | Production builds produce | `make build` (Go + Rust release + web) | All succeed |
| 2.8 | Environment configured | §5 matrix, every variable set | No missing required variable |
| 2.9 | Backup/rollback assumption | §6 | Previous images still pullable |

**Backup assumption.** Because no migration ships, the rollback path is image-only. The
existing Postgres backup job (`deploy/systemd/sigap-postgres-backup.*`,
`docs/operations/BACKUP_RESTORE.md`) continues to run on its normal schedule; it is **not**
a prerequisite of this cutover, and it must not be treated as one.

---

## 3. Deploy order

The order is dictated by `docker-compose.yml`'s `depends_on` + healthcheck graph. It is a
strict DAG; do not reorder.

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
| 3.1 | database / migrations | **Nothing to run.** No migration exists. If the target database has never been initialised, apply `packages/db/migrations/*.sql` in filename order once, as a separate, one-off operator action — **not** as part of this cutover. | `git diff` empty; existing DB already at `0010` |
| 3.2 | postgres | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d postgres` (waits on `pg-cert-init`) | `pg_isready -h 127.0.0.1 -p 5433 -U sigap -d sigap` |
| 3.3 | queue engine | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d rust-engine` | `nc -z localhost 50051` |
| 3.4 | API | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d api` | `GET /health` → 200; `GET /readyz` → ready |
| 3.5 | web | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d web` | `GET /` → 200 |
| 3.6 | reverse proxy / edge | Attach the edge network and enable routing: `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml up -d web` with `ENABLE_EDGE_ROUTING=true`, `EDGE_NETWORK`, `SIGAP_PUBLIC_HOST`, `EDGE_CERT_RESOLVER` set. | Traefik router `sigap-web` + `sigap-web-secure` registered; certificate issued |
| 3.7 | post-deploy smoke | §4 | All checks pass |

> **Always pass BOTH overlays.** Without `docker-compose.prod-ports.yml`, steps 3.2–3.5 publish
> Postgres, the engine and the API on every interface (5433, 50051, 8080) instead of loopback —
> the exact exposure that overlay exists to prevent. The overlay's own header mandates it, and
> `8080` is commonly taken by unrelated apps on this host.

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
| `PUBLIC_SUPABASE_URL` | Supabase project URL (publishable) | set | `lib/server/auth.ts:27`, `lib/server/supabase/server.ts:26` |
| `PUBLIC_SUPABASE_ANON_KEY` | Supabase anon key (publishable) | set | `lib/server/auth.ts:27`, `lib/server/supabase/server.ts:27` |
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

| Phase | Ends at | What changed | Safe rollback to | Expected behaviour after rollback | DB restore? |
|---|---|---|---|---|---|
| 3B0 | `6c80ba5` (GATE 1) | backend authorization (facility-scoped read/mutation provenance), proxy repairs, local seed identities | `6d7f940` | previous API **and** web images; P0 privilege-escalation path reopens — **prefer a forward fix** | **No** |
| 3B1 | `b1303aa` (GATE 2) | shared UI primitives + test tooling; additive | `16268de` | previous web image; no user-visible change | **No** |
| 3B2 + 3B3 | `0f98da9` | citizen shell, `/faskes`, `/queues/new`, transactional citizen surfaces | `5b25e2c` | previous web image; `/faskes` and `/queues/new` disappear; Beranda reverts | **No** |
| 3B4 + 3B5 | `0270bbd` (GATE 5) | admin shell + read/mutation surfaces | `0f98da9` | previous web image; `/admin` disappears | **No** |
| 3B6 | `c19c0a6` (this phase) | dead-code removal (legacy demo components, `maplibre-gl`), E2E stabilization, guards | `0270bbd` | previous web image; deleted components return from git history | **No** |

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

Assumes the compose deployment. Replace `<PREVIOUS_TAG_OR_COMMIT>` with the image tag for
the phase's rollback commit in §6.

```bash
# 1. Code rollback — redeploy the previous images (no git history change required).
git fetch --all --tags
git checkout <PREVIOUS_TAG_OR_COMMIT>      # or pin image tags in your registry

# 2. Rebuild + restart only what changed (web-only for 3B1..3B6; api+web for 3B0).
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml build web
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d web
# For a 3B0 rollback, also rebuild/restart the API:
# docker compose ... build api && docker compose ... up -d api

# 3. Health verification
curl -fsS http://127.0.0.1:18080/health     # API
curl -fsS http://127.0.0.1:18080/readyz     # API readiness (engine reachable)
curl -fsS http://127.0.0.1:3005/            # web

# 4. Smoke verification — REHEARSAL stack only (the readiness script refuses
#    non-loopback targets and writes probe rows; see §4a). On a compose
#    deployment the web port is 3005; on the bare-metal E2E stack it is 4173.
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

---

## 9. Failure / abort conditions

Abort the deployment (do not proceed; roll back if already partially applied) if **any**:

- the release commit is not the intended one, or the tree is dirty;
- a required environment variable (§5.1–5.2) is missing;
- `SIGAP_ENV=local` in the target environment (including any case-variant — the guard matches `local` case-insensitively);
- `SIGAP_DEV_IDENTITY` is enabled, or `SIGAP_LOCAL_RBAC_TEST_IDENTITY` / `SIGAP_LOCAL_E2E_ACTOR` is set;
- the local identity selector is active (`SIGAP_ENV=local` + a subject);
- `make db-seed` would **not** refuse (the demo-seed guard is bypassed);
- a migration appears in the diff (the premise is broken);
- any production build fails;
- any smoke check fails;
- an out-of-scope notification retry **writes** (row changed) instead of returning 404;
- a zero-assignment actor's notification summary leaks non-zero counts;
- a critical route is unavailable;
- the smoke target cannot be distinguished from production;
- the rollback commit for the phase is unavailable.

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
