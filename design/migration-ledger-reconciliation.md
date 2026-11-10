# Migration Ledger Reconciliation

READ-ONLY audit of the live dev database (`sigap` @ `127.0.0.1:5433`, PostgreSQL 16.15)
against the tracked migration chain `packages/db/migrations/0001–0014`.

**Reconciliation span (R3, 2026-11-10):** live carries **`0001–0013`** applied
(`organizations` and the org-tenancy columns exist; `appointments.patient_account_id`
does **not**). `0014_patient_account.sql` exists on disk but is **pending** — not yet
applied to live. Everything below is reconciled through the highest **applied**
migration (0013); 0014 is recorded and classified separately as a pending branch
migration.

No live DDL/DML was issued. No migration file was modified. No `schema_migrations`
row was written. A disposable database (`sigap_reconcile_scratch`) was created,
migrated, diffed, and is left to be dropped (see Cleanup).

---

## 1. Headline findings

| # | Finding | Severity |
|---|---------|----------|
| 1 | `public.schema_migrations` exists but is **EMPTY** (`count(*) = 0`) | High (ledger gap) |
| 2 | Application schema is **byte-identical** to a fresh `migrate.Run` DB through the highest **applied** span **`0001–0013`** (19 tables incl. `organizations`; all columns/constraints/indexes/types/functions) | — (no drift) |
| 3 | `appointment_day(timestamptz)` is already the **WIB immutable** body from `0012_service_day_wib.sql` | — (no drift) |
| 4 | `notification_outbox` has all **10 CHECK constraints** with the **strengthened** phone predicates | — (no drift) |
| 5 | Naive `migrate.Run` on live would **fail at 0001** (duplicate `CREATE TYPE facility_type`) because the ledger is empty | Blocker for naive rebaseline |
| 6 | `0014_patient_account.sql` exists on disk (SHA-256 `001e7784…e21624`) but is **NOT applied** to live: `appointments.patient_account_id` + `idx_appointments_patient_account_id` + FK are absent | Pending branch migration (unchanged from prior classification pattern) |

**Bottom line: safe baseline is ACHIEVABLE (READY)** for the **verified `0001–0013`
span** via a ledger-only backfill of the 13 `(version, checksum)` rows — because the
live schema is proven identical to what the migrations through 0013 produce. `0014` is
**pending** (not in live) and must NOT be backfilled until it is actually applied/live-
diffed. It is *not* achievable by simply re-running the migrator (Finding 5).


---

## 2. Method

1. Inspected `public` catalog directly (tables, columns, constraints, indexes, enums,
   functions) on the live DB.
2. Read all 14 migration files and the seed files (`rbac.sql`, `dev.sql`, `demo.sql`,
   `regional_tenancy.sql`).
3. Built **two** disposable scratch DBs with the **tracked Go migrator**
   (`apps/api/cmd/ci-migrate`, `migrate.Run`):
   - `sigap_reconcile_scratch` — full chain `0001…0014` (14 applied, ledger populated);
   - `sigap_reconcile_scratch13` — chain `0001…0014` applied, then the 0014 objects
     stripped (`DROP INDEX idx_appointments_patient_account_id; ALTER TABLE appointments
     DROP COLUMN patient_account_id;`) to yield the pure `0001…0013` runner schema.
4. Confirmed the scratch ledger `(version, checksum)` rows match SHA-256 of the
   migration file bytes (identical to `migrate.go`'s `checksum()` =
   `sha256.Sum256(os.ReadFile(...))`) for all 14.
5. Diffed live vs scratch with two independent methods:
   - **pg_dump** `--schema-only --no-owner --no-privileges --no-comments --schema=public`
   - **Structured catalog dump** (relname/attname/typedef/default/constraintdef/indexdef/
     enum labels/function def).

---

## 3. Diff results

### 3.1 Structured catalog diff (live vs scratch13 — pure 0001–0013 runner schema)

```
=== CATALOG DIFF (live vs scratch13) ===
(clean)
```

**Zero differences.** Every `TABLE`, `COLUMN`, `CONSTRAINT`, `INDEX`, `TYPE`, and `FUNC`
row matches exactly between live and the tracked-runner schema through **`0001–0013`**
(`sigap_reconcile_scratch13`: full 14-chain applied, 0014 objects stripped). Structurally
this is the same closed-diff result as before, now spanning the org-tenancy migration.

### 3.2 pg_dump diff (live vs scratch13)

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
**not schema objects**. Filtering them out leaves the two dumps byte-identical
(`diff` exit 0, zero lines).

### 3.3 Concrete live inventory (current, R3)

- `public` application tables: **19** (18 as before + `organizations` from 0013)
- `public` total tables (incl. `schema_migrations`): **20**
- `public` indexes: **63** (58 + 5 from 0013: `idx_facilities_organization_id`,
  `idx_user_roles_organization_id`, `uq_user_roles_active_org_uniq`, plus the two
  constraint-backing unique indexes `organizations_pkey` and `organizations_name_key`)
- `public` constraints: **75** (71 + 4 from 0013: `organizations_pkey`,
  `organizations_name_key`, `facilities_organization_id_fkey`,
  `user_roles_organization_id_fkey`)
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

### 3.5 R3 diff isolation: the 0014 delta is the ONLY frontier difference

Live vs the full-chain scratch (`sigap_reconcile_scratch`, 14 applied) shows **exactly
the 0014 additions** — nothing else:

```
--- structured catalog diff (live vs scratch_full14) ---
COLUMNS : + appointments.patient_account_id uuid NULL
CONSTRAINTS: + appointments_patient_account_id_fkey FOREIGN KEY (patient_account_id)
               REFERENCES app_users(id) ON DELETE SET NULL
INDEXES : + idx_appointments_patient_account_id ON appointments (patient_account_id)
```

`pg_dump` (after stripping `\restrict`/`\unrestrict` tokens) agrees: the only diff hunks
are the `patient_account_id` column, its index, and its FK. No other table/constraint/
index/type/function differs between live and a runner that has applied 0014. This is the
expected forward-only additive shape of `0014_patient_account.sql` (nullable column, one
index, one FK `ON DELETE SET NULL`).


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
- A subsequent `migrate.Run()` would treat **all 14 migrations as pending** and re-execute
  them, failing at `0001_init.sql` with `type "facility_type" already exists` (rolled back,
  ledger still empty). Naive rebaseline is therefore **unsafe**.

---

## 5. Migration ordering & checksums (real file-byte SHA-256)

The migrator discovers by `NNNN_name.sql`, sorts by numeric version, applies each in its own
transaction, and records `checksum = sha256(file bytes)` as BYTEA. These are the exact values
a correct ledger must hold:

| version | file | SHA-256 (hex) | span |
|---------|------|----------------|------|
| 1 | `0001_init.sql` | `1481fe8af216b2512c4d4b047a7b0e7741c0801ac8c6e8149354fe0baf756dad` | verified |
| 2 | `0002_medical_records.sql` | `bf7dc24edab9c01e3787cffa59e080451460f66aa900a10e420d92989b7a8d03` | verified |
| 3 | `0003_identity_rbac.sql` | `17296851b6b9be15308c679518e93a4083152da68e6ee653c82879d187d7d2c2` | verified |
| 4 | `0004_audit_events.sql` | `97880eb1f40c476061e3c4d296a0910fc2819b8a13084773b0dbbd8525727483` | verified |
| 5 | `0005_appointments.sql` | `a9e2bf2e2fc5f18bfc88fb6c0dd97bde5e03f78fc5290b1e57587a8263682fc1` | verified |
| 6 | `0006_notifications.sql` | `1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119` | verified |
| 7 | `0007_checkin_constraints.sql` | `cbd9865c4c1d3e2e389893aa449b244d55395691befbfe6cb421c3b63ab46e8d` | verified |
| 8 | `0008_identity_subject.sql` | `016c9092b868cff0fae6062ff117598da7194f1218af930ef99192efdbed7f03` | verified |
| 9 | `0009_user_roles_lifecycle.sql` | `b7b5d0a043ba4499d6bd5a3f80c94d2c8de956c05d8a806e7b83c331642055f6` | verified |
| 10 | `0010_demo_notes_column.sql` | `154a433dc6d2e9938d791e694de08118b44352ace0dce7bbe698226a3ac9c881` | verified |
| 11 | `0011_notification_outbox_phone_constraints.sql` | `3568a910fed6a4e73baedeed093924cf8850597dd4bc7c0884fe1c740373d0b2` | verified |
| 12 | `0012_service_day_wib.sql` | `618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663` | verified |
| 13 | `0013_organizations.sql` | `6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1` | verified (live-diffed, R3) |
| 14 | `0014_patient_account.sql` | `001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624` | pending (on disk, not applied) |

Ordering is a simple 1–14 numeric sequence — no gaps, no out-of-order versions. Rows
1–13 are the **verified span** (live schema proven identical through 0013); row 14 is
recorded for completeness and is **pending** (live does not yet carry its objects).

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
 (12, decode('618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663','hex')),
 (13, decode('6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1','hex'))
ON CONFLICT (version) DO NOTHING;
```

**Row 14 (`0014_patient_account.sql`) is NOT included** — it is a pending branch
migration (not present in live). If/when it is applied to live and re-diffed, append:

```sql
 (14, decode('001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624','hex'))
```

inside the same transaction.

If `applicable flag` guarantees are desired, wrap in `BEGIN; … COMMIT;` and re-check
`SELECT count(*) FROM schema_migrations;` must return `13` (or `14` if 0014 was applied
and backfilled).

### Post-backfill verification

- `migrate.Run` becomes an idempotent **no-op** (all 13 verified versions already applied) →
  returns count `0` and logs `all migrations up to date`.
- `migrate.Status` reports `applied = 13`, `pending = 1` (0014) — or `applied = 14`,
  `pending = 0` if 0014 was also applied and backfilled.

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
- **Seed files (`rbac.sql`, `dev.sql`, `demo.sql`, `regional_tenancy.sql`) are data-only
  and outside the ledger.** They are not tracked by `schema_migrations` and were
  intentionally not part of this diff; live seed *data* (facilities, app_users, roles,
  permissions, orgs, etc.) was not compared row-by-row to the seed files. A follow-up
  data-parity check would be needed to confirm seed freshness, but it does not affect the
  schema-ledger reconciliation.
- **R3: `0014_patient_account.sql` is pending, not applied.** Its checksum is recorded
  (§5) so the ledger is future-ready, but it must NOT be backfilled until it is applied to
  live and the §3 diff is re-run against the 14-span. The verified backfill span is
  `0001–0013` only.

---

## 8. Cleanup (to be performed)

- `DROP DATABASE sigap_reconcile_scratch;` and `DROP DATABASE
  sigap_reconcile_scratch13;` (disposable scratch DBs, no longer needed).
- Remove temporary `design/_scratch/` artifacts (live.sql, fresh.sql, fresh13.sql,
  *_nr.sql, *.diff, *.txt, *.cat, catalog.sql, schema_catalog.sql, *.err).