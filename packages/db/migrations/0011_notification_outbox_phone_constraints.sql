-- 0011_notification_outbox_phone_constraints.sql
-- Forward-only remediation of the confirmed production constraint drift on
-- notification_outbox.
--
-- WHY THIS EXISTS
--   Migration 0006 created the notification_outbox phone denylist. Commit
--   9d4e68e LATER strengthened both predicates inside that same, already-shipped
--   file. The runner (apps/api/internal/migrate/migrate.go) selects pending work
--   by VERSION number only and never re-reads the recorded checksum, so a
--   database that applied 0006 before 9d4e68e keeps the weaker predicate
--   forever. A read-only production inspection on 2026-10-09 confirmed exactly
--   that state: PostgreSQL 16.15, migrations 0001-0010 applied, the expected 17
--   columns and 10 CHECK constraints present, and both phone constraints in the
--   older weak form (classification OLDER_WEAKER_CONSTRAINTS).
--
-- WHAT IT DOES
--   Converges the live definitions of the two phone constraints onto the
--   release predicate, whatever the starting point, and FAILS CLOSED on any
--   state it does not recognise.
--
--     both constraints already strengthened  -> no-op
--     both constraints in the weak form      -> DROP and re-ADD strengthened
--     both constraints absent                -> ADD strengthened
--     anything else (mixed, NOT VALID,
--     unrecognised predicate)                -> RAISE, change nothing
--
-- DESIGN CONSTRAINTS (deliberate, do not "simplify" these away)
--   * NO BEGIN/COMMIT in this file. The runner already wraps each migration in
--     its own transaction, so the DDL and the schema_migrations version row
--     commit atomically. 0006 carries its own BEGIN/COMMIT and is the only
--     migration that does; a nested COMMIT there ends the runner's transaction
--     early. This file must not repeat that.
--   * Constraint NAMES are preserved exactly. The release inventory of 10 CHECK
--     constraints on notification_outbox is therefore unchanged, so the
--     metadata classifier's expected inventory needs no name change and the
--     negative controls it relies on keep working.
--   * Idempotent. A database created from HEAD already carries the strengthened
--     predicate (0006 at HEAD is already strengthened), so this migration is a
--     no-op there. Re-running it by hand is also a no-op.
--   * NO existing row is rewritten or deleted. If any existing row violates the
--     strengthened predicate, ADD CONSTRAINT fails, the transaction aborts and
--     the database is left exactly as it was. Remediating such rows is an
--     operator decision and is deliberately NOT attempted here.
--   * Predicate comparison is CANONICAL, not textual. The expected constraint
--     is materialised on a TEMP table and compared through
--     pg_get_constraintdef, so both sides are rendered by the same PostgreSQL
--     code path in the same session. A literal string comparison would be
--     wrong: pg_get_constraintdef renders the regex backslash differently
--     depending on standard_conforming_strings, and appends " NOT VALID" to an
--     unvalidated constraint.
--   * A NOT VALID final state is never accepted. An unvalidated constraint has
--     not been checked against existing rows, so it must not be read as
--     hardened. This migration always ADDs with validation.
--   * The migrator is NOT bypassed and no checksum row is touched. This file
--     ships as a normal tracked migration; production SIGAP_AUTO_MIGRATE stays
--     unset and this migration is not applied by this change.
--
-- NOT IN SCOPE
--   * No application code change. The API's application-layer masking
--     (apps/api/internal/notification/masking.go) ships with the release and is
--     the effective control either way; this migration restores the database
--     backstop only.
--   * No change to any other table, constraint, column or index.

DO $sigap_0011$
DECLARE
    c_subject constant text := 'notification_outbox_no_raw_phone_in_subject_chk';
    c_body    constant text := 'notification_outbox_no_raw_phone_in_body_chk';

    v_exp_subject text;
    v_exp_body    text;
    v_weak_subject text;
    v_weak_body   text;

    v_act_subject text;
    v_act_body    text;
    v_has_subject boolean := false;
    v_has_body    boolean := false;
    v_other_type  integer;
BEGIN
    -- Absence of the table means this database is not in a state this migration
    -- understands. Fail closed rather than silently no-op.
    IF to_regclass('notification_outbox') IS NULL THEN
        RAISE EXCEPTION
            '0011: table notification_outbox is absent; refusing to guess at the schema state';
    END IF;

    -- Canonical renderings, produced by PostgreSQL itself so that no assumption
    -- is made about quoting, casting or parenthesisation.
    --
    -- The regex literals are DOLLAR-QUOTED, not single-quoted, and that is
    -- load-bearing. Under standard_conforming_strings=off a plain
    -- '...\-...' literal has its backslash consumed by the string-literal
    -- escape, so the server parses '[0-9][0-9-._() ]{10,}[0-9]' -- in which
    -- '9-.' is an invalid character range. The constraint is then created with
    -- a predicate that RAISES on every insert. Because the expected text below
    -- would be built from the same degraded literal, a self-referential
    -- comparison would call it "already strengthened" and silently no-op.
    -- Dollar quoting is immune to standard_conforming_strings, so the intended
    -- regex is what both the expectation and the real constraint are built from.
    CREATE TEMP TABLE sigap_0011_expected (
        subject       text,
        body_template text,
        CONSTRAINT sigap_0011_expected_subject_chk
            CHECK (subject !~ $re$[0-9]{8,}$re$ AND subject !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$),
        CONSTRAINT sigap_0011_expected_body_chk
            CHECK (body_template !~ $re$[0-9]{8,}$re$ AND body_template !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$)
    ) ON COMMIT PRESERVE ROWS;

    CREATE TEMP TABLE sigap_0011_weak (
        subject       text,
        body_template text,
        CONSTRAINT sigap_0011_weak_subject_chk CHECK (subject !~ '[0-9]{8,}'),
        CONSTRAINT sigap_0011_weak_body_chk    CHECK (body_template !~ '[0-9]{8,}')
    ) ON COMMIT PRESERVE ROWS;

    SELECT pg_get_constraintdef(oid) INTO v_exp_subject
      FROM pg_constraint
     WHERE conrelid = 'sigap_0011_expected'::regclass
       AND conname  = 'sigap_0011_expected_subject_chk';
    SELECT pg_get_constraintdef(oid) INTO v_exp_body
      FROM pg_constraint
     WHERE conrelid = 'sigap_0011_expected'::regclass
       AND conname  = 'sigap_0011_expected_body_chk';
    SELECT pg_get_constraintdef(oid) INTO v_weak_subject
      FROM pg_constraint
     WHERE conrelid = 'sigap_0011_weak'::regclass
       AND conname  = 'sigap_0011_weak_subject_chk';
    SELECT pg_get_constraintdef(oid) INTO v_weak_body
      FROM pg_constraint
     WHERE conrelid = 'sigap_0011_weak'::regclass
       AND conname  = 'sigap_0011_weak_body_chk';

    -- Current state of the two constraints on the real table.
    SELECT pg_get_constraintdef(oid) INTO v_act_subject
      FROM pg_constraint
     WHERE conrelid = 'notification_outbox'::regclass
       AND conname  = c_subject
       AND contype  = 'c';
    v_has_subject := FOUND;

    SELECT pg_get_constraintdef(oid) INTO v_act_body
      FROM pg_constraint
     WHERE conrelid = 'notification_outbox'::regclass
       AND conname  = c_body
       AND contype  = 'c';
    v_has_body := FOUND;

    -- A constraint carrying one of the target names but NOT of type CHECK
    -- (PG 18 allows named NOT NULL constraints, contype 'n'). The state queries
    -- above filter on contype='c', so such a constraint is invisible to them --
    -- yet DROP CONSTRAINT is type-agnostic and would remove it, silently
    -- dropping a constraint this migration has no business touching. Refuse.
    SELECT count(*) INTO v_other_type
      FROM pg_constraint
     WHERE conrelid = 'notification_outbox'::regclass
       AND conname IN (c_subject, c_body)
       AND contype <> 'c';
    IF v_other_type > 0 THEN
        RAISE EXCEPTION
            '0011: a constraint named % or % exists on notification_outbox but is not a CHECK constraint; refusing to drop it',
            c_subject, c_body;
    END IF;

    -- Already converged: nothing to do. This is the path taken on a database
    -- built from HEAD, where 0006 already creates the strengthened predicate.
    IF v_has_subject AND v_has_body
       AND v_act_subject = v_exp_subject
       AND v_act_body    = v_exp_body THEN
        RAISE NOTICE '0011: both phone constraints already strengthened; no change';
        DROP TABLE sigap_0011_expected;
        DROP TABLE sigap_0011_weak;
        RETURN;
    END IF;

    -- Known drifted state: the pre-9d4e68e weak form on both columns.
    IF v_has_subject AND v_has_body
       AND v_act_subject = v_weak_subject
       AND v_act_body    = v_weak_body THEN
        RAISE NOTICE '0011: weak phone constraints detected; replacing with the release predicate';

    -- Denylist entirely absent. Adding it is a strict improvement and matches
    -- the classifier''s MISSING_CONSTRAINTS finding.
    ELSIF NOT v_has_subject AND NOT v_has_body THEN
        RAISE NOTICE '0011: phone constraints absent; adding the release predicate';

    -- Anything else: a half-present denylist, an unrecognised predicate, or a
    -- NOT VALID (unvalidated) constraint. Do not guess. Change nothing.
    ELSE
        RAISE EXCEPTION
            '0011: unrecognised phone-constraint state (subject present=%, subject matches release=%, subject matches weak=%, body present=%, body matches release=%, body matches weak=%); refusing to modify the schema',
            v_has_subject, v_has_subject AND v_act_subject = v_exp_subject,
            v_has_subject AND v_act_subject = v_weak_subject,
            v_has_body, v_has_body AND v_act_body = v_exp_body,
            v_has_body AND v_act_body = v_weak_body;
    END IF;

    DROP TABLE sigap_0011_expected;
    DROP TABLE sigap_0011_weak;

    -- Converge. DROP ... IF EXISTS covers both the "weak" and the "absent"
    -- paths. ADD validates existing rows; if any row violates the strengthened
    -- predicate this raises, the runner rolls the transaction back, and the
    -- database is left untouched.
    --
    -- Dollar-quoted regex literals again: a plain '...\-...' literal would be
    -- mangled under standard_conforming_strings=off and would install a
    -- predicate that raises on every insert (see the note above).
    ALTER TABLE notification_outbox
        DROP CONSTRAINT IF EXISTS notification_outbox_no_raw_phone_in_subject_chk;
    ALTER TABLE notification_outbox
        DROP CONSTRAINT IF EXISTS notification_outbox_no_raw_phone_in_body_chk;

    ALTER TABLE notification_outbox
        ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk
        CHECK (subject !~ $re$[0-9]{8,}$re$ AND subject !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$);
    ALTER TABLE notification_outbox
        ADD CONSTRAINT notification_outbox_no_raw_phone_in_body_chk
        CHECK (body_template !~ $re$[0-9]{8,}$re$ AND body_template !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$);

    RAISE NOTICE '0011: phone constraints converged onto the release predicate';
END
$sigap_0011$;
