# Main-checkout re-sync after the attribution rewrite — REINTRODUCTION HAZARD

The rewrite corrected `lostsky1403/Sigap`, but the **main checkout** `F:\Sehat-Hub` was still on pre-rewrite ancestry (`local/r2-drill @ 704633c`). It held the old trailer commits locally; a future push/merge from it would have reintroduced the `codebuff-team` / `traeagent` / `claude` contributor-graph entries.

> **STATUS: RESOLVED 2026-11-13.** The re-sync below has been EXECUTED (non-destructively). See
> `design/local-checkout-reconciliation.md` for the per-branch inventory, the archive-tag list and the
> verified invariant ("branches still reaching the old main tip: NONE").
>
> The procedure was deliberately performed **without** `reflog expire` / `gc --prune=now`: every
> pre-rewrite tip stays reachable under an `archive/<branch>-pre-rewrite-2026-11-13` tag, so no
> local-only work is destroyed and nothing needs re-materialising.

## Verify the exposure (read-only)

```bash
cd F:\Sehat-Hub
git log --all --format='%(trailers:key=Co-authored-by,valueonly)' | grep -icE 'traeagent|codebuff|claude|internal-model|t <t@t>'
# > 0  => this checkout still carries old history
```

## Re-sync procedure (local-only, non-destructive, fully reversible)

```bash
cd F:\Sehat-Hub

# 1. Refresh remote-tracking refs to the rewritten tips.
git fetch --prune origin

# 2. ARCHIVE FIRST — never delete before the tip is reachable by name.
for b in $(git for-each-ref --format='%(refname:short)' refs/heads/); do
  git tag "archive/${b//\//-}-pre-rewrite-$(date +%F)" "$b"
done

# 3. Re-point the branches whose content already matches the rewritten origin.
#    (Verify per branch: git diff --stat origin/<b> <b>  → 0 changed files.)
git branch -f <branch> origin/<branch>

# 4. Re-point local main to the canonical baseline.
git branch -f main origin/main

# 5. Move the checkout itself off the stale branch.
git checkout main

# 6. Safety re-check — must print "clean".
git merge-base --is-ancestor b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 HEAD \
  && echo "STALE — do not push" || echo "clean"
```

**Deliberately NOT included:** `git reflog expire --expire=now --all` and `git gc --prune=now`.
Those destroy the recovery path for local-only work and make the archive tags point at pruned
objects. Object retention is preserved deliberately; a stale object cannot be pushed by accident
because no branch references it any more.

## Branches intentionally left in place

`design/sigap-v2-prototype`, `design/ui-ux-overhaul`, `feat/sigap-regional-pilot`,
`feat/sigap-selfhosted-auth`, `feat/sigap-v2-implementation`, `local/r2-drill`,
`safety/pre-main-sync-3f0f768` carry content that is not on the remote (or diverges from it). They are
preserved, not deleted, and are archived by tag.

`local/r2-drill` and `safety/pre-main-sync-3f0f768` still point at pre-rewrite commits (32 / 31
trailer commits). Harmless while unpushed — **rebase or retire them before any push.**

## Recovery

```bash
# restore an archived pre-rewrite tip:
git branch <name> archive/<branch>-pre-rewrite-2026-11-13
# or pull the objects back from the durable backup:
git fetch "%USERPROFILE%\sigap-attribution-backup\sigap-mirror.git" 'refs/heads/*:refs/archive-mirror/*'
```

Recovery of any pre-rewrite object also remains available in
`%USERPROFILE%\sigap-attribution-backup\` (mirror + bundle, restore-drill verified).
