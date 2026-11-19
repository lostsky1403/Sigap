# GitHub AI-attribution rewrite — rehearsal report (`lostsky1403/Sigap`)

**Date:** 2026-11-11 · **Status:** REHEARSAL ONLY — **nothing was pushed, and the real repository is untouched.**

| | |
|---|---|
| **Frozen revision** | `bc99280c50bf8ab8f1a82408370cba21cb09725d` — `F:\sigap-design-v2`, branch `feat/sigap-selfhosted-auth`, captured 2026-11-11T11:10:34+07:00 |
| **Tooling** | `git 2.43.0.windows.1` · `git-filter-repo 2.47.0` (`C:\Python313\Scripts\git-filter-repo.exe`) · Python 3.13.3 |
| **Method** | `git clone --mirror F:\sigap-design-v2` → scrub the clone → verify offline. The source repo was only ever read (`clone`/`fetch`/`rev-list`). |
| **Related docs** | `design/github-attribution-audit.md` (evidence), `design/github-attribution-owner-decision.md` (options), `docs/operations/git-attribution.md` (prevention) |

### Where the rehearsal lives (all disposable, outside the worktree)

| Path | Role |
|---|---|
| `%TEMP%\sigap-rewrite-rehearsal-mirror` | the rewritten mirror — the "after" repository |
| `%TEMP%\sigap-rw\mirror-prerewrite` | byte copy of that mirror taken **immediately before** the rewrite — the "before" objects |
| `%TEMP%\sigap-rw\` | evidence: ref manifests, `commit-map`, raw logs |
| `%TEMP%\sigap-pre-rewrite-all.bundle` | 19,843,936-byte `git bundle --all` of the pre-rewrite mirror |

**The worktree `F:\sigap-design-v2` was not modified by this rehearsal.** Verified against the manifest in § 3: all 52 refs matched exactly (no ref deleted, rewritten or force-moved), there is no `filter-repo` directory and no `refs/original`, `refs/backup` or `refs/pre-rewrite` ref there, and no `push`, `gc`, `reflog expire` or rewrite command was ever run in it — only `clone`/`fetch`/`rev-list`/`for-each-ref`.

The worktree is **live**, so refs move for reasons that have nothing to do with this rehearsal: after the freeze, `refs/heads/feat/sigap-selfhosted-auth` advanced by **2 commits made by other agents** (`b994926`, `053aea5`, both authored 2026-11-11 11:19 +07:00) — that is the only difference from the manifest. A real rewrite must therefore be preceded by a hard freeze (§ 8, item 2).

---

## 0. Answer up front

| Question | Measured answer |
|---|---|
| How many commits carry an AI `Co-authored-by:` trailer line? | **32** (line-anchored, case-insensitive: `^[ \t]*co-authored-by:`), 5 distinct identities |
| How many commits get a new SHA if those trailer lines are stripped? | **438 of 484** in the rehearsed universe; **296 of 342** on the public branch `main` |
| How many keep their SHA? | **46** (the bootstrap chain that precedes the oldest trailer commit) |
| Are file trees preserved? | **Yes — 484/484**. Every mapped commit has an identical tree; `git diff --stat <old> <new>` is empty for all 32 targets *and* for the branch tips |
| Are author / committer / dates preserved? | **Yes — 484/484** identical name, e-mail and ISO date |
| Did **only** the 32 messages change? | **Yes** — `message_altered = 32`, `message_identical = 452`, `problems = 0` |
| Any trailer line left anywhere in the rewritten history? | **No** — `0` matches in the rewritten branches + tags |
| Which refs move? | **All 22 local branches**, **3 of 4 tags**, `refs/stash` and both `refs/trae_merge/*`; `refs/tags/v1.0.0-beta` is the only ref that keeps its SHA. **23** `refs/remotes/origin/*` refs are folded into local branches |
| Do PRs reference the changed SHAs? | **All 93** PRs (every PR in the repo) reference at least one changed SHA in `mergeCommit` and/or `headRefOid` |
| Do feature branches rebase cleanly? | The rehearsal's own replay **conflicts** — but a control run on the **unchanged** base conflicts **identically**, so the conflict is pre-existing in the branch history and is **not introduced by the rewrite** |
| Was anything pushed? | **No.** No `git push`, no `git remote set-url`, no API write of any kind |

---

## 1. What exactly was rehearsed

A **message-only** history rewrite: for each of the 32 commits, delete the `Co-authored-by:` trailer **line** from the commit message and change nothing else. Trees, author, committer, both dates and the parent graph stay as they are (parents are only remapped to their rewritten counterparts, which is unavoidable).

Run inside the mirror clone only:

```bash
git clone --mirror F:/sigap-design-v2 "$TEMP/sigap-rewrite-rehearsal-mirror"
cd "$TEMP/sigap-rewrite-rehearsal-mirror"
git filter-repo --force --preserve-commit-hashes --message-callback '
import re
had_nl = message.endswith(b"\n")
m = re.sub(rb"(?im)^[ \t]*co-authored-by:[^\n]*\r?\n?", b"", message)
m = re.sub(rb"\n{3,}", b"\n\n", m)
m = re.sub(rb"[ \t]+\n", b"\n", m)
m = re.sub(rb"\n+$", b"", m)
return m + (b"\n" if had_nl else b"")
'
```

Three details in that callback are load-bearing; each was found by getting it wrong first (§ 10):

* `\r?\n?` — the trailing newline must be **optional**. 15 of the 32 messages store the trailer as the **last line with no terminating LF** (all GitHub squash-merges), so a `\n`-terminated pattern silently skips them.
* `--preserve-commit-hashes` — without it, filter-repo *also* rewrites abbreviated commit SHAs **inside** message bodies. Measured: `main @ de15089` became `main @ 93decec` in `05dd7da9` (PR #64), and **33 extra messages** were edited. With the flag, exactly 32 messages change.
* `had_nl` — keeps the "no trailing LF" property of those 15 messages instead of normalising it.

---

## 2. The 32 targets and their rewritten SHAs

Old SHA → new SHA for the full 32 (short forms for reading; the full mapping is the `.git/filter-repo/commit-map` of the rehearsal). `msg tail` = whether the stored message ends with an LF.

| # | old SHA | new SHA | msg tail | trailer identity | subject |
|---|---|---|---|---|---|
| 1 | `030447b` | `d6dfa9d` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 2 | `0b8b7c2` | `38887c7` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | feat: public catalog endpoints and centralized web proxy auth (#48) |
| 3 | `142dc5d` | `844922b` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: sanitize proxy headers for real client IP resolution (#47) |
| 4 | `29e8c73` | `e653e4d` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 5 | `30790d3` | `47f69f7` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | feat: add security headers to API and web server (#46) |
| 6 | `3476555` | `40dc926` | none | `Co-Authored-By: internal-model` | docs: reconcile audit after postgres backup and restore (#68) |
| 7 | `48f68be` | `0bfbb4b` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: make /readyz check database connectivity and surface audit state (#49) |
| 8 | `5d762f8` | `619cb45` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 9 | `7119c69` | `13f261e` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 10 | `776d0c2` | `c048559` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 11 | `7cf4505` | `101b54c` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | fix(security): remove hardcoded Postgres password, parameterize secrets |
| 12 | `7de5952` | `b065217` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | feat: harden HTTP server with timeouts, body limits, and graceful shutdown (#50) |
| 13 | `8bf9120` | `d27d294` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | feat(api): route registry + deny-by-default 401 middleware |
| 14 | `8e57eee` | `64e78ba` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 15 | `8f0370e` | `94d58fc` | none | `Co-authored-by: t <t@t>` | chore: remove the historical gitleaks allowlist (#87) |
| 16 | `908dd21` | `212d07a` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 17 | `a144eb8` | `2ee4d4d` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | fix(security): enforce Postgres TLS (sslmode=require) and run web as non-root |
| 18 | `a22313f` | `31fcf06` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: guard dev-only capabilities by environment (#40) |
| 19 | `bd512df` | `05a91a6` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: require TLS termination confirmation outside local environment (#43) |
| 20 | `be7e159` | `bd496db` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | fix(security): close open PHI routes behind a default-off gate |
| 21 | `c179bdf` | `7948529` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 22 | `c756edb` | `a28840f` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: delete demo PHI handlers and remove SIGAP_ENABLE_DEMO_PHI (#42) |
| 23 | `cd89f10` | `628ad95` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 24 | `cfef087` | `a2d3633` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: require SIGAP_AUTH_MODE in compose and add boot banner (#41) |
| 25 | `d3ea784` | `9338753` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 26 | `da9f83b` | `a635f0c` | none | `Co-Authored-By: internal-model` | ops: add PostgreSQL backup and restore workflow (#67) |
| 27 | `dcdb643` | `6057617` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: restrict CORS loopback exception to loopback deployments (#44) |
| 28 | `dcfa16f` | `89b866b` | LF | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` | chore: pre-termination backup |
| 29 | `e8219d5` | `ef1c455` | none | `Co-Authored-By: internal-model` | docs: correct backup restore closure status (#69) |
| 30 | `e9b7237` | `4c9bb35` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | ci: pin Go via go.mod, run Rust tests, add router coverage gate |
| 31 | `ec3a80c` | `dc996fd` | LF | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` | chore: add MIT LICENSE, SECURITY.md, and CODEOWNERS |
| 32 | `f0dd744` | `f4ec998` | none | `Co-authored-by: Codebuff <noreply@codebuff.com>` | fix: remove hardcoded Access-Control-Allow-Origin: * from SSE endpoint (#45) |

Trailer census: **11** `traeagent`, **11** `Codebuff`, **6** `Claude`, **3** `internal-model`, **1** `t <t@t>`.
`31` of the 32 are ancestors of the worktree's local `main`; the 32nd (`8f0370e`, #87) is on the `ci/remove-gitleaks-allowlist` line that reaches `main` through `origin/main`.

> Deliberately **not** touched: the `🤖 Generated with Codebuff` body line and the plain-text prose in `0b02e53` ("…because Co-authored-by trailers whose e-mails map to real GitHub accounts…"). Those are body text, not trailers. Consequence: a naive `git log --grep=co-authored-by` still matches that one prose commit after the rewrite, so verification must be **line-anchored**.

---

## 3. Frozen pre-rewrite ref manifest (read-only backup manifest)

Every ref the worktree knew at capture time, all 52 — `git for-each-ref --format='%(refname) %(objectname)'`. This is the recovery manifest: any of these SHAs can be re-fetched from the untouched worktree or from the bundle.

```
refs/heads/ci/bucket-lock-drill                          f614805cfddc5e23239d81c10c515106da7439d0
refs/heads/ci/gate-a-verified                            1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3
refs/heads/ci/lock-drill-refusal-reason                  05e19616401003ae669aa0e6870b690489e60de1
refs/heads/ci/ops-suite-baseline                         092b1ed86486d15d88472d93deb5de25fb348948
refs/heads/ci/r2-list-objects-fix                        98470f020df843990d2f29d2b728d8511c6e90b2
refs/heads/ci/remove-gitleaks-allowlist                  c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e
refs/heads/ci/sigap-88-validation                        d7404374734a3783224656e37a9714b22c3385af
refs/heads/ci/sigap-r2-activation                        73955d694ec7cd1447db169236aa94c90e536327
refs/heads/design/sigap-v2-prototype                     4e470a5332b6db037ed68b2e13873f4a1ff31e4a
refs/heads/design/ui-ux-overhaul                         d7af334d381a48fbd1054060e5eaffcc6c4521a5
refs/heads/feat/sigap-regional-pilot                     522fbc48550d357f1ab162a6b316fd4e91c611b7
refs/heads/feat/sigap-selfhosted-auth                    bc99280c50bf8ab8f1a82408370cba21cb09725d
refs/heads/feat/sigap-v2-implementation                  e163189490aeea71ef798ce610a2a455d313de8d
refs/heads/fix/auth-form-actions                         919c8a797b01ab0890e781f3d0977adabd370e82
refs/heads/fix/grpc-readyz-idle-connect                  072b1b1a082a040a3f72fbc328e37671bf5c58d2
refs/heads/fix/prod-localhost-binds                      fb5204a712e08b1703759f4ffa80fdeb81c569f9
refs/heads/fix/request-id-propagation                    795aec95e03978bb25225ffa7fc9d193814d92e5
refs/heads/fix/web-supabase-ws-transport                 c86e215322f9ee9f4eabe64b3a59860d2614697d
refs/heads/local/r2-drill                                704633cc7f317b9fc3cbe1189bd41f235f4d6d57
refs/heads/main                                          87f218efe71deca217fbc6c46668e3b0be499503
refs/heads/ops/release-readiness-hardening               9ec6c52cbe439df8a3295975cf3475e727082dac
refs/heads/safety/pre-main-sync-3f0f768                  3f0f7689d8687d067dc18bd15a6a534317dfe1d8
refs/remotes/origin/ci/bucket-lock-drill                 f614805cfddc5e23239d81c10c515106da7439d0
refs/remotes/origin/ci/gate-a-verified                   1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3
refs/remotes/origin/ci/lock-drill-refusal-reason         05e19616401003ae669aa0e6870b690489e60de1
refs/remotes/origin/ci/ops-suite-baseline                092b1ed86486d15d88472d93deb5de25fb348948
refs/remotes/origin/ci/r2-list-objects-fix               98470f020df843990d2f29d2b728d8511c6e90b2
refs/remotes/origin/ci/remove-gitleaks-allowlist         c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e
refs/remotes/origin/ci/sigap-88-validation               d7404374734a3783224656e37a9714b22c3385af
refs/remotes/origin/ci/sigap-r2-activation               73955d694ec7cd1447db169236aa94c90e536327
refs/remotes/origin/design/ui-ux-overhaul                704633cc7f317b9fc3cbe1189bd41f235f4d6d57
refs/remotes/origin/docs/audit-701-production-proof      f449d46033287f613d0d391d741bb3fec3673c64
refs/remotes/origin/feat/prod-api-port-override          f8b4ef852597db742358a954fc67a85a3d0d88a7
refs/remotes/origin/feat/web-supabase-auth               fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6
refs/remotes/origin/fix/auth-form-actions                919c8a797b01ab0890e781f3d0977adabd370e82
refs/remotes/origin/fix/backup-postgres-pg16-client      68852794f263ec89068bbb4580e090f392afc975
refs/remotes/origin/fix/backup-postgres-server-version-gate 6d90116c249657cd33b5cf02e65248d14a43d879
refs/remotes/origin/fix/compose-auth-env-passthrough     b7aff840981e6521ff9ccaaa98d01265727a156b
refs/remotes/origin/fix/grpc-readyz-idle-connect         072b1b1a082a040a3f72fbc328e37671bf5c58d2
refs/remotes/origin/fix/prod-localhost-binds             fb5204a712e08b1703759f4ffa80fdeb81c569f9
refs/remotes/origin/fix/prod-ports-override-replace      2595bc327059bf83c34ff915d49cf385a075d68f
refs/remotes/origin/fix/request-id-propagation           795aec95e03978bb25225ffa7fc9d193814d92e5
refs/remotes/origin/fix/web-supabase-ws-transport        c86e215322f9ee9f4eabe64b3a59860d2614697d
refs/remotes/origin/main                                 b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3
refs/remotes/origin/ops/release-readiness-hardening      9ec6c52cbe439df8a3295975cf3475e727082dac
refs/stash                                               7419094c3fe4ce1b8b50270530f2fa4a9eea66f6
refs/tags/demo-handoff-green-2026-07-10                  62754ed83d97e5d887ac92a7893b95f8f5a95851
refs/tags/frontend-demo-green-2026-07-10                 001057de41fd4678983da243caff791f1a8cfc71
refs/tags/local-demo-green-2026-07-09                    fc0df5c8778d4775336b8bd3f13060e21814bff0
refs/tags/v1.0.0-beta                                    3380ec26b303e4f1525185a32693b5284c7acb37
refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/mainHash        0f454996dd5c7974009e721d221c805942569866
refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/worktreeHash  f4f7c9add59c93e79b195fd37373707bfe6a0e28
```

All four tags are **annotated**, so the manifest SHAs above are the *tag objects*; the commits they point at are:

| tag (tag object) | commit |
|---|---|
| `demo-handoff-green-2026-07-10` (`62754ed8`) | `0f454996dd5c7974009e721d221c805942569866` |
| `frontend-demo-green-2026-07-10` (`001057de`) | `7ec14da1a9628b985f896d720c54199ff4460695` |
| `local-demo-green-2026-07-09` (`fc0df5c8`) | `9339571979c44dd74d8b735dfb0043f0c4b7e3a8` |
| `v1.0.0-beta` (`3380ec26`) | `13c59e7a03dcca019e9125ea6051ace4344e1177` |

Only `v1.0.0-beta`'s commit (`13c59e7a`) predates every trailer commit, which is why both it and its tag object keep their SHA.

Also recorded, read-only: `git remote -v` → `origin https://github.com/lostsky1403/Sigap.git` (fetch+push); the worktree branch had no upstream, `origin/main` = `b96c5174`.

---

## 4. Verification results (all measured in the rehearsal)

Every number below came from commands run against the two temp repositories; raw logs and the `commit-map` are in `%TEMP%\sigap-rw\`.

### 4.1 Trees, identity, graph (the whole rewrite, not just the targets)

Old objects read from `mirror-prerewrite`, new objects from the rewritten mirror — 484 mapped commit pairs:

| Check | Result |
|---|---|
| commit pairs in `commit-map` | **484** |
| commits with a new SHA | **438** |
| commits keeping their SHA | **46** |
| messages altered | **32** |
| SHAs changed but message untouched (descendants only) | **406** |
| tree mismatches (`<old>^{tree}` ≠ `<new>^{tree}`) | **0 / 484** |
| author / committer / ISO-date mismatches | **0 / 484** |
| parent-count or parent-remap mismatches | **0 / 484** |
| messages differing by anything other than the removed trailer line | **0** |
| unresolved objects | **0** |

### 4.2 The 32 targets specifically

* `git diff --stat <old> <new>` → **empty for all 32** (0 lines of output each).
* trailer line still present in the rewritten message → **0 / 32**.
* `tree_same` → **32 / 32**.

### 4.3 Global invariants

* Old tip tree `1bc273aa6c7ab5a603d94c671d653eba4288cf6d` == new tip tree — `git diff --stat bc99280 59066ae` is **empty**.
* Trailer-strict commit count across the rewritten branches+tags: **0** (it was 32 before).
* `git rev-list --all --count`: **484 before → 484 after**; heads **473 → 481** (the folded `origin/*` refs add reachable commits); tags **111 → 111**.
* Per-ref history size is identical for **all 52 refs** (e.g. `refs/heads/feat/sigap-selfhosted-auth` 432 → 432, `refs/heads/main` 180 → 180, `refs/tags/demo-handoff-green-2026-07-10` 111 → 111). No commit was added, dropped or merged.
* **Determinism:** an independent earlier rehearsal with a different ref scope produced the *same* rewritten SHA for all 473 SHAs the two runs shared.

### 4.4 Cross-check against the audit

`design/github-attribution-audit.md` § 8.1 predicted analytically that **296 of 342** commits on `origin/main` would change and **46** would keep their SHA. The rehearsal rewrote `origin/main` (`b96c5174`, 342 commits → `dbadcd3d`) and measured exactly **296 changed / 46 preserved**, tree identical. § 8.2's "45 branch refs, 3 of 4 tags" also matches (45 = 22 local + 23 remote-tracking; 3 of 4 tags move). The audit's numbers and the rehearsal's numbers were produced by different methods and agree.

---

## 5. Ref, tag and PR impact

### 5.1 What moves

* **22 / 22 local branches** get a new tip.
* **3 / 4 tags** get a new tip; `refs/tags/v1.0.0-beta` (`3380ec2`, 26 commits) is the only ref in the whole repository that keeps its SHA — it predates every trailer commit.
* `refs/stash` and both `refs/trae_merge/*` refs also move (they are refs like any other; `filter-repo` rewrites what it is given).
* **23 `refs/remotes/origin/*`** are deleted and folded into `refs/heads/*`. 22 of them carried the same name *and the same SHA* as an existing local branch, so the fold is a no-op; the side effect worth knowing is that **7 branches that existed only on the remote** (`docs/audit-701-production-proof`, `feat/prod-api-port-override`, `feat/web-supabase-auth`, `fix/backup-postgres-pg16-client`, `fix/backup-postgres-server-version-gate`, `fix/compose-auth-env-passthrough`, `fix/prod-ports-override-replace`) re-appear as **local** branches in the rewritten clone.
* `origin/main` (`b96c5174`, 342 commits) is the one exception: it was **not** folded onto the local `main`, because the worktree's local `main` (`87f218ef`, 180 commits) is a different, older line. Its rewritten equivalent `dbadcd3d` survives — `origin/main` is an ancestor of the frozen branch tip, so that commit is reachable from three rewritten branches — but **no ref names it** in the rehearsal clone.
* `origin` itself is removed by filter-repo (it prints `NOTICE: Removing 'origin' remote`); a real operation must re-add the remote deliberately before pushing.

Per-ref tip mapping (first 7 and the two that decide the shape of the change):

| Ref | old | new |
|---|---|---|
| `refs/heads/main` | `87f218ef` | `4d1669ee` |
| `refs/heads/feat/sigap-selfhosted-auth` | `bc99280c` | `59066aea` |
| `refs/heads/design/ui-ux-overhaul` | `d7af334d` | `c81003c0` |
| `refs/heads/ops/release-readiness-hardening` | `9ec6c52c` | `3ac4099b` |
| `refs/heads/local/r2-drill` | `704633cc` | `6649c78c` |
| `refs/heads/ci/bucket-lock-drill` | `f614805c` | `6d74d83d` |
| `refs/heads/feat/sigap-regional-pilot` | `522fbc48` | `feae4ca7` |
| `refs/tags/demo-handoff-green-2026-07-10` | `62754ed8` | `0a94a3f3` |
| `refs/tags/frontend-demo-green-2026-07-10` | `001057de` | `090819c4` |
| `refs/tags/local-demo-green-2026-07-09` | `fc0df5c8` | `c147729f` |
| `refs/tags/v1.0.0-beta` | `3380ec26` | `3380ec26` (unchanged) |
| `refs/remotes/origin/main` | `b96c5174` | `dbadcd3d` (ref removed; commit reachable from 3 rewritten branches, no ref names it) |

The full 52-row table is `%TEMP%\sigap-rw\f-refmove.txt`, and `%TEMP%\sigap-rewrite-rehearsal-mirror\filter-repo\ref-map` holds filter-repo's own old→new list.

> Note the two different `main`s: the **worktree's local `main`** is `87f218ef` (180 commits, **162 behind** `origin/main`), while **`origin/main`** is `b96c5174` (342 commits). A real operation must decide which one is authoritative *before* rewriting.

### 5.2 PRs

* `gh pr list --state all` → **93 PRs, all MERGED**.
* **93 / 93** reference at least one changed SHA through `mergeCommit.oid` and/or `headRefOid`. Examples: PR #48's `mergeCommit` is `0b8b7c29` (a target), PR #93's `mergeCommit` is `b96c5174` (`origin/main`).
* GitHub advertises **93 `refs/pull/*`** refs for this repository, and they are **still resolvable read-only** — verified with `git ls-remote`:
  `refs/pull/48/head = b0635fab`, `refs/pull/87/head = c443ce02`, `refs/pull/40/head = d37f8fed`, `refs/pull/50/head = 510a915c`.
  Those refs pin the *pre-squash branch tips*, which are exactly the commits that would otherwise become unreachable after a force-push — see § 7.
* Consequence for a real operation: every merged PR's "commits" view keeps pointing at the objects GitHub retained, while the branch SHAs it references disappear. Nothing breaks functionally, but every stale permalink in review threads becomes a 404 on the branch view and resolves only via the PR ref.

---

## 6. Feature-branch rebase assessment

**Question asked of the rehearsal:** after such a rewrite, does an existing feature branch rebase onto the new base without conflicts?

**Dry run performed (in the clone, on a scratch branch):**

```bash
# OLD branch = pre-rewrite tip; OLD base = pre-rewrite main; NEW base = rewrite of OLD base
git checkout -B test-branch refs/prerewrite/heads/feat/sigap-selfhosted-auth
git rebase --onto <NEW base> <OLD base> test-branch
```

Result: **fails** — `Rebasing (4/249)`, `error: could not apply 3d934ca... fix(api): enforce notification scope`, one conflicted file, `apps/api/internal/notification/types.go`, 1 conflict entry.

**Control experiment** — the *same* commit range rebased onto the **unchanged old base** (`git rebase --onto <OLD base> <OLD base>`) — fails **identically**: same commit `3d934ca`, same single file, same conflict count, identical transcript prefix.

**Conclusion:** the conflict is a pre-existing property of that branch's history (its own commits do not replay cleanly, most likely because of duplicated/cherry-picked changes), **not** something the attribution rewrite introduces. The rewrite leaves every intermediate content state untouched — which is exactly why the two runs fail at the same place. A prototype-verifiable consequence of that: since trees are identical everywhere, a rebase can only fail for reasons that already existed.

Also relevant to migrating developer clones:

* Per-ref history sizes are unchanged (§ 4.3), so no work is lost — but every local branch must be **re-pointed at the new SHAs** or the clone must be re-made. Merging is not an option: old and new histories share no commit identity.
* 17 of the 22 local branches are published (`origin/*` exists); the 5 without an upstream (`design/sigap-v2-prototype`, `feat/sigap-regional-pilot`, `feat/sigap-selfhosted-auth`, `feat/sigap-v2-implementation`, `safety/pre-main-sync-3f0f768`) are local-only and could simply be re-created from the rewritten mirror.
* 406 commits change SHA without being edited. Their permalinks, any `CHANGELOG` links, and any CI/deployment pin break even though the content is byte-identical.

---

## 7. Recovery strategy

Ordered from "always available" to "last resort".

1. **The real repository is the backup — right now.** Nothing was pushed and no ref in `F:\sigap-design-v2` was rewritten: every SHA in the § 3 manifest is still present in that repository, and the working branch has merely advanced by two later commits. The single most important recovery control is therefore *not to rewrite until a frozen mirror has been verified*, and to keep the source repository and its `origin` untouched until the very last step.
2. **Export before rewriting (required).** Verified working in the rehearsal:
   ```bash
   git clone --mirror F:/sigap-design-v2 "$TEMP/sigap-freeze"
   git -C "$TEMP/sigap-freeze" bundle create "$TEMP/sigap-pre-rewrite-all.bundle" --all   # 19,843,936 bytes here
   git -C "$TEMP/sigap-freeze" for-each-ref --format='%(refname) %(objectname)' > refs-manifest.txt
   Copy-Item -Recurse "$TEMP/sigap-freeze" "$TEMP/sigap-freeze-copy"                      # 41 MB here
   ```
   Restore is then `git clone`/`git fetch <bundle>` + `git update-ref` from the manifest — the manifest is the recovery map.
3. **Do not rely on in-repo refs as the backup.** Measured: 26 `refs/backup/pre-rewrite/*` refs were created before the rewrite and **26/26 were rewritten by filter-repo** (`CLOBBERED`), and filter-repo's repack **pruned the replaced objects** (old `refs/stash` and `refs/trae_merge` objects no longer resolvable in the rewritten clone; only the ones re-fetched from the source did). `filter-repo` does **not** create `refs/original/*`. If an in-repo safety net is wanted, use `git filter-branch --msg-filter`, which does leave `refs/original/refs/heads/*` behind — at the cost of being far slower and far more error-prone on Windows.
4. **`commit-map` is the mapping, keep it.** `filter-repo` writes `.git/filter-repo/commit-map` (484 `old new` pairs here). It is the only authoritative record of what became what, and it is what makes a partial roll-forward/roll-back possible.
5. **GitHub retains the old objects through PR refs.** 93 `refs/pull/*` refs exist and resolve; they pin the pre-squash branch tips of every merged PR. They survive a force-push (GitHub keeps PR refs for merged PRs), so a fetchable copy of most pre-rewrite content stays on the remote even after the rewrite — but **not** of the 15 squash-merge commits themselves (those exist only on the branch and are covered by the bundle).
6. **Teammates and CI.** Any existing clone, fork, or deployment pinned to an old SHA must be re-cloned or hard-reset from the manifest; `git pull` on an old clone would *reintroduce* the pre-rewrite objects and create a divergent history.

---

## 8. What a real operation would require (approval gate)

**Nothing below has been executed. This section describes the gate, not a plan of record.**

The owner must give explicit, written approval for Option 2 of `design/github-attribution-owner-decision.md` **before** the first command below runs. Until that approval exists, nothing on the remote may be written.

Gate checklist — every item must be true *before* any `push`:

1. ☐ Owner approval recorded, naming the frozen revision SHA and the ref list.
2. ☐ Repository frozen: no other agent/user committed to `F:\sigap-design-v2` since the freeze SHA (the worktree was live during this rehearsal — HEAD moved 5 times in ~30 minutes — so a freeze is mandatory, not a formality).
3. ☐ Mirror cloned, bundle + refs manifest + `commit-map` captured to storage outside the repo (§ 7.2).
4. ☐ The rewrite re-run on the frozen mirror and the five invariants re-measured: 484/484 trees identical, 0 author/committer/date mismatches, consistent parent remap, `message_altered = 32`, trailer-strict count `0`.
5. ☐ Every ref that must move is enumerated and the two `main`s (`87f218ef` local vs `b96c5174` remote) reconciled.
6. ☐ Only then: force-push the refs one by one (`git push --force <remote> <new>:<ref>`); re-add the `origin` remote first, because filter-repo removes it. Decide explicitly whether tags are pushed (`--force` on 3 tags) or left alone.
7. ☐ Post-push: re-read `/stats/contributors` before claiming the graph changed — the audit already warns this is cached and not instantaneous.

Explicitly **out of scope / not authorised** by this rehearsal: pushing, deleting the `origin` remote of the real repository, running `filter-repo` or `filter-branch` inside `F:\sigap-design-v2`, `git reflog expire`/`git gc` there, and any rewrite of `refs/stash` or `refs/trae_merge/*` in the real repository.

---

## 9. Tooling findings (each one cost a wrong run)

1. **`--refs` silently disabled the rewrite.** `git filter-repo --refs 'refs/heads/*' --refs 'refs/tags/*'` completes successfully, prints `New history written`, and rewrites **nothing** (empty `commit-map`, messages untouched). `--refs` implies `--partial`; repeated flags do not accumulate in 2.47.0. Repro: a 2-commit scratch repo, that exact command → trailer still present. **Recommendation: do not pass `--refs`; the default (all refs) rewritten scope is also the safer one for a mirror.**
2. **Trailer regex must tolerate a missing final LF.** 15 of the 32 messages end with `Co-authored-by: …` and **no newline** (GitHub squash-merges). `^co-authored-by:[^\n]*\r?\n` matches 17 and silently skips those 15; the rehearsal's first pass then produced 15 new commits *still* carrying the trailer. `\r?\n?` (or anchoring with `$` under `re.M`) catches both shapes.
3. **`--preserve-commit-hashes` matters for a "message-only" claim.** By default filter-repo rewrites abbreviated SHA references *inside* message bodies (`main @ de15089` → `main @ 93decec`), editing 33 further messages. That is still message-only, but it is not "only the trailer lines", and it is invisible unless you diff messages. With the flag: 32 altered messages, 0 unexplained.
4. **filter-repo removes `origin`, folds `refs/remotes/origin/*` into `refs/heads/*`, prunes replaced objects, and never creates `refs/original/*`.** All four were observed. The first is a deliberate safety feature; the second changes the shape of the ref namespace; the third is why an in-repo ref-based backup must not be trusted (§ 7.3).
5. **A `--mirror` rehearsal is not equivalent to a plain clone.** `git clone --no-hardlinks F:/sigap-design-v2` gives the clone only the worktree's **local** branches as `origin/*` (473 reachable commits, 22 branch refs) and misses the 23 remote-tracking refs. The mirror clone gave 484 commits and all 52 refs. Use `--mirror` when the goal is "every ref the repository knows".

---

## 10. Limits — what this rehearsal does not prove

* It does **not** prove that GitHub will drop the three AI accounts from the Contributors graph. That is a GitHub-side recomputation, cached, and explicitly out of scope here (`design/github-attribution-owner-decision.md` § 5 already says no method can promise it).
* It runs against a **local mirror**, not against GitHub. A real force-push can still be rejected (branch protection), and `refs/pull/*` behaviour under a force-push is asserted from GitHub's documented retention of merged-PR refs, not from an actual push — `[INFERENCE]` where it goes beyond the `ls-remote` evidence in § 5.2.
* The rehearsed universe is the worktree's 52 refs. GitHub may hold objects the worktree never fetched.
* The worktree was **live** during the rehearsal (other agents committed to it; its HEAD moved from `60de7d6` to `bc99280` and beyond). All numbers are pinned to the frozen revision named at the top.
* Signature status was not re-measured here; `design/github-attribution-audit.md` reports 15 GitHub-signed, 17 unsigned commits, and a rewrite necessarily re-mints the 15 signed ones as unsigned.
* No phase gate, test suite, linter or build was run — by instruction and by scope.

---

## 11. Cleanup

The rehearsal is entirely disposable. When the owner is done with it:

```powershell
Remove-Item -Recurse -Force "$env:TEMP\sigap-rewrite-rehearsal-mirror"
Remove-Item -Recurse -Force "$env:TEMP\sigap-rw"
Remove-Item -Force "$env:TEMP\sigap-pre-rewrite-all.bundle"
```

Nothing in `F:\sigap-design-v2` — refs, objects, config, working tree — depends on those directories, and deleting them cannot affect the repository.
