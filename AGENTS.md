# AGENTS.md — the contract for AI agents working in this repository

This file is read by the agent tools used on this repository (OMP, Trae, Codebuff,
Claude Code and similar). It states the rules that apply to every change you make
here. Everything else — architecture, auth conventions, how to run the stack — is in
[CONTRIBUTING.md](CONTRIBUTING.md) and [docs/](docs/).

## Hard rule: never add AI attribution trailers

**NEVER append a `Co-authored-by:` trailer naming an AI tool or model to a commit
message, a squashed merge message, or a pull-request description.** That includes —
but is not limited to — all of these, which are the shapes that already appear in this
repository's history:

```
Co-authored-by: traeagent <traeagent@users.noreply.github.com>
Co-authored-by: Codebuff <noreply@codebuff.com>
Co-Authored-By: Claude 4.7 <noreply@anthropic.com>
Co-Authored-By: internal-model
```

The commit records what was reviewed and merged into this repository; it is not a
place to advertise the tool that typed it. The committer's name belongs there, and
that is all.

If you are a tool that has an attribution setting of its own, turn it off rather than
relying on the rules below. Two things make this rule enforceable rather than
advisory: [`.githooks/commit-msg`](.githooks/commit-msg) strips these lines before the
message is recorded, and CI ([`scripts/check-attribution.mjs`](scripts/check-attribution.mjs))
fails on any new commit that still carries one. A commit that reaches CI with an AI
trailer fails the run, so it is cheaper to never write one.

Never edit, revert, or work around those two controls. They exist because the trailers
were added 31 times in this repository's history, not because anyone is expected to
argue about them.

## Your git identity — already set, do not change it

Commits in this repository are authored as:

| Field | Value |
| --- | --- |
| `user.name` | `lostsky1403` |
| `user.email` | `moch22188ti@student.nurulfikri.ac.id` |

Do not invent an identity, do not commit as the tool's own account, and do not add
`--author` to a commit in place of the configured one. If `git config user.name` or
`git config user.email` is empty in your worktree, stop and tell the operator instead
of guessing — the "Git identity" section of
[docs/operations/git-attribution.md](docs/operations/git-attribution.md) shows how to set
it from PowerShell or the Git CLI.

## Mandatory worktree/branch preflight

Before **any** Git-changing operation, run the read-only topology preflight and follow
[docs/operations/git-worktree-policy.md](docs/operations/git-worktree-policy.md). Never
check out one branch in two worktrees, never switch the active development worktree to
investigate another PR, and never reset, clean, prune, force-push or delete a branch or
worktree without explicit owner approval. Uncertain entries are UNKNOWN, never
SAFE_TO_DELETE. The live absolute-path inventory is in the project's OpenChamber knowledge.

## Before you commit

- Commit from the worktree you edited, in that worktree's checkout of the branch.
- Leave the message body describing what changed and why; that is the part a reader
  keeps.
- Full attribution policy, install commands, and the CI gate's exit codes:
  [docs/operations/git-attribution.md](docs/operations/git-attribution.md).

If your tool reads a different project-instructions file (Claude Code reads
`CLAUDE.md`, for example), treat the two rules above as binding regardless — they are
repository policy, not a per-tool preference.
