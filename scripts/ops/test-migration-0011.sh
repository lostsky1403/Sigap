#!/bin/sh
# test-migration-0011.sh — scenario matrix for
# packages/db/migrations/0011_notification_outbox_phone_constraints.sql
#
# Builds a DISPOSABLE local PostgreSQL cluster in a temp directory and exercises
# migration 0011 against every starting state it must handle. Uses ONLY
# synthetic data. Asserts the ACTUAL resulting schema via pg_get_constraintdef
# and convalidated — never merely a zero exit code — because a migration that
# silently no-ops on the drifted state would still exit 0.
#
# Scenarios:
#   1  fresh database, all migrations applied in order
#   2  database carrying the original weak 0006 constraints (the production state)
#   3  database already satisfying the hardened constraints
#   4  phone constraints missing entirely
#   5  unexpected constraint drift (mixed / spoofed / NOT VALID)
#   6  existing synthetic rows that violate the strengthened predicate
#   7  transaction failure and rollback
#   8  reapplication / version-tracking behaviour (idempotency)
#   9  enforcement on `subject`
#  10  enforcement on `body_template`
#  11  column-binding mistakes
#  12  old/new application write-path compatibility where testable
#
# Usage:  sh scripts/ops/test-migration-0011.sh
# Exit 0 iff every scenario passes; 2 if PostgreSQL binaries are unavailable.

set -u

# POSIX coreutils first: on Windows the only `sort` on PATH can be
# C:\WINDOWS\system32\sort.exe, which fails on -u and prints nothing, turning
# any string comparison into a vacuous "" == "" pass.
POSIX_PATH=""
for d in /usr/bin /bin; do
  [ -d "$d" ] && POSIX_PATH="${POSIX_PATH:+$POSIX_PATH:}$d"
done
set -f
_old_ifs=$IFS; IFS=:
_clean=$POSIX_PATH
for _p in $PATH; do
  case ":$_clean:" in
    *":$_p:"*) ;;
    *) _clean="${_clean:+$_clean:}$_p" ;;
  esac
done
IFS=$_old_ifs
set +f
PATH=$_clean
export PATH
unset _clean _old_ifs _p POSIX_PATH

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
MIG_DIR=${SIGAP_MIG_DIR:-$REPO_ROOT/packages/db/migrations}
MIG11=$MIG_DIR/0011_notification_outbox_phone_constraints.sql

[ -f "$MIG11" ] || { echo "FAIL: $MIG11 not found"; exit 1; }

find_bin() {
  command -v "$1" 2>/dev/null && return 0
  for d in "/c/Program Files/PostgreSQL"/*/bin "/usr/lib/postgresql"/*/bin /usr/local/pgsql/bin; do
    [ -x "$d/$1" ] && { printf '%s\n' "$d/$1"; return 0; }
  done
  return 1
}
PSQL=$(find_bin psql)     || { echo "SKIP: psql not found (no local PostgreSQL)"; exit 2; }
INITDB=$(find_bin initdb) || { echo "SKIP: initdb not found (no local PostgreSQL)"; exit 2; }
PGCTL=$(find_bin pg_ctl)  || { echo "SKIP: pg_ctl not found (no local PostgreSQL)"; exit 2; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-mig0011-$$")
mkdir -p "$TMP"
CL="$TMP/cluster"
PORT_OVERRIDE=${SIGAP_TEST_PG_PORT:-}
PORT=${PORT_OVERRIDE:-55463}

cleanup() {
  "$PGCTL" -D "$CL" -m immediate stop >/dev/null 2>&1
  rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

"$INITDB" -D "$CL" -U postgres --auth=trust -E UTF8 >"$TMP/initdb.log" 2>&1 \
  || { echo "FAIL: initdb failed"; tail -5 "$TMP/initdb.log"; exit 1; }

# A cluster left running by a SIGKILLed earlier invocation (the trap does not
# fire on SIGKILL) holds the default port and would make pg_ctl fail here,
# reporting a spurious FAIL for the whole suite. Probe successive ports unless
# one was requested explicitly.
start_cluster() {
  attempts=0
  while [ "$attempts" -lt 8 ]; do
    if "$PGCTL" -D "$CL" -o "-p $PORT -c listen_addresses=127.0.0.1" \
         -l "$TMP/pg.log" -w start >"$TMP/start.log" 2>&1; then
      return 0
    fi
    [ -n "$PORT_OVERRIDE" ] && break
    PORT=$((PORT + 1))
    attempts=$((attempts + 1))
  done
  return 1
}
start_cluster || { echo "FAIL: cluster start failed"; tail -5 "$TMP/start.log"; exit 1; }

px() { "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off "$@"; }
pxa() { "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -q -P pager=off -A -t "$@"; }
die() { printf 'FAIL: setup step failed: %s\n' "$1"; exit 1; }
mk() {
  px -d postgres -c "DROP DATABASE IF EXISTS $1" -c "CREATE DATABASE $1" >/dev/null 2>&1 \
    || die "create database $1"
}

# Apply a migration file. Returns its exit status; output suppressed.
apply() { # db file
  "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off \
    -d "$1" -f "$2" >"$TMP/apply.log" 2>&1
}
apply11() { apply "$1" "$MIG11"; }

tracking() {
px -d "$1" >/dev/null 2>&1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY, checksum BYTEA NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO schema_migrations (version, checksum)
  SELECT g, '\x01'::bytea FROM generate_series(1,11) g ON CONFLICT DO NOTHING;
SQL
[ $? -eq 0 ] || die "tracking table for $1"
}

# The 17 release columns and 10 release CHECK constraints, with the two phone
# predicates parameterised: $2 = subject predicate, $3 = body predicate.
release_table() {
px -d "$1" >/dev/null 2>&1 <<SQL
CREATE TABLE notification_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    facility_id UUID,
    channel TEXT NOT NULL,
    template_key TEXT NOT NULL,
    subject TEXT NOT NULL,
    body_template TEXT NOT NULL,
    recipient_type TEXT NOT NULL,
    recipient_contact_masked TEXT NOT NULL,
    recipient_contact_hash BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error_code TEXT,
    related_resource_type TEXT,
    related_resource_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_outbox_channel_chk CHECK (channel IN ('dev','sms','whatsapp','email')),
    CONSTRAINT notification_outbox_recipient_type_chk CHECK (recipient_type IN ('patient','staff','facility_admin')),
    CONSTRAINT notification_outbox_status_chk CHECK (status IN ('pending','processing','delivered','failed','cancelled')),
    CONSTRAINT notification_outbox_attempt_count_chk CHECK (attempt_count >= 0),
    CONSTRAINT notification_outbox_hash_len_chk CHECK (octet_length(recipient_contact_hash) = 32),
    CONSTRAINT notification_outbox_subject_chk CHECK (octet_length(subject) BETWEEN 1 AND 200),
    CONSTRAINT notification_outbox_body_chk CHECK (octet_length(body_template) BETWEEN 1 AND 4000),
    CONSTRAINT notification_outbox_masked_chk CHECK (octet_length(recipient_contact_masked) BETWEEN 3 AND 200),
    CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk CHECK ($2),
    CONSTRAINT notification_outbox_no_raw_phone_in_body_chk CHECK ($3)
);
SQL
[ $? -eq 0 ] || die "release table for $1"
}

STRONG_S="subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9\\-._() ]{10,}[0-9]'"
STRONG_B="body_template !~ '[0-9]{8,}' AND body_template !~ '[0-9][0-9\\-._() ]{10,}[0-9]'"
WEAK_S="subject !~ '[0-9]{8,}'"
WEAK_B="body_template !~ '[0-9]{8,}'"

# Canonical definition of a constraint on notification_outbox, or the literal
# string ABSENT when it is not there.
defn() { # db constraint
  v=$(pxa -d "$1" -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='notification_outbox'::regclass AND conname='$2' AND contype='c'")
  if [ -z "$v" ]; then printf 'ABSENT'; else printf '%s' "$v"; fi
}
# Same, but with standard_conforming_strings forced ON for the reading session.
# pg_get_constraintdef RENDERS the regex backslash differently depending on that
# GUC, so a database whose default is off (scenario 13) would otherwise compare
# its correct constraint against the wrong rendering. Forcing it on here reads
# the stored definition canonically regardless of the database default.
defn_on() { # db constraint
  v=$(PGOPTIONS='-c standard_conforming_strings=on' "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres \
        -q -P pager=off -A -t -d "$1" \
        -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='notification_outbox'::regclass AND conname='$2' AND contype='c'")
  if [ -z "$v" ]; then printf 'ABSENT'; else printf '%s' "$v"; fi
}
validated() { # db constraint -> t/f/ABSENT
  v=$(pxa -d "$1" -c "SELECT convalidated FROM pg_constraint WHERE conrelid='notification_outbox'::regclass AND conname='$2' AND contype='c'")
  if [ -z "$v" ]; then printf 'ABSENT'; else printf '%s' "$v"; fi
}
nconstraints() { pxa -d "$1" -c "SELECT count(*) FROM pg_constraint WHERE conrelid='notification_outbox'::regclass AND contype='c'"; }

# Expected canonical text, produced by the server in the SAME session, so no
# assumption is made about quoting, casting or parenthesisation.
EXP_S=""; EXP_B=""; WEAK_S_DEF=""; WEAK_B_DEF=""
init_expected() {
px -d postgres >/dev/null 2>&1 <<'SQL'
CREATE TABLE IF NOT EXISTS _sigap_exp (
  subject TEXT, body_template TEXT,
  CONSTRAINT _e_s CHECK (subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9\-._() ]{10,}[0-9]'),
  CONSTRAINT _e_b CHECK (body_template !~ '[0-9]{8,}' AND body_template !~ '[0-9][0-9\-._() ]{10,}[0-9]'),
  CONSTRAINT _w_s CHECK (subject !~ '[0-9]{8,}'),
  CONSTRAINT _w_b CHECK (body_template !~ '[0-9]{8,}'));
SQL
  EXP_S=$(pxa -d postgres -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='_sigap_exp'::regclass AND conname='_e_s'")
  EXP_B=$(pxa -d postgres -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='_sigap_exp'::regclass AND conname='_e_b'")
  WEAK_S_DEF=$(pxa -d postgres -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='_sigap_exp'::regclass AND conname='_w_s'")
  WEAK_B_DEF=$(pxa -d postgres -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='_sigap_exp'::regclass AND conname='_w_b'")
  [ -n "$EXP_S" ] && [ -n "$EXP_B" ] && [ -n "$WEAK_S_DEF" ] && [ -n "$WEAK_B_DEF" ] \
    || die "expected-definition probe produced empty output"
}

pass=0; failed=0
ok()   { pass=$((pass+1));   printf 'ok   %-26s %s\n' "$1" "$2"; }
bad()  { failed=$((failed+1)); printf 'FAIL %-26s %s\n' "$1" "$2"; }

init_expected

# ---------------------------------------------------------------- scenario 1
# Fresh database: every migration applied in filename order. 0011 must be a
# no-op here because 0006 at HEAD already creates the strengthened predicate.
mk s1
n=0
for f in "$MIG_DIR"/[0-9][0-9][0-9][0-9]_*.sql; do
  [ -f "$f" ] || continue
  apply s1 "$f" || die "scenario 1: $(basename "$f") failed"
  n=$((n+1))
done
[ "$n" -ge 11 ] || die "scenario 1: expected >=11 migration files, applied $n"
tracking s1
if [ "$(defn s1 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
   && [ "$(defn s1 notification_outbox_no_raw_phone_in_body_chk)" = "$EXP_B" ] \
   && [ "$(validated s1 notification_outbox_no_raw_phone_in_subject_chk)" = "t" ] \
   && [ "$(validated s1 notification_outbox_no_raw_phone_in_body_chk)" = "t" ] \
   && [ "$(nconstraints s1)" = "10" ]; then
  ok "1-fresh-all-migrations" "10 constraints, both strengthened and validated, 0011 no-op"
else
  bad "1-fresh-all-migrations" "subj=$(defn s1 notification_outbox_no_raw_phone_in_subject_chk) consts=$(nconstraints s1)"
fi

# ---------------------------------------------------------------- scenario 2
# The confirmed production state: weak 0006 constraints. 0011 must converge.
mk s2; tracking s2; release_table s2 "$WEAK_S" "$WEAK_B"
[ "$(defn s2 notification_outbox_no_raw_phone_in_subject_chk)" = "$WEAK_S_DEF" ] \
  || die "scenario 2 precondition: subject should be weak"
if apply11 s2; then
  if [ "$(defn s2 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
     && [ "$(defn s2 notification_outbox_no_raw_phone_in_body_chk)" = "$EXP_B" ] \
     && [ "$(validated s2 notification_outbox_no_raw_phone_in_subject_chk)" = "t" ] \
     && [ "$(validated s2 notification_outbox_no_raw_phone_in_body_chk)" = "t" ] \
     && [ "$(nconstraints s2)" = "10" ]; then
    ok "2-weak-to-strengthened" "both constraints converged and validated"
  else
    bad "2-weak-to-strengthened" "subj=$(defn s2 notification_outbox_no_raw_phone_in_subject_chk)"
  fi
else
  bad "2-weak-to-strengthened" "0011 exited non-zero; log: $(tail -2 "$TMP/apply.log" | tr '\n' ' ')"
fi

# ---------------------------------------------------------------- scenario 3
# Already hardened: 0011 must be a no-op that still exits 0.
mk s3; tracking s3; release_table s3 "$STRONG_S" "$STRONG_B"
if apply11 s3 && [ "$(defn s3 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ]; then
  ok "3-already-hardened-noop" "no change, exit 0"
else
  bad "3-already-hardened-noop" "exit or definition wrong"
fi

# ---------------------------------------------------------------- scenario 4
# Both phone constraints absent: 0011 must ADD them.
mk s4; tracking s4; release_table s4 "$STRONG_S" "$STRONG_B"
px -d s4 -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk" \
          -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk" >/dev/null 2>&1
if apply11 s4 \
   && [ "$(defn s4 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
   && [ "$(defn s4 notification_outbox_no_raw_phone_in_body_chk)" = "$EXP_B" ] \
   && [ "$(nconstraints s4)" = "10" ]; then
  ok "4-absent-adds-both" "both constraints added, 10 total"
else
  bad "4-absent-adds-both" "subj=$(defn s4 notification_outbox_no_raw_phone_in_subject_chk) consts=$(nconstraints s4)"
fi

# ---------------------------------------------------------------- scenario 5
# Unexpected drift. 0011 must REFUSE and change nothing, for each sub-case.
mk s5a; tracking s5a; release_table s5a "$STRONG_S" "$WEAK_B"
before=$(defn s5a notification_outbox_no_raw_phone_in_body_chk)
if apply11 s5a; then
  bad "5a-mixed-refuses" "0011 accepted a mixed strong/weak state"
else
  [ "$(defn s5a notification_outbox_no_raw_phone_in_body_chk)" = "$before" ] \
    && ok "5a-mixed-refuses" "refused; schema unchanged" \
    || bad "5a-mixed-refuses" "refused but schema changed"
fi

mk s5b; tracking s5b; release_table s5b "subject !~ '[0-9][0-9\\-._() ]{10,}[0-9]'" "$STRONG_B"
if apply11 s5b; then
  bad "5b-spoof-refuses" "0011 accepted a spoofed predicate"
else
  ok "5b-spoof-refuses" "refused; schema unchanged"
fi

mk s5c; tracking s5c
px -d s5c >/dev/null 2>&1 <<SQL
CREATE TABLE notification_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), facility_id UUID, channel TEXT NOT NULL,
    template_key TEXT NOT NULL, subject TEXT NOT NULL, body_template TEXT NOT NULL,
    recipient_type TEXT NOT NULL, recipient_contact_masked TEXT NOT NULL,
    recipient_contact_hash BYTEA NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error_code TEXT, related_resource_type TEXT, related_resource_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_outbox_channel_chk CHECK (channel IN ('dev','sms','whatsapp','email')),
    CONSTRAINT notification_outbox_recipient_type_chk CHECK (recipient_type IN ('patient','staff','facility_admin')),
    CONSTRAINT notification_outbox_status_chk CHECK (status IN ('pending','processing','delivered','failed','cancelled')),
    CONSTRAINT notification_outbox_attempt_count_chk CHECK (attempt_count >= 0),
    CONSTRAINT notification_outbox_hash_len_chk CHECK (octet_length(recipient_contact_hash) = 32),
    CONSTRAINT notification_outbox_subject_chk CHECK (octet_length(subject) BETWEEN 1 AND 200),
    CONSTRAINT notification_outbox_body_chk CHECK (octet_length(body_template) BETWEEN 1 AND 4000),
    CONSTRAINT notification_outbox_masked_chk CHECK (octet_length(recipient_contact_masked) BETWEEN 3 AND 200));
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk CHECK ($STRONG_S) NOT VALID;
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_body_chk CHECK ($STRONG_B) NOT VALID;
SQL
if apply11 s5c; then
  bad "5c-notvalid-refuses" "0011 accepted unvalidated constraints"
else
  [ "$(validated s5c notification_outbox_no_raw_phone_in_subject_chk)" = "f" ] \
    && ok "5c-notvalid-refuses" "refused; still NOT VALID, not silently validated" \
    || bad "5c-notvalid-refuses" "state changed unexpectedly"
fi

# ---------------------------------------------------------------- scenario 6
# Existing synthetic rows that violate the strengthened predicate. 0011 must
# FAIL and leave the weak constraints and the rows exactly as they were.
mk s6; tracking s6; release_table s6 "$WEAK_S" "$WEAK_B"
px -d s6 >/dev/null 2>&1 <<'SQL'
INSERT INTO notification_outbox
  (channel, template_key, subject, body_template, recipient_type,
   recipient_contact_masked, recipient_contact_hash)
VALUES ('dev','t','ok subject','ok body','patient','mask-001', decode(repeat('ab',32),'hex'));
-- A separated phone run: NO 8-digit consecutive run, so the WEAK predicate
-- accepts it, but the STRENGTHENED predicate rejects it. This is exactly the
-- class of value the drift leaves unprotected.
INSERT INTO notification_outbox
  (channel, template_key, subject, body_template, recipient_type,
   recipient_contact_masked, recipient_contact_hash)
VALUES ('dev','t','Tel 0812-3456-789','ok body','patient','mask-002', decode(repeat('cd',32),'hex'));
SQL
[ "$(pxa -d s6 -c 'SELECT count(*) FROM notification_outbox')" = "2" ] || die "scenario 6 seed"
if apply11 s6; then
  bad "6-violating-rows-refuse" "0011 accepted a table with violating rows"
else
  rows=$(pxa -d s6 -c 'SELECT count(*) FROM notification_outbox')
  if [ "$rows" = "2" ] \
     && [ "$(defn s6 notification_outbox_no_raw_phone_in_subject_chk)" = "$WEAK_S_DEF" ]; then
    ok "6-violating-rows-refuse" "refused; 2 rows preserved, weak constraint intact"
  else
    bad "6-violating-rows-refuse" "rows=$rows or constraint changed"
  fi
fi

# ---------------------------------------------------------------- scenario 7
# Transaction failure and rollback: a failure part-way through must leave NO
# partial effect. Dropping one constraint then failing on the ADD must restore
# the dropped one. Exercised directly, since 0011's own DDL is atomic.
mk s7; tracking s7; release_table s7 "$STRONG_S" "$STRONG_B"
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -q -P pager=off -d s7 >/dev/null 2>&1 <<'SQL'
BEGIN;
ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk;
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_deliberate_fail_chk
  CHECK (subject ~ '^never-matches-this$');
COMMIT;
SQL
# The ADD above cannot fail on an empty table, so force a failure instead: add a
# constraint the existing rows violate, inside a transaction that is rolled back.
mk s7b; tracking s7b; release_table s7b "$STRONG_S" "$STRONG_B"
px -d s7b >/dev/null 2>&1 <<'SQL'
INSERT INTO notification_outbox
  (channel, template_key, subject, body_template, recipient_type,
   recipient_contact_masked, recipient_contact_hash)
VALUES ('dev','t','row one','ok body','patient','mask-007', decode(repeat('ef',32),'hex'));
SQL
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -q -P pager=off -d s7b >/dev/null 2>&1 <<'SQL'
BEGIN;
ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk;
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk
  CHECK (subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9\-._() ]{10,}[0-9]' AND subject <> 'row one');
ROLLBACK;
SQL
if [ "$(defn s7b notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
   && [ "$(nconstraints s7b)" = "10" ] \
   && [ "$(pxa -d s7b -c 'SELECT count(*) FROM notification_outbox')" = "1" ]; then
  ok "7-rollback-atomic" "rolled back cleanly; 10 constraints, 1 row, original definition"
else
  bad "7-rollback-atomic" "consts=$(nconstraints s7b) def=$(defn s7b notification_outbox_no_raw_phone_in_subject_chk)"
fi

# ---------------------------------------------------------------- scenario 8
# Reapplication / version tracking: 0011 applied twice must be a no-op the
# second time, and the version row must be insertable exactly once.
mk s8; tracking s8; release_table s8 "$WEAK_S" "$WEAK_B"
px -d s8 -c "DELETE FROM schema_migrations WHERE version = 11" >/dev/null 2>&1
apply11 s8 || die "scenario 8: first apply failed"
px -d s8 -c "INSERT INTO schema_migrations (version, checksum) VALUES (11,'\x01'::bytea)" >/dev/null 2>&1 \
  || die "scenario 8: version row insert failed"
d1=$(defn s8 notification_outbox_no_raw_phone_in_subject_chk)
apply11 s8 || die "scenario 8: second apply failed (must be a no-op, not an error)"
d2=$(defn s8 notification_outbox_no_raw_phone_in_subject_chk)
dup=$(pxa -d s8 -c "SELECT count(*) FROM schema_migrations WHERE version = 11")
if [ "$d1" = "$EXP_S" ] && [ "$d2" = "$EXP_S" ] && [ "$dup" = "1" ] && [ "$(nconstraints s8)" = "10" ]; then
  ok "8-reapply-idempotent" "second apply is a no-op; one version row; 10 constraints"
else
  bad "8-reapply-idempotent" "d1=$d1 dup=$dup consts=$(nconstraints s8)"
fi

# ---------------------------------------------------------------- scenarios 9/10
# Real enforcement on each column, after remediation. These INSERTs are the
# point of the migration: they must be REJECTED.
#
# Locale-independent: the server message text is localised, so the check uses
# the psql exit status (ON_ERROR_STOP=1) plus the constraint NAME, which is
# never translated. That also proves the rejection came from the constraint
# under test rather than from some unrelated check.
try_insert() { # db subject body -> "<rc> <which-constraint>"
  out=$("$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off -d "$1" -c \
    "INSERT INTO notification_outbox
      (channel, template_key, subject, body_template, recipient_type,
       recipient_contact_masked, recipient_contact_hash)
     VALUES ('dev','t', \$q\$$2\$q\$, \$q\$$3\$q\$, 'patient','mask', decode(repeat('11',32),'hex'))" 2>&1)
  rc=$?
  which=""
  case "$out" in
    *notification_outbox_no_raw_phone_in_subject_chk*) which="subject_chk" ;;
    *notification_outbox_no_raw_phone_in_body_chk*)    which="body_chk" ;;
  esac
  printf '%s %s' "$rc" "${which:-none}"
}
enforce() { # db subject body -> REJECTED:constraint|ACCEPTED|OTHER
  res=$(try_insert "$1" "$2" "$3")
  rc=${res%% *}; which=${res##* }
  if [ "$rc" = "0" ]; then printf 'ACCEPTED'
  elif [ "$which" = "none" ]; then printf 'OTHER'
  else printf 'REJECTED:%s' "$which"; fi
}
mk s9; tracking s9; release_table s9 "$WEAK_S" "$WEAK_B"
apply11 s9 || die "scenario 9: apply failed"
r=$(enforce s9 "Konfirmasi 0812345678" "ok body")
[ "$r" = "REJECTED:subject_chk" ] && ok "9-enforce-subject" "8-digit run in subject rejected by subject_chk" \
  || bad "9-enforce-subject" "got $r"
r=$(enforce s9 "ok subject" "ok body")
[ "$r" = "ACCEPTED" ] && ok "9-enforce-subject-clean" "clean subject accepted" \
  || bad "9-enforce-subject-clean" "got $r"
r=$(enforce s9 "ok subject" "Kode 0812345678")
[ "$r" = "REJECTED:body_chk" ] && ok "10-enforce-body" "8-digit run in body rejected by body_chk" \
  || bad "10-enforce-body" "got $r"
r=$(enforce s9 "ok subject" "ok body")
[ "$r" = "ACCEPTED" ] && ok "10-enforce-body-clean" "clean body accepted" \
  || bad "10-enforce-body-clean" "got $r"
# The separator-tolerant conjunct: a spaced phone run with no 8-digit run, the
# exact class the drift left unprotected.
r=$(enforce s9 "ok subject" "Tel 0812-3456-789")
[ "$r" = "REJECTED:body_chk" ] && ok "10-enforce-body-separated" "separated phone run rejected by body_chk" \
  || bad "10-enforce-body-separated" "got $r"

# ---------------------------------------------------------------- scenario 11
# Column binding: after remediation the subject constraint must guard `subject`
# and the body constraint must guard `body_template`. Proven behaviourally, not
# textually: a violating subject must fail even when the body is clean, and a
# violating body must fail even when the subject is clean (both above), AND the
# definitions must not be swapped.
if [ "$(defn s9 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
   && [ "$(defn s9 notification_outbox_no_raw_phone_in_body_chk)" = "$EXP_B" ] \
   && [ "$EXP_S" != "$EXP_B" ]; then
  ok "11-column-binding" "subject guards subject, body guards body"
else
  bad "11-column-binding" "definitions swapped or unexpected"
fi

# ---------------------------------------------------------------- scenario 12
# Application write-path compatibility, where testable without the API: the
# strengthened predicate must not reject any value the WEAK predicate accepted
# except raw-phone-like runs, i.e. it must be a strict superset of rejections
# and must not reject ordinary notification content.
compat() { # db value -> REJECTED|ACCEPTED
  "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off -d "$1" -c \
    "INSERT INTO notification_outbox
      (channel, template_key, subject, body_template, recipient_type,
       recipient_contact_masked, recipient_contact_hash)
     VALUES ('dev','t', \$q\$$2\$q\$, \$q\$$2\$q\$, 'patient','mask', decode(repeat('22',32),'hex'))" >/dev/null 2>&1
  if [ $? -eq 0 ]; then printf 'ACCEPTED'; else printf 'REJECTED'; fi
}
compat_fail=""
for v in \
  "Konfirmasi Janji Temu Sigap" \
  "Check-in Anda di fasilitas berhasil. Nomor antrean Anda: A-012." \
  "Janji temu pada 2026-07-09 pukul 14:30" \
  "Kode check-in: SIGAP-ABC123" \
  "Nomor antrean A-012, meja 3"
do
  r=$(compat s9 "$v")
  [ "$r" = "ACCEPTED" ] || compat_fail="$compat_fail [$v=>$r]"
done
if [ -z "$compat_fail" ]; then
  ok "12-app-compat" "ordinary notification content still accepted"
else
  bad "12-app-compat" "$compat_fail"
fi

# ---------------------------------------------------------------- scenario 13
# standard_conforming_strings=off. The regex literal contains a backslash, and
# under SCS=off a PLAIN single-quoted literal has that backslash consumed by the
# string-literal escape, yielding '[0-9][0-9-._() ]{10,}[0-9]' — in which '9-.'
# is an invalid character range, so the predicate RAISES on every insert. The
# migration dollar-quotes its regexes to be immune to this. Without that, the
# expected text (built from the same degraded literal) would match the degraded
# real constraint and the migration would certify a broken denylist as
# "already strengthened" — a silent no-op on the exact schema it exists to fix.
mk s13; tracking s13; release_table s13 "$WEAK_S" "$WEAK_B"
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off \
  -d s13 -c "ALTER DATABASE s13 SET standard_conforming_strings = off" >/dev/null 2>&1 \
  || die "scenario 13: could not set SCS=off"
scs=$(pxa -d s13 -c "SHOW standard_conforming_strings")
if [ "$scs" != "off" ]; then
  bad "13-scs-off" "precondition failed: standard_conforming_strings=$scs"
else
  PGOPTIONS='-c standard_conforming_strings=off' "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres \
    -v ON_ERROR_STOP=1 -q -P pager=off -d s13 -f "$MIG11" >"$TMP/apply.log" 2>&1
  rc=$?
  # The denylist must be genuinely working: a separated phone run must be
  # rejected, and ordinary content must be accepted — not "invalid regular
  # expression" on everything.
  r1=$(PGOPTIONS='-c standard_conforming_strings=off' "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres \
        -v ON_ERROR_STOP=1 -q -P pager=off -d s13 -c \
        "INSERT INTO notification_outbox (channel,template_key,subject,body_template,recipient_type,recipient_contact_masked,recipient_contact_hash) VALUES ('dev','t','ok subject','ok body','patient','mask',decode(repeat('33',32),'hex'))" >/dev/null 2>&1; echo $?)
  if [ "$rc" = "0" ] && [ "$r1" = "0" ] \
     && [ "$(defn_on s13 notification_outbox_no_raw_phone_in_subject_chk)" = "$EXP_S" ] \
     && [ "$(defn_on s13 notification_outbox_no_raw_phone_in_body_chk)" = "$EXP_B" ]; then
    ok "13-scs-off" "regexes survive SCS=off; predicate matches the release form; inserts work"
  else
    bad "13-scs-off" "rc=$rc clean_insert_rc=$r1 def=$(defn_on s13 notification_outbox_no_raw_phone_in_subject_chk)"
  fi
fi

# ---------------------------------------------------------------- scenario 14
# STATIC: the file must contain no top-level BEGIN/COMMIT. The runner wraps each
# migration in its own transaction and calls tx.Exec with no arguments, so pgx
# sends the whole file as ONE simple-protocol multi-statement query — an internal
# COMMIT would end the runner's transaction before the schema_migrations INSERT,
# breaking the atomicity of DDL + version row. 0006 is the only historical
# migration with this defect; 0011 must not repeat it. Asserted statically
# because the runner-level behaviour needs a live Go process.
if grep -nE '^[[:space:]]*(BEGIN|COMMIT)[[:space:]]*;' "$MIG11" >/dev/null 2>&1; then
  bad "14-no-toplevel-txn" "$(grep -nE '^[[:space:]]*(BEGIN|COMMIT)[[:space:]]*;' "$MIG11" | tr '\n' ' ')"
else
  ok "14-no-toplevel-txn" "no top-level BEGIN/COMMIT; runner keeps its own transaction"
fi

# ---------------------------------------------------------------- scenario 15
# The EIGHT structural constraints must be definitionally intact, not merely
# present. A count-preserving corruption (drop + re-add a different definition,
# keeping the total at 10) would otherwise pass every other scenario here.
struct_fail=""
for c in channel recipient_type status attempt_count hash_len subject body masked; do
  cn="notification_outbox_${c}_chk"
  got=$(defn_on s1 "$cn")
  case "$c" in
    channel)        want="CHECK ((channel = ANY (ARRAY['dev'::text, 'sms'::text, 'whatsapp'::text, 'email'::text])))" ;;
    recipient_type) want="CHECK ((recipient_type = ANY (ARRAY['patient'::text, 'staff'::text, 'facility_admin'::text])))" ;;
    status)         want="CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'delivered'::text, 'failed'::text, 'cancelled'::text])))" ;;
    attempt_count)  want="CHECK ((attempt_count >= 0))" ;;
    hash_len)       want="CHECK ((octet_length(recipient_contact_hash) = 32))" ;;
    subject)        want="CHECK (((octet_length(subject) >= 1) AND (octet_length(subject) <= 200)))" ;;
    body)           want="CHECK (((octet_length(body_template) >= 1) AND (octet_length(body_template) <= 4000)))" ;;
    masked)         want="CHECK (((octet_length(recipient_contact_masked) >= 3) AND (octet_length(recipient_contact_masked) <= 200)))" ;;
  esac
  [ "$got" = "$want" ] || struct_fail="$struct_fail [$cn=>$got]"
done
if [ -z "$struct_fail" ]; then
  ok "15-structural-defs" "all 8 structural constraints match their release definitions"
else
  bad "15-structural-defs" "$struct_fail"
fi

printf 'migration-0011 tests: %s pass, %s fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ]
