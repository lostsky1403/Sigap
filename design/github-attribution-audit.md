# GitHub AI-Attribution Audit — `lostsky1403/Sigap`

**Date:** 2026-11-11 · **Repo:** `lostsky1403/Sigap` (public, default branch `main`, forks: 0, open issues/PRs: 0)
**Method:** read-only. GitHub REST (unauthenticated) + the Contributors page's own data endpoint + local `git` object inspection in `F:\sigap-design-v2` (branch `feat/sigap-selfhosted-auth` @ `60de7d6`, `origin/main` @ `b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3`).
**No rewrite was performed. § 8 is an impact assessment only.**

---

## 0. Answer up front

| Question | Answer |
|---|---|
| **Q1 — Is the AI attribution only in commit messages?** | **No.** It is in commit messages **and** in the Contributors **graph**. 3 AI identities (`claude`, `codebuff-team`, `traeagent`) are first-class rows in the Insights → Contributors page/data. Only the repo side-panel/REST `/contributors` list is clean. |
| **Q2 — Which trailer identities, and are they real accounts?** | 32 trailers, 5 values. **3 resolve to real GitHub accounts and are credited** (traeagent, codebuff-team, claude → 28 commits). **2 do not** (`internal-model` ×3, `t <t@t>` ×1 → 4 commits are attribution-inert). In all 32, the **author is a human identity of `lostsky1403`**; none is authored or committed by an AI identity. |
| **Q3 — Ancestors of public `main`?** | **All 32 of 32.** Zero trailer commits exist only on local-only branches. |
| **Q4 — Committer identity?** | 15 = `GitHub <noreply@github.com>` (web-flow squash merges); 11 = `lostsky1403@users.noreply.github.com` (direct local pushes, no PR); 6 = `moch22188ti@student.nurulfikri.ac.id` (local, merged via PR #1). |
| **Q5 — Signatures?** | 15 verified (GitHub web-flow signed), 17 unsigned. |
| **Q6 — PR linkage?** | 15 at commits carry `(#NN)` (squash-merged PR #40–#50, #67–#69, #87); 6 came in through merge PR #1 (`3807050`); 11 have **no** PR (`/pulls` → `[]`). |
| **Q7 — Rewrite impact** | Stripping the trailers rewrites **296 of 342** commits on `main` (46 keep their SHA), touches **45/45** branch refs, **3 of 4** tags, **16** PRs, and silently un-verifies 15 signed commits. Force-push required. |

**One-line root cause:** the repo's commit messages carry `Co-authored-by:` trailers whose e-mail addresses resolve to real GitHub accounts — `traeagent@users.noreply.github.com` → `traeagent`, `noreply@codebuff.com` → `codebuff-team`, `noreply@anthropic.com` → `claude` — and GitHub credits a resolvable co-author in the contributor graph **in addition to** the commit author, so the graph shows 4 contributors for a repository whose every commit is authored by `lostsky1403`.

---

## 1. Q1 — Contributors list vs Contributors graph: the raw evidence

### 1.1 REST `GET /repos/lostsky1403/Sigap/contributors?per_page=100` → **1 entry**

```json
[
  {
    "login": "lostsky1403",
    "id": 114224549,
    "node_id": "U_kgDOBs7tpQ",
    "url": "https://api.github.com/users/lostsky1403",
    "html_url": "https://github.com/lostsky1403",
    "type": "User",
    "site_admin": false,
    "contributions": 342
  }
]
```

### 1.2 REST `GET /contributors?anon=1&per_page=100` → **identical, still 1 entry**

The `anon=1` flag adds unlinked-e-mail contributors to the response. It added nothing, which independently shows the trailer identities are **not** unlinked authors — there are none: every one of the 342 commits is authored by an e-mail linked to `lostsky1403` (§ 1.7).

### 1.3 The web page HTML (`/graphs/contributors`) does **not** contain the list

`GET https://github.com/lostsky1403/Sigap/graphs/contributors` is a client-rendered React app. The server-rendered HTML body contains only the shell — no contributor logins, no avatars. Its embedded bootstrap JSON is:

```json
{"payload":{"repoUrl":"https://github.com/lostsky1403/Sigap","defaultBranch":"main",
 "graphDataPath":"/lostsky1403/Sigap/graphs/contributors-data","isUsingContributionInsights":false},
 "title":"Contributors to lostsky1403/Sigap","appPayload":null}
```

So "what does the page list?" cannot be answered from the HTML: it must be answered from the endpoint the page calls, or from the rendered DOM.

### 1.4 The rendered page → **3 rows, and it is not only `lostsky1403`**

Rendered with a headless browser at `https://github.com/lostsky1403/Sigap/graphs/contributors?from=8/8/2026` (default period *Last 3 months*, subtitle *"Contributions per week to `main`, excluding merge commits"*), the contributor list resolves to:

| Row | Commits shown |
|---|---|
| `lostsky1403` | 229 |
| `traeagent` | 11 |
| `codebuff-team` | 11 |

`claude` is absent **only because of the default time window** — its 6 commits are dated 2026-06-17, outside "Last 3 months". § 1.5 shows it is in the same data.

### 1.5 The page's own data endpoint → **4 entries** (raw)

`GET https://github.com/lostsky1403/Sigap/graphs/contributors-data` (the exact `graphDataPath` from § 1.3) returns, in this order, after `weeks[]` arrays are elided:

```json
[
  {"author":{"id":81847,"login":"claude","avatar":"https://avatars.githubusercontent.com/u/81847?s=60&v=4",
             "path":"/claude","hovercard_url":"/users/claude/hovercard"},"total":6},
  {"author":{"id":230265278,"login":"codebuff-team","avatar":"...u/230265278?s=60&v=4",
             "path":"/codebuff-team","hovercard_url":"/users/codebuff-team/hovercard"},"total":11},
  {"author":{"id":272135137,"login":"traeagent","avatar":"...u/272135137?s=60&v=4",
             "path":"/traeagent","hovercard_url":"/users/traeagent/hovercard"},"total":11},
  {"author":{"id":114224549,"login":"lostsky1403","avatar":"...u/114224549?s=60&v=4",
             "path":"/lostsky1403","hovercard_url":"/users/lostsky1403/hovercard"},"total":334}
]
```

Weekly commit counts (`c`) confirm the mapping to the trailers, committing the attribution to specific weeks:

* `claude` → `c: 6` in week `1781395200` (2026-06-14 … 06-20) = the 6 `Claude Opus 4.7` commits of 2026-06-17.
* `codebuff-team` → `c: 11` in week `1787443200` (2026-08-23 … 08-29) = the 11 `Codebuff` commits of 2026-08-25.
* `traeagent` → `c: 4` in week `1786233600` + `c: 7` in week `1786838400` = the 11 `traeagent` commits of 2026-08-11 … 08-22.

### 1.6 REST `GET /repos/lostsky1403/Sigap/stats/contributors` → the same **4 entries**

This is the server-side endpoint behind the Contributors graph, and it agrees with § 1.5 exactly:

| login | id | total |
|---|---|---|
| `claude` | 81847 | 6 |
| `codebuff-team` | 230265278 | 11 |
| `traeagent` | 272135137 | 11 |
| `lostsky1403` | 114224549 | 334 |

Sum = 362.

> **This is the finding that matters.** The "Contributors" list that a visitor sees under Insights is **not** restricted to `lostsky1403`. Two entries are visible in the default view; a third appears as soon as the period selector is widened.

### 1.7 Why `334` (graph) and `342` (REST) differ — the counting model, verified

| Step | Value | Evidence |
|---|---|---|
| Commits reachable from `origin/main` | **342** | `git rev-list --count origin/main` |
| Author e-mails on `main` | 239 × `moch22188ti@student.nurulfikri.ac.id`, 92 × `114224549+lostsky1403@users.noreply.github.com`, 11 × `lostsky1403@users.noreply.github.com` = **342** | `git log origin/main --format='%ae' \| sort \| uniq -c` |
| Distinct authors / committers | 3 author e-mails, 3 committer e-mails — **no AI identity is ever an author or committer** | same |
| Merge commits | 7 → non-merge = **335** | `git rev-list --count --merges origin/main` |
| Graph total for `lostsky1403` | **334** = 335 − 1 | `stats/contributors` |
| Graph total, all four identities | **362** = 334 + 28 | § 1.6 |
| Co-author credits | **28** = 32 trailers − 4 unresolvable | § 2 |

The model that reproduces every number:

1. the graph counts **non-merge commits on the default branch** (the page says so verbatim: *"excluding merge commits"*), in a rolling window;
2. a commit with a resolvable `Co-authored-by:` is credited to the **author *and* the co-author** — which is why 362 > 342 and why `lostsky1403`'s figure equals the *entire* non-merge count, co-authored commits included;
3. commits whose trailer does not resolve to an account contribute nothing extra (`internal-model`, `t <t@t>`).

**Independent cross-check of (1) and (2):** for the page's default 3-month window, `git rev-list --count --no-merges --since=2026-08-08 origin/main` = **229**, which is *exactly* the number the rendered page prints for `lostsky1403` (§ 1.4) — while `traeagent` (11) and `codebuff-team` (11) are counted *on top*, over the same 229 commits.

**The single remaining commit.** 335 non-merge commits exist in the window but `lostsky1403` is credited 334. Exactly one commit is unaccounted for. Its identity cannot be read off the API, but the residue appears *only in windows that contain the repository's first commit*: in the 3-month window (which starts after it) the match is exact, 229 = 229. The unique structural candidate is the root/initial commit `f390c1081cf6fcb3b7db672c5647f750f6657da4` ("chore: bootstrap Sigap monorepo", 2026-06-12, author `lostsky <moch22188ti@student.nurulfikri.ac.id>`, no parents). \[INFERENCE — GitHub's exclusion rule for that one commit is not publicly documented; the arithmetic (335 − 1 = 334) and the window-dependence are observed.\]
`f390c10` is **not** affected by any rewrite anyway (§ 8.1).

### 1.8 Q1 conclusion

* **Contributors side panel / REST `/contributors`:** `lostsky1403` only (342). **Not** affected.
* **Insights → Contributors graph (page + `/stats/contributors` + `/graphs/contributors-data`):** **affected** — 4 entries. Visible by default: `lostsky1403` (229), `traeagent` (11), `codebuff-team` (11); `claude` (6) appears when the period is widened past the last 3 months.
* The attribution lives in **commit metadata** (`Co-authored-by:` trailer e-mail → GitHub account), so it is visible in the graph, in the commit pages' co-author avatars, and in the commit messages themselves.

---

## 2. Q2 — Trailer identities: are they real GitHub accounts?

| Trailer value (verbatim) | Count | Resolves to a GitHub account? | Account evidence | Credited in graph? |
|---|---|---|---|---|
| `traeagent <traeagent@users.noreply.github.com>` | 11 | **Yes** — `traeagent`, id 272135137 | `created_at 2026-03-30T07:11:08Z`, `public_repos 0`, `followers 0`, `following 0`, no name/bio/blog → a tool account | **Yes**, 11 |
| `Codebuff <noreply@codebuff.com>` | 11 | **Yes** — `codebuff-team`, id 230265278 | `type User`, `name "Codebuff"`, `company "@CodebuffAI"`, `followers 453`, `created_at 2025-09-04`, `public_repos 0` | **Yes**, 11 |
| `Claude Opus 4.7 <noreply@anthropic.com>` | 6 | **Yes** — `claude`, id 81847 | `name "Claude"`, `company "@anthropics"`, `blog "https://anthropic.com/claude-code"`, `followers 230450`, `created_at 2009-05-07` | **Yes**, 6 |
| `internal-model` (no e-mail, no display name) | 3 | **No** — no e-mail, so nothing to resolve | — | No |
| `t <t@t>` | 1 | **No** — `t@t` is a placeholder domain that maps to no account | — | No |

Notes:

* `traeagent@users.noreply.github.com` is the canonical `login@users.noreply.github.com` form, so it resolves by login regardless of the account's e-mail settings.
* The e-mail `noreply@codebuff.com` resolves to **`codebuff-team` (230265278)**, **not** to the pre-existing, unrelated account `codebuff` (id 5947267, a human, "Deepankar Tyagi", @google, 10 repos, 60 followers, created 2013-11-15). The commit subject that pairs with these trailers also says `🤖 Generated with Codebuff`, and the account's `name` is literally "Codebuff" with `company "@CodebuffAI"` — so the attribution is the Codebuff product/team account, not that person.
* **Linkage is therefore per-trailer, not per-commit:** 28 of the 32 affected commits are linked to an AI account; 4 are inert. This is exactly why the graph total is 334 + 28 = 362.
* `internal-model` and `t <t@t>` are the only trailer contributions that GitHub cannot attribute to anybody; they are also the two that no code in this repo generates (see § 5).

### 2.1 Anyone can make this happen (why it is not "GitHub is broken")

GitHub resolves a `Co-authored-by:` trailer by looking the address up against accounts' verified e-mail addresses (plus the permanent `<login>@users.noreply.github.com` form). Any address that resolves adds a contributor row. Nothing in the trailer is authenticated: a commit author can name any account, which is precisely the situation here — and the reason the graph cannot be cleaned by configuration (§ 8.6).

---

## 3. Q2 continued — per-commit detail (all 32)

All 32 commits are ancestors of `origin/main`. Dates are author dates (short). `sig` is `git log --format=%G?` (`E` = signature present but unverifiable locally, i.e. GitHub's web-flow key; `N` = no signature).

### 3.1 `traeagent <traeagent@users.noreply.github.com>` — 11 commits

| SHA | Date | Subject | Author | Committer | sig | PR |
|---|---|---|---|---|---|---|
| `c179bdf` | 2026-08-11 | chore: pre-termination backup | `lostsky1403 <lostsky1403@users.noreply.github.com>` | `lostsky1403 <lostsky1403@users.noreply.github.com>` | N | **none** |
| `7119c69` | 2026-08-13 | chore: pre-termination backup | ″ | ″ | N | none |
| `cd89f10` | 2026-08-14 | chore: pre-termination backup | ″ | ″ | N | none |
| `dcfa16f` | 2026-08-15 | chore: pre-termination backup | ″ | ″ | N | none |
| `908dd21` | 2026-08-16 | chore: pre-termination backup | ″ | ″ | N | none |
| `5d762f8` | 2026-08-17 | chore: pre-termination backup | ″ | ″ | N | none |
| `29e8c73` | 2026-08-18 | chore: pre-termination backup | ″ | ″ | N | none |
| `d3ea784` | 2026-08-19 | chore: pre-termination backup | ″ | ″ | N | none |
| `8e57eee` | 2026-08-20 | chore: pre-termination backup | ″ | ″ | N | none |
| `776d0c2` | 2026-08-21 | chore: pre-termination backup | ″ | ″ | N | none |
| `030447b` | 2026-08-22 | chore: pre-termination backup | ″ | ″ | N | none |

`GET /repos/lostsky1403/Sigap/commits/030447b5af557f15b279533f83d74ce537dcb6af` → `"verification": {"verified": false, "reason": "unsigned"}`; `GET .../commits/030447b.../pulls` → `[]`.

### 3.2 `Codebuff <noreply@codebuff.com>` — 11 commits

| SHA | Date | Subject (PR) | Author | Committer | sig |
|---|---|---|---|---|---|
| `a22313f` | 2026-08-24/25 | fix: guard dev-only capabilities by environment (**#40**) | `Mochammad Fikri Chaerul Chalik Ramdhan <114224549+lostsky1403@users.noreply.github.com>` | `GitHub <noreply@github.com>` | E |
| `cfef087` | 2026-08-25 | fix: require SIGAP_AUTH_MODE in compose and add boot banner (#41) | ″ | ″ | E |
| `c756edb` | 2026-08-25 | fix: delete demo PHI handlers and remove SIGAP_ENABLE_DEMO_PHI (#42) | ″ | ″ | E |
| `bd512df` | 2026-08-25 | fix: require TLS termination confirmation outside local environment (#43) | ″ | ″ | E |
| `dcdb643` | 2026-08-25 | fix: restrict CORS loopback exception to loopback deployments (#44) | ″ | ″ | E |
| `f0dd744` | 2026-08-25 | fix: remove hardcoded `Access-Control-Allow-Origin: *` from SSE endpoint (#45) | ″ | ″ | E |
| `30790d3` | 2026-08-25 | feat: add security headers to API and web server (#46) | ″ | ″ | E |
| `142dc5d` | 2026-08-25 | fix: sanitize proxy headers for real client IP resolution (#47) | ″ | ″ | E |
| `0b8b7c2` | 2026-08-25 | feat: public catalog endpoints and centralized web proxy auth (#48) | ″ | ″ | E |
| `48f68be` | 2026-08-25 | fix: make /readyz check database connectivity and surface audit state (#49) | ″ | ″ | E |
| `7de5952` | 2026-08-25 | feat: harden HTTP server with timeouts, body limits, and graceful shutdown (#50) | ″ | ″ | E |

`GET .../commits/a22313f…` → `"verification": {"verified": true, "reason": "valid", "verified_at": "2026-08-24T19:04:51Z"}` with a PGP signature in the payload, and `committer.login = "web-flow"` (id 19864447).

### 3.3 `internal-model` (no e-mail) — 3 commits

| SHA | Date | Subject (PR) | Author | Committer | sig | Trailer verbatim |
|---|---|---|---|---|---|---|
| `da9f83b` | 2026-09-06 | ops: add PostgreSQL backup and restore workflow (**#67**) | `Mochammad Fikri Chaerul Chalik Ramdhan <114224549+lostsky1403@users.noreply.github.com>` | `GitHub <noreply@github.com>` | E | `Co-Authored-By: internal-model` |
| `3476555` | 2026-09-06 | docs: reconcile audit after postgres backup and restore (#68) | ″ | ″ | E | ″ |
| `e8219d5` | 2026-09-06 | docs: correct backup restore closure status (#69) | ″ | ″ | E | ″ |

### 3.4 `t <t@t>` — 1 commit

| SHA | Date | Subject (PR) | Author | Committer | sig | Trailer verbatim |
|---|---|---|---|---|---|---|
| `8f0370e` | 2026-10-13 | chore: remove the historical gitleaks allowlist (**#87**) | `Mochammad Fikri Chaerul Chalik Ramdhan <114224549+lostsky1403@users.noreply.github.com>` | `GitHub <noreply@github.com>` | E | `Co-authored-by: t <t@t>` |

### 3.5 `Claude Opus 4.7 <noreply@anthropic.com>` — 6 commits

| SHA | Date | Subject | Author | Committer | sig | PR |
|---|---|---|---|---|---|---|
| `ec3a80c` | 2026-06-17 | chore: add MIT LICENSE, SECURITY.md, and CODEOWNERS | `lostsky <moch22188ti@student.nurulfikri.ac.id>` | `lostsky <moch22188ti@student.nurulfikri.ac.id>` | N | merged via **#1** |
| `be7e159` | 2026-06-17 | fix(security): close open PHI routes behind a default-off gate | ″ | ″ | N | #1 |
| `8bf9120` | 2026-06-17 | feat(api): route registry + deny-by-default 401 middleware | ″ | ″ | N | #1 |
| `7cf4505` | 2026-06-17 | fix(security): remove hardcoded Postgres password, parameterize secrets | ″ | ″ | N | #1 |
| `a144eb8` | 2026-06-17 | fix(security): enforce Postgres TLS (sslmode=require) and run web as non-root | ″ | ″ | N | #1 |
| `e9b7237` | 2026-06-17 | ci: pin Go via go.mod, run Rust tests, add router coverage gate | ″ | ″ | N | #1 |

`GET .../commits/ec3a80c…` → `"verification": {"verified": false, "reason": "unsigned"}`, `committer.login = "lostsky1403"`.

**Author verdict (Q2, final):** in **32 / 32** commits the author is a human identity of `lostsky1403` (three e-mails, all linked to the one account) and the committer is either GitHub's web-flow or the same human identity. No AI identity is ever the author or committer; the AI identify appears **only inside the message body**.

---

## 4. Q3 — Which trailer commits are ancestors of the public `main`?

**All 32.** None is confined to a local feature branch.

```
git log --all      --format='%(trailers:key=Co-authored-by,valueonly)' | grep -c …   # 32
git log origin/main --format='%(trailers:key=Co-authored-by,valueonly)' | grep -c …   # 32
git log --all      … | awk -F'\t' '$2!=""{print $1}'  →  32 SHAs, every one ∈ git rev-list origin/main
# trailer commits NOT on origin/main: (empty)
```

`origin/main` = `b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3`, 342 commits. The local branch under test, `feat/sigap-selfhosted-auth` (`60de7d6`), is **84 commits ahead** of `origin/main` and has `origin/main` as an ancestor; local `main` = `87f218e…`.

> **Correction to a circulating figure.** An earlier summary recorded "31 of the trailer commits are ancestors of public `main`". The verified figure is **32 of 32**. The likely origin of "31" is the sum of the three resolvable identities plus `internal-model` (11 + 11 + 6 + 3 = 31), which omits the fourth and easily-missed trailer, `t <t@t>` on `8f0370e`. Both numbers are "affected commits" under different definitions: **32** commits carry a trailer; **28** of them are attributed to an account. Any count of "1 of the 32 is not on main" is not reproducible here.

---

## 5. Q4 — Committer identity vs the AI trailers

| Committer | Commits | Meaning |
|---|---|---|
| `GitHub <noreply@github.com>` (web-flow, id 19864447) | **15** | squash-merged through the GitHub web flow; each subject carries `(#NN)`. The AI trailer was added by the contributor **before** the merge; GitHub only re-committed the squashed result. |
| `lostsky1403 <lostsky1403@users.noreply.github.com>` | **11** | `traeagent` commits — authored **and** committed from a local git client, pushed directly to `main`. No PR, no web-flow. |
| `lostsky <moch22188ti@student.nurulfikri.ac.id>` | **6** | `Claude Opus 4.7` commits — local author/committer, delivered through merge PR #1. |

So the web-flow/GitHub signature and the AI attribution are **independent**: none of the 15 GitHub-committed commits was written by GitHub, and none of the 17 locally-committed commits avoids an AI trailer.

---

## 6. Q5 — Signed-commit status of the 32 affected commits

| Group | Commits | `git %G?` | GitHub API | Note |
|---|---|---|---|---|
| Web-flow squash merges (`#40`–`#50`, `#67`–`#69`, `#87`) | 15 | `E` (signature present, key not in local keyring) | sampled `a22313f`: `verified: true`, `reason: valid`, `verified_at 2026-08-24T19:04:51Z` | signed **by GitHub's web-flow key**, i.e. not by the contributor |
| `traeagent` direct pushes | 11 | `N` | sampled `030447b`: `verified: false`, `reason: unsigned` | unsigned |
| `Claude Opus 4.7` (PR #1 branch) | 6 | `N` | sampled `ec3a80c`: `verified: false`, `reason: unsigned` | unsigned |

Two of the three groups are unsigned and one is verifiable only because GitHub signed the squash. **The 15 signed commits are exactly the ones whose signature is lost if their messages are rewritten** (§ 8.4).

---

## 7. Q6 — PR linkage

| Trailer group | Linked PR | Evidence |
|---|---|---|
| `Codebuff` (11) | **PR #40, #41, #42, #43, #44, #45, #46, #47, #48, #49, #50** — squash | `(#NN)` in subject; committer = web-flow |
| `internal-model` (3) | **PR #67, #68, #69** — squash | same |
| `t <t@t>` (1) | **PR #87** — squash | same |
| `Claude Opus 4.7` (6) | **PR #1** — *merge* (not squash). `GET .../commits/ec3a80c…/pulls` → `number: 1`, `merged_at 2026-06-19T20:29:18Z`, `merge_commit_sha 3807050a1a5ad9578dcfa0aa2c5592faa83fe178`; all 6 are ancestors of `3807050` ("Merge pull request #1 from lostsky1403/feat/first-10-prs-security-foundation") | API `/pulls` + `git merge-base --is-ancestor` |
| `traeagent` (11) | **no PR at all** — `GET .../commits/030447b…/pulls` → `[]` | direct pushes to `main` |

Net: **16 PRs** (15 squash + #1) reference affected commits; 11 affected commits are unassociated with any PR.

---

## 8. Q7 — Rewrite impact assessment (**no rewrite performed**)

Premise: removing the trailers means editing the **commit message** of each of the 32 commits. A commit's SHA is a hash over its message, so each edited commit gets a new SHA, and **every descendant must be rewritten too** (same trees, new SHAs) — which on a shared `main` means a history rewrite plus force-push.

### 8.1 SHA churn on the public branch

The 32 affected commits are all descendants of `ec3a80ce19e0df630a378904e85406b87825f70a` (verified: `git merge-base --is-ancestor ec3a80c <Z>` → true for all 32), so the union of "this commit and all its descendants" collapses to that single commit's subtree:

| Quantity | Value | Command |
|---|---|---|
| Commits on `origin/main` | 342 | `git rev-list --count origin/main` |
| History of `ec3a80c` (self + ancestors) | 47 | `git rev-list --count ec3a80c` |
| Descendants of `ec3a80c` | 295 | `git rev-list --count --ancestry-path ec3a80c..origin/main` |
| **Commits that change SHA** | **296** (86.6 % of `main`) | 1 + 295 |
| **Commits that keep their SHA** | **46** | 47 − 1 — the ancestors of `ec3a80c`, ending in the root `f390c10` |

Only the pre-2026-06-17 bootstrap chain (root `f390c10` … `64367f2`) survives unchanged; the whole rest of `main` is rewritten. Blob/tree objects are untouched — only commit objects change — so the rewrite is content-preserving, but every SHA permalink below the 46 survivors becomes stale.

### 8.2 Branches and tags (refs that must move)

All **45** local + remote-tracking branch refs contain at least one affected commit, so every one needs rebasing/re-pushing. Affected-commit counts per ref (hist = `git rev-list --count R`; affected = 1 + `git rev-list --count R ^ec3a80c`):

| Ref | hist | affected | | Ref | hist | affected |
|---|---|---|---|---|---|---|
| `feat/sigap-selfhosted-auth` (current) | 426 | **380** | | `main` (local `87f218e`) | 180 | 134 |
| `feat/sigap-regional-pilot` | 414 | 368 | | `safety/pre-main-sync-3f0f768` | 269 | 223 |
| `feat/sigap-v2-implementation` | 391 | 345 | | `ci/sigap-88-validation` | 329 | 283 |
| `design/sigap-v2-prototype` | 359 | 313 | | `fix/request-id-propagation` | 155 | 109 |
| `ci/sigap-r2-activation` | 355 | 309 | | `origin/docs/audit-701-production-proof` | 166 | 120 |
| `local/r2-drill`, `ops/release-readiness-hardening`, `origin/design/ui-ux-overhaul` | 341 | 295 | | `v1.0.0-beta` (tag) | 26 | **0** |

Tags — 3 of 4 point inside the rewritten region and would be orphaned (their targets cease to be reachable from the rewritten `main`); re-pointing a tag is a delete + recreate (or forced update), i.e. itself a destructive ref operation:

| Tag | Commit | Affected | Impact |
|---|---|---|---|
| `demo-handoff-green-2026-07-10` | `0f45499` | 65 | orphaned → must be recreated |
| `frontend-demo-green-2026-07-10` | `7ec14da` | 64 | orphaned → must be recreated |
| `local-demo-green-2026-07-09` | `9339571` | 63 | orphaned → must be recreated |
| `v1.0.0-beta` | `13c59e7` (2026-06-12) | **0** | unaffected — predates the rewrite window, keeps its SHA, remains an ancestor of the rewritten `main` |

### 8.3 Pull requests

* **15 squash merges** (`#40`–`#50`, `#67`–`#69`, `#87`): the merged commit *is* the rewritten commit, so each PR's `merge_commit_sha` and its "merged commit" link resolve to a SHA no longer present in `main`. The PR stays closed and its repository keeps serving the old objects, but its commit list and diff base diverge from `main`.
* **PR #1**: its merge commit `3807050` is itself in the rewritten region (affected) → same orphaning; the PR's 6 `Claude Opus 4.7` commits are replaced by new-SHA equivalents.
* **11 `traeagent` commits**: no PR exists, so no PR linkage breaks — but they are also *not* protected by PR history, so their old SHAs vanish from `main` with no other record.
* GitHub also keeps `refs/pull/<NN>/head` refs; after the rewrite those point at commits that are no longer ancestors of `main`, which is precisely what makes a force-pushed PR look "merged from a commit that isn't in the branch".

### 8.4 Signatures

The 15 web-flow commits (§ 6) are signed by **GitHub's** key over `tree + parents + author + committer + message`. Their rewritten versions cannot carry that signature: GitHub signs only when it creates the commit, and the contributor cannot re-sign with GitHub's key. Rewriting therefore converts 15 verified commits into unsigned ones (the other 17 are already unsigned). CI configuration that requires verified commits (or `web_commit_signoff_required`-style policies — currently `false`) would start rejecting them.

### 8.5 In-repo and external references

* **Tracked-file SHA citations of affected commits** (they break; the cited SHAs no longer exist on `main`):
  * `docs/PRODUCTION_READINESS_AUDIT_RECONCILIATION.md:13` → `` `a22313f` `` (Codebuff, #40)
  * `docs/PRODUCTION_READINESS_AUDIT_RECONCILIATION.md:23` → `` `7de5952` `` (Codebuff, #50)
  * `docs/PRODUCTION_READINESS_AUDIT_RECONCILIATION.md:842, 844, 937` → `` `da9f83b` `` (internal-model, #67) — including "Baseline: `main @ da9f83b`", which is used as an audit anchor
  * Citations of *non-affected* commits (`cecc5ca`, `b47e07d`) in the same document survive.
* **Branch names in CI/docs**: `.github/workflows/ci.yml`, `ops-sandbox.yml`, `r2-offhost-backup.yml` and several `design/` + `docs/operations/` documents reference branches such as `feat/sigap-v2-implementation` and `design/ui-ux-overhaul`; re-pushing rewritten branches re-triggers those workflows and their concurrency/ref filters.
* **Forks/clones:** `forks_count: 0`, so no downstream clone must be reconciled; but every existing local clone (including the audit worktree's 45 branches) and every saved CI log, artifact and cached `git blame` link holding a post-`ec3a80c` SHA goes stale.
* **Open work:** `open_issues_count: 0`, so no in-flight PR targets the rewritten history.

### 8.6 What does **not** remove the attribution (non-destructive options are all ineffective)

The graph is derived from the **commits' metadata**, server-side; nothing repo-side or settable retro-fits it:

* `.mailmap` rewrites author names/e-mails for `git shortlog`, `git log` and similar **local** tooling. It cannot touch a `Co-authored-by:` trailer's account resolution (the trailer text is inside the commit message, which is hashed) and it is not a contributor-attribution input for GitHub.
* `.git-blame-ignore-revs` affects blame display only.
* Repository settings expose no contributor-attribution control — the graph data comes from `stats/contributors`, not from a repo file.
* Renaming or deleting a **branch** does not alter which commits `main` contains.
* Deleting the *accounts'* involvement is not available: dropping `traeagent`, `codebuff-team` or `claude` from the graph means their trailers must stop resolving, which is a change to commit messages.
* The only lever that exists without a rewrite is **presentation**: the page's default period is already *"Last 3 months"* (feature-flag-limited), so `claude` (June 2026) is hidden by default today and older entries age out of the default view over time — but the entries remain in `/stats/contributors` and reappear the moment a viewer selects a wider period. Concretely, with the default window starting 2026-08-08 as observed today (2026-11-11): the `claude` row has already left the default view, the `traeagent` rows leave it during 2026-11-11…11-22, and the `codebuff-team` row after 2026-11-25.

**Net:** removing the three graph entries requires rewriting the 32 trailer commits (new SHAs + force-push of `main`, 296 commits churn, 45 branches, 3 tags, 16 PRs, 15 signature losses). No non-destructive method — mailmap, settings, branch rename — changes the graph.

**Chronology of options, if the goal is only to stop future attribution:** changing the *authoring tooling* (i.e. not emitting `Co-authored-by:` for AI assistance, or emitting an ASCII-only note that resolves to no account) prevents new entries from 2026-11-11 onward without touching history; the 28 existing credits and the 32 existing trailer commits remain.

---

## 9. Reproduction commands

```bash
# --- GitHub side (unauthenticated REST) ---
curl -s 'https://api.github.com/repos/lostsky1403/Sigap/contributors?per_page=100'          # 1 entry, 342
curl -s 'https://api.github.com/repos/lostsky1403/Sigap/contributors?anon=1&per_page=100'   # still 1 entry
curl -s 'https://api.github.com/repos/lostsky1403/Sigap/stats/contributors'                 # 4 entries: 6/11/11/334
curl -s 'https://api.github.com/users/traeagent'                                            # tool account, id 272135137
curl -s 'https://api.github.com/users/codebuff-team'                                        # "Codebuff", id 230265278
curl -s 'https://api.github.com/users/claude'                                               # "Claude", id 81847
curl -s 'https://api.github.com/repos/lostsky1403/Sigap/commits/030447b5af557f15b279533f83d74ce537dcb6af/pulls'  # []

# The page's own data endpoint (needs the XHR header):
#   curl -s -H 'X-Requested-With: XMLHttpRequest' -H 'Accept: application/json' \
#        'https://github.com/lostsky1403/Sigap/graphs/contributors-data'

# Rendered page (client-side): open https://github.com/lostsky1403/Sigap/graphs/contributors

# --- local git (read-only) ---
git log --all       --format='%(trailers:key=Co-authored-by,valueonly)' | grep -v '^$' | sort | uniq -c
git log origin/main --format='%H%x09%an <%ae>%x09%cn <%ce>%x09%G?%x09%(trailers:key=Co-authored-by,valueonly)' \
  | grep -P '\t(traeagent|Codebuff|Claude Opus|internal-model|t <t@t>)'
git log origin/main --format='%ae' | sort | uniq -c            # 239 / 92 / 11 = 342
git rev-list --count origin/main                               # 342
git rev-list --count --no-merges origin/main                   # 335
git rev-list --count --no-merges --since=2026-08-08 origin/main # 229 == the page's number

# --- rewrite impact ---
E=ec3a80ce19e0df630a378904e85406b87825f70a
git rev-list --count $E                                                  # 47  = unchanged set + 1
git rev-list --count --ancestry-path $E..origin/main                     # 295 = descendants
# per ref: affected = 1 + $(git rev-list --count <ref> ^$E)  (0 if $E is not an ancestor)
for t in $(git tag); do echo "$t $(git rev-parse $t^{commit})"; done
```

---

## 10. Limits of this audit

* **Attribution mechanism.** This audit proves *which* accounts GitHub credits and that the credit counts match the trailer counts exactly (6/11/11 and 334 = 335−1). The precise server-side rule (noreply-login match vs verified e-mail match, and whether the credit is additive rather than a replacement) is GitHub's; the additive model above is the only one consistent with all observed numbers, and is labelled \[INFERENCE\] where it goes beyond them.
* **The 1-commit residue** in `lostsky1403`'s graph total (334 vs 335) is not attributable from public data; the root commit `f390c10` is identified by elimination plus the window-dependence of the discrepancy. \[INFERENCE\]
* **`internal-model` / `t <t@t>`**: no account, so their authors' intent cannot be read from GitHub. `internal-model` is a bare trailer with no e-mail (not RFC-822-shaped) and therefore can never resolve; `t <t@t>` uses a placeholder domain.
* **Verification sampling.** `%G?` was taken for all 32 commits; the GitHub `verification` object was fetched for 3 representative commits (one per group) rather than all 32.
* **No rewrite was executed**, and no `git fetch`/`push`/`gc` was run against the remote; the remote state used is the locally-cached `origin/main` @ `b96c517` plus live API responses.
