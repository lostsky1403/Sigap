# Local checkout reconciliation — `F:\Sehat-Hub` (post-rewrite re-sync)

**Date:** 2026-11-13 · **Reference:** `design/local-checkout-reconciliation.md` (full read-only inventory)

The stale main checkout was on `local/r2-drill @ 704633c` — pre-rewrite ancestry. A push or merge from it would have reintroduced the 32 AI-trailer commits and, with them, the `codebuff-team` / `traeagent` / `claude` contributor-graph entries.

## What was done (non-destructive)

1. **Archive-tagged every local branch first.** 23 tags `archive/<branch>-pre-rewrite-2026-11-13` point at the exact pre-rewrite tips. Nothing was lost: every branch tip is still reachable by name, and the old `main` tip `87f218e` is reachable via `archive/main-pre-rewrite-2026-11-13`.
2. **`git fetch --prune origin`** so the remote-tracking refs point at the rewritten tips.
3. **Re-pointed the 13 content-identical mirror branches** (`ci/*`, `fix/auth-form-actions`, `fix/grpc-readyz-idle-connect`, `fix/prod-localhost-binds`, `fix/request-id-propagation`, `fix/web-supabase-ws-transport`, `ops/release-readiness-hardening`) to their rewritten `origin/*` tips. Their tree was already byte-identical — only the commit topology was pre-rewrite.
4. **Re-pointed local `main`** to `origin/main` (`a5c0e34`).
5. **Moved the stale checkout itself** off `local/r2-drill` onto `main` (`a5c0e34`).
6. **Cleared two phantom modifications** (`deploy/systemd/sigap-backup-postgres.sh`, `sigap-postgres-backup.service`) — proven byte-identical to `origin/main` (stat-cache artifact, no content change).
7. **Left the unique-work branches alone**: `design/sigap-v2-prototype`, `feat/sigap-regional-pilot`, `feat/sigap-selfhosted-auth`, `feat/sigap-v2-implementation`, `local/r2-drill`, `safety/pre-main-sync-3f0f768`, `design/ui-ux-overhaul`. These carry content that is not on the remote (or diverges from it) and are preserved as-is.

**No `reflog expire`, no `gc --prune=now`, no branch deletion, no force-push.** Unreachable objects remain in the repository and can be resurrected by SHA from the archive tags.

## Verified invariant

```
branches still reaching the OLD main tip (b96c5174): NONE
```

No local branch can reintroduce the pre-rewrite history. The 6 preserved local-only branches contain **0** AI-trailer commits each (they were rebuilt on the rewritten base).

## Remaining caution

`local/r2-drill` and `safety/pre-main-sync-3f0f768` are local-only and still point at pre-rewrite commits (32 / 31 trailer commits respectively). They are harmless while unpushed. **If one is ever pushed, it would reintroduce its trailer commits.** Before pushing either, rebase it onto `origin/main` or delete it after confirming nothing unique is lost (both tips are archived under `archive/*-pre-rewrite-2026-11-13`).

## Recovery

```bash
# restore any archived pre-rewrite tip:
git branch <name> archive/<branch>-pre-rewrite-2026-11-13
# or fetch it from the durable backup:
git fetch "%USERPROFILE%\sigap-attribution-backup\sigap-mirror.git" 'refs/heads/*:refs/archive-mirror/*'
```
