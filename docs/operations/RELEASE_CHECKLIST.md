# Sigap Redesign — Release Checklist

**Phase:** 3B7 (T-3B7-03) · **Companion to:** `docs/operations/DEPLOYMENT_RUNBOOK.md`
**Rule:** a box may be ticked only when its evidence is recorded here. Boxes marked
**OPERATOR** cannot be ticked from the repository — they are ticked during the deployment.

Legend: `[x]` verified in-repo (with evidence) · `[ ]` not yet done · **OPERATOR** = requires
the live environment.

---

## A. Release identity

| | Item | Evidence | Status |
|---|---|---|---|
| A1 | Correct release commit selected | `git rev-parse HEAD` → the branch tip, recorded as `RELEASE_CANDIDATE_HEAD`. The deployable artefact set is defined by the **commit actually built**, not by a hard-coded historical hash; record the exact value at build time. | `[ ]` |
| A2 | Branch is `design/ui-ux-overhaul` | `git rev-parse --abbrev-ref HEAD` | `[x]` |
| A3 | **Tracked** paths clean at the release commit | `git status --porcelain --untracked-files=no` → empty. Untracked paths are **classified, not ignored** (runbook §2a): an unexpected untracked path is a hard abort; the known sensitive artifact is recorded with an explicit disposition. **The host tree is reported as *not fully clean* until that disposition is resolved — do not fabricate a clean-tree PASS.** | `[ ]` |
| A4 | Migration provenance classified (A–F) | `git diff --stat origin/main..HEAD -- packages/db/migrations` → **empty** (source agrees with main). `git diff --stat 6d7f940..HEAD -- packages/db/migrations` → `0006_notifications.sql \| 4 ++--`, **non-empty by design** (inherited upstream hardening `9d4e68e`, runbook §1 **C**). **Neither diff proves the deployed schema** — `DATABASE_SCHEMA_COMPATIBILITY` stays UNKNOWN until §12 runs. **No migration PASS may be claimed from these diffs.** | `[x]` (diffs recorded) |
| A5 | Seed changes reviewed (seeds are **not** empty — they were extended for local test identities) | `git diff --stat 6d7f940..HEAD -- packages/db/seed` → `dev.sql`, `demo.sql` (synthetic local identities/fixtures only; seeds are never applied in production — `postgres` mounts `packages/db/migrations`, not `packages/db/seed`) | `[x]` |
| A6 | `.env` backup artifact preserved, not staged | `/opt/sigap/.env.bak-phase5` present (mode `600`), **never** staged/moved/deleted, never `git clean`; `.gitignore` now matches it (`git check-ignore -v .env.bak-phase5`) while `.env.example` stays trackable | **OPERATOR** |

## B. Gate evidence

| | Item | Evidence | Status |
|---|---|---|---|
| B1 | Gate 3 PASS | recorded in `design/sigap-redesign-task-breakdown.md` | `[x]` |
| B2 | Gate 5 PASS | commit `0270bbd` ("test(gate5): close admin mutation evidence coverage") | `[x]` |
| B3 | Gate 6 PASS | `design/` phase record + this phase's verification run | `[x]` |
| B4 | Gate 6 report accepted | asserted by the 3B7 phase record. **The 40-item Gate 6 report is NOT a committed artefact** — it is not in `design/`, `docs/`, or the repo; only this checklist references it. The repository-side evidence for the same claim is: `design/sigap-redesign-task-breakdown.md` (gate task definitions + canonical order), the commit trail (`0270bbd` GATE 5, `c19c0a6` 3B6), and the phase's own verification runs. Treat B4 as **recorded, not independently re-verifiable from the repository**. | `[x]` (recorded) |

## C. Environment configuration (names only — never values)

| | Item | Expected | Status |
|---|---|---|---|
| C1 | `SIGAP_ENV` set | **not `local`** — recommend exactly `production`. The API guard compares case-INSENSITIVELY (`strings.EqualFold`), so any case-variant of `local` (e.g. `Local`) is treated as local and would permit the dev flags below. | **OPERATOR** |
| C2 | `SIGAP_DEV_IDENTITY` unset/false | unset or `false` | **OPERATOR** |
| C3 | `SIGAP_AUTH_MODE` | `jwt` | **OPERATOR** |
| C4 | `SIGAP_AUTH_ISSUER` / `_AUDIENCE` / `_JWKS_URL` | set (jwt mode) | **OPERATOR** |
| C5 | `PUBLIC_SUPABASE_URL` configured | set (publishable) | **OPERATOR** |
| C6 | `PUBLIC_SUPABASE_ANON_KEY` configured | set (publishable; **never** a service-role key) | **OPERATOR** |
| C7 | `SIGAP_API_INTERNAL` configured | resolves to the API (`http://api:8080`) | **OPERATOR** |
| C8 | `SIGAP_DATABASE_URL` configured | set | **OPERATOR** |
| C9 | `SIGAP_WEB_ORIGIN` configured | the real public origin | **OPERATOR** |
| C10 | `SIGAP_TLS_TERMINATED` | `true` behind TLS | **OPERATOR** |
| C11 | `SIGAP_TRUSTED_PROXIES` | `2` (edge + SvelteKit proxy) | **OPERATOR** |
| C12 | Local identity path inactive | `SIGAP_LOCAL_E2E_ACTOR` and `SIGAP_LOCAL_RBAC_TEST_IDENTITY` **unset** | **OPERATOR** |
| C13 | `SIGAP_AUTO_MIGRATE` | unset/`false` (no migration ships) | **OPERATOR** |
| C14 | `make db-seed` would refuse | `make db-seed` with a non-`local` `SIGAP_ENV` exits 1: "Refusing to run demo seeds unless SIGAP_ENV=local" (`Makefile` `db-seed`). Verified against the guard predicate: non-local → refuse, `local` → proceed. | `[x]` |
| C15 | Production overlay refuses an unset/empty `SIGAP_ENV` | `docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml config` with `SIGAP_ENV` unset → interpolation error **before** any container starts (G10/G11) | `[x]` |
| C16 | Production env gate passes a clean env | `SIGAP_DEPLOY_DIR=. SIGAP_ENV=staging SIGAP_AUTH_MODE=jwt sh scripts/ops/preflight-production-env.sh` → exit 0. **`SIGAP_DEPLOY_DIR` is required by the gate** (runbook §7a); without it the gate correctly exits 1 | `[x]` |
| C17 | Compose version ≥ 2.24.4 | `docker compose version` (the overlays use `!override`) | **OPERATOR** |
| C18 | Production env gate run on the host with the REAL env | `sh scripts/ops/preflight-production-env.sh` on the deploy host → exit 0 | **OPERATOR** |
| C19 | Dev-only capabilities absent from the running containers | `docs/operations/OPERATOR_EVIDENCE_BUNDLE.md` §4/§5: `SIGAP_DEV_IDENTITY` **ABSENT**, `SIGAP_ENV` not `local` — a three-state PRESENT/ABSENT/UNKNOWN result per **exact** container name | **OPERATOR** |

## D. Build artifacts

| | Item | Evidence | Status |
|---|---|---|---|
| D1 | Go API builds | `make build` → `apps/api/bin/sigap-api` | `[x]` |
| D2 | Rust engine builds (release) | `cargo build --release` → `apps/queue-engine/target/release/sigap-queue-engine` | `[x]` |
| D3 | Web builds | `pnpm --filter sigap-web build` | `[x]` |
| D4 | Dockerfiles present for all three services | `apps/api/Dockerfile`, `apps/queue-engine/Dockerfile`, `apps/web/Dockerfile` | `[x]` |

## E. Smoke — rehearsal against a production-equivalent LOCAL stack

Rehearsal only; see `DEPLOYMENT_RUNBOOK.md` §4a. All checks are in
`sigap-production-readiness-smoke.ps1` unless noted.

| | Item | Evidence | Status |
|---|---|---|---|
| E1 | Smoke target confirmed NON-production during rehearsal | `-ApiBase`/`-WebBase` loopback; the script refuses anything else before a request | `[x]` |
| E2 | Critical web routes smoke green (availability) | steps 1–7: `/`, `/faskes`, `/queues/new`, `/admin`, `/appointments/new`, `/appointments/check-in`, `/patient/status` → 200 + SSR marker | `[x]` |
| E3 | Public catalog green | step 8 → 200 + non-empty `data` | `[x]` |
| E4 | **P0-1 positive control**: in-scope retry → 200 **and** the row mutates | steps 9–10 | `[x]` |
| E5 | **P0-1**: cross-facility retry → 404 | step 11 | `[x]` |
| E6 | **P0-1**: whole-row state unchanged (before == after) | step 12 (`row_to_json` comparison) | `[x]` |
| E7 | **P0-3**: zero-assignment actor → all five declared status keys present and zero | step 13 | `[x]` |
| E8 | **§4.5 contrast**: global super_admin → non-zero global counts | step 14 | `[x]` |
| E9 | Admin queue status transition persists | step 15 (walk-in → waiting→called) | `[x]` |
| E10 | Admin facility deactivate sets `is_active=false` | step 16 (disposable facility) | `[x]` |
| E11 | Negative controls A–D prove the new assertions can fail | see §G | `[x]` |

> **`/admin` is an ANONYMOUS shell check (step 4), not a session/counts check.** The admin
> layout deliberately carries no session, role or permission check
> (`apps/web/src/routes/admin/+layout.svelte`), so a 200 + `sigap-admin-shell` is the correct
> expected behaviour; the plan's "with a scoped admin session … counts render" expectation is
> covered by the Playwright suite (`apps/web/e2e/admin-read.spec.ts`), not by this smoke step.

## F. Rollback readiness

| | Item | Evidence | Status |
|---|---|---|---|
| F1 | Rollback lineage understood | runbook §6a: `HISTORICAL_PHASE_ROLLBACK_POINTER` is **NOT** a production rollback target — every pointer predates the `origin/main` security merge (`6c0ac82`), so using one reverts four security commits | `[x]` |
| F2 | Rollback commands documented | runbook §7d (restore preserved images) — the legacy rebuild path is marked superseded (§7e) | `[x]` |
| F3 | No DB restore is required **for this rollback path** | runbook §6c. This is a statement about the rollback path only — it does **not** establish deployed schema compatibility, which stays UNKNOWN until the DB inspection (runbook §12) completes | `[x]` (path) / **OPERATOR** (schema) |
| F4 | Rollback uses the SAME overlay set as deploy (base + prod-ports + prod-edge) | runbook §7d | `[x]` |
| F5 | Edge sanity check in the rollback path (web loopback-only + serves through the edge) | runbook §7d | `[x]` |
| F6 | **`CURRENT_PRODUCTION_IMAGE_ID` captured per service** (full `sha256:` id, by exact container name) | runbook §7a; `OPERATOR_EVIDENCE_BUNDLE.md` §4 | **OPERATOR** |
| F7 | **`CURRENT_PRODUCTION_VERSION` captured** (and not equal to the release being deployed) | runbook §7a | **OPERATOR** |
| F8 | **`SIGAP_DEPLOY_DIR` recorded and proven** | runbook §7a/§7b | **OPERATOR** |
| F9 | **`PRESERVED_ROLLBACK_IMAGE_REF` created per service from the running image IDs** (never from `latest`) | runbook §7c | **OPERATOR** |
| F10 | **Rollback archives checksummed** (`sha256sum -c SHA256SUMS` passes) | runbook §7c/§7d | **OPERATOR** |
| F11 | **Rollback restore exercised/verified** (re-tag or load, `up -d --no-deps rust-engine api web`, exact-ID match) | runbook §7d steps 1–6 | **OPERATOR** |
| F12 | **Postgres NOT restarted during rollback** | runbook §7d step 4 (`up -d --no-deps`, no `postgres`) | **OPERATOR** |
| F13 | **Compose identity preserved** (project `sigap`; volumes `sigap_pgdata`/`sigap_pgcerts`; networks `sigap_default`/`traefik`; loopback-only `18080`/`5433`/`50051`/`3005`) | runbook §7d; evidence bundle | **OPERATOR** |

## G. Negative controls (non-vacuity of the new smoke assertions)

Each control was executed against the local production-equivalent stack; the observed result is
recorded verbatim so the claim is auditable.

| | Control | Method | Observed result | Status |
|---|---|---|---|---|
| G1 | Retry assertion fails if a mutation is permitted | `-OtherFacilityId` set to the facility the scoped subject MANAGES, so the probe row is in scope and the retry is permitted | `[FAIL] p0.retry.out_of_scope.status` — expected 404, got 200; suite exit 1 | `[x]` |
| G2 | No-write assertion fails if the row mutates | same run | `[FAIL] p0.retry.out_of_scope.no_write` — "ROW CHANGED after a denied retry"; suite exit 1 | `[x]` |
| G3 | Route smoke fails on an unreachable route | `-WebBase http://127.0.0.1:9` (closed port) | all 7 `web.route.*` steps `[FAIL]`; suite exit 1 | `[x]` |
| G4 | Production-target safety fails for a forbidden HTTP target | `-ApiBase https://sigap.chaerulchalik.web.id` | `[FAIL] parameters` — refusing `sigap.chaerulchalik.web.id`; exit 2, no request made | `[x]` |
| G5 | Summary assertion fails on non-zero data | `-ZeroScopeSubject` set to a subject that HAS scope | `[FAIL] p0.summary.zero_assignment` — non-zero counts observed | `[x]` |
| G6 | Database-target safety fails for a forbidden DB host | `-DatabaseUrl postgresql://u:p@db.prod.example.com:5432/sigap` | `[FAIL] parameters` — "DatabaseUrl must point at a LOOPBACK host"; exit 2, no DML run | `[x]` |
| G7 | Re-run is idempotent (no false abort from the walk-in rate limit) | run the suite twice in a row | both runs `18/18`, exit 0; probe residue 0 after each | `[x]` |
| G8 | `selector_armed` aborts on an UNARMED API (fail-closed 403) | stub API answering 403 on `/api/v1/admin/notifications/summary` (loopback, no DB) | `[FAIL] precondition.selector_armed` — "not resolved (HTTP 403, expected 200); the local test identity selector is not armed"; exit 2 **before** `fixtures.create` | `[x]` |
| G9 | `selector_armed` passes on an ARMED API (200) | same stub answering 200 | `[PASS] precondition.selector_armed` — "resolved (HTTP 200)"; no parameter abort | `[x]` |
| G10 | Production compose refuses an unset `SIGAP_ENV` | `docker compose … -f docker-compose.prod-ports.yml config` with `SIGAP_ENV` unset | interpolation error before any container starts | `[x]` |
| G11 | Production compose refuses an empty `SIGAP_ENV` | same, `SIGAP_ENV=` | interpolation error | `[x]` |
| G12 | Production env gate rejects `SIGAP_ENV=local` (and `Local`/`LOCAL`) | `preflight-production-env.sh` (with `SIGAP_DEPLOY_DIR=.` exported, so the rejection is attributable to `SIGAP_ENV`, not to a missing directory) | exit 1 | `[x]` |
| G13 | Production env gate rejects `dev`/`disabled`/unknown auth mode | `preflight-production-env.sh` with each mode, `SIGAP_DEPLOY_DIR=.` exported | exit 1 | `[x]` |
| G14 | Production env gate rejects dev-only flags | `SIGAP_DEV_IDENTITY=true`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`, `SIGAP_DEPLOY_DIR=.` exported | exit 1 | `[x]` |
| G15 | Production env gate accepts a clean production env | `SIGAP_DEPLOY_DIR=. SIGAP_ENV=staging SIGAP_AUTH_MODE=jwt` | exit 0 | `[x]` |

> G1–G9 are **rehearsal evidence** on the local production-equivalent stack (G8/G9 use a
> loopback stub; no database). G10–G15 are the fail-closed env guards, re-run by
> `scripts/ops/test-production-deploy-guards.sh` (**28 pass / 0 fail**) and
> `scripts/ops/test-preflight-production-env.sh` (**18 pass / 0 fail**). G1/G2 are also
> covered in-process by the scripted positive control (steps 9–10), which performs the same
> proof and cleans up.

## H. Post-deploy verification (during the real deployment)

| | Item | Check | Status |
|---|---|---|---|
| H1 | No 5xx increase | API logs | **OPERATOR** |
| H2 | Deny-by-default 401 rate unchanged | API logs | **OPERATOR** |
| H3 | No new 403s for scoped admin identities | API logs | **OPERATOR** |
| H4 | Admin Antrean last-updated advances | UI poll | **OPERATOR** |
| H5 | Edge certificate valid | `openssl s_client` / Traefik dashboard | **OPERATOR** |
| H6 | API boot printed no dev-flag guard error | `docker compose logs api \| grep -i 'dev-only capabilities'` → no match | **OPERATOR** |
| H7 | Operator production config verification completed | runbook §11 O1–O9 | **OPERATOR** |
| H8 | Production deployment separately authorized | change ticket / approval | **OPERATOR** |
| H9 | Business-data presence recorded (separate from health) | `curl -fsS https://<host>/api/v1/public/facilities` → count active facilities; record the number and the product-owner acknowledgement | **OPERATOR** |
| H10 | Release identity changed for every rebuilt service | `/_app/version.json` (web) + `docker compose images api web` digests | **OPERATOR** |

## I. Standing constraints

| | Item | Status |
|---|---|---|
| I1 | No URL renamed | `[x]` |
| I2 | No migration **added or edited** by the redesign | `[x]` — no redesign commit adds or edits a `packages/db/migrations/*` file. The only in-range change is the **inherited** upstream hardening `9d4e68e` to `0006_notifications.sql` (runbook §1 **C**), which is **not** redesign work. **`0006` is never edited again** — any correction is a new forward-only migration. |
| I3 | No new dependency added without documented reason | `[x]` |
| I4 | No production deployment performed as part of a gate | `[x]` |
| I5 | `/wallet` retained and excluded from navigation (decision D7) — an **intentional deferred route**, not an accidental orphan | `[x]` |

## J. Blocking gates (all must be closed before deploy)

Every item below is **BLOCKING** and must remain **unchecked** until its evidence is
recorded. "Unknown" is never a PASS.

| | Item | Evidence | Status |
|---|---|---|---|
| J1 | **DB metadata inspection authorized** | explicit operator approval for read-only DB access (runbook §12) | **OPERATOR** |
| J2 | **DB metadata inspection completed** | `scripts/ops/db-metadata-inspection.sql` executed; output recorded | **OPERATOR** |
| J3 | **DB schema classification recorded** | exactly one of `MATCHES_CURRENT_SECURITY_CONSTRAINTS` / `OLDER_WEAKER_CONSTRAINTS` / `MISSING_CONSTRAINTS` / `UNEXPECTED_DRIFT` / `UNKNOWN` (runbook §12). **`UNKNOWN` blocks the deploy.** | **OPERATOR** |
| J4 | **Drift disposition recorded** | if the class is not `MATCHES_CURRENT_SECURITY_CONSTRAINTS`: explicit operator decision (accept with a recorded compensating control, or remediate forward with a **new** migration) | **OPERATOR** |
| J5 | **Disk capacity gate passed** | runbook §13a — rollback archives present **and verified**; build peak evaluated; ≥10% margin after both; **no** `prune`/`rmi` before preservation | **OPERATOR** |
| J6 | **Untracked-file disposition recorded** | runbook §2a — every untracked path classified; `.env.bak-phase5` retained in place with owner/mode/size/mtime; **no** `git clean` | **OPERATOR** |
| J7 | **Release identity validated post-deploy** | `/_app/version.json` changed for every rebuilt service; `docker compose images api web` digests recorded (runbook §3.0/§3.8) | **OPERATOR** |
| J8 | **Protected smoke requires SEPARATE authorization** | any probe of a protected route writes an `audit_events` row (runbook §8a). A protected-route smoke against production is **NOT** covered by this checklist and needs its own explicit approval. | **OPERATOR** |
| J9 | **Production env gate passes on the host** | C18 — `preflight-production-env.sh` exit 0 against the real environment | **OPERATOR** |

---

## Operator command reference

```bash
# C1/C2/C3/C12 — inspect the deployed API/web environment (names only, never echo secrets)
docker compose exec api env | grep -E '^SIGAP_(ENV|DEV_IDENTITY|AUTH_MODE|LOCAL_E2E_ACTOR|LOCAL_RBAC_TEST_IDENTITY)=' | sort
docker compose exec web env | grep -E '^(SIGAP_ENV|PUBLIC_SUPABASE_URL|SIGAP_API_INTERNAL)=' | sort

# H6 — confirm the API did not fall back to a dev capability
docker compose logs api | grep -i 'dev-only capabilities' && echo 'ABORT: dev flag present' || echo 'OK'

# E1 — confirm the smoke target is loopback during rehearsal
echo "$SIGAP_API_BASE" "$SIGAP_WEB_BASE"

# J1–J4 — DB metadata inspection. READ-ONLY, METADATA ONLY.
# *** NOT AUTHORIZED. Requires explicit operator approval before running. ***
# Runs against the running postgres container; selects only pg_catalog/schema_migrations.
docker exec -i sigap-postgres psql -U sigap -d sigap \
  -v ON_ERROR_STOP=1 --no-psqlrc -P pager=off -f - < scripts/ops/db-metadata-inspection.sql

# J5 — disk capacity gate (runbook §13a). Record before/after free space.
df -h /var/lib/docker

# A6 / J6 — confirm the sensitive backup is ignored but never staged.
git check-ignore -v .env.bak-phase5   # expected: matches .gitignore
git check-ignore -v .env.example      # expected: no output (exit 1 = NOT ignored)
git status --porcelain --untracked-files=no   # TRACKED must be empty
```
