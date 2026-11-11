# GitHub historical-attribution correction — REMOTE EXECUTION MANIFEST

**Status: AWAITING ONE OWNER AUTHORIZATION.** Nothing has been pushed. No remote ref has been touched.
**Prepared from:** the verified rehearsal (`design/github-rewrite-rehearsal-v2.md`, 145 refs / 570 commits, nothing pushed) + the recovery baseline (`design/github-recovery-baseline.md`) + the source map (`design/github-attribution-source-map.md`).

This is authorization for the **precise remote Git transaction below** — not a request to reconsider the already-decided cleanup objective.

---

## 1. What is being corrected (the verified root cause)

Three GitHub accounts appear in the repository's **homepage Contributors panel, the Insights graph, `GET /stats/contributors` and the commit-page co-author chips**:
`codebuff-team` (11), `traeagent` (11), `claude` (6) — because `Co-authored-by:` trailers whose e-mail addresses resolve to real GitHub accounts are credited **in addition to** the commit author.

`internal-model` (3) and `t <t@t>` (1) **never surfaced** (no `Name <email>` shape / a domain-less dummy address) but remain in the message text.

**No AI identity is a Git author or committer anywhere.** All 46 affected commits are authored by the owner (`lostsky1403`, in three name/e-mail forms of account 114224549) or committed by GitHub's own web-flow identity (`GitHub <noreply@github.com>`). The correction is therefore **message-only**: strip 46 trailer lines. Nothing else about any commit changes.

## 2. Exact remote repository

`https://github.com/lostsky1403/Sigap.git` — public, default `main`, 0 forks, 0 releases. Current remote `main` = `b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3`.
Remote inventory: **125 `ls-remote` lines = 120 refs** (23 branches + 93 `refs/pull/*` + 4 annotated tags) + 4 peeled lines.

## 3. Branches to replace (23 — force-with-lease)

| remote branch | expected old tip | new tip |
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

## 4. Tags to replace (3 of 4)

| tag | old tag object | new tag object | old target | new target |
|---|---|---|---|---|
| `demo-handoff-green-2026-07-10` | `62754ed8` | `0a94a3f3` | `0f454996` | `4c1dd9ba` |
| `frontend-demo-green-2026-07-10` | `001057de` | `090819c4` | `7ec14da1` | `9a0c3ce7` |
| `local-demo-green-2026-07-09` | `fc0df5c8` | `c147729f` | `93395719` | `e4ebf2d1` |
| `v1.0.0-beta` | `3380ec26` | *(unchanged)* | `13c59e7a` | *(unchanged)* |

## 5. What is NOT pushed (and why that is safe)

- **`refs/pull/*` (93):** GitHub-managed; a push cannot update them. They keep the OLD SHAs — which is also the recovery asset. The rehearsal rewrote a *local* copy only.
- **No branch is deleted. No repository is recreated. `main` is not reset to a single commit.**
- **93 merged PRs (#1–#93)** reference rewritten SHAs; their links resolve through GitHub's retained objects, not through the pushed branches. **0 open PRs** exist, so nothing is disrupted mid-review.

## 6. Quantified consequences (accept-or-not facts)

| Consequence | Measured |
|---|---|
| Commits whose SHA changes | **524 of 570** (46 keep their SHA — the bootstrap chain) |
| Commit **messages** altered | **46** (exactly the 46 trailer lines removed; 478 change SHA with byte-identical messages) |
| Trailer lines remaining | **46 → 0** |
| File trees changed | **0** — 570/570 byte-identical |
| Author / committer / date changed | **0** — 570/570 preserved |
| Commits dropped / merges lost | **0 dropped**, merges 11 → 11 |
| **PGP signatures lost** | **93 verified commits become unsigned** (all GitHub web-flow; 15 of them are targets; the web-flow signature is unreproducible by any user-side rewrite). Tags: 0 of 4 were signed |
| Affected PRs | **93** (all) |
| Affected tags | **3 of 4** move |

## 7. Backup + restoration verification

- Location (durable, outside both checkouts, user-profile ACL): **`%USERPROFILE%\sigap-attribution-backup\`**
  `sigap-mirror.git` (120/120 refs identical to live), `sigap-local-all.bundle` (19,961,112 B), `refs-manifest-2026-11-11.txt`, `SHA256SUMS.txt`, `evidence\` (52 captures).
- **Verified:** `sha256sum -c SHA256SUMS.txt` → 55/55 OK; mirror `git fsck --full` exit 0; **restore drill re-run at the durable path → 52/52 refs SHA-identical, fsck exit 0 — PASS**.
- Bundle sha256 `eff7eaf5a523df80cbd2f2db4081000eaa0dd156b3da1ddb0ab8218a478274d9`; mirror pack sha256 `fb46b1fb465c7ab3b6ade7a85974f644fb2379cc8bbc126da0363b25d3f5826e`.

## 8. Development work is preserved (confirmed)

**107 unpublished commits** exist across 4 local feature branches + a stash. In the rehearsal each was rebased onto the rewritten ancestry **with its file tree byte-preserved**:

| branch | old tip | new tip | unpublished | rebase |
|---|---|---|---|---|
| `feat/sigap-selfhosted-auth` | `a5423ec3` | `8c3064ba` | 104 | dry-run CLEAN, tree preserved |
| `feat/sigap-regional-pilot` | `522fbc48` | `feae4ca7` | 72 | dry-run CLEAN, tree preserved |
| `feat/sigap-v2-implementation` | `e1631894` | `122d56c0` | 49 | dry-run CLEAN, tree preserved |
| `design/sigap-v2-prototype` | `4e470a53` | `98feb053` | 18 | dry-run CLEAN, tree preserved |
| `refs/stash` | `7419094c` | `ecf2aeea…` | 2 | rewritten in place, still a valid stash |

`go build ./...` **PASS** on the rewritten `main` (`dbadcd3d`) and the rewritten feature tip (`8c3064ba`).

## 9. No human authorship is falsely reassigned (confirmed)

570/570 commits keep their original author, committer and dates. Every affected commit is the owner's; the only other committer identity present is GitHub's own web-flow (`GitHub <noreply@github.com>`), which is platform metadata, not a contributor. No external human collaborator exists in this repository's history, so nothing human is being removed or reassigned. Genuine human co-authors — should any appear in future — are preserved by the strip rule (it matches only the known AI/placeholder addresses).

## 10. Exact operations proposed (after authorization)

```
# 1. STOP: confirm each remote tip still equals the §3 "expected old tip"; abort on ANY difference.
git ls-remote origin refs/heads/... refs/tags/...

# 2. Push each branch individually with an explicit lease (no --mirror, no --force):
git push --force-with-lease=refs/heads/main:b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 origin <rewritten-main>:refs/heads/main
#    ... one such command per branch in §3, each pinned to its expected old tip.

# 3. Tags: delete-and-recreate only the 3 moving tags (annotated objects recreated by the rewrite):
git push origin :refs/tags/demo-handoff-green-2026-07-10   # etc.
git push origin <new-tag-object>:refs/tags/demo-handoff-green-2026-07-10

# 4. NEVER: git push --force --mirror ; NEVER touch refs/pull/* ; NEVER delete a branch.
```

If any lease fails, or any ref differs from §3, **stop and report** — do not force.

## 11. Rollback

1. The pre-rewrite objects are retained by GitHub (`refs/pull/*` + unreachable objects) **and** in the durable backup.
2. Restore: `git push --force-with-lease=<ref>:<new-sha> origin <old-sha>:refs/heads/<ref>` for each ref in §3, in the order given, using the backup mirror/bundle as the object source — the exact commands are in `github-rewrite-rehearsal-v2.md` §7.
3. Confirmed recovery assets: the mirror (120 refs), the bundle (52 refs, drill-verified), and the 93 GitHub-held `refs/pull/*`.

## 12. What the owner is authorizing

> **Yes — rewrite the commit messages of the 46 trailer commits on `lostsky1403/Sigap`, force-with-lease the 23 branches and 3 tags listed above, accepting that 93 verified commits become unsigned and that all 93 merged PRs will reference the old SHAs, with the durable backup in place.**

`REMOTE_HISTORY_CORRECTION` stays `PENDING` until that authorization is given. `GITHUB_CONTRIBUTORS_CLEAN` stays `NO/PENDING_CACHE` until GitHub recomputes (up to ~24 h; the panel, graph and `/stats/contributors` must be re-read afterwards).
