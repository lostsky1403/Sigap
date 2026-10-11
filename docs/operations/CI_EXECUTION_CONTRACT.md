# CI Execution Contract — SIGAP-88 Verification

**Status: IMPLEMENTED, NOT YET EXECUTED.** Workflow `.github/workflows/ops-sandbox.yml`
at release-branch HEAD `0c6a2b0`.

## Why this exists

Incident SIGAP-88: a QA reviewer subagent generated a shell helper in which
`BIN=$T/bin` expanded with `$T` **unset in the child process**, so `BIN` collapsed to
`/bin` — which maps to `C:\laragon\bin\git\usr\bin` on this Windows host — and
`cat > "$BIN/gzip"` overwrote the Git-for-Windows `gzip.exe`. That wrapper re-exec'd
`/usr/bin/gzip`, recursing into >6,000 `sh.exe` processes and exhausting RAM and pagefile.

The conclusion drawn from it: **the Windows host must never run these suites.** Not
"must run them carefully" — the host is the asset being protected, and the incident was
caused by a mistake that looked correct at every step.

## Trigger, and why it is not `workflow_dispatch`

GitHub registers `workflow_dispatch` only for workflows **present on the default
branch**. A feature-branch-only `workflow_dispatch` workflow is therefore **not
immediately runnable** — it would need to reach `main` first, which is not authorized.

This workflow uses **`pull_request` on `design/ui-ux-overhaul` only**, so:

- it runs from the feature branch **without merging the redesign into `main`**;
- it is path-filtered, so unrelated PR activity does not trigger it.

**Activation status: `NOT_VERIFIED`.** The workflow has never run. Running it requires
opening a PR from `design/ui-ux-overhaul`, which needs the owner's explicit approval —
that PR is for CI validation only and **must not be merged**.

## Security contract

Enforced by the trigger and job configuration, not by convention:

| Control | Mechanism |
|---|---|
| No repository write | `permissions: contents: read` |
| No persisted git credential | `actions/checkout` with `persist-credentials: false` |
| No secret exposure to forks | **`pull_request`, never `pull_request_target`** |
| No secrets at all | no `secrets.*` reference anywhere in the file |
| No self-hosted runner | `runs-on: ubuntu-latest` |
| No deployment / no environment | neither is declared |
| No main mutation | the trigger targets the feature branch; nothing merges |
| Bounded run | `timeout-minutes` on every job |
| No runaway concurrency | `concurrency` group with `cancel-in-progress` |
| Bounded artifacts | `retention-days: 14`, `if-no-files-found: ignore` |
| Pinned actions | `actions/checkout@v4`, `actions/upload-artifact@v4` — no wildcards |

## What this workflow does NOT claim

- **No network-deny policy is claimed.** GitHub-hosted runners have public egress by
  default. Production is unreachable here **only because no production credential is
  present in this workflow** — not because of a network boundary. That is a real
  distinction and it is stated rather than blurred.
- **Host VM isolation is not a sandbox for every adversarial command.** The adversarial
  job therefore adds a container with `--read-only`, `--network none`, `--pids-limit`,
  `--memory`, and **no host volume or Docker-socket mount**.
- **The `adversarial` job depends on `apt-get` succeeding without network.** It runs
  with `--network none`, so it relies on the `ubuntu:24.04` image cache. If that fails,
  the job **fails loudly** rather than silently skipping — a skipped adversarial test is
  reported BLOCKED, never PASS.

## Runner budget (GitHub-hosted standard, `ubuntu-latest`)

4 vCPU · 16 GB RAM · 14 GB ephemeral SSD. The adversarial container is further bounded
to 2 CPUs, 2 GB, and 256 processes, which bounds the exhaustion classes that caused the
incident. Exact allocation is GitHub-controlled and not guaranteed.

## The three jobs

| Job | Docker | Proves | Does not prove |
|---|---|---|---|
| `sandbox-tests` | none | containment guard logic, restore-contract failure modes, exit-code propagation | real Docker behaviour |
| `adversarial` | container, `--read-only --network none` | **the SIGAP-88 shape is refused by the OS**, not by a script | anything about the runner VM itself |
| `real-restore` | runner's disposable daemon | exact image-ID correspondence through save → gzip → load | encryption, production images |

## Evidence standard

A PASS requires: the actual command, the runner identity, the exit code, a pass/fail
count, the timeout status, the resource limits in force, whether anything was skipped,
and a sanitized artifact URL. **Static YAML validity is not evidence.**

## Remaining host execution risk

`HOST_PROTECTION = NOT_YET_ENFORCED`. Until an OMP-level control denies local execution
and routes agent commands to an isolated target, a reviewer subagent can still write to
the Windows host. See `REVIEWER_SANDBOX_POLICY.md`.

**OMP interception correction:** upstream OMP documentation *does* document
pre-execution `tool_call` interception returning `{ block: true, reason }`, plus
`before_subagent_spawn` and `user_bash`/`user_python` overrides. A previous assessment
concluded no interception existed based only on locally installed extensions; that was
**wrong**. Whether the *installed* runtime (v18.4.5) exposes it is **`UNKNOWN`** — string
scanning its compiled bundle was inconclusive. Documented bypasses exist regardless:
print/ACP input is outside the interception contract, and restricted children do not admit
extension tools. **No OMP hook is implemented here.**
