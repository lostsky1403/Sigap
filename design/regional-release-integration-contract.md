# Regional release integration contract (single source of truth)

**Status:** PROPOSED_CONTRACT — precise and internally consistent; verified at file and
disposable-database level. Nothing in this document authorizes a merge, a force-push, a
production migration, or a production deployment.

Scope: PR #96 (`hotfix/post-merge-ci-rust-stabilization`, head
`036a1aa9f04108cb95f8ce85e93a5c8a925056fc`) and the future regional release that follows
the development branch `feat/sigap-selfhosted-auth` (worktree `F:\sigap-design-v2`,
development head `7c4f7f46cfa830538154994a420b44656520abbb`).

---

## 6.2 Release-integration strategy — exactly one, and no obsolete alternatives

**WITHDRAWN / REMOVED:** the suggestion to *"promote `feat/sigap-selfhosted-auth` as the
declared release branch"*. It is obsolete and is not an acceptance gate. A feature branch
is not a release branch; relabelling it would falsify the release migration guard and hide
the distinction between development and release history. This withdrawal also supersedes
any earlier report wording that treated that feature branch as the declared release branch.

The only release-integration strategy is:

1. **Preserve the existing development feature branch.** `feat/sigap-selfhosted-auth`
   stays as-is; no rename, no relabel, no reset.
2. **Integrate canonical `origin/main` into the feature branch with a normal, non-rewriting
   merge**, when separately authorized. Canonical main is
   `51ba1d53fb85e76c557de3ff0d84b27688645e34`.
3. **Resolve conflicts while preserving** Better Auth, regional RBAC, Patient Continuity,
   and migrations `0012`–`0014`.
4. **After integration and regression verification, create a NEW dedicated release branch
   from the integrated development tip.** Proposed name: `release/sigap-regional-v1`.
   This is a proposed future branch name, **not** proof that the branch already exists.
5. **Do not rename or falsely relabel the feature branch to satisfy the release migration
   guard.** Do not merge the feature branch into `main` in this task.

Historical note: `design/ui-ux-overhaul` was the *historical* release branch for the
`d7705f09`-anchored UI-redesign migration guard. It is not the regional release branch and
creates no second strategy; the historical guard assertions stay untouched.

---

## Regional migration baseline — separate from the historical anchor

```
REGIONAL_MIGRATION_BASE = 51ba1d53fb85e76c557de3ff0d84b27688645e34
REGIONAL_RELEASE_BRANCH = release/sigap-regional-v1          (proposed, future)
```

The historical `d7705f09` anchor and its `0006` + `0011` assertions remain **global to the
historical runbook** and must not be repurposed for the regional release.

### PROPOSED_CONTRACT (asserted against `REGIONAL_MIGRATION_BASE`)

Expected added migration set — exactly these three, relative to `51ba1d5…`:

```
packages/db/migrations/0012_service_day_wib.sql
packages/db/migrations/0013_organizations.sql
packages/db/migrations/0014_patient_account.sql
```

Expected SHA-256 checksums (verified against files on `feat/sigap-selfhosted-auth`):

| Migration | SHA-256 |
| --- | --- |
| `0012_service_day_wib.sql` | `618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663` |
| `0013_organizations.sql` | `6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1` |
| `0014_patient_account.sql` | `001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624` |

The contract asserts:

- Exactly those three migrations are **added** relative to the selected baseline.
- **No removed migrations** and **no renamed migrations** (`git diff --name-status` yields
  only `A` rows for those three paths).
- **No modification** to previously applied migrations `0001`–`0011` (the diff over those
  paths is empty; checksums unchanged).
- Valid forward-only version ordering (`0001 … 0014`, no gaps, no duplicates, no
  out-of-order versions).
- The checksums above match byte-for-byte.
- Correct SQL behavior in a **disposable PostgreSQL database** (see below).

### VERIFIED_IN_DISPOSABLE_BRANCH (evidence observed, not a release claim)

- `git diff --name-status 51ba1d5…..feat/sigap-selfhosted-auth -- packages/db/migrations`
  → exactly `A 0012_service_day_wib.sql`, `A 0013_organizations.sql`,
  `A 0014_patient_account.sql`; no `D`, `M`, or `R` rows.
- `origin/main @ 51ba1d5…` carries `0001_init.sql … 0011_notification_outbox_phone_constraints.sql`
  only; the feature tip carries `0001 … 0014`.
- The three SHA-256 values above were recomputed from the actual files and match the
  values recorded in `design/migration-baseline-recovery-plan.md` and
  `design/migration-ledger-reconciliation.md` (where `0014` is labelled *pending*).
- Disposable PostgreSQL 18.4 cluster (synthetic, temp dir, dropped after the check, no
  production host/DB): all fourteen migrations `0001 … 0014` applied in forward order
  with `ON_ERROR_STOP=1` and exit 0. Assertions passed: `appointment_day(timestamptz)` is
  the immutable UTC+7 WIB definition; `uq_active_booking_per_patient_day` exists;
  `organizations` exists with nullable `facilities.organization_id` and
  `user_roles.organization_id`; `appointments.patient_account_id` exists with its index
  and an `ON DELETE SET NULL` FK. `appointment_day('2026-11-18 23:30+07') = 2026-11-18`.

### IMPLEMENTED_IN_RELEASE_BRANCH

**NONE.** `release/sigap-regional-v1` is proposed, not created. No migration was applied to
a production database and no release guard was switched over in this task.

---

## Exact future guard configuration (to be added, not yet implemented)

Add a **regional-only** block to `scripts/ops/test-production-deploy-guards.sh`, keeping the
existing `d7705f09` block intact for historical branches:

```sh
REGIONAL_RELEASE_BRANCH=release/sigap-regional-v1
REGIONAL_MIGRATION_BASE=51ba1d53fb85e76c557de3ff0d84b27688645e34
EXPECTED_REGIONAL_MIGRATIONS="packages/db/migrations/0012_service_day_wib.sql packages/db/migrations/0013_organizations.sql packages/db/migrations/0014_patient_account.sql"
EXPECTED_REGIONAL_SHA256="0012_service_day_wib.sql:618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663 0013_organizations.sql:6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1 0014_patient_account.sql:001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624"
```

Assertions for that block, only when the branch context is the regional release:

1. `git merge-base --is-ancestor "$REGIONAL_MIGRATION_BASE" HEAD` exits 0.
2. `git diff --name-status "$REGIONAL_MIGRATION_BASE"..HEAD -- packages/db/migrations`
   yields exactly three rows, all `A`, with the three paths above — no `D`, `M`, `R`.
3. `git diff --name-only "$REGIONAL_MIGRATION_BASE"..HEAD -- <0001..0011 paths>` is empty.
4. Migration versions parse as integers `1..14` with no gap, duplicate, or regression.
5. `sha256sum` of each declared file equals its expected value.
6. Disposable-Postgres scenario (mirroring `test-migration-0011.sh`): apply all migrations
   from the baseline in order and assert the schema objects, not merely exit status.

### Negative tests required before the guard can be called READY

| Negative case | Expected result |
| --- | --- |
| Modified applied migration (e.g. edit `0006_notifications.sql`) | FAIL — non-empty diff over `0001`–`0011` |
| Deleted applied migration | FAIL — `D` row in name-status |
| Undeclared new migration (`0015_*.sql`) | FAIL — extra `A` row |
| Missing declared migration (`0013` absent) | FAIL — expected set not matched |
| Unexpected change to any of `0001`–`0011` | FAIL — checksum/diff assertion |
| Incorrect release-branch context (guard run on feature branch or `main`) | FAIL — context assertion |

---

## Runbook rows that need updating (all proposed, none applied here)

| File | Location | Change required |
| --- | --- | --- |
| `docs/operations/DEPLOYMENT_RUNBOOK.md` | §1 rows A/B/C (~L39–41); §1 notes (~L49, L56) | Add a clearly separated *regional* inventory row; keep the historical `d7705f09` wording unchanged for the historical release. |
| `docs/operations/DEPLOYMENT_RUNBOOK.md` | §3.1 (~L177) | State that the `0011` one-off applies to the historical release, not the regional release. |
| `docs/operations/DEPLOYMENT_RUNBOOK.md` | §5 anchor section (~L352–397), §5 guard lines (~L435–440) | Keep historical anchor assertions; add the separate `REGIONAL_MIGRATION_BASE` block. |
| `docs/operations/DEPLOYMENT_RUNBOOK.md` | §12 (~L1173–1177, ~L1570–1578) | Extend with the regional expected set and checksums; do not replace the historical set. |
| `docs/operations/RELEASE_CHECKLIST.md` | A4 (~L19) | Add a regional A4 row with the three-path set; preserve the historical `d7705f09` row. |
| `scripts/ops/test-production-deploy-guards.sh` | ~L186, ~L219, ~L236–257, check 19 (~L384–407) | Add the regional-only block above; leave the historical anchor/ancestor checks intact. |

**Acceptance state:** contract precise and internally consistent = YES; evidence gathered in
disposable branch/DB = YES; implemented in release branch = NO. The owner must authorize the
integration merge and the release-branch creation before `MIGRATIONS_0012_0014_PLAN` can be
reported as implemented.
