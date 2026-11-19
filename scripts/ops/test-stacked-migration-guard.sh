#!/bin/sh
# test-stacked-migration-guard.sh — positive/negative matrix for context E
# (stacked-child migration validation on scripts/ops/check-migration-inventory.sh).
#
# Builds THROWAWAY git repos that simulate a GitHub pull_request merge checkout:
#   baseline (0001-0011)  ->  parent (adds 0012-0014)  ->  child (non-migration)
#   merge commit with two parents: HEAD^1 = parent tip, HEAD^2 = child tip.
# Uses the REAL migration files so SHA-256 assertions are genuine. No branch,
# tag, worktree, database or production resource is touched.
#
# Usage: sh scripts/ops/test-stacked-migration-guard.sh
# Exit 0 iff every case behaves as expected.

set -u
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
GUARD="$SCRIPT_DIR/check-migration-inventory.sh"
SRC="$REPO_ROOT/packages/db/migrations"

PASS=0 FAIL=0
OUT=''
assert() { # name expected actual
  if [ "$2" -eq "$3" ]; then PASS=$((PASS + 1)); printf 'PASS  %s (exit %s)\n' "$1" "$3";
  else FAIL=$((FAIL + 1)); printf 'FAIL  %s: expected %s, got %s\n' "$1" "$2" "$3"; [ -f "$OUT" ] && sed -n '1,8p' "$OUT"; fi
}

# setup [parent_variant] [child_variant] [head_ref] [base_ref] [repo] [event] [spoof]
# Creates a disposable fixture; sets globals TMP, OUT, REPO, BASE, PARENT, CHILD.
setup() {
  pvar=${1:-parent_ok}; cvar=${2:-child_ok}
  href=${3:-feat/sigap-identity-deactivation}; bref=${4:-feat/sigap-selfhosted-auth}
  repo=${5:-lostsky1403/Sigap}; evt=${6:-pull_request}; spoof=${7:-none}
  TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-stacked-$$")
  OUT="$TMP/out"
  REPO="$TMP/repo"
  git init -q "$REPO" || exit 1
  cd "$REPO" || exit 1
  git config user.name 'stacked-guard-test'
  git config user.email 'stacked-guard-test@example.org'
  mkdir -p packages/db/migrations docs/operations
  : > docs/operations/DEPLOYMENT_RUNBOOK.md
  cp "$SRC"/000[1-9]_*.sql "$SRC"/001[01]_*.sql packages/db/migrations/ 2>/dev/null
  git add -A
  git commit -q -m 'baseline 0001-0011'
  BASE=$(git rev-parse HEAD)
  git update-ref refs/remotes/origin/main "$BASE"

  # Parent commit
  git checkout -q -b feat/sigap-selfhosted-auth
  case "$pvar" in
    parent_ok) cp "$SRC"/0012_*.sql "$SRC"/0013_*.sql "$SRC"/0014_*.sql packages/db/migrations/ ;;
    parent_missing) cp "$SRC"/0012_*.sql "$SRC"/0014_*.sql packages/db/migrations/ ;;
    parent_extra) cp "$SRC"/0012_*.sql "$SRC"/0013_*.sql "$SRC"/0014_*.sql packages/db/migrations/; printf 'SELECT 1;\n' > packages/db/migrations/0015_unexpected.sql ;;
  esac
  git add -A && git commit -q -m 'parent regional migrations'
  PARENT=$(git rev-parse HEAD)
  git update-ref refs/remotes/origin/feat/sigap-selfhosted-auth "$PARENT"

  # Child commit from parent (non-migration content only, unless variant says otherwise)
  git checkout -q -b feat/sigap-identity-deactivation
  printf 'stacked feature\n' > docs/operations/STACKED.md
  git add -A && git commit -q -m 'child feature'
  case "$cvar" in
    child_add15) printf 'SELECT 1;\n' > packages/db/migrations/0015_unexpected.sql; git add -A && git commit -q -m 'child adds migration' ;;
    child_mod12) printf '\n-- child edit --\n' >> packages/db/migrations/0012_service_day_wib.sql; git add -A && git commit -q -m 'child modifies 0012' ;;
    child_del13) rm -f packages/db/migrations/0013_organizations.sql; git add -A && git commit -q -m 'child deletes 0013' ;;
    child_ren14) mv packages/db/migrations/0014_patient_account.sql packages/db/migrations/0014_renamed.sql; git add -A && git commit -q -m 'child renames 0014' ;;
  esac
  CHILD=$(git rev-parse HEAD)
  git update-ref refs/remotes/origin/feat/sigap-identity-deactivation "$CHILD"

  # Build the PR merge commit: HEAD^1 = parent tip, HEAD^2 = child tip.
  TREE=$(git rev-parse HEAD^{tree})
  if [ "$spoof" = 'no_merge' ]; then
    # single-parent commit -> must fail closed
    MERGE=$(printf 'not a merge\n' | git commit-tree "$TREE" -p "$PARENT")
  elif [ "$spoof" = 'wrong_parent' ]; then
    # merge whose first parent is the BASE, not the verified parent
    MERGE=$(printf 'wrong parent\n' | git commit-tree "$TREE" -p "$BASE" -p "$CHILD")
  else
    MERGE=$(printf 'PR merge\n' | git commit-tree "$TREE" -p "$PARENT" -p "$CHILD")
  fi
  git checkout -q --detach "$MERGE"

  # Export the PR context the guard reads.
  GITHUB_EVENT_NAME=$evt
  GITHUB_BASE_REF=$bref
  GITHUB_HEAD_REF=$href
  GITHUB_REPOSITORY=$repo
}

run() { # [branch]
  GITHUB_EVENT_NAME="$GITHUB_EVENT_NAME" GITHUB_BASE_REF="$GITHUB_BASE_REF" \
  GITHUB_HEAD_REF="$GITHUB_HEAD_REF" GITHUB_REPOSITORY="$GITHUB_REPOSITORY" \
  sh "$GUARD" --branch "${1:-feat/sigap-identity-deactivation}" --context E --baseline "$BASE" >"$OUT" 2>&1
  echo $?
}

# 1. Valid parent + migration-free child -> PASS
setup; assert 'E positive: valid parent + migration-free child' 0 "$(run)"
# 2-5. Child migration mutations -> FAIL
setup parent_ok child_add15; assert 'E neg: child adds migration' 1 "$(run)"
setup parent_ok child_mod12; assert 'E neg: child modifies 0012' 1 "$(run)"
setup parent_ok child_del13; assert 'E neg: child deletes 0013' 1 "$(run)"
setup parent_ok child_ren14; assert 'E neg: child renames 0014' 1 "$(run)"
# 6. Child checksum drift (modify declared file content without git name-status showing M at merge tree)
setup parent_ok child_mod12; assert 'E neg: child checksum drift' 1 "$(run)"
# 7-8. Parent inventory violations -> FAIL
setup parent_missing; assert 'E neg: parent lacks declared migration' 1 "$(run)"
setup parent_extra; assert 'E neg: parent has unexpected migration' 1 "$(run)"
# 9. Incorrect baseline -> FAIL
setup; GITHUB_BASE_REF='feat/wrong-base'; assert 'E neg: wrong base ref' 2 "$(run)"
# 10. Parent not ancestor of child -> FAIL (wrong merge parent)
setup parent_ok child_ok feat/sigap-identity-deactivation feat/sigap-selfhosted-auth lostsky1403/Sigap pull_request wrong_parent
assert 'E neg: parent not ancestor of child' 1 "$(run)"
# 11. Missing merge parents -> FAIL CLOSED
setup parent_ok child_ok feat/sigap-identity-deactivation feat/sigap-selfhosted-auth lostsky1403/Sigap pull_request no_merge
assert 'E fail-closed: not a PR merge checkout' 2 "$(run)"
# 12. Different child branch -> binding FAIL (not a stacked exemption)
setup; assert 'E neg: different child branch' 1 "$(run feat/other-child)"
# 13. Different base branch -> FAIL CLOSED
setup parent_ok child_ok feat/sigap-identity-deactivation feat/other-base; assert 'E neg: different base branch' 2 "$(run)"
# 14. Missing/spoofed PR context -> FAIL CLOSED
setup parent_ok child_ok feat/sigap-identity-deactivation feat/sigap-selfhosted-auth lostsky1403/Sigap push
assert 'E fail-closed: non-pull_request event' 2 "$(run)"
setup parent_ok child_ok feat/sigap-identity-deactivation feat/sigap-selfhosted-auth evil/repo
assert 'E fail-closed: wrong repository' 2 "$(run)"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
printf 'PASS: STACKED_MIGRATION_GUARD\n'
