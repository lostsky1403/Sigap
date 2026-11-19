# GitHub AI-attribution rewrite — rehearsal report **V2 (full ref set)** — `lostsky1403/Sigap`

**Date:** 2026-11-11 · **Status:** REHEARSAL ONLY — **nothing was pushed; the remote and `F:\sigap-design-v2` were never written to.**
**Supersedes the counting basis of `design/github-attribution-rewrite-rehearsal.md` (V1, 52 refs / 484 commits): V2 rehearses the *full* GitHub ref set (145 refs / 570 commits). Where the two universes overlap, the measured new SHAs are identical (see §8).**

| | |
|---|---|
| **Frozen revision** | `https://github.com/lostsky1403/Sigap.git` as advertised at rehearsal start: `git ls-remote` = **125 lines** = `HEAD` + 23 `refs/heads/*` + 93 `refs/pull/*/head` + 4 annotated tags (each advertised twice: ref + peeled `^{}`) |
| **Mirror actually rewritten** | 120 remote refs + HEAD, plus 25 local refs fetched from `F:\sigap-design-v2` → **145 refs, 570 reachable commits** |
| **Rules applied** | `design/github-attribution-correction-rules.md` (V2) — strip only the 5 listed trailer identities; message-only |
| **Tooling** | `git 2.43.0.windows.1` · `git-filter-repo` (2.47.0, `C:\Python313\Scripts\git-filter-repo.exe`) · Python 3.13.3 · `go1.26.7 windows/amd64` |
| **Where it lives** | `%TEMP%\sigap-rw-v2\{mirror-prerewrite, mirror-prerewrite-copy, fsck-clone, build-check, migration\, analysis\}` — all disposable |
| **Pre-rewrite backup** | `migration\pre-rewrite-all.bundle` — 20,430,507 bytes, `git bundle --all` of all 145 refs (old objects) + byte copy `mirror-prerewrite-copy` |

## 0. Answer up front

| Question | Measured answer |
|---|---|
| Ref universe rewritten | **145 refs** (23 remote branches, 4 tags, 93 `refs/pull/*/head`, 25 local refs incl. `refs/stash`), **570 commits** |
| Commits carrying a strippable trailer | **46** (32 reachable from branches+tags, 14 only from `refs/pull/*`) — identities: Codebuff 22, traeagent 11, Claude Opus 4.7 6, internal-model 6, `t <t@t>` 1 |
| Commits that changed SHA | **524 of 570** (46 keep their SHA — the bootstrap chain that precedes the oldest target) |
| Messages actually altered | **46** — exactly the 46 trailer removals; 478 commits changed SHA with a byte-identical message (descendants only) |
| Trailer lines before → after | **46 → 0** across all rewritten refs (pushable subset 32 → 0) |
| File trees preserved | **570/570 byte-identical** (0 mismatches); branch tips verified separately: `main` `20451d4f…`, feat tip `e624807d…` |
| Author/committer/dates preserved | **570/570 byte-identical** (0 mismatches) |
| Commits dropped / merges lost | **0 dropped** (570 → 570; per-ref history counts identical for all 145 refs); merges **11 → 11** |
| Refs moved | **144 of 145**; only `refs/tags/v1.0.0-beta` keeps its SHA (tag object *and* commit) |
| PRs affected | **93 of 93** (every PR references ≥1 changed SHA in `merge_commit_sha` and/or `headRefOid`) |
| Signatures | 93 signed commits before (all GitHub web-flow PGP) → **0 after** (filter-repo drops `gpgsig` on rewritten commits); 15 of the 46 targets were signed; **0/4 tags signed** |
| `git fsck --full` on a fresh clone of the result | **clean** (exit 0, no output) |
| `go build ./...` in `apps/api` of the result | **PASS** — rewritten `main` `dbadcd3d` and rewritten feat tip `8c3064ba` |
| Rebase dry-runs (A6) | **all clean, trees preserved**: selfhosted-auth (104 commits, 1 merge), regional-pilot (72, 1), v2-implementation (49, 1), design/sigap-v2-prototype (18, 0) |
| Anything pushed? | **No.** No `git push`, no remote write, no API write |

## 1. What was rehearsed

A **message-only** history rewrite over the full advertised ref set:

```bash
git clone --mirror https://github.com/lostsky1403/Sigap.git mirror-prerewrite     # 120 refs + HEAD
git -C mirror-prerewrite fetch F:/sigap-design-v2 \
    '+refs/heads/*:refs/local/heads/*' '+refs/stash:refs/local/stash' '+refs/trae_merge/*:refs/local/trae_merge/*'
git -C mirror-prerewrite remote remove origin
git -C mirror-prerewrite filter-repo --force --preserve-commit-hashes \
    --message-callback "$(cat migration/message-callback.txt)"
```

* The callback is the reference implementation in `design/github-attribution-correction-rules.md` §4 (line-anchored, case-insensitive key, 5 identities, byte-exact removal incl. the 15 no-final-LF messages).
* `--preserve-commit-hashes` prevents filter-repo from rewriting abbreviated SHAs inside message bodies; **no `--refs`** (it silently disables the rewrite in 2.47.0).
* filter-repo removes `origin`, folds `refs/remotes/*` into `refs/heads/*` (none here), rewrites all 145 refs, and writes `filter-repo/commit-map` (570 pairs), `ref-map` (145), `changed-refs` (144), `suboptimal-issues` = *"No filtering problems encountered."*
* The rehearsal process touched `F:\sigap-design-v2` **read-only** (`clone`/`fetch`/`rev-list`/`for-each-ref` only). Its only writes anywhere were the two design documents and `%TEMP%`.

## 2. The 46 targets (old → new)

| # | old SHA | new SHA | identity | msg tail | signed |
|---|---|---|---|---|---|
| 1 | `018eaf25` | `421e8a3f` | Codebuff | LF | — |
| 2 | `030447b5` | `d6dfa9d2` | traeagent | LF | — |
| 3 | `03120eb8` | `418de66a` | internal-model | LF | — |
| 4 | `0b8b7c29` | `38887c79` | Codebuff | no LF | PGP |
| 5 | `142dc5d6` | `844922bb` | Codebuff | no LF | PGP |
| 6 | `143bee57` | `bd2ac3ea` | Codebuff | LF | — |
| 7 | `1460ac8b` | `1057d9f2` | internal-model | LF | — |
| 8 | `1745e4d3` | `81e1fe5f` | Codebuff | LF | — |
| 9 | `29e8c73d` | `e653e4db` | traeagent | LF | — |
| 10 | `30790d37` | `47f69f7c` | Codebuff | no LF | PGP |
| 11 | `3476555a` | `40dc9262` | internal-model | no LF | PGP |
| 12 | `395db605` | `c814ea38` | Codebuff | LF | — |
| 13 | `4877a6bd` | `591fdff7` | Codebuff | LF | — |
| 14 | `48f68be1` | `0bfbb4b6` | Codebuff | no LF | PGP |
| 15 | `510a915c` | `c9844a09` | Codebuff | LF | — |
| 16 | `5d762f89` | `619cb450` | traeagent | LF | — |
| 17 | `6ae8c5c8` | `9cbed710` | Codebuff | LF | — |
| 18 | `7119c690` | `13f261e7` | traeagent | LF | — |
| 19 | `776d0c29` | `c048559f` | traeagent | LF | — |
| 20 | `7cf4505c` | `101b54c6` | Claude Opus 4.7 | LF | — |
| 21 | `7de59522` | `b0652172` | Codebuff | no LF | PGP |
| 22 | `81eb966f` | `5cf7a982` | Codebuff | LF | — |
| 23 | `8bf9120d` | `d27d294f` | Claude Opus 4.7 | LF | — |
| 24 | `8e57eeea` | `64e78ba3` | traeagent | LF | — |
| 25 | `8f0370e8` | `94d58fc3` | t <t@t> | no LF | PGP |
| 26 | `908dd217` | `212d07a3` | traeagent | LF | — |
| 27 | `a144eb8b` | `2ee4d4d1` | Claude Opus 4.7 | LF | — |
| 28 | `a22313f5` | `31fcf060` | Codebuff | no LF | PGP |
| 29 | `b0635fab` | `3d0b3fc2` | Codebuff | LF | — |
| 30 | `bd512dfd` | `05a91a65` | Codebuff | no LF | PGP |
| 31 | `be7e1592` | `bd496db5` | Claude Opus 4.7 | LF | — |
| 32 | `c179bdf7` | `7948529a` | traeagent | LF | — |
| 33 | `c756edba` | `a28840fd` | Codebuff | no LF | PGP |
| 34 | `cd89f10a` | `628ad959` | traeagent | LF | — |
| 35 | `cfef087a` | `a2d36332` | Codebuff | no LF | PGP |
| 36 | `d37f8fed` | `e6d1c629` | Codebuff | LF | — |
| 37 | `d3ea7847` | `9338753b` | traeagent | LF | — |
| 38 | `da9f83bf` | `a635f0c1` | internal-model | no LF | PGP |
| 39 | `dcdb6430` | `6057617d` | Codebuff | no LF | PGP |
| 40 | `dcfa16ff` | `89b866bb` | traeagent | LF | — |
| 41 | `e12cc327` | `14676556` | internal-model | LF | — |
| 42 | `e8219d56` | `ef1c4559` | internal-model | no LF | PGP |
| 43 | `e9b72377` | `4c9bb35f` | Claude Opus 4.7 | LF | — |
| 44 | `ec3a80ce` | `dc996fdf` | Claude Opus 4.7 | LF | — |
| 45 | `f0dd7443` | `f4ec998c` | Codebuff | no LF | PGP |
| 46 | `f5bb7217` | `e160c272` | Codebuff | LF | — |

32 of these are ancestors of the branch/tag refs (the set `github-attribution-audit.md` §3 lists); the other 14 are reachable only from `refs/pull/{40..50,67..69}/head` (pre-squash tips, same trailers as their squash merges). Every one of the 46 is authored by an owner identity and committed by an owner identity or GitHub web-flow — no AI identity is an author or committer in the universe.

## 3. Verification results (all measured; raw data `analysis/verification.json`)

| Check | Result |
|---|---|
| commit pairs in `commit-map` | 570 (every reachable commit) |
| new SHA | 524 | 
| SHA kept | 46 (all ancestors of `ec3a80c`, the oldest target) |
| message altered | **46** (`message_altered = 46`, `removed_lines_total = 46`) |
| SHA changed, message byte-identical | 478 |
| `new_message == reference_transform(old_message)` | **570/570**, 0 unexplained differences |
| message changed for anything other than the trailer removal | **0** |
| tree mismatches | **0 / 570** |
| author/committer/date mismatches | **0 / 570** |
| parent count or remap mismatches | **0 / 570**; merges 11 → 11 |
| commits dropped | **0** — universe 570 → 570; per-ref `rev-list --count` identical for **all 145 refs**; map injective; all old commits mapped; all new commits reachable |
| line-anchored trailer census after (all rewritten refs) | **0** (was 46) |
| trailer census after on pushable refs (heads+tags) | **0** (was 32) |
| `git fsck --full` (fresh clone of the result) | clean |
| `go build ./...` in `apps/api` @ rewritten `main` `dbadcd3d` / feat tip `8c3064ba` | exit 0 / exit 0 |
| tip trees (rewritten == pre-rewrite) | `main` `20451d4f7190ccd42b4b0859419d4c1a660a0de4` ✓ · feat `e624807da3bdd75fc982c55d325439b89323c0b0` ✓ |
| tag objects | 3 re-created with the same message/tagger and remapped target; `v1.0.0-beta` byte-identical |

Mid-run finding (kept as a tooling note): the first callback pass skipped the **final line when it had no terminator** — exactly the 15 no-final-LF messages — and left the census at 15. Fixed (`_is_artifact` guard) and the rewrite re-run from the pristine copy; all numbers above are from the corrected run.

## 4. Ref, tag and PR impact

### 4.1 Remote branches (force-push targets — 23)

| remote branch | old tip | new tip |
|---|---|---|
| `ci/bucket-lock-drill` | `f614805c` | `6d74d83d` |
| `ci/gate-a-verified` | `1ba464bf` | `8a4a406c` |
| `ci/lock-drill-refusal-reason` | `05e19616` | `cba2dcfa` |
| `ci/ops-suite-baseline` | `092b1ed8` | `7f538076` |
| `ci/r2-list-objects-fix` | `98470f02` | `25c5225b` |
| `ci/remove-gitleaks-allowlist` | `c443ce02` | `060a0d00` |
| `ci/sigap-88-validation` | `d7404374` | `777945b2` |
| `ci/sigap-r2-activation` | `73955d69` | `3ce46fa5` |
| `design/ui-ux-overhaul` | `704633cc` | `6649c78c` |
| `docs/audit-701-production-proof` | `f449d460` | `7267c5c1` |
| `feat/prod-api-port-override` | `f8b4ef85` | `db103b79` |
| `feat/web-supabase-auth` | `fbd60b24` | `22415868` |
| `fix/auth-form-actions` | `919c8a79` | `99118126` |
| `fix/backup-postgres-pg16-client` | `68852794` | `ec5d7fe7` |
| `fix/backup-postgres-server-version-gate` | `6d90116c` | `8c76a925` |
| `fix/compose-auth-env-passthrough` | `b7aff840` | `de9af9f0` |
| `fix/grpc-readyz-idle-connect` | `072b1b1a` | `40900393` |
| `fix/prod-localhost-binds` | `fb5204a7` | `95c1ec79` |
| `fix/prod-ports-override-replace` | `2595bc32` | `051d2d5d` |
| `fix/request-id-propagation` | `795aec95` | `059088a2` |
| `fix/web-supabase-ws-transport` | `c86e2153` | `be018189` |
| `main` | `b96c5174` | `dbadcd3d` |
| `ops/release-readiness-hardening` | `9ec6c52c` | `3ac4099b` |

### 4.2 Local-only branches (not on GitHub) + stash

| local-only branch | old tip | new tip | unpublished | rebase base |
|---|---|---|---|---|
| `design/sigap-v2-prototype` | `4e470a53` | `98feb053` | 18 | remote `design/ui-ux-overhaul` @ `704633cc` |
| `feat/sigap-regional-pilot` | `522fbc48` | `feae4ca7` | 72 | remote `main` @ `b96c5174` |
| `feat/sigap-selfhosted-auth` | `a5423ec3` | `8c3064ba` | 104 | remote `main` @ `b96c5174` |
| `feat/sigap-v2-implementation` | `e1631894` | `122d56c0` | 49 | remote `main` @ `b96c5174` |
| `safety/pre-main-sync-3f0f768` | `3f0f7689` | `81d97104` | 0 | already in rewritten history (0 unpublished) |

All other local branches (`ci/*`, `fix/*`, `local/r2-drill`, `ops/*`, `main`, `design/ui-ux-overhaul`, …) share name **and SHA** with a remote counterpart except local `main` (`87f218ef` → `4d1669ee`) and local `design/ui-ux-overhaul` (`d7af334d` → `c81003c0`); all have **0 unpublished commits** → migration is just re-pointing at the mapped SHA. `refs/stash` (`7419094c`) is rewritten in place to `ecf2aeea…` and remains a valid stash entry (its parent chain is remapped). Full tables: `migration/branch-map.tsv`, `migration/pullref-map.tsv`.

### 4.3 Tags (4)

| tag | old tag obj | new tag obj | old target | new target | moved |
|---|---|---|---|---|---|
| `demo-handoff-green-2026-07-10` | `62754ed8` | `0a94a3f3` | `0f454996` | `4c1dd9ba` | yes |
| `frontend-demo-green-2026-07-10` | `001057de` | `090819c4` | `7ec14da1` | `9a0c3ce7` | yes |
| `local-demo-green-2026-07-09` | `fc0df5c8` | `c147729f` | `93395719` | `e4ebf2d1` | yes |
| `v1.0.0-beta` | `3380ec26` | `3380ec26` | `13c59e7a` | `13c59e7a` | no |

### 4.4 Pull requests — 93/93 affected

Every PR's `merge_commit_sha` and/or `headRefOid` is a rewritten SHA; all are inside the rewritten universe (0 outside). Affected numbers: **#1–#93** (all) — details in `analysis/affected-prs.json`. Consequence: each merged PR's commit links resolve only through GitHub's retained objects/PR refs, not through the pushed branches. In the rehearsal the 93 `refs/pull/*/head` refs were rewritten locally as well; **a real push cannot update them** (GitHub-managed) — GitHub keeps them at the old SHAs, which is also the recovery asset (§7).

## 5. Signature consequence (explicit, quantified)

* Universe-wide there are **93 signed commits** — every one authored by the owner and committed by **GitHub's web-flow key** (`GitHub <noreply@github.com>`); 0 use any other signer. Of these, **15 are among the 46 trailer targets** (`8f0370e8 e8219d56 3476555a da9f83bf 7de59522 48f68be1 0b8b7c29 142dc5d6 30790d37 f0dd7443 dcdb6430 bd512dfd c756edba cfef087a a22313f5` — matches the orchestrator's independent census exactly) and **78 are non-targets** that nevertheless change SHA as descendants.
* **All 93 change SHA and all 93 lose their PGP signature**: filter-repo re-creates the commit objects without the (now-invalid) `gpgsig` header — measured `gpgsig 93 → 0`. The 17 unsigned targets stay unsigned. Net: the rewrite converts 93 verified commits into 93 unsigned ones; nothing else about authorship changes (570/570 author/committer/date preservation).
* GitHub's web-flow signature **cannot be reproduced** by any user-side rewrite (only GitHub can sign with that key); re-signing with an owner key is a different, optional operation and was not rehearsed.
* **Tags: 0 of 4 carry signatures** → no tag signature loss.

### 5.1 Human-author preservation report (measured, all 46 targets)

| group | count | author | committer |
|---|---|---|---|
| `traeagent` direct pushes | 11 | `lostsky1403 <lostsky1403@users.noreply.github.com>` | same |
| Codebuff squash merges (`#40`–`#50`) | 11 | `Mochammad Fikri Chaerul Chalik Ramdhan <114224549+lostsky1403@users.noreply.github.com>` | `GitHub <noreply@github.com>` |
| `internal-model` squash merges (`#67`–`#69`) | 3 | same as above | `GitHub <noreply@github.com>` |
| `t <t@t>` squash merge (`#87`) | 1 | same as above | `GitHub <noreply@github.com>` |
| `Claude Opus 4.7` (arrived via merge PR `#1`) | 6 | `lostsky <moch22188ti@student.nurulfikri.ac.id>` | same |
| pull-ref-only pre-squash tips (`#40`–`#50`, `#67`–`#69`) | 14 | `lostsky1403 <moch22188ti@student.nurulfikri.ac.id>` | same |

* **0** targets — and **0** commits in the whole 570-commit universe — have an author or committer outside the owner's three identity forms and GitHub web-flow: no AI identity is ever an author or committer, before or after the rewrite.
* After the rewrite, author/committer name, e-mail and both dates are **byte-identical for 570/570 pairs** (`analysis/verification.json`); per-commit record: `analysis/target-meta.txt`, `migration/signed-commits-map.tsv`.

## 6. Branch reconciliation (A6)

Two migration paths exist; the rehearsal proves both.

**Path A — rewrite locally, then push (recommended).** Re-running the same rule on a fresh local mirror rewrites every local ref in place (this is what the rehearsal did): unpublished branches need **no rebase at all** — their commits already have new SHAs with identical trees/metadata. Then: force-push the 23 branches + 3 tags, push the unpublished branches normally.

**Path B — rebase after a remote-only rewrite.** Exact commands (old base = the branch's merge base, new base = its mapped counterpart), dry-run in `build-check` (old + new objects in one clone, scratch branches):

| branch | exact command | range | dry-run |
|---|---|---|---|
| `feat/sigap-selfhosted-auth` | `git rebase --rebase-merges --onto dbadcd3d0da4709804e11792c3e0cc7530ffd23e b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 feat/sigap-selfhosted-auth` | 104 commits, 1 merge | **CLEAN** — tip `8d3bff9`; tree `e624807d…` == old tip tree; control run on the unchanged base: clean |
| `feat/sigap-regional-pilot` | `git rebase --rebase-merges --onto dbadcd3d0da4709804e11792c3e0cc7530ffd23e b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 feat/sigap-regional-pilot` | 72 commits, 1 merge | **CLEAN** — tip `a629f91`; tree `4670012…` preserved |
| `feat/sigap-v2-implementation` | `git rebase --rebase-merges --onto dbadcd3d0da4709804e11792c3e0cc7530ffd23e b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 feat/sigap-v2-implementation` | 49 commits, 1 merge | **CLEAN** — tip `61e6218`; tree `61e62180…` preserved |
| `design/sigap-v2-prototype` | `git rebase --rebase-merges --onto 6649c78cd94d8c8dac5212277bfd2fd9892f91d6 704633cc7f317b9fc3cbe1189bd41f235f4d6d57 design/sigap-v2-prototype` | 18 commits, 0 merges | **CLEAN** — tip `e33e491`; tree `d3f0451e…` preserved |

Notes:
* `--rebase-merges` is required for an exact-shape replay: each feature branch contains **1 merge commit** in its unpublished range. A plain `git rebase` also applies cleanly (measured: selfhosted-auth tip `e113783`, regional-pilot `252d64a`, trees preserved) but **linearizes the merge** — use `--rebase-merges`.
* **Conflict risk: measured LOW** for all four -- zero conflicts, and the structural reason is the same as the mechanism: the new base carries the **byte-identical tree** of the old base, so every replayed hunk applies exactly as it originally did. (V1's rehearsal reported a conflict for a differently-based replay; with the correct base — the remote `main` merge base — V2 finds none.)
* **File-tree preservation proof** for Path B: after each dry-run, `git rev-parse <rebased>^{tree}` == `git rev-parse <old tip>^{tree}` — identical for all four (values above). For Path A the proof is the commit-map: all 570 pairs have identical trees, including every unpublished commit of every branch.
* `ci/*` and `design/ui-ux-overhaul`, `fix/*`, `ops/*`, `local/r2-drill`, `safety/*`, local `main`: **0 unpublished commits** → no rebase; re-point/re-push the mapped SHAs from `migration/branch-map.tsv`.
* Stash: rewritten in place (`7419094c` → `ecf2aeea…`). If a clone is migrated by rebase instead, re-apply the stash from the pre-rewrite bundle (`git stash apply 7419094c…` in a repo that has the old objects) — the stash's tree is preserved in the map either way.

## 7. Rollback / recovery

1. **Nothing was pushed**, so as of now rollback = deleting the temp directories. The real repository (`F:\sigap-design-v2`) and GitHub are untouched; every pre-rewrite SHA is still live there.
2. **Pre-rewrite backup (evidence + restore source):**
   * `migration/pre-rewrite-all.bundle` — 20,430,507 bytes, `git bundle --all` over all **145 refs** (old objects; includes the 15 squash commits that exist nowhere else).
   * `mirror-prerewrite-copy/` — byte copy of the pre-rewrite mirror (same objects).
   * `analysis/ref-manifest-prerewrite.txt` — `refname sha` for all 145 refs (the recovery map).
   * `migration/sha-map.tsv` — full old→new map (`filter-repo/commit-map` copy) — keep it; it is the only authoritative record of what became what.
3. **Restore procedure if a future push needs undoing:**
   ```bash
   git clone --mirror "%TEMP%\sigap-rw-v2\migration\pre-rewrite-all.bundle" sigap-restore.git
   git -C sigap-restore.git for-each-ref --format='%(refname) %(objectname)'   # == the 145-ref manifest
   git -C sigap-restore.git remote add origin https://github.com/lostsky1403/Sigap.git
   git -C sigap-restore.git push --force origin 'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*'
   ```
   (`refs/pull/*` must never be pushed — GitHub manages them; the rehearsal's local PR refs are incidental.)
4. **GitHub itself is a secondary backup:** the 93 server-side `refs/pull/*/head` refs keep the pre-squash tips reachable after a force-push (documented GitHub behaviour; `[INFERENCE]` for the push case — `ls-remote` confirms the refs exist today).
5. **Do not rely on in-repo refs as the backup:** filter-repo prunes replaced objects and creates no `refs/original/*`. `git pull`/`git fetch` on a stale clone would re-introduce the old objects and create a divergent history; migrate clones by re-pointing refs or re-cloning.
6. `commit-map` + `ref-map` + `changed-refs` are kept under `mirror-prerewrite/filter-repo/` and copied into `migration/`.

## 8. Determinism and cross-check with V1

The V1 rehearsal (52-ref local mirror, 484 commits) and this V2 rehearsal (145-ref GitHub mirror, 570 commits) rewrote different universes, but every SHA both runs produced is **identical**:

| commit/ref | V1 reported | V2 measured |
|---|---|---|
| `origin/main` `b96c5174` → | `dbadcd3d` | `dbadcd3d0da4709804e11792c3e0cc7530ffd23e` |
| `refs/heads/main` `87f218ef` → | `4d1669ee` | `4d1669ee…` |
| `feat/sigap-regional-pilot` `522fbc48` → | `feae4ca7` | `feae4ca7…` |
| `design/ui-ux-overhaul` `d7af334d` → | `c81003c0` | `c81003c0…` |
| target `a22313f5` → | `31fcf06` | `31fcf06…` |
| tag `demo-handoff-green-2026-07-10` obj → | `0a94a3f3` | `0a94a3f3…` |

The rewrite is a pure function of (commits, rules): the same inputs give the same SHAs, independent of ref scope, machine and run.

## 9. Limits — what this rehearsal does not prove, and checks not run

* **No push, no GitHub-side effect.** That the Contributors graph drops the three AI accounts after a force-push is a GitHub-side, cached recomputation (`/stats/contributors`, minutes to ~24 h) — asserted from the audit's model, not proven here.
* Branch protection settings are unknown → a force-push might be rejected; the rehearsal cannot test that.
* `refs/pull/*` on GitHub stay at old SHAs after a push (server-managed); the rehearsal's rewritten PR refs exist only inside the temp mirror.
* GitHub may hold objects no ref advertises; they are outside the rewritten universe.
* Not run: unit tests, `go test`, linters, CI workflows, any write API call. The only build executed was `go build ./...` in `apps/api` (exit 0 on both checked tips).
* Known harness limit: fetching the two long `refs/trae_merge/…/worktreeHash` refs into the non-bare build clone hit Windows' path-length limit; they **were** rewritten in the mirror (present in `ref-map`) — only the convenience fetch was skipped.
* The signature loss is a property of filter-repo re-creating objects; whether an owner-side re-signing step is acceptable is a decision outside this rehearsal.

## 10. Artefacts (all under `%TEMP%\sigap-rw-v2\`)

| Path | Content |
|---|---|
| `migration/sha-map.tsv` | all 570 old→new pairs (+ changed flag) |
| `migration/sha-map-changed.tsv` | the 524 changed pairs |
| `migration/sha-map-targets.tsv` | the 46 targets with identity, tail-LF, signature, subject |
| `migration/branch-map.tsv`, `tag-map.tsv`, `pullref-map.tsv` | ref-level maps (145 refs) |
| `migration/signed-commits-map.tsv` | the 93 signed commits, old→new, target flag, committer |
| `migration/pre-rewrite-all.bundle` | 20,430,507-byte bundle, all 145 refs (recovery) |
| `migration/message-callback.txt`, `apply.sh`, `verify.py`, `rebase-dryrun.sh` | exact rule code + reproducible commands |
| `analysis/ref-manifest-prerewrite.txt` | 145-ref frozen manifest |
| `analysis/census-pre.json`, `verification.json`, `per-ref.json`, `affected-prs.json`, `local-branch-info.json`, `rebase-summary.txt` + logs | raw evidence |
| `mirror-prerewrite/`, `mirror-prerewrite-copy/`, `fsck-clone/`, `build-check/` | the rewritten result, the pre-rewrite copy, the fsck clone, the rebase/build scratch clone |

## 11. Cleanup

Everything except the two design documents is disposable; when the owner is done:

```powershell
Remove-Item -Recurse -Force "$env:TEMP\sigap-rw-v2"    # after archiving the bundle if the rewrite is to be executed
```

Nothing in `F:\sigap-design-v2` — refs, objects, config, working tree — depends on these directories.
