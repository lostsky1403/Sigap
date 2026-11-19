# Secret-scan classification — development branch (2026-11-18)

Scope: full-history `gitleaks detect` on development HEAD `c916d710b15d05bb55c512258e1f62434970b96f` (967 commits), plus a working-tree-only scan. **No secret values are reproduced in this document** — only rule, file, line, commit and the evidence used to classify.

## Summary

| Metric | Value |
| --- | --- |
| Raw full-history findings (development HEAD, all local refs in the worktree) | **15** |
| Raw findings in the publishable branch history alone (clean clone) | **14** — the one excluded finding is `9e0d61a2…` on a local-only branch; it is classified below anyway |
| Findings also present on canonical main `4813de64…` | **2** |
| Findings introduced on the development branch | **13** |
| Genuine exposed credentials | **0** |
| Unresolved findings | **0** |
| Policy-adjusted findings (commit-scoped exclusion) | **0** |
| Working-tree-only scan | 14 hits, all in ignored/untracked local artifacts or the 3 classified text findings below |

All 15 findings use the `generic-api-key` rule. Six **distinct matched values** account for them; none carries a credential prefix (no `sk-`, `sk-ant-`, `ghp_`, `github_pat_`, `AKIA`/`ASIA`, `xox`, `pk_live`/`sk_live`, `AIza`). Two are UUIDs, two are short local test tokens, and two are 64-hex test placeholders.

## Classification by value (6 unique values → 15 findings)

| # | Values' shape | Files / lines | Introduction commits (author, date) | Classification | Evidence |
| --- | --- | --- | --- | --- | --- |
| 1 | 36-char **UUID** | `design/prototypes/sigap-civic-v2.pen:29690`, `:36199` (8 findings) | `95879c7a…`, `aa8d06aa…`, `fffae092…`, `1689e531…`, `146ad669…`, `ffc9a247…`, `aa2c2b10…`, `90c73b78…` — "SIGAP Design", 15 Oct 2026 | **False positive — design prototype UUID** | Matches `^[0-9a-f]{8}-…{12}$`; gitleaks' own matched context contains "token" (a field name in a Pen.dev design canvas); one unique value across two fixed lines carried unchanged through eight design commits; occurs in no other repo file; no service/config references it. The `.pen` format is encrypted and the Pencil app was unavailable, so the value was not read; classification rests on shape + context + usage evidence. |
| 2 | 24-char alnum | `design/email-mfa-flows-verification.md:213`, `:323` (2 findings) | `ec505269…` — lostsky1403, 11 Nov 2026 | **Verified documentation placeholder** | Lines are a captured local `127.0.0.1:5173` password-reset URL and a local `auth.verification` DB snapshot; loopback-only, synthetic `.sigap.local` addresses; no credential prefix; same fingerprint at both lines. |
| 3 | 36-char **UUID** | `apps/api/internal/auth/better_auth_compat_test.go:24` (1 finding) | `3907aeab…` — lostsky1403, 10 Nov 2026 | **False positive — test subject** | Constant is `betterAuthSub`; adjacent constants pin issuer/audience/kid to local dev values; a Better Auth user id (UUID), not a secret. |
| 4 | 64-hex | `scripts/ops/test-restore-contract.sh:74` (2 findings) | `1d55c436…`, `d1f4c051…` — lostsky1403, 10 Oct 2026 | **Verified synthetic test fixture** | Current file documents the value as "A SYNTHETIC 64-hex placeholder, deliberately low-entropy" and replaced it with an obvious low-entropy constant; only format is load-bearing. |
| 5 | 64-hex | `scripts/ops/test-restore-contract.sh:128` (2 findings) | `9e0d61a2…`, `bf84cce1…` — lostsky1403, 12 Oct 2026 | **Verified synthetic test fixture** | Same documented fixture; historic "realistic-looking digest" explicitly described as a false positive in the file comment. |

(Values 4 and 5 are the 2 findings also present on canonical main; both were already accepted there.)

## Working-tree-only findings (not published)

`.env:118`, `.scratch/rbac/*.token` (5 files), and `supabase/.temp/.../docker.env` (4 hits) are all **ignored and untracked** (`git check-ignore` and `git ls-files` verified). They are local dev/probe artifacts on this workstation and cannot be published by pushing the branch. They are deliberately **not** allowlisted — a developer must not rely on the scanner being silenced for them.

## Policy (exact-finding, not commit-wide)

`.gitleaks.toml` contains **only** `title` and `[extend] useDefault = true` — no rules are disabled and no global/path/regex allowlist is defined. All 15 classified findings are excluded by exact **commit-qualified fingerprint** (`commit:file:rule:start-line`) in `.gitleaksignore`, the mechanism documented by Gitleaks 8.30.1. Fingerprints contain no secret values. Suppressing one exact finding is strictly narrower than suppressing every finding in a commit: a new credential in an allowlisted file, at an allowlisted line, or in a new commit has a different fingerprint and is reported.

Adversarial verification (disposable clone, synthetic values only, exit status checked): raw scan, policy-adjusted scan, working-tree scan, known-false-positive suppression, new secret in an allowlisted file, new secret at an allowlisted line in a new commit, new secret in an unrelated file, a non-generic built-in rule (`aws-access-token`), CI config parity, and unrelated historical findings remain visible.

Raw and policy-adjusted results must always be reported separately; a clean policy-adjusted scan is never evidence that the raw findings vanished.

## Verdict

No genuine or possibly-active credential was found, no rotation/revocation is required, no history rewrite is justified, and the development history is safe to publish from the secret-scan perspective.
