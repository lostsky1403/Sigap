# Post-execution verification procedure (run only AFTER the owner authorizes and the push completes)

Companion to `design/github-remote-execution-manifest.md`. Execute in order; stop and roll back on any mismatch.

## 0. Preconditions

- The manifest is authorized **verbatim** (23 branches + 3 tags, the listed old→new SHAs).
- The durable backup is intact: `sha256sum -c %USERPROFILE%\sigap-attribution-backup\SHA256SUMS.txt` → all OK.
- No one else is pushing: `git ls-remote origin refs/heads/main` still equals `b96c5174…`.

## 1. Ref integrity (do this first)

1. `git ls-remote origin` → record every line.
2. Compare **every** branch and tag tip to the manifest's **new** SHAs. Any ref that still holds its **old** SHA, or holds anything not in the manifest, is a failure → stop.
3. Confirm the 93 `refs/pull/*` were **not** changed (they cannot be; if any moved, note it — it is informational, not a failure).

## 2. Source-tree integrity

For each rewritten ref: `git cat-file -p <new-ref>` tree SHA must equal the tree recorded for the old ref in `github-rewrite-rehearsal-v2.md` §4.1/§4.3. Rehearsal baseline: **570/570 trees identical, 0 mismatches** — any difference means a source change slipped in.

## 3. Message integrity (the actual correction)

1. `git log --all --format='%(trailers:key=Co-authored-by,valueonly)' origin -- ` style census → count must be **0** for `traeagent`, `Codebuff`, `Claude`, `internal-model`, `t <t@t>`.
2. Sanity: exactly **46** messages differ from the baseline messages, and the difference is **only** a removed trailer line (compare against the bundle's pre-rewrite objects).
3. Human attribution intact: no surviving `Co-authored-by` line was dropped that was not on the AI list.

## 4. Authorship + committer integrity

`git log --format='%an|%ae|%cn|%ce|%at|%ct'` over the rewritten history vs the baseline: **570/570 identical** expected. The only committer other than the owner must be `GitHub <noreply@github.com>` (platform metadata, never a contributor).

## 5. Signature reality check

Expect **0** `gpgsig` headers across the rewritten range (all 93 verified commits became unsigned — this is the accepted cost, not a regression). Record the count so the loss is on the record.

## 6. GitHub surfaces (the actual objective)

1. `GET /repos/lostsky1403/Sigap/stats/contributors` → the target accounts must be gone; expect exactly one entry (`lostsky1403`).
2. `GET /repos/lostsky1403/Sigap/graphs/contributors-data` → same.
3. `GET /repos/lostsky1403/Sigap/contributors` → still one entry (it always was).
4. Fetch the repository homepage HTML → the side panel must read `Contributors 1`; grep the page for `codebuff-team`, `traeagent`, `claude` → each must be absent from the panel markup.
5. Spot-check a former trailer commit's page → its `authors` JSON must contain only `lostsky1403`.

**Cache reality:** GitHub's contributor statistics are recomputed on a delay (commonly cited up to ~24 h; the same figure appears in GitHub's own docs). If §6 shows the accounts still present immediately after the push, that is expected — **re-read after a delay before concluding failure**, and only then follow GitHub's cache/support guidance. Do not rewrite again on the strength of a same-hour reading.

## 7. Reintroduction check

1. For each active feature branch: `git merge-base --is-ancestor <old-main-tip> <branch-tip>` must be **false** (an old-history branch would reintroduce the trailers).
2. Rebase the four unpublished branches with `--rebase-merges --onto <new-base> <old-base>` (commands in `github-rewrite-rehearsal-v2.md` §6) and confirm each final file tree matches the recorded tree.
3. Re-point the zero-unpublished branches and the stash.
4. Re-run the attribution check on a test commit: `node scripts/check-attribution.mjs --range HEAD~1..HEAD` → exit 0; and confirm the local hook is installed (`git config core.hooksPath` → `.githooks`).

## 8. Prevention status (already verified pre-execution, re-state after)

Verified on 2026-11-11: the `commit-msg` hook strips `Co-authored-by: Codebuff <noreply@codebuff.com>` while preserving `Co-authored-by: Real Human <real@example.com>`; `scripts/check-attribution.mjs` exits 1 on the historical `030447b5` and `a22313f5` and 0 on a clean commit; the CI `Commit Attribution Check` job is wired (new-commit range only); `AGENTS.md` + `docs/operations/git-attribution.md` document the policy. **`FUTURE_AGENT_ATTRIBUTION=PREVENTED`** rests on that evidence.

## 9. What is and is not removable (state it in the final report)

**Removed from the canonical repository:** the three accounts from the homepage panel, the Insights graph and the stats API — after the cache refresh.
**NOT removed:** the trailer text inside old commits reachable only through `refs/pull/*` pre-squash tips and through third-party **forks/clones** made before the rewrite; and any cached statistic until it refreshes. GitHub Support does not guarantee purging non-sensitive metadata, so "cleaned" means *canonical current history + the contributor displays*, not *every copy of the old objects*.
