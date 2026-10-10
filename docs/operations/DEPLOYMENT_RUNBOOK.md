# Sigap — Deployment & Rollback Runbook

**Phase:** 3B7 (production migration and deployment readiness)
**Applies to:** the Sigap redesign stream on branch `design/ui-ux-overhaul`
**Status:** readiness documentation only. **This runbook has not been executed. No deployment and no merge has occurred.**

**Production ledger — read-only contacts, enumerated.** Production has been accessed
**read-only**, and every contact is listed here; the invariant is that **no** DDL, DML,
migration, seed, restart, image change, filesystem change or deployment has been performed
in any of them:

| When | What | Scope |
|---|---|---|
| 2026-10-08 | host evidence refresh (§11a) | read-only host inspection |
| 2026-10-09 | PostgreSQL schema inspection (`scripts/ops/db-metadata-inspection.sql`, §12) | metadata-only, inside a read-only transaction; returned the classification recorded in §12 |
| 2026-10-09 | host capacity + running-image identity (`df`, `findmnt -T`, `docker system df`, `docker image inspect`, §13) | read-only SSH inspection producing the §13 figures |
| 2026-10-10 | Jira checkpoint write-path diagnosis (SIGAP-1 entity property) | a **successful** probe write confined to the MCP server's own `atlassian-mcp.` namespace; no Jira content, no SIGAP data, no production stack touched |

Anything not listed above has **not** happened. Production row counts remain `UNKNOWN` — the
authorization in force is metadata-only, and no application records were queried or modified.

This runbook is the canonical cutover procedure for the redesign. It is written against
the repository as it actually is, not against a hypothetical platform. Where a fact lives
only outside the repository (a secret, a DNS record, a platform toggle), it is marked
**OPERATOR VERIFICATION REQUIRED** with the exact check to perform — never asserted.

---

## 1. Scope and non-negotiable rules

The redesign ships **one** migration of its own, and the six statements below must not be
collapsed into one:

| # | Statement | How to verify |
|---|---|---|
| **A** | The redesign adds **exactly one** migration, `0011_notification_outbox_phone_constraints.sql`, and edits **no** historical migration. | `git diff --name-only origin/main..HEAD -- packages/db/migrations` → exactly `packages/db/migrations/0011_notification_outbox_phone_constraints.sql`. |
| **B** | The release's migration directory therefore **differs from current `origin/main`** by that one added file. | `git diff --stat origin/main..HEAD -- packages/db/migrations` → `0011_notification_outbox_phone_constraints.sql \| N ++++`, **non-empty by design**. |
| **C** | The diff against the **historical anchor `6d7f940` is NON-EMPTY**. | `git diff --stat 6d7f940..HEAD -- packages/db/migrations` → `0006_notifications.sql \| 4 ++--` plus `0011_notification_outbox_phone_constraints.sql`. `origin/main` commit `9d4e68e` **hardened** an already-shipped migration (added a formatted-phone predicate to two `notification_outbox` CHECK constraints). It reached this branch through the `origin/main` merge (`6c0ac82`), **not** through redesign work. |
| **D** | **Production DB constraints DO NOT match the hardened source migration.** | **CONFIRMED** by the 2026-10-09 inspection (§12): classification `OLDER_WEAKER_CONSTRAINTS`. Production applied `0006` before `9d4e68e` and keeps the weak predicate. |
| **E** | The migrator is **version-only**: it does **not** reapply a modified, already-applied migration. | `apps/api/internal/migrate/migrate.go` selects pending work by version number and never re-reads the stored checksum; production `SIGAP_AUTO_MIGRATE` is empty. |
| **F** | Production DB schema compatibility is **`OLDER_WEAKER_CONSTRAINTS`**, remediated forward by `0011` (§12). | The inspection result, and `scripts/ops/test-migration-0011.sh`. |

Consequences that follow directly:

- **Never claim migration status PASS from repository diff evidence alone.** A non-empty
  `origin/main..HEAD` diff (**B**) says the *source* adds a migration; it says nothing about
  whether that migration has been *applied* to the database (**D**/**F**).
- **Do not edit `0006` again.** It is shipped. The correction is the **new, forward-only**
  migration `0011` (§12).
- A **non-empty** `6d7f940..HEAD` diff (**C**) is *expected and classified*: an inherited
  upstream security hardening plus the redesign's own `0011`. Record it; do not abort on it,
  and do not "fix" it by reverting the hardening.
- **`0011` is a database change, so a rollback is no longer purely image-based.** Rolling the
  application back to a pre-`0011` image while the database carries the strengthened
  constraints is the compatibility question in §7f — it is **UNKNOWN** and blocking until
  answered (§7f, §12).

Rules that must hold at every step:

1. Deploy only from a clean tree at the exact release commit.
2. Never enable a dev-only capability outside `SIGAP_ENV=local`.
3. Never point a smoke run at production unless that deployment is separately and
   explicitly authorized.
4. Roll back by restoring the **preserved, currently running image** (§6/§7) — never by
   checking out an old design-phase commit, and never by editing an applied migration.
5. `0011` is applied **only** by an authorized operator action (§12), never automatically:
   production `SIGAP_AUTO_MIGRATE` stays unset.

---

## 2. Pre-deploy

| # | Check | How | Required result |
|---|---|---|---|
| 2.1 | Release commit selected | `git rev-parse HEAD` | The branch tip. Record it as `RELEASE_CANDIDATE_HEAD` (§6). |
| 2.2 | Branch | `git rev-parse --abbrev-ref HEAD` | `design/ui-ux-overhaul` |
| 2.3 | Working-tree policy | `git status --porcelain` | **Tracked** paths clean. Untracked paths are classified per §2a — an expected sensitive local artifact does **not** fail the gate, but it must be recorded with an explicit disposition. **Never run `git clean`.** |
| 2.4 | Migration provenance classified | §1 (**A**–**F**) | **A**/**B**: exactly one added migration, `0011`. **C** recorded as inherited upstream hardening plus `0011`. A migration PASS may **not** be claimed from these diffs alone — **D**/**F** are answered by §12, and `0011` must be **applied** (§12a) before the database matches the source. |
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
| 3.1 | database / migrations | **One migration must be RUN, deliberately, by the operator — not by this cutover.** `0011_notification_outbox_phone_constraints.sql` (§12a) converges the two `notification_outbox` phone constraints onto the release predicate. Production `SIGAP_AUTO_MIGRATE` is empty, so `docker compose up` applies **no** DDL: the cutover itself changes no schema, and the migration is a separate, explicitly authorized one-off. The inherited hardening of `0006` (§1 **C**) **will not** be reapplied by the version-only migrator (§1 **E**), which is exactly why `0011` exists. | `git diff --name-only origin/main..HEAD -- packages/db/migrations` = `0011_...` only; **§12 result recorded**; `0011` applied and re-classified `MATCHES_CURRENT_SECURITY_CONSTRAINTS` |
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
| `SIGAP_AUTO_MIGRATE` | run tracked migrations on startup | `false`; **leave unset**. `0011` is applied as a separate, explicitly authorized operator action (§12b), never by container startup. |
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

No rollback in this stream requires a **database restore** — but the redesign now **does**
add schema, so this is a narrower statement than it used to be. The release carries:

- an **inherited upstream constraint hardening** (`0006`, §1 **C**), and
- the redesign's own **`0011`** (§12b), which converges the production constraints onto that
  hardened predicate.

`0011` is **additive and forward-only**: it replaces two CHECK constraints and touches no
column, no row, and no other table. It is therefore **not** reversible by a database restore
and **not** undone by an image rollback — the constraint lives in the database. See §7f for
the resulting rollback-compatibility question, which is **BLOCKING and UNKNOWN**.

```
git diff --name-only origin/main..HEAD -- packages/db/migrations  # 0011_...sql  (one added file)
git diff --stat origin/main..HEAD     -- packages/db/migrations  # non-empty by design (§1 B)
git diff --stat 6d7f940..HEAD         -- packages/db/migrations  # 0006 (4 ++--) + 0011
```

Neither diff proves anything about the **deployed database** — that is §12. The inspection has
since run and returned `OLDER_WEAKER_CONSTRAINTS`; the database reaches
`MATCHES_CURRENT_SECURITY_CONSTRAINTS` only once `0011` is applied.

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

# REDACTION RULE: NEVER run a bare `docker compose config` in a logged shell, a CI job, a
# transcript, or a review artefact. It interpolates .env and prints POSTGRES_PASSWORD and any
# other secret in cleartext. Query the specific field you need (`config --images`, or
# `config --format json | jq -r .services.api.environment.SIGAP_API_PORT`), and if you must
# dump the whole config, pipe it through a redaction filter first:
#   $COMPOSE config | sed -E 's/(PASSWORD|SECRET|TOKEN|KEY)=.*/\1=<redacted>/'
# The deploy-guards script follows this rule; do not weaken it to make an assertion easier.

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

#### 7d.1 Restoring from the off-host archive (§13f artifact)

§13f's artifact is a **client-side-encrypted, compressed** multi-image archive
(`sigap-pre-redesign-<STAMP>.tar.gz.age`) whose digest is recorded **out-of-band** — not the
unencrypted `*.tar.gz` + `SHA256SUMS` pair §7d steps 1–2 consume. Restoring it on a fresh host
requires the §13f key custody model (private key lives on the archive host, never on the
workstation that fetched the ciphertext):

```bash
# ---- fetch: ciphertext only lands on the rollback host ----
: "${ARCHIVE_HOST:?}" "${ARCHIVE_DIR:?}" "${ARCHIVE_KEY:?}" "${STAMP:?}"
scp "$ARCHIVE_HOST:$ARCHIVE_DIR/sigap-pre-redesign-$STAMP.tar.gz.age" ./

# ---- verify the digest against the OUT-OF-BAND record, not a file beside the archive ----
sha256sum "sigap-pre-redesign-$STAMP.tar.gz.age"
#   compare with the digest recorded in RELEASE_CHECKLIST.md / the Jira issue at §13f step 2.
#   Mismatch = the archive is not the one that was verified off-host. ABORT.

# ---- decrypt, decompress, load (order is the inverse of §13f step 1: age -d THEN gzip -d).
#      Per-stage status is captured because `age -d | gzip -d | docker load` reports 0
#      even when `age` fails — see the fail-closed rule in §13f and its test suite.
#      The rollback host's Docker daemon must be asserted non-production before loading.
#      BOTH spellings of a local socket, and the production host itself, are refused:
#      the short `/var/run/docker.sock` form must not evade a guard that only matches
#      `unix:///var/run/docker.sock`. [OBSERVED] the naive string compare does evade it.
: "${ISOLATED_DOCKER_HOST:?set to this rollback host's DOCKER_HOST; refusing to load unguarded}"
is_production_target() {
  case "$1" in
    # the production SSH host, under any spelling
    ssh://fikriserver|fikriserver|ssh://fikriserver:*) return 0 ;;
    # a local daemon socket, long or short form
    unix:///var/run/docker.sock|/var/run/docker.sock|var/run/docker.sock) return 0 ;;
    # the active context, whatever it happens to be
    "$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null)") return 0 ;;
  esac
  return 1
}
if is_production_target "$ISOLATED_DOCKER_HOST"; then
  echo "REFUSED: ISOLATED_DOCKER_HOST '$ISOLATED_DOCKER_HOST' is a production target"
  exit 1
fi
# Belt and braces: assert it REALLY is a different daemon by checking the hostname/ID.
echo "restore target: $ISOLATED_DOCKER_HOST (asserted non-production)"
ST=$(mktemp -d)
{ age -d -i "$ARCHIVE_KEY" "sigap-pre-redesign-$STAMP.tar.gz.age" | gzip -d
  echo $? > "$ST/st.1"
} | { DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker load; echo $? > "$ST/st.2"; }
for f in "$ST"/st.*; do
  [ "$(cat "$f")" -eq 0 ] || { echo "REFUSED: restore stage $f failed; do not proceed"; rm -rf "$ST"; exit 1; }
done
rm -rf "$ST"

# ---- re-tag by ID to the tags §7d expects, then assert the three IDs ----
for id in 43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f \
          2a81c86258fb79c8bd9d49dcb7672234de4a962d07011dd75b6ffeae82ef7a0c \
          8ce2200fabf3e31f3be2290ac3749210c49267ecf8696231aa3cfa86e6f4a7e4; do
  docker image inspect "sha256:$id" >/dev/null || { echo "MISSING $id — ABORT"; exit 1; }
done
# Re-tag each to <project>-<service>:rollback-<STAMP>-<short> per §7c, then continue with §7d
# step 3 (retag to the computed image name), step 4 (up -d --no-deps), and step 5 (EXACT
# identity check) exactly as written there.
```

Both restore paths converge on §7d step 3. This subsection is the **prerequisite** the §13f
plan depends on: without it the off-host archive is not a usable rollback artifact and §13e
cannot claim §7c's preservation requirement is satisfied. §7d.1 must never be reverted
independently of §13f — they ship together.

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

### 7f. Rollback compatibility with the strengthened constraints (BLOCKING — UNKNOWN)

**The problem.** `0011` (§12b) strengthens two database CHECK constraints. An application
rollback restores a *previously running image* (§7c) — and **that image's source revision is
not proven**. The runbook already records that the checked-out tree is not evidence of what
the running containers were built from. So it cannot be assumed that the old API's write path
satisfies the strengthened predicate.

**Why this is a real risk, not a theoretical one.** The strengthened predicate rejects values
the weak predicate accepted — specifically a **separated** phone run with no 8-digit
consecutive run (e.g. `0812-3456-789`). If the old API ever writes such a value into
`subject` or `body_template`, then after `0011` is applied those inserts **fail at the
database**, and rolling the API back does not restore them: the constraint is in the database,
not in the image.

**Assessment: `ROLLBACK_SAFETY = UNKNOWN`.** It cannot be established from the repository,
because the running production image has no proven source revision. Two things would settle
it, and neither is available now:

1. the running image's actual source revision, or
2. a static reading of that revision's outbox write path against the strengthened predicate.

**A claim that this is `SAFE` was made and is WRONG — do not reinstate it.** The argument was
that the application denylist and the database constraint were "the same predicate", so any
image would write only values the constraint accepts. **That premise was false `[OBSERVED]`:**
the Go denylist carried a second alternative,
`[0-9]{3,4}[-._() ][0-9]{3,4}[-._() ][0-9]{2,4}`, which the CHECK constraint does not have. Go
was therefore **more permissive** than the database at that revision, and an image built from
it could write a body the strengthened constraint rejects. The two predicates agree again in
this release — Go was aligned to the immutable database predicate — but that repair says
nothing about the *old* image, whose revision is still unproven. `UNKNOWN` stands.

**Consequence: production migration authorization is BLOCKED on this.** Do not apply `0011`
to production until rollback safety is established or explicitly accepted.

**Never** treat "drop the strengthened constraint again" as the rollback strategy. Reversing a
security constraint to make a rollback work converts a deployment problem into a permanent
security regression, and it re-opens the drift this migration exists to close. If rollback
compatibility genuinely cannot be achieved, the correct answer is to keep the constraint and
fix the writer — not to weaken the database.

**Disposition options if the old image legitimately writes a rejected value on a rolled-back
deployment.** "Fix the writer" is not actionable for a binary that is already running — its
code is fixed. The real options are, in order of preference:

1. **Do not roll back the API; roll back only `web`.** Most redeploy triggers are
   frontend/UX regressions. `web` writes nothing to `notification_outbox`; if the trigger is
   web-only, keep the current (predicate-aligned) API image and revert only the web image. This
   is the default answer and should be the first thing checked when a rollback trigger fires.
2. **If the API itself must be rolled back while `0011` is applied:** accept **write refusal**
   on the rejected value class as a known, temporary degradation — the insert fails, the
   notification is lost, but no raw phone number is ever persisted. Record the refusal rate and
   the decision in the incident record. This is the trade-off the constraint exists to force.
3. **If write refusal is operationally unacceptable** (e.g. check-in notifications are the
   product), the correct remediation is a **new image built from a revision whose write path
   satisfies the strengthened predicate** — i.e. re-apply the Go-side alignment this release
   already contains, packaged against whatever older application behaviour triggered the
   rollback. That is a build-and-deploy action, not a rollback action, and must be scheduled as
   such.
4. **Never** resolve it by weakening or dropping the database constraint (see above).

**Reducing the unknown, without applying anything:**
- Recover the running image's revision: `docker image inspect` labels/`RepoDigests` on the
  host, or the deployed `/_app/version.json` (§3.0). If a revision is recovered, read its
  outbox insert path against the strengthened predicate.
- Note that the application-layer masking ships with the release and, once deployed, rejects
  the same class of value before it reaches the database. That makes the constraint a
  **backstop** for the new image — but it says nothing about the *old* image's behaviour.
- **Image IDs alone do not close this.** `[OBSERVED]` on 2026-10-09: the running api image is
  `sha256:43fcbcb3…`, which matches the recorded historical ID, and `RepoDigests` is empty, so
  the image carries no source-commit identity at all. **The correct instrument is an empirical
  test of the preserved binary**, described in §7g — not a source-code argument.

---

### 7g. Legacy API image compatibility test (PLAN ONLY — not executed)

**Purpose.** Answer §7f empirically: *does the image that is actually running in production
today write values that the strengthened (post-0011) predicate rejects?* Image boot success is
**not** evidence and must not be reported as such.

**Precondition — HARD GATE.** The test **requires the preserved image artifact** from §13f and
the owner's explicit authorization. `docker image save`/`load` and any transfer of image data
are **forbidden until then**. Do **not** substitute an image rebuilt from source: a rebuilt
image is a different binary and answers a different question.

**Environment (isolated, disposable).**

| Requirement | Specification |
|---|---|
| Network | **`docker network create --internal sigap-legacy-test`.** A default bridge NATs to the internet and can reach production's public IP, so "isolated" must be a **control, not a sentence**. `--network none` is wrong here — the test still needs the disposable PostgreSQL and the preserved engine. Assert **from inside** the API container, before any operation, that production's hostname/IP and a public address are both unreachable. |
| Credentials | **None.** No production secrets and no production `.env`. The preserved image takes **all** config from runtime env (`DATABASE_URL`, `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`, …), so a **synthetic env file is required and must be asserted**: fail if `DATABASE_URL` does not resolve to the disposable PG, if `SIGAP_AUTH_JWKS_URL` is non-empty and not a local stub, or if any value matches a production hostname. Use `SIGAP_ENV=local` with a local identity provider. **Never reuse `/opt/sigap/.env` values** — that is the natural shortcut and it silently voids this control. |
| Database | **Disposable local PostgreSQL 16** (matching production's major version), empty at start. |
| Data | **Fully synthetic**: facilities, appointments, notifications, identities, roles. |
| Architecture | `linux/amd64` — confirmed to match the captured images. |
| Docker daemon | A **separate daemon or host** from production, so `docker load` cannot touch production images. Set `DOCKER_HOST` explicitly and assert it is not the production context before loading anything. |

**Schema variants to run against.**

- **A. pre-0011 weak schema** — apply migrations `0001`–`0010` and rewrite the two phone
  constraints to the pre-`9d4e68e` weak form. This is the **baseline**: what the image does
  today. **Test-cluster only:** the weak-form rewrite is a destructive schema mutation and
  must run **only** against the disposable test cluster — assert the target `DATABASE_URL`
  host/port is the test cluster (e.g. port ≥ 55900 or a dedicated test socket) and refuse
  otherwise, so the weak-constraint rewrite can never reach a shared or production database.
- **B. post-0011 hardened schema** — apply `0001`–`0011` in order (a fresh build from the
  release already carries the strengthened predicate in `0006`).
- **C. pre-0011 → real `0011` transition** — apply `0001`–`0010`, install the weak form, then
  run migration `0011` **through the real runner**. This is the state a production rollback
  would actually encounter.

**Operations to exercise, in each variant.**

The two write paths below are the **only** places a legacy image can write a value the
strengthened predicate might reject. `[OBSERVED]` in `apps/api/internal/handler/booking.go`:

| # | Path | Exact location | Contract |
|---|---|---|---|
| 1 | `fireBookingConfirmation` | `booking.go:310`, enqueue at `booking.go:338` | `go func(){ defer recover(); ... Enqueue(ctx, ...) }` — **fire-and-forget** |
| 2 | `fireCheckInConfirmation` | `booking.go:361`, enqueue at `booking.go:388` | same shape |

**Both are goroutines with a `recover()` and a `slog.Warn` on error.** A rejected insert is
therefore **logged and swallowed** — the HTTP response has already been sent, so the failure is
invisible to every caller and to any smoke check. That is exactly why this test must read
`notification_outbox` for **absence** of a row rather than trusting a non-error response.

Exact values each path writes, which is what the constraint actually sees:

| Path | `subject` | `body_template` | template vars |
|---|---|---|---|
| booking | `Konfirmasi Janji Temu Sigap` | `Janji temu Anda berhasil dicatat. Kode check-in: {checkin_code}.` | `checkin_code` |
| check-in | `Status Check-in Sigap` | `Check-in Anda berhasil. Nomor antrean: {queue_number}.` | `queue_number` |

So the decisive input is the **rendered** `queue_number`, i.e. `{short_code}-{NNNN}` — item 4
below. A `checkin_code` is separately rendered and must be checked in item 6.

1. `notification` **Enqueue** — direct service calls, plus both paths above driven **through
   their real HTTP handlers** (create appointment, check in), not by calling `Enqueue` directly,
   because the handler is what picks the template and the channel.
2. **Appointment-booking notification path** — `fireBookingConfirmation` via `POST` booking.
3. **Check-in notification path** — `fireCheckInConfirmation` via the check-in route.
4. **Queue-number rendering** — the real `format!("{}-{:04}", short_code, n)` for
   `n ∈ {1, 9, 10, 99, 100, 300}` across the boundary `short_code` set:
   `RSK`, `DEMO-GIGI`, `f1`, `AB12CD`, `FSK-0001`, **`E2E123456`** (accepted boundary),
   **`E2E1234567`** (rejected boundary), `AB12345678` (8-digit run). Render each with the
   **engine's actual `queue.rs` formatter**, then have the legacy API render it into the
   check-in body and verify the row either exists (legitimate) or is refused (must match variant
   B/C exactly).
5. **Timestamps** — both the localized form (`9 Oktober 2026 pukul 14.30`) and the bare ISO
   form (`2026-10-09 14:30`), since the constraint's second conjunct rejects the latter.
6. **Legitimate identifiers** — appointment codes, check-in codes, facility names.
7. **Unsafe phone-like content** — must be **rejected in every variant**, never silently
   accepted in one and rejected in another.

**Comparison and reporting.** For every operation, record `RESULT BEFORE 0011` vs
`RESULT AFTER 0011`, and flag **any legitimate write that succeeds in A or C but fails in B or
C**. Report the **exact** failing value, the SQLSTATE, and the constraint name — this is the
signal that distinguishes "rollback breaks a legitimate flow" (an operator decision) from
"rollback breaks a security control" (never acceptable).

**Testable without production exposure.** The API's notification enqueue and booking/check-in
paths are reachable through its own HTTP API with a **locally minted identity** and synthetic
data, so no production endpoint or secret is needed. Anything requiring a live third-party
service (SMS/email gateways, SatuSehat) must be **excluded and disclosed as not covered** —
an untested dependency keeps that aspect `UNKNOWN` rather than `PASS`.

**Acceptance (§8).** `ROLLBACK_COMPATIBILITY` may move off `UNKNOWN` only when **all** of:
artifact preserved and identity-verified; restored in isolation; the full operation matrix run
in **A, B and C**; no legitimate write lost; no safety control relaxed; and every
production-specific dependency either covered or explicitly disclosed as a limitation.

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

### 8c. Deployment strategy comparison (PLAN ONLY — neither is executed)

Two candidate paths, evaluated against **measured 2026-10-09 host evidence**. The decisive
inputs: root filesystem **82.22 GiB free**, floor **15.74 GiB**, SIGAP application images
**420 MiB total**, build cache **0 B**, and the host is **shared with many other projects**.

| Axis | **Option 1** — preserve, expand, build and deploy on host | **Option 2** — preserve, build off-host, verify, import, deploy |
|---|---|---|
| Storage peak | A cold build of api + web on the host competes for the same 82.22 GiB the other projects' stacks use. Peak **unmeasured**, but headroom above the floor is 66.05 GiB, so it fits unless the build is enormous. | **Negligible on the production host.** Only the ~0.44 GB `docker load` transfer touches it. |
| Operational complexity | Lower — one machine, the existing compose files, no transfer step. | Higher — a build host, a transfer, an import, and a checksum verification hop. |
| Artifact identity | The registry-free `:latest` tag is re-pointed by the build; **provenance rests on the build host's state at build time**, and a build retag makes the running release a dangling image. | **Strongest.** Immutable digests are recorded before and after transfer, so what is deployed is provably what was reviewed. |
| Rollback preservation | Same requirement in both: §13f's off-host archive must exist **before** any build (§13b). | Same. |
| Database compatibility | Identical — `0011` is not applied by either path, and `SIGAP_AUTO_MIGRATE` stays unset. | Identical. |
| Downtime risk | One host; a `build` that fills the disk could disturb **other projects** on the shared host, which raises the blast radius beyond SIGAP. | Production impact is limited to the `docker load` + `compose up -d` window. |
| Secrets | `.env` and `.env.bak-phase5` already live on the host. No new exposure. | **Risk to manage.** The build host must not receive production secrets. **Build the image from a fresh `git clone` of the release commit, never by copying the deploy tree** — copying the tree would ship `.env.bak-phase5` (and any other untracked secret) into the build context. Assert `docker build --no-cache --progress=plain` output contains no `.env` in the "transferring context" step. There are no `ARG` instructions in any Dockerfile, so build-args validation is not the control; **context contents** are. The transfer itself must be authenticated and encrypted. |
| Network exposure | None new. | A single authenticated transfer channel (SCP/SSH) to a build host. |
| Verification | Build output is trusted from the host that built it. | Transfer is verified by digest before the image is deployed. |
| Runbook changes needed | **None.** This is the path the existing runbook already describes. | **Yes, minimal but real.** §3 needs an off-host build + verify + import stage, and §13's ordering rule gains a transfer step. Compose configuration itself needs **no change** — the project identity (`sigap`) and service definitions are unchanged; only the *provenance* of the images changes. |

**Recommendation.** **Option 2**, on the shared-host criterion rather than the capacity one.
Capacity is no longer the constraint — it passes with 66.1 GiB to spare — but the host runs
`orbit-chat`, `portal-sekolah`, the ELK stack, Traefik and Grafana/Prometheus alongside SIGAP.
A disk-hungry build on that host can degrade or break stacks that have nothing to do with this
release, and free space does not remove that **peak-load interference** — a build can saturate
CPU, memory and I/O long before it fills the disk. Off-host building confines that risk to the
short import window, and it is the only option that produces a digest-verified artifact.

Neither option is authorized or executed. Both remain blocked on the **rollback safety**
(§7f/§7g) and **build-peak measurement** (§13e.4) gates.

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
- the migration inventory differs from the declared release set: `git diff --name-only
  origin/main..HEAD -- packages/db/migrations` must be **exactly**
  `packages/db/migrations/0011_notification_outbox_phone_constraints.sql` (§1 **A**). Any
  *other* added or edited migration aborts. **A non-empty `6d7f940..HEAD` migration diff does
  NOT abort** — it is the expected inherited upstream hardening (§1 **C**) plus `0011`. The
  abort is for a migration outside the declared inventory;
- the DB schema inspection has not run, or classified `UNKNOWN` / `UNEXPECTED_DRIFT` (§12);
- the database is still `OLDER_WEAKER_CONSTRAINTS` and `0011` has not been applied — the
  schema does not yet match the release source (§12b). Applying `0011` is a separate
  authorized operator action, not part of the cutover (§3.1);
- **rollback safety is not established** (§7f): the previously running image's source
  revision is unknown, so it cannot be shown that the old write path satisfies the
  strengthened predicate. `ROLLBACK_SAFETY = UNKNOWN` is **BLOCKING**;
- rollback images are not preserved, or their archives fail `sha256sum -c` (§7c/§13);
- disk headroom after the projected archive + build peak would fall below the §13a **floor**
  (`max(2 GB, 10% of the filesystem size)`), or the build peak has not been measured
  (§13a step 4, §13e.4);
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
| **`facilities.short_code` has no database CHECK, and pre-existing rows are not remediated** | `validateShortCode` (§12c) enforces the contract on **create and update** only. A row written before this release — or by a seed script or any out-of-band writer — is not re-validated, and the Rust queue engine renders `{short_code}-{NNNN}` without checking it. **[INFERRED]** A facility whose stored `short_code` renders a phone-like queue number (e.g. `A1234567` → `A1234567-0300`, which the predicate rejects) would have **every** check-in notification for that facility refused by `notification_outbox_no_raw_phone_in_body_chk` and lost silently, because `Enqueue` is fire-and-forget. | Adding a `CHECK` on `short_code` needs a **new migration**, and this release deliberately ships exactly one (`0011`) and touches no other table. Prepared **read-only** scan for the operator, to be run with the §12a authorization and **not executed here**: `SELECT short_code FROM facilities WHERE short_code ~ '[0-9]{8,}' OR short_code ~ '[0-9][0-9\-._() ]{7,}[0-9]' OR short_code !~ '^[A-Za-z]' OR length(short_code) > 10;` — any row returned is a facility that must be renamed before its notifications can be delivered. |
| The phone predicate's `[0-9]` range is collation-resolved in PostgreSQL but ASCII in Go RE2 | recorded as a **direction-safe** residual | **[OBSERVED]** on the tested cluster, `[0-9]` matched only ASCII digits (Arabic-Indic, full-width, superscript, circled, Devanagari, Bengali, Thai and Roman-numeral forms all excluded). Any divergence makes PostgreSQL **stricter** than Go, i.e. Go accepts a value the database rejects → a lost notification, never a stored phone. The exact production locale (glibc `en_US.UTF-8`) is **[UNKNOWN]** and was not testable off-host. A differential fuzz over the deployment locale would close it. |

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
> banner, and the local Docker engine was down, so no host fact could be read *at that time*.
> That has since changed: read-only SSH to `fikriserver` as `fikri` is available and was used
> on 2026-10-09 for the database metadata inspection (§12), and earlier for the recorded host
> evidence (`/tmp/3b7-host-evidence.md`, host refresh 2026-10-08). Production HTTPS alone
> still cannot read env, containers, image digests or the deployment directory, and cannot
> probe admin routes without writing audit rows (§8a) — so the bundle remains the right
> instrument for the remaining preflight facts.

---

## 12. Database metadata inspection (EXECUTED 2026-10-09 — result recorded)

**Status: AUTHORIZED AND EXECUTED, read-only, once.** The operator authorized one
metadata-only inspection and it was run on 2026-10-09 against the production host.

- Script `scripts/ops/db-metadata-inspection.sql` — the revision that ran was
  `657eb675…a1174a6c` (blob `36959d81…c693343`, 17052 bytes), **re-verified byte-identical on
  the host before execution**. The shipped revision is now `43394fa3…4bbaebc5` (blob
  `0dde17bc…9f0161a5`, 21516 bytes) — the version-inventory fix, the STATUS header, and the
  **false-PASS fix for the invalid unescaped separator class** (`[0-9-._() ]`, which PostgreSQL
  accepts at `CREATE CONSTRAINT` and only rejects on evaluation). The recorded classification is
  unaffected, but **§12a must be re-run before the cutover** so the recorded hash matches the
  shipped artifact; `EXPECT_SHA` has been updated to the new value.
- Target: host `fikriserver`, database `sigap`, PostgreSQL **16.15**.
- Safety: `SELECT`-only, `BEGIN READ ONLY`, `statement_timeout = 15s`, final `ROLLBACK`.
  Metadata only. No application data row was read, no PHI, no DDL, no DML, no migration, no
  seed, no secret logged. Exit status 0.
- **Result: `OLDER_WEAKER_CONSTRAINTS`.** Migrations `0001`–`0010` applied, the expected 17
  columns and 10 CHECK constraints present, and **both phone constraints in the older weak
  form**:

  ```
  production : subject !~ '[0-9]{8,}'
  repo HEAD  : subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9\-._() ]{10,}[0-9]'
  ```

  Column set, constraint count and the applied-version set all match the release exactly; the
  **only** divergence is the two phone predicates. Sanitized evidence is recorded on Jira
  `SIGAP-60` (comment plus the `sigap.db.inspection` issue property).

**Why it was needed.** The release carries an inherited edit to an already-shipped migration:
`9d4e68e` strengthened the `notification_outbox` phone-denylist CHECK constraints inside
`0006_notifications.sql`. The migration runner
(`apps/api/internal/migrate/migrate.go` → `Run()`) skips any version already recorded as
applied (`if applied[m.Version] { continue }`) and **never re-reads the stored checksum**, so
a database that applied `0006` before `9d4e68e` retains the **weaker** constraints
indefinitely. Nothing in the repository could reveal which definition was live; the
inspection did, and the answer was the weak form.

**The package:** `scripts/ops/db-metadata-inspection.sql` — SELECT-only, `BEGIN READ ONLY`,
`statement_timeout` set, `pg_catalog`/`schema_migrations` metadata only. It never reads
application data rows, never writes, and never runs DDL.

### 12a. Exact authorized command (prepared — do not run without authorization)

Run this **on the production host**, from `$SIGAP_DEPLOY_DIR` (the compose repo that holds
`scripts/ops/`), only after the authorization is recorded in `RELEASE_CHECKLIST.md` J1. It
contains no credential: `docker exec` enters the container as the Postgres superuser already
running there, and the script itself is piped in on stdin so nothing is written into the
container.

```bash
# 0. Declare the expected artifact and the expected target. Everything below is
#    COMPARED against these, not merely printed.
#    The deploy tree must hold the reviewed revision; a stale copy fails the
#    verdict at the end of this block.
EXPECT_SHA=43394fa3a1bf560e436a6bffa2d061ccdde4bf1c1effb673c90a33bc4bbaebc5
EXPECT_DB=sigap
EXPECT_SCHEMA=public

# 1. Run the read-only inspection. Capture BOTH streams and the exit code.
#    --no-psqlrc stops the CONTAINER's psqlrc (and any PSQLRC it points at) from
#    injecting statements into the session. The script's own BEGIN READ ONLY /
#    ROLLBACK bracket the whole run; output is small and bounded (10 versions +
#    10 constraint definitions + 17 columns + 4 counts).
#    The capture goes OUTSIDE the deploy tree — an untracked file inside it
#    trips the §2a tree gate (see J2).
OUT="/root/sigap-evidence/sigap-db-metadata-$(date -u +%Y%m%dT%H%M%SZ).txt"
install -d -m 700 "$(dirname "$OUT")"
docker exec -i sigap-postgres \
  psql -U sigap -d sigap -v ON_ERROR_STOP=1 --no-psqlrc -P pager=off -f - \
  < scripts/ops/db-metadata-inspection.sql > "$OUT" 2>&1
RC=$?
echo "psql exit=$RC  output=$OUT"

# 2. Verify the artifact, the target, and the completeness of the capture.
#    - The class is taken from the ANCHORED match ONLY. Query 3 emits
#      pg_get_constraintdef text, which is attacker-influenced; an unanchored
#      grep could pick a class token out of a constraint definition and report
#      the wrong class.
#    - psql's aligned format pads the value, so the pattern tolerates whitespace.
#    - Exactly ONE class line, exactly ONE ROLLBACK tag, exactly ONE context row.
PAT='^[[:space:]]*(MATCHES_CURRENT_SECURITY_CONSTRAINTS|OLDER_WEAKER_CONSTRAINTS|MISSING_CONSTRAINTS|UNEXPECTED_DRIFT)[[:space:]]*$'
SHA=$(tr -d '\r' < scripts/ops/db-metadata-inspection.sql | sha256sum | cut -d' ' -f1)
CLASS=$(grep -oE "$PAT" "$OUT" | tr -d '[:space:]')
n=$(grep -cE "$PAT" "$OUT")
rb=$(grep -cx 'ROLLBACK' "$OUT")
ctx=$(grep -cE "^[[:space:]]*${EXPECT_DB}[[:space:]]*\|[[:space:]]*${EXPECT_SCHEMA}[[:space:]]*\|" "$OUT")
sha_ok=0; [ "$SHA" = "$EXPECT_SHA" ] && sha_ok=1
echo "sha_ok=$sha_ok rc=$RC class_lines=$n rollback_tags=$rb context_rows=$ctx"

# 3. Record the target identity next to the class in J3. A correct class for the
#    wrong database is void.
grep -E "^[[:space:]]*${EXPECT_DB}[[:space:]]*\|" "$OUT" | tail -1

# 4. Verdict. Do NOT add `exit`/`set -e`: this block is pasted into an
#    interactive shell and must not close the session. The FINAL command is the
#    test, so the block's own exit status is 0 only for a complete, correctly
#    targeted, unmodified run — a wrapper can rely on it.
ok=0
if [ "$sha_ok" -eq 1 ] && [ "$RC" -eq 0 ] && [ "$n" -eq 1 ] && [ "$rb" -eq 1 ] && [ "$ctx" -eq 1 ]; then
  ok=1; echo "CLASSIFICATION OK: $CLASS"
else
  echo "CLASSIFICATION NOT OK (sha_ok=$sha_ok rc=$RC class_lines=$n rollback_tags=$rb context_rows=$ctx) -> RECORD: UNKNOWN"
fi
[ "$ok" -eq 1 ]
```

Rules for the operator:

| Rule | Reason |
|---|---|
| **Do not** run this without J1 recorded. | It is a production read; the mission requires explicit authorization. |
| **Do not** add `-c`/`--command` flags, `\i`, or any write statement. | The script is the whole inspection. |
| **Do not** remove `--no-psqlrc`. | The container's `psqlrc` (or a `PSQLRC` it points at) could inject statements. |
| **Do not** redirect into the deploy tree or any Git-tracked path. | An untracked path there is a hard abort (§2a). |
| Set `EXPECT_DB`/`EXPECT_SCHEMA` to the target **before** running. | The verdict requires exactly one context row matching them; a class taken from the wrong database is void. |
| Record the class **only** if step 4 prints `CLASSIFICATION OK: <class>`. Anything else — a non-zero `psql` exit, a hash mismatch, zero or several class lines, no `ROLLBACK` tag, no matching context row — is `UNKNOWN`. | Partial, tampered or mis-targeted output is not evidence. |
| Record the class verbatim in `RELEASE_CHECKLIST.md` J3, **with** the database name and `server_version` from step 3. | Exactly one of the five values; a class for the wrong database is void. |
| `UNKNOWN` and `UNEXPECTED_DRIFT` are **blocking**. | §12 classification table. |
| **No password appears in any argument.** | `docker exec` uses the container's own superuser; no `-W`/`PGPASSWORD` is needed or permitted. |
| The container's environment is trusted. | `psql` honours the container's `PGOPTIONS`/`PGHOST`/`PGSERVICE`; a `search_path` override could point the inspection at a shadow schema. If that environment is not trusted, scrub it with `docker exec -e PGOPTIONS= … -e PGHOST= …`. |

Expected hash of the prepared script (LF-normalised):
`43394fa3a1bf560e436a6bffa2d061ccdde4bf1c1effb673c90a33bc4bbaebc5`
(git blob `0dde17bc4adb01735f2706527a1822829f0161a5`, 21516 bytes). §12a step 0 sets
`EXPECT_SHA` to this value and step 4 **compares** it, so a stale or doctored copy in the
deploy tree fails the verdict rather than merely printing a different number. If the value
above and the committed blob ever disagree, stop and re-read this section.

> **This hash CHANGED in the drift-remediation round.** The inspection that produced the
> recorded 2026-10-09 result ran the *previous* revision
> (`657eb675…a1174a6c`, blob `36959d81…c693343`, 17052 bytes), which pinned the applied-version
> inventory at `1..10`. That inventory is wrong for the post-remediation world — a database
> that has had `0011` applied must not be reported as drift — so the script gained a
> `required_version` set (`1..10`) distinct from the known-version set (`1..11`). The
> **recorded classification is unaffected** (the observed production state was `1..10` + weak
> constraints, which both revisions classify as `OLDER_WEAKER_CONSTRAINTS`), and the change
> was verified against that exact state. Re-run §12a before the cutover so the recorded hash
> and the shipped artifact agree.

> **And it changed AGAIN for a FALSE PASS, which is the more serious of the two.** The
> classifier normalises a constraint definition by collapsing a doubled backslash before
> matching it against the expected predicate. That normalisation made the **invalid** class
> `[0-9-._() ]` — in which `9-.` is an invalid character range — textually identical to the
> valid `[0-9\-._() ]`, so a constraint carrying the invalid class was certified as
> `MATCHES_CURRENT_SECURITY_CONSTRAINTS`. **The state is reachable:** PostgreSQL accepts such
> a predicate at `CREATE CONSTRAINT` and only raises when it is evaluated, and
> `standard_conforming_strings=off` produces exactly that degradation from a plain
> single-quoted literal. A false PASS here means a constraint that **raises on every insert**
> is reported as healthy. The classifier now detects the unescaped class explicitly (via
> `strpos`, since `LIKE` treats `\` as an escape), and `scripts/ops/test-db-classifier.sh`
> gained variant `T-unescaped-separator-class` with two preconditions — 33/0, was 32/0. The
> recorded production classification is unaffected: the observed state carried the **weak**
> form, not the invalid one. Re-run §12a before the cutover.

**What it establishes:**
1. every recorded `schema_migrations` version (and `0006`'s stored checksum, diagnostic only);
2. the **full** `pg_get_constraintdef` text of every CHECK on `notification_outbox`
   (authoritative);
3. a single-row **classification** (`4b`) derived from a predicate-bound residue test **plus**
   a full contract comparison — see the classifier note below;
4. the supporting counts behind that classification (`4`);
5. column inventory + expected constraint names;
6. context identifiers (database, schema, server version).

**The classifier binds predicates to columns and compares the whole contract. Four simpler
designs were rejected, because each can report a false PASS:**

| Rejected design | Why it is unsafe |
|---|---|
| Bare fragment test `LIKE '%{10,}%'` | A **drifted** predicate such as `subject !~ '[0-9]{10,}'` also contains `{10,}`, so it is counted as "strengthened" and classified `MATCHES` — a false PASS in the class that authorises **no action**. |
| Residue test that strips the column identifiers | It cannot tell that `..._subject_chk` guards `body_template`, so a copy-paste error that leaves `subject` **unguarded** still classifies `MATCHES` — a false PASS with a live raw-phone insert path. |
| Matching only the two phone constraints | A dropped structural constraint, an unexpected constraint, a changed column, or a diverged applied-version set would all still report `MATCHES`. |
| Residue test where "residue empty + contains `{10,}`" counts as strengthened | The `{10,}` conjunct **alone** also strips to an empty residue, yet it is strictly *weaker* than the release definition: it admits an 8-digit raw run that the release's `!~ '[0-9]{8,}'` conjunct rejects. This was a real false PASS in an earlier revision of this classifier; variant `H-strong-clause-only` now covers it. |

The shipped classifier therefore does **both** of the following, and any deviation forces
`UNEXPECTED_DRIFT`:

1. **Predicate-bound residue.** For `..._subject_chk` it removes `subject !~ '[0-9]{8,}'` and
   `subject !~ '[0-9][0-9\-._() ]{10,}[0-9]'`, and likewise for `..._body_chk` with
   `body_template`. Both known forms reduce to an empty residue; anything else — a mis-bound
   column, an `OR`, a widened quantifier, an extra term — leaves a non-empty residue.
   The `{10,}` conjunct **alone** is not accepted: it strips to an empty residue yet is
   strictly *weaker* than the release definition (it admits an 8-digit raw run that
   `!~ '[0-9]{8,}'` rejects), so the strengthened form requires **both** conjuncts.
2. **Full contract comparison.** The 10 expected CHECK names, the 17 expected columns and the
   applied-version set `1..10` are each compared against what the database actually has, in
   both directions (unexpected *and* missing). The two phone constraints are judged by (1),
   not by the name inventory, so a merely absent denylist is still reported as
   `MISSING_CONSTRAINTS` while an *extra* constraint is reported as drift.

Precedence: any contract deviation — unexpected/missing structural constraint, column or
version, or a phone predicate that is neither known form — is `UNEXPECTED_DRIFT`; an intact
contract with fewer than two phone constraints is `MISSING_CONSTRAINTS`; only then are the
strengthened and weak forms considered. `MATCHES_CURRENT_SECURITY_CONSTRAINTS` therefore
requires **all** of: both phone predicates bound to their own columns and matching the
strengthened form exactly, the full constraint inventory, the full column inventory, and the
applied-version set.

Non-vacuity is proven by `scripts/ops/test-db-classifier.sh`, which builds a **disposable
local** cluster and asserts 33 checks over 32 schema variants — the **real release schema**
(migrations `0001`–`0011` applied in order), the genuine pre-hardening weak form, the
fragment-spoof case, the `{10,}`-conjunct-only case (a predicate that looks strengthened but
is weaker), both mixed strong/weak columns, a `NOT VALID` phone constraint (present but not
validated against existing rows), a missing subject/body constraint, both phone
constraints absent with the rest of the contract intact and with an extra constraint added,
the mis-bound-column case, an extra constraint, three logically altered predicates, absent
migration history, five kinds of unexpected metadata, a missing table, an
explicitly-correct hardened schema, a run of the release variant with
`standard_conforming_strings=off` (the backslash-independence check), the **real pre-`0011`
production shape (`1..10` + weak) and its post-`0011` result**, the absent-denylist state
before and after `0011`, the **`version 11 recorded but constraints still weak`** desync, the
**`hardened but version 11 not recorded`** case, the **invalid unescaped separator class
`[0-9-._() ]`** (variant `T-unescaped-separator-class` — a predicate PostgreSQL accepts at
`CREATE CONSTRAINT` but raises on when evaluated, and which the classifier's backslash
normalisation previously rendered textually identical to the valid `[0-9\-._() ]`, a **false
PASS**), and a static consistency check between the SQL's expected inventories and
`packages/db/migrations`.
The observed result is **33 pass / 0 fail**; only a genuinely strengthened schema reports
`MATCHES_CURRENT_SECURITY_CONSTRAINTS`. Each assertion was confirmed to be non-vacuous by
mutating the classifier and observing the suite fail.

A non-zero `psql` exit (a missing table, a missing tracking table, a timeout, partial output)
yields no classification row and must be recorded as `UNKNOWN`. `UNKNOWN` and
`UNEXPECTED_DRIFT` are both blocking.

> **Two version sets, deliberately.** `expected_version` is the KNOWN set `1..11` and catches a
> version outside it. `required_version` is the MANDATORY set `1..10` and catches a missing
> version. They differ because version 11 is optional in a legitimate sense: the Go runner
> records it, a plain `psql -f 0011_...sql` does not, and both leave the constraints
> converged. Requiring 11 would report a correctly-remediated database as `UNEXPECTED_DRIFT` —
> a false STOP on the exact state `0011` exists to produce.

> **Maintenance:** the expected constraint names, column names and version lists in queries
> 4/4b are transcribed from `packages/db/migrations/`. **Adding a migration, a column or a
> constraint requires updating them**, or every subsequent inspection will report
> `UNEXPECTED_DRIFT`. The test's `M-inventory` check exists to catch exactly that drift.
> The suite needs local PostgreSQL binaries (`initdb`, `pg_ctl`, `psql`) and exits **2** when
> they are absent. It **is** wired into the `ops-guards` job in `.github/workflows/ci.yml`
> (alongside the migration and contract suites), which installs those binaries itself.
>
> **When running it in CI, do NOT use `sh suite; rc=$?; [ "$rc" -eq 2 ] || exit "$rc"`.**
> GitHub Actions runs `run:` blocks under `bash -e`, so the suite's exit 2 terminates the step
> before the guard executes — the guard is dead code and a clean skip becomes a hard failure.
> Use the `if` form, which captures the status without tripping `set -e` (this is what the
> `ops-guards` job uses):
>
> ```sh
> if sh scripts/ops/test-db-classifier.sh; then
>   :                       # 0 = PASS
> else
>   rc=$?
>   [ "$rc" -eq 2 ] || exit "$rc"   # 2 = MISSING PREREQUISITE, anything else = FAIL
> fi
> ```
>
> **Exit codes are a contract: `0` PASS, `1` FAIL, `2` MISSING PREREQUISITE.** An unexpected
> skip must FAIL where the prerequisite is expected to be present. In CI the PostgreSQL
> binaries are installed by the job, so exit 2 there means the job is misconfigured, not that
> the suite is inapplicable — gate the job on binary availability rather than tolerating 2.
> Never use `sh suite || true` or `sh suite || rc=$?`: both turn a genuine exit-1 failure into
> a silent pass.

**Classification — record exactly one:**

| Class | Meaning | Action |
|---|---|---|
| `MATCHES_CURRENT_SECURITY_CONSTRAINTS` | Both phone predicates carry **both** conjuncts, bound to their own columns; the full 10-constraint and 17-column inventories match the release; no required version is missing and no version outside `1..11` is recorded | None. This is the state of a database built from HEAD (where `0006` already carries the strengthened predicate, so `0011` is a no-op) **and** the post-`0011` state of a drifted database, whether or not `0011`'s version row was recorded. This classifier deliberately does not distinguish them: the predicate is the control, the version row is bookkeeping. |
| `OLDER_WEAKER_CONSTRAINTS` | Both phone constraints present and matching the pre-`9d4e68e` weak form (`<col> !~ '[0-9]{8,}'` only); everything else matches. **This is the recorded production state.** | Operator decides: **accept** (record compensating control) or **remediate forward** via a **new** migration — the prepared remediation is `0011` (§12b) |
| `MISSING_CONSTRAINTS` | Fewer than two phone-denylist CHECKs exist, with the rest of the contract intact | Security finding — do not deploy until remediated or explicitly accepted in writing. `0011` also handles this state (§12b). |
| `UNEXPECTED_DRIFT` | Any other deviation: unexpected/missing structural constraint, unexpected/missing column, unexpected/missing applied version, or a phone predicate that is neither known form (`{10,}` conjunct only, mis-bound column, `OR`, altered quantifier, extra term, or the two columns in different forms) | **STOP.** Do not classify, do not deploy. Escalate. `0011` refuses this state too, rather than guessing. |
| `UNKNOWN` | Inspection not completed (no authorization, failure, timeout, partial output, non-zero `psql` exit, or step 2 not printing `CLASSIFICATION OK`) | **Default and BLOCKING.** Never a PASS. |

**`UNKNOWN` is always the default, and it is always blocking.** If a future inspection cannot
complete — no authorization, a connection failure, a timeout, partial output, or a non-zero
`psql` exit — then `DATABASE_SCHEMA_COMPATIBILITY = UNKNOWN` and **no schema-compatibility
claim may be made**. "Unknown" is never a PASS. The same applies after `0011` is applied: if
the re-inspection cannot complete, the post-migration state is `UNKNOWN`, not "presumed
fixed".

**Inspected state vs planned state — keep these distinct.** The classification above is the
**inspected production state** as of 2026-10-09. The **planned release state** is
`MATCHES_CURRENT_SECURITY_CONSTRAINTS`, and it is reached only when `0011` is applied by an
authorized operator action (§12b, checklist J10). Until then, the database does **not** match
the release source.

**Remediation is forward-only and is prepared as `0011`.** The production class is
`OLDER_WEAKER_CONSTRAINTS`, so the correction ships as a **new** migration,
`packages/db/migrations/0011_notification_outbox_phone_constraints.sql` (§12b). **Never** edit
`packages/db/migrations/0006_notifications.sql` again — the version-only runner would never
reapply it, so the edit would be inert.

**Do not infer the class from the repository diff.** A non-empty
`git diff 6d7f940..HEAD -- packages/db/migrations` (§1 **C**) says only what the *source*
contains, not what the *database* has.

### 12b. Prepared remediation — `0011_notification_outbox_phone_constraints.sql`

**Status: written, tested locally, NOT APPLIED to production.** No production mutation is
authorized by this document.

The migration converges the two phone constraints onto the release predicate whatever the
starting point, and **fails closed** on anything it does not recognise:

| Starting state | Behaviour |
|---|---|
| both constraints already strengthened | **no-op** (this is a database built from HEAD) |
| both constraints in the weak form (**production**) | `DROP` and re-`ADD` strengthened |
| both constraints absent | `ADD` strengthened |
| a target name exists but is NOT a CHECK constraint | `RAISE EXCEPTION`, **change nothing** (a type-agnostic `DROP CONSTRAINT` would otherwise remove it) |
| mixed / `NOT VALID` / unrecognised predicate | `RAISE EXCEPTION`, **change nothing** |
| `notification_outbox` absent | `RAISE EXCEPTION`, **change nothing** |

Properties that matter for the cutover:

- **Names are preserved.** The release inventory of 10 CHECK constraints on
  `notification_outbox` is unchanged, so the classifier's expected inventory needs no name
  change and its negative controls keep working.
- **Lock behaviour — this is an `ACCESS EXCLUSIVE` window, and it blocks the notification
  worker too.** `ALTER TABLE ... DROP CONSTRAINT` and `ADD CONSTRAINT` both take
  `ACCESS EXCLUSIVE` on `notification_outbox`. `ACCESS EXCLUSIVE` conflicts with **every**
  other lock mode, so during the window **no reader can `SELECT` the table and no writer can
  `INSERT`/`UPDATE` it** — which includes the notification worker's own
  `status`/`attempt_count`/`next_attempt_at` `UPDATE` and every `Enqueue` `INSERT`. `ADD
  CONSTRAINT` validates **every existing row** while holding the lock, so the window scales
  with the table; on a large outbox it is a table-wide outage for its duration.
  **[OBSERVED]** on a disposable cluster: with an open `SELECT` transaction the migration does
  not complete until that transaction ends, and the same holds for an open `UPDATE`
  (`apps/api/internal/notification/migration_lock_test.go`,
  `TestMigration0011_BlocksOnConcurrentReader` / `_BlocksOnConcurrentWriter`).
- **The runner sets neither `lock_timeout` nor `statement_timeout`, and has no retry loop.**
  `migrate.Run` (`apps/api/internal/migrate/migrate.go`) issues the migration body directly and
  aborts on the first error; there is no automatic re-attempt. So an unbounded lock wait is the
  **default**, and the only bound is the caller's context. A bounded window is therefore the
  **operator's** control, not the migration's. Impose it on the migration connection, e.g.
  `PGOPTIONS='-c lock_timeout=5s -c statement_timeout=120s'`, or `SET LOCAL lock_timeout` /
  `SET LOCAL statement_timeout` in the wrapping transaction.
- **The lock queue is FIFO, so the table is unavailable from the moment the ALTER *requests*
  the lock.** PostgreSQL grants locks in request order to prevent starvation. Once `0011`'s
  `ACCESS EXCLUSIVE` request is queued behind an open transaction, every later `SELECT`/`INSERT`
  on `notification_outbox` queues **behind the migration** rather than proceeding. **[OBSERVED]**:
  a plain `SELECT count(*)` issued while the migration waited did not return. The practical
  consequence is that the outage begins when the migration issues the `ALTER`, not when it
  acquires the lock, and it ends only when both the blocker and the migration have drained.
  Draining the table of long transactions **before** starting the migration is therefore the
  control that actually bounds the window.
- **A lock timeout fails closed and is retryable.** **[OBSERVED]**
  (`TestMigration0011_LockTimeoutFailsClosed`): with `lock_timeout` set and a concurrent reader
  holding `ACCESS SHARE`, the migration abandons the wait with SQLSTATE **`55P03`
  (`lock_not_available`)** and leaves **no partial state** — the version-11 row is **not**
  recorded and both constraint definitions are unchanged. After the blocker releases, a plain
  re-run applies cleanly and records version 11. Retrying is a **manual operator action**; the
  runner will not do it for you. Note that the timeout **message text is locale-dependent** —
  the test cluster renders it in Indonesian (`pembatalan perintah karena kunci kehabisan waktu
  tunggu`) — so gate on the SQLSTATE, never on the prose.
- **`statement_timeout` bounds the validation scan, not the wait.** Set both: `lock_timeout`
  stops a migration that cannot get the lock, `statement_timeout` stops one that got the lock
  but is scanning a table far larger than expected. Choose them from the measured row count
  below, not from a guess.
- **Measure before applying (read-only, prepared — NOT executed).** The row count and the
  current lock wait are **not known**. Obtain them with the §12a read-only path, which already
  runs under `BEGIN READ ONLY` with `statement_timeout = 15s`, or with this aggregate, which
  reads no row content:

  ```sql
  -- read-only; returns a count, never row content
  SELECT count(*) AS outbox_rows FROM notification_outbox;
  ```

  `notification_outbox` is a queue, so its steady-state size should be small; a large count
  means the validation scan is long and a maintenance window is mandatory. **Do not apply
  `0011` until this number is recorded** — it is what makes the window estimable at all.

  **Separately prepared, NOT executed — the violating-row aggregate.** `count(*)` answers the
  window question but not the *will it apply* question. The count that decides whether `0011`
  succeeds at all is:

  ```sql
  -- read-only; a single number, no row content, no identifiers, no bodies.
  -- The regex literals are DOLLAR-QUOTED deliberately — see the note below.
  SELECT count(*) AS violating_rows
    FROM notification_outbox
   WHERE subject      ~ $re$[0-9]{8,}$re$ OR subject      ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
      OR body_template ~ $re$[0-9]{8,}$re$ OR body_template ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$;
  ```

  Both conjuncts of **both** production constraints are covered (`subject` and `body_template`,
  against `notification_outbox_no_raw_phone_in_subject_chk` /
  `notification_outbox_no_raw_phone_in_body_chk`, exactly the predicates `0011` installs at
  `0011_notification_outbox_phone_constraints.sql:223-227`). A row satisfying any one of the
  four is a row `0011` will refuse, so `violating_rows > 0` ⇒ `0011` **cannot** apply.

  **AUTHORIZATION GATE — separate request required, NOT granted.** The 2026-10-09 inspection
  was authorized for **metadata only** (`pg_catalog` + `schema_migrations`). These two
  aggregates read **application rows**, which that authorization does not cover, so they are
  **prepared and NOT executed**. Both row counts are `[UNKNOWN]`.

  When the owner grants a new authorization, the bracket must be:

  ```sql
  BEGIN READ ONLY;
  SET LOCAL statement_timeout = '60s';
  SET LOCAL lock_timeout = '5s';
  -- the two aggregates above; then ROLLBACK
  ```

  **Numeric output only.** Return the two counts and nothing else: **no** row IDs, **no**
  `body_template`/`subject` values, **no** patient names or contact values, **no** sample
  records, **no** `SELECT *`. `[INFERENCE]` a single leaked body is a patient-data incident;
  the aggregate's decision value is entirely in the number. Run the aggregates with
  `-t -A -F ' '` so `psql` emits bare numbers, and paste only those numbers into evidence.

  **Do not infer `0` violating rows.** A synthetic local test cannot observe production rows,
  and the local suites only prove the *mechanism* refuses violating rows — never that none
  exist.

  **Why dollar-quoted and not single-quoted — verified, not assumed.** Under
  `standard_conforming_strings=on` (the modern default) a plain `'…\-…'` literal reaches the
  regex engine intact. Under `standard_conforming_strings=off` the **backslash is consumed by
  the string-literal escape**, so the class becomes `[0-9-._() ]` — in which `9-.` is an
  **invalid character range**. `[OBSERVED]` on PostgreSQL 18.4, with `scs=off`:

  - a plain `'[0-9\-]'` literal and a `'[0-9-]'` literal are **equal and both length 6** — the
    backslash is silently discarded; and
  - writing the invalid range **literally** as `[0-9-._() ]` makes `~` raise
    **`invalid character range`** at evaluation time.

  The second conjunct's class therefore has two failure shapes: it can silently widen, or it
  can fail to evaluate at all. **A constraint in that state is created without error** and
  then **raises on every insert** — which is precisely the false-PASS this release's
  classifier work closes. Dollar quoting is immune to `scs` in either direction, so it is
  used here and in `0011` itself. `TestMigration0011_Transition_*` asserts the dollar-quoted
  form is what `0011` installs.

  `TestMigration0011_Transition_ViolatingHistory` proves the consequence: any row in that set
  makes `0011` raise **`23514`**, records **no** version 11, and changes nothing — so the
  migration **cannot** be applied until such rows are remediated by a separate, explicitly
  authorized decision. **This query has been deliberately NOT run** under the current
  authorization, which covers metadata only. It is prepared here so the operator can run it in
  the same read-only bracket as §12a if and when they choose. `0` rows means `0011` applies;
  a non-zero count means **STOP** — that is an operator remediation decision, not a migration
  change.

  **`[UNKNOWN]` as of this session:** neither count is known. The 2026-10-09 inspection read
  `pg_catalog` and `schema_migrations` only, and no aggregate over application rows was taken.
- **No `BEGIN`/`COMMIT` in the file.** The runner already wraps each migration in its own
  transaction, so the DDL and the `schema_migrations` version row commit **atomically**. (Note
  that `0006` is the only migration carrying its own `BEGIN`/`COMMIT`, which ends the runner's
  transaction early; `0011` deliberately does not repeat that, and the migration suite asserts
  the absence statically.)
- **The version row is written by the runner, not by `psql -f`.** Applying the file with plain
  `psql` runs the DDL but never records version 11. The constraints converge either way, so
  this is not a security gap — but the runner would then attempt to re-apply on the next start.
  Prefer the migrator (checklist J10), or record the row in the same transaction as the DDL.
- **`standard_conforming_strings` cannot corrupt the predicate.** The regex literals are
  **dollar-quoted** (`$re$…$re$`). This is load-bearing: under
  `standard_conforming_strings=off` a plain `'…\-…'` literal has its backslash consumed by the
  string-literal escape, so the server parses `[0-9][0-9-._() ]{10,}[0-9]` — in which `9-.` is
  an **invalid character range**, producing a constraint that RAISES on every insert. Worse, a
  self-referential comparison would then have certified that broken predicate as "already
  strengthened". Dollar quoting is immune, so the intended regex is what both the expectation
  and the real constraint are built from. Asserted by scenario 13 of the migration suite.
- **Canonical predicate comparison.** The expected constraint is materialised on a `TEMP`
  table and compared through `pg_get_constraintdef`, so both sides are rendered by the same
  PostgreSQL code path in the same session. A literal string comparison would be wrong:
  `pg_get_constraintdef` renders the regex backslash differently under
  `standard_conforming_strings=off`, and appends ` NOT VALID` to an unvalidated constraint.
- **A non-CHECK constraint with a target name is refused, not dropped.** `DROP CONSTRAINT` is
  type-agnostic, so a same-named `NOT NULL` constraint (PostgreSQL 18 allows named ones) would
  be silently removed. The migration detects that case and raises.
- **A `NOT VALID` final state is never accepted**, because an unvalidated constraint has not
  been checked against existing rows. `0011` always `ADD`s with validation.

Tested by `scripts/ops/test-migration-0011.sh` — a disposable local cluster covering a fresh
all-migrations database, the weak state, the already-hardened state, the absent state, mixed /
spoofed / `NOT VALID` drift, pre-existing violating rows, transaction rollback, reapplication
and version tracking, real enforcement on `subject` and on `body_template`, column binding,
ordinary-content compatibility, `standard_conforming_strings=off`, the absence of a top-level
`BEGIN`/`COMMIT`, and the definitions of all eight structural constraints. Observed result:
**20 pass / 0 fail**. Every assertion checks the **actual resulting schema**
(`pg_get_constraintdef`, `convalidated`, constraint count), not merely a zero exit code, and
uses only synthetic data.

Lock and timeout behaviour is tested separately, against the **real runner** on a
production-shaped database (migrations `1..10` recorded, both constraints in the weak form):
`apps/api/internal/notification/migration_lock_test.go` —
`TestMigration0011_BlocksOnConcurrentReader` (an open `ACCESS SHARE` blocks the migration; the
ungranted `AccessExclusiveLock` is observed in `pg_locks`; it completes once released),
`TestMigration0011_BlocksOnConcurrentWriter` (same for an open `ROW EXCLUSIVE` `UPDATE`), and
`TestMigration0011_LockTimeoutFailsClosed` (`lock_timeout` applied to the **runner's**
connections → SQLSTATE `55P03`, no partial state, version 11 unrecorded, clean retry
afterwards). The remaining branches of the state machine are covered by
`TestMigration0011_Transition_MixedStateRefused` (mixed strong/weak → the migration's own
`RAISE`, SQLSTATE `P0001`, nothing changed) and `TestMigration0011_Transition_AbsentConstraintsAdded`
(both absent → added strengthened). Run as part of `sh scripts/ops/test-notification-contract.sh`,
alongside the transition tests that reconstruct the observed production shape and prove 0011
fails closed on violating history, and `TestPredicate_CorpusAgreesWithTheDatabase`, which drives
the shared predicate corpus through a live constraint.

**Each DB-backed test provisions its OWN isolated database** (`migPool` creates and drops one per
test). This is required, not tidiness: the tests assert they just applied migrations `0001..0010`,
which is false on any database that already has version 11 recorded — including the CI `api` job's
shared database, which `ci-migrate` migrates *before* `go test ./...` runs. `DATABASE_URL` is
therefore a connection to a **server**, not a database under test, and the role needs `CREATEDB`.
Note also that a harness failure is now propagated: each sub-suite's status is read from the
`go test` invocation itself, not from a pipeline's `tail` (see `test-skip-exit-codes.sh` for the
same class of defect in shell).

**Idempotent.** A second run is a no-op, whether or not the version row was recorded. No row is
rewritten or deleted: if any existing row violates the strengthened predicate, `ADD CONSTRAINT`
raises, the transaction aborts and the database is left exactly as it was. Whether such rows
exist is **not known** — the inspection read metadata only and never touched application rows.
If the migration fails on this, **STOP**: remediating those rows is a separate, explicitly
authorized decision, not part of this migration.

**Applying `0011` is a separate, explicitly authorized operator action.** It is *not* part of
the deploy cutover (§3.1): production `SIGAP_AUTO_MIGRATE` is empty, so `docker compose up`
applies no DDL. After it is applied, re-run the inspection (§12a) and record
`MATCHES_CURRENT_SECURITY_CONSTRAINTS`.

---

### 12c. `facilities.short_code` — the queue-number safety contract (NO migration)

**Status: application-layer control only. Not a database constraint, and not applied to existing rows.**

`short_code` is interpolated verbatim into the queue number the Rust queue engine renders as
`format!("{}-{:04}", short_code, next_number)` (`apps/queue-engine/src/engine/queue.rs`), and
that queue number reaches `notification_outbox.body_template` through the `{queue_number}`
template variable. A `short_code` built from digits and phone separators can therefore render a
value indistinguishable from a phone number, and the insert is then refused by
`notification_outbox_no_raw_phone_in_body_chk` — a silent failure, because `Enqueue` runs on a
fire-and-forget goroutine.

`validateShortCode` (`apps/api/internal/handler/admin.go`) closes this at the source, on create
and on update. It enforces: non-empty; at most 10 characters; first character an ASCII letter;
only letters, digits and `-`; and — the load-bearing rule — the **rendered** queue number must
not match the phone predicate, tested with `notification.ContainsRawPhoneDigits`, the same
function the denylist uses.

**The rule is the rendered value, not a digit count.** An earlier revision capped the digits in
`short_code` at 2. That was safe but **wrong in the other direction**: it rejected codes the
database accepts, silently narrowing the product and breaking the E2E suite's own `E2E123456`,
whose rendered queue number `E2E123456-0300` is 11 characters — one short of the 12 the second
conjunct requires. Hand-deriving a numeric bound is what makes that class of rule wrong in one
direction or the other; testing the rendered value against the actual predicate has no bound to
get wrong.

Boundary, **[OBSERVED]** against a live constraint (`TestRenderedQueueNumber_GoAndDatabaseAgree`
inserts the rendered value directly, bypassing the Go layer):

| `short_code` | rendered (`-0300`) | database verdict |
|---|---|---|
| `E2E123456`, `A123456` | `E2E123456-0300`, `A123456-0300` | **accepted** (6 trailing digits is the most that fits) |
| `E2E1234567`, `A1234567` | `E2E1234567-0300`, `A1234567-0300` | **rejected** (one digit more crosses the line) |
| `AB12345678` | `AB12345678-0300` | **rejected** (8 consecutive digits — the first conjunct) |

**Residual, recorded in §10:** there is no `CHECK` on `facilities.short_code`, and rows written
before this release are not re-validated. Adding the constraint needs a **new migration**, which
this release deliberately does not ship. The prepared read-only scan in §10 lists any facility
that must be renamed.

---

## 13. Disk capacity and backup/build safety

**Why this needs care.** The application images have **no registry backing**
(`RepoDigests` empty), so `latest` is their only reference and a `build` retags it — the
orphaned running release *becomes* a dangling image, which is why preservation must precede
any build (§13b).

**Status: GATE PASSES — measured 2026-10-09.** An authorized read-only SSH inspection of
`fikriserver` recorded the host's actual figures, and they are **materially better** than the
estimate this section previously carried:

| Measurement | Value |
|---|---|
| Filesystem | `/dev/sda2`, `ext4`, 160 GB disk (single partition, no separate volume) |
| Total | `168,971,526,144` bytes = **157.37 GiB** |
| Used | `73,607,806,976` bytes = 68.55 GiB |
| **Available** | **`88,279,465,984` bytes = 82.22 GiB (52.25%)** — re-measured 2026-10-10 |
| `/var/lib/docker` | **same filesystem** (`/dev/sda2`) — confirmed with `findmnt -T` |
| Docker images | 33 images, 12.56 GB total, **100% reclaimable** |
| Docker build cache | **0 B** |
| Containers | 34 (25 running), 4.895 GB |
| Local volumes | 38, 5.4 GB (731.6 MB reclaimable) |
| `sigap_pgdata` | 66,449,216 bytes = **63.4 MiB** |

Against the floor in §13a.3b: `max(2 GB, 10% × 168,971,526,144 B) = 16,897,152,614 B`
= **15.74 GiB**. Headroom above the floor is **82.22 − 15.74 = 66.48 GiB**, and the three
SIGAP application images total `440,801,639` bytes = **420.4 MiB** — about 3.5% of all Docker
image storage. Archiving them locally drops free space to ≈ 81.89 GiB, still **66.2 GiB above
the floor**.

> **The earlier blocker no longer applies, and it was never a storage problem — it was a
> measurement problem.** §13 previously recorded *"16 GB free of a 158 GB filesystem"* with a
> *"90% used"* reading and concluded the gate failed with ~0.2 GB of margin. The **actual**
> measured state is **82.22 GiB available = 52.25% of the filesystem, i.e. 47.75% used or
> reserved** (68.55 GiB used, 44.4% of the volume). No decision in this document was based on
> the wrong reading — it was recorded as `BLOCKED` — but the recorded numbers were wrong, and
> they are now corrected. The capacity gate **PASSES on measured evidence**.
>
> **One caveat that still gates a `build`.** Free space is not the only binding input: the
> **build peak is still unmeasured** (§13e.4), and the host is **shared** with many other
> projects (`orbit-chat`, `portal-sekolah`, the ELK stack, Traefik, Grafana/Prometheus and
> more — 33 images, 25 running containers). A build is still gated on the measured peak, and
> reclaiming the host's other images is **not** this release's decision to make.

### 13a. Capacity gate (before any `build` or `save`)

1. Record free space and the image sizes (evidence bundle §4/§6).
2. Compute the archive budget. Raw `Σ (image size)` = 26,911,970 + 323,542,368 + 90,347,301 =
   **440,801,639 bytes ≈ 420.4 MiB**; the gzip allowance is `× ~1.05` → **≈ 463 MB ≈ 0.46 GB**.
   Use the **0.46 GB** figure as the budget (the raw sum is not the budget, and this document
   previously labelled the raw sum with the `×1.05` formula's name). **Plus** the build peak
   (a rebuild can transiently hold the old image, the new layers, and the build cache
   simultaneously).
3. **Abort** if free space after the projected archive **plus a stated build-peak allowance**
   would fall below the floor in 3b. The margin is expressed as an **absolute floor**, not a
   bare percentage: the previous "10% margin" wording named no base (10% of the volume? of
   free space? of the archive?) and was therefore undecidable.

   **3b. The floor is `max(2 GB, 10% of the filesystem size)` free after archive + build peak.**
   Naming the base is the fix; it is not a relaxation. On this host the 10%-of-volume term is
   **15.74 GiB**, which is *stricter* than the 2 GB term and is therefore the binding one. The
   2 GB term exists only so the rule still means something on a much smaller filesystem.
4. **The build peak is an unmeasured input and must be measured before a `build` may run.**
   **On the measured 2026-10-09 figures** the capacity portion of the gate **passes** with a
   wide margin: `82.22 GiB − 0.46 GB ≈ 81.79 GiB` free after the archive, against a floor of
   **15.74 GiB** — i.e. **66.1 GiB of headroom**, so an unmeasured build peak of up to
   ~66 GiB would still fit. That is far more than any plausible peak for 420 MiB of
   application images. The build peak nonetheless remains **UNMEASURED**, so this gate is
   `PASS (capacity) / BLOCKED (peak unmeasured)` and a `build` still waits on §13e.4.
5. Storage expansion is **not required** for this release on measured evidence. See §13e —
   the earlier `BLOCKED` verdict rested on an estimate that was wrong by roughly 66 GiB, and
   the options are re-ranked accordingly.
6. To free space **before** preservation, use **order-safe** reclamation: non-Docker space
   (logs, old build output, `$ARCHIVE_DIR` contents from a superseded release) **and build
   cache** (`docker builder prune`, §13e S3). **Do not** run `docker image prune`,
   `docker system prune`, or `docker rmi` at this point: after a build retags `:latest`, the
   orphaned running release *is* a dangling image, so image pruning here can destroy exactly
   the artifact §7c preserves. Image pruning is permitted **only after** §7c's archives exist
   and `sha256sum -c` passes (§13b).

   Note: reclaiming the host's **other** projects' images is not this release's decision. The
   host runs many unrelated stacks (33 images, 25 running containers); `docker system df`
   reports 12.56 GB of images as 100% reclaimable and 4.895 GB of container writable layers,
   but most of that belongs to `orbit-chat`, `portal-sekolah`, the ELK stack, Traefik and
   Grafana/Prometheus. **Build cache is 0 B** on this host, so §13e S3 has nothing to reclaim
   here and the build-peak measurement in §13e.4 measures a cold-cache build from scratch.
   Image reclamation remains **only after §7c preservation**, and is not needed on the
   measured figures.

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

After a successful deploy, confirm free space did not drop below the §13a **floor**
(`max(2 GB, 10% of the filesystem size)`). Record the before/after figures in
`RELEASE_CHECKLIST.md`.

### 13e. Storage remediation options (plan only — nothing here is executed)

**Status: PLAN ONLY.** No archive is created, no image is removed, no volume is changed, no
prune is run by this release.

**MEASURED 2026-10-09 (read-only SSH, `fikriserver`).** `floor = 15.74 GiB`, `available =
82.22 GiB`. Headroom above the floor is **66.48 GiB**, archiving the three application images
costs **0.46 GB**, so after preservation `81.89 GiB` remains — **66.2 GiB above the floor**.
**Storage expansion is not required for this release, and an off-host archive is no longer
needed to make the gate pass.** It remains valuable for a different reason: it is the only
mechanism that produces an **immutable, identity-verified rollback artifact** (§7c), which is
a rollback requirement, not a capacity one.

**What previously read as a ~0.2 GB deficit was a measurement error, not a storage
constraint.** The earlier figures (`16 GB free`, `90% used`) were an estimate; the host is
actually **82.22 GiB available = 52.25% of the volume, i.e. 47.75% used or reserved**. The arithmetic that followed from the wrong input
(`16 − 0.46 = 15.54 GB < 15.8 GB floor`, and the conclusion that an off-host archive could
not clear the gate) was correct **for the input it was given** — but the input was wrong, so
the conclusion does not hold. The gate passes on measured evidence.

| # | Option | Effect | Cost / risk | Verdict |
|---|---|---|---|---|
| **S1** | **Off-host encrypted image archive.** `docker save` the three images **by immutable ID**, gzip, encrypt (`age`/`gpg`), move ciphertext off-host. | Produces the **immutable, checksum-verified rollback artifact** the runbook's ordering rule requires before any build. Removes even the transient 0.46 GB local peak if streamed (see §13f). | Requires an off-host destination, a key held off-host, and a transfer. Encrypt **before** it leaves the host. | **Recommended, for rollback — not for capacity.** |
| **S2** | **Grow the constrained filesystem**, e.g. a VPS disk resize. | Adds free space `d` while adding only `0.1·d` to the floor, so the margin grows by `0.9·d`. | Provider-side resize + filesystem grow on a running production host. **The whole host is shared with many other projects, so a resize benefits all of them and is not solely a SIGAP decision.** A second volume used *only* for `$ARCHIVE_DIR` does **not** move the root filesystem's margin at all. | **NOT required** on measured figures. |
| **S3** | **Reclamation** — build cache, then images after preservation. | Build cache is **0 B** on this host, so `docker builder prune` has nothing to reclaim. The 12.56 GB of images is 100% reclaimable but **mostly belongs to other projects** (`orbit-chat`, `portal-sekolah`, ELK, Traefik, Grafana/Prometheus) — only ~420 MiB is SIGAP's. | `docker image prune` is order-constrained: after a build retags `:latest` the orphaned running release *is* a dangling image. | **NOT required**, and **not recommended** — it would delete other projects' artifacts and buys nothing SIGAP needs. |
| **S4** | **Build strategies that minimise peak pressure.** Build one service at a time (`build api`, then `build web`), avoid `docker buildx` multi-platform output, `DOCKER_BUILDKIT=1`. | Lowers the peak, which is the one unmeasured term. | Slightly longer cutover window. | **Adopt.** Build cache is empty here, so the build is cold regardless; S4 is the cheapest way to keep the peak small. |

**13e.5 — The gate now passes on capacity.** `STORAGE_READINESS`
is **`READY_FOR_AUTHORIZATION`** rather than `BLOCKED`: the capacity arithmetic passes with
66.1 GiB to spare. Three caveats keep this from being an unqualified PASS:

1. The **measured build peak** is still outstanding (§13e.4 item 4), so a `build` is gated —
   now gated on a measurement, not on a shortfall.
2. The **off-host archive destination is unconfirmed** (§13e.4 item 5, §13f). S1 is the option
   §13e marks *recommended*, and its target is a prerequisite of authorizing §13f — so the
   destination confirmation belongs on the same list, not after it.
3. The host is **shared**, so a SIGAP build's peak competes with other projects' activity.
   Sample free space during a maintenance window, not during someone else's build.

**13e.4 — Measurements now taken, and the one still outstanding.**

| Input | Status | Value |
|---|---|---|
| 1. Exact free bytes (`df -B1 /`) | **DONE (re-measured 2026-10-10)** | `88,279,465,984` bytes = 82.25 GiB (was 88,368,025,600 on 2026-10-09; −0.09 GiB of normal host activity) |
| 2. `docker system df -v` | **DONE** | Images 12.56 GB (100% reclaimable); Containers 4.895 GB; Volumes 5.4 GB (731.6 MB reclaimable); **Build Cache 0 B** |
| 3. Exact per-image bytes | **DONE** | api `26,911,970`; web `323,542,368`; engine `90,347,301`; postgres `294,282,098` |
| 4. **Measured build peak** | **OUTSTANDING** | strategy in §13e.6 below. **No production build may run to obtain it.** |
| 5. `du -sh $ARCHIVE_DIR` and off-host capacity | **OUTSTANDING** | `/opt/sigap/backups/sigap` is currently **empty** (4 KiB), **mode 775 (world-readable)** and on the **same** root filesystem; the off-host destination is unconfirmed, and `age` is absent on the host. See §13f. |

**13e.6 — Build-peak measurement strategy (no production Docker build).**

The build peak is the one unmeasured input, and the only reason it blocks `build`. It can be
measured **without any production build** — which matters because a production build both
competes with five unrelated stacks and risks retagging the running release's images.

**Preferred: measure on a disposable daemon.** The workstation has ~99 GB free and a local
Docker daemon (when running). The build is CPU/IO/disk-shaped identically, so the peak
transfers as an upper bound.

```bash
# On the disposable daemon. NEVER on the production host, and never with the release
# image names — a throwaway tag cannot retag `:latest`.
STAMP=$(date -u +%Y%m%dT%H%M%SZ)

# Sample free bytes every second for the whole build, into a file, and take the min.
( while :; do df -B1 --output=avail . | tail -1 >> peak.log; sleep 1; done ) &
SAMPLER=$!
trap "kill $SAMPLER 2>/dev/null" EXIT

docker build --no-cache --progress=plain \
  -t "sigap-baseline-api:$STAMP" -f apps/api/Dockerfile   apps/api
docker build --no-cache --progress=plain \
  -t "sigap-baseline-web:$STAMP" -f apps/web/Dockerfile   apps/web

kill $SAMPLER 2>/dev/null
# peak = baseline_available − min(sampled_available)
sort -n peak.log | head -1     # the minimum free space observed = the peak footprint
```

Record: **baseline free**, **minimum free during build**, **peak = baseline − minimum**, and
the elapsed wall time. Both services are built separately because `apps/api` and `apps/web`
have very different dependency footprints (Go modules vs. a SvelteKit node tree), and S4's
"build one at a time" advice only means what it says if each peak is known.

**Three rules that must not be relaxed to obtain the number:**
- **No production build.** A `docker compose build` on `fikriserver` retags `sigap-api`/`sigap-web`
  `:latest`, which is exactly what §7c/§13b exists to prevent.
- **No unapproved pruning, no cross-project cleanup.** The 12.56 GB of "100% reclaimable"
  images belong to `orbit-chat`, `portal-sekolah`, the ELK stack and others. They are **not
  SIGAP's to reclaim**, and removing them to make room is out of scope for this release.
- **The floor is not weakened to fit a bad measurement.** `max(2 GB, 10% of the filesystem)` =
  15.74 GiB on this host, after archive **and** build peak. If the measured peak cannot fit
  under the floor, the answer is S2 (grow) or S4 (shrink the peak), never a smaller reserve.

**What the measurement does not need to capture.** The `postgres:16-alpine` image is public and
registry-recoverable, so it is not part of the peak that matters for preservation. The engine
image is unchanged by the redesign build path in the same way as `api`/`web`, but it is built
in the same cutover window, so measure it too if the daemon is available.

**Do not weaken this gate to permit a deploy.** A deploy that fills the disk mid-build leaves
a partially-tagged stack with no preserved rollback image — the exact failure §7c exists to
prevent.

---

### 13f. Off-host image preservation (PLAN ONLY — not executed)

**Purpose.** Produce an **immutable, identity-verified** copy of the three running application
images so that (a) a `build` can retag `:latest` without destroying the only copy of the
running release (§13b), and (b) the legacy-API rollback test (§7g) can run an actual binary
image rather than a source guess.

**Immutable IDs captured read-only on 2026-10-09** (`docker inspect` / `docker image inspect`),
with layer digests recorded for restore-identity verification:

| Service | Image ID | Size (bytes) | Created | Layers |
|---|---|---|---|---|
| api | `sha256:43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f` | 26,911,970 | 2026-09-14T08:13:56Z | 4 |
| web | `sha256:2a81c86258fb79c8bd9d49dcb7672234de4a962d07011dd75b6ffeae82ef7a0c` | 323,542,368 | 2026-09-08T17:21:29Z | 10 |
| rust-engine | `sha256:8ce2200fabf3e31f3be2290ac3749210c49267ecf8696231aa3cfa86e6f4a7e4` | 90,347,301 | 2026-09-14T08:18:24Z | 4 |

Container IDs: api `40bf694b849d`, web `4f316e3cdca7`, rust-engine `793ba3a38496`,
postgres `03df9148a857`. Architecture `amd64`, OS `linux` for all; `RepoDigests` empty.
**Not to be preserved by this release**: `postgres:16-alpine`
(`sha256:75f5a96988cdf694a215073c3e9c001b706b371e2f94df3967f2efdec2787f6b`, a public image
recoverable from any registry) — the SIGAP **data volume** is the irreplaceable asset and is
handled by §6c/§13c, not by image preservation.

**The api image ID matches the historical value exactly**
(`43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f`), and so do the web
(`2a81c862…`) and engine (`8ce2200f…`) IDs. **`[OBSERVED]` — the historical IDs are CURRENT**,
not stale. The remaining caution is the one §7f already records: an image ID does not disclose
its source revision, so the running api/web/engine images still have **no proven source
commit**. That is precisely what §7g's isolated test exists to settle, and why `ROLLBACK_SAFETY`
stays `UNKNOWN`.

**Design: stream, do not stage.** `docker save` output is ~0.44 GB and the host has 82.22 GiB
free, so staging locally is *feasible*; but streaming still removes the transient footprint
and the risk of a half-written local archive.

**Pre-transfer secret check (mandatory, read-only).** `docker image save` embeds the image
**config**, including its `Env` array, in addition to every layer. This repo has shipped a
baked credential before — `docs/PRODUCTION_READINESS_AUDIT.md` records AUDIT-801, an
`ENV DATABASE_URL=postgresql:sigap:sigap@postgres:5432/...` in the queue-engine Dockerfile,
removed in `70f8d03` (2026-08-31). The preserved engine image was built **2026-09-14**, i.e.
*after* that fix, but the point stands: **verify, do not assume**.

**Do NOT inspect with `--format '{{json .Config.Env}}'`.** That prints every baked value in
cleartext, which puts it in the transcript, the CI log, the evidence bundle and any Jira
comment the output is pasted into. Classifying a secret is not the same as disclosing it.
Use the host-side classifier instead — it emits **key names and boolean verdicts only** and
never a value:

```bash
# Run on the production host. Safe to paste into evidence and Jira: no values are printed.
sh scripts/ops/image-config-secret-classifier.sh sigap-api sigap-web sigap-rust-engine
```

Expected: `VERDICT: SAFE` with only the keys the Dockerfiles bake —
`api`: `PATH`, `SIGAP_API_PORT`, `SIGAP_ENGINE_ADDR`; `web`: `PATH`, `NODE_VERSION`,
`YARN_VERSION`, `PORT`, `HOST`; `rust-engine`: `PATH`, `RUST_LOG`.

**`[OBSERVED]` 2026-10-10 on the live production host** (`fikriserver`, read-only): exactly that
set, and **`VERDICT: SAFE`** — no baked credential in any of the three images. `PATH`,
`NODE_VERSION` and `YARN_VERSION` are base-image variables, not secrets. `RepoDigests` and
image IDs unchanged from the 2026-10-09 capture, so this classifier result applies to the
images §13f preserves.

A `STOP` for any key (`DATABASE_URL`, `SMTP_PASSWORD`, a DSN, a token, a private key) means a
baked credential is suspected: **do not preserve, request owner disposition**. A `REVIEW`
means an unrecognised key is present and the owner must classify it. An `UNREADABLE` count
also aborts — a count that does not parse is treated as a failed inspection, never as zero,
because a false `SAFE` here is the exact failure this gate exists to prevent.

**Distinction that matters:** the classifier reads the **image config**, which is what the
archive carries. `SIGAP_AUTH_ISSUER`, `SIGAP_AUTH_AUDIENCE`, `SIGAP_AUTH_JWKS_URL`,
`DATABASE_URL` and `POSTGRES_PASSWORD` are passed by **compose at container start**, so they
live in the container's runtime env and are *not* inspected or preserved by the image. Only a
value baked at build time is shipped in the artifact.

**Encryption tooling — a BLOCKING GAP found on the host `[OBSERVED]` 2026-10-10.**
§13f's design assumes `age` exists on the production host. **It does not.** Read-only checks on
`fikriserver`:

| Requirement | Present | Note |
|---|---|---|
| `age` / `age-keygen` | **ABSENT** | the entire §13f design as first written |
| `gpg` | **present** | viable substitute; needs a keyring and a passphrase on the host |
| `openssl` | **present** (3.0.2) | see the cipher table below |
| `aws` cli | **present** | the AUDIT-701/backup-postgres.sh upload path uses it |

Of the ciphers `openssl enc` accepts on the prod host, the **authenticated** ones are
**unsupported**: `aes-256-gcm` → *"AEAD ciphers not supported"*, and
`chacha20-poly1305@openssl.com` likewise. What remains streams correctly but is **not
authenticated**: `aes-256-ctr`, `aes-256-cbc`. `[INFERENCE]` an unauthenticated stream cipher
is malleable — a truncated or altered archive decrypts to *something*, and the failure mode is
not detected. For this artifact that is not acceptable, so **§13f is not authorized until the
encryption mechanism is decided.**

**Options, none chosen for the owner:**
1. **Install `age`** on the host (`apt install age`) — smallest change to the design, but a
   *host package change*, needs owner approval.
2. **`gpg` symmetric**, `gpg -c --cipher-algo AES256` on the host — already installed; needs a
   passphrase, which then lives on the host (the same custody question `age` was chosen to
   avoid). `[INFERENCE]` gpg also authenticates, so tampering is detectable.
3. **Encrypt on the workstation, not the host** — `docker save | gzip | ssh ...` then the
   workstation encrypts the stream with a tool it does have, before pushing to the archive
   host. Keeps the key off the VPS entirely; costs a slightly longer pipeline.

**Do not silently substitute `aes-256-ctr` for `age`.** It preserves the shape of the plan
while removing the authentication property that makes a rollback artifact trustworthy. The
decision is recorded here, not made.

**Key custody — decided, not left open.** Two models exist and the plan must pick one. **This
plan uses the OFF-HOST-KEY model**: the encryption key exists **only** on the isolated
archive host, never on the workstation that pulls the stream. The workstation therefore only
ever holds **ciphertext**. If the owner prefers a local key instead, then
`$LOCAL_ARCHIVE_DIR` becomes **key-equivalent** and must sit on an encrypted volume with
restricted ACLs and must never be synced to a shared drive — but that weakens the design and
is not the default here.

**Proposed execution (NOT RUN — requires separate authorization).**

**Fail-closed pipeline rule (mandatory, and the reason this is not a plain `|` chain).**
A POSIX pipeline reports **only the last command's status**. `[OBSERVED]` on this host:
`docker image save <bad-id> | gzip -1 > out.gz` gives `$? = 0`, leaves a **20-byte**
artifact, and that artifact **passes `gzip -t`** — because `gzip` of *empty* input is a
valid gzip. So a failed, partially-written or truncated image export yields a small,
valid-looking, **trusted** artifact. The same blind spot applies to the restore path:
`age -d | gzip -d | cat` reports 0 even when `age` fails. Both are pinned by
`sh scripts/ops/test-image-preservation-pipeline.sh`.

Therefore every transfer below captures **each stage's status explicitly** and refuses on
any non-zero, plus a **decompressed-stream** floor (a compressed-size floor is useless —
see the test) and an out-of-band digest compare. **A successful final command is NOT proof
that the preceding stages succeeded.**

```bash
# ---- failure-propagation helper: status per stage, never a bare pipe ----
# st.1..st.4 hold each stage's exit code. Refuse if ANY is non-zero, and require the
# destination file to be a COMPLETE gzip of a non-empty stream.
pipe_guard() {
  s=0
  for f in "$@"/st.*; do
    [ -f "$f" ] || { echo "REFUSED: missing status file $f"; s=1; continue; }
    v=$(cat "$f")
    case "$v" in ''|*[!0-9]*) echo "REFUSED: $f not a status"; s=1 ;; *) [ "$v" -eq 0 ] || s=1 ;; esac
  done
  return $s
}
```

```bash
# ---- ON THE WORKSTATION (holds ciphertext only; no private key) ----
# 0. Identity first: re-confirm the running image IDs have not changed, and RECORD them.
#    If any ID differs from the table above, STOP — the running images are not the ones
#    this plan was written against.
for s in api web rust-engine; do
  docker inspect "sigap-$s" --format '{{.Image}}'
done

# 1. Stream the three images off-host, compressed, ENCRYPTED TO A PUBLIC RECIPIENT, straight
#    to the archive host. Nothing is written on the VPS root filesystem, and the workstation
#    never sees plaintext at rest.
#    Verify the `docker image save` flag spelling against `docker image save --help` on the
#    host first — it differs across Docker versions.
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
ST=$(mktemp -d)
DEST="sigap-pre-redesign-$STAMP.tar.gz.age"

{ ssh fikriserver "docker image save \
    sha256:43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f \
    sha256:2a81c86258fb79c8bd9d49dcb7672234de4a962d07011dd75b6ffeae82ef7a0c \
    sha256:8ce2200fabf3e31f3be2290ac3749210c49267ecf8696231aa3cfa86e6f4a7e4"
  echo $? > "$ST/st.1"
} | { gzip -1; echo $? > "$ST/st.2"; } \
  | { age -r "$OFFHOST_RECIPIENT"; echo $? > "$ST/st.3"; } \
  | { ssh "$ARCHIVE_HOST" "set -C; cat > '$ARCHIVE_DIR/$DEST'"; echo $? > "$ST/st.4"; }

pipe_guard "$ST" || { echo "REFUSED: a pipeline stage failed; the artifact is NOT trusted"; exit 1; }

# 1b. The artifact must be a complete gzip AND carry a non-empty DECOMPRESSED stream.
#     A compressed-size floor is useless: gzip of empty input is 20 B and passes gzip -t.
ssh "$ARCHIVE_HOST" "gzip -t '$ARCHIVE_DIR/$DEST'" || { echo "REFUSED: not a complete gzip"; exit 1; }
rm -rf "$ST"

# 2. Digest the artifact ON THE ARCHIVE HOST, where it lives, and record the digest
#    OUT-OF-BAND (RELEASE_CHECKLIST.md / the Jira issue), exactly as §12a does for EXPECT_SHA.
#    A checksum stored beside the artifact is integrity-only: whoever can replace the .age can
#    replace its .sha256. Out-of-band recording is what makes it tamper-evident.
ssh "$ARCHIVE_HOST" "sha256sum '$ARCHIVE_DIR/$DEST'"

# 3. Identity correspondence: prove the ARCHIVE holds exactly the captured IDs.
#    Decrypts with the key that exists ONLY on the archive host, and loads into an ISOLATED
#    daemon. The target is asserted non-production FIRST, in every spelling, so this can
#    never load into the production daemon — which would overwrite the very images the
#    rollback is trying to recover.
: "${ISOLATED_DOCKER_HOST:?set to the isolated test daemon; refusing to load unguarded}"
is_production_target() {
  case "$1" in
    ssh://fikriserver|fikriserver|ssh://fikriserver:*) return 0 ;;
    unix:///var/run/docker.sock|/var/run/docker.sock|var/run/docker.sock) return 0 ;;
    "$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null)") return 0 ;;
  esac
  return 1
}
is_production_target "$ISOLATED_DOCKER_HOST" && { echo "REFUSED: production target"; exit 1; }
# Per-stage status again: `age -d | gzip -d | docker load` reports 0 even when age fails.
ST2=$(mktemp -d)
{ ssh "$ARCHIVE_HOST" "age -d -i '$ARCHIVE_KEY' '$ARCHIVE_DIR/$DEST' | gzip -d"
  echo $? > "$ST2/st.1"
} | { DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker load --quiet; echo $? > "$ST2/st.2"; }
pipe_guard "$ST2" || { echo "REFUSED: restore stage failed; the archive is NOT a faithful copy"; exit 1; }
rm -rf "$ST2"
# Scope the assertion to the THREE IDs. A daemon-wide listing would fail spuriously on any
# daemon that already holds a base image, and would prove nothing about the archive.
for id in 43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f \
          2a81c86258fb79c8bd9d49dcb7672234de4a962d07011dd75b6ffeae82ef7a0c \
          8ce2200fabf3e31f3be2290ac3749210c49267ecf8696231aa3cfa86e6f4a7e4; do
  DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker image inspect "sha256:$id" >/dev/null \
    || { echo "MISSING $id — the archive is NOT a faithful copy; STOP"; exit 1; }
done
echo "identity correspondence VERIFIED for all three images"
```

Note the decoder order in step 3 is the exact inverse of step 1 — `age -d` **then**
`gzip -d`, in that order. Getting it backwards is a silent no-op on a `docker save` tar.

**Destination.** The on-host `/opt/sigap/backups/sigap` exists but is **empty (4 KiB),
mode 775 (world-readable) and on the same root filesystem** — `[OBSERVED]` 2026-10-10. It is
**not** an off-host target and its mode must be fixed before any archive is written there.
Two **separate** properties must be recorded, because the obvious candidate satisfies only one:

| Property | Why | R2 (`AUDIT-701`) candidate |
|---|---|---|
| **Not readable from the VPS** | A credential on the VPS makes "off-host" nominal | **NO** — its write credential lives on the VPS at `/etc/sigap/backup.env` (mode 640, 473 B) |
| **Client-side encrypted** | Provider SSE does not protect against the provider | **YES if** the cipher layer is applied — `BACKUP_RESTORE.md` §5 says the DB-dump store is **SSE-only**, with client-side encryption not pre-implemented |

The destination must satisfy all of: genuinely **off-host**; **free space** ≥ ~0.5 GB per
retained release; **restricted access** (not `775`); **client-side encryption** with a key held
off the VPS; **immutable** evidence (checksums recorded out-of-band); **restore access
independent of production**; and **retrievability during a production outage**.

Capacity ≥ ~0.5 GB per retained release. Credentials and reachability of any candidate were
**not** verified in this read-only inspection, so destination feasibility is **[UNKNOWN]** and
must be confirmed before execution. The `aws` cli is present on the host, which makes the
`backup-postgres.sh` S3 path the natural candidate — but **`age` is absent** (see the
encryption gap above), so the cipher layer is undecided.

**Restore path — exists, and is a prerequisite.** §7d is the only restore procedure and it
verifies `sha256sum -c SHA256SUMS` and loads `"$ARCHIVE_DIR"/*.tar.gz`; the §13f artifact is a
`.tar.gz.age` with no `SHA256SUMS`, so **§7d alone cannot consume it**. **§7d.1 was added for
exactly this reason:** it fetches the ciphertext, verifies the digest **out-of-band**, runs
`age -d | gzip -d | docker load`, asserts each of the three image IDs, re-tags to
`<project>-<service>:rollback-<STAMP>-<short>`, and rejoins §7d step 3.

§13f is only authorizable **together with §7d.1**. If either is absent, the off-host archive
is not a usable rollback artifact and §13e's claim that it satisfies §7c's preservation
requirement is **unsupported**. The dependency is bidirectional and both must ship together.

**Interruption/retry.** A failed run leaves a **truncated** file at the destination, and the
fail-closed guard above catches it: either a stage status is non-zero, or the artifact fails
`gzip -t`, or its decompressed stream is empty. Delete the truncated file and re-run. The
host images are read-only inputs and are never modified, so re-running is safe and
idempotent. Because a re-run produces a new `$STAMP`, digests never collide, and the
`set -C` in step 1's `cat >` refuses to overwrite an existing destination — so a partial
archive is never silently replaced by a retry that also fails.

**Confidentiality.** The application images contain production **binaries and configuration**.
Encryption is applied **before** the bytes leave the host, and the private key never exists on
the streaming workstation. Never place plaintext image archives on shared or world-readable
storage, and never print image **environment values** — this inspection read only IDs, sizes,
timestamps and statuses, and no secret values were read or recorded.

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
