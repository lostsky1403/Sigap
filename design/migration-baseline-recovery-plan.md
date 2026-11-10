# Migration Baseline Recovery Plan

> **STATUS: PROPOSED — NOT EXECUTED.**
> This document converts the findings of
> [`migration-ledger-reconciliation.md`](./migration-ledger-reconciliation.md) into an
> auditable, non-destructive recovery procedure. Nothing here has been run against the
> live `sigap` database. No `schema_migrations` row has been written. The ledger has NOT
> been mutated (and MUST NOT be mutated until the owner approves this plan).
>
> **R3 (2026-11-10):** the reconciliation ledger has been extended through
> `0014_patient_account.sql` (disk) with the verified live span at **`0001–0013`** —
> live is byte-identical to a fresh tracked-runner build through 0013, and `0014` is a
> pending migration (file present, objects absent from live). This plan's verified
> backfill span is therefore `0001–0013`; `0014` is conditional (§2.4).

---

## 0. Owner approval gate (BLOCKS any execution)

Every mutating step below (§2 backfill, §4 rollback) is gated on **owner sign-off**:

- [ ] Owner confirms the live inventory/checksum facts derived in this doc and in the
      reconciliation ledger.
- [ ] Owner confirms the **branch divergence gate** (§0.1) — whether `0001–0013` only or
      `0001–0014` is the baseline to record.
- [ ] Owner records the approval (who, when, ticket reference) in the ledger's change
      note **after** execution, never before.

Until that happens this is a **design proposal**, and the correct state of the live
ledger remains "empty `schema_migrations` (13-tracked-installed schema, ledger-gap)".


### 0.1 Branch divergence gate (must be resolved before the ledger write)

The reconciliation ledger (`migration-ledger-reconciliation.md`) is scoped to
**`0001–0013`** (VERIFIED, R3) — 13 migration files, live schema proven byte-identical to
a fresh tracked-runner build through 0013, ledger empty. That is the **VERIFIED**
baseline.

This feature branch (`feat/sigap-regional-pilot`) additionally contains:

| version | file | status relative to the reconciliation |
|---------|------|------------------------------------------|
| 14 | `0014_patient_account.sql` | **NOT part of the verified reconciliation** — pending; live was never re-diffed against it (objects absent) |

`0014_patient_account.sql` adds one nullable FK column `appointments.patient_account_id`
(`REFERENCES app_users(id) ON DELETE SET NULL`), the index
`idx_appointments_patient_account_id`, and a column comment; it is forward-only over
`0001–0013`. Its checksum is recorded in §2.5 for completeness, but its `(14, checksum)`
row is a **PROPOSED extension, not a verified reconciliation result**:

- The live `appointments.patient_account_id` does **not** exist (verified R3), so
  `0014` is a **pending** migration. Backfilling row 14 must NOT happen until §1's diff
  has (a) confirmed live still equals the `0001–0013` schema and (b) confirmed `0014` is
  either already present in live or deliberately out of the baseline.
- If the owner wants a **13-row baseline** (matches the ledger verbatim), backfill §2
  applies to versions `1–13` only, and `0014` remains a normal pending migration that a
  later approved `migrate.Run` will apply under its own transaction.

The classification "differences = `none`" inherited from the prior `READY` result applies
to the **verified `0001–0013` span only**. No classification is asserted for `0014` here.

---

## 1. Disposable-DB diff procedure (build from the tracked runner)

Re-proves "live schema == what the tracked migrator produces" before any ledger write, and
serves as the re-run gate every time a recovery is contemplated. This is non-destructive:
it uses a throwaway database and never connects the migrator to live.

### 1.1 Build a tracked scratch DB

The reference schema MUST come from the **tracked Go migrator** (not `psql` over the raw
`.sql` files), so it is byte-for-byte the schema the running application produces. The
tracked entrypoints are:

- `make db-migrate-all` → `cd apps/api && SIGAP_AUTO_MIGRATE=true go run ./cmd/server`
- `go run ./cmd/ci-migrate` (reads `DATABASE_URL`, calls `migrate.Run`)
- `migrate.Run(ctx, pool, migrate.MigrateDir())` in-process.

Procedure:

```sh
export SCRATCH_DBURL="postgres://<user>@127.0.0.1:5433/sigap_reconcile_scratch?sslmode=disable"

# 1. Create a fresh disposable database (do NOT reuse the reconciliation scratch unless re-created empty).
psql "postgres://<user>@127.0.0.1:5433/postgres" -c "DROP DATABASE IF EXISTS sigap_reconcile_scratch;"
psql "postgres://<user>@127.0.0.1:5433/postgres" -c "CREATE DATABASE sigap_reconcile_scratch;"

# 2. Run the tracked migration chain over it (not psql -f over 0001).
DATABASE_URL="$SCRATCH_DBURL" go run ./apps/api/cmd/ci-migrate
#   expect: "Applied 14 migrations" (all files on disk incl. 0014) — the runner applies
#   every pending version; the *live* span is then established by the §1.3 diff.
```

The runner creates `schema_migrations`, applies each migration inside its own transaction,
and records `(version, checksum)` rows — so the scratch DB's **application** schema is the
reference. (The scratch ledger will be populated; that is expected and is deliberately
excluded from the app-object diff in §1.4.)

For an exact live-span comparison, ALSO build a `scratch13` DB: run the same chain, then
strip the 0014 objects (`DROP INDEX idx_appointments_patient_account_id; ALTER TABLE
appointments DROP COLUMN patient_account_id;`) so the pure `0001–0013` runner schema can
be diffed byte-for-byte against live (R3 used this shape).

### 1.2 Record each migration file's real SHA-256 (byte-for-byte)

Independent of the ledger's copied values, re-derive from the actual tracked bytes so the
baseline is auditable, not transcribed:

```sh
cd packages/db/migrations
sha256sum 0001_init.sql 0002_medical_records.sql 0003_identity_rbac.sql \
 0004_audit_events.sql 0005_appointments.sql 0006_notifications.sql \
 0007_checkin_constraints.sql 0008_identity_subject.sql 0009_user_roles_lifecycle.sql \
 0010_demo_notes_column.sql 0011_notification_outbox_phone_constraints.sql \
 0012_service_day_wib.sql 0013_organizations.sql 0014_patient_account.sql
```

Acceptance: the 13 `0001–0013` values MUST equal the table in §2.5 (and the ledger),
and `0014` must re-hash to its recorded value. Any mismatch HALTS the plan — a file
changed under our feet invalidates the "no drift" conclusion and requires a fresh
reconciliation before any ledger write.

### 1.3 Diff scratch (runner-built) vs live

Use both independent methods from the reconciliation (they independently agreed). Diff
**live against `scratch13`** (the pure `0001–0013` runner schema) for the byte-identity
proof; diff live against the full scratch (14) only to confirm the delta is exactly the
0014 additions:

```sh
# A. pg_dump structural diff (live vs scratch13)
pg_dump --schema-only --no-owner --no-privileges --no-comments --schema=public \
  "$SCRATCH13_DBURL" > scratch13.sql
LIVE_DBURL="postgres://<user>@127.0.0.1:5433/sigap?sslmode=disable"
pg_dump --schema-only --no-owner --no-privileges --no-comments --schema=public \
  "$LIVE_DBURL" > live.sql
diff <(grep -v '\\restrict' <(grep -v '\\unrestrict' scratch13.sql)) \
     <(grep -v '\\restrict' <(grep -v '\\unrestrict' live.sql))
```

Both dumps MUST be byte-identical after stripping pg_dump 18's per-database
`\restrict`/`\unrestrict` session-encryption tokens (§3.2 of the ledger).

```text
# B. Structured catalog diff (objects, not restrict tokens)
-- compare relname / attname / typedef / default / constraintdef / indexdef /
-- enum labels / function def for live vs scratch13; MUST return zero rows.
```

Expected result (R3): **zero differences**. A diff against the full 14-chain scratch must
show exactly the 0014 objects (column `patient_account_id`, index
`idx_appointments_patient_account_id`, FK `appointments_patient_account_id_fkey`) and
nothing else.

### 1.4 Expected inventory (acceptance whenever §1 is re-run)

Scoped to `public`, application schema (the 19 tracked tables through 0013;
`schema_migrations` is the 20th and is deliberately excluded from the app-object diff
because its row-set is the subject of §2, not a schema object):

| Object | Expected count (live, per reconciliation R3) |
|--------|--------------------------------------------|
| application tables | 19 (incl. `organizations`) |
| indexes | 63 |
| constraints | 75 |
| functions | 1 → `appointment_day(timestamptz) RETURNS date IMMUTABLE PARALLEL SAFE` |
| `notification_outbox` CHECK constraints | 10 |
| `appointment_day` body | WIB immutable: `SELECT (($1 AT TIME ZONE 'UTC') + INTERVAL '7 hours')::date` |

If `0014` is in the diff span, expand by its objects consistently on both sides (column
`patient_account_id` + index `idx_appointments_patient_account_id` + FK
`appointments_patient_account_id_fkey`; never diff a 14-side against a 13-side). A
mismatch in **any** row HALTS the plan.

### 1.5 Seed files are out of scope (unchanged from the ledger)

`seed/rbac.sql`, `seed/dev.sql`, `seed/demo.sql`, `seed/regional_tenancy.sql` are data-only
and not tracked by `schema_migrations`. This plan does not compare seed row-data live vs
scratch; a data-parity check is a separate, already-flagged follow-up and does not gate
the schema-ledger baseline.

### 1.6 Cleanup (do this every run, even on HALT)

```sh
psql "postgres://<user>@127.0.0.1:5433/postgres" -c "DROP DATABASE IF EXISTS sigap_reconcile_scratch;"
# remove any temporary scratch.sql / live.sql / *.cat / catalog artifacts under design/_scratch/
```

---

## 2. Checksum baseline + ledger backfill (PROPOSED — needs §0 approval)

Because live schema is proven identical to the tracked chain but its
`schema_migrations` is **empty**, the only safe baseline is a **ledger-only backfill**:
record the checksums for what is already on disk; NEVER re-execute DDL.

### 2.1 Why a backfill and not `migrate.Run`

The runner selects pending work by **version only** and never re-reads a stored checksum
(`appliedVersions()` reads `version`; `Run()` skips `if applied[m.Version]`). With the
empty ledger it would treat all migrations as pending and re-apply `0001_init.sql`, which
fails on the duplicate `CREATE TYPE facility_type` with `type "facility_type" already
exists` — rolled back, ledger still empty. The backfill is the only safe path.

### 2.2 Precondition (must be re-verified, then hold)

§1's diff is **clean** for the backfill span, and the file bytes re-hash to §2.5.

### 2.3 Idempotent backfill SQL (non-destructive; no DDL)

`schema_migrations` is `version INTEGER PRIMARY KEY, checksum BYTEA NOT NULL, applied_at
TIMESTAMPTZ NOT NULL DEFAULT now()`. `ON CONFLICT (version) DO NOTHING` makes this
idempotent and safe to re-run. Execute once inside a transaction against live, only with
owner approval:

```sql
BEGIN;

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

COMMIT;

-- immediately assert the count:
SELECT count(*) FROM schema_migrations;  -- MUST equal 13 (the VERIFIED span)
```

### 2.4 `0014` row — conditional, owner-decided (NOT part of the verified span)

Do NOT insert row 14 until §0.1 is resolved and §1 has diffed the `0014` span. If approved
(e.g. live already carries `appointments.patient_account_id`), append the row **inside the
same transaction** as 2.3 with this single additional value:

```sql
 (14, decode('001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624','hex')),
```

If the owner elects a **13-row baseline**, omit row 14 entirely and leave `0014` pending
for a later approved `migrate.Run`.

### 2.5 Real file-byte SHA-256 registry (re-derived in §1.2; matches the ledger for 1–13)

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
| 13 | `0013_organizations.sql` | `6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1` | verified (R3 live-diffed) |
| 14 | `0014_patient_account.sql` | `001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624` | pending/proposed |

### 2.6 Post-backfill no-op verification

After the approved backfill, the tracked runner must become a **no-op** over the recorded
span:

```sh
DATABASE_URL="$LIVE_DBURL" go run ./apps/api/cmd/ci-migrate
#   expect: "all migrations up to date", "Applied 0 migrations"
```

`migrate.Status` over the same span must report `applied = 13`, `pending = 1`
(`0014`). (If a 14-row baseline was approved and 0014 is applied, expect `applied = 14`,
`pending = 0`.) Any non-zero apply or any DDL log line HALTS — treat as "ledger write did
not make the runner idempotent" and do not widen the ledger.

---

## 3. Safety gate (MUST be enforced in every tool path)

The invariant this plan exists to protect: **never re-apply `0001` (or any already-installed
migration) over an existing schema.**

3.1 **Root hazard.** `0001_init.sql` contains non-idempotent DDL (`CREATE TYPE
facility_type` etc.). A `migrate.Run` against a populated database with an empty/partial
ledger re-runs it and fails with `type "facility_type" already exists`. The duplicate-CREATE-TYPE
failure is the canary — treat it as "ledger gap", never "just drop the type".

3.2 **Gate rules (apply to operators AND tooling):**
- The only sanctioned writes to `schema_migrations` on an existing schema are
  **`INSERT ... ON CONFLICT (version) DO NOTHING`** (§2.3) or the runner's own
  `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)` inside the same
  transaction as a genuinely-new migration.
- Before any `migrate.Run`, require `SELECT count(*) FROM schema_migrations;` to be
  consistent with the schema (empty ledger ⇒ stopped for §0/§1, never auto-forwarded).
- Never issue `CREATE TYPE`/`CREATE TABLE` "fixups" by hand to paper over a failed 0001.
- If `appliedVersions()` can be empty while live objects exist, the migrator MUST be
  treated as **unsafe** until the ledger is backfilled (§2) or a
  version-aware `migrate.VerifyChecksums()` guard exists (ledger §7, BLOCKED — a separate
  code change, not part of this plan).

3.3 **Known runner caveats (must not be mistaken for drift):**
- `0006_notifications.sql` carries its own `BEGIN/COMMIT`, ending the runner's
  outer transaction early for that one migration (documented in its header). It affects
  only that migration's atomicity, not the final schema.
- The runner checks **version membership only**; it does not compare a stored checksum to
  file bytes. Backfill makes it idempotent today; it does **not** give future tamper
  detection. That gap remains an explicitly **BLOCKED** recommendation in the ledger (§7),
  not an open item in this recovery plan.

---

## 4. Rollback (non-destructive by design)

4.1 **The backfill is additive and reversible.** The only ledger-write mutation in this
plan is inserting rows with `ON CONFLICT DO NOTHING`. Rolling back the baseline is a
**single tombstoning DELETE**, never any DDL:

```sql
-- Only with owner approval, and only if the schema is still otherwise unchanged.
BEGIN;
DELETE FROM schema_migrations WHERE version IN (1,2,3,4,5,6,7,8,9,10,11,12,13);  -- and 14 if it was backfilled
-- assert nothing else was touched:
SELECT count(*) FROM schema_migrations;  -- MUST return 0 (or only pre-existing rows)
COMMIT;
```

4.2 **No live schema is ever dropped.** There is no `DROP TABLE`/`DROP TYPE`/`DROP
COLUMN` anywhere in this plan. Rollback returns the database to its pre-recovery state
(schema intact, ledger empty or previously-backed-off) without touching application
objects.

4.3 **Scratch cleanup (§1.6) is the only destructive step** and is confined to
`sigap_reconcile_scratch` + `design/_scratch/` artifacts — never to `sigap`.

4.4 **If a backfill row is written with an already-stale checksum** (a migration file
changed between §1.2 and §2.3), do NOT delete-and-reinsert blind; stop, re-run §1, and only
then correct the single row with a targeted `UPDATE ... SET checksum = ... WHERE version = n`
covered by a fresh owner approval.

---

## 5. Execution checklist (all PROPOSED except §0/§1 which are read-only)

| Step | Section | Mutates live? | State |
|------|---------|---------------|-------|
| Owner approves plan + span decision | §0 | no | PROPOSED |
| Build scratch via tracked runner | §1.1 | no (scratch only) | PROPOSED |
| Re-hash each `NNNN_*.sql` file | §1.2 | no | PROPOSED (values re-derived & match ledger for 1–13; 14 recorded pending) |
| Diff scratch vs live (2 methods) | §1.3/1.4 | no | PROPOSED |
| Backfill `schema_migrations` 1–13 (and 14 if approved) | §2.3/2.4 | **yes — ledger only** | PROPOSED, gated |
| Verify `migrate.Run` no-op / `migrate.Status` | §2.6 | no | PROPOSED, gated |
| Rollback (if needed) | §4 | **yes — ledger only** | PROPOSED, gated |
| Scratch + artifact cleanup | §1.6 | no (scratch/artifacts only) | PROPOSED |

---

## 6. Classification of differences (inherited from the prior READY result)

- **Differences between live and a fresh tracked-runner schema (0001–0013 span):**
  **`none`** — zero catalog differences and a byte-identical pg_dump (after stripping the
  per-database `\restrict`/`\unrestrict` session tokens). This is inherited unchanged from
  the reconciliation ledger's `READY` finding and re-proven in R3.
- The **only** divergence on-disk is the **empty `schema_migrations` ledger** (a tracking
  gap), which §2 closes by recording — not re-executing — the already-installed schema.
- `0014_patient_account.sql` is **not classified** by this baseline; it is a pending branch
  migration whose span must be re-diffed before any row 14 is recorded (§0.1, §2.4).