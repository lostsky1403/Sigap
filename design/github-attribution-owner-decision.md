# GitHub contributor attribution — owner decision package

**Repo:** `lostsky1403/Sigap` · **Default branch:** `main` @ `b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3`
**Audited:** read-only. No push, no merge, no rewrite, no settings change.

---

## 1. What is actually visible (verified)

Two different GitHub endpoints disagree, and the difference is the whole story:

| Endpoint | What it powers | Result |
|---|---|---|
| `GET /repos/lostsky1403/Sigap/contributors` | the repo side-panel "Contributors" count | **only `lostsky1403`** (342) |
| `GET /repos/lostsky1403/Sigap/stats/contributors` | the **Contributors graph** (`/graphs/contributors`) | **4 entries** |

`/stats/contributors` returns exactly:

| Account | id | Commits | Cause |
|---|---|---|---|
| `lostsky1403` | 114224549 | 334 | the human owner (author + committer) |
| `codebuff-team` | 230265278 | 11 | `Co-authored-by: Codebuff <noreply@codebuff.com>` trail |
| `traeagent` | 272135137 | 11 | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` trail |
| `claude` | 81847 | 6 | `Co-authored-by: Claude Opus 4.7 <noreply@anthropic.com>` trail |

`internal-model` (3 trailers) does **not** appear — that trailer has no matching GitHub account.

## 2. Root cause (one sentence)

The AI identities are visible because **`Co-authored-by:` trailers whose e-mail addresses map to real GitHub accounts** are attributed as contributor-graph entries; **32 trailer commits** (28 with a trailer that resolves to a real account, 4 inert) are ancestors of public `main`, and those 28 are the sole reason the three accounts appear.

No other mechanism is involved: commit **authors** are all `lostsky1403 <…@users.noreply.github.com>`; there is no collaborator/bot account, no PR-author attribution, and no `.mailmap`. The trailers come from the AI **tools** (Trae, Codebuff, Claude Code) that generated the commit messages — the local and global git identity are already correct (`lostsky1403`), and there is no commit template, hook, or agent-config rule that adds them.

## 3. Which methods can and cannot clean the display

| Method | Changes commit metadata? | Can remove a graph entry? | Verdict |
|---|---|---|---|
| Install a `commit-msg` hook + CI check + agent rule | no (future commits only) | **no** — the graph is computed from existing history | stops the bleeding, cleans nothing historic |
| `.mailmap` | no | **no** — GitHub does not consult `.mailmap`; it is a client-side `git shortlog` feature | useless here |
| Remove a collaborator / block an account | no | **no** | useless here |
| Rename a branch | no | **no** | useless here |
| Rewrite the 32 commit **messages** (strip only the trailer lines) | **yes** (new SHAs) | **yes** — after GitHub recomputes the stats | the only method that can |

## 4. Options

### OPTION 1 — NON-DESTRUCTIVE (forward-only) — *recommended default*
- Install: `commit-msg` strip hook (`core.hooksPath .githooks`), the CI attribution check on the PR commit range, and the root `AGENTS.md` rule.
- **Visible outcome:** no *new* AI contributor ever appears. The three existing graph entries **remain** (history is untouched).
- **Probability of removing historical entries:** ~0 %.
- **Risk:** none to history; the hook is local and reversible (`git config --unset core.hooksPath`).
- **Owner approval:** none required.

### OPTION 2 — TARGETED HISTORICAL CORRECTION (message rewrite)
- Rewrite **only the `Co-authored-by:` trailer lines** in the 32 affected commit messages. Author, committer, dates and file trees are preserved byte-for-byte.
- **Blast radius (MEASURED in `github-attribution-rewrite-rehearsal.md`, not estimated):** a message-only strip in a mirror clone rewrote **438 of 484 reachable commits** (46 keep their SHA); exactly **32 messages were altered** and the other 406 changed only because an ancestor did. On the public branch, `origin/main` @ `b96c5174` (342 commits) becomes `dbadcd3d` — **296 changed / 46 preserved**. Across the 52 recorded refs: **22/22 local branches and 3/4 tags move** (only `refs/tags/v1.0.0-beta` keeps its SHA) and **all 93 `refs/pull/*` PR refs reference changed SHAs**. Trees are byte-identical for **484/484** mapped pairs, with 0 author/committer/date mismatches. A **force-push of `main`** is required, and in-repo `refs/backup/*` are clobbered by filter-repo — the recovery manifest (mirror clone + bundle + 52-SHA refs list) must live **outside** the repository.
- **Visible outcome:** once GitHub recomputes the contributors stats, the three accounts should drop from the graph. **Not instantaneous** — GitHub caches contributor stats; expect a delay (minutes to ~24 h) and verify via `/stats/contributors`.
- **Risks:** published-history rewrite; every cloned/forked repo diverges; open PRs against old SHAs need retargeting; CI/deploy refs pinned to old SHAs break; signed commits lose their signature; a naive `git pull` can reintroduce the old objects.
- **Owner approval:** **REQUIRED**, explicitly, before any remote operation.

### OPTION 3 — PRESERVE HISTORY
- Keep the trailers; publish an accurate maintainer/human-responsibility note instead.
- **Visible outcome:** the three accounts remain; the repo documents that all commits are authored by `lostsky1403`.
- **Risk:** none. **Owner approval:** none.

## 5. What no option can promise

There is **no** method that guarantees *instantaneous* removal from the Contributors graph. Even the rewrite relies on GitHub's stats recomputation. Nothing here should be reported as "cleaned" until `/stats/contributors` is re-read and shows the change.

## 6. Recommendation

**Option 1 now** (it is safe, immediate, and stops all future leaks), and hold **Option 2** as a rehearsed, owner-approved, single-purpose rewrite (`filter-branch`/`filter-repo` message-only, on a disposable clone first — see `design/github-attribution-rewrite-rehearsal.md`). Option 3 is the fallback if the owner judges the rewrite risk higher than the display benefit.

**Owner approval required:** YES (for Option 2 only). **Remote history rewritten:** NO.
