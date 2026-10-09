#!/bin/sh
# test-db-classifier.sh — non-vacuity controls for scripts/ops/db-metadata-inspection.sql
#
# Builds a DISPOSABLE local PostgreSQL cluster in a temp directory, creates the
# variants below, runs the read-only inspection script against each, and asserts
# the classification. Proves the classifier cannot report a false PASS: a
# weaker, missing, spoofed, mis-bound or drifted schema must never classify as
# MATCHES_CURRENT_SECURITY_CONSTRAINTS.
#
# Variants (see docs/operations/DEPLOYMENT_RUNBOOK.md §12):
#   A  actual release schema (migrations 0001..0010 applied in order)
#   B  genuine older (pre-9d4e68e) weak constraints
#   C  fragment-spoofed constraint (`[0-9]{10,}`)                     [SEC-01]
#   D  subject phone constraint missing
#   E  body phone constraint missing
#   F  mis-bound column (subject constraint guards body_template)      [SEC-04]
#   G  unexpected extra CHECK constraint                               [SEC-03]
#   H  logically altered predicate (`OR`, widened quantifier, `AND TRUE`)
#   I  absent migration history (no schema_migrations table)
#   J  unexpected metadata (extra column / missing column / extra version /
#      missing version / missing structural constraint)
#   K  query failure (no notification_outbox table)
#   L  explicit correct hardened schema
#   M  static inventory consistency with packages/db/migrations
#
# NOTE: PostgreSQL folds unquoted identifiers to lower case, so every database
# name here is lower case.
#
# Usage:  sh scripts/ops/test-db-classifier.sh [path-to-inspection.sql]
# Exit 0 iff every variant classifies as expected; 2 if PostgreSQL client/server
# binaries are unavailable locally (skip, not a failure).

set -u

# This suite relies on POSIX coreutils (sort, grep, sed, awk, tr, ls). On a
# Windows host the invoking shell can hand us a PATH whose only `sort` is
# C:\WINDOWS\system32\sort.exe — which fails on `-u`, prints NOTHING, and would
# make the M-inventory comparison below compare "" against "" and pass
# vacuously. Prepend the POSIX directories, then deduplicate keeping the FIRST
# occurrence so they cannot be shadowed from further down PATH.
POSIX_PATH=""
for d in /usr/bin /bin; do
  [ -d "$d" ] && POSIX_PATH="${POSIX_PATH:+$POSIX_PATH:}$d"
done
# set -f: without it, the unquoted $PATH expansion below would undergo pathname
# expansion, and a PATH entry containing * or ? would be globbed and could be
# mistaken for a duplicate of an unrelated directory. Field splitting on IFS is
# unaffected, so the colon-separated walk still works.
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
SUT=${1:-$SCRIPT_DIR/db-metadata-inspection.sql}
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
MIG_DIR=${SIGAP_MIG_DIR:-$REPO_ROOT/packages/db/migrations}

[ -f "$SUT" ] || { echo "FAIL: $SUT not found"; exit 1; }

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

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-dbclass-$$")
mkdir -p "$TMP"
CL="$TMP/cluster"
PORT_OVERRIDE=${SIGAP_TEST_PG_PORT:-}
PORT=${PORT_OVERRIDE:-55451}

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
# Setup must fail LOUDLY: a silently-failed CREATE DATABASE would make the two
# UNKNOWN variants pass for the wrong reason (a connection error also yields
# UNKNOWN), so the suite would claim coverage it does not have.
die() { printf 'FAIL: setup step failed: %s\n' "$1"; exit 1; }
mk() {
  px -d postgres -c "DROP DATABASE IF EXISTS $1" -c "CREATE DATABASE $1" >/dev/null 2>&1 \
    || die "create database $1"
}

# Classification under test. Prints one of the five classes, or UNKNOWN when
# psql exits non-zero or the classification row is absent.
classify() {
  out=$("$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off -A -t \
        -d "$1" -f "$SUT" 2>/dev/null)
  rc=$?
  if [ "$rc" -ne 0 ]; then printf 'UNKNOWN'; return; fi
  c=$(printf '%s\n' "$out" | tr -d '\r' \
      | grep -xE 'MATCHES_CURRENT_SECURITY_CONSTRAINTS|OLDER_WEAKER_CONSTRAINTS|MISSING_CONSTRAINTS|UNEXPECTED_DRIFT' | tail -1)
  printf '%s' "${c:-UNKNOWN}"
}

# Same, but with standard_conforming_strings OFF. pg_get_constraintdef then
# renders the regex backslash DOUBLED, so a classifier that compares against a
# backslash LITERAL (instead of stripping the escape with chr(92)) stops
# matching and misreports a genuinely strengthened schema as UNEXPECTED_DRIFT.
# This is the only assertion that covers the chr(92) fix.
classify_scsoff() {
  out=$(PGOPTIONS='-c standard_conforming_strings=off' \
        "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off -A -t \
        -d "$1" -f "$SUT" 2>/dev/null)
  rc=$?
  if [ "$rc" -ne 0 ]; then printf 'UNKNOWN'; return; fi
  c=$(printf '%s\n' "$out" | tr -d '\r' \
      | grep -xE 'MATCHES_CURRENT_SECURITY_CONSTRAINTS|OLDER_WEAKER_CONSTRAINTS|MISSING_CONSTRAINTS|UNEXPECTED_DRIFT' | tail -1)
  printf '%s' "${c:-UNKNOWN}"
}

# Seed the tracking table. $2 is the highest applied version and defaults to 10,
# because a database that applied the real migrations WITHOUT the Go runner
# records only 1..10 — and that is the actual production shape. Variant A passes
# 11 explicitly, since it simulates the runner having applied 0011 too. Passing
# 11 everywhere (as this fixture used to) made the "pre-0011" variants test a
# state that cannot exist before 0011 runs, which is exactly the bug the
# classifier's required_version set exists to avoid.
tracking() {
maxv=${2:-10}
px -d "$1" >/dev/null 2>&1 <<SQL
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY, checksum BYTEA NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO schema_migrations (version, checksum)
  SELECT g, '\x01'::bytea FROM generate_series(1,$maxv) g ON CONFLICT DO NOTHING;
SQL
[ $? -eq 0 ] || die "tracking table for $1"
}

# The 17 release columns and the 10 release CHECK constraints, with the two
# phone predicates parameterised: $2 = subject predicate, $3 = body predicate.
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

pass=0; failed=0
check() { # db expected label
  got=$(classify "$1"); [ -z "$got" ] && got=UNKNOWN
  if [ "$got" = "$2" ]; then pass=$((pass+1)); printf 'ok   %-16s %-36s %s\n' "$3" "$got" "$1"
  else failed=$((failed+1)); printf 'FAIL %-16s got=%-36s want=%s (%s)\n' "$3" "$got" "$2" "$1"; fi
}
check_scsoff() { # db expected label — classifier run with standard_conforming_strings=off
  got=$(classify_scsoff "$1"); [ -z "$got" ] && got=UNKNOWN
  if [ "$got" = "$2" ]; then pass=$((pass+1)); printf 'ok   %-16s %-36s %s (scs=off)\n' "$3" "$got" "$1"
  else failed=$((failed+1)); printf 'FAIL %-16s got=%-36s want=%s (%s, scs=off)\n' "$3" "$got" "$2" "$1"; fi
}

# A — actual release schema: apply the real migrations in order.
#     The glob must be four-digit so that 0010+ are included: `000*.sql` matches
#     only 0001-0009 and would silently test a stale schema while still claiming
#     full-migration coverage.
mk ca
mig_count=0
for f in "$MIG_DIR"/[0-9][0-9][0-9][0-9]_*.sql; do
  [ -f "$f" ] || continue
  "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off -d ca -f "$f" >/dev/null 2>&1 \
    || die "migration failed to apply: $(basename "$f")"
  mig_count=$((mig_count+1))
done
[ "$mig_count" -gt 0 ] || die "no migration files matched under $MIG_DIR"
tracking ca 11
check ca MATCHES_CURRENT_SECURITY_CONSTRAINTS A-release-schema

# N — the SAME release schema with standard_conforming_strings OFF must classify
#     identically. pg_get_constraintdef then doubles the regex backslash; the
#     classifier strips it with chr(92), so the comparison is unaffected. A
#     classifier comparing against a backslash LITERAL fails only here.
check_scsoff ca MATCHES_CURRENT_SECURITY_CONSTRAINTS N-scs-off

# B — genuine older (weak) constraints.
mk cb; tracking cb; release_table cb "$WEAK_S" "$WEAK_B"
check cb OLDER_WEAKER_CONSTRAINTS B-weak

# C — fragment spoof: predicate contains {10,} but is weaker.
mk cc; tracking cc; release_table cc "subject !~ '[0-9]{10,}'" "body_template !~ '[0-9]{10,}'"
check cc UNEXPECTED_DRIFT C-spoof

# D — subject phone constraint missing.
mk cd; tracking cd; release_table cd "$STRONG_S" "$STRONG_B"
px -d cd -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk" >/dev/null 2>&1
check cd MISSING_CONSTRAINTS D-subject-missing

# E — body phone constraint missing.
mk ce; tracking ce; release_table ce "$STRONG_S" "$STRONG_B"
px -d ce -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk" >/dev/null 2>&1
check ce MISSING_CONSTRAINTS E-body-missing

# F — mis-bound column: the SUBJECT constraint guards body_template.
mk cf; tracking cf
px -d cf >/dev/null 2>&1 <<SQL
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
    CONSTRAINT notification_outbox_masked_chk CHECK (octet_length(recipient_contact_masked) BETWEEN 3 AND 200),
    CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk CHECK ($STRONG_B),
    CONSTRAINT notification_outbox_no_raw_phone_in_body_chk CHECK ($STRONG_B)
);
SQL
check cf UNEXPECTED_DRIFT F-mis-bound-column

# G — unexpected extra CHECK constraint.
mk cg; tracking cg; release_table cg "$STRONG_S" "$STRONG_B"
px -d cg -c "ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_extra_chk CHECK (attempt_count < 1000000)" >/dev/null 2>&1
check cg UNEXPECTED_DRIFT G-extra-constraint

# H — logically altered predicates.
mk ch1; tracking ch1; release_table ch1 \
  "subject !~ '[0-9]{8,}' OR subject !~ '[0-9][0-9\\-._() ]{10,}[0-9]'" "$STRONG_B"
check ch1 UNEXPECTED_DRIFT H-or
mk ch2; tracking ch2; release_table ch2 \
  "subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9\\-._() ]{11,}[0-9]'" "$STRONG_B"
check ch2 UNEXPECTED_DRIFT H-widened-quantifier
mk ch3; tracking ch3; release_table ch3 "$STRONG_S" \
  "body_template !~ '[0-9]{8,}' AND body_template !~ '[0-9][0-9\\-._() ]{10,}[0-9]' AND TRUE"
check ch3 UNEXPECTED_DRIFT H-extra-term

# H4 — `{10,}` CONJUNCT ONLY, on BOTH columns. Residue-empty and contains
#      `{10,}`, but strictly WEAKER than the release definition: it admits an
#      8-digit raw run that `!~ '[0-9]{8,}'` rejects. Must NOT be MATCHES.
mk ch4; tracking ch4; release_table ch4 \
  "subject !~ '[0-9][0-9\\-._() ]{10,}[0-9]'" "body_template !~ '[0-9][0-9\\-._() ]{10,}[0-9]'"
check ch4 UNEXPECTED_DRIFT H-strong-clause-only

# H5 — MIXED forms: subject hardened, body still weak. Reaches the CASE ELSE
#      branch (one column protected, the other not) and must be DRIFT.
mk ch5; tracking ch5; release_table ch5 "$STRONG_S" "$WEAK_B"
check ch5 UNEXPECTED_DRIFT H-mixed-strong-and-weak

# H6 — `{10,}`-only subject + weak body: the other mixed form.
mk ch6; tracking ch6; release_table ch6 \
  "subject !~ '[0-9][0-9\\-._() ]{10,}[0-9]'" "$WEAK_B"
check ch6 UNEXPECTED_DRIFT H-mixed-spoof-and-weak

# H7 — NOT VALID phone constraints. `ALTER TABLE ... ADD CONSTRAINT ... NOT VALID`
#      is the only supported way to create them, and pg_get_constraintdef appends
#      " NOT VALID", which must NOT be read as a strengthened predicate: the
#      constraint is not validated against existing rows, so the table may already
#      hold raw phone values. (Note: an inline `NOT VALID` in CREATE TABLE is
#      silently ignored by PostgreSQL, so this must go through ALTER TABLE.)
mk ch7; tracking ch7
px -d ch7 >/dev/null 2>&1 <<SQL
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
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk
  CHECK ($STRONG_S) NOT VALID;
ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_body_chk
  CHECK ($STRONG_B) NOT VALID;
SQL
[ $? -eq 0 ] || die "variant H7 setup"
[ "$(px -d ch7 -A -t -c "SELECT bool_and(NOT convalidated) FROM pg_constraint WHERE conname LIKE '%no_raw_phone%'")" = "t" ] \
  || die "variant H7 precondition: both phone constraints should be NOT VALID"
check ch7 UNEXPECTED_DRIFT H-not-valid

# T — UNESCAPED separator class: the regex has lost its backslash, so
#     `[0-9-._() ]` is an INVALID CHARACTER RANGE. PostgreSQL ACCEPTS this at
#     CREATE CONSTRAINT time and only raises when the regex is evaluated, so the
#     state is reachable and every INSERT then fails. It must classify as
#     UNEXPECTED_DRIFT, never MATCHES: the normalized comparison strips chr(92)
#     from both sides, which would otherwise make this textually identical to
#     the valid escaped form.
mk ct; tracking ct; release_table ct \
  "subject !~ '[0-9]{8,}' AND subject !~ '[0-9][0-9-._() ]{10,}[0-9]'" \
  "body_template !~ '[0-9]{8,}' AND body_template !~ '[0-9][0-9-._() ]{10,}[0-9]'"
# Precondition 1: the constraint really is present (not a MISSING case).
[ "$(px -d ct -A -t -c "SELECT count(*) FROM pg_constraint WHERE conname = 'notification_outbox_no_raw_phone_in_subject_chk'")" = "1" ] \
  || die "variant T precondition: constraint absent"
# Precondition 2: the constraint is FUNCTIONALLY BROKEN — an ordinary insert
# raises. This is what makes MATCHES a false PASS rather than a cosmetic
# mismatch.
px -d ct -c "INSERT INTO notification_outbox (channel, template_key, subject, body_template, recipient_type, recipient_contact_masked, recipient_contact_hash) VALUES ('dev','t','halo','halo','patient','+62••••1234', decode(repeat('00',32),'hex'))" >/dev/null 2>&1 \
  && die "variant T precondition: the unescaped constraint accepted an insert; fixture is not broken"
check ct UNEXPECTED_DRIFT T-unescaped-separator-class

# I — absent migration history (no schema_migrations table). Assert the
#     pre-condition, so this cannot pass because the DATABASE was missing.
mk ci; release_table ci "$STRONG_S" "$STRONG_B"
[ "$(px -d ci -A -t -c "SELECT to_regclass('schema_migrations') IS NULL")" = "t" ] \
  || die "variant I precondition: schema_migrations should be absent"
check ci UNKNOWN I-no-tracking-table

# J — unexpected metadata.
mk cj1; tracking cj1; release_table cj1 "$STRONG_S" "$STRONG_B"
px -d cj1 -c "ALTER TABLE notification_outbox ADD COLUMN unexpected_col TEXT" >/dev/null 2>&1
check cj1 UNEXPECTED_DRIFT J-extra-column
mk cj2; tracking cj2; release_table cj2 "$STRONG_S" "$STRONG_B"
px -d cj2 -c "ALTER TABLE notification_outbox DROP COLUMN last_error_code" >/dev/null 2>&1
check cj2 UNEXPECTED_DRIFT J-missing-column
mk cj3; tracking cj3; release_table cj3 "$STRONG_S" "$STRONG_B"
px -d cj3 -c "INSERT INTO schema_migrations (version, checksum) VALUES (99,'\x63'::bytea)" >/dev/null 2>&1
check cj3 UNEXPECTED_DRIFT J-extra-version
mk cj4; tracking cj4; release_table cj4 "$STRONG_S" "$STRONG_B"
px -d cj4 -c "DELETE FROM schema_migrations WHERE version > 5" >/dev/null 2>&1
check cj4 UNEXPECTED_DRIFT J-missing-version
mk cj5; tracking cj5; release_table cj5 "$STRONG_S" "$STRONG_B"
px -d cj5 -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_masked_chk" >/dev/null 2>&1
check cj5 UNEXPECTED_DRIFT J-missing-structural

# K — query failure (no notification_outbox table at all). Assert the
#     pre-condition so this cannot pass because the DATABASE was missing.
mk ck; tracking ck
[ "$(px -d ck -A -t -c "SELECT to_regclass('notification_outbox') IS NULL")" = "t" ] \
  || die "variant K precondition: notification_outbox should be absent"
check ck UNKNOWN K-no-table

# G2 — BOTH phone constraints absent AND an unexpected constraint present.
#      The unexpected constraint must be reported as DRIFT, not masked by the
#      missing-denylist branch: re-adding only the phones would otherwise leave
#      the rogue constraint in place.
mk cg2; tracking cg2; release_table cg2 "$STRONG_S" "$STRONG_B"
px -d cg2 -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk" \
           -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk" \
           -c "ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_evil_chk CHECK (attempt_count < 1000000)" >/dev/null 2>&1 \
  || die "variant G2 setup"
check cg2 UNEXPECTED_DRIFT G2-missing-phones-plus-extra-constraint

# G3 — both phone constraints absent, structural inventory otherwise intact:
#      still MISSING_CONSTRAINTS (the security finding), not DRIFT.
mk cg3; tracking cg3; release_table cg3 "$STRONG_S" "$STRONG_B"
px -d cg3 -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk" \
           -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk" >/dev/null 2>&1 \
  || die "variant G3 setup"
check cg3 MISSING_CONSTRAINTS G3-both-phones-missing

# L — explicit correct hardened schema (built by hand, not from migrations).
mk cl; tracking cl; release_table cl "$STRONG_S" "$STRONG_B"
check cl MATCHES_CURRENT_SECURITY_CONSTRAINTS L-hardened

# P — the pre-0011 drifted state, then migration 0011 applied. Proves the
#     classifier reports the SAME readiness class before and after remediation,
#     and that 0011 converges the weak form onto the release predicate. Without
#     this variant the suite would only ever see 0011 run against a schema that
#     already satisfied it (variant A), where 0011 is a no-op.
#
#     tracking defaults to 10 here, which is the point: this is the REAL
#     production shape (versions 1..10, weak constraints), and applying 0011
#     with `psql -f` does NOT add a version row — only the Go runner does. Both
#     the pre state (1..10 + weak) and the post state (1..10 + strong) are
#     legitimate and must classify as OLDER_WEAKER_CONSTRAINTS and
#     MATCHES_CURRENT_SECURITY_CONSTRAINTS respectively.
mk cp; tracking cp; release_table cp "$WEAK_S" "$WEAK_B"
check cp OLDER_WEAKER_CONSTRAINTS P-pre-0011-weak
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off \
  -d cp -f "$MIG_DIR/0011_notification_outbox_phone_constraints.sql" >/dev/null 2>&1 \
  || die "variant P: 0011 failed to apply to the weak schema"
check cp MATCHES_CURRENT_SECURITY_CONSTRAINTS P-post-0011

# Q — denylist entirely absent, then 0011 applied: 0011 must ADD both
#     constraints, taking MISSING_CONSTRAINTS to MATCHES.
mk cq; tracking cq; release_table cq "$STRONG_S" "$STRONG_B"
px -d cq -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk" \
           -c "ALTER TABLE notification_outbox DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk" >/dev/null 2>&1
check cq MISSING_CONSTRAINTS Q-pre-0011-absent
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off \
  -d cq -f "$MIG_DIR/0011_notification_outbox_phone_constraints.sql" >/dev/null 2>&1 \
  || die "variant Q: 0011 failed to apply to the absent-denylist schema"
check cq MATCHES_CURRENT_SECURITY_CONSTRAINTS Q-post-0011

# R — version 11 RECORDED but the constraints still weak. Reachable if the runner
#     recorded 11 and a later manual edit reverted the constraints, or if the
#     version row was inserted out of band. Must still be OLDER_WEAKER (a
#     security finding), never MATCHES — the version row is bookkeeping, the
#     predicate is the control.
mk cr; tracking cr 11; release_table cr "$WEAK_S" "$WEAK_B"
check cr OLDER_WEAKER_CONSTRAINTS R-v11-recorded-but-weak

# S — the mirror: versions 1..10 (0011 NOT recorded) with the constraints already
#     strengthened. This is what `psql -f 0011_...sql` actually produces, so it
#     must be MATCHES, not UNEXPECTED_DRIFT. Before the required_version fix this
#     was misreported as drift, which would have made J10's own acceptance
#     criterion unreachable.
mk cs; tracking cs; release_table cs "$STRONG_S" "$STRONG_B"
check cs MATCHES_CURRENT_SECURITY_CONSTRAINTS S-hardened-without-v11

# M — static inventory consistency between the SQL and the release migrations.
#     Only the FIRST expected_constraint/column/version block is inspected
#     (each appears in both query 4 and query 4b).
sql_constraints=$(sed -n '/expected_constraint(name) AS (/,/^)/p' "$SUT" | grep -oE "'notification_outbox_[a-z_]+'" | tr -d "'" | sort -u)
sql_columns=$(sed -n '/expected_column(name) AS (/,/^)/p' "$SUT" | grep -oE "'[a-z_]+'" | tr -d "'" | sort -u)
sql_versions=$(sed -n '/expected_version(version) AS (/,/^)/p' "$SUT" | grep -oE '\b[0-9]+\b' | sort -n -u | tr '\n' ' ')

mig_constraints=$(grep -oE 'CONSTRAINT notification_outbox_[a-z_]+' "$MIG_DIR/0006_notifications.sql" | awk '{print $2}' | sort -u)
mig_columns=$(sed -n '/CREATE TABLE IF NOT EXISTS notification_outbox/,/^);/p' "$MIG_DIR/0006_notifications.sql" \
  | grep -E '^[[:space:]]+[a-z_]+[[:space:]]' | awk '{print $1}' | grep -v '^CONSTRAINT$' | sort -u)
mig_versions=$(ls "$MIG_DIR" | grep -oE '^[0-9]{4}' | sed 's/^0*//' | sort -n -u | tr '\n' ' ')
# Count of files matching the four-digit pattern variant A actually applies.
# If a migration is ever named outside that pattern, variant A would silently
# test a stale schema; this makes that a failure instead.
mig_files_globbed=$(ls "$MIG_DIR"/[0-9][0-9][0-9][0-9]_*.sql 2>/dev/null | wc -l)
mig_files_total=$(ls "$MIG_DIR"/*.sql 2>/dev/null | wc -l)

m_ok=0
# Anti-vacuity: an extraction that produced nothing must never be compared,
# because "" == "" would silently "pass". This is what made the suite green on a
# host whose `sort` was C:\WINDOWS\system32\sort.exe (fails on -u, prints
# nothing).
for pair in "sql_constraints:$sql_constraints" "sql_columns:$sql_columns" \
            "sql_versions:$sql_versions" "mig_constraints:$mig_constraints" \
            "mig_columns:$mig_columns" "mig_versions:$mig_versions"; do
  case "$pair" in
    *:) m_ok=1; printf 'FAIL %-16s extraction produced no output (%s)\n' M-inventory "${pair%%:*}" ;;
  esac
done
if [ "$mig_constraints" != "$sql_constraints" ]; then
  m_ok=1; printf 'FAIL %-16s constraint inventory differs from the migration\n' M-inventory
  printf '  migration: %s\n  sql      : %s\n' "$(printf '%s' "$mig_constraints" | tr '\n' ' ')" "$(printf '%s' "$sql_constraints" | tr '\n' ' ')"
fi
if [ "$mig_columns" != "$sql_columns" ]; then
  m_ok=1; printf 'FAIL %-16s column inventory differs from the migration\n' M-inventory
  printf '  migration: %s\n  sql      : %s\n' "$(printf '%s' "$mig_columns" | tr '\n' ' ')" "$(printf '%s' "$sql_columns" | tr '\n' ' ')"
fi
if [ "$(printf '%s' "$mig_versions" | tr -s ' ')" != "$(printf '%s' "$sql_versions" | tr -s ' ')" ]; then
  m_ok=1; printf 'FAIL %-16s version inventory differs from the migrations directory\n' M-inventory
  printf '  migrations: [%s]\n  sql       : [%s]\n' "$mig_versions" "$sql_versions"
fi
# Variant A must apply EVERY migration file. A name outside the four-digit
# pattern would be silently skipped, so the A variant would validate a stale
# schema while the docs still claim full-migration coverage.
if [ "$mig_files_globbed" -ne "$mig_files_total" ]; then
  m_ok=1; printf 'FAIL %-16s variant A glob covers %s of %s migration files\n' M-inventory \
    "$mig_files_globbed" "$mig_files_total"
  printf '  unmatched files: %s\n' "$(ls "$MIG_DIR"/*.sql 2>/dev/null | grep -vE '/[0-9]{4}_' | tr '\n' ' ')"
fi
if [ "$m_ok" -eq 0 ]; then
  pass=$((pass+1))
  ncons=$(printf '%s\n' "$mig_constraints" | grep -c .)
  ncols=$(printf '%s\n' "$mig_columns" | grep -c .)
  printf 'ok   %-16s %-36s %s constraints, %s columns, versions %s\n' M-inventory "matches migrations" \
    "$ncons" "$ncols" "$(printf '%s' "$mig_versions" | tr -s ' ')"
else
  failed=$((failed+1))
fi

printf 'db-classifier tests: %s pass, %s fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ]
