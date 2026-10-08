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

The redesign is a **code-only** change *relative to its own work*. Migration provenance is
subtler than "no migration", and the six statements below must not be collapsed into one:

| # | Statement | How to verify |
|---|---|---|
| **A** | The redesign **created no migration** of its own. | No `packages/db/migrations/*` file was added or edited by a redesign commit. |
| **B** | The release's migration directory is **byte-identical to current `origin/main`**. | `git diff --stat origin/main..HEAD -- packages/db/migrations` → **empty**. |
| **C** | The diff against the **historical anchor `6d7f940` is NON-EMPTY**. | `git diff --stat 6d7f940..HEAD -- packages/db/migrations` → `0006_notifications.sql \| 4 ++--`. `origin/main` commit `9d4e68e` **hardened** an already-shipped migration (added a formatted-phone predicate to two `notification_outbox` CHECK constraints). It reached this branch through the `origin/main` merge (`6c0ac82`), **not** through redesign work. |
| **D** | **Production DB constraints may not match the hardened source migration.** | Requires DB metadata inspection (§12). Not derivable from the repository. |
| **E** | The migrator is **version-only**: it does **not** reapply a modified, already-applied migration. | `apps/api/internal/migrate/migrate.go` selects pending work by version number and never re-reads the stored checksum; production `SIGAP_AUTO_MIGRATE` is empty. |
| **F** | **Production DB schema compatibility is UNKNOWN** until §12 completes. | — |

Consequences that follow directly:

- **Never claim migration status PASS from repository diff evidence alone.** A clean
  `origin/main..HEAD` diff (**B**) says the *source* agrees with main; it says nothing about
  the *database* (**D**/**F**).
- **Do not edit `0006` again.** It is shipped. If a correction is needed it is a **new,
  forward-only** migration (§12).
- A **non-empty** `6d7f940..HEAD` diff (**C**) is *expected and classified*: an inherited
  upstream security hardening, not redesign drift. Record it; do not abort on it, and do not
  "fix" it by reverting the hardening.

A **rollback** in this stream does not require a database restore — nothing in the redesign
adds schema. That is a statement about the *rollback path*; it does **not** prove production
schema compatibility (**D**/**F**, §12).

Rules that must hold at every step:

1. Deploy only from a clean tree at the exact release commit.
2. Never enable a dev-only capability outside `SIGAP_ENV=local`.
3. Never point a smoke run at production unless that deployment is separately and
   explicitly authorized.
4. Roll back by restoring the **preserved, currently running image** (§6/§7) — never by
   checking out an old design-phase commit, and never by editing an applied migration.

---

## 2. Pre-deploy

| # | Check | How | Required result |
|---|---|---|---|
| 2.1 | Release commit selected | `git rev-parse HEAD` | The branch tip. Record it as `RELEASE_CANDIDATE_HEAD` (§6). |
| 2.2 | Branch | `git rev-parse --abbrev-ref HEAD` | `design/ui-ux-overhaul` |
| 2.3 | Working-tree policy | `git status --porcelain` | **Tracked** paths clean. Untracked paths are classified per §2a — an expected sensitive local artifact does **not** fail the gate, but it must be recorded with an explicit disposition. **Never run `git clean`.** |
| 2.4 | Migration provenance classified | §1 (**A**–**F**) | **B** empty; **C** recorded as inherited upstream hardening. A migration PASS may **not** be claimed from these diffs alone — **F** stays UNKNOWN until §12. |
| 2.5 | Gate 6 evidence accepted | `RELEASE_CHECKLIST.md` §B | Full E2E + Go + Rust + web gates green |
| 2.6 | Build prerequisites present | `go version`, `cargo --version`, `protoc --version`, `node --version`, `pnpm --version`, **`docker compose version`** | All resolve; **Compose ≥ 2.24.4** (the overlays use `!override`) |
| 2.7 | Production builds produce | `make build` (Go + Rust release + web) | All succeed |
| 2.8 | Environment configured | §5 matrix + `sh scripts/ops/preflight-production-env.sh` | Script exits 0; no missing required variable. `SIGAP_ENV` explicit — the production overlay refuses unset/empty and the script rejects `local` (any case) |
| 2.9 | Current production image IDs captured (OPERATOR) | `docs/operations/OPERATOR_EVIDENCE_BUNDLE.md` §4, by **exact name** | `CURRENT_PRODUCTION_IMAGE_ID` recorded **per service** (full `sha256:` id), plus `CURRENT_PRODUCTION_VERSION` and `SIGAP_DEPLOY_DIR`. **None may be "unknown".** |
| 2.9b | Rollback images PRESERVED | §7c (preserve) + §7d (restore) | Immutable refs created from the running image IDs; archives checksummed; **restore path verified**. **Blocking — see §13.** |
| 2.9c | DB schema inspection approved + completed | §12 | Result classified; drift disposition recorded. **Blocking** for any claim about schema compatibility. |
| 2.10 | Deployment directory proven | `cd "$SIGAP_DEPLOY_DIR"`, `git rev-parse --show-toplevel`, `git status --porcelain --untracked-files=no` | Inside the compose repo; **tracked** clean; untracked classified per §2a |
| 2.11 | Disk headroom sufficient | §13 | Rollback archives present **and verified**; build peak evaluated; **never** run an automated `prune`/`rmi` before preservation is verified |
| 2.12 | Backup/rollback assumption | §6 | Previous images identified (2.9) **and preserved** (2.9b) — not merely "pullable" |

### 2a. Working-tree policy (do not weaken this to "ignore all untracked files")

The production deploy tree contains a known, sensitive, untracked local artifact
(`.env.bak-phase5`). A gate that demands a byte-empty `git status --porcelain` pressures an
operator to **delete secret evidence** to pass; a gate that ignores *all* untracked files
would let a stray credential or a stray build artefact through. Neither is acceptable.

The rule is therefore three-part:

1. **Tracked source must be clean.**
   `git status --porcelain --untracked-files=no` must be **empty**. Any modification to a
   tracked file is a **hard abort**.
2. **Unexpected untracked files block.** Any untracked path that is *not* on the classified
   list below is a **hard abort**.
3. **Classified untracked artifacts are recorded, not deleted.** Each must appear in the
   deployment record with an explicit operator disposition.

| Classified untracked path | Nature | Required disposition |
|---|---|---|
| `/opt/sigap/.env.bak-phase5` | Sensitive. Mode `600`, 449 bytes, untracked **and not git-ignored**. | Retain in place. Record owner/mode/size/mtime. **Never** stage, move, delete or `git clean`. Resolve its long-term retention separately (§14). |

**The production tree must continue to be reported as *not fully clean*** until that
disposition is resolved. Do not fabricate a clean-tree PASS. Note that ignoring a path in
`.gitignore` (§14) does **not** make the host clean — it only prevents accidental staging.

**Never** run `git clean`, `git reset --hard`, or `git checkout -- .` on the deploy tree.

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
| 2 | **3B1** | Additive only (web: 59 files; no `apps/api` change). May be bundled with 3B2 or deployed alone; **rebuild web only**. | Web gate green. |
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
| 3.1 | database / migrations | **Nothing to RUN — but the schema is not thereby verified.** No *new* migration ships (§1 **A**/**B**) and production `SIGAP_AUTO_MIGRATE` is empty, so this cutover applies no DDL. The inherited hardening of `0006` (§1 **C**) **will not** be reapplied by the version-only migrator (§1 **E**), so live constraints must be checked per **§12** before any compatibility claim. If the database has never been initialised, apply `packages/db/migrations/*.sql` in filename order once as a separate one-off action — **not** part of this cutover. | `git diff origin/main..HEAD -- packages/db/migrations` empty; **§12 result recorded** |
| 3.2 | build images | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml build api web` (**`--build` is mandatory** — a bare `up -d` reuses the previously built images and deploys nothing, while still passing health checks) | build exits 0 for both services |
| 3.3 | postgres | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d postgres` (waits on `pg-cert-init`) | `pg_isready -h 127.0.0.1 -p 5433 -U sigap -d sigap` |
| 3.4 | queue engine | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d rust-engine` — rebuild **only** if `git diff --name-only <release-base>..RELEASE_CANDIDATE_HEAD -- apps/queue-engine` is non-empty (never in this stream). `<release-base>` is the commit the **currently deployed** images were built from — the value recorded in 3.0, not a historical design-phase pointer (§6a). | `nc -z localhost 50051` |
| 3.5 | API | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml up -d api` (image built in 3.2) | `GET /health` → 200; `GET /readyz` → ready |
| 3.6 | web | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml up -d web` (image built in 3.2) with `ENABLE_EDGE_ROUTING=true`, `EDGE_NETWORK`, `SIGAP_PUBLIC_HOST`, `EDGE_CERT_RESOLVER` set. **The edge overlay is required here**: without it, web republishes on plaintext `0.0.0.0:3005` and detaches from Traefik. | `GET /` → 200; Traefik router `sigap-web` + `sigap-web-secure` registered; certificate issued |
| 3.7 | reverse proxy / edge | No separate step — the edge network join happens in 3.6 via the overlay. Confirm routing and certificate per 3.6. | same as 3.6 |
| 3.8 | release identity (post-deploy) | If this deploy rebuilt web: `curl -fsS https://<host>/_app/version.json` must differ from the 3.0 value. Always: `docker compose images api web` digests must match the just-built images for every service rebuilt in 3.2. **If a rebuilt service's identity is unchanged, the deploy was a no-op: STOP and investigate (§9).** | version/digest changed for every rebuilt service |
| 3.9 | post-deploy smoke | §4 | All checks pass |

> **Always pass BOTH overlays (steps 3.2–3.7).** Without `docker-compose.prod-ports.yml`, steps 3.3–3.5 publish
> Postgres, the engine and the API on every interface (5433, 50051, 8080) instead of loopback —
> the exact exposure that overlay exists to prevent. Without `docker-compose.prod-edge.yml`,
> step 3.6 republishes web on plaintext `0.0.0.0:3005` and detaches it from the Traefik network.
> Rollback (§7) uses the **same file set** — never a subset. `8080` is commonly taken by
> unrelated apps on this host.

> **Rebuild every service the release diff touches.** Compute from the diff, not the phase label:
> `git diff --name-only <release-base>..RELEASE_CANDIDATE_HEAD -- apps/api apps/web apps/queue-engine` — any path
> with changes means that service's image must be built in 3.2 (or in §7 for rollback).
> For this stream: 3B1/3B3/3B6 are web-only; 3B2/3B4/3B5 are api+web; the engine never changes
> (`0` `apps/queue-engine` files in every phase range). Never rebuild Postgres for a code change.
> `<release-base>` is the deployed image's source commit (3.0), **not** a §6b historical pointer.

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

### 6a. Terminology — these are DIFFERENT concepts

| Term | Meaning |
|---|---|
| `HISTORICAL_PHASE_COMMIT` | A commit at which a redesign phase was recorded complete. |
| `HISTORICAL_PHASE_ROLLBACK_POINTER` | The commit *before* that phase's deployable work. **A development-history pointer only — NOT a production rollback target.** |
| `CURRENT_PRODUCTION_IMAGE_ID` | The exact `sha256:` image ID of each container **currently running** in production. This is the real rollback target. |
| `PRESERVED_ROLLBACK_IMAGE_REF` | An immutable, release-specific tag on a preserved production image, e.g. `sigap-api:rollback-2026-09-14-43fcbcb`. |
| `RELEASE_CANDIDATE_HEAD` | The commit being deployed. |

**The historical pointers in §6b are NOT production rollback targets.** Every one of them
predates the `origin/main` security reconciliation (merge `6c0ac82`), which brought in four
security commits — `96c2570` (remove the enumerable `formatted_number` lookup), `9d4e68e`
(harden the notification denylist), `6f567f8` (restrict dev identity to read-only),
`87f218e` (template rendering). `git merge-base --is-ancestor 87f218e <target>` is **false
for all of them**, so **checking out a §6b pointer would silently revert live production
security fixes.**

> **Rule: never roll production back to a historical design-phase commit.** Production
> rollback means restoring the **preserved, currently running image** — see §7c/§7d.

### 6b. Historical phase history (development record, not a rollback target)

These hashes are retained to explain how the branch was built. They are **not** deployable
rollback targets for production.

**Anchor:** `6d7f940` — *"deploy: make production origin env-driven and route Sigap through
the shared edge (#83)"*. It is the **merge-base with `origin/main`** and an **ancestor** of
it, but it is now **5 commits behind** it:

```
git merge-base --is-ancestor 6d7f940 origin/main    # exit 0  — 6d7f940 IS an ancestor
git rev-list --count 6d7f940..origin/main           # 5
git merge-base --is-ancestor 87f218e 6d7f940        # exit 1  — the security fixes are NOT in it
```

So the operative fact is **not** that `6d7f940` is unrelated to `origin/main` — it is that
the five commits above it (`920e967`, `6f567f8`, `9d4e68e`, `96c2570`, `87f218e`) are
**descendants** of it. Checking it out therefore **loses those five commits**, four of which
are security fixes. That is why it is not a production rollback target.

| Phase | Phase end commit | What changed | Historical pointer (commit BEFORE the phase) | Expected behaviour if that pointer were deployed | DB restore? |
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

> **"Phase end commit" ≠ "historical pointer".** The **end** column names the commit at which
> the phase was *recorded complete* (often a docs commit). The **pointer** column names the
> commit *before* that phase's deployable work — verified with
> `git merge-base --is-ancestor <end> <next-phase-pointer>`; e.g. `5b25e2c` is the true 3B1
> tip (two commits after the GATE-2 record `b1303aa`), and `16268de` is the 3B0.2 tip.

### 6c. Database-restore rule

No rollback in this stream requires a **database restore**, because the redesign adds no
schema: the only migration change in range is an **inherited upstream constraint hardening**
(§1 **C**). That does **not** mean the migration diff is empty for the historical anchor:

```
git diff --stat origin/main..HEAD -- packages/db/migrations   # EMPTY  (release source == main)
git diff --stat 6d7f940..HEAD     -- packages/db/migrations   # 0006_notifications.sql | 4 ++--
```

The second command is **non-empty by design** and is expected; see §1 (**C**). Neither
command proves anything about the **deployed database** — that is §12, and until it runs,
`DATABASE_SCHEMA_COMPATIBILITY = UNKNOWN`.

Rollback **triggers** (restore the preserved image): a 5xx-rate increase attributable to the
release; a broken citizen critical path; an uncovered admin read/mutation regression; an
unexpected 403 increase for scoped admins; a missing `/faskes`, `/queues/new`, or `/admin`
after the release that introduced it.

**Do not** roll back for the intended fixes: zero-assignment non-super_admin 404s, cross-
facility notification retry/cancel failures, or zero summary counts for a zero-assignment
actor — those are the fix, not a regression. If a global `super_admin` **loses** access,
that is a regression in the fix and must be corrected **forward**.

---

## 7. Rollback commands

### 7a. Rollback target contract (BLOCKING — before any `build`/`up`)

| Required value | Source | Reject if |
|---|---|---|
| `CURRENT_PRODUCTION_IMAGE_ID` — **per service** (api, web, engine) | `docs/operations/OPERATOR_EVIDENCE_BUNDLE.md` §4, by **exact container name** | unset / "unknown" |
| `CURRENT_PRODUCTION_VERSION` | `curl -fsS https://<host>/_app/version.json` | unset / "unknown" / equal to the release being deployed |
| `PRESERVED_ROLLBACK_IMAGE_REF` — per service | §7c/§7d | unset / archive missing / checksum failed |
| `SIGAP_DEPLOY_DIR` | operator-selected compose directory | unset / not a compose repo |

If **any** value is unavailable: **ABORT before `build`/`up`.** The historical anchor
`6d7f940` is **not** the current production version and is **not** a valid rollback target
(§6a/§6b).

> **Identity contract.** A running image's revision is **never** inferred from the host Git
> checkout, from an image creation timestamp, or from the mutable `latest` tag. It is read
> from the container's `.Image` (a full `sha256:` id) via the evidence bundle. The host
> checkout (`87f218e` on `main`) is **not** evidence of what the running containers were
> built from — the images predate it.

### 7b. Directory + environment proof (before any build or up)

```bash
cd "$SIGAP_DEPLOY_DIR" || { echo "SIGAP_DEPLOY_DIR not set"; exit 1; }
git rev-parse --show-toplevel                 # must be the compose repo
git rev-parse --abbrev-ref HEAD               # expected branch
git rev-parse HEAD                            # expected release commit
git status --porcelain --untracked-files=no   # TRACKED must be empty (§2a)
docker compose version                        # >= 2.24.4 (the overlays use !override)
sh scripts/ops/preflight-production-env.sh    # exit 0 required
```

### 7c. Preserving the rollback images (do this BEFORE the first build)

`api`, `web` and `rust-engine` are **build-only services**: they declare no `image:` key, so
compose derives their image name as `<project>-<service>` (on the production host the project
is `sigap`, giving `sigap-api`, `sigap-web`, `sigap-rust-engine`). Those images have **no
registry backing** (`RepoDigests` is empty), so the implicit `:latest` tag is their **only**
named reference, and `docker compose build` retags it — orphaning the running release.
Derive the names from compose rather than hard-coding them, then tag by **image ID**:

```bash
# Run from $SIGAP_DEPLOY_DIR, with the SAME overlay set as deploy.
# compose reads .env automatically; `config` needs POSTGRES_PASSWORD / SIGAP_AUTH_MODE set.
COMPOSE="docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml"

# Immutable references created FROM the running containers' image IDs (never from `latest`).
STAMP=$(date -u +%Y%m%d)
for svc in rust-engine api web; do
  img=$($COMPOSE config --images | grep -E -- "-${svc}$")
  [ -n "$img" ] || { echo "ABORT: cannot resolve the image name for $svc"; exit 1; }
  id=$(docker inspect --format '{{.Image}}' "sigap-${svc}") || { echo "ABORT: cannot read sigap-${svc}"; exit 1; }
  short=$(printf '%s' "$id" | sed 's/^sha256://' | cut -c1-7)
  docker tag "$id" "${img}:rollback-${STAMP}-${short}"
  printf '%s  %s -> %s:rollback-%s-%s\n' "$svc" "$id" "$img" "$STAMP" "$short"
done
# Archive + checksum (see §13 for the disk/abort rules). Archive the PRESERVED refs.
#   for ref in $(docker image ls --format '{{.Repository}}:{{.Tag}}' | grep -E ':rollback-'); do
#     docker save "$ref" | gzip -9 > "$ARCHIVE_DIR/$(printf '%s' "$ref" | tr '/:' '__').tar.gz"
#   done
#   ( cd "$ARCHIVE_DIR" && sha256sum *.tar.gz > SHA256SUMS && sha256sum -c SHA256SUMS )
```

Nothing here removes an image. **Never** run `docker image prune` / `docker system prune`
before the archives exist and verify.

### 7d. Restoring the preserved images (production rollback)

Restores the **exact preserved images**, not an old commit and not a rebuild. Uses the
**same overlay set as deployment** — `docker-compose.yml` + `docker-compose.prod-ports.yml`
+ `docker-compose.prod-edge.yml`, never a subset. Omitting the edge overlay detaches `web`
from Traefik while republishing it on plaintext `0.0.0.0:3005`.

```bash
cd "$SIGAP_DEPLOY_DIR" || exit 1
COMPOSE="docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml"

# 0. Set the preservation stamp and archive directory. A rollback runs days after the
#    deploy, in a fresh shell, so these MUST be set here — they are NOT inherited from §7c.
#    STAMP is the date printed by §7c (the preservation date), NOT today's date.
: "${STAMP:?set STAMP to the preservation date from §7c, e.g. STAMP=20261008}"
: "${ARCHIVE_DIR:?set ARCHIVE_DIR to the directory holding the §7c archives}"
[ -d "$ARCHIVE_DIR" ] || { echo "ABORT: ARCHIVE_DIR '$ARCHIVE_DIR' does not exist"; exit 1; }

# 1. Verify the archives BEFORE touching the stack (fail closed).
( cd "$ARCHIVE_DIR" && sha256sum -c SHA256SUMS ) || { echo "ABORT: archive checksum failed"; exit 1; }

# 2. Restore the preserved images. `docker load` is only needed on a host that lost them;
#    if the preserved tags are still present this is a no-op.
#    for a in "$ARCHIVE_DIR"/*.tar.gz; do docker load -i "$a"; done

# 3. Re-point the COMPUTED image names at the preserved image IDs.
#    These services are build-only, so there is no `image:` key to override; compose
#    resolves <project>-<service>. Re-tagging that name back to the preserved ID is what
#    makes `up` recreate the containers from the rollback image. Because no build is
#    requested, nothing is rebuilt.
for svc in rust-engine api web; do
  img=$($COMPOSE config --images | grep -E -- "-${svc}$")
  [ -n "$img" ] || { echo "ABORT: cannot resolve the image name for $svc"; exit 1; }
  refs=$(docker image ls --format '{{.Repository}}:{{.Tag}}' "$img" | grep -E ":rollback-${STAMP}-")
  n=$(printf '%s\n' "$refs" | grep -c .)
  [ "$n" -eq 1 ] || { echo "ABORT: expected exactly 1 preserved tag for $img, found $n"; exit 1; }
  docker tag "$refs" "$img"
  printf '%s -> %s\n' "$refs" "$img"
done

# 4. Recreate ONLY the application services. PostgreSQL is NOT restarted: it is
#    unchanged, holds the data, and restarting it is an availability risk with no
#    rollback benefit. Same for the one-shot pg-cert-init. No --build.
$COMPOSE up -d --no-deps rust-engine api web

# 5. EXACT-IDENTITY verification: every restored container must run the preserved id.
for c in sigap-rust-engine sigap-api sigap-web; do
  printf '%s %s\n' "$c" "$(docker inspect --format '{{.Image}}' "$c")"
done
#    Compare each against the recorded CURRENT_PRODUCTION_IMAGE_ID. ANY mismatch = FAIL.

# 6. Identity + health verification.
curl -fsS https://<SIGAP_PUBLIC_HOST>/_app/version.json    # must equal CURRENT_PRODUCTION_VERSION
curl -fsS http://127.0.0.1:18080/health                    # API liveness
curl -fsS http://127.0.0.1:18080/readyz                    # API readiness (engine reachable)
curl -fsS http://127.0.0.1:3005/ >/dev/null                # web (loopback)
curl -fsS https://<SIGAP_PUBLIC_HOST>/ >/dev/null          # through the edge (Traefik route)
```

> Step 3 fails closed: it aborts if a computed image name cannot be resolved, or if the
> preserved tag for `$STAMP` is missing or ambiguous. If a future release adds an explicit
> `image:` key to these services, the retag step becomes unnecessary — set that key to the
> preserved ref instead.

**Preserved identities that must NOT change during a rollback:** Compose project name
`sigap`; the `sigap_pgdata` volume (data — never recreated); the `sigap_default` and
`traefik` networks; loopback-only bindings for `18080`/`5433`/`50051`/`3005`.

**Edge sanity** — assert on the **web** service only (an unscoped `host_ip` grep also matches
postgres/engine/api from `prod-ports` and can never fail):

```bash
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  config --format json | python3 -c '
import json, sys
w = json.load(sys.stdin)["services"]["web"]
ips = {p.get("host_ip") for p in w.get("ports", [])}
nets = set(w.get("networks", {}))
assert ips == {"127.0.0.1"}, f"web host_ip {ips} is not loopback-only (plaintext 3005 exposure)"
assert "edge" in nets, f"web is not attached to the edge network: {nets}"
print("edge sanity OK:", ips, nets)'
```

### 7e. Legacy rebuild-based rollback (superseded — retained for history)

The earlier procedure (`git checkout <historical pointer>` then `docker compose build`)
is **superseded**. It (a) rebuilds from source rather than restoring the artifact that was
running, and (b) would revert the `origin/main` security fixes, since every historical
pointer predates the merge (§6a). Do not use it for production.

<details>
<summary>Historical command shape (do not use)</summary>

```bash
# SUPERSEDED. Reverts security fixes; rebuilds instead of restoring the preserved image.
git fetch --all --tags
git checkout <HISTORICAL_PHASE_ROLLBACK_POINTER_FROM_6B>
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  build api web
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml \
  up -d postgres rust-engine api web
```
</details>

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
| **Application request with a side effect via audit** | any **protected-route probe** — RBAC-denied (`authz.go` → `logAuthzDenied`) **or** authorized (`handler/admin.go` `logAccess`/`logQueueAccess`/…) — writes an `audit_events` row even though the request is refused/served read-only. E.g. `GET /api/v1/admin/facilities`, `/api/v1/admin/notifications/summary` | **NO — never during read-only preflight** |
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
- the release commit is not the intended one, or **tracked** files are modified, or an
  **unclassified** untracked path is present (§2a — a classified sensitive artifact does
  **not** abort, and must never be deleted to pass the gate);
- `docker compose version` < 2.24.4 (the overlays use `!override`);
- the rollback target (§7a contract) is unknown, unset, or equals the release being deployed;
- a required environment variable (§5.1–5.2) is missing;
- `SIGAP_ENV` unset/empty, or `local` in the target environment (any case-variant — the guard matches `local` case-insensitively); the production overlay itself refuses unset/empty;
- `SIGAP_AUTH_MODE` is missing, `dev`, `disabled`, or any unknown value (canonical modes from `apps/api/internal/auth/config.go`: `dev`, `jwt`, `disabled`; production requires `jwt`. `disabled` boots but makes every admin route 403 — a dead deployment, indistinguishable from a broken one);
- `SIGAP_DEV_IDENTITY` is enabled, or `SIGAP_LOCAL_RBAC_TEST_IDENTITY` / `SIGAP_LOCAL_E2E_ACTOR` is set;
- the local identity selector is active (`SIGAP_ENV=local` + a subject);
- `make db-seed` would **not** refuse (the demo-seed guard is bypassed);
- a **redesign** commit added or edited a migration. **A non-empty `6d7f940..HEAD` migration
  diff does NOT abort** — it is the expected inherited upstream hardening (§1 **C**). The
  abort is for a migration introduced by this stream;
- the DB schema inspection has not run, or classified `UNKNOWN` / `UNEXPECTED_DRIFT` (§12);
- rollback images are not preserved, or their archives fail `sha256sum -c` (§7c/§13);
- disk headroom after the projected archive + build peak would fall below the 10% margin (§13a);
- the required compose overlay set differs from the runbook's (both `prod-ports` AND `prod-edge` for an edge-fronted deploy);
- the Compose version is below the required minimum;
- the expected compose files are missing from the host deployment directory;
- the rollback image IDs / version / deploy directory (§7a contract) are not recorded;
- any production build fails (step 3.2 runs before the cutover, so there is nothing to roll back).

**Post-deploy rollback triggers (evaluate only after the cutover):**

- the release identity (`/_app/version.json`) did not change after a deploy that rebuilt web, or does not match the release;
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
| Explicit `SIGAP_ENV=local` still resolves through compose (only `${SIGAP_ENV:?...}` on unset/empty is enforced) | rejected by `scripts/ops/preflight-production-env.sh` (§2.8/§7a) and by abort condition §9 | compose cannot express a value blacklist; the operator-invoked gate is the control, and `.env.example` ships `local` for dev |
| `-AllowNonLocalDatabase` can disable the smoke script's DB loopback guard (pre-existing) | retained for explicitly authorized staging rehearsals; off by default | removing it would block the sanctioned staging rehearsal path; the HTTP targets remain loopback-guarded regardless |
| Traefik middleware labels are not printed verbatim by the evidence bundle | keys only (§7 of the bundle) | label VALUES can hold credential material (e.g. `*.basicauth.users`); no such label exists today |

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

---

## 12. Database metadata inspection (PLANNED — requires separate authorization)

**Status: NOT AUTHORIZED, NOT EXECUTED.** This section defines the inspection; it does not
authorize it. Running it requires an explicit operator decision recorded in
`RELEASE_CHECKLIST.md`. Until it runs, `DATABASE_SCHEMA_COMPATIBILITY = UNKNOWN` and
**no schema-compatibility claim may be made** (§1 **F**, §2.4, §2.9c).

**Why it is needed.** The redesign introduces no migration of its own (§1 **A**/**B**), but
the release *does* carry an inherited edit to an already-shipped migration:
`9d4e68e` strengthened the `notification_outbox` phone-denylist CHECK constraints inside
`0006_notifications.sql`. The migration runner
(`apps/api/internal/migrate/migrate.go` → `Run()`) skips any version already recorded as
applied (`if applied[m.Version] { continue }`) and **never re-reads the stored checksum**, so
a database that applied `0006` before `9d4e68e` retains the **weaker** constraints
indefinitely. Nothing in the repository can reveal which definition is live.

**The package:** `scripts/ops/db-metadata-inspection.sql` — SELECT-only, `BEGIN READ ONLY`,
`statement_timeout` set, `pg_catalog`/`schema_migrations` metadata only. It never reads
application data rows, never writes, and never runs DDL.

**What it establishes:**
1. every recorded `schema_migrations` version (and `0006`'s stored checksum, diagnostic only);
2. the **full** `pg_get_constraintdef` text of every CHECK on `notification_outbox`
   (authoritative);
3. a single-boolean strengthened-vs-weak classifier (searches for the `{10,}` fragment that
   only the strengthened predicate contains);
4. column inventory + expected constraint names;
5. context identifiers (database, schema, server version).

**Classification — record exactly one:**

| Class | Meaning | Action |
|---|---|---|
| `MATCHES_CURRENT_SECURITY_CONSTRAINTS` | Strengthened checks present; versions consistent | None |
| `OLDER_WEAKER_CONSTRAINTS` | Phone constraints present but pre-`9d4e68e` definitions | Operator decides: **accept** (record compensating control) or **remediate forward** via a **new** migration |
| `MISSING_CONSTRAINTS` | No phone-denylist CHECK at all | Security finding — do not deploy until remediated or explicitly accepted in writing |
| `UNEXPECTED_DRIFT` | Unknown version, unknown constraint form, type/NOT-NULL change | **STOP.** Do not classify, do not deploy. Escalate. |
| `UNKNOWN` | Inspection not completed (no authorization, failure, timeout, partial output) | **Default and BLOCKING.** Never a PASS. |

**Remediation is forward-only.** If the class is `OLDER_WEAKER_CONSTRAINTS` or
`MISSING_CONSTRAINTS` and remediation is chosen, ship a **new** migration that `DROP`s and
re-`ADD`s the constraints. **Never** edit `packages/db/migrations/0006_notifications.sql`
again — the version-only runner would never reapply it, so the edit would be inert.

**Do not infer the class from the repository diff.** A non-empty
`git diff 6d7f940..HEAD -- packages/db/migrations` (§1 **C**) says only what the *source*
contains, not what the *database* has.

---

## 13. Disk capacity and backup/build safety

**Why this is blocking.** The application images are built locally and have **no registry
backing** (`RepoDigests` empty), so `latest` is their only reference and a `build` retags it.
The host was observed at **90% disk used / 17 GB free**. A `docker save` of the three
application images plus a rebuild peak can exhaust that headroom, and an out-of-space build
can leave the stack in a partially-tagged state.

### 13a. Capacity gate (before any `build` or `save`)

1. Record free space and the image sizes (evidence bundle §4/§6).
2. Compute the archive budget: `Σ (image size) × ~1.05` for the gzipped archives
   (api ≈ 27 MB, web ≈ 324 MB, engine ≈ 90 MB → ≈ 440 MB, worst case ~460 MB), **plus**
   the build peak (a rebuild can transiently hold the old image, the new layers, and the
   build cache simultaneously).
3. **Abort** if free space after the projected archive + build peak would fall below a
   **10% margin**.
4. To free space **before** preservation, free **non-Docker** space only (logs, old build
   output, `$ARCHIVE_DIR` contents from a superseded release). **Do not** run
   `docker image prune` / `docker rmi` at this point: after a build retags `:latest`, the
   orphaned running release *is* a dangling image, so pruning here can destroy exactly the
   artifact §7c preserves. Dangling-image pruning is permitted **only after** §7c's archives
   exist and `sha256sum -c` passes (§13b).

### 13b. Ordering rule (non-negotiable)

```
verify disk headroom  →  preserve rollback images (§7c)  →  verify archives  →  build  →  deploy
```

**Never** run `docker image prune`, `docker system prune`, `docker rmi`, or any
`docker compose down` that would remove images **before** the rollback archives exist and
pass `sha256sum -c`. Pruning first destroys the only copy of the running release.

### 13c. Backup interaction

`docker compose build` does not touch `sigap_pgdata`, but a disk-full event can corrupt a
running Postgres. The Postgres data volume (`sigap_pgdata`) is **not** part of a redesign
rollback (§6c) and **must not** be removed, recreated, or restored during one. If any step
here would delete a volume, stop.

### 13d. Post-deploy capacity

After a successful deploy, confirm free space did not drop below the §13a margin. Record the
before/after figures in `RELEASE_CHECKLIST.md`.

---

## 14. Secret retention and ignore-file disposition

The production deploy tree contains `.env.bak-phase5` — a sensitive backup of the production
environment file: untracked **and not git-ignored**, mode `600`, 449 bytes. Its mere
existence is not a failure (§2a), but it must be handled deliberately:

| Item | Rule |
|---|---|
| `.gitignore` | Adds **narrow** patterns for secret backups (`.env.bak*`, `.env.*.bak`, `*.env.bak`) so the artifact can never be accidentally staged. **`.env.example` must stay trackable** — verify with `git check-ignore -v .env.example` (must NOT match). |
| Host tree cleanliness | Adding an ignore pattern does **not** make the host tree clean. It only prevents accidental staging. The host continues to be reported as *not fully clean* until the artifact is removed by the owner. |
| Deletion | **Never** delete, move, or `git clean` this file from the deploy tree. It is the only local copy of production secrets. |
| Retention | Long-term retention/rotation is a separate operator task, tracked outside this release. |
| Staging | **Never** `git add` it. If it ever appears in `git status --porcelain` as staged, **hard abort** the deploy. |

Verify the ignore rules with:

```bash
git check-ignore -v .env.bak-phase5     # expected: matches a new rule
git check-ignore -v .env.example        # expected: no output (exit 1 = not ignored)
```

### 14a. Restricted backup of the production environment file (PLANNED — plan only)

**Status: documented, NOT executed.** The production `.env` is the **only** copy of the
production secrets, and it lives on a single host with no off-host replica. Losing that host
loses the secrets. This subsection specifies the remedy; it is **not** performed by this
release and is **not** a deploy gate.

Rules that constrain any such procedure:

| Constraint | Reason |
|---|---|
| The backup MUST be encrypted at rest **before** it leaves the host. | The file contains `POSTGRES_PASSWORD`, `SIGAP_AUTH_*`, and Supabase keys. |
| It MUST NOT be written into the deploy tree, into Git, or into any directory the compose stack mounts. | Prevents accidental staging/serving. |
| It MUST NOT be copied to an unencrypted or shared location. | The existing `.env.bak-phase5` on the host is already an unmanaged second copy; do not add a third that is worse. |
| Reading, copying, moving, or deleting `/opt/sigap/.env*` is **outside** this release's authorization. | Recorded as a known residual (§2a, §14). |
| The procedure MUST be reviewed by the operator/security owner before first use. | Secret-handling change. |

Sketch (illustrative, not authorized, not run):

```
# On the host, as root, into a mode-700 directory OUTSIDE the deploy tree:
umask 077
install -d -m 700 /root/sigap-secret-backups
age -r <operator-public-key> -o /root/sigap-secret-backups/sigap-env-<date>.age /opt/sigap/.env
sha256sum /root/sigap-secret-backups/sigap-env-<date>.age > /root/sigap-secret-backups/SHA256SUMS
# Store the .age file off-host in the SAME encrypted store used for DB dumps
# (see docs/operations/BACKUP_RESTORE.md). Never upload the plaintext.
```

Deliberately unspecified here: the key-management mechanism, the off-host target, and the
rotation policy. Those are decisions for the security owner, and this release does not make
them.
