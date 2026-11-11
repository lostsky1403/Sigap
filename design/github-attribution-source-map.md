# GitHub AI-attribution source map — `lostsky1403/Sigap`

**Date:** 2026-11-11 (live re-verification 13:35–13:52 +07:00) · **Method:** read-only. GitHub REST (unauthenticated), the real rendered pages in Chrome (DOM + the endpoints they call), and local git in the Sigap worktrees. Nothing was pushed, rewritten, configured, or hidden.

**Evidence:** every claim below maps to a saved artifact in `%TEMP%\sigap-attribution-backup\evidence\` (index in § 7).
**Related:** `design/github-attribution-audit.md` (rewrite-impact analysis), `design/github-attribution-owner-decision.md` (options), `design/github-attribution-rewrite-rehearsal.md` (measured message-only rewrite), `design/github-recovery-baseline.md` (backup + restore drill).

> **One correction to `github-attribution-audit.md` (§ 1.8 / § 0 Q1).** The audit said "the repo side-panel ... is clean" and attributed visibility only to the Insights graph. The live render on 2026-11-11 shows the **repository homepage side panel itself renders all four contributor identities** — `Contributors 4`: `lostsky1403`, `codebuff-team`, `traeagent`, `claude`. The audit inferred the panel from `GET /contributors` (which returns 1 entry); that endpoint is not what feeds this panel. The panel hydrates from the contributors **graph** data (all-time), so it includes `claude` even though those claude commits are from June. See § 1.1.

---

## 0. Answer up front — the real mechanism, per identity

| identity (trailer value) | GitHub account | where it IS visible | where it is NOT visible | mechanism | commits | exact trailer line |
|---|---|---|---|---|---|---|
| `Codebuff <noreply@codebuff.com>` | **`codebuff-team`** (id 230265278) | homepage side panel; Insights graph data (`/stats/contributors`, `/graphs/contributors-data`); commit pages (co-author chip) | `GET /contributors` list (1 entry) | `Co-authored-by:` trailer; the e-mail resolves to that account | 11 | `Co-authored-by: Codebuff <noreply@codebuff.com>` |
| `traeagent <traeagent@users.noreply.github.com>` | **`traeagent`** (id 272135137) | same three surfaces | same | `Co-authored-by:` trailer; canonical `login@users.noreply.github.com` form | 11 | `Co-authored-by: traeagent <traeagent@users.noreply.github.com>` |
| `Claude Opus 4.7 <noreply@anthropic.com>` | **`claude`** (id 81847) | same three surfaces (all-time; outside the chart's default 3-month window) | same | `Co-authored-by:` trailer; e-mail resolves to that account | 6 | `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` |
| `internal-model` | — (none) | **nowhere** — not in any list, graph or commit-page author set | every surface | not parseable as a person: no `Name <email>` at all → GitHub never creates a co-author entry | 3 | `Co-Authored-By: internal-model` |
| `t <t@t>` | — (none credited) | **nowhere** — `GET /users/t` exists (id 60341) but receives nothing; commit page renders `login: null` + generic gravatar | every surface | `t@t` is a dummy address (no domain) that resolves to no account; resolution is by e-mail address, not display name | 1 | `Co-authored-by: t <t@t>` |

**Would stripping only the trailer lines remove it?** `codebuff-team`, `traeagent`, `claude`: **yes — all three surfaces** the trailer created (panel, graph data, commit-page chip) are derived from the trailer e-mail; with 0 trailer lines left in history (measured in the rehearsal) and GitHub's stats recompute (documented), the entries have no remaining input. GitHub caches contributor data; the docs say up to ~24 h (§ 2). `internal-model` and `t <t@t>`: they never appeared on any surface, so stripping changes nothing visible (it only removes the raw text from commit messages).

---

## 1. Surface-by-surface evidence (what is really rendered / returned)

### 1.1 Repository homepage — sidebar "Contributors" panel → **VISIBLE: 4 identities** (live render, 2026-11-11)

Rendered text of the panel (verbatim, from the live DOM):

```text
Contributors
4
 (4)
lostsky1403Mochammad Fikri Chaerul Chalik Ramdhan
codebuff-teamCodebuff
traeagent
claudeClaude
```

The four entries in the rendered DOM (full verbatim capture saved as `evidence/sidebar-contributors-panel-2026-11-11.html`; reproduced in full):

```html
<div class="SidebarSection-module_TOhNLG_sidebarSection"><h2 class="SidebarSection-module_TOhNLG_sectionHeading prc-Heading-Heading-MtWFE" data-variant="small" data-component="Heading"><span class="SidebarSection-module_TOhNLG_headingLinkWrapper"><a class="SidebarSection-module_TOhNLG_headingLink prc-Link-Link-9ZwDx" data-component="Link" data-muted="true" href="/lostsky1403/Sigap/graphs/contributors"><span>Contributors</span></a><span aria-hidden="true" data-variant="secondary" data-component="CounterLabel" class="ml-1 prc-CounterLabel-CounterLabel-X-kRU">4</span><span class="prc-VisuallyHidden-VisuallyHidden-Q0qSB">&nbsp;(4)</span></span></h2><ul class="SidebarContributors-module_cGmjDG_avatarList"><li class="SidebarContributors-module_cGmjDG_avatarListItem"><a class="SidebarContributors-module_cGmjDG_avatarLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/lostsky1403/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/lostsky1403"><img data-component="Avatar" class="prc-Avatar-Avatar-0xaUi" alt="@lostsky1403" width="32" height="32" data-testid="github-avatar" src="https://avatars.githubusercontent.com/u/114224549?s=64&amp;v=4" style="--avatarSize-regular: 32px;"></a><span class="SidebarContributors-module_cGmjDG_contributorName prc-Truncate-Truncate-2G1eo" title="lostsky1403 Mochammad Fikri Chaerul Chalik Ramdhan" style="--truncate-max-width: none;"><a class="SidebarContributors-module_cGmjDG_nameLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/lostsky1403/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/lostsky1403"><strong>lostsky1403</strong><span class="SidebarContributors-module_cGmjDG_profileName">Mochammad Fikri Chaerul Chalik Ramdhan</span></a></span></li><li class="SidebarContributors-module_cGmjDG_avatarListItem"><a class="SidebarContributors-module_cGmjDG_avatarLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/codebuff-team/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/codebuff-team"><img data-component="Avatar" class="prc-Avatar-Avatar-0xaUi" alt="@codebuff-team" width="32" height="32" data-testid="github-avatar" src="https://avatars.githubusercontent.com/u/230265278?s=64&amp;v=4" style="--avatarSize-regular: 32px;"></a><span class="SidebarContributors-module_cGmjDG_contributorName prc-Truncate-Truncate-2G1eo" title="codebuff-team Codebuff" style="--truncate-max-width: none;"><a class="SidebarContributors-module_cGmjDG_nameLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/codebuff-team/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/codebuff-team"><strong>codebuff-team</strong><span class="SidebarContributors-module_cGmjDG_profileName">Codebuff</span></a></span></li><li class="SidebarContributors-module_cGmjDG_avatarListItem"><a class="SidebarContributors-module_cGmjDG_avatarLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/traeagent/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/traeagent"><img data-component="Avatar" class="prc-Avatar-Avatar-0xaUi" alt="@traeagent" width="32" height="32" data-testid="github-avatar" src="https://avatars.githubusercontent.com/u/272135137?s=64&amp;v=4" style="--avatarSize-regular: 32px;"></a><span class="SidebarContributors-module_cGmjDG_contributorName prc-Truncate-Truncate-2G1eo" title="traeagent" style="--truncate-max-width: none;"><a class="SidebarContributors-module_cGmjDG_nameLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/traeagent/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/traeagent"><strong>traeagent</strong></a></span></li><li class="SidebarContributors-module_cGmjDG_avatarListItem"><a class="SidebarContributors-module_cGmjDG_avatarLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/claude/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/claude"><img data-component="Avatar" class="prc-Avatar-Avatar-0xaUi" alt="@claude" width="32" height="32" data-testid="github-avatar" src="https://avatars.githubusercontent.com/u/81847?s=64&amp;v=4" style="--avatarSize-regular: 32px;"></a><span class="SidebarContributors-module_cGmjDG_contributorName prc-Truncate-Truncate-2G1eo" title="claude Claude" style="--truncate-max-width: none;"><a class="SidebarContributors-module_cGmjDG_nameLink prc-Link-Link-9ZwDx" data-component="Link" data-hovercard-url="/users/claude/hovercard" data-hovercard-type="user" octo-click="hovercard-link-click" octo-dimensions="link_type:self" aria-keyshortcuts="Alt+ArrowUp" href="/claude"><strong>claude</strong><span class="SidebarContributors-module_cGmjDG_profileName">Claude</span></a></span></li></ul></div>
```

Notes:
* The server-sent HTML (`homepage-live.html`, 364,710 B) contains only the section heading + skeleton (the section is `<div class="sidebarSection__oqw6Y8">` with `SkeletonText` placeholders); the contents hydrate client-side. Neither raw HTML file contains the AI names — the render does.
* `traeagent` has no profile name; the other three show `lostsky1403` / `Codebuff` / `Claude`.
* The badge count is **4**, and the heading links to `/lostsky1403/Sigap/graphs/contributors` — i.e. the panel is driven by the graph data, not by `GET /contributors`.

### 1.2 Insights → Contributors page (`/graphs/contributors`)

The page is a client-rendered SPA. Server HTML (saved as `contributors-page-live.html`, 133,083 B) contains its bootstrap payload:

```json
{"payload":{"repoUrl":"https://github.com/lostsky1403/Sigap","defaultBranch":"main","graphDataPath":"/lostsky1403/Sigap/graphs/contributors-data","isUsingContributionInsights":false}}
```

Rendered DOM scan on 2026-11-11 (both the default `Period: Last 3 months` and a widened `2026-06-01…2026-11-11` range): **no contributor names exist in the visible text/DOM of this page** — the only name-bearing text is the breadcrumb (`lostsky1403`). The chart is a single-series weekly bar chart: its aria-labels read `Commits, series 1 of 2. Bar series with 15 bars. Y axis, Contributions. X axis, Time.` and `Navigator 1, series 2 of 2 …`; period buttons read `Period: Last 3 months` / `Contributions: Commits`. The identity list lives in the page's **data**, which the page fetches from `graphDataPath`.

`GET https://github.com/lostsky1403/Sigap/graphs/contributors-data` (live, 2026-11-11; the page's own data endpoint) returns exactly these 4 entries (weeks trimmed to weeks with commits; full file `graphs-contributors-data-live.json`):

```json
[
 {
  "author": {
   "id": 81847,
   "login": "claude",
   "path": "/claude"
  },
  "total": 6,
  "weeks_nonzero_commits": [
   {
    "w": 1781395200,
    "c": 6
   }
  ]
 },
 {
  "author": {
   "id": 230265278,
   "login": "codebuff-team",
   "path": "/codebuff-team"
  },
  "total": 11,
  "weeks_nonzero_commits": [
   {
    "w": 1787443200,
    "c": 11
   }
  ]
 },
 {
  "author": {
   "id": 272135137,
   "login": "traeagent",
   "path": "/traeagent"
  },
  "total": 11,
  "weeks_nonzero_commits": [
   {
    "w": 1786233600,
    "c": 4
   },
   {
    "w": 1786838400,
    "c": 7
   }
  ]
 },
 {
  "author": {
   "id": 114224549,
   "login": "lostsky1403",
   "path": "/lostsky1403"
  },
  "total": 334,
  "weeks_nonzero_commits": [
   {
    "w": 1780790400,
    "c": 46
   },
   {
    "w": 1781395200,
    "c": 27
   },
   {
    "w": 1782000000,
    "c": 13
   },
   {
    "w": 1782604800,
    "c": 7
   },
   {
    "w": 1783209600,
    "c": 12
   },
   {
    "w": 1786233600,
    "c": 4
   },
   {
    "w": 1786838400,
    "c": 7
   },
   {
    "w": 1787443200,
    "c": 26
   },
   {
    "w": 1788048000,
    "c": 8
   },
   {
    "w": 1788652800,
    "c": 19
   },
   {
    "w": 1789257600,
    "c": 5
   },
   {
    "w": 1789862400,
    "c": 29
   },
   {
    "w": 1790467200,
    "c": 49
   },
   {
    "w": 1791072000,
    "c": 48
   },
   {
    "w": 1791676800,
    "c": 34
   }
  ]
 }
]
```

Weekly commit counts pin each identity to its commits: `claude` c=6 in week 1781395200; `codebuff-team` c=11 in week 1787443200; `traeagent` c=4 (1786233600) + c=7 (1786838400) — the same weeks as the trailer commits in § 3.

### 1.3 `GET /repos/lostsky1403/Sigap/contributors` → **1 entry only** (and `?anon=1` identical)

```json
[
 {
  "login": "lostsky1403",
  "id": 114224549,
  "type": "User",
  "site_admin": false,
  "contributions": 342
 }
]
```
`GET .../contributors?anon=1&per_page=100` returns the identical single entry (`api-contributors-anon.json`). GitHub's REST docs explain why this list is the wrong instrument here: *"GitHub identifies contributors by author email address. This endpoint groups contribution counts by GitHub user, which includes all associated email addresses."* — co-authored commits are not *authored* by these accounts, so they never enter this list. The endpoint description also states the data *"may return information that is a few hours old because the GitHub REST API caches contributor data."*

### 1.4 `GET /repos/lostsky1403/Sigap/stats/contributors` → **4 entries** (server-side data behind the graph)

```json
[
 {
  "author": {
   "login": "claude",
   "id": 81847
  },
  "total": 6,
  "weeks_nonzero_commits": [
   {
    "w": 1781395200,
    "c": 6
   }
  ]
 },
 {
  "author": {
   "login": "codebuff-team",
   "id": 230265278
  },
  "total": 11,
  "weeks_nonzero_commits": [
   {
    "w": 1787443200,
    "c": 11
   }
  ]
 },
 {
  "author": {
   "login": "traeagent",
   "id": 272135137
  },
  "total": 11,
  "weeks_nonzero_commits": [
   {
    "w": 1786233600,
    "c": 4
   },
   {
    "w": 1786838400,
    "c": 7
   }
  ]
 },
 {
  "author": {
   "login": "lostsky1403",
   "id": 114224549
  },
  "total": 334,
  "weeks_nonzero_commits": [
   {
    "w": 1780790400,
    "c": 46
   },
   {
    "w": 1781395200,
    "c": 27
   },
   {
    "w": 1782000000,
    "c": 13
   },
   {
    "w": 1782604800,
    "c": 7
   },
   {
    "w": 1783209600,
    "c": 12
   },
   {
    "w": 1786233600,
    "c": 4
   },
   {
    "w": 1786838400,
    "c": 7
   },
   {
    "w": 1787443200,
    "c": 26
   },
   {
    "w": 1788048000,
    "c": 8
   },
   {
    "w": 1788652800,
    "c": 19
   },
   {
    "w": 1789257600,
    "c": 5
   },
   {
    "w": 1789862400,
    "c": 29
   },
   {
    "w": 1790467200,
    "c": 49
   },
   {
    "w": 1791072000,
    "c": 48
   },
   {
    "w": 1791676800,
    "c": 34
   }
  ]
 }
]
```
REST docs say this endpoint *"Returns the total number of commits authored by the contributor"* — yet the four totals sum to 362, not to the 335 non-merge commits on `main`. The 28-commit surplus is exactly the 28 trailer-carrying commits whose trailer resolves to an account: the co-authored commit is credited to the author **and** to the co-author (§ 2.3).

### 1.5 Commit pages — the co-author chip (the third visible surface)

**`traeagent` resolves to a real account** — commit `c179bdf7485dbf013e6220c9738f69188728bf58` (`commit-traeagent.html`)

```json
"authors":[{"login":"lostsky1403","displayName":"lostsky1403","avatarUrl":"https://avatars.githubusercontent.com/u/114224549?v=4","path":"/lostsky1403","profileName":"Mochammad Fikri Chaerul Chalik Ramdhan","isGitHub":false},{"login":"traeagent","displayName":"traeagent","avatarUrl":"https://avatars.githubusercontent.com/u/272135137?v=4","path":"/traeagent","profileName":null,"isGitHub":false}]
```

**`codebuff-team` resolves** — commit `a22313f572a7450ee40f67c44e993d73115b5c03` (`commit-codebuff.html`)

```json
"authors":[{"login":"lostsky1403","displayName":"Mochammad Fikri Chaerul Chalik Ramdhan","avatarUrl":"https://avatars.githubusercontent.com/u/114224549?v=4","path":"/lostsky1403","profileName":"Mochammad Fikri Chaerul Chalik Ramdhan","isGitHub":false},{"login":"codebuff-team","displayName":"Codebuff","avatarUrl":"https://avatars.githubusercontent.com/u/230265278?v=4","path":"/codebuff-team","profileName":"Codebuff","isGitHub":false}]
```

**`claude` resolves** — commit `ec3a80ce19e0df630a378904e85406b87825f70a` (`commit-claude.html`)

```json
"authors":[{"login":"lostsky1403","displayName":"lostsky","avatarUrl":"https://avatars.githubusercontent.com/u/114224549?v=4","path":"/lostsky1403","profileName":"Mochammad Fikri Chaerul Chalik Ramdhan","isGitHub":false},{"login":"claude","displayName":"Claude Opus 4.7","avatarUrl":"https://avatars.githubusercontent.com/u/81847?v=4","path":"/claude","profileName":"Claude","isGitHub":false}]
```

**`t <t@t>` does NOT resolve — `"login":null`, placeholder gravatar, `"profileName":null`** — commit `8f0370e803c4afcc1131baa92c36396ceea2359a` (`commit-t.html`)

```json
"authors":[{"login":"lostsky1403","displayName":"Mochammad Fikri Chaerul Chalik Ramdhan","avatarUrl":"https://avatars.githubusercontent.com/u/114224549?v=4","path":"/lostsky1403","profileName":"Mochammad Fikri Chaerul Chalik Ramdhan","isGitHub":false},{"login":null,"displayName":"t","avatarUrl":"https://github.githubassets.com/images/gravatars/gravatar-user-420.png","profileName":null,"isGitHub":false}]
```

**`internal-model` is not a person at all — the array contains only the owner** — commit `da9f83bf64c9fb97a2f3d522fda07a47dd3b44d1` (`commit-internal-model.html`)

```json
"authors":[{"login":"lostsky1403","displayName":"Mochammad Fikri Chaerul Chalik Ramdhan","avatarUrl":"https://avatars.githubusercontent.com/u/114224549?v=4","path":"/lostsky1403","profileName":"Mochammad Fikri Chaerul Chalik Ramdhan","isGitHub":false}]
```

> The JSON above is the commit page's own embedded `"authors"` array (`commit/<sha>.html` captures in the evidence dir). Every captured commit page also renders the trailer text itself in the message body (`Co-authored-by: …`), which is why the raw string is visible even where it resolves to nothing.

### 1.6 Where the identity is NOT: negative evidence

* `GET /contributors` and `?anon=1`: 1 entry (owner) — § 1.3.
* Authors/committers on all 342 commits of `main`: 3 author e-mails and 3 committer e-mails, all human owner identities or GitHub web-flow; no AI identity is ever an author or committer (`origin-main-commits-live-2026-11-11.tsv`, 342 rows; verified on the 32 trailer commits via the API: `author.login` = `lostsky1403` ×32, `committer.login` = `web-flow` ×15 + `lostsky1403` ×17).
* PR authorship: all 93 PRs are authored by `lostsky1403` (`prs-inventory-2026-11-11.tsv`).
* Repo metadata: 0 forks, 0 releases, size 3,018 KB (`api-repo-live.json`). No bot/app account is involved.

---

## 2. Mechanism — current GitHub documentation + measurement

### 2.1 The trailer is the entire mechanism (docs)

GitHub Docs, *Creating a commit with multiple authors* (https://docs.github.com/en/pull-requests/how-tos/commit-changes/creating-a-commit-with-multiple-authors):

> Add one or more `Co-authored-by` trailers to a commit message to attribute a commit to multiple authors.
>
> Before adding a co-author, get the email address they want used in the trailer. **For the commit to count as a contribution, use an email address associated with their account on GitHub.com.**

That sentence is the causal link: the trailer e-mail selects the credited account. (`traeagent@users.noreply.github.com` is the canonical `USERNAME@users.noreply.github.com` form; `noreply@codebuff.com` is the address configured on `codebuff-team`; `noreply@anthropic.com` resolves to `claude`.)

GitHub Docs, *Troubleshooting missing contributions* (https://docs.github.com/en/account-and-profile/setting-up-and-managing-contribution-graphs-on-your-profile/why-are-my-contributions-not-showing-up-on-my-profile):

> Commits must be made with an email address that is connected to your account on GitHub, or the GitHub-provided `noreply` email address …, in order to appear on your contributions graph.
>
> **Generic email addresses, such as `jane@computer.local`, cannot be added to GitHub accounts and linked to commits.** If you've authored any commits using a generic email address, the commits will not be linked to your GitHub profile and will not show up in your contribution graph.

And on rewrites: *"Contribution counts are stale after rewriting history … GitHub Support may be able to rebuild your contribution data"* (same page) — the documented recompute path if a rewrite lands.

GitHub Docs, *Email addresses reference* (https://docs.github.com/en/account-and-profile/reference/email-addresses-reference): the `noreply` address is `<ID+USERNAME@users.noreply.github.com>` (accounts created after 2017-07-18) or `<USERNAME@users.noreply.github.com>` (older + private). A co-author trailer carrying one of those forms resolves by construction.

GitHub Docs, *Viewing a project's contributors* (https://docs.github.com/en/repositories/viewing-activity-and-data-for-your-repository/viewing-a-projects-contributors):

> You can view the top 100 contributors to a repository in the contributors graph. Merge commits and empty commits aren't counted as contributions for this graph.
> … If the email address you used to author the commits is not connected to your GitHub account, your commits won't be linked to your account, and you won't appear in the contributors graph.
> Contributor data is stale after history changes … can take about 24 hours to refresh.

### 2.2 The two REST endpoints measure different things (docs)

* `GET /repos/{owner}/{repo}/contributors` — *"GitHub identifies contributors by **author** email address … only the first 500 author email addresses in the repository link to GitHub users"* (REST docs, List repository contributors). Co-author-only credits are invisible to it → 1 entry.
* `GET /repos/{owner}/{repo}/stats/contributors` — *"Returns the total number of commits authored by the contributor"* (REST docs, Get all contributor commit activity). The measured totals include the trailer credits → 4 entries.

### 2.3 The measured counting model (reproduces every number)

| Quantity | Value | Evidence |
|---|---|---|
| Commits on `origin/main` (`b96c5174`) | 342 | `git rev-list --count origin/main` |
| Merge commits / non-merge | 7 / 335 | `git rev-list --count --merges origin/main` |
| `lostsky1403` total in `/stats/contributors` | 334 = 335 − 1 root commit | live JSON § 1.4 |
| Sum of all four totals | 362 = 334 + 28 | live JSON § 1.4 |
| Trailer-carrying commits | 32 = 28 resolving (11+11+6) + 4 inert (3+1) | § 3 |
| Co-author credits | 28 | arithmetic identity |

Model: the graph counts **non-merge commits on the default branch**; a commit whose `Co-authored-by:` resolves is credited to **author + co-author**; unresolvable trailers add nothing. The weekly `c` values in § 1.2 confirm the mapping commit-by-commit.

---

## 3. The exact commits and trailer lines (all 32)

32 commits carry a trailer line (line-anchored, case-insensitive `^[ ]*co-authored-by:`). All 32 are ancestors of public `main` @ `b96c5174`. Data: `origin-main-commits-live-2026-11-11.tsv` + `attribution-32-by-identity.json` + `api-commit-verification.json` (per-commit GitHub verification payloads).

### 3.1 `codebuff-team` — 11 commits — trailer line (verbatim per commit): `Co-authored-by: Codebuff <noreply@codebuff.com>`

| commit (full SHA) | date | subject | author e-mail | committer e-mail | git %G? | GitHub `verification.verified` |
|---|---|---|---|---|---|---|
| `7de59522f26a42f55dae36b7681d834feb58701e` | 2026-08-25 | feat: harden HTTP server with timeouts, body limits, and graceful shutdown (#50) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `48f68be1598649b13b69f284ca61408a78736022` | 2026-08-25 | fix: make /readyz check database connectivity and surface audit state (#49) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `0b8b7c294ab8e6b7198fad9ade6b98ecc3fcc2a7` | 2026-08-25 | feat: public catalog endpoints and centralized web proxy auth (#48) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `142dc5d64b9a6fb63e499b8fef815e46a7931020` | 2026-08-25 | fix: sanitize proxy headers for real client IP resolution (#47) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `30790d37b3531b11e4c6287df642e1dd075fd95b` | 2026-08-25 | feat: add security headers to API and web server (#46) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `f0dd74437d2a25dc13d725a2effe857a57bb5804` | 2026-08-25 | fix: remove hardcoded Access-Control-Allow-Origin: * from SSE endpoint (#45) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `dcdb6430c6fb88ab82d0faf39bde4f094661918d` | 2026-08-25 | fix: restrict CORS loopback exception to loopback deployments (#44) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `bd512dfd680af10725823c5fd0e0f7da09647b0f` | 2026-08-25 | fix: require TLS termination confirmation outside local environment (#43) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `c756edbada220db880d54ab4bc49d8711a56802c` | 2026-08-25 | fix: delete demo PHI handlers and remove SIGAP_ENABLE_DEMO_PHI (#42) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `cfef087a494b9661d275efd9dd71e047b69e4e1c` | 2026-08-25 | fix: require SIGAP_AUTH_MODE in compose and add boot banner (#41) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `a22313f572a7450ee40f67c44e993d73115b5c03` | 2026-08-25 | fix: guard dev-only capabilities by environment (#40) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |

### 3.2 `traeagent` — 11 commits — trailer: `Co-authored-by: traeagent <traeagent@users.noreply.github.com>`

| commit (full SHA) | date | subject | author e-mail | committer e-mail | git %G? | GitHub `verification.verified` |
|---|---|---|---|---|---|---|
| `c179bdf7485dbf013e6220c9738f69188728bf58` | 2026-08-11 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `7119c690af773d9e7970a16d3a3dec045da554a8` | 2026-08-13 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `cd89f10ab5c3e596f2371a99af6a249d80439aea` | 2026-08-14 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `dcfa16ff109e64a206d06321f35d3e3c74b89a97` | 2026-08-15 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `908dd21718fd2d125ece9fc3f4c74a03057bd10e` | 2026-08-16 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `5d762f89101087cbb2fca144712874a6cb267558` | 2026-08-17 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `29e8c73dda822f589e2bddec7747ca6e0e9ab0d0` | 2026-08-18 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `d3ea7847f51291291ce49105119095812956bfcf` | 2026-08-19 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `8e57eeea84990ac7facaffcccf896f041684ced1` | 2026-08-20 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `776d0c291c54b5b36fb8dac8402d9f8a6a01e7bf` | 2026-08-21 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |
| `030447b5af557f15b279533f83d74ce537dcb6af` | 2026-08-22 | chore: pre-termination backup | `lostsky1403@users.noreply.github.com` | `lostsky1403@users.noreply.github.com` | N | False |

### 3.3 `claude` — 6 commits — trailer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`

| commit (full SHA) | date | subject | author e-mail | committer e-mail | git %G? | GitHub `verification.verified` |
|---|---|---|---|---|---|---|
| `e9b72377d10d3f0e66a7e2c557823418641486ea` | 2026-06-17 | ci: pin Go via go.mod, run Rust tests, add router coverage gate | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |
| `a144eb8bb06e747c8db54ada699d2172b6256bb1` | 2026-06-17 | fix(security): enforce Postgres TLS (sslmode=require) and run web as non-root | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |
| `7cf4505c13c60cf943f700d0cf12e52d8d3c7519` | 2026-06-17 | fix(security): remove hardcoded Postgres password, parameterize secrets | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |
| `8bf9120de809def2ac9f7970d8e1c3c99db0e856` | 2026-06-17 | feat(api): route registry + deny-by-default 401 middleware | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |
| `be7e1592c86809306098204042c5615bc57cdbeb` | 2026-06-17 | fix(security): close open PHI routes behind a default-off gate | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |
| `ec3a80ce19e0df630a378904e85406b87825f70a` | 2026-06-17 | chore: add MIT LICENSE, SECURITY.md, and CODEOWNERS | `moch22188ti@student.nurulfikri.ac.id` | `moch22188ti@student.nurulfikri.ac.id` | N | False |

### 3.4 `internal-model` — 3 commits — trailer: `Co-Authored-By: internal-model`

| commit (full SHA) | date | subject | author e-mail | committer e-mail | git %G? | GitHub `verification.verified` |
|---|---|---|---|---|---|---|
| `e8219d56bf89f1c379530bd3fb00a41d6b686f6a` | 2026-09-06 | docs: correct backup restore closure status (#69) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `3476555abed65c73900bd53b7778c21a36c3a71b` | 2026-09-06 | docs: reconcile audit after postgres backup and restore (#68) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |
| `da9f83bf64c9fb97a2f3d522fda07a47dd3b44d1` | 2026-09-06 | ops: add PostgreSQL backup and restore workflow (#67) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |

### 3.5 `t <t@t>` — 1 commit — trailer: `Co-authored-by: t <t@t>`

| commit (full SHA) | date | subject | author e-mail | committer e-mail | git %G? | GitHub `verification.verified` |
|---|---|---|---|---|---|---|
| `8f0370e803c4afcc1131baa92c36396ceea2359a` | 2026-10-13 | chore: remove the historical gitleaks allowlist (#87) | `114224549+lostsky1403@users.noreply.github.com` | `noreply@github.com` | E | True |

Notes: `git %G?` `E` = a signature is present that the local keyring cannot verify (GitHub's web-flow key); `N` = unsigned. The `verified` column is GitHub's own `verification.verified` from `GET /repos/.../commits/{sha}`. The 15 `verified: true` commits are the web-flow squash-merges (#40–#50, #67–#69, #87); the 17 unsigned are the 11 direct-push `traeagent` commits and the 6 `claude` commits delivered via merge PR #1.

---

## 4. Verdict: does stripping ONLY the trailers make each identity disappear?

| identity | surfaces that show it today | input to that surface | after a message-only trailer strip (all trailer lines) |
|---|---|---|---|
| `codebuff-team` | side panel; graph data (`/stats/contributors`, `/graphs/contributors-data`); commit chip | 11 `Co-authored-by` e-mails | **Gone from all three** once GitHub recomputes; docs say contributor data can take ~24 h to refresh; `/stats/contributors` is the re-check instrument |
| `traeagent` | same | 11 trailers | same |
| `claude` | same (all-time surfaces) | 6 trailers | same |
| `internal-model` | never shown anywhere | no resolvable identity | **No visual change** (was never rendered); the message text is the only carrier |
| `t <t@t>` | never shown as an account | `t@t` resolves to nothing | **No visual change**; note the collision trap below |

Why nothing else keeps them alive: (a) no AI identity is an author or committer on any commit; (b) all PRs are authored by `lostsky1403`; (c) the repository has no forks, no releases, no bot/app account; (d) the rehearsal already proved a strip leaves **0** trailer lines in the rewritten history, with trees/author/dates byte-identical (see `github-attribution-rewrite-rehearsal.md`).

---

## 5. `t <t@t>` — resolved classification (requested)

Evidence, in order of strength:

1. **The trailer does not resolve.** The commit page for `8f0370e803c4afcc1131baa92c36396ceea2359a` (#87) renders the co-author as `{"login":null,"displayName":"t","avatarUrl":"https://github.githubassets.com/images/gravatars/gravatar-user-420.png","profileName":null,"isGitHub":false}` — a generic placeholder gravatar and no account link, in contrast to the resolved entries in § 1.5.
2. **No surface credits it.** `/stats/contributors` and `/graphs/contributors-data` contain no `t`; the side panel lists 4 identities, none of them `t`.
3. **Name collision, not credit.** A GitHub user literally named `t` exists (`id 60341`, `GET /users/t` → 200, saved as `api-user-t.json`), yet gains nothing — GitHub resolves the trailer by **e-mail address**, and `t@t` (no domain) cannot be an address associated with an account. The docs' line applies verbatim: generic addresses *"cannot be added to GitHub accounts and linked to commits"*.
4. Author and committer of that commit are the owner and GitHub web-flow respectively — the trailer was injected by tooling into the squash message, in the middle of the AI-trailer era (PR #87 of #31–#93).

**Classification:** placeholder metadata — same class as `internal-model`: erroneous, automatically-injected, never a human co-author and never a graph entry. Recommendation (matches the owner-decision doc): strip it with the AI trailer lines so the history carries no dead attribution text.

---

## 6. Repro commands (read-only)

```bash
# live surfaces
curl -s https://api.github.com/repos/lostsky1403/Sigap/contributors?per_page=100        # 1 entry
curl -s https://api.github.com/repos/lostsky1403/Sigap/contributors?anon=1&per_page=100   # 1 entry
curl -s https://api.github.com/repos/lostsky1403/Sigap/stats/contributors                 # 4 entries
curl -s https://github.com/lostsky1403/Sigap/graphs/contributors-data                       # 4 entries (page payload)
# trailer census on the frozen tip
git -C F:/sigap-design-v2 log origin/main --format='%H %ad %s' --date=short --grep='' \
  | head   # full variant in evidence/origin-main-commits-live-2026-11-11.tsv
# per-commit verification
curl -s https://api.github.com/repos/lostsky1403/Sigap/commits/<sha>   # .commit.verification.verified
```

---

## 7. Evidence index (`%TEMP%\sigap-attribution-backup\evidence\`)

| file | size |
|---|---|
| `api-commit-030447b-live.json` | 4127 B |
| `api-commit-a22313f-live.json` | 22655 B |
| `api-commit-verification.json` | 38651 B |
| `api-contributors-anon.json` | 985 B |
| `api-contributors-live.json` | 985 B |
| `api-contributors.json` | 985 B |
| `api-pulls-all.json` | 1569366 B |
| `api-pulls-open-live.json` | 2 B |
| `api-pulls-stateall-live.json` | 1569420 B |
| `api-ratelimit-live.json` | 427 B |
| `api-releases-live.json` | 2 B |
| `api-repo-live.json` | 5115 B |
| `api-repo.json` | 5857 B |
| `api-stats-contributors-live.json` | 7230 B |
| `api-stats-contributors.json` | 7230 B |
| `api-tags-live.json` | 1955 B |
| `api-tags.json` | 1957 B |
| `api-user-claude.json` | 1201 B |
| `api-user-codebuff-team.json` | 1273 B |
| `api-user-internal-model.json` | 89 B |
| `api-user-t.json` | 1094 B |
| `api-user-traeagent.json` | 1190 B |
| `attribution-32-by-identity.json` | 10254 B |
| `bundle-heads-2026-11-11.txt` | 4588 B |
| `commit-claude.html` | 439387 B |
| `commit-codebuff.html` | 459647 B |
| `commit-internal-model.html` | 951594 B |
| `commit-t.html` | 189639 B |
| `commit-traeagent.html` | 188096 B |
| `commits-all.json` | 810775 B |
| `contributors-graph.html` | 133082 B |
| `contributors-page-live.html` | 133083 B |
| `drill-bare-refs.txt` | 4482 B |
| `email-addresses-reference.md` | 2797 B |
| `graphs-contributors-data-live.json` | 4127 B |
| `homepage-live.html` | 364710 B |
| `homepage.html` | 364706 B |
| `local-branches-2026-11-11.tsv` | 2222 B |
| `mirror-refs-live-2026-11-11.txt` | 7580 B |
| `origin-main-commits-live-2026-11-11.tsv` | 63661 B |
| `origin-main-dates.txt` | 17784 B |
| `prs-inventory-2026-11-11.tsv` | 10268 B |
| `refs-live-2026-11-11.txt` | 7943 B |
| `refs-local-foreachref-2026-11-11.txt` | 4482 B |
| `refs-local-foreachref.txt` | 5391 B |
| `refs-remote-lsremote.txt` | 7943 B |
| `rest-repos-docs.md` | 91291 B |
| `rest-statistics-docs.md` | 8597 B |
| `sidebar-contributors-panel-2026-11-11.html` | 5306 B |
| `trailer-commits-live-2026-11-11.json` | 9423 B |
| `trailer-commits.json` | 103678 B |

Key files: `sidebar-contributors-panel-2026-11-11.html` (§ 1.1), `contributors-page-live.html` + `graphs-contributors-data-live.json` (§ 1.2), `api-contributors-live.json` (§ 1.3), `api-stats-contributors-live.json` (§ 1.4), `commit-*.html` (§ 1.5), `origin-main-commits-live-2026-11-11.tsv` + `attribution-32-by-identity.json` (§ 3), `api-commit-verification.json` (GitHub verification payloads for all 32).

## 8. Sources (all retrieved 2026-11-11)

1. https://docs.github.com/en/pull-requests/how-tos/commit-changes/creating-a-commit-with-multiple-authors — co-author trailers; "For the commit to count as a contribution …"
2. https://docs.github.com/en/account-and-profile/how-tos/contribution-settings/troubleshooting-missing-contributions — connected e-mail rule; generic addresses cannot be linked; stale counts after rewrites
3. https://docs.github.com/en/account-and-profile/reference/email-addresses-reference — `noreply` address forms
4. https://docs.github.com/en/repositories/viewing-activity-and-data-for-your-repository/viewing-a-projects-contributors — graph definition (top 100, no merge commits); stale after history changes (~24 h)
5. https://docs.github.com/en/repositories/viewing-activity-and-data-for-your-repository/about-repository-graphs — graphs availability
6. https://docs.github.com/en/rest/repos/repos#list-repository-contributors — author-email grouping; REST cache
7. https://docs.github.com/en/rest/metrics/statistics#get-all-contributor-commit-activity — "total number of commits authored"; weeks schema

