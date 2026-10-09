#!/bin/sh
# test-guard-shallow-clone.sh — negative tests for the CI ops-guards checkout.
#
# The defect this pins: `scripts/ops/test-production-deploy-guards.sh` diffs
# `origin/main..HEAD` and `6d7f940..HEAD`. At actions/checkout's default
# fetch-depth of 1 on a `pull_request` event, refs/remotes/origin/main is never
# created, so `git diff origin/main..HEAD` writes to stderr (discarded) and
# prints nothing. The guard then failed with a MISLEADING
# "origin/main..HEAD migration diff is EMPTY", which reads like a real
# inventory problem rather than a missing-history problem.
#
# These tests build real clones at two depths and assert:
#   * shallow (no origin/main)  -> guard FAILS and NAMES the missing ref
#   * shallow                   -> guard does NOT emit the misleading message
#   * full    (origin/main)     -> guard PASSES
#
# Exit codes: 0 pass, 1 fail, 2 missing prerequisite (git or a base ref absent).
set -u

REPO=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
GUARD=scripts/ops/test-production-deploy-guards.sh
BRANCH=$(git -C "$REPO" rev-parse --abbrev-ref HEAD 2>/dev/null)

command -v git >/dev/null 2>&1 || { echo "SKIP: git not found"; exit 2; }
[ -n "$BRANCH" ] || { echo "SKIP: cannot determine branch (detached HEAD?)"; exit 2; }
# The shallow test needs a base ref that the shallow clone will NOT have.
git -C "$REPO" rev-parse --verify --quiet origin/main^{commit} >/dev/null 2>&1 \
  || { echo "SKIP: origin/main is absent in the source repo; run git fetch first"; exit 2; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-guard-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-30s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-30s %s\n' "$1" "$2"; }

run_guard() { # dir -> writes output to $dir/out.txt, echoes exit code
  ( cd "$1" && sh "$GUARD" ) >"$1/out.txt" 2>&1
  echo $?
}

# The clone carries the COMMITTED guard. Copy the working-tree guard and docs
# over it so this suite tests the change under review, not the last commit —
# otherwise the negative tests would only start passing after a commit.
sync_worktree() {
  cp -f "$REPO/$GUARD" "$1/$GUARD"
  mkdir -p "$1/docs/operations"
  cp -f "$REPO"/docs/operations/*.md "$1/docs/operations/" 2>/dev/null || true
}

# --- 1. SHALLOW clone: origin/main absent -----------------------------------
echo "== building shallow clone =="
git clone --quiet --depth 1 --branch "$BRANCH" "file://$REPO" "$TMP/shallow" >/dev/null 2>&1 \
  || { echo "SKIP: shallow clone failed (file:// may be unavailable)"; exit 2; }
sync_worktree "$TMP/shallow"

if git -C "$TMP/shallow" rev-parse --verify --quiet origin/main^{commit} >/dev/null 2>&1; then
  echo "SKIP: the shallow clone unexpectedly has origin/main; cannot test the absent-ref path"
  exit 2
fi

rc=$(run_guard "$TMP/shallow")
if [ "$rc" -ne 0 ]; then
  ok "shallow/fails" "guard exits $rc (correct: cannot verify inventory)"
else
  bad "shallow/fails" "guard passed without origin/main — a missing base ref must not read as success"
fi

if grep -q "origin/main" "$TMP/shallow/out.txt" && grep -qi "not available in this checkout" "$TMP/shallow/out.txt"; then
  ok "shallow/names-missing-ref" "message names origin/main and the fix"
else
  bad "shallow/names-missing-ref" "message does not name the missing ref: $(grep -i 'origin/main' "$TMP/shallow/out.txt" | head -1)"
fi

if grep -q "migration diff is EMPTY" "$TMP/shallow/out.txt"; then
  bad "shallow/no-misleading-msg" "still reports the misleading 'diff is EMPTY' for a missing ref"
else
  ok "shallow/no-misleading-msg" "does not misreport a missing ref as an empty diff"
fi

# A skip must never downgrade an observed failure: the shallow assertions above
# have already run, so if any failed, report FAIL now rather than exiting 2
# (MISSING PREREQUISITE) and letting a CI that tolerates 2 read it as a skip.
finish_or_skip() {
  if [ "$failed" -ne 0 ]; then
    printf 'guard shallow-clone tests: %s pass, %s fail\n' "$pass" "$failed"
    exit 1
  fi
  printf 'SKIP: %s\n' "$1"
  exit 2
}

# --- 2. FULL clone: origin/main present -------------------------------------
echo "== building full clone =="
git clone --quiet "file://$REPO" "$TMP/full" >/dev/null 2>&1 \
  || finish_or_skip "full clone failed"
sync_worktree "$TMP/full"

if git -C "$TMP/full" rev-parse --verify --quiet origin/main^{commit} >/dev/null 2>&1; then
  rc=$(run_guard "$TMP/full")
  if [ "$rc" -eq 0 ]; then
    ok "full/passes" "guard exits 0 with full history"
  else
    bad "full/passes" "guard exits $rc with full history; tail: $(tail -3 "$TMP/full/out.txt" | tr '\n' ' ')"
  fi
else
  bad "full/passes" "full clone lacks origin/main"
fi

printf 'guard shallow-clone tests: %s pass, %s fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ]
