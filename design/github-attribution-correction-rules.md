# GitHub AI-attribution correction — rules (V2, as applied)

**Repo:** `lostsky1403/Sigap` (public) · **Date:** 2026-11-11 · **Status:** the rule set actually executed in the V2 rehearsal (`design/github-rewrite-rehearsal-v2.md`); **nothing was pushed to GitHub**.
**Applies to:** every ref in the rewrite universe — 23 remote branches, 4 tags, 93 `refs/pull/*/head`, 25 local-only refs (`refs/local/*`: 22 branches, `refs/stash`, 2 `refs/trae_merge/*`).

## 1. Scope — what is corrected

A **message-only** correction: delete the machine/AI `Co-authored-by:` trailer **lines** listed in §2 from commit messages.
Everything else in a commit object is preserved byte-for-byte: tree, parent list (parents are only remapped to their rewritten counterparts, which is unavoidable), author identity+date, committer identity+date, and every other message line.

## 2. The strip set (exact and complete)

Match the **key** case-insensitively (`Co-authored-by:`, `Co-Authored-By:`, `co-authored-by:` all appear in this history) and the **value** case-insensitively against exactly these five identities:

| # | matched value (case-insensitive) | lines in the 145-ref universe | on pushable refs (23 branches + 4 tags) | only on `refs/pull/*` |
|---|---|---|---|---|
| 1 | `traeagent <traeagent@users.noreply.github.com>` | 11 | 11 | 0 |
| 2 | `Codebuff <noreply@codebuff.com>` | 22 | 11 | 11 |
| 3 | `Claude Opus 4.7 <noreply@anthropic.com>` (+ any `Claude`/`…@anthropic.com` variant present) | 6 | 6 | 0 |
| 4 | `internal-model` (no e-mail — not RFC-822 shaped) | 6 | 3 | 3 |
| 5 | `t <t@t>` (see §3) | 1 | 1 | 0 |
| | **total** | **46** | **32** | **14** |

**Census (line-anchored, case-insensitive) over the pre-rewrite universe:** 46 commits carry a trailer; those 46 commits carry exactly **46** `Co-authored-by`-keyed lines in total — i.e. **every** line-anchored `Co-authored-by:` line in the repository is one of the five identities above, and there are **no other** `co-authored-by`-keyed lines to preserve (V2 verification, `analysis/verification.json`). The 14 pull-ref-only targets are the pre-squash branch tips of PRs #40–#50 and #67–#69 (same trailers as their squash merges).

## 3. `t <t@t>` — resolved from evidence, then stripped

*The trailer appears exactly once, on `8f0370e8` — `chore: remove the historical gitleaks allowlist (#87)` (author: owner `114224549+lostsky1403@…`; committer: GitHub web-flow squash identity).*

Evidence:
1. GitHub user `t` exists (id 60341) but the address `t@t` is a **dummy local address with no domain**; the only address form that resolves by login is `<login>@users.noreply.github.com` — `t@t` is not it, and no account's verified e-mail is `t@t` (unverifiable from outside; consistent with the observed data).
2. It contributes **nothing** to the contributor graph: `/stats/contributors` totals are `334 + 28 = 362` for four identities, and 28 = traeagent 11 + codebuff-team 11 + claude 6. `internal-model` (3) and `t <t@t>` (1) are excluded — the same inert class.
3. It sits inside the same AI-trailer era (`#31`–`#93`) and is machine-injected placeholder metadata, not a human co-author.

**Decision (consistent with the orchestrator's classification): strip it together with the other four.** Leaving it would keep a non-human `Co-authored-by` line in history and leave the trailer census at 1 instead of 0, with zero benefit.

## 4. Removal semantics (byte-exact)

For every commit message:

1. A line is a **target** iff it matches `^[ \t]*co-authored-by[ \t]*:[ \t]*(value)[ \t]*\r?$` (case-insensitive key) and `value` case-insensitively equals one of §2's identities. **Line-anchored**: prose that merely mentions the phrase mid-line is never matched (e.g. `0b02e532` "…because Co-authored-by trailers whose e-mails map to real GitHub accounts…" survives untouched).
2. Delete the **whole target line including its terminator** (`\n` or `\r\n`). A final target line with **no** terminator is deleted the same way — the removal must not add or drop a trailing newline for it.
3. After removal, collapse the now-trailing run of empty lines so that the message ends **exactly as it ended before**: one `\n` if the original message ended with `\n`; no `\n` if it did not. (46 targets: 31 ended with LF, **15 squash-merge messages ended with no LF** and keep that property.)
4. If nothing was removed, the message is returned **byte-identical** (no normalization pass touches non-target messages).

Reference implementation (the exact code passed as `git filter-repo --message-callback`, also in `%TEMP%\sigap-rw-v2\migration\message-callback.txt`):

```python
import re
_ID_TRAE     = re.compile(rb'traeagent <traeagent@users\.noreply\.github\.com>', re.I)
_ID_CODEBUFF = re.compile(rb'codebuff <noreply@codebuff\.com>', re.I)
_ID_CLAUDE   = re.compile(rb'claude[^<]*<[^>]*(anthropic|claude)[^>]*>', re.I)
_ID_ANTHROPIC= re.compile(rb'[^<]*<[^>]*anthropic\.com>', re.I)
_ID_INTERNAL = re.compile(rb'internal-model', re.I)
_ID_T        = re.compile(rb't <t@t>', re.I)

def _is_target_key(line):
    m = re.match(rb'^[ \t]*co-authored-by[ \t]*:[ \t]*(.*?)[ \t]*\r?$', line, re.I)
    if not m:
        return False
    v = m.group(1)
    return bool(_ID_TRAE.fullmatch(v) or _ID_CODEBUFF.fullmatch(v) or _ID_CLAUDE.fullmatch(v)
                or _ID_ANTHROPIC.fullmatch(v) or _ID_INTERNAL.fullmatch(v) or _ID_T.fullmatch(v))

_removed = False; _out = []
_lines = message.split(b'\n'); _had_nl = message.endswith(b'\n')
for _i, _l in enumerate(_lines):
    _is_artifact = (_i == len(_lines) - 1) and _had_nl   # trailing '' from a final LF
    if not _is_artifact and _is_target_key(_l):
        _removed = True
        continue
    _out.append(_l)
if _removed:
    message = b'\n'.join(_out).rstrip(b'\n') + (b'\n' if _had_nl else b'')
return message
```

> Tooling guard: run with `--preserve-commit-hashes`, otherwise filter-repo also rewrites abbreviated SHAs **inside** message bodies (v1 measured 33 extra messages edited without it). Also **do not pass `--refs`** (it silently disables the rewrite in 2.47.0).

## 5. Must NOT be touched

* **Author / committer / dates.** All 46 targets (and all 570 commits in the universe) are authored by the owner's identities (`moch22188ti@student.nurulfikri.ac.id`, `lostsky1403@users.noreply.github.com`, `114224549+lostsky1403@users.noreply.github.com`) and committed by the owner or GitHub web-flow `<noreply@github.com>`. Confirmed per-commit in the V2 rehearsal: **570/570 identities and dates byte-identical** after rewrite.
* **File trees.** 570/570 trees byte-identical (0 mismatches).
* **Any other line**, including: the final line with no trailing newline, the `🤖 Generated with Codebuff` body line (not a trailer), prose mentions of "Co-authored-by", and every other trailer type (none present, but the rule set has no others).
* **In-message SHA references** (`--preserve-commit-hashes`).
* **Tags' messages/tagger** — tag objects are only re-created because their target commit moved; tag message and tagger are byte-identical (verified: `meta_identical` and `msg_identical` = true for all four).

## 6. Verification this rule set must pass (and did, V2)

1. Line-anchored trailer census across **all rewritten refs**: 46 → **0** (pushable subset 32 → 0).
2. For every mapped commit pair: `new_message == reference_transform(old_message)` — 570/570, 0 unexplained differences; `message_altered = 46`, `removed_lines_total = 46`.
3. Trees 570/570 identical; author/committer/date 570/570 identical; parent count and remap 0 mismatches; merges 11 → 11.
4. No commit dropped: universe 570 → 570, per-ref history counts identical for all 145 refs, map injective, all old commits mapped, all new commits reachable.
5. `git fsck --full` clean on a fresh clone of the result.

## 7. Non-goals

No author/committer rewrite, no `.mailmap`, no branch renames, no tag renames, no deletion of old objects on GitHub, no change to GitHub settings, no attempt to rename/erase the AI accounts themselves. The correction removes the **trailer lines**; GitHub's contributor-graph recomputation after a force-push is a GitHub-side, cached process (not something this rule set can guarantee — see the rehearsal report §9).
