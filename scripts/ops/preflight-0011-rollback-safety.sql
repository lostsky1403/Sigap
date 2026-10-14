-- SIGAP — pre-flight for migration 0011: rollback safety and applicability
-- =====================================================================
-- READ-ONLY. SELECT-only against pg_catalog, facilities and notification_outbox.
-- No INSERT/UPDATE/DELETE, no DDL, no migration. Wrapped in a read-only
-- transaction with a statement timeout, so the server itself refuses any write.
--
-- WHY THIS EXISTS
--   Migration 0011 converges the two notification_outbox phone CHECK
--   constraints onto the RELEASE predicate:
--
--     col !~ $re$[0-9]{8,}$re$ AND col !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
--
--   i.e. a value is REJECTED when it MATCHES either conjunct. The release
--   predicate is STRICTLY STRICTER than the weak form it replaces (the weak form
--   has only the first conjunct). Two consequences follow, and neither is
--   visible from the schema alone:
--
--   (1) APPLICABILITY. 0011 ADDs the constraint WITH validation, so it FAILS
--       CLOSED if any EXISTING notification_outbox row violates the release
--       predicate. The migration then aborts and the database is left exactly
--       as it was. Section 2 answers "will 0011 apply?" before an operator
--       attempts it.
--
--   (2) ROLLBACK SAFETY. An application rollback restores the PREVIOUSLY
--       RUNNING (older) release image. That image's write path was written
--       against the WEAK predicate, so it can legitimately attempt a write the
--       strengthened constraint refuses. The only runtime-varying value the old
--       write path puts into a guarded column is the CHECK-IN body, which it
--       builds by CONCATENATION as
--
--         "Check-in Anda berhasil. Nomor antrean: " || queue_number || "."
--
--       where queue_number is the queue engine's `{short_code}-{NNNN}`
--       (apps/queue-engine/src/engine/queue.rs: format!("{}-{:04}", ...)).
--       A facility whose short_code renders a separator-run queue number
--       therefore makes EVERY check-in notification for that facility fail the
--       CHECK constraint. The failure is swallowed by the fire-and-forget
--       goroutine in booking.go, so the notification is lost silently.
--       Section 3 answers "does such a facility exist?".
--
--       The NEW release cannot create such a facility — validateShortCode
--       (apps/api/internal/handler/admin.go) rejects a short_code whose
--       rendered queue number fails ContainsRawPhoneDigits — but it does NOT
--       retroactively validate facilities created by the old release. A
--       pre-existing unsafe short_code therefore breaks notifications on the
--       new release TOO, and is not a rollback-only problem.
--
--   Section 3 is therefore the decision input for BOTH the migration
--   authorization and the rollback plan. A non-empty result is an operator
--   decision (correct the short_code, or accept write refusal for that
--   facility), NOT a reason to weaken the constraint.
--
-- WHAT THIS FILE DOES NOT DO
--   * It does not authorize or apply 0011. Passing this check is NOT
--     authorization.
--   * It does not read or return patient/contact content. Section 2 returns
--     counts and primary keys only; no subject/body text leaves the database.
--   * It does not remediate anything. Correcting a short_code is a data
--     mutation and is deliberately left to the operator.
--
-- PROVENANCE OF THE PREDICATE BELOW
--   The regex literals are DOLLAR-QUOTED so they are immune to
--   standard_conforming_strings (the reason 0006's plain-quoted literal is
--   mangled under SCS=off; see 0011's header). They are copied verbatim from
--   packages/db/migrations/0011_notification_outbox_phone_constraints.sql.
--   Section 1 prints the DEPLOYED definitions so the operator can see which
--   state the database is actually in and confirm that the predicate this file
--   applies is the one 0011 installs.

\pset pager off
\set ON_ERROR_STOP on

BEGIN;
SET TRANSACTION READ ONLY;
SET LOCAL statement_timeout = '60s';

\echo ''
\echo '=== 1. deployed phone-constraint state (informational) ==='
\echo '    canonical = pg_get_constraintdef with backslashes stripped, so the'
\echo '    rendering is independent of standard_conforming_strings.'

SELECT c.conname,
       c.convalidated,
       replace(pg_get_constraintdef(c.oid), chr(92), '') AS canonical_def
FROM pg_constraint c
WHERE c.conrelid = 'notification_outbox'::regclass
  AND c.conname IN ('notification_outbox_no_raw_phone_in_subject_chk',
                    'notification_outbox_no_raw_phone_in_body_chk')
  AND c.contype = 'c'
ORDER BY c.conname;

\echo ''
\echo '=== 2. 0011 APPLICABILITY: existing rows REJECTED by the release predicate ==='
\echo '    Non-zero count => 0011 ADD CONSTRAINT will FAIL CLOSED and abort.'
\echo '    Such rows were accepted by the weak predicate; 0011 does not touch them.'

SELECT count(*) AS violating_outbox_rows
FROM notification_outbox
WHERE body_template ~ $re$[0-9]{8,}$re$
   OR body_template ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
   OR subject       ~ $re$[0-9]{8,}$re$
   OR subject       ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$;

-- Sample of offending primary keys only. No subject/body text is returned, so
-- this cannot surface message content (or a phone number) in a log.
SELECT id, created_at
FROM notification_outbox
WHERE body_template ~ $re$[0-9]{8,}$re$
   OR body_template ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
   OR subject       ~ $re$[0-9]{8,}$re$
   OR subject       ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
ORDER BY created_at
LIMIT 20;

\echo ''
\echo '=== 3. ROLLBACK SAFETY: facilities whose rendered queue number is REJECTED ==='
\echo '    Rendered form is {short_code}-{NNNN} (engine format!("{}-{:04}")).'
\echo '    The predicate depends only on the digit/separator run, so any four-digit'
\echo '    counter is an equivalent witness; -0300 is the engine''s daily cap and is'
\echo '    the same worst case validateShortCode uses.'
\echo '    A row here => that facility''s check-in notifications are REFUSED by the'
\echo '    strengthened constraint, on BOTH the old and the new release, and are'
\echo '    lost silently (fire-and-forget enqueue in booking.go).'

SELECT f.id,
       f.short_code,
       f.short_code || '-0300' AS rendered_queue_number,
       f.is_active
FROM facilities f
WHERE (f.short_code || '-0300') ~ $re$[0-9]{8,}$re$
   OR (f.short_code || '-0300') ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
ORDER BY f.short_code, f.id;

\echo ''
\echo '=== 4. verdict ==='

WITH violations AS (
  SELECT count(*) AS n
  FROM notification_outbox
  WHERE body_template ~ $re$[0-9]{8,}$re$
     OR body_template ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
     OR subject       ~ $re$[0-9]{8,}$re$
     OR subject       ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
),
unsafe AS (
  SELECT count(*) AS n
  FROM facilities f
  WHERE (f.short_code || '-0300') ~ $re$[0-9]{8,}$re$
     OR (f.short_code || '-0300') ~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$
)
SELECT CASE
         WHEN v.n = 0 AND u.n = 0 THEN 'PREFLIGHT_OK'
         ELSE 'PREFLIGHT_BLOCKED'
       END AS verdict,
       v.n AS violating_outbox_rows,
       u.n AS unsafe_facilities,
       CASE
         WHEN v.n > 0 THEN '0011 will FAIL CLOSED on these existing rows; remediate or accept a no-apply.'
         WHEN u.n > 0 THEN 'check-in notifications are silently lost for these facilities on old AND new release.'
         ELSE 'no existing row blocks 0011, and no facility renders an unsafe queue number.'
       END AS detail
FROM violations v, unsafe u;

COMMIT;
