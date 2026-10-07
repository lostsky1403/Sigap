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
| A1 | Correct release commit selected | `git rev-parse HEAD` → the branch tip. The **deployable release commit is `c19c0a6`** (3B6 — the last commit carrying a deployable artefact); everything after it (3B7 and any follow-up) is documentation and smoke only, so the deployable artefact set is unchanged by later doc commits. | `[x]` |
| A2 | Branch is `design/ui-ux-overhaul` | `git rev-parse --abbrev-ref HEAD` | `[x]` |
| A3 | Working tree clean at the release commit | `git status --porcelain` → empty at HEAD, verified after the 3B7 docs/scripts landed | `[x]` |
| A4 | Migration diff empty | `git diff --stat 6d7f940..HEAD -- packages/db/migrations` → empty | `[x]` |
| A5 | Seed changes reviewed (seeds are **not** empty — they were extended for local test identities) | `git diff --stat 6d7f940..HEAD -- packages/db/seed` → `dev.sql`, `demo.sql` (synthetic local identities/fixtures only; seeds are never applied in production — `postgres` mounts `packages/db/migrations`, not `packages/db/seed`) | `[x]` |

## B. Gate evidence

| | Item | Evidence | Status |
|---|---|---|---|
| B1 | Gate 3 PASS | recorded in `design/sigap-redesign-task-breakdown.md` | `[x]` |
| B2 | Gate 5 PASS | commit `0270bbd` ("test(gate5): close admin mutation evidence coverage") | `[x]` |
| B3 | Gate 6 PASS | `design/` phase record + this phase's verification run | `[x]` |
| B4 | Gate 6 report accepted | the Gate 6 final report (40 items) | `[x]` |

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
| F1 | Rollback commit identified per phase | `docs/operations/DEPLOYMENT_RUNBOOK.md` §6 | `[x]` |
| F2 | Rollback commands documented | runbook §7 | `[x]` |
| F3 | DB restore **not** required | runbook §6, verified empty migration diff | `[x]` |
| F4 | Previous images still pullable | registry check | **OPERATOR** |

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

> G1–G7 are **rehearsal evidence** on the local production-equivalent stack, re-run against the
> final script. G1/G2 are now also covered in-process by the scripted positive control
> (steps 9–10), which performs the same proof and cleans up after itself.

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

## I. Standing constraints

| | Item | Status |
|---|---|---|
| I1 | No URL renamed | `[x]` |
| I2 | No migration introduced | `[x]` |
| I3 | No new dependency added without documented reason | `[x]` |
| I4 | No production deployment performed as part of a gate | `[x]` |
| I5 | `/wallet` retained and excluded from navigation (decision D7) — an **intentional deferred route**, not an accidental orphan | `[x]` |

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
```
