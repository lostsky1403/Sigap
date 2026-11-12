# Main-checkout re-sync after the attribution rewrite — REINTRODUCUTION HAZARD

The rewrite corrected `lostsky1403/Sigap`, but the **main checkout** `F:\Sehat-Hub` is still on pre-rewrite ancestry (`local/r2-drill @ 704633c`). It holds the old trailer commits locally; a future push/merge from it reintroduces the three contributor-graph entries.

## Verify the exposure

```bash
cd F:\Sehat-Hub
git log --all --format='%(trailers:key=Co-authored-by,valueonly)' | grep -icE 'traeagent|codebuff|claude|internal-model|t <t@t>'
# > 0  => this checkout still carries old history (currently 32 trailer references)
```

## Re-sync / retire (local-only, safe, REVERSIBLE via the durable backup)

```bash
cd F:\Sehat-Hub
git fetch --prune origin                 # remote-tracking refs -> rewritten tips
git branch -f main origin/main           # local main -> rewritten main (a5c0e34+)
# retire the stale pre-rewrite local branches (NOT on the remote):
git branch -D local/r2-drill ops/release-readiness-hardening design/ui-ux-overhaul \
  ci/r2-list-objects-fix ci/lock-drill-refusal-reason ci/gate-a-verified ci/bucket-lock-drill \
  safety/pre-main-sync-3f0f768
# drop the now-unreachable old objects so they cannot be re-pushed:
git reflog expire --expire=now --all
git gc --prune=now
# safety re-check:
git merge-base --is-ancestor b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 HEAD \
  && echo "STALE — do not push" || echo "clean"
```

Until the above is run, treat `F:\Sehat-Hub` as **frozen** and never push from it.

Recovery of any pre-rewrite object remains available in `%USERPROFILE%\sigap-attribution-backup\` (mirror + bundle, restore-drill verified), so retiring the local refs is lossless.