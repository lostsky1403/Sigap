#!/bin/sh
# test-migration-inventory-policy.sh — negative/positive matrix for the four-context
# migration inventory guard (scripts/ops/check-migration-inventory.sh).
#
# Builds throwaway git repos containing the REAL migration files (so checksums are
# genuine). No branch, tag, database or production resource is touched.
#
# Usage: sh scripts/ops/test-migration-inventory-policy.sh
# Exit 0 iff every case behaves as expected.

set -u
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
GUARD="$SCRIPT_DIR/check-migration-inventory.sh"
SRC="$REPO_ROOT/packages/db/migrations"

PASS=0 FAIL=0
assert() { # name expected actual
	if [ "$2" -eq "$3" ]; then PASS=$((PASS + 1)); printf 'PASS  %s (exit %s)\n' "$1" "$3";
	else FAIL=$((FAIL + 1)); printf 'FAIL  %s: expected %s, got %s\n' "$1" "$2" "$3"; [ -f "$OUT" ] && sed -n '1,6p' "$OUT"; fi
}

setup() { # [omit_0005]
	TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-migpol-$$")
	OUT="$TMP/out"
	REPO="$TMP/repo"
	git init -q "$REPO" || exit 1
	cd "$REPO" || exit 1
	git config user.name 'policy-test'
	git config user.email 'policy-test@example.org'
	mkdir -p packages/db/migrations docs/operations
	: > docs/operations/DEPLOYMENT_RUNBOOK.md
	cp "$SRC"/000[1-9]_*.sql "$SRC"/001[01]_*.sql packages/db/migrations/ 2>/dev/null
	[ "${1:-}" = 'omit0005' ] && rm -f packages/db/migrations/0005_appointments.sql
	git add -A
	git commit -q -m 'baseline 0001-0011'
	BASE=$(git rev-parse HEAD)
	git update-ref refs/remotes/origin/main "$BASE"
	cp "$SRC"/0012_*.sql "$SRC"/0013_*.sql "$SRC"/0014_*.sql packages/db/migrations/
	git add -A
	git commit -q -m 'regional development migrations'
}

setup_b() { # baseline 0001-0014 as origin/main (ordinary-branch base)
	TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-migpol-$$")
	OUT="$TMP/out"
	REPO="$TMP/repo"
	git init -q "$REPO" || exit 1
	cd "$REPO" || exit 1
	git config user.name 'policy-test'
	git config user.email 'policy-test@example.org'
	mkdir -p packages/db/migrations docs/operations
	: > docs/operations/DEPLOYMENT_RUNBOOK.md
	cp "$SRC"/000[1-9]_*.sql "$SRC"/001[01]_*.sql "$SRC"/0012_*.sql "$SRC"/0013_*.sql "$SRC"/0014_*.sql packages/db/migrations/ 2>/dev/null
	git add -A
	git commit -q -m 'baseline 0001-0014'
	git update-ref refs/remotes/origin/main HEAD
}

run() { sh "$GUARD" "$@" >"$OUT" 2>&1; echo $?; }

# Positive: declared 0012-0014 on the regional development branch.
setup
assert 'C: declared regional set accepted' 0 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 1: an applied migration is modified.
setup
printf '\n-- unauthorized edit --\n' >> packages/db/migrations/0006_notifications.sql
git add -A && git commit -q -m 'edit applied migration'
assert 'negative: applied migration modified' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 2: an applied migration is deleted.
setup
rm -f packages/db/migrations/0007_checkin_constraints.sql
git add -A && git commit -q -m 'delete applied migration'
assert 'negative: applied migration deleted' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 3: an unexpected migration is added.
setup
printf 'SELECT 1;\n' > packages/db/migrations/0015_unexpected.sql
git add -A && git commit -q -m 'add unexpected migration'
assert 'negative: unexpected migration added' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 4: a declared migration is omitted.
setup
rm -f packages/db/migrations/0013_organizations.sql
git add -A && git commit -q -m 'omit declared migration'
assert 'negative: declared migration omitted' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 5: incorrect version ordering (gap in applied history).
setup omit0005
assert 'negative: incorrect order/version' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline "$BASE")"

# Negative 6: incorrect baseline.
setup
assert 'negative: incorrect baseline' 1 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline HEAD)"

# Negative 7: incorrect branch context.
setup
assert 'negative: incorrect branch context' 1 "$(run --branch release/sigap-regional-v1 --context C --baseline "$BASE")"

# Negative 8: unauthorized migration accepted via branch-name override.
setup
SIGAP_RELEASE_BRANCH=feat/sigap-selfhosted-auth sh "$GUARD" --branch hotfix/try >"$OUT" 2>&1
assert 'negative: branch-name override to dev branch' 1 $?

# Positive B: an ordinary branch adding only the declared hotfix migration passes.
setup_b
cp "$SRC"/0015_*.sql packages/db/migrations/
git add -A && git commit -q -m 'add declared hotfix migration'
assert 'B: declared hotfix migration accepted' 0 "$(run --branch fix/queue-counter-race-0015)"

# Negative B1: an ordinary branch adding an undeclared migration fails.
setup_b
printf 'SELECT 1;\n' > packages/db/migrations/0016_unexpected.sql
git add -A && git commit -q -m 'add unexpected migration'
assert 'negative B: unexpected migration added' 1 "$(run --branch fix/queue-counter-race-0015)"

# Negative B2: a tampered declared hotfix migration fails the checksum.
setup_b
cp "$SRC"/0015_*.sql packages/db/migrations/
printf '\n-- unauthorized edit --\n' >> packages/db/migrations/0015_queue_ticket_service_date_unique.sql
git add -A && git commit -q -m 'tamper declared hotfix migration'
assert 'negative B: hotfix checksum mismatch' 1 "$(run --branch fix/queue-counter-race-0015)"

setup_b2() { # baseline 0001-0015 as origin/main (ordinary-branch base for 0016)
	TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-migpol-$$")
	OUT="$TMP/out"
	REPO="$TMP/repo"
	git init -q "$REPO" || exit 1
	cd "$REPO" || exit 1
	git config user.name 'policy-test'
	git config user.email 'policy-test@example.org'
	mkdir -p packages/db/migrations docs/operations
	: > docs/operations/DEPLOYMENT_RUNBOOK.md
	cp "$SRC"/000[1-9]_*.sql "$SRC"/001[01]_*.sql "$SRC"/0012_*.sql "$SRC"/0013_*.sql "$SRC"/0014_*.sql "$SRC"/0015_*.sql packages/db/migrations/ 2>/dev/null
	git add -A
	git commit -q -m 'baseline 0001-0015'
	git update-ref refs/remotes/origin/main HEAD
}

# Positive B3: an ordinary branch adding only the declared 0016 hotfix passes.
setup_b2
cp "$SRC"/0016_*.sql packages/db/migrations/
git add -A && git commit -q -m 'add declared hotfix migration 0016'
assert 'B: declared hotfix 0016 accepted' 0 "$(run --branch fix/data-honesty-phase1)"

# Negative B3: a tampered declared 0016 hotfix fails the checksum.
setup_b2
cp "$SRC"/0016_*.sql packages/db/migrations/
printf '\n-- unauthorized edit --\n' >> packages/db/migrations/0016_facilities_geo_demo.sql
git add -A && git commit -q -m 'tamper declared hotfix migration 0016'
assert 'negative B: hotfix 0016 checksum mismatch' 1 "$(run --branch fix/data-honesty-phase1)"

# Fail-closed: an unresolvable baseline must not pass.
setup
assert 'fail-closed: unresolvable baseline' 2 "$(run --branch feat/sigap-selfhosted-auth --context C --baseline 0000000000000000000000000000000000000000)"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
printf 'PASS: MIGRATION_INVENTORY_POLICY\n'
