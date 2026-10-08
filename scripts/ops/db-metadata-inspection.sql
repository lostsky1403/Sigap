-- SIGAP — production DB metadata inspection (READ-ONLY, METADATA ONLY)
-- =====================================================================
-- STATUS: PLANNED. NOT AUTHORIZED. NOT EXECUTED.
--   Running this requires an explicit operator authorization for read-only
--   database access (see docs/operations/DEPLOYMENT_RUNBOOK.md §12).
--
-- PURPOSE
--   Determine whether the deployed schema carries main's STRENGTHENED 0006
--   CHECK constraints (commit 9d4e68e) or the earlier, weaker definition.
--
--   Why this matters: 9d4e68e EDITED an already-shipped migration file
--   (packages/db/migrations/0006_notifications.sql). The migration runner
--   (apps/api/internal/migrate/migrate.go) selects pending work by VERSION
--   number ONLY and never re-verifies the recorded checksum, so a database
--   that applied 0006 before 9d4e68e keeps the weaker constraints forever.
--   The API's application-layer masking (apps/api/internal/notification/masking.go)
--   SHIPS WITH the release and, ONCE DEPLOYED, is the effective control either
--   way; whether it is actually running in production is NOT established here
--   (the running API image has no proven source revision). This inspection only
--   establishes which DB-layer definition is live.
--
-- SAFETY
--   * SELECT-only. No INSERT/UPDATE/DELETE, no DDL, no ALTER, no migration,
--     no seed.
--   * Reads pg_catalog metadata and the schema_migrations bookkeeping table.
--   * NEVER selects from patient/appointment/notification_outbox DATA rows.
--   * Wrapped in a read-only transaction with a statement timeout.
--
-- PROPOSED INVOCATION (NOT AUTHORIZED, NOT RUN):
--   docker exec -i sigap-postgres psql -U sigap -d sigap \
--     -v ON_ERROR_STOP=1 --no-psqlrc -P pager=off -f - < db-metadata-inspection.sql
--
-- EXPECTED VALUES FOR COMPARISON (from the release source at HEAD 824a9a5)
--   0006 sha256 (LF bytes, as committed) = 1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119
--   pre-9d4e68e 0006 sha256 (LF bytes)  = d9be0f160952a0f9913638327e7bc617b24f50dc6e582e85b26e3b0e6375b51a
--   Strengthened CHECK adds the second predicate:
--     AND <col> !~ '[0-9][0-9\-._() ]{10,}[0-9]'
--   => a STRENGTHENED deployment contains the literal  {10,}
--      a WEAK deployment does not.
--
-- NOTE ON THE STORED CHECKSUM
--   The runner hashes os.ReadFile(path) bytes. On Windows the working tree
--   stores CRLF, so a local sha256 of the file will NOT equal the server-side
--   value. Compare only for EQUALITY-ACROSS-ENVIRONMENTS, or compute the LF
--   form. The authoritative constraint test is query 4.

BEGIN READ ONLY;
SET LOCAL statement_timeout = '15s';

-- 1. Which migration versions are recorded as applied?
--    Establishes that 0006 was applied, and whether any recorded version is
--    absent from the release source (unexpected divergence).
SELECT version, applied_at
FROM schema_migrations
ORDER BY version;

-- 2. Stored checksum for 0006 (hex). Diagnostic only; see the note above.
SELECT version, encode(checksum, 'hex') AS checksum_hex
FROM schema_migrations
WHERE version = 6;

-- 3. DECISIVE: full definitions of every CHECK constraint on notification_outbox.
--    Look for the second predicate containing  {10,}  in the two
--    notification_outbox_no_raw_phone_in_{subject,body}_chk constraints.
SELECT c.conname AS constraint_name,
       pg_get_constraintdef(c.oid) AS definition
FROM pg_constraint c
JOIN pg_class t     ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relname = 'notification_outbox'
  AND c.contype = 'c'
ORDER BY c.conname;

-- 4. Explicit strengthened-vs-weak classifier (single boolean, easy to audit).
SELECT
  count(*) FILTER (WHERE pg_get_constraintdef(oid) LIKE '%{10,}%') AS strengthened_constraints,
  count(*) FILTER (WHERE conname IN (
      'notification_outbox_no_raw_phone_in_subject_chk',
      'notification_outbox_no_raw_phone_in_body_chk')) AS expected_phone_constraints
FROM pg_constraint
WHERE conrelid = 'notification_outbox'::regclass
  AND contype = 'c';

-- 5. Column inventory (names/types only) — confirms the shape the release expects.
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'notification_outbox'
ORDER BY ordinal_position;

-- 6. Presence of the expected constraint NAMES.
SELECT conname
FROM pg_constraint
WHERE conname IN (
  'notification_outbox_no_raw_phone_in_subject_chk',
  'notification_outbox_no_raw_phone_in_body_chk',
  'notification_outbox_masked_chk'
)
ORDER BY conname;

-- 7. Context identifiers only.
SELECT current_database() AS db,
       current_schema()   AS schema,
       current_setting('server_version') AS pg_version;

ROLLBACK;

-- =====================================================================
-- INTERPRETATION — classification enum (record exactly one in the checklist)
-- =====================================================================
--
-- MATCHES_CURRENT_SECURITY_CONSTRAINTS
--   strengthened_constraints = 2, both expected phone constraint names present,
--   and every recorded schema_migrations version exists in the release source.
--   => DB carries 9d4e68e's strengthened checks. No action.
--
-- OLDER_WEAKER_CONSTRAINTS
--   strengthened_constraints = 0 while the two expected phone constraint names
--   ARE present (the pre-9d4e68e definitions).
--   => DB-layer defence-in-depth is weaker than the release source.
--      Application-layer masking (masking.go) ships with the release and, once
--      deployed, enforces the denylist; its live presence is NOT established by
--      this inspection. Requires an explicit operator decision:
--        (a) ACCEPT — record the residual and the compensating control, or
--        (b) REMEDIATE FORWARD — ship a NEW migration (e.g. 0011) that DROP and
--            re-ADD both constraints. NEVER edit 0006_notifications.sql again:
--            the runner is version-only, so an edit to 0006 would never reapply.
--
-- MISSING_CONSTRAINTS
--   Either expected phone constraint name is ABSENT.
--   => The DB has no phone-denylist CHECK at all. Treat as a security finding:
--      do not deploy until remediated forward, or until an operator explicitly
--      accepts the application-layer-only control in writing.
--
-- UNEXPECTED_DRIFT
--   Any of: a recorded schema_migrations version absent from the release source;
--   a constraint definition that is neither the strengthened nor the known weak
--   form; a NOT NULL / type change against query 5; extra unexpected CHECKs.
--   => STOP. Do not classify, do not deploy. Escalate for review.
--
-- UNKNOWN
--   Inspection could not be completed (no authorization, connection failure,
--   timeout, or partial output).
--   => This is the DEFAULT state and it is BLOCKING: DATABASE_SCHEMA_COMPATIBILITY
--      stays UNKNOWN and no schema-compatibility claim may be made. "Unknown"
--      is never a PASS.
--
-- NOTE: the classifier in query 4 counts the literal  {10,}  in the constraint
-- definition. Do not rely on that single fragment alone — confirm against the
-- full definitions returned by query 3, which are authoritative.
