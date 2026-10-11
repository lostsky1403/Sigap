# Reviewer / QA Agent Execution Policy (incident SIGAP-88)

**Status: DOCUMENTED, NOT ENFORCED.**

This policy exists because of incident SIGAP-88. A reviewer subagent (`QaReview4`)
generated a helper script that ran `sh /tmp/mk3.sh`, in which `BIN=$T/bin` expanded with
`$T` **unset in the child process**, collapsing `BIN` to `/bin`. On this Windows/MSYS
host `/bin` maps to `C:\laragon\bin\git\usr\bin`, so `cat > "$BIN/gzip"` overwrote the
**host Git-for-Windows gzip executable**. The result was recursive process exhaustion
and 38,351 stray temp files.

The critical lesson: **source-level validation in the repository harness would not have
prevented this**, because the write came from an ad hoc script the agent generated, not
from a committed file. Any control that only hardens `scripts/ops/*.sh` addresses the
symptom, not the cause.

## Required properties of an isolated execution environment

Every QA, reviewer, or test agent must run where **all** of the following hold. Each is
individually necessary; a subset is not sufficient.

| # | Property | Why |
|---|---|---|
| 1 | **Writable disposable filesystem only** | Prevents any write outside a throwaway root. This is the control that would have stopped SIGAP-88. |
| 2 | **Host executable directories inaccessible or read-only** | `/bin`, `/usr/bin`, `/usr/local/bin`, `/sbin`, `/opt`, `C:\laragon\bin\git\*`, `C:\Program Files\Git\*` must not be writable. |
| 3 | **No writable mount of the Laragon Git toolchain** | The specific directory damaged in the incident. |
| 4 | **No host Docker socket** | Prevents `docker load` into a production daemon, and prevents container-escape writes. |
| 5 | **No production credentials** | Keeps a compromised agent from reaching production. |
| 6 | **No access to production networks** | Denies lateral movement and any production SSH path. |
| 7 | **Bounded CPU, memory, process count and timeout** | Contains a fork bomb: the SIGAP-88 symptom was >6,000 `sh.exe` and ~55 GiB committed. A process-count limit alone would have capped the damage. |
| 8 | **Explicit approval for any test needing wider host access** | Default-deny, with a named human decision for exceptions. |

## The bypass that must be closed

An agent that can write `/tmp/*.sh` and execute it can trivially construct any of the
above by accident. **The policy is only real if the sandbox is enforced by something the
agent cannot modify** — a container, a separate VM, a restricted user account, or a
`bubblewrap`/`sandbox-exec` wrapper. A policy recorded in a Markdown file is a request,
not a control.

Corollary: `scripts/ops/ops-sandbox.sh` (positive containment for test doubles) is
**defence in depth for the committed harness**, and is explicitly *not* a substitute for
the isolation above. It raises the cost of the mistake; it does not make the mistake
impossible for an agent writing its own scripts.

## Enforcement status for this workspace

**NOT VERIFIED.** As of 2026-10-11:

- No container runtime is available: `docker` fails
  (`//./pipe/dockerDesktopLinuxEngine` not found), `podman` is absent.
- `wsl.exe` exists, but starting a distro would mutate host state, which is outside the
  authorization granted for that work.
- Therefore **no enforced sandbox exists here**, and reviewer agents have, in practice,
  run with the same privileges as the host user.

Consequence, stated plainly: **until an enforced sandbox exists, reviewer agents must not
be relaunched for testing work that creates or executes scripts.**

## Enforcement options, in increasing order of containment

1. **Restricted Windows user account** — run reviewer agents as a user with no write
   access to `C:\laragon\bin\git`, `C:\Program Files\Git`, or `C:\Windows`. No tooling
   required; enforces properties 1–3 only.
2. **Docker/Podman container** — read-only root filesystem, `tmpfs` for `/tmp`, no
   Docker socket, no network. Enforces 1–7. Requires a working container runtime.
3. **Disposable VM** — the strongest option; the host toolchain is not present at all.

## Acceptance criteria for this policy to be considered verified

- [ ] A reviewer agent, instructed to write a test double to `$T/bin` with `$T` unset,
      **cannot** create or modify a file in any host executable directory.
- [ ] Process-count and memory limits are demonstrably active inside the sandbox.
- [ ] `command -v docker` inside the sandbox fails or points to a disposable daemon.
- [ ] The production SSH alias and production host are unreachable from inside the
      sandbox.
- [ ] The check above was performed by an **agent inside the sandbox**, not from outside
      it — otherwise it proves nothing.
