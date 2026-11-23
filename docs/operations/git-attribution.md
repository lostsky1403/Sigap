# Git attribution — AI-agent `Co-authored-by` trailers

How this repository keeps AI-agent attribution out of its commit messages: what is
stripped locally, what fails in CI, what is deliberately left alone, and how to set up
the git identity those tools should be committing as.

Applies to: every commit in this repository, on every branch and every worktree.
Related: [`AGENTS.md`](../../AGENTS.md) (the agent-facing rule),
[`scripts/check-attribution.mjs`](../../scripts/check-attribution.mjs) (the gate),
[`.githooks/commit-msg`](../../.githooks/commit-msg) (the local stripper).

---

## 1. The problem this solves

The agent tools used on this repository (Trae, Codebuff, Claude Code, OMP) append a
`Co-authored-by:` trailer naming themselves to the commit message they create. Those
trailers are in the history today:

```sh
git log --all --format='%B' | grep -i -E '^[[:space:]]*co-authored-by:' | sort | uniq -c | sort -rn
```

```
  11 Co-authored-by: traeagent <traeagent@users.noreply.github.com>
  11 Co-authored-by: Codebuff <noreply@codebuff.com>
   6 Co-Authored-By: Claude 4.7 <noreply@anthropic.com>
   3 Co-Authored-By: internal-model
   1 Co-authored-by: t <t@t>
```

The last line is a human address and is left alone — 31 of the 32 lines are AI trailers,
which is the number this policy refers to elsewhere.

The important diagnostic is what is **not** wrong in those same commits: their author
was always the maintainer, not the tool.

```sh
git log --all --format='%an <%ae>|%(trailers:key=Co-authored-by,valueonly)' | grep -i traeagent | head -1
# lostsky1403 <lostsky1403@users.noreply.github.com>|traeagent <traeagent@users.noreply.github.com>
```

So this is not a git configuration problem, and it cannot be fixed by setting
`user.name`/`user.email` — those were already correct. The trailer is **text inside the
message body**, injected by the tool after it composed the message. The only places it
can be removed before it becomes history are the message itself (local hook) and the
merge gate (CI).

Defence in depth, layered so no single layer has to be trusted:

| Layer | Artefact | Effect |
| --- | --- | --- |
| 1. Local | `.githooks/commit-msg` | Deletes matching trailer lines before the message is recorded. Never blocks a commit. |
| 2. CI | `scripts/check-attribution.mjs` (job `Commit Attribution Check`) | Fails a push/PR whose **new** commits still carry one. Scans new commits only, so history can never fail CI. |
| 3. Instruction | `AGENTS.md` | Tells agents not to write the trailer in the first place. |

---

## 2. What counts as AI attribution

A line is a candidate when, case-insensitively and after optional leading whitespace, it
starts with `co-authored-by:` and has a non-empty value. A candidate is **AI** when
either rule matches:

| Rule | Values |
| --- | --- |
| Email is one of | `traeagent@users.noreply.github.com` (Trae), `noreply@codebuff.com` (Codebuff), `noreply@anthropic.com` (Claude Code) |
| Name is, or starts with | `traeagent`, `codebuff`, `claude`, `internal-model`, `anthropic` |

The name rule matches a whole name or a name prefix followed by a non-alphanumeric
character, which is what makes `Claude`, `Claude 4.7` and `Claude Opus 4.7` all match —
and what keeps `Claudette Moreau` and a human `Jean-Claude Van Damme` out of it. The
trailer is matched whether or not it carries an email, so `Co-Authored-By: internal-model`
is caught with no address attached.

`.githooks/commit-msg` and `scripts/check-attribution.mjs` implement the same two rules.
The script is the source of truth; keep the hook in step when a new tool appears.

### The human-co-author rule

**A trailer whose email is allowlisted is never treated as AI, whatever its name looks
like.** Two ways in:

| Source | Values |
| --- | --- |
| Built into the check | `real@example.com` (documented test fixture), `114224549+lostsky1403@users.noreply.github.com` and `moch22188ti@student.nurulfikri.ac.id` (this repository's maintainer) |
| `SIGAP_ATTRIBUTION_ALLOWLIST` env var | Comma-, semicolon- or space-separated emails. Honoured by **both** the hook and the check. |

Allowlisting matches on the **email**, and it wins over every detection rule — it is the
escape hatch for a real person whose name or address would otherwise look like a tool
(`Claude Dupont <claude@some-lab.example>`).

Everything else is left alone, by design: a trailer that matches neither rule is the
normal case — a human co-author — not something to clean up.

```sh
# Git Bash / POSIX
SIGAP_ATTRIBUTION_ALLOWLIST='claude@some-lab.example,partner@example.org' \
  node scripts/check-attribution.mjs --range origin/main..HEAD
```

```powershell
# PowerShell
$env:SIGAP_ATTRIBUTION_ALLOWLIST = 'claude@some-lab.example,partner@example.org'
node scripts/check-attribution.mjs --range origin/main..HEAD
```

Export it in the shell that runs the command. For the hook, that means the shell you
commit from; for the gate, the shell that runs the check (or the CI step's `env:`).

---

## 3. Layer 1 — install the local hook

The hook lives in the repository as `.githooks/commit-msg`, so it is versioned and
reviewable. Git only runs it once the clone is told where to look:

```sh
git config core.hooksPath .githooks
```

Run it once per clone, from inside the checkout. Details that matter:

- **The path is relative on purpose.** Git resolves a relative `core.hooksPath` against
  the worktree the commit is made from, so a linked worktree (this repository is
  developed in one: the checkout's `.git` is a pointer file) runs *its own*
  `.githooks/commit-msg`. Verified, not assumed: with the setting in place, a commit in
  a linked worktree executes that worktree's copy of the file.
- **The setting is per clone, not per worktree.** `core.hooksPath` is stored in the
  clone's config, which every worktree of that clone shares — so one command covers all
  of them, and a fresh clone needs it again.
- **It replaces `$GIT_DIR/hooks`.** If a clone has local hooks there, they stop running;
  check before installing (`ls "$(git rev-parse --git-common-dir)/hooks"`). This
  repository tracks no hooks of its own — that directory holds only git's `.sample`
  files, which are inert.
- **The executable bit is committed evidence.** On POSIX clones the file must be
  `100755`. On Windows `core.fileMode` is false, so the bit is not taken from the
  filesystem; set it explicitly when adding the file:

  ```sh
  git add --chmod=+x .githooks/commit-msg
  git ls-files -s .githooks/commit-msg   # expect mode 100755
  ```

  `.gitattributes` pins `.githooks/*` to LF for the same class of reason:
  a `#!/bin/sh` shebang with a CR breaks on Linux and macOS, and a stray CR inside the
  message file would end up in the recorded message.

### What the hook does — and does not do

- Deletes matching `Co-authored-by:` lines from the message file; everything else in the
  message, including its line count and other trailers, is untouched.
- **Never blocks a commit.** Every path exits 0 — a missing argument, a missing or
  unreadable message file, no `awk` on the machine, a failed temp file, any internal
  error. A hook that refuses work is a worse bug than the line it deletes.
- **Is a no-op when there is nothing to strip.** The file is not rewritten, so a clean
  message keeps its exact bytes.
- Does **not** check or set the committer identity, and does not touch trailers that are
  not `Co-authored-by:`.

Verify it is installed and working (this bypasses the hook to leave a trailer behind,
then shows the hook removing it):

```sh
git config core.hooksPath                  # expect: .githooks
git commit --allow-empty -m "chore: hook check" -m "Co-authored-By: Claude 4.7 <noreply@anthropic.com>"
git log -1 --format=%B                     # the trailer is gone
git commit --amend -m "chore: hook check"  # tidy the throwaway commit (or drop it)
```

Uninstall: `git config --unset core.hooksPath`.

---

## 4. Layer 2 — the CI gate

```sh
node scripts/check-attribution.mjs                        # default range: HEAD~1..HEAD
node scripts/check-attribution.mjs --range origin/main..HEAD
node scripts/check-attribution.mjs --help
```

Exit codes — the contract CI relies on:

| Code | Meaning |
| --- | --- |
| 0 | Clean: no AI trailer in the scanned range. |
| 1 | An AI trailer was found. Lists each commit and line. |
| 2 | Missing prerequisite: not a git work tree, git unusable, or the range cannot be resolved. **Never a pass** — CI installs the prerequisites and fetches full history, so a 2 there is a broken gate. |

Range semantics, and why the gate cannot fail on history:

- Only commits **in the range** are scanned, so the 31 trailers already in this
  repository's history are invisible to it. The range is what makes this safe to turn on
  for a repository whose past is unpoliced.
- A range needs `A..B` shape. A bare revision (`--range HEAD`) scans its whole ancestry
  and the script warns about it on stderr for exactly that reason.
- `<rev>^!` means "just that commit" and is the form used for a single-commit check.
- An empty range (a re-push of the same commit) is a valid, clean scan.

The `Commit Attribution Check` job in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
computes the range per event: `github.event.pull_request.base.sha..github.sha` on a pull
request (the merge commit's ancestry is exactly the PR's commits, and no fork refs are
needed), and `github.event.before..github.sha` on a push. A new branch pushes with
`before` set to forty zeros, which is not a revision, so that case scans the pushed tip
(`--range "$head^!"`) instead. The job checks out with `fetch-depth: 0`, because a
shallow clone cannot resolve the base commit and would report exit 2.

---

## 5. Layer 3 — tell the agents

[`AGENTS.md`](../../AGENTS.md) states the rule for every agent tool that reads it: never
append an AI `Co-authored-by:` trailer, and commit as the configured identity. This is
the cheapest layer — a commit that is never written needs no stripping and no gate.

Claude Code has its own setting and honours it: `attribution` (an object with `commit`,
`pr` and `sessionUrl`, or `false` to hide all attribution) replaces the deprecated
`includeCoAuthoredBy`. Its documentation is explicit that repository instructions win:
"Claude Code tells Claude that your own instructions about attribution, such as a
CLAUDE.md or memory rule, take precedence over these commit and PR lines, unless the
line is set in managed settings." Turning the setting off is worthwhile, but the hook
and the gate are what this repository depends on, because they also cover Trae, Codebuff
and OMP, and they cannot be unset by a tool update.

---

## 6. Git identity (Windows / PowerShell / Git CLI / agent tools)

The identity is read from git configuration, and every agent tool here shells out to
`git`, so it inherits the same one. On this workstation it is set globally in
`C:\Users\<user>\.gitconfig`:

```sh
git config --show-origin --get-all user.name
git config --show-origin --get-all user.email
git var GIT_AUTHOR_IDENT
```

```
file:C:/Users/losts/.gitconfig      lostsky1403
file:C:/Users/losts/.gitconfig      moch22188ti@student.nurulfikri.ac.id
lostsky1403 <moch22188ti@student.nurulfikri.ac.id> 1794365293 +0700
```

Set it — same commands in PowerShell, Git Bash, `cmd` or any shell, because they are
just git:

```powershell
git config --global user.name  "lostsky1403"
git config --global user.email "moch22188ti@student.nurulfikri.ac.id"

# Recommended on Windows: refuse to invent an identity from the machine's hostname or
# account name. A commit that cannot be attributed should stop, not guess.
git config --global user.useConfigOnly true
```

For a single checkout instead of the whole machine, drop `--global` and run it inside
that checkout; a local value overrides the global one.

Who reads it:

| Tool | Where its identity comes from | This repository's control |
| --- | --- | --- |
| Git CLI | `user.name` / `user.email` | — |
| PowerShell / `cmd` | the same git config; nothing else to set | — |
| Git for Windows | ships the `sh` that runs `.githooks/commit-msg`; the hook needs no separate setup | hook (layer 1) |
| OMP | inherits the git config of the shell it commits from | `AGENTS.md` + hook + gate |
| Trae | inherits the git config of the shell it commits from | hook + gate |
| Codebuff | inherits the git config of the shell it commits from | hook + gate |
| Claude Code | inherits the git config; its own `attribution` setting controls the trailer | `attribution` + `AGENTS.md` + hook + gate |

Historical note: history also carries GitHub noreply variants of the same addresses —
`114224549+lostsky1403@users.noreply.github.com` and
`lostsky1403@users.noreply.github.com` — and one older spelling of the name
(`Mochammad Fikri Chaerul Chalik Ramdhan`). They are all the same person; both noreply
addresses are allowlisted by the check, and no history needs rewriting to satisfy this
policy.

---

## 7. Verification — the two cases that must behave this way

Run against a throwaway repository so nothing from a real branch is touched. `--no-verify`
bypasses the hook on purpose: it reproduces a machine where the hook was never installed,
which is the case CI exists for.

```sh
# Git Bash / POSIX. <repo> is the path to this repository.
tmp=$(mktemp -d); cd "$tmp"; git init -q .
git config user.name "lostsky1403"
git config user.email "moch22188ti@student.nurulfikri.ac.id"

# Case 1 — AI trailer must fail the gate.
git commit -q --allow-empty --no-verify \
  -m "chore: ai trailer survives" \
  -m "Co-authored-By: Claude 4.7 <noreply@anthropic.com>"
node <repo>/scripts/check-attribution.mjs --range 'HEAD^!'; echo "exit=$?"

# Case 2 — a genuine human co-author must pass.
git commit -q --allow-empty --no-verify \
  -m "feat: human co-author" \
  -m "Co-authored-by: Real Person <real@example.com>"
node <repo>/scripts/check-attribution.mjs --range 'HEAD^!'; echo "exit=$?"
```

Observed output for case 1 — **exit 1**:

```
FAIL: 1 AI attribution trailer(s) in range HEAD^!

  c306ec824a  chore: ai trailer survives
    Co-authored-By: Claude 4.7 <noreply@anthropic.com>   <- Claude Code

Agent tools (Trae, Codebuff, Claude Code, OMP) append these when they
create a commit. Remove the line and re-commit:
  git commit --amend            # for the tip
  git rebase --exec ... / git filter-repo   # for older commits in the range

Prevent it locally:  git config core.hooksPath .githooks
If the address is a human co-author, add it to SIGAP_ATTRIBUTION_ALLOWLIST.
See docs/operations/git-attribution.md.
exit=1
```

Observed output for case 2 — **exit 0**:

```
PASS: no AI attribution trailers in 1 commit(s) (range HEAD^!)
exit=0
```

Two more cases tie the layers together, both observed:

| Case | Command | Result |
| --- | --- | --- |
| The hook removes the line before it is recorded | `git commit -m "chore: x" -m "Co-Authored-By: Claude 4.7 <noreply@anthropic.com>"` (hook installed) | `git log -1 --format=%B` shows the subject only; the gate reports `PASS … exit=0` |
| An allowlisted address overrides detection | `SIGAP_ATTRIBUTION_ALLOWLIST='noreply@anthropic.com' node scripts/check-attribution.mjs --range 'HEAD^!'` on a commit with `Co-authored-by: Claude Dupont <noreply@anthropic.com>` | `PASS … exit=0` (without the variable: `FAIL … exit=1`) |

Also worth knowing: the hook leaves a message with no AI trailer byte-identical, and a
commit with a mix of four AI trailers and four genuine human co-authors keeps exactly the
four humans — including `Jean-Claude Van Damme` and `Claudette Moreau`, the two names the
prefix rule is written to avoid.

---

## 8. Scope, and what is deliberately not done

- **Only `Co-authored-by:` lines are examined.** Other attribution trailers a tool may
  write (for example Claude Code's `Claude-Session` link on cloud sessions) are not
  detected by this check; the `AGENTS.md` rule covers them, and a companion check is the
  place to add them if they appear in practice.
- **History is not rewritten.** The 31 existing trailers stay where they are: they are a
  record of what happened, and the gate's promise — that history cannot fail CI — depends
  on leaving them alone.
- **The hook is convenience, the gate is the control.** A developer who never installs the
  hook still cannot merge a trailer, because CI scans new commits on every push and PR.
- **No blocking behaviour locally.** Nothing in this policy stops a commit; the failure is
  reported at the gate, where it can be fixed once, deliberately.

---

## 9. Commit author / committer identity is a separate control (PR #96 finding)

The hook and the CI gate above examine **message trailers only**. They say nothing about
the `author`/`committer` fields of a commit, so a clean `check-attribution.mjs` run is **not**
proof of correct attribution. This section closes that finding.

**Coverage gap at the PR head:** `core.hooksPath=.githooks` is set in the checkout, but the
PR #96 head `036a1aa9…` and its base `51ba1d53…` do not contain
`.githooks/commit-msg`, `scripts/check-attribution.mjs`, or the `Commit Attribution Check`
CI job — those exist only on the development branch `feat/sigap-selfhosted-auth`. Therefore
**no attribution guard ran on PR #96**, and even where the controls exist they are
trailer-only. This is a separate finding from the historical trailer problem.

**Observed:** the three commits of PR #96
(`707c7076…`, `26f7c53a…`, `036a1aa9…`) carry `Author: sigap-ops <ops@local>` and
`Committer: sigap-ops <ops@local>`. GitHub reports `author: null`, `committer: null`
(no linked account), `author_association: null`, and `verified: false` for all three.
`ops@local` is not a registered GitHub email, so those commits are not credited to any
GitHub account in the Contributors graph; the historical attribution correction credits
commits by matching the commit email to a GitHub account. The owner's authorized identity
in this repository is `lostsky1403 <moch22188ti@student.nurulfikri.ac.id>` (also seen as
the linked noreply form `114224549+lostsky1403@users.noreply.github.com`).

**Where it came from:** no local, global, or system git config sets `sigap-ops` — the
effective config in every checkout is `lostsky1403` / `moch22188ti@student.nurulfikri.ac.id`.
The identity was applied per invocation to the PR commits, not configured as an authorized
automation identity. It is not documented anywhere as an accepted bot; `AGENTS.md` requires
commits to be authored by the configured human identity.

**Required correction:** a **branch-only** author/committer correction on
`hotfix/post-merge-ci-rust-stabilization`, using the maintainer identity, before merge.
A dry-run rewrite in a disposable clone (nothing pushed) produced this exact mapping; the
tree, message bodies, and timestamps are unchanged, so only metadata changes:

| Old SHA | Proposed new SHA |
| --- | --- |
| `707c70764589c5ad9aabb9796670f2fe7e750b72` | `49f0cf3c66a0c22b3f8b2fef081585175db066bf` |
| `26f7c53ae8ac1723424cc45d02de3045afb712e0` | `3cdb0b8b6b55e2bc3b1df1a4d1049a07eca7e888` |
| `036a1aa9f04108cb95f8ce85e93a5c8a925056fc` | `3cb89538154ec1a415c6c456019c2700d550eea1` |

Impact: the PR head moves from `036a1aa9…` to `3cb89538…`, forcing a force-push of the PR
branch, a new CI run on the new head, and a new review baseline. `main` is untouched.
**Do not force-push or rewrite without explicit owner authorization.** After correction,
confirm the commit author/committer and `git var GIT_AUTHOR_IDENT` are the maintainer
identity; run `node scripts/check-attribution.mjs --range origin/main..HEAD` from a checkout
that actually contains the script (the PR branch currently does not), or add the control in
the follow-up PR below.

**Regression control (implemented in this follow-up):** `scripts/check-commit-identity.mjs`
now verifies both the author and committer identity on new commits, rejects generic
unlinked automation placeholders (`sigap-ops <ops@local>`, `root@localhost`, `bot@local`,
AI-tool names), and delegates AI `Co-authored-by` detection to `check-attribution.mjs`.
It runs in the `Commit Attribution Check` CI job after the trailer scan. On trusted
maintainer branches (`main`, `design/ui-ux-overhaul`, `feat/sigap-selfhosted-auth`,
`release/*`, `hotfix/*`) it requires the maintainer or an explicitly authorized automation
identity (`SIGAP_AUTHORIZED_COMMITTERS`); fork PRs run in permissive mode so genuine external
contributors are not blocked for having a different email. It scans only new commits, so
historical canonical commits are never rewritten or failed. Verified by
`sh scripts/ops/test-commit-identity.sh` (11 positive/negative cases, all passing).

---

## 10. How OpenChamber and other coding agents must commit in SIGAP

Before creating **any** commit in this repository, an agent (OpenChamber, Claude Code,
Trae, Codebuff, OMP or any other tool) must:

1. **Use the configured git identity — never invent one.** Check it first:
   ```sh
   git config user.name            # expect: lostsky1403
   git config user.email           # expect: moch22188ti@student.nurulfikri.ac.id
   git var GIT_AUTHOR_IDENT        # expect: lostsky1403 <moch22188ti@...> +0700
   ```
   If either value is empty or unexpected, stop and ask the operator. Do not guess, and
   do not pass `--author` to `git commit` as a substitute.
2. **Never set a local automation identity** such as `sigap-ops <ops@local>`, `ci@local`,
   or any `@local`/`@localhost` address. Those are exactly what the new gate rejects.
   Authorized automation identities must be declared through `SIGAP_AUTHORIZED_COMMITTERS`
   by the operator, not improvised by the agent.
3. **Write no AI attribution trailers.** No `Co-authored-by:` naming a tool or model
   (see §1–2). A legitimate human co-author is fine; add unusual human addresses to
   `SIGAP_ATTRIBUTION_ALLOWLIST`.
4. **Keep `core.hooksPath=.githooks` installed** in each clone; the local hook strips AI
   trailers before they are recorded. Do not bypass it with `--no-verify`.
5. **Verify after committing:**
   ```sh
   git log -1 --format='%an <%ae> | %cn <%ce>%n%B'
   node scripts/check-commit-identity.mjs --range 'HEAD^!' --mode auto
   ```
   For a range of several new commits use `--range origin/main..HEAD` instead.
6. **Never rewrite or force-push a branch without explicit owner authorization.** A
   branch-only author/committer correction (like PR #96's) is a metadata-only rewrite:
   trees, messages, dates and commit order must be verified identical before pushing,
   and the push must use `--force-with-lease=<ref>:<exact-old-sha>` targeting only that ref.

These rules are enforced in CI by both gates; passing one is not evidence that the other
passed. Attribution is correct only when author **and** committer are the right human (or
an explicitly authorized automation identity) **and** no AI trailer is present.

## 11. Historical design-tool exception — exact SHA only

PR #97 contains 19 historical design/prototype commits authored by `SIGAP Design`
(18) or `SIGAP Impl` (1) with local placeholder emails. All are committed by the
maintainer, have no human co-author trailers, touch only design artifacts, and create no
extra GitHub Contributors entry. The owner decided to **preserve** them without rewriting.

The identity gate grandfathers those authors **by exact immutable commit SHA** via
`SIGAP_LEGACY_AUTHOR_COMMITS` (the 19 full SHAs are listed in the CI workflow). It never
uses a date, name, email or path pattern — backdating or reusing an allowed email cannot
create a new exception. A new commit with `design@sigap`, `design@sigap.local`,
`impl@sigap.local` or `sigap-ops <ops@local>` is rejected. Only the AUTHOR check is
grandfathered; every commit still gets the AI-trailer check and a committer-identity
check. GitHub's own merge committer (`noreply@github.com`) is accepted only for a real
merge commit (two or more parents) whose author is a verified maintainer.

`sh scripts/ops/test-attribution-exception.sh` proves both sides: the 19 historical
commits pass, while new placeholder authors, `sigap-ops`, and AI trailers fail, and a
maintainer commit, a fork contributor in permissive mode and a genuine human co-author
pass.
