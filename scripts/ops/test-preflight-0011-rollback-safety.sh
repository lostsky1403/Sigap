#!/bin/sh
# test-preflight-0011-rollback-safety.sh — proves the read-only 0011 pre-flight
# guard (scripts/ops/preflight-0011-rollback-safety.sql) actually DETECTS the two
# conditions it exists to detect, and stays read-only.
#
# Why this test exists. The guard's whole value is that it is not vacuous: a
# guard whose predicate was accidentally inverted, or narrowed to the weak
# conjunct only, would report PREFLIGHT_OK on exactly the database that needs to
# be flagged. J11 (rollback safety) was left UNKNOWN for weeks because an earlier
# review argued from a predicate that was asserted, not executed. So every
# scenario below asserts the VERDICT AND the identifying row, not merely an exit
# code, and scenario 6 asserts the guard leaves the database untouched.
#
# Synthetic data only. Disposable cluster in a temp dir, non-default port.
#
# Usage:  sh scripts/ops/test-preflight-0011-rollback-safety.sh
# Exit 0 iff every scenario passes; 2 if PostgreSQL binaries are unavailable.

set -u

# POSIX coreutils first: on Windows the only `sort` on PATH can be
# C:\WINDOWS\system32\sort.exe, which fails on -u and prints nothing, turning any
# string comparison into a vacuous "" == "" pass.
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
GUARD=$SCRIPT_DIR/preflight-0011-rollback-safety.sql

[ -f "$GUARD" ] || { echo "FAIL: $GUARD not found"; exit 1; }

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

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-pf0011-$$")
mkdir -p "$TMP"
CL="$TMP/cluster"
PORT_OVERRIDE=${SIGAP_TEST_PG_PORT:-}
PORT=${PORT_OVERRIDE:-55491}

cleanup() {
  "$PGCTL" -D "$CL" -m immediate stop >/dev/null 2>&1
  rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

"$INITDB" -D "$CL" -U postgres --auth=trust -E UTF8 >"$TMP/initdb.log" 2>&1 \
  || { echo "FAIL: initdb failed"; tail -5 "$TMP/initdb.log"; exit 1; }

# Where the postmaster puts its unix socket. Set EXPLICITLY on Unix, NOT SET AT
# ALL on Windows — both halves are load-bearing (see test-migration-0011.sh for
# the two observed failure modes).
case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT|windows*) SOCK_OPT="" ;;
  *) SOCK_OPT="-c unix_socket_directories=$TMP" ;;
esac

start_cluster() {
  attempts=0
  while [ "$attempts" -lt 8 ]; do
    if "$PGCTL" -D "$CL" \
         -o "-p $PORT -c listen_addresses=127.0.0.1 $SOCK_OPT" \
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

px()  { "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -v ON_ERROR_STOP=1 -q -P pager=off "$@"; }
pxa() { "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -q -P pager=off -A -t "$@"; }
die() { printf 'FAIL: setup step failed: %s\n' "$1"; exit 1; }
mk() {
  px -d postgres -c "DROP DATABASE IF EXISTS $1" -c "CREATE DATABASE $1" >/dev/null 2>&1 \
    || die "create database $1"
}

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-34s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-34s %s\n' "$1" "$2"; }

# Apply every migration in filename order with psql. The guard reads only
# pg_catalog / facilities / notification_outbox, so schema_migrations is not
# needed here. 0006 carries its own BEGIN/COMMIT, which is harmless in psql's
# autocommit mode (it is the Go runner's wrapping transaction it would break).
apply_all() { # db
  for f in "$MIG_DIR"/[0-9][0-9][0-9][0-9]_*.sql; do
    [ -f "$f" ] || continue
    px -d "$1" -f "$f" >"$TMP/apply.log" 2>&1 || die "$1: $(basename "$f")"
  done
}

# Rewrite both phone constraints to the pre-9d4e68e WEAK form. Test-cluster only.
weaken() { # db
px -d "$1" >/dev/null 2>&1 <<'SQL'
ALTER TABLE notification_outbox
  DROP CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk,
  DROP CONSTRAINT notification_outbox_no_raw_phone_in_body_chk;
ALTER TABLE notification_outbox
  ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk CHECK (subject !~ '[0-9]{8,}'),
  ADD CONSTRAINT notification_outbox_no_raw_phone_in_body_chk    CHECK (body_template !~ '[0-9]{8,}');
SQL
}

# Run the guard. Output to a file; echo the verdict, or MISSING.
guard() { # db -> verdict
  "$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -P pager=off \
    -d "$1" -f "$GUARD" >"$TMP/guard.out" 2>&1
  v=$(grep -oE 'PREFLIGHT_(OK|BLOCKED)' "$TMP/guard.out" | head -1)
  if [ -z "$v" ]; then printf 'MISSING'; else printf '%s' "$v"; fi
}

facility() { # db shortcode id
px -d "$1" -c "INSERT INTO facilities (id,name,type,address,kecamatan,kabupaten_kota,provinsi,phone,total_beds,available_beds,short_code,is_active)
               VALUES ('$3','F','rumah_sakit','Jl','Kec','Kota','Prov','000',10,10,'$2',true)" >/dev/null 2>&1 \
  || die "insert facility $2"
}

# ---------------------------------------------------------------- scenario 1
# Clean release schema: every migration applied, no facilities, no rows. The
# guard must NOT report a blocked verdict just because it found nothing.
mk s1
apply_all s1
if [ "$(guard s1)" = "PREFLIGHT_OK" ]; then
  ok 1-clean-release-schema "PREFLIGHT_OK on an empty release schema"
else
  bad 1-clean-release-schema "expected PREFLIGHT_OK, got $(guard s1)"; sed -n 1,20p "$TMP/guard.out"
fi

# ---------------------------------------------------------------- scenario 2
# A safe (letter-prefixed) facility must NOT be flagged. This is the negative
# control: it is what the production facility set looks like (RSK, PKM, DEMO…).
facility s1 RSK '00000000-0000-0000-0000-0000000b0001'
if [ "$(guard s1)" = "PREFLIGHT_OK" ]; then
  ok 2-safe-facility-not-flagged "letter short_code RSK stays PREFLIGHT_OK"
else
  bad 2-safe-facility-not-flagged "RSK was flagged; expected PREFLIGHT_OK, got $(guard s1)"
fi

# ---------------------------------------------------------------- scenario 3
# The counterexample: a digit/separator-heavy short_code the OLD release accepted
# (it had no validateShortCode) and whose rendered queue number the strengthened
# constraint REJECTS. Must be flagged AND named.
facility s1 'A1234567-89' '00000000-0000-0000-0000-0000000b0002'
v=$(guard s1)
if [ "$v" = "PREFLIGHT_BLOCKED" ] \
   && grep -q '00000000-0000-0000-0000-0000000b0002' "$TMP/guard.out"; then
  ok 3-unsafe-facility-detected "PREFLIGHT_BLOCKED and the facility is named"
else
  bad 3-unsafe-facility-detected "expected BLOCKED + named facility, got $v"; sed -n 1,40p "$TMP/guard.out"
fi

# ---------------------------------------------------------------- scenario 4
# A weak (pre-0011) schema carrying an existing row the release predicate
# rejects: 0011's validated ADD CONSTRAINT would abort, so the guard must report
# the row count and NOT report OK.
mk s4
apply_all s4
weaken s4
px -d s4 -c "INSERT INTO notification_outbox
  (channel,template_key,subject,body_template,recipient_type,recipient_contact_masked,recipient_contact_hash)
  VALUES ('dev','t','Status Check-in Sigap',
          'Check-in Anda berhasil. Nomor antrean: A1234567-89-0001.','patient','+62••••0002',sha256('p'::bytea))" \
  >/dev/null 2>&1 || die "insert violating row"
v=$(guard s4)
if [ "$v" = "PREFLIGHT_BLOCKED" ] && grep -qE '^\s*1\s*$' "$TMP/guard.out"; then
  ok 4-violating-row-detected "PREFLIGHT_BLOCKED with a non-zero row count"
else
  bad 4-violating-row-detected "expected BLOCKED + count 1, got $v"; sed -n 1,40p "$TMP/guard.out"
fi

# ---------------------------------------------------------------- scenario 5
# The same weak schema with NO violating row must NOT be blocked for row reasons:
# an existing-but-clean database is exactly the expected production state before
# 0011, and a guard that flags it is useless.
mk s5
apply_all s5
weaken s5
if [ "$(guard s5)" = "PREFLIGHT_OK" ]; then
  ok 5-clean-weak-schema "weak schema with no violating row stays PREFLIGHT_OK"
else
  bad 5-clean-weak-schema "expected PREFLIGHT_OK, got $(guard s5)"; sed -n 1,40p "$TMP/guard.out"
fi

# ---------------------------------------------------------------- scenario 6
# READ-ONLY PROOF. The guard must not mutate anything, and must not surface
# message content in its output.
before=$(pxa -d s4 -c "SELECT count(*) FROM notification_outbox")
guard s4 >/dev/null 2>&1
after=$(pxa -d s4 -c "SELECT count(*) FROM notification_outbox")
if [ "$before" = "$after" ] && ! grep -q 'A1234567-89-0001' "$TMP/guard.out"; then
  ok 6-read-only-and-no-content "row count unchanged; no subject/body text emitted"
else
  bad 6-read-only-and-no-content "before=$before after=$after; content leak=$(grep -c 'A1234567-89-0001' "$TMP/guard.out")"
fi

printf 'preflight-0011 tests: %d pass, %d fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
