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

## Activation — exact PR direction (corrected)

`on.pull_request.branches` is a **BASE branch** filter: GitHub matches it against the PR's
*target*, not its head. The first published version documented the activation direction as
`design/ui-ux-overhaul → main`, which does **not** match `branches: [design/ui-ux-overhaul]`
and would never have triggered the workflow. Corrected.

**The direction that activates this workflow is:**

```
ci/sigap-88-validation  ->  design/ui-ux-overhaul
   (head)                     (base — matches the filter)
```

A PR in the opposite direction (`design/ui-ux-overhaul → main`) does **not** match and will
never run it. The base is deliberately the feature branch, not `main`: the harnesses are
validated on the redesign stream without anything reaching the default branch, and without a
merge into `main`.

The PR head branch must contain a change matching the `paths` filter — editing
`.github/workflows/ops-sandbox.yml`, any listed `scripts/ops/*.sh`, or this document is
sufficient.

**Activation status: `NOT_VERIFIED`.** The workflow has never run. Opening that PR needs the
owner's explicit approval; it is for CI validation only and **must not be merged**.

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

- **No network-deny policy is claimed, and the distinction is load-bearing.**
  GitHub-hosted runners have public egress by default. Three separate facts must not be
  conflated:
  1. **No production credentials are supplied** — true, verified by the absence of any
     `secrets.*` reference.
  2. **No production service is *intentionally* contacted** — true, because no step in this
     workflow addresses a production endpoint.
  3. **Network isolation is actually enforced** — true **only** inside the `adversarial`
     container (`--network none`). The `sandbox-tests` and `real-restore` jobs have full
     egress.

  Absence of a credential does **not** make a production endpoint unreachable: an
  unauthenticated endpoint, or one reachable without a secret, is still reachable. The
  correct claim is (1) and (2), not (3) workflow-wide.
- **Action pinning.** `@v4` is a **major-version tag**, a mutable ref the maintainer can
  move. That is the conventional pin, and it is **not** immutable commit-SHA pinning.
  Hardening to `@<full-commit-sha>` is a deliberate improvement, recorded as **not done**.
- **Host VM isolation is not a sandbox for every adversarial command.** The adversarial
  job therefore adds a container with `--read-only`, `--network none`, `--pids-limit`,
  `--memory`, and **no Docker-socket mount**. It **does** bind-mount the repository
  read-only (`-v "$GITHUB_WORKSPACE:/repo:ro"`) — that mount is the adversarial input and
  the container's working directory. The accurate claim is that **no writable host path
  is exposed**, not that the host filesystem is unreachable.
- **The `adversarial` job's dependencies are pre-installed in a separate networked build
  step** (`sigap-adversarial:local`), so the isolated container has no network from its
  first command. The earlier design ran `apt-get` inside the `--network none` container,
  which could never succeed; that was corrected rather than worked around.
- **Authenticated encryption is a hard acceptance criterion for `real-restore`, not an
  optional extra.** `age` is installed in a networked prepare step and its absence is a
  hard failure — there is **no fallback branch**, so the job cannot reach the restore step
  with the encryption stage unrun. The round-trip must produce byte-identical plaintext
  **and** reject a tampered ciphertext. Plain-image and compression results are reported
  as they are produced, but the restore input is the *decrypted* archive, so a run that
  never encrypted cannot restore at all.

## Runner budget (GitHub-hosted standard, `ubuntu-latest`)

4 vCPU · 16 GB RAM · 14 GB ephemeral SSD. The adversarial container is further bounded
to 2 CPUs, 2 GB, and 256 processes, which bounds the exhaustion classes that caused the
incident. Exact allocation is GitHub-controlled and not guaranteed.

## The three jobs

| Job | Docker | Proves | Does not prove |
|---|---|---|---|
| `sandbox-tests` | none | containment guard logic, restore-contract failure modes, exit-code propagation | real Docker behaviour |
| `adversarial` | container, `--read-only --network none` | **the SIGAP-88 shape is refused by the OS**, not by a script | anything about the runner VM itself |
| `real-restore` | runner's disposable daemon | exact image-ID correspondence through save → gzip → **age encrypt → decrypt** → load, plus tamper rejection | production images, production keys |

## Corrections applied before the first run

Audited and fixed before any execution, each verified in source:

1. **Trigger direction.** `branches` is a base filter; the documented activation direction was
   inverted. Corrected above — `ci/sigap-88-validation → design/ui-ux-overhaul`.
2. **Adversarial dependencies.** The container ran `apt-get update` under `--network none`,
   which cannot succeed. Dependencies are now installed in a separate **networked build step**
   (`sigap-adversarial:local`), so the isolated run has no network from its first command.
   Isolation is not weakened to make the test pass.
3. **Dummy-image build context.** `docker build … -` with `FROM scratch` + `COPY README.md`
   supplied no context. The context is now created deterministically with synthetic content
   only, and the round-trip verifies exact image-ID correspondence plus a real `age`
   authenticated-encryption round-trip including a tamper-rejection check.
4. **Hardcoded Windows path.** The containment suite sourced
   `/f/Sehat-Hub-ui-redesign/scripts/ops/ops-sandbox.sh`, which does not exist on a hosted
   runner. The library is resolved from the script's own location and passed as an explicit,
   validated argument; wrong, empty and nonexistent paths fail closed, and no Windows-specific
   fallback was added.
5. **Containment enforcement.** The harness runs `set -u` but not `set -e`, so a bare
   `sbx_require_contained` could be ignored and the write would proceed anyway.
   `sbx_write_gate` now aborts before **every** write, `chmod`, and temporary-payload
   creation, gating the exact destination rather than a validated prefix.
6. **Cleanup ownership.** Cleanup could `rm -rf` any child of the approved root, so a caller
   replacing `SBX_ROOT` with a sibling could delete a directory it did not create. Cleanup now
   removes only the root recorded at init (`SBX_CREATED`), with a negative control proving a
   sibling survives while the created root is still removable.
7. **Classifier reporting.** The workflow called the classifier with no image arguments and
   masked its exit with `|| true`, turning a security check into an always-passing no-op.
   Production-image inspection is now marked **NOT APPLICABLE** to this job, stated plainly
   rather than reported as a pass.

## Evidence standard

A PASS requires: the actual command, the runner identity, the exit code, a pass/fail
count, the timeout status, the resource limits in force, whether anything was skipped,
and a sanitized artifact URL. **Static YAML validity and source review are not evidence of
execution**, and are reported as such.

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
