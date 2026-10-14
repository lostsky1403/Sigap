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
| A4 | Migration provenance classified (A–F) | `git diff --name-only origin/main..HEAD -- packages/db/migrations` → **exactly** `0011_notification_outbox_phone_constraints.sql` (runbook §1 **A**). `git diff --stat 6d7f940..HEAD -- packages/db/migrations` → `0006_notifications.sql \| 4 ++--` **plus** `0011`, **non-empty by design** (inherited upstream hardening `9d4e68e` + the redesign's own `0011`, runbook §1 **C**). **Neither diff proves the deployed schema** — that is §12, whose result is `OLDER_WEAKER_CONSTRAINTS`. **No migration PASS may be claimed from these diffs.** | `[x]` (diffs recorded) |
| A5 | Seed changes reviewed (seeds are **not** empty — they were extended for local test identities) | `git diff --stat 6d7f940..HEAD -- packages/db/seed` → `dev.sql`, `demo.sql` (synthetic local identities/fixtures only; seeds are never applied in production — `postgres` mounts `packages/db/migrations`, not `packages/db/seed`) | `[x]` |
| A6 | `.env` backup artifact preserved, not staged | `/opt/sigap/.env.bak-phase5` present (mode `600`), **never** staged/moved/deleted, never `git clean`; `.gitignore` now matches it (`git check-ignore -v .env.bak-phase5`) while `.env.example` stays trackable | **OPERATOR** |

## B. Gate evidence

| | Item | Evidence | Status |
|---|---|---|---|
| B1 | Gate 3 PASS | recorded in `design/sigap-redesign-task-breakdown.md` | `[x]` |
| B2 | Gate 5 PASS | commit `0270bbd` ("test(gate5): close admin mutation evidence coverage") | `[x]` |
| B3 | Gate 6 E2E run — **this phase's evidence** | The **five-actor** local E2E suite was executed in this phase against the local seeded stack: `pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1` → **738 passed / 82 skipped / 0 failed** across all five actors (`e2e-schedule-manager` 154/10, `local-global-super-admin` 158/6, `e2e-schedule-mixed` 154/10, `e2e-schedule-reader` 152/12, `local-zero-scope-admin` 120/44), gate output `GATE 2 / GATE 3 E2E: PASS`. The **Go gate** (`go vet` + `go test ./...` against a migrated database), router coverage **90.6%**, `svelte-check` **0 errors 0 warnings**, web unit **699 tests / 34 files**, and Rust tests are all green at this revision. **The historical 40-item Gate 6 report itself is NOT a committed artefact** (see B4) — this row records the *current* run, not that report. | `[x]` (re-run in this phase) |
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
| C13 | `SIGAP_AUTO_MIGRATE` | unset/`false`. **Leave unset**: `0011` is applied only as a separate, explicitly authorized operator action (J10), never by container startup. | **OPERATOR** |
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
| F14 | **Gate A — R2 bucket created and secured** (private; **Bucket Lock rule** with ≥90-day retention added post-creation; incomplete-multipart abort rule) | runbook §13g.1, §13g.4 A1–A3 | **VERIFIED (configuration only)** 2026-10-13 — see **F14 evidence** below. `R2_BUCKET = OWNER_SETUP_PENDING` → **`CONFIGURATION_VERIFIED`** · `R2_BUCKET_EXISTS = PASS` · `R2_PRIVATE_ACCESS_CONFIGURATION = PASS` · `BUCKET_LOCK_RULE_VERIFIED = PASS` (inventory only) · `MULTIPART_LIFECYCLE_VERIFIED = PASS` |
| F15 | **Gate A — object credentials scoped to the bucket** (uploader = **Object Read & Write**, recovery = **Object Read**; bucket-configuration capability kept interactive and separate) | runbook §13g.3, §13g.4 A5–A7, A9 | **OPERATOR** |
| F15b | **Gate A — second-copy decision recorded** (second provider / offline copy / explicitly accepted risk) | runbook §13g.1 | **OPERATOR** |
| F16 | **Gate A — age key pair generated off-host and two recovery copies secured** (neither in Git, CI, R2, Jira or on the relay) | runbook §13h.2, §13h.4 C1–C6 | **OPERATOR** — `PRODUCTION_PRIVATE_KEY = NOT_GENERATED_BY_AGENT` |
| F17 | **Gate A — public recipient recorded and matched to the recovery identity** via `age-keygen -y` and a round-trip drill | runbook §13h.3, §13h.5 | **OPERATOR** |
| F18 | **Artifact URI and SHA-256 recorded out-of-band** (checklist + Jira gate, never a co-located sidecar in R2) | runbook §13g.7 | **OPERATOR** — `PRODUCTION_IMAGES_PRESERVED = NO` |
| F19 | **Isolated restore from R2 exercised with the REAL image IDs** via `sigap-archive-restore.sh` with `ARCHIVE_URI=r2://…` | runbook §7d.1, §13f step 3 | **OPERATOR** — `REAL_ISOLATED_RESTORE = NOT_RUN` |
| F20 | **`J11_ROLLBACK_SAFETY` resolved** — preserved binary tested against the post-`0011` schema (§7g), **not** inferred from image IDs | runbook §7f/§7g | **RESOLVED 2026-10-14 — `CONDITIONALLY_SAFE`, not `UNKNOWN`.** The older API was rebuilt from git (both candidate families, since the running revision is unproven) and driven over real HTTP against an isolated post-`0011` database. It writes fine for letter-only `short_code`s, but a facility whose `short_code` renders a separator-run queue number (e.g. `A1234567-89`) makes the check-in insert **refused by the strengthened CHECK** and the notification **lost silently**. That class also affects the **new** release, since `validateShortCode` guards create/update but does not retroactively validate facilities the old release created. **`scripts/ops/preflight-0011-rollback-safety.sql` is a REQUIRED read-only precondition before applying `0011` or relying on a rollback** | runbook **§7f** | **DONE (verdict)** — `MIGRATION_0011_AUTHORIZATION` remains `NOT_AUTHORIZED`; passing the pre-flight is **not** authorization |
| F21 | **Bucket Lock retention behaviour verified on the real service**, using a throwaway object — **not** by the MinIO CI test, which cannot prove it. **THREE things, recorded separately:** (1) `BUCKET_LOCK_CONFIG_VERIFIED = PASS` — the production rule's configuration, from the operator's live `lock list` (F14 evidence); (2) `DISPOSABLE_LOCK_MECHANISM_TEST = PASS` — **executed live 2026-10-13**, script exit **0**: a real disposable object was uploaded, a temporary 1-day rule created, `DeleteObject` rejected with `ObjectLockedByBucketPolicy`, the temporary rule removed, deletion then succeeded, no drill rule or object remained; (3) `PRODUCTION_RULE_UNCHANGED_AFTER_DRILL = PASS` — the production rule was unchanged before and after. **A verified configuration and a green mechanism test do NOT prove the rule *enforces* retention against a real archive: no object under `sigap/` was deleted or tested, and this must never be claimed.** Evidence dir `lock-drill-evidence/20261013T172242Z-29924d` | runbook **§13g.14** | **DONE** — all three verdicts `PASS`. The temporary drill credential should be **revoked by the owner** once evidence custody is complete; it must NOT become the production uploader identity |
| F21a | **The drill's own assertions are tested** — `scripts/ops/test-bucket-lock-drill.sh` runs it against a stub executor: the happy path; the failure paths (deletable locked object, refusal without a retention reason, rule that never applied, production rule changed); the guard refusals (ambient AWS credentials, non-R2 endpoint, extra host label, longer id, trailing dot, endpoint with path, malformed account id, wrong bucket, pre-existing drill state); **one regression case per preflight condition** — disabled rule, missing/empty `enabled`, wrong/empty/missing prefix, retention 89/1/100/365/plain/`N day`, retention missing/empty/malformed/date/indefinite, duplicate rule, contradictory record, empty rule name, mixed-rule attributes, differently-cased and superstring names; and **the drill-prefix listing** — the real R2 empty shape that omits `KeyCount`, a non-empty prefix, a paginated response whose object is on page 2, and refusal of `None`, empty output, malformed JSON, a JSON string/array, a `Contents` entry with no/empty `Key`, a non-numeric `KeyCount`, a `KeyCount` with no `Contents`, and `IsTruncated` without a token. Every refusal asserts the **specific reason** and the **absence of mutation**, not just the exit code. Wired into the `ops-guards` CI job | `scripts/ops/test-bucket-lock-drill.sh` | **DONE** — 169/169 checks pass locally under pwsh 7.6 and Windows PowerShell 5.1; no real bucket contacted |
| F21c | **CI test EXECUTED and PASSING** — the **latest** successful drill is run `38289010467` on **`d06a9a7`**, all 19 steps, including all four negative controls at their expected exit codes and refusal *reasons*. Earlier passing runs (`38281214221` on `c4afc1c`, `38280297031` on `89f9bcf`) are superseded by it. The drill **no longer re-runs on push**: the `r2-validation` label was removed once validation completed, so the job correctly reports *skipped* | runbook §13g.12 | **DONE** — `R2_MINIO_ROUND_TRIP = PASS`, `R2_NEGATIVE_CONTROLS = PASS` (MinIO only; **not** retention). The result is **historical**: it was produced on `d06a9a7`, which is no longer the branch head, and it is evidence that the gate is reachable and the round trip executes end to end — **not** a claim about any later commit |
| F21b | **CI test activated** — the `r2-validation` label created, a PR opened with base `design/ui-ux-overhaul`, and the label **applied** (the `labeled` activity type is what starts the run; without it the gate is unreachable) | runbook §13g.11 | **OPERATOR** — `GITHUB_TEST_ACTIVATION = READY`, `R2_RUNTIME_TEST = NOT_RUN` |
| F22 | **`PRODUCTION_ENGINE_ID` recorded** (`docker info .ID` on the production daemon) and exported for every restore | runbook §7d.1, §13f | **OPERATOR** — Gate D. The only alias-proof production control; without it an SSH tunnel or DNS alias of production defeats the string denylist |
| F23 | **A trusted relay host identified and reachable** — the `ARCHIVE_RELAY` that §13f streams through. It must have `age` (to encrypt with the **public** recipient), `aws` (to upload to R2) and `ssh` from production, plus `$RELAY_STAGE` writable under `umask 077`. It must **not** hold the private key | runbook §13f (prerequisites and step 1) | **OPERATOR** — `ARCHIVE_RELAY = NOT_IDENTIFIED`. **This is a hard prerequisite of §13f**: the preservation procedure streams `production → gzip → ssh → relay(age) → R2`, so with no relay there is no §13f run. It was previously implicit — the procedure references `$ARCHIVE_RELAY` throughout but no item listed it, so the handoff was not executable as written |

**F14 evidence — accepted 2026-10-13, configuration inventory only.** Operator-provided
command output, obtained directly from the owner's Cloudflare account with **Wrangler 4.100.0**
on Windows. Reproduced verbatim; **no credential, token or key is recorded anywhere in this
file**.

*Bucket listing:*

```
BUCKET_NAME = sigap-backups
BUCKET_EXISTS = YES
```

*Bucket lock inventory:*

```
name:       sigap-retention-90d
enabled:    Yes
prefix:     sigap/
condition:  after 90 days
```

*Lifecycle inventory:*

```
name:     Default Multipart Abort Rule
enabled:  Yes
prefix:   (all prefixes)
action:   Abort incomplete multipart uploads after 7 days
```

*Public-access inspection:*

```
r2.dev URL: disabled
Custom domains: none connected
```

**Each required attribute, checked against what the runbook asks for:**

| Attribute | Required | Observed | Verdict |
|---|---|---|---|
| Bucket name | `sigap-backups` | `sigap-backups` | **PASS** |
| Bucket exists | yes | YES | **PASS** |
| Rule NAME | `sigap-retention-90d` | `sigap-retention-90d` | **PASS** |
| Rule PREFIX | `sigap/` | `sigap/` | **PASS** |
| Rule ENABLED | true | `Yes` | **PASS** |
| Rule RETENTION | ≥ 90 days | `after 90 days` | **PASS** — exactly 90, not below |
| Multipart abort | 7-day default active | `Default Multipart Abort Rule`, enabled, all prefixes, 7 days | **PASS** |
| Private access | no public URL | `r2.dev URL: disabled`, no custom domains | **PASS** |

**What this proves and what it does not.** These are *configuration* facts read from the live
service: the rule exists, is enabled, is scoped to `sigap/`, and is 90 days. It does **not**
prove the rule **enforces** retention, and it does **not** prove a real artifact round-trips.
`BUCKET_LOCK_BEHAVIOR_TEST = NOT_RUN` and `R2_REAL_UPLOAD_READBACK = NOT_RUN` both stay —
see **F21**. The lifecycle rule's presence is likewise confirmed, not its effect.

**Recording note.** This block is the durable record of an inspection the owner performed; no
agent executed a Wrangler command against the real account. A1–A3 are therefore **not** to be
re-run. If any of these values later changes — in particular `enabled` flipping to `No`, or the
prefix drifting off `sigap/` — this row is stale and must be re-verified before relying on it.

**F23 — why the relay is a listed prerequisite and not an implementation detail.** §13f's
preservation path is `docker image save | gzip | ssh RELAY "age -r RECIP > STAGE/$DEST"`, then
the relay uploads the ciphertext to R2 and digests it. That ordering is deliberate: the private
key is absent from production **and** from the relay, no unencrypted archive is ever persisted
on the relay, and production needs no `age` installation (§13f records `age` as **ABSENT** on
the production host and **not required**). Every one of those properties depends on the relay
existing as a distinct host. The **recovery** path deliberately does not need it (§13g.8: "no
production access, no relay"), which is why the relay is a *preservation* prerequisite only —
but it is a hard one. A relay that turns out to be production itself collapses the separation
the design rests on, so it must be a genuinely separate host.

**F23a — relay candidates, evaluated, and why none is adopted.** Three SSH entries are
configured on the workstation. Only one of them is a *genuinely separate host*, and it was
nearly the wrong answer:

| Alias | Resolves to | Verdict |
|---|---|---|
| `fikriserver` | `103.47.225.64` (user `fikri`) | **production itself** — using it as the relay collapses the separation this exists to enforce (§13f) |
| `wg` | **`103.47.225.64` — the identical address**, user `fikri`, adding only `LocalForward 51821 → 127.0.0.1:51821` | **same host as production, not a relay.** It is a WireGuard tunnel to the production server, so it reads like a separate entry while being the same machine. A relay chosen this way would silently defeat the design while every check still looked satisfied |
| `zenith` | `139.59.251.34` (user `root`, dedicated key `id_ed25519_zenith`) | remote, distinct — **candidate** |
| `vps-nagoya` | `43.156.93.105` (user `root`, key **`~/.ssh/id_ed25519`**) | remote, distinct — **candidate, with a caveat below** |

`ARCHIVE_RELAY = NOT_IDENTIFIED` stands, and it is an **operator** decision, because the
remaining acceptance checks are things only the owner's network position can prove: that
**production can reach the relay on 22** (the relay must accept an SSH connection *from*
`fikriserver`, which is the direction §13f uses), that `age`, `aws` and `shred` are present on
it, that `$RELAY_STAGE` is writable under `umask 077`, and that **no age private key exists on
it** — F16's whole point is that the relay holds only the public recipient.

**Recommendation: `zenith` (`139.59.251.34`) over `vps-nagoya`, on host-identity grounds.**
`zenith` authenticates with its **own** key file (`id_ed25519_zenith`); `vps-nagoya` uses
`~/.ssh/id_ed25519` — the **same** private key that authenticates to production. Reusing one
private key across production and a backup-path host means one key compromise reaches both,
which is precisely the separation §13f is built on. This is an architectural criterion, not a
performance one, so it holds regardless of which host is faster or has more disk. Nothing else
was observed, so this is a preference and not a finding about either host's suitability — the
acceptance checks above still have to pass.

**Why the relay is not simply deleted.** §13f's chain is
`docker image save | gzip | ssh RELAY "age -r RECIP > STAGE/$DEST"` because `age` is
**absent** on production and deliberately **not required** there. The relay is the only reason
production carries no encryption tool. So it is load-bearing, and the alternative — installing
`age` on production and dropping the relay — is a design change, not a configuration change;
it must be approved rather than assumed. See §13f's note that a relay which turns out to be
production itself collapses the separation.

**Verification performed.** Both candidates were probed **from this agent workstation**: TCP
connected, then SSH stalled at *banner exchange* and timed out — on `zenith`, on `vps-nagoya`,
and on `fikriserver` **alike**. Since the known-good production host fails identically, the
limit is this environment's SSH path, not any of the three machines. So no relay property
(age/aws/ssh presence, `umask`, key absence) could be checked here, and none was assumed.
The runbook's checks remain the operative procedure; this record only narrows the choice.

**F23b — relay versus source-side encryption (RECOMMENDATION ONLY — not adopted).** Two
concrete options, assessed on the criteria that matter. **No change is made to §13f**; this
needs owner approval before anything is rewritten.

*Option A — retain the relay* (`production | gzip | ssh RELAY "age -r RECIP"`, relay uploads).
*Option B — encrypt on production* with the age **public recipient**, upload with the `aws` CLI
production already has, and delete the relay from the path.

| Criterion | A — relay (zenith) | B — source-side encryption |
|---|---|---|
| Host independence | needs a **third** host; `zenith`/`vps-nagoya` are candidates but **unverified** — neither could be reached from the agent environment | **removes** a host; production and R2 only |
| SSH trust | adds a production→relay edge, and production must hold a key authorised on the relay | **no SSH hop at all** |
| Plaintext exposure | plaintext (gzip) transits to the relay and exists in the `age` process's memory there; only *disk* is kept clean | plaintext never leaves the host that already holds it |
| Upload credentials | the bucket-scoped **Object Read & Write** token must live on the relay | the same token lives on production — which **already** holds S3 credentials for the AUDIT-701 upload path |
| Resource consumption | an entire extra host: disk for `RELAY_STAGE`, CPU for `age`, patching, monitoring | one static `age` binary (~5 MB) plus CPU/disk already present |
| Failure recovery | more hops, so more independent ways for the preservation run to fail | fewer moving parts on the critical path |
| Script compatibility | §13f works **unchanged** | §13f's `stream_offhost` must be edited — the `ssh` hop is removed and `age` runs locally |

**Recommendation: Option B**, and the decisive point is that A's central stated benefit does not
hold. §13f says "no unencrypted archive is ever persisted on the relay" — true of **disk**, but
the relay *must* receive the gzip stream on stdin to encrypt it, so the plaintext passes through
the relay's memory on every run. A therefore adds a host, a trust edge and a second location for
the bucket write token **without removing the plaintext exposure it exists to prevent**. B has
plaintext only where it already is, and keeps the property that actually protects the archive:
`age` public-key encryption needs only the **recipient**, so the **private key stays off
production** exactly as it does today.

The counter-argument is that B grants production write access to the archive bucket. It does —
but A grants the *same* access to the relay, so compromising either host yields it, and Bucket
Lock is the control that limits what write access can destroy (a 90-day floor on deletion). B
removes one of the two credential locations rather than adding a third.

**Acceptance criteria, if B is approved** — all must hold before §13f is rewritten:
1. `age` installed on production and recorded as **present**, replacing the "ABSENT — not
   required" row; the binary's version and checksum recorded.
2. **Only** the public recipient exists on production: a check that no `age` identity file is
   present, and that the recipient string matches the one the recovery side holds.
3. The R2 token on production is **Object Read & Write scoped to `sigap-backups` only** (A5) —
   not account-wide, and not `Admin Read & Write`.
4. A real end-to-end preservation run produces an object whose digest matches the local
   computation, and `R2_REAL_UPLOAD_READBACK` is exercised against it.
5. The recovery path decrypts that object with the private identity **off-host**.
6. §13f's `stream_offhost` is edited to drop the `ssh` hop, and its per-stage `PIPESTATUS`
   handling re-verified — that logic is subtle and was corrected twice (see §13f's notes).
7. The relay requirement (F23) is **removed** from the checklist, and `ARCHIVE_RELAY` retires
   rather than sitting `NOT_IDENTIFIED` forever.

**Do not act on this yet.** F23 remains `NOT_IDENTIFIED` and §13f remains as written until the
owner chooses; if A is chosen instead, F23a's `zenith` preference and the relay acceptance
checks apply unchanged.

> **Gate A verdicts.** `OFFHOST_STORAGE_DESIGN = READY` · `R2_BUCKET = CONFIGURATION_VERIFIED` ·
> `R2_CONTRACT_ACCURACY = PASS` (corrected against the platform's published semantics — see
> below) · `ENCRYPTION_DESIGN = READY` · `PRODUCTION_PRIVATE_KEY = NOT_GENERATED_BY_AGENT` ·
> `PRODUCTION_IMAGES_PRESERVED = NO` · `REAL_ISOLATED_RESTORE = NOT_RUN` ·
> `GITHUB_TEST_ACTIVATION = PASS` (label created and applied; the `labeled` event executed the
> run) · `R2_MINIO_ROUND_TRIP = PASS` · `R2_NEGATIVE_CONTROLS = PASS` ·
> `REAL_CLOUDFLARE_R2_PROOF = PENDING` — **configuration verified, behaviour partially
> proven.** The configuration controls below are `PASS` from the operator's own live inspection
> (F14 evidence). On **2026-10-13** the owner then executed the **real** disposable retention
> drill against the live bucket (§13g.14, runbook procedure, script exit **0**), which is the
> first behavioural evidence this release has. `R2_REAL_UPLOAD_READBACK = NOT_RUN` remains open,
> so a real *artifact* round-trip is still unproven and this stays **non-`PASS`** ·
> `R2_BUCKET_EXISTS = PASS` · `R2_PRIVATE_ACCESS_CONFIGURED = PASS` ·
> `BUCKET_LOCK_CONFIG_VERIFIED = PASS` (**configuration inventory only** — the rule exists,
> is enabled, is scoped to `sigap/`, and is 90 days) ·
> `MULTIPART_LIFECYCLE_VERIFIED = PASS` ·
> `DISPOSABLE_LOCK_MECHANISM_TEST = PASS` (**live, 2026-10-13** — a real throwaway object under
> `lock-drill/<STAMP>/` was refused deletion with `ObjectLockedByBucketPolicy` while a temporary
> 1-day rule applied, then deleted once the rule was removed; no drill rule or object survived.
> This proves the **mechanism on a disposable prefix**, NOT enforcement over a real archive:
> no object under `sigap/` was deleted or tested) ·
> `PRODUCTION_RULE_UNCHANGED_AFTER_DRILL = PASS` (**live, 2026-10-13** — the production rule
> was byte-identical before and after) ·
> `ARCHIVE_RELAY = NOT_IDENTIFIED` (F23 — blocks §13f) ·
> `J11_ROLLBACK_SAFETY = CONDITIONALLY_SAFE` (**resolved 2026-10-14** — the older release was
> rebuilt from git and driven over real HTTP against an isolated post-`0011` database: it is
> compatible for letter-only `short_code`s, but a facility whose `short_code` renders a
> separator-run queue number makes the check-in insert refused by the strengthened CHECK and
> the notification **lost silently**. `scripts/ops/preflight-0011-rollback-safety.sql` is a
> required read-only precondition. See §7f) ·
> `MIGRATION_0011 = NOT_AUTHORIZED` ·
> `PRODUCTION_DEPLOYMENT = BLOCKED`. The design is complete. **No production image has been
> exported, no production database has been touched, and no deployment has occurred**, and no
> approval transfers between gates. What has run is (a) the CI drill — a disposable MinIO
> endpoint, a throwaway age key, a synthetic image and an isolated daemon on a GitHub runner
> (§13g.12) — and (b) the owner's **live** disposable retention drill on 2026-10-13. The CI
> result is a statement about the *contract*; the live drill is a statement about the *lock
> mechanism on a throwaway prefix*. **Neither says anything about a real archive under
> `sigap/`**, and neither makes the age key, the preserved images or the rollback path any
> less outstanding.
>
> **Two corrections that weaken earlier claims, recorded deliberately.** Bucket Lock is **not**
> AWS S3 Object Lock and is **administratively removable** — it is a retention control, not an
> immutable-write guarantee. And the upload credential is **Object Read & Write**: R2 has no
> write-only object preset, and the bucket-scoped write group explicitly includes read. Both
> were previously overstated; see runbook §13g.1 and §13g.3.

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

## H. Jira checkpoint status

The Jira reconciliation checkpoint property `sigap.reconciliation.checkpoint` (schema v2) is
**STALE** and must not be treated as current evidence.

**Cause, isolated `[OBSERVED]` on 2026-10-10 by direct probe:** the MCP server rejects the
app-owned namespace **client-side** with HTTP 400 before contacting Jira —

| Attempt | Key | Result |
|---|---|---|
| `editJiraEntityProperty` | `atlassian-mcp.probe-namespace-test` | **created** — the write path works and read back correctly |
| `editJiraEntityProperty` | `sigap.reconciliation.checkpoint` | **HTTP 400**, *"must be non-empty and namespaced under 'atlassian-mcp.'"* |

Because an `atlassian-mcp.*` write succeeded against the same issue, the authenticated session
**does** hold issue-property write access. This is therefore a **tool namespace restriction,
not a REST permission gap and not an authorization failure**. No bypass was attempted.

**Handling: preserve the existing property unchanged, mark it STALE, and record the correct
HEAD plus gate evidence on SIGAP-1.** A second, MCP-namespaced mirror was considered and
**rejected** — two sources of truth that can disagree is the failure mode the reconciliation
procedure exists to prevent.

**Remediation requires the owner's decision (NOT executed).** In preference order: (a) have the
owning application write `sigap.*` through its own authenticated REST client; (b) relocate the
single source of truth to a surface this tooling can write; (c) mirror under
`atlassian-mcp.sigap.checkpoint` **only** with an explicit back-reference naming
`sigap.reconciliation.checkpoint` as canonical. **Do not choose (c) unilaterally.**

**No automatic synchronization is claimed, in either direction.** Gate evidence for this release
is recorded in the SIGAP-1 comment dated 2026-10-09 (HEAD `4eb2ddd`) and extended by the
2026-10-10 host-evidence comment (recorded at HEAD `07a6109`; the host-evidence content it
cites is `605e53c`). **Convention: the HEAD is the commit that was checked out when the
comment was written**, not necessarily the commit whose content it evidences.

## I. Post-deploy verification (during the real deployment)

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

## K. Standing constraints

| | Item | Status |
|---|---|---|
| I1 | No URL renamed | `[x]` |
| I2 | The redesign adds **exactly one** migration (`0011_notification_outbox_phone_constraints.sql`) and edits **no historical migration** | `[x]` — `git diff --name-only origin/main..HEAD -- packages/db/migrations` is exactly `packages/db/migrations/0011_notification_outbox_phone_constraints.sql` (runbook §1 **A**). The only other in-range change is the **inherited** upstream hardening `9d4e68e` to `0006_notifications.sql` (runbook §1 **C**), which is **not** redesign work. **`0006` is never edited again** — any correction is a new forward-only migration. Note the earlier wording of this row claimed the redesign adds *no* migration, which was **false** once `0011` shipped and contradicted §1A; `0011` exists and must be applied (J10). |
| I3 | No new dependency added without documented reason | `[x]` |
| I4 | No production deployment performed as part of a gate | `[x]` |
| I5 | `/wallet` retained and excluded from navigation (decision D7) — an **intentional deferred route**, not an accidental orphan | `[x]` |
| I6 | **The `.gitleaks.toml` allowlist is removed after the squash merge** — it exists solely to silence a **historical** false positive, so its justification disappears with the commit that carried it | **OPERATOR (post-merge)** — see the note below |

**I6 — why the allowlist must not be retained.** `.gitleaks.toml` was added by this branch to
allowlist ONE finding: a synthetic 64-hex test constant (`43fcbcb3…`) in
`scripts/ops/test-restore-contract.sh`, flagged by `generic-api-key` on entropy in commit
`9e0d61a`. The constant was replaced with a low-entropy placeholder in `04c5628`, but gitleaks
scans the PR's **commit range**, so the stale finding is still reported against the commit that
introduced it.

The allowlist is scoped on four axes with `condition = "AND"` — rule `generic-api-key`, that
single commit, that single path, that exact string — and `useDefault = true` keeps the full
built-in rule set. It is deliberately the narrowest form the schema supports. Even so, an
allowlist whose justification has expired is dead weight that silently widens the scan surface,
so it goes:

1. Squash-merge the branch. The squash creates ONE new commit on the base branch, so `9e0d61a`
   is no longer in the reachable history and the allowlist's `commits` criterion can never match.
2. Delete `.gitleaks.toml` entirely. Because `useDefault = true` was the only thing it set, and
   the file's sole purpose was that one allowlist, deleting it restores the untouched default
   rule set — **no scoping is lost**, and full secret scanning is preserved.
3. Verify: re-run the Secret Leak Scan on the merged commit and confirm it passes with no config
   file present. A finding here means the constant was NOT actually replaced and the allowlist was
   masking a live string.

**Pre-verified 2026-10-13, before the merge, so the cleanup is a mechanical step rather than a
gamble.** Simulated the post-squash state in a scratch clone — the PR's final tree applied as ONE
commit on the base — and confirmed all four facts with gitleaks **8.24.3**, the exact CI version:

| Check | Result |
|---|---|
| `9e0d61a` reachable in the simulated post-squash history | **NO** — the `commits` criterion can never match again |
| The old constant (`43fcbcb3…`) still in the tree | **0 occurrences**; the tree carries the `aaaa…` placeholder |
| Range scan **without** `.gitleaks.toml` | **no leaks found**, exit 0 |
| Default rules still fire **without** the config (planted `ghp_…` token) | **caught** — `RuleID: github-pat`, entropy 4.14, exit 1 |

The last row is the one that matters for "without reducing the default detection rules": deleting
the file does not blind the scanner, it removes a single narrow exception. `9e0d61a` is
**branch-local** — it is not an ancestor of `design/ui-ux-overhaul` — which is why the file is
still required *until* the squash lands, and why it must not be removed before then.

Do not "fix" this by widening the allowlist or by lowering the `generic-api-key` entropy
threshold — either would suppress real secrets across the whole repository.

## L. Blocking gates (all must be closed before deploy)

Every item below is **BLOCKING** and must remain **unchecked** until its evidence is
recorded. "Unknown" is never a PASS.

| | Item | Evidence | Status |
|---|---|---|---|
| J1 | **DB metadata inspection authorized** | explicit operator approval for read-only DB access (runbook §12/§12a) | `[x]` (granted 2026-10-09) |
| J2 | **DB metadata inspection completed** | `scripts/ops/db-metadata-inspection.sql` executed once, read-only, on 2026-10-09 against `fikriserver` / `sigap` (PostgreSQL 16.15). The revision that ran was `657eb675…a1174a6c` (blob `36959d81…c693343`), re-verified **on the host** before execution; `BEGIN READ ONLY` + `ROLLBACK`; exit 0. The shipped revision is now `43394fa3…4bbaebc5` (blob `0dde17bc…9f0161a5`, 21516 bytes) — the version-inventory fix, the STATUS header, and the **false-PASS fix for the invalid unescaped separator class** `[0-9-._() ]` (which PostgreSQL accepts at `CREATE CONSTRAINT` and only rejects on evaluation, and which the classifier previously normalised into textual equality with the valid `[0-9\-._() ]`). The recorded classification is unaffected, but §12a must be re-run before the cutover so the recorded hash matches the shipped artifact; `EXPECT_SHA` has been updated. Sanitized evidence on Jira `SIGAP-60`. | `[x]` (re-run pending) |
| J2b | **Classifier proven non-vacuous (in-repo)** | `sh scripts/ops/test-db-classifier.sh` → **33 pass / 0 fail** on a disposable local cluster, over 32 schema variants including the **real release schema** (migrations `0001`–`0011`), the `{10,}`-conjunct-only loophole, the fragment-spoof, the mis-bound-column case, mixed strong/weak columns, a `NOT VALID` phone constraint, the release schema re-run with `standard_conforming_strings=off`, missing subject/body constraints, both phone constraints absent with and without an extra constraint, three altered predicates, unexpected column/version/constraint metadata, the **real pre-`0011` production shape (`1..10` + weak) and its post-`0011` result**, the absent-denylist state before and after `0011`, the **`version 11 recorded but weak`** desync, the **`hardened but version 11 not recorded`** case, the **invalid unescaped separator class `[0-9-._() ]`** (which PostgreSQL accepts at `CREATE CONSTRAINT` and only rejects on evaluation, and which was textually indistinguishable from the valid `[0-9\-._() ]` after the backslash normalisation — this was a **false PASS** in the classifier, now closed by variant `T-unescaped-separator-class`), and a static inventory check. Only a genuinely strengthened schema reports `MATCHES_CURRENT_SECURITY_CONSTRAINTS`. | `[x]` (33/0 observed) |
| J2c | **`0011` migration proven on a disposable cluster** | `sh scripts/ops/test-migration-0011.sh` → **20 pass / 0 fail**. Covers fresh all-migrations, the weak state (**the production state**), the already-hardened state, the absent state, mixed/spoofed/`NOT VALID` drift (all **refused**, schema unchanged), pre-existing violating rows (**refused**, rows preserved), transaction rollback, reapplication/version tracking, real enforcement on `subject` and on `body_template`, column binding, ordinary-content compatibility, `standard_conforming_strings=off`, absence of a top-level `BEGIN`/`COMMIT`, and the definitions of all eight structural constraints. Asserts the **actual resulting schema**, not a zero exit code. Synthetic data only. | `[x]` (20/0 observed) |
| J2d | **App/DB predicate agreement + production-shaped `0011` transition proven** | `sh scripts/ops/test-notification-contract.sh` → **PASS** (7 sub-suites) on disposable local clusters, synthetic data only. Each sub-suite's status is read from the `go test` invocation itself — **not** from a pipeline's `tail`, which would report tail's status and make the harness unfalsifiable. (a) `TestEnqueue_GoAndDatabaseAgree` — 17 vectors through the REAL `Service.Enqueue`; each vector's **database** verdict is observed independently via a direct INSERT that bypasses the Go layer, so the divergence check cannot be satisfied by the Go verdict echoing itself. This closes the **app/DB divergence** where the Go denylist carried a second alternative (`[0-9]{3,4}[-._() ][0-9]{3,4}[-._() ][0-9]{2,4}`) the CHECK constraint does not have. (b) `TestPredicate_CorpusAgreesWithTheDatabase` — the shared corpus through a live constraint, distinguishing a **phone** rejection from an unrelated constraint's (the empty body is refused by a different CHECK, and treating any error as a phone rejection would fake a divergence). (c) `TestMigration0011_Transition_CompatibleHistory` — reconstructs the **observed production shape** and runs the **REAL runner**; asserts version 11 recorded, both constraints validated and strengthened, row count unchanged. (d) `TestMigration0011_Transition_ViolatingHistory` — **`0011` FAILS CLOSED**: raises **`23514`** (SQLSTATE pinned, and the failing constraint is named), version 11 NOT recorded, definitions and rows unchanged; then a **non-destructive** remediation (`0812-3456-7890` → the masked form, verified against the predicate before the retry) unblocks a clean retry with the record preserved. **Whether such rows exist in production is UNKNOWN.** (e) `TestMigration0011_Transition_MixedStateRefused` — mixed strong/weak → the migration's own `RAISE` (`P0001`), nothing changed. (f) `TestMigration0011_Transition_AbsentConstraintsAdded` — both absent → added strengthened. (g) `TestRenderedQueueNumber_GoAndDatabaseAgree` — rendered queue numbers inserted **directly**, proving the database's own verdict on `{short_code}-{NNNN}`. | `[x]` (PASS observed) |
| J2e | **`0011` lock/timeout behaviour characterised** | `apps/api/internal/notification/migration_lock_test.go`, run via J2d's harness → **PASS**. `DROP`/`ADD CONSTRAINT` take `ACCESS EXCLUSIVE`, which conflicts with **every** other lock mode, so reads, writes, the notification worker's `UPDATE`s and every `Enqueue` block for the window; `ADD CONSTRAINT` validates every row while holding it. **[OBSERVED]** an open `ACCESS SHARE` or `ROW EXCLUSIVE` transaction blocks the migration, and the ungranted `AccessExclusiveLock` is visible in `pg_locks` (polled to a deadline, not a fixed sleep). The lock queue is **FIFO**, so the outage begins when the ALTER *requests* the lock, not when it acquires it. `migrate.Run` sets **no** `lock_timeout`/`statement_timeout` and has **no retry loop**; `lock_timeout` applied to the runner's connections yields SQLSTATE **`55P03`** with **no partial state** and a clean manual retry — and because the timeout path drives the **real runner**, the "version 11 NOT recorded" assertion is real rather than vacuous. **Consequence for J10: measure the outbox row count and prefer a maintenance window.** | `[x]` (PASS observed) |
| J2f | **CI release gates proven** | `sh scripts/ops/test-guard-shallow-clone.sh` → **7 pass / 0 fail** (real shallow and full clones, plus a CI-shaped-HEAD regression case): the migration-inventory guard **fails and names the missing base ref** when `origin/main` is absent, does **not** emit the misleading *"migration diff is EMPTY"*, and passes with full history. Fixes the defect where the `ops-guards` job was red on **every** PR at `actions/checkout`'s default `fetch-depth: 1` while passing locally. **NOTE:** the count is **7** only once the CI-baseline repair (PR #86) is merged. That repair fixed a *second, independent* defect in this suite, which the *"4 pass"* previously recorded here did not describe: the shallow fixture built itself with `--branch "$(git rev-parse --abbrev-ref HEAD)"`, and on a `pull_request` event that is `pull/<N>/merge` — a name existing only as a **remote-tracking** ref, so `git clone --branch` failed and the suite exited **2** (MISSING PREREQUISITE) having run **ZERO** assertions. The *"4 pass"* was therefore observed **locally only**; on CI this row's evidence was never actually produced. The full-clone fixture was broken on CI too (a clone inherits only `refs/heads/*`, while CI keeps `main` under `refs/remotes/origin/`, so `origin/main` was absent and the positive assertion could never pass). Both are fixed, and a fixture failure is now a hard FAIL under CI rather than a skip. The inventory assertion is skipped when `HEAD == origin/main`, so the job does not go red on `main` itself after the merge. `sh scripts/ops/test-skip-exit-codes.sh` → **12 pass / 0 fail**: pins the exit-code contract (`0` PASS, `1` FAIL, `2` MISSING PREREQUISITE) under plain shell, `set -e` and `set -eu`, reproduces the broken `suite; rc=$?; [ "$rc" -eq 2 ] || exit "$rc"` form (dead code under `set -e`), rejects the `\|\| true` anti-pattern, and asserts the runbook recommends the guarded form while mentioning the broken one only as a prohibition. `sh scripts/ops/test-production-deploy-guards.sh` → **28 pass / 0 fail** (its §13a pruning check is scoped to the sentence and joins wrapped lines, so it can no longer pass on an unrelated phrase, nor fail on a wrapped continuation). **All three suites, plus the classifier, migration and contract suites, are now wired into the `ops-guards` CI job** using the guarded `if` form, with the PostgreSQL binaries installed by the job so that exit 2 is a misconfiguration rather than a legitimate skip. | `[x]` (7/0, 12/0, 28/0 observed) |
| J3 | **DB schema classification recorded** | **Recorded: `OLDER_WEAKER_CONSTRAINTS`**, with `db = sigap` and `pg_version = 16.15`. The script hash matched, `psql` exited 0, and exactly one anchored class line was emitted. A database built from HEAD (or one that has had `0011` applied) reports `MATCHES_CURRENT_SECURITY_CONSTRAINTS` instead. | `[x]` |
| J4 | **Drift disposition recorded** | Class is not `MATCHES_CURRENT_SECURITY_CONSTRAINTS`, so a disposition was required. **Decision: remediate forward** with the new migration `0011` (§12b) — prepared, tested (J2c), and **not yet applied**. Applying it is J10. | `[x]` (remediate forward; J10 outstanding) |
| J5 | **Disk capacity gate passed** | runbook §13a — rollback archives present **and verified**; **build peak measured** (§13e.4); free space after both ≥ the floor **`max(2 GB, 10% of the filesystem size)`**. **MEASURED 2026-10-09, re-measured 2026-10-10 (read-only SSH): the capacity portion PASSES.** Root FS `/dev/sda2` ext4, `168,971,526,144` B total (157.37 GiB), **`88,279,465,984` B = 82.22 GiB available (52.25%)** (was `88,368,025,600` on 2026-10-09; −88,559,616 B = −0.08 GiB); floor **15.74 GiB**; headroom above the floor **66.48 GiB**; the three SIGAP images total `440,801,639` B = 420.4 MiB, so after preservation ≈81.79 GiB remains — **66.05 GiB above the floor**. `/var/lib/docker` is on the **same** filesystem (no separate Docker volume); build cache **0 B**; `sigap_pgdata` 66,449,216 B = 63.4 MiB. **This row previously recorded `BLOCKED` on an estimate of "16 GB free / 90% used" that was wrong by roughly 66 GiB**; the conclusion did not follow from the real numbers and is corrected. **Remaining:** the **build peak is still unmeasured** (§13e.4 item 4), so a `build` is gated on a measurement rather than a shortfall. **Note the host is shared** with `orbit-chat`, `portal-sekolah`, the ELK stack, Traefik and Grafana/Prometheus (33 images / 25 running containers), so a build's peak competes with other stacks — a reason to prefer the off-host build in §8c, not a SIGAP-only capacity issue. **Storage expansion is NOT required.** | **OPERATOR** (peak measurement) |
| J6 | **Untracked-file disposition recorded** | runbook §2a — every untracked path classified; `.env.bak-phase5` retained in place with owner/mode/size/mtime; **no** `git clean` | **OPERATOR** |
| J7 | **Release identity validated post-deploy** | `/_app/version.json` changed for every rebuilt service; `docker compose images api web` digests recorded (runbook §3.0/§3.8) | **OPERATOR** |
| J8 | **Protected smoke requires SEPARATE authorization** | any probe of a protected route writes an `audit_events` row (runbook §8a). A protected-route smoke against production is **NOT** covered by this checklist and needs its own explicit approval. | **OPERATOR** |
| J9 | **Production env gate passes on the host** | C18 — `preflight-production-env.sh` exit 0 against the real environment | **OPERATOR** |
| J10 | **`0011` applied to production** | separate, explicitly authorized operator action (runbook §12b). Apply it **through the migrator** so the `schema_migrations` version-11 row is recorded — a bare `psql -f` runs the DDL but never records the version, leaving the database remediated yet un-recorded (harmless to the constraint, but it means the runner would try to re-apply on the next start). Then **re-run §12a** and confirm the class is `MATCHES_CURRENT_SECURITY_CONSTRAINTS`. **NOT applied. Requires its own authorization** — not covered by the deploy approval. | **OPERATOR** |
| J11 | **Rollback safety established** | runbook §7f/§7g — the previously running image's source revision recovered **or** the preserved binary image tested against the post-`0011` schema in isolation (§7g). **Currently `UNKNOWN`, which BLOCKS J10 and the deploy.** An earlier review asserted `SAFE` on the premise that the application denylist and the constraint were "the same predicate". **That premise was false `[OBSERVED]`:** the Go regex carried a second alternative (`[0-9]{3,4}[-._() ][0-9]{3,4}[-._() ][0-9]{2,4}`) the constraint does not have, so an image at that revision could write a body the strengthened constraint rejects. The predicates now agree in this release, but the production image's revision is still unproven. **`[OBSERVED]` 2026-10-09:** the running api image is `sha256:43fcbcb35ed2…`, which **matches** the recorded historical ID, and `RepoDigests` is **empty** — so the image carries **no source-commit identity at all**. An image ID does not close the question; §7g's empirical test of the preserved binary does. `UNKNOWN` stands: **once `0011` is applied, a rollback is no longer purely image-based.** Do **not** resolve this by weakening the constraint. | **OPERATOR** |

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

# J1–J3 — DB metadata inspection. READ-ONLY, METADATA ONLY.
# *** J1–J3 ARE COMPLETE. The inspection ran once on 2026-10-09 (see J2/J3). ***
# *** Do NOT re-run this without a fresh explicit authorization. The reference
#     below is retained only for the post-0011 re-classification required by J10. ***
# *** Run the FULL §12a block (runbook DEPLOYMENT_RUNBOOK.md §12a), which also
#     captures the output, applies the CLASSIFICATION OK guard, and records the
#     database identity. This bare invocation alone does NOT satisfy J2 or J3. ***
docker exec -i sigap-postgres psql -U sigap -d sigap \
  -v ON_ERROR_STOP=1 --no-psqlrc -P pager=off -f - < scripts/ops/db-metadata-inspection.sql

# J10 — apply the corrective migration. *** NOT AUTHORIZED. Requires its own
#     explicit operator approval, separate from the deploy approval. ***
#     Apply it THROUGH THE MIGRATOR so the schema_migrations version-11 row is
#     recorded. A bare `psql -f` runs the DDL but never records the version, so
#     the database would be remediated yet un-recorded. The migrator is
#     version-only, so a one-shot run with SIGAP_AUTO_MIGRATE=true for THIS
#     invocation only is the supported path — set it in the command
#     environment, never in the persisted production env file (C13).
#     Run from $SIGAP_DEPLOY_DIR at the release commit, then re-run the §12a
#     block above and confirm the class is MATCHES_CURRENT_SECURITY_CONSTRAINTS.
SIGAP_AUTO_MIGRATE=true docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml \
  run --rm -e SIGAP_AUTO_MIGRATE=true api
# Fallback if a one-shot runner invocation is unavailable: apply the DDL and
# record the version row in ONE transaction, so the two cannot diverge.
#   { cat packages/db/migrations/0011_notification_outbox_phone_constraints.sql
#     echo "INSERT INTO schema_migrations (version, checksum) VALUES (11, decode('58913737a490aa53b050f6808f3a1323b1708b4191336d7965758c3535d9882a','hex')) ON CONFLICT DO NOTHING;"
#   } | docker exec -i sigap-postgres psql -U sigap -d sigap --single-transaction -v ON_ERROR_STOP=1 -f -

# J5 — disk capacity gate (runbook §13a). Record before/after free space.
df -h /var/lib/docker

# A6 / J6 — confirm the sensitive backup is ignored but never staged.
git check-ignore -v .env.bak-phase5   # expected: matches .gitignore
git check-ignore -v .env.example      # expected: no output (exit 1 = NOT ignored)
git status --porcelain --untracked-files=no   # TRACKED must be empty
```
