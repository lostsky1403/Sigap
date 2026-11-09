# Migration Ledger Reconciliation

READ-ONLY audit of the live dev database (`sigap` @ `127.0.0.1:5433`, PostgreSQL 16.15)
against the tracked migration chain `packages/db/migrations/0001–0012`.

No live DDL/DML was issued. No migration file was modified. No `schema_migrations`
row was written. A disposable database (`sigap_reconcile_scratch`) was created,
migrated, diffed, and is left to be dropped (see Cleanup).

---

## 1. Headline findings

| # | Finding | Severity |
|---|---------|----------|
| 1 | `public.schema_migrations` exists but is **EMPTY** (`count(*) = 0`) | High (ledger gap) |
| 2 | Application schema is **byte-identical** to a fresh `migrate.Run` DB (18 tables, all columns/constraints/indexes/types/functions) | — (no drift) |
| 3 | `appointment_day(timestamptz)` is already the **WIB immutable** body from `0012_service_day_wib.sql` | — (no drift) |
| 4 | `notification_outbox` has all **10 CHECK constraints** with the **strengthened** phone predicates | — (no drift) |
| 5 | Naive `migrate.Run` on live would **fail at 0001** (duplicate `CREATE TYPE facility_type`) because the ledger is empty | Blocker for naive rebaseline |

**Bottom line: safe baseline is ACHIEVABLE (READY)** via a ledger-only backfill of the
12 `(version, checksum)` rows — because the live schema is proven identical to what the
migrations produce. It is *not* achievable by simply re-running the migrator (Finding 5).

---

## 2. Method

1. Inspected `public` catalog directly (tables, columns, constraints, indexes, enums,
   functions) on the live DB.
2. Read all 12 migration files and the two seed files (`rbac.sql`, `dev.sql`).
3. Built `sigap_reconcile_scratch` with `CREATE DATABASE` and applied `0001…0012` in
   numeric order through `psql` (`ON_ERROR_STOP=1`).
4. Seeded the scratch ledger with the 12 correct `(version, checksum)` rows derived from
   SHA-256 of the migration file bytes (identical to `migrate.go`'s
   `checksum()` = `sha256.Sum256(os.ReadFile(...))`).
5. Diffed live vs fresh with two independent methods:
   - **pg_dump** `--schema-only --no-owner --no-privileges --no-comments --schema=public`
   - **Structured catalog dump** (relname/attname/typedef/default/constraintdef/indexdef/
     enum labels/function def).

---

## 3. Diff results

### 3.1 Structured catalog diff (live vs fresh+ledger)

```
=== CATALOG DIFF (live vs fresh+ledger) ===
(clean)
```

**Zero differences.** Every `TABLE`, `COLUMN`, `CONSTRAINT`, `INDEX`, `TYPE`, and `FUNC`
row matches exactly. The only reason the pre-ledger diff was non-empty was the absence of
the `schema_migrations` table on the fresh side (mimicking a raw `psql` apply rather than
the migrator's `ensureTrackingTable()`); once the ledger table + 12 rows were added, the
catalog diff closed completely.

### 3.2 pg_dump diff (live vs fresh+ledger)

Only the per-database **restriction tokens** differ:

```
5c5
< \restrict POMqrB...jac12i
---
> \restrict Gojm1...ZM7J
1037c1037
< \unrestrict POMqrB...jac12i
---
> \unrestrict Gojm1...ZM7J
```

These are pg_dump 18's random `\restrict`/`\unrestrict` wrappers (session encryption tokens),
**not schema objects**. Filtering them out leaves the two dumps byte-identical.

### 3.3 Concrete live inventory

- `public` application tables: **18**
- `public` total tables (incl. `schema_migrations`): **19**
- `public` indexes: **58**
- `public` constraints: **71**
- `public` functions: **1** → `appointment_day(timestamptz) RETURNS date`
  `LANGUAGE sql IMMUTABLE PARALLEL SAFE`, body:
  `SELECT (($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date`
- `notification_outbox` CHECK constraints: **10**
- Live strengthened phone predicate (canonical, matches 0011 release form):
  `CHECK ((subject !~ '[0-9]{8,}') AND (subject !~ '[0-9][0-9\-._() ]{10,}[0-9]'))`

### 3.4 `appointment_day` (the "should now be WIB immutable" check)

Live definition (from `pg_get_functiondef`):

```sql
CREATE OR REPLACE FUNCTION public.appointment_day(timestamp with time zone)
 RETURNS date
 LANGUAGE sql
 IMMUTABLE PARALLEL SAFE
AS $function$
        SELECT (($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date
      $function$
```

**Confirmed correct** — this is the session-timezone-independent WIB day (`UTC+7`, no DST)
introduced by `0012_service_day_wib.sql`, not the older session-tz-dependent `$1::date`
from `0007`. No drift.

---

## 4. The empty ledger (`public.schema_migrations`)

```
schema_migrations_count = 0
migration_versions     = <empty>
```

`public.schema_migrations` **exists** (table + PK `schema_migrations_pkey` present) with the
correct shape (`version INTEGER`, `checksum BYTEA`, `applied_at TIMESTAMPTZ DEFAULT now()`),
but holds **0 rows**. So the tracking table was created (likely by an earlier
`ensureTrackingTable()` in `apps/api/internal/migrate/migrate.go`) while no version was ever
recorded.

This is a **pre-existing tracking gap**, not evidence of schema drift. It means:

- The migrator's `appliedVersions()` returns the empty set.
- A subsequent `migrate.Run()` would treat **all 12 migrations as pending** and re-execute
  them, failing at `0001_init.sql` with `type "facility_type" already exists` (rolled back,
  ledger still empty). Naive rebaseline is therefore **unsafe**.

---

## 5. Migration ordering & checksums (real file-byte SHA-256)

The migrator discovers by `NNNN_name.sql`, sorts by numeric version, applies each in its own
transaction, and records `checksum = sha256(file bytes)` as BYTEA. These are the exact values
a correct ledger must hold:

| version | file | SHA-256 (hex) |
|---------|------|----------------|
| 1 | `0001_init.sql` | `1481fe8af216b2512c4d4b047a7b0e7741c0801ac8c6e8149354fe0baf756dad` |
| 2 | `0002_medical_records.sql` | `bf7dc24edab9c01e3787cffa59e080451460f66aa900a10e420d92989b7a8d03` |
| 3 | `0003_identity_rbac.sql` | `17296851b6b9be15308c679518e93a4083152da68e6ee653c82879d187d7d2c2` |
| 4 | `0004_audit_events.sql` | `97880eb1f40c476061e3c4d296a0910fc2819b8a13084773b0dbbd8525727483` |
| 5 | `0005_appointments.sql` | `a9e2bf2e2fc5f18bfc88fb6c0dd97bde5e03f78fc5290b1e57587a8263682fc1` |
| 6 | `0006_notifications.sql` | `1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119` |
| 7 | `0007_checkin_constraints.sql` | `cbd9865c4c1d3e2e389893aa449b244d55395691befbfe6cb421c3b63ab46e8d` |
| 8 | `0008_identity_subject.sql` | `016c9092b868cff0fae6062ff117598da7194f1218af930ef99192efdbed7f03` |
| 9 | `0009_user_roles_lifecycle.sql` | `b7b5d0a043ba4499d6bd5a3f80c94d2c8de956c05d8a806e7b83c331642055f6` |
| 10 | `0010_demo_notes_column.sql` | `154a433dc6d2e9938d791e694de08118b44352ace0dce7bbe698226a3ac9c881` |
| 11 | `0011_notification_outbox_phone_constraints.sql` | `3568a910fed6a4e73baedeed093924cf8850597dd4bc7c0884fe1c740373d0b2` |
| 12 | `0012_service_day_wib.sql` | `618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663` |

Ordering is a simple 1–12 numeric sequence — no gaps, no out-of-order versions.

---

## 6. SAFE, REPEATABLE rebaseline procedure (NOT executed)

Because the live schema is byte-identical to a fresh migrate chain, the correct fix is a
**ledger-only backfill** — record what is already on disk, never re-execute DDL.

### Precondition (already proven this audit)

Live `public` catalog must equal the migrate-chain output. Verified via the two independent
diffs in §3 (both clean). Re-verify before production use:

```
pg_dump --schema-only --no-owner --no-privileges --no-comments --schema=public \
  | compare against a migrate.Run scratch DB, ignoring \restrict/\unrestrict tokens
```

### Procedure (one idempotent backfill; no DDL re-run)

```sql
INSERT INTO schema_migrations (version, checksum) VALUES
 (1,  decode('1481fe8af216b2512c4d4b047a7b0e7741c0801ac8c6e8149354fe0baf756dad','hex')),
 (2,  decode('bf7dc24edab9c01e3787cffa59e080451460f66aa900a10e420d92989b7a8d03','hex')),
 (3,  decode('17296851b6b9be15308c679518e93a4083152da68e6ee653c82879d187d7d2c2','hex')),
 (4,  decode('97880eb1f40c476061e3c4d296a0910fc2819b8a13084773b0dbbd8525727483','hex')),
 (5,  decode('a9e2bf2e2fc5f18bfc88fb6c0dd97bde5e03f78fc5290b1e57587a8263682fc1','hex')),
 (6,  decode('1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119','hex')),
 (7,  decode('cbd9865c4c1d3e2e389893aa449b244d55395691befbfe6cb421c3b63ab46e8d','hex')),
 (8,  decode('016c9092b868cff0fae6062ff117598da7194f1218af930ef99192efdbed7f03','hex')),
 (9,  decode('b7b5d0a043ba4499d6bd5a3f80c94d2c8de956c05d8a806e7b83c331642055f6','hex')),
 (10, decode('154a433dc6d2e9938d791e694de08118b44352ace0dce7bbe698226a3ac9c881','hex')),
 (11, decode('3568a910fed6a4e73baedeed093924cf8850597dd4bc7c0884fe1c740373d0b2','hex')),
 (12, decode('618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663','hex'))
ON CONFLICT (version) DO NOTHING;
```

If `applicable flag` guarantees are desired, wrap in `BEGIN; … COMMIT;` and re-check
`SELECT count(*) FROM schema_migrations;` must return `12`.

### Post-backfill verification

- `migrate.Run` becomes an idempotent **no-op** (all 12 versions already applied) → returns
  count `0` and logs `all migrations up to date`.
- `migrate.Status` reports `applied = 12`, `pending = 0`.

### Why NOT naive `migrate.Run`

The migrator selects pending work by **version only** and never re-reads a stored checksum
(`appliedVersions()` reads `version`; `Run()` skips `if applied[m.Version]`). With the
ledger empty it would re-apply 0001 and fail on the duplicate `CREATE TYPE facility_type`.
The backfill above is the only safe path.

---

## 7. Limitations / recommendations (BLOCKED items, marked honestly)

- **Checksum drift is not detected by the migrator.** Even after backfill,
  `migrate.Run`/`migrate.Status` compare `version` only, not the recorded `checksum` vs the
  file bytes. A future silent edit to an *already-applied* migration file (exactly the
  `9d4e68e`→`0011` history) would go undetected. **Recommendation (BLOCKED — not performed,
  requires a code change + operator decision):** add a `migrate.VerifyChecksums()` that
  compares `schema_migrations.checksum` to `sha256(file bytes)` and fails closed on
  mismatch. This audit proves the *current* files match the *current* schema, but future
  drift needs this guard.
- **`0006_notifications.sql` is the only migration with its own `BEGIN/COMMIT`** —
  the runner wraps each migration in a transaction, so that file ends the runner's
  transaction early (documented in its header). This affects only the atomicity guarantee
  for that one migration, not the final schema; noted for completeness.
- **`0011`/`0012` are one-shot DO blocks** that no-op on a HEAD-built DB. Re-running them is
  harmless, but they are not needed for the rebaseline (backfill records them as applied
  without re-executing).
- **Seed files (`rbac.sql`, `dev.sql`, `demo.sql`) are data-only and outside the ledger.**
  They are not tracked by `schema_migrations` and were intentionally not part of this diff;
  live seed *data* (facilities, app_users, roles, permissions, etc.) was not compared row-by-row
  to the seed files. A follow-up data-parity check would be needed to confirm seed freshness,
  but it does not affect the schema-ledger reconciliation.

---

## 8. Cleanup (to be performed)

- `DROP DATABASE sigap_reconcile_scratch;` (disposable scratch DB, no longer needed).
- Remove temporary `design/_scratch/` artifacts (live.sql, fresh.sql, *.cat, catalog.sql,
  schema_catalog.sql, *.err).