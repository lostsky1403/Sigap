# SIGAP session handoff — context recovery for coding agents

This is the portable entry point for any future coding agent (OpenChamber, OMP, Trae,
Codebuff, Claude Code or compatible tool) working on SIGAP. It contains **no private
machine details**: absolute paths, local-only branch inventories and credentials live in
local untracked files or the project's OpenChamber knowledge.

## Canonical facts

- Repository: `lostsky1403/Sigap` (origin `https://github.com/lostsky1403/Sigap.git`).
- Canonical branch: `main`; current reviewed baseline `4813de64fcbac89ae9ce419f003a9120177c6867`.
- Primary checkout role: tracks `main` only; may be behind origin; never reset or cleaned.
- Active development branch: `feat/sigap-selfhosted-auth` — Better Auth, regional RBAC,
  Patient Continuity, migrations 0012–0014; source of **Draft PR #97** against `main`.
- PR #97 reviewed head: `e292949864dcb854e65107d3d614f2a84bd88d8c` (9/9 CI jobs green).
  Keep it Draft; do not mark ready or merge without explicit owner authorization.

## Policies that bind every agent

1. [AGENTS.md](../../AGENTS.md) — attribution and the 10-step worktree preflight.
2. [git-worktree-policy.md](git-worktree-policy.md) — worktree/branch roles, naming,
   preflight, ownership, PR handling, checkpoints, cleanup authorization.
3. [git-attribution.md](git-attribution.md) — commit identity, hooks, CI exit codes;
   historical Pen.dev design authors are grandfathered by exact commit SHA only.
4. [secret-scan-classification.md](secret-scan-classification.md) — exact-fingerprint
   Gitleaks exceptions; default detector rules remain active.
5. [regional-release-integration-contract.md](../../design/regional-release-integration-contract.md) —
   regional migration baseline, inventory and the future dedicated release branch.

## Naming conventions

`review/pr-<number>-<topic>`, `hotfix/<issue>-<topic>`, `test/<topic>`,
`release/<release-name>`; one branch and path per active task. `release/sigap-regional-v1`
is a proposed future branch, not created by this work.

## Active safety checkpoints

`safety/pre-integration-20261118-e633cc0`, `safety/pre-main-sync-3f0f768` and the
`backups/` artifacts are preservation points. They must never be deleted or rewritten.

## Preflight and post-task audit

Run the read-only preflight (worktree list, branch -avv, status, remotes, toplevel,
common-dir, hooksPath) before any work. At the end, re-audit: verify the branch/worktree
you touched, that work is committed or deliberately preserved, that no untracked file was
lost, and report every new branch, worktree, push and cleanup action. Do not delete or
prune stale worktrees or local-only branches without owner approval.

## Known safety constraints

- No force-push, history rewrite, reset --hard, clean, branch -D or worktree remove.
- No direct `main` modification; synchronize the primary checkout only by fast-forward.
- No production deployment, production DB change, or release-branch creation.
- Production 0011 convergence and migrations 0012–0014 remain separately authorized.
- TLS edge, SMTP, real secrets, least-privilege `auth` role, durable rate limiting and
  audit sink remain owner/infrastructure blockers. `SESSION_LIFECYCLE=PARTIAL`,
  `STAFF_MFA=PARTIAL`, `PRODUCTION_READINESS=NOT_READY`.

## Recovering current local topology without guessing

Public docs intentionally omit absolute paths. The current machine-specific worktree
inventory is stored in the project's OpenChamber knowledge (local to this workstation);
alternatively regenerate it from the preflight commands above. Never infer a branch's
purpose from its name — mark uncertain entries UNKNOWN.

## Where to resume

Read the policy links, re-run the preflight, confirm `origin/main` and the PR head, then
continue from the subsystem review evidence in PR #97. Do not restart completed
integration, migration or attribution work without a concrete reason.
