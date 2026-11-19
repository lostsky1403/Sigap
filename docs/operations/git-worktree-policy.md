# Git worktree and branch governance policy

**Status:** mandatory for every coding agent on SIGAP (OpenChamber, OMP, Trae, Codebuff,
Claude Code and compatible tools). Read this before any Git-changing operation.

This document defines durable roles and conventions. It deliberately uses **roles**, not
machine-specific absolute paths: the authoritative current inventory lives in the
project's OpenChamber knowledge (local to this workstation), not in public documentation.

## 1. Canonical repository and branch roles

| Role | Purpose | Rules |
| --- | --- | --- |
| Primary checkout | Tracks canonical `main`; the only place `main` is checked out | May be behind `origin/main`. Never reset, clean or prune. Synchronize only with `git fetch origin` + `git merge --ff-only origin/main`, and only when the checkout is clean, on `main`, with no in-progress Git operation and no unique local commits. |
| Active development worktree | Better Auth / Regional V2 development on `feat/sigap-selfhosted-auth` | Keep attached to that branch while Draft PR #97 is open. Preserve its unrelated untracked file `docs/operations/test-production-deploy-guards.sh`. Never switch its branch to investigate another PR. |
| Temporary worktrees | Review / hotfix / test / release-cut tasks | Created under a dedicated parent directory, one branch per worktree, removed only with explicit owner approval. The parent directory is created only when needed and after verifying the path. |

Never check out the same branch in two linked worktrees. Avoid detached HEAD for
persistent work; a detached checkout is acceptable only for temporary read-only
inspection.

## 2. Naming convention

- `review/pr-<number>-<topic>`
- `hotfix/<issue>-<topic>`
- `test/<topic>`
- `release/<release-name>`

Each active task gets a unique branch **and** worktree path. The regional release branch
`release/sigap-regional-v1` is a proposed future branch and must not be created as part
of an unrelated task.

## 3. Topology preflight (before any Git change)

Every agent performs a read-only preflight first:

```sh
git worktree list --porcelain
git branch -avv
git status --short
git remote -v
git rev-parse --show-toplevel
git rev-parse --git-common-dir
git config --show-origin --get core.hooksPath
```

Record for each worktree: path, branch or detached HEAD, local HEAD SHA, upstream,
ahead/behind, cleanliness, tracked modifications, untracked files (never print secret
values), unique local commits, and whether another agent/process is using it. Mark
uncertain entries **UNKNOWN**, never SAFE_TO_DELETE. Do not infer purpose from a name.

Before creating a worktree: verify disk space and path, verify the target ref, check
existing worktrees and branch ownership, confirm no path/name collision, and record the
new worktree in the task ledger.

After finishing: verify the work is committed or deliberately preserved, verify no
untracked or unpushed work would be lost, and report cleanup eligibility. Do **not**
remove a worktree or delete a branch without explicit owner approval.

## 4. Branch ownership and protection

- `main`: canonical history; no force-push, no history rewrite, no direct modification.
- `feat/sigap-selfhosted-auth`: active development branch; source of Draft PR #97;
  protected while the PR is open.
- Merged-PR branches and hotfix branches: preserved, not deleted automatically.
- Safety/checkpoint branches (`safety/...`): preserved; they are recovery points.
- Local-only branches: treated as potential unique work until proven otherwise.
- Stale/prunable worktree registrations (gitdir file points to a missing path) are
  metadata only; they are **not** authorization to delete the underlying branch.

## 5. PR branch management

Draft PR #97 is open against `main`. A large PR (hundreds of files) is reviewed by
subsystem, not by artificial history splitting. Corrections land as normal commits and
normal pushes; no squash, rebase, force-push or history rewrite. Keep the PR in Draft
until the owner explicitly requests ready-for-review. Never merge automatically.

## 6. Backups, checkpoints and no destructive cleanup

Forbidden without explicit owner authorization: `git reset --hard`, `git clean`,
`git branch -D`, `git worktree remove`, destructive `git gc`/prune, force-push, and
removal of backup/checkpoint refs. Cleanup commands are *reported* for future approval,
never executed opportunistically.

## 7. Commit identity and hooks

Commit as the configured maintainer identity (`lostsky1403 <moch22188ti@student.nurulfikri.ac.id>`);
verify with `git var GIT_AUTHOR_IDENT` before committing. Never use `--author`. Never add
AI `Co-authored-by:` trailers. Keep `core.hooksPath=.githooks` installed. The CI identity
gate checks new commits; its authorization list for legacy design-tool identities is a
historical exception, not permission to create new commits under those identities.

## 8. Discovery

This file is referenced from `AGENTS.md`. Agents that read a different instructions file
must treat this policy as binding repository policy. The live absolute-path inventory is
kept in the project's OpenChamber knowledge so it can be refreshed without a public
commit.
