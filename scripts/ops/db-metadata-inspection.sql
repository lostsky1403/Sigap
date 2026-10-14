-- SIGAP — production DB metadata inspection (READ-ONLY, METADATA ONLY)
-- =====================================================================
-- STATUS: EXECUTED. Read-only, once, on 2026-10-09 against fikriserver/sigap
--   (PostgreSQL 16.15), with explicit operator authorization. Result:
--   OLDER_WEAKER_CONSTRAINTS. The hash of the revision that ran was
--   657eb6754f2e00b824a3e47db67787e066b0d153daa30d0291544382a1174a6c
--   (blob 36959d81370f772a0b6c4b0b345a325cbc693343, 17052 bytes), verified on
--   the host before execution. THIS revision differs from that one: the
--   applied-version inventory was split into a known set (1..11) and a
--   mandatory set (1..10), so a database that has had 0011 applied is no longer
--   reported as drift. The recorded classification is unaffected. Re-run per
--   docs/operations/DEPLOYMENT_RUNBOOK.md §12a before the cutover so the
--   recorded hash and the shipped artifact agree.
--
--   Re-running requires read-only production access and its own authorization
--   (see docs/operations/DEPLOYMENT_RUNBOOK.md §12/§12a).
--
-- PURPOSE
--   Determine whether the deployed schema carries main's STRENGTHENED 0006
--   CHECK constraints (commit 9d4e68e) or an earlier, weaker definition.
--
--   Why this matters: 9d4e68e EDITED an already-shipped migration file
--   (packages/db/migrations/0006_notifications.sql). The migration runner
--   (apps/api/internal/migrate/migrate.go) selects pending work by VERSION
--   number ONLY and never re-verifies the recorded checksum, so a database
--   that applied 0006 before 9d4e68e keeps the weaker constraints forever.
--   The API's application-layer masking (apps/api/internal/notification/masking.go)
--   SHIPS WITH the release and, ONCE DEPLOYED, is the effective control either
--   way; whether it is actually running in production is NOT established here.
--
-- SAFETY
--   * SELECT-only. No INSERT/UPDATE/DELETE, no DDL, no ALTER, no migration.
--   * Reads pg_catalog metadata and the schema_migrations bookkeeping table.
--   * NEVER selects from patient/appointment/notification_outbox DATA rows.
--   * Wrapped in a read-only transaction with a statement timeout.
--
-- PROPOSED INVOCATION (NOT AUTHORIZED, NOT RUN): see runbook §12a.
--
-- CLASSIFIER DESIGN (why it is written this way)
--   Four earlier designs were rejected because each can report a false PASS:
--     (a) a bare fragment test `LIKE '%{10,}%'` counts a DRIFTED predicate such
--         as `subject !~ '[0-9]{10,}'` as "strengthened" => false MATCHES;
--     (b) a residue test that strips the column identifiers cannot tell that
--         `..._subject_chk` guards `body_template`, leaving `subject` unguarded
--         while still reporting MATCHES => false PASS on a copy-paste error;
--     (c) matching on the two phone constraints alone ignores the rest of the
--         table contract, so a dropped structural constraint or an unexpected
--         column/constraint would still report MATCHES;
--     (d) counting "residue empty AND contains {10,}" as strengthened. The
--         `{10,}` conjunct ALONE also strips to an empty residue, yet it is
--         strictly WEAKER than the release predicate: it admits an 8-digit raw
--         run that `!~ '[0-9]{8,}'` rejects => false MATCHES on a live
--         raw-phone insert path. This is why n_strong below requires BOTH
--         conjuncts, not merely the `{10,}` one.
--   This version (1) binds each expected predicate to ITS OWN column, and
--   (2) compares the FULL constraint inventory, the FULL column inventory and
--   the FULL applied-version set against the release's own definitions. Any
--   deviation is UNEXPECTED_DRIFT, never MATCHES.
--
--   `[[:space:]]` is used rather than `\s`, and every literal compared below is
--   backslash-free: pg_get_constraintdef prints the regex text with a backslash
--   escape, and `replace(def, chr(92), '')` removes it from BOTH sides before the
--   comparison. Using chr(92) rather than a backslash literal is what makes the
--   result independent of the server's standard_conforming_strings setting.
--
--   A "strengthened" predicate must carry BOTH conjuncts. The `{10,}` conjunct
--   alone is STRICTLY WEAKER than the release definition (it admits an 8-digit
--   raw run that `!~ '[0-9]{8,}'` rejects), so residue-empty + `{10,}` is not
--   sufficient — the `[0-9]{8,}` conjunct is required as well.
--
-- EXPECTED VALUES FOR COMPARISON (release source at HEAD e8c7096)
--   0006 sha256 (LF bytes, as committed) = 1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119
--   pre-9d4e68e 0006 sha256 (LF bytes)  = d9be0f160952a0f9913638327e7bc617b24f50dc6e582e85b26e3b0e6375b51a
--
-- 0011_notification_outbox_phone_constraints.sql converges the two phone
-- constraints onto the release predicate on databases that applied 0006 before
-- 9d4e68e. It PRESERVES both constraint names, so the expected constraint
-- inventory below is unchanged; only the applied-version set moves to 1..11.
--
-- READINESS, BEFORE AND AFTER
--   OLDER_WEAKER_CONSTRAINTS  the pre-0011 state: run 0011 to converge.
--   MATCHES_CURRENT_SECURITY_CONSTRAINTS
--                             the post-0011 state, and also the state of a
--                             database built from HEAD. Both mean "schema
--                             ready"; the applied-version set distinguishes
--                             them, so this classifier does not assert which
--                             one produced it.
--
-- MAINTENANCE: the expected inventories below are transcribed from
--   packages/db/migrations/0006_notifications.sql (constraints, columns) and
--   the set of migration files 0001..0011 (versions). Adding a migration, a
--   column or a constraint REQUIRES updating them, or every later inspection
--   will report UNEXPECTED_DRIFT.

BEGIN READ ONLY;
SET LOCAL statement_timeout = '15s';

-- 1. Which migration versions are recorded as applied?
SELECT version, applied_at
FROM schema_migrations
ORDER BY version;

-- 2. Stored checksum for 0006 (hex). Diagnostic only: the runner hashes
--    os.ReadFile bytes, so a CRLF working tree will not match the server value.
SELECT version, encode(checksum, 'hex') AS checksum_hex
FROM schema_migrations
WHERE version = 6;

-- 3. AUTHORITATIVE: full definitions of every CHECK constraint on
--    notification_outbox. Query 4b is derived from this text; read both.
SELECT c.conname AS constraint_name,
       pg_get_constraintdef(c.oid) AS definition
FROM pg_constraint c
JOIN pg_class t     ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relname = 'notification_outbox'
  AND c.contype = 'c'
ORDER BY c.conname;

-- 4. Supporting counts (diagnostic detail behind query 4b).
WITH rel AS (
  SELECT 'notification_outbox'::regclass AS oid
),
expected_constraint(name) AS (
  VALUES ('notification_outbox_channel_chk'),
         ('notification_outbox_recipient_type_chk'),
         ('notification_outbox_status_chk'),
         ('notification_outbox_attempt_count_chk'),
         ('notification_outbox_hash_len_chk'),
         ('notification_outbox_subject_chk'),
         ('notification_outbox_body_chk'),
         ('notification_outbox_masked_chk'),
         ('notification_outbox_no_raw_phone_in_subject_chk'),
         ('notification_outbox_no_raw_phone_in_body_chk')
),
expected_column(name) AS (
  VALUES ('id'), ('facility_id'), ('channel'), ('template_key'), ('subject'),
         ('body_template'), ('recipient_type'), ('recipient_contact_masked'),
         ('recipient_contact_hash'), ('status'), ('attempt_count'),
         ('next_attempt_at'), ('last_error_code'), ('related_resource_type'),
         ('related_resource_id'), ('created_at'), ('updated_at')
),
expected_version(version) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10),(11)
),
-- Versions that MUST be present. This is 1..10, not 1..11, and the difference is
-- load-bearing: 0011 converges a drifted database, and whether version 11 is
-- *recorded* depends on HOW it was applied. The runner inserts the row; a plain
-- `psql -f 0011_...sql` does not. Both outcomes are legitimate end states, so
-- requiring 11 would report a correctly-remediated database as drift. Version 11
-- remains in expected_version above, which still catches a version OUTSIDE the
-- known set and still catches a missing version <= 10.
required_version(version) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10)
),
phone AS (
  SELECT c.conname,
         pg_get_constraintdef(c.oid) AS def,
         CASE WHEN c.conname = 'notification_outbox_no_raw_phone_in_subject_chk'
              THEN 'subject' ELSE 'body_template' END AS col
  FROM pg_constraint c, rel
  WHERE c.conrelid = rel.oid
    AND c.contype = 'c'
    AND c.conname IN ('notification_outbox_no_raw_phone_in_subject_chk',
                      'notification_outbox_no_raw_phone_in_body_chk')
),
norm AS (
  SELECT conname, def,
         coalesce(regexp_replace(
           replace(
             replace(
               replace(def, chr(92), ''),
               col || ' !~ ''[0-9][0-9-._() ]{10,}[0-9]''', ''),
             col || ' !~ ''[0-9]{8,}''', ''),
           '[[:space:]()]|CHECK|::text|AND', '', 'g'), '') AS residue
  FROM phone
),
actual_constraint(name) AS (
  SELECT c.conname FROM pg_constraint c, rel WHERE c.conrelid = rel.oid AND c.contype = 'c'
),
actual_column(name) AS (
  SELECT a.attname FROM pg_attribute a, rel
  WHERE a.attrelid = rel.oid AND a.attnum > 0 AND NOT a.attisdropped
)
SELECT
  (SELECT count(*) FROM actual_constraint)                             AS check_constraints,
  (SELECT count(*) FROM norm)                                          AS phone_constraints,
  -- Both conjuncts required, AND the separator class must still carry its
  -- backslash escape. The normalization above strips chr(92) from BOTH sides so
  -- the comparison does not depend on standard_conforming_strings — but that
  -- also makes the INVALID predicate '[0-9-._() ]' (an invalid character range
  -- that raises "invalid character range" on every insert) look identical to the
  -- valid '[0-9\-._() ]'. The raw definition is therefore checked for the
  -- escaped class here. Without this, a functionally broken constraint is
  -- reported as MATCHES_CURRENT_SECURITY_CONSTRAINTS.
  (SELECT count(*) FROM norm
    WHERE residue = '' AND def LIKE '%[0-9]{8,}%' AND def LIKE '%{10,}%'
      AND strpos(replace(def, chr(92) || chr(92), chr(92)), '[0-9' || chr(92) || '-._() ]') > 0)              AS strengthened_form,
  (SELECT count(*) FROM norm
    WHERE residue = '' AND def LIKE '%[0-9]{8,}%' AND def NOT LIKE '%{10,}%') AS weak_form,
  (SELECT count(*) FROM norm
    WHERE NOT ((residue = '' AND def LIKE '%[0-9]{8,}%' AND def LIKE '%{10,}%'
                AND strpos(replace(def, chr(92) || chr(92), chr(92)), '[0-9' || chr(92) || '-._() ]') > 0)
            OR (residue = '' AND def LIKE '%[0-9]{8,}%' AND def NOT LIKE '%{10,}%')))
                                                                       AS unrecognized_form,
  (SELECT count(*) FROM actual_constraint a
    WHERE NOT EXISTS (SELECT 1 FROM expected_constraint e WHERE e.name = a.name)) AS unexpected_constraints,
  (SELECT count(*) FROM expected_constraint e
    WHERE NOT EXISTS (SELECT 1 FROM actual_constraint a WHERE a.name = e.name))   AS missing_constraints,
  (SELECT count(*) FROM actual_column a
    WHERE NOT EXISTS (SELECT 1 FROM expected_column e WHERE e.name = a.name))     AS unexpected_columns,
  (SELECT count(*) FROM expected_column e
    WHERE NOT EXISTS (SELECT 1 FROM actual_column a WHERE a.name = e.name))       AS missing_columns,
  (SELECT count(*) FROM schema_migrations s
    WHERE NOT EXISTS (SELECT 1 FROM expected_version e WHERE e.version = s.version)) AS unexpected_versions,
  (SELECT count(*) FROM required_version e
    WHERE NOT EXISTS (SELECT 1 FROM schema_migrations s WHERE s.version = e.version)) AS missing_versions;

-- 4b. SINGLE-ROW CLASSIFICATION. Read this value.
--     UNEXPECTED_DRIFT is a STOP: do not classify further, do not deploy.
--     Any deviation of the constraint inventory, the column inventory or the
--     applied-version set forces UNEXPECTED_DRIFT, so a weaker or unrecognized
--     schema can never be reported as MATCHES_CURRENT_SECURITY_CONSTRAINTS.
WITH rel AS (
  SELECT 'notification_outbox'::regclass AS oid
),
expected_constraint(name) AS (
  VALUES ('notification_outbox_channel_chk'),
         ('notification_outbox_recipient_type_chk'),
         ('notification_outbox_status_chk'),
         ('notification_outbox_attempt_count_chk'),
         ('notification_outbox_hash_len_chk'),
         ('notification_outbox_subject_chk'),
         ('notification_outbox_body_chk'),
         ('notification_outbox_masked_chk'),
         ('notification_outbox_no_raw_phone_in_subject_chk'),
         ('notification_outbox_no_raw_phone_in_body_chk')
),
expected_column(name) AS (
  VALUES ('id'), ('facility_id'), ('channel'), ('template_key'), ('subject'),
         ('body_template'), ('recipient_type'), ('recipient_contact_masked'),
         ('recipient_contact_hash'), ('status'), ('attempt_count'),
         ('next_attempt_at'), ('last_error_code'), ('related_resource_type'),
         ('related_resource_id'), ('created_at'), ('updated_at')
),
expected_version(version) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10),(11)
),
-- Versions that MUST be present. This is 1..10, not 1..11, and the difference is
-- load-bearing: 0011 converges a drifted database, and whether version 11 is
-- *recorded* depends on HOW it was applied. The runner inserts the row; a plain
-- `psql -f 0011_...sql` does not. Both outcomes are legitimate end states, so
-- requiring 11 would report a correctly-remediated database as drift. Version 11
-- remains in expected_version above, which still catches a version OUTSIDE the
-- known set and still catches a missing version <= 10.
required_version(version) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(10)
),
phone AS (
  SELECT c.conname,
         pg_get_constraintdef(c.oid) AS def,
         CASE WHEN c.conname = 'notification_outbox_no_raw_phone_in_subject_chk'
              THEN 'subject' ELSE 'body_template' END AS col
  FROM pg_constraint c, rel
  WHERE c.conrelid = rel.oid
    AND c.contype = 'c'
    AND c.conname IN ('notification_outbox_no_raw_phone_in_subject_chk',
                      'notification_outbox_no_raw_phone_in_body_chk')
),
norm AS (
  SELECT conname, def,
         coalesce(regexp_replace(
           replace(
             replace(
               replace(def, chr(92), ''),
               col || ' !~ ''[0-9][0-9-._() ]{10,}[0-9]''', ''),
             col || ' !~ ''[0-9]{8,}''', ''),
           '[[:space:]()]|CHECK|::text|AND', '', 'g'), '') AS residue
  FROM phone
),
actual_constraint(name) AS (
  SELECT c.conname FROM pg_constraint c, rel WHERE c.conrelid = rel.oid AND c.contype = 'c'
),
actual_column(name) AS (
  SELECT a.attname FROM pg_attribute a, rel
  WHERE a.attrelid = rel.oid AND a.attnum > 0 AND NOT a.attisdropped
),
counts AS (
  SELECT
    (SELECT count(*) FROM norm)                                                 AS n_phone,
    -- "Strengthened" requires BOTH conjuncts AND the escaped separator class.
    -- `residue = ''` alone is not enough: a predicate carrying only the `{10,}`
    -- conjunct also strips to an empty residue, yet it is strictly weaker than
    -- the release definition. The escape check is the third requirement: after
    -- the backslash is stripped from both sides, the INVALID predicate
    -- '[0-9-._() ]' (an invalid character range that raises on every insert) is
    -- textually identical to the valid '[0-9\-._() ]', so the raw definition
    -- must be inspected for the escape.
    (SELECT count(*) FROM norm
      WHERE residue = ''
        AND def LIKE '%[0-9]{8,}%'
        AND def LIKE '%{10,}%'
        AND strpos(replace(def, chr(92) || chr(92), chr(92)), '[0-9' || chr(92) || '-._() ]') > 0)                     AS n_strong,
    (SELECT count(*) FROM norm
      WHERE residue = ''
        AND def LIKE '%[0-9]{8,}%'
        AND def NOT LIKE '%{10,}%')                                              AS n_weak,
    -- Anything that is neither the strengthened form nor the weak form: a
    -- non-empty residue, a missing weak conjunct, or an unescaped (broken)
    -- separator class.
    (SELECT count(*) FROM norm
      WHERE NOT ((residue = '' AND def LIKE '%[0-9]{8,}%' AND def LIKE '%{10,}%'
                  AND strpos(replace(def, chr(92) || chr(92), chr(92)), '[0-9' || chr(92) || '-._() ]') > 0)
              OR (residue = '' AND def LIKE '%[0-9]{8,}%' AND def NOT LIKE '%{10,}%')))
                                                                                 AS n_badform,
    -- Structural constraint inventory: the two phone names are excluded here and
    -- judged by n_phone/n_strong/n_weak/n_badform instead, so that a merely
    -- absent phone denylist is reported as MISSING_CONSTRAINTS rather than
    -- masked as drift.
    (SELECT count(*) FROM actual_constraint a
      WHERE a.name NOT IN ('notification_outbox_no_raw_phone_in_subject_chk',
                           'notification_outbox_no_raw_phone_in_body_chk')
        AND NOT EXISTS (SELECT 1 FROM expected_constraint e WHERE e.name = a.name))
      + (SELECT count(*) FROM expected_constraint e
      WHERE e.name NOT IN ('notification_outbox_no_raw_phone_in_subject_chk',
                           'notification_outbox_no_raw_phone_in_body_chk')
        AND NOT EXISTS (SELECT 1 FROM actual_constraint a WHERE a.name = e.name)) AS n_inventory,
    (SELECT count(*) FROM actual_column a
      WHERE NOT EXISTS (SELECT 1 FROM expected_column e WHERE e.name = a.name))
      + (SELECT count(*) FROM expected_column e
      WHERE NOT EXISTS (SELECT 1 FROM actual_column a WHERE a.name = e.name))     AS n_columns,
    (SELECT count(*) FROM schema_migrations s
      WHERE NOT EXISTS (SELECT 1 FROM expected_version e WHERE e.version = s.version))
      + (SELECT count(*) FROM required_version e
      WHERE NOT EXISTS (SELECT 1 FROM schema_migrations s WHERE s.version = e.version)) AS n_versions
)
SELECT CASE
  -- Any deviation from the release contract is drift, never MATCHES.
  WHEN n_inventory > 0 OR n_columns > 0 OR n_versions > 0 OR n_badform > 0
       THEN 'UNEXPECTED_DRIFT'
  -- Phone denylist absent/incomplete: name that finding specifically.
  WHEN n_phone < 2 THEN 'MISSING_CONSTRAINTS'
  WHEN n_strong = 2 THEN 'MATCHES_CURRENT_SECURITY_CONSTRAINTS'
  WHEN n_weak = 2 THEN 'OLDER_WEAKER_CONSTRAINTS'
  -- Mixed forms (one column hardened, the other not) and any other residual.
  ELSE 'UNEXPECTED_DRIFT'
END AS classification
FROM counts;

-- 5. Column inventory (names/types only).
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = 'notification_outbox'
ORDER BY ordinal_position;

-- 6. Presence of the expected phone constraint NAMES.
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
-- INTERPRETATION — record exactly ONE class
-- =====================================================================
--
-- MATCHES_CURRENT_SECURITY_CONSTRAINTS
--   Both phone constraints present and matching the strengthened form exactly
--   FOR THEIR OWN COLUMN; the full 10-constraint inventory, the full 17-column
--   inventory and the applied-version set 1..10 all match the release.
--   => No action.
--
-- OLDER_WEAKER_CONSTRAINTS
--   Both phone constraints present and both matching the pre-9d4e68e weak form
--   (`<col> !~ '[0-9]{8,}'` only); everything else matches. DB-layer
--   defence-in-depth is weaker than the release source. Application-layer
--   masking ships with the release and, once deployed, enforces the denylist;
--   its live presence is NOT established here. Operator decision:
--   (a) ACCEPT with a recorded compensating control, or
--   (b) REMEDIATE FORWARD with a NEW migration that DROPs and re-ADDs both
--       constraints. NEVER edit 0006_notifications.sql again — the runner is
--       version-only, so an edit to 0006 would never reapply.
--
-- MISSING_CONSTRAINTS
--   Fewer than two phone constraints exist. Security finding: do not deploy
--   until remediated forward or explicitly accepted in writing.
--
-- UNEXPECTED_DRIFT
--   Any of: an unexpected or missing constraint; an unexpected or missing
--   column; a recorded version outside the release set, or a release version
--   not recorded; a phone constraint whose definition is neither known form
--   (including a mis-bound column, an `OR`, or a superficially similar
--   predicate). => STOP. Do not classify further, do not deploy. Escalate.
--
-- UNKNOWN
--   Inspection could not be completed (no authorization, connection failure,
--   timeout, partial output, or a non-zero psql exit — e.g. the table or the
--   tracking table does not exist). This is the DEFAULT and it is BLOCKING:
--   DATABASE_SCHEMA_COMPATIBILITY stays UNKNOWN and no schema-compatibility
--   claim may be made. "Unknown" is never a PASS.
