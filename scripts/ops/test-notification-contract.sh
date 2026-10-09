#!/bin/sh
# Disposable-database harness for the notification contract tests.
#
#   sh scripts/ops/test-notification-contract.sh
#
# Creates a throwaway PostgreSQL cluster, runs the real Go tests against it, and
# destroys the cluster. Synthetic data only. Never contacts production.
#
# Exit codes: 0 pass, 1 fail, 2 missing prerequisite (no local PostgreSQL).
set -u

# A crashed earlier invocation (the trap fires on EXIT/INT/TERM, not SIGKILL)
# can leave a cluster holding the default port, which would make pg_ctl start
# fail and be misreported as a FAIL rather than a prerequisite problem. So the
# port is probed: an explicit SIGAP_CONTRACT_PG_PORT is used strictly, otherwise
# successive ports are tried.
PORT_OVERRIDE=${SIGAP_CONTRACT_PG_PORT:-}
PORT=${PORT_OVERRIDE:-55472}
DB=sigap_contract
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

find_bin() {
  command -v "$1" 2>/dev/null && return 0
  for d in "/c/Program Files/PostgreSQL"/*/bin "/usr/lib/postgresql"/*/bin /usr/local/pgsql/bin; do
    [ -x "$d/$1" ] && { printf '%s\n' "$d/$1"; return 0; }
  done
  return 1
}
INITDB=$(find_bin initdb) || { echo "SKIP: initdb not found (no local PostgreSQL)"; exit 2; }
PGCTL=$(find_bin pg_ctl)   || { echo "SKIP: pg_ctl not found"; exit 2; }
PSQL=$(find_bin psql)      || { echo "SKIP: psql not found"; exit 2; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-contract-$$")
mkdir -p "$TMP"
CL="$TMP/cluster"
cleanup() { "$PGCTL" -D "$CL" -m immediate stop >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT INT TERM

"$INITDB" -D "$CL" -U postgres --auth=trust -E UTF8 >"$TMP/initdb.log" 2>&1 \
  || { echo "FAIL: initdb"; tail -10 "$TMP/initdb.log"; exit 1; }

start_cluster() {
  # Try the requested port, then the next few, so a stale cluster on the default
  # port degrades to a different port instead of a spurious FAIL.
  attempts=0
  while [ "$attempts" -lt 8 ]; do
    if "$PGCTL" -D "$CL" -o "-p $PORT -c listen_addresses=127.0.0.1" \
         -l "$TMP/pg.log" -w start >"$TMP/start.log" 2>&1; then
      return 0
    fi
    [ -n "$PORT_OVERRIDE" ] && break   # an explicit port is honoured, not retried
    PORT=$((PORT + 1))
    attempts=$((attempts + 1))
  done
  return 1
}

start_cluster || { echo "FAIL: cluster start"; tail -10 "$TMP/start.log"; exit 1; }

# Two databases only. The migration tests provision their OWN isolated database
# from whatever DATABASE_URL points at (see migPool), because they assert they
# just applied migrations 0001..0010 — which is false on any database that
# already has version 11 recorded, including CI's shared one.
"$PSQL" -h 127.0.0.1 -p "$PORT" -U postgres -q -v ON_ERROR_STOP=1 \
  -c "CREATE DATABASE ${DB}_agree" \
  -c "CREATE DATABASE ${DB}_mig" >/dev/null 2>&1 || { echo "FAIL: createdb"; exit 1; }

cd "$REPO/apps/api" || exit 1

# Each section captures into a file and reads $? from the `go test` invocation
# itself. Do NOT use `go test ... | tail`: a pipeline's exit status is the LAST
# command's, so `rc=$?` after a pipe would be tail's status — always 0 — and the
# gate below would report PASS while every suite failed.
run_suite() { # name, database, -run pattern
  label=$1; dbname=$2; pattern=$3
  printf '\n=== %s ===\n' "$label"
  DATABASE_URL="postgresql://postgres@127.0.0.1:$PORT/${dbname}?sslmode=disable" \
    go test ./internal/notification/ -run "$pattern" -count=1 >"$TMP/$dbname.out" 2>&1
  rc=$?
  tail -20 "$TMP/$dbname.out"
  return $rc
}

run_suite "agreement test (fresh release schema, real Enqueue)" "${DB}_agree" 'TestEnqueue_GoAndDatabaseAgree'
rc1=$?

run_suite "transition: compatible history (0001-0010 weak -> 0011 converges)" "${DB}_mig" 'TestMigration0011_Transition_CompatibleHistory'
rc2=$?

run_suite "transition: violating history (0011 must fail closed)" "${DB}_mig" 'TestMigration0011_Transition_ViolatingHistory'
rc3=$?

run_suite "transition: mixed strong/weak and absent-constraint branches" "${DB}_mig" 'TestMigration0011_Transition_(MixedStateRefused|AbsentConstraintsAdded)'
rc4=$?

run_suite "lock/timeout: ACCESS EXCLUSIVE blocking and lock_timeout" "${DB}_mig" 'TestMigration0011_(BlocksOnConcurrentReader|BlocksOnConcurrentWriter|LockTimeoutFailsClosed)'
rc5=$?

run_suite "rendered queue number agreement (short_code -> queue_number)" "${DB}_agree" 'TestRenderedQueueNumber_GoAndDatabaseAgree'
rc6=$?

run_suite "predicate corpus vs the live constraint" "${DB}_agree" 'TestPredicate_CorpusAgreesWithTheDatabase'
rc7=$?

if [ "$rc1" -ne 0 ] || [ "$rc2" -ne 0 ] || [ "$rc3" -ne 0 ] || [ "$rc4" -ne 0 ] || [ "$rc5" -ne 0 ] || [ "$rc6" -ne 0 ] || [ "$rc7" -ne 0 ]; then
  echo "notification-contract tests: FAIL (agree=$rc1 ok=$rc2 bad=$rc3 mixed=$rc4 lock=$rc5 queue=$rc6 corpus=$rc7)"
  exit 1
fi
echo "notification-contract tests: PASS"
exit 0
