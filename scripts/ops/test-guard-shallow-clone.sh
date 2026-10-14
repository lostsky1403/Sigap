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

command -v git >/dev/null 2>&1 || { echo "SKIP: git not found"; exit 2; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-guard-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-30s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-30s %s\n' "$1" "$2"; }

# A fixture that cannot be BUILT is a broken test, not a skipped one.
#
# Exit 2 means MISSING PREREQUISITE, and this suite used to reach it for a reason
# that had nothing to do with a missing prerequisite: the shallow clone failed, so
# not one assertion ran. On CI that is a silent skip of a mandatory safety check.
# The job definition guarantees git and a cloneable repository, so under CI a
# fixture failure is a hard FAIL. Locally (no CI variable) 2 is still reported,
# because a developer without a fetchable origin/main should not see a red suite.
#
# A skip must also never downgrade an observed failure: if any assertion already
# ran and failed, report FAIL rather than exiting 2.
fixture_failed() {
  if [ "$failed" -ne 0 ]; then
    printf 'guard shallow-clone tests: %s pass, %s fail\n' "$pass" "$failed"
    exit 1
  fi
  if [ -n "${CI:-}" ]; then
    printf 'FAIL: %s\n' "$1"
    printf '      CI installs the prerequisites for this suite, so a fixture failure here\n'
    printf '      is a broken test, not a legitimate skip. Refusing to report it as exit 2.\n'
    exit 1
  fi
  printf 'SKIP: %s\n' "$1"
  exit 2
}

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
# NO --branch. `git clone --depth 1 --branch "$(git rev-parse --abbrev-ref HEAD)"`
# was the defect: on a `pull_request` event actions/checkout leaves HEAD on
# `refs/remotes/pull/<N>/merge`, so `abbrev-ref` returns e.g. `pull/85/merge`. That
# name exists in the source repo only as a REMOTE-TRACKING ref, and `--branch`
# resolves LOCAL branch names, so the clone died with
#
#     warning: Could not find remote branch pull/85/merge to clone.
#     fatal: Remote branch pull/85/merge not found in upstream origin
#
# [OBSERVED] in CI run 38289010215, and reproduced locally by pointing --branch at a
# ref that exists only under refs/remotes/. The fixture was therefore never built,
# the suite exited 2 (MISSING PREREQUISITE), and ZERO assertions ran — a mandatory
# safety check silently skipped on every CI run while the local run passed.
#
# Cloning the source's own HEAD with no --branch is both simpler and a MORE faithful
# reproduction of the CI checkout: in CI the source HEAD *is* the merge ref, so the
# clone is shallow (one commit) with no refs/remotes/origin/main — exactly the state
# the guard must detect.
echo "== building shallow clone =="
git clone --quiet --depth 1 "file://$REPO" "$TMP/shallow" >/dev/null 2>&1 \
  || fixture_failed "shallow clone failed (is file:// transport available?)"
sync_worktree "$TMP/shallow"

# Guard the fixture itself: if the clone were NOT shallow, or it somehow carried
# origin/main, the assertions below would not test the absent-ref path at all.
[ -d "$TMP/shallow/.git" ] \
  || fixture_failed "the shallow clone has no .git directory"
if git -C "$TMP/shallow" rev-parse --verify --quiet origin/main^{commit} >/dev/null 2>&1; then
  fixture_failed "the shallow clone unexpectedly has origin/main; cannot test the absent-ref path"
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

# The shallow assertions above MUST have run. If the fixture had been skipped, the
# suite would have exited before reaching this line with zero assertions recorded —
# which is precisely the failure mode this file exists to prevent. Make that
# explicit so a future fixture regression cannot present as a green run.
[ "$pass" -ge 3 ] \
  || bad "shallow/ran-assertions" "only $pass shallow assertion(s) ran; the fixture did not exercise the absent-ref path"

# --- 2. FULL clone: origin/main present -------------------------------------
# This is the ONLY assertion that needs a base ref in the SOURCE repo, so the
# prerequisite is checked here rather than at the top of the file: a source repo
# without main can still prove the shallow path, and skipping the whole suite for
# it would suppress the assertions that do not depend on it.
#
# BOTH spellings are accepted, because the two environments differ: a developer
# clone keeps `main` under refs/heads/, while actions/checkout with fetch-depth 0
# on a `pull_request` keeps it under refs/remotes/origin/.
MAIN_SHA=$(git -C "$REPO" rev-parse --verify --quiet refs/remotes/origin/main^{commit} 2>/dev/null)
[ -n "$MAIN_SHA" ] || MAIN_SHA=$(git -C "$REPO" rev-parse --verify --quiet refs/heads/main^{commit} 2>/dev/null)
[ -n "$MAIN_SHA" ] \
  || fixture_failed "no main ref in the source repo (checked refs/remotes/origin/main and refs/heads/main)"

echo "== building full clone =="
git clone --quiet "file://$REPO" "$TMP/full" >/dev/null 2>&1 \
  || fixture_failed "full clone failed"

# A clone inherits only the source's refs/heads/*, mapped to refs/remotes/origin/*.
# In CI the source keeps main under refs/remotes/origin/, so the clone would have NO
# origin/main and the positive assertion could never pass — the fixture would be
# testing nothing while looking green. [OBSERVED] in the CI simulation: the full
# clone reported "full clone lacks origin/main" until this fetch was added.
#
# Fetching the source's remote-tracking namespace as well reproduces exactly what
# actions/checkout's `fetch-depth: 0` makes available, and the explicit update-ref is
# the backstop that guarantees the ref regardless of transport ref-mapping.
git -C "$TMP/full" fetch --quiet --no-tags "file://$REPO" \
  "+refs/remotes/origin/*:refs/remotes/origin/*" >/dev/null 2>&1 || true
git -C "$TMP/full" rev-parse --verify --quiet origin/main^{commit} >/dev/null 2>&1 \
  || git -C "$TMP/full" update-ref refs/remotes/origin/main "$MAIN_SHA"
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

# --- 3. REGRESSION: a pull/<N>/merge HEAD must not break the fixture --------
#
# This is the case that was silently skipped in CI. actions/checkout on a
# `pull_request` event leaves HEAD on refs/remotes/pull/<N>/merge, so
# `git rev-parse --abbrev-ref HEAD` returns `pull/85/merge`. The fixture must not
# depend on that name, and both halves are pinned here: the OLD form must FAIL
# (that is the defect), and the NEW form must SUCCEED with no origin/main (that is
# the fix). Runs on any POSIX host with git, so a GitHub-hosted Ubuntu runner
# exercises it on every CI run — no network and no GitHub API involved.
echo "== regression: CI-like HEAD (pull/N/merge) =="
SIM="$TMP/cihead"
mkdir -p "$SIM/src"
( cd "$SIM/src" \
  && git init -q -b main . \
  && git -c user.email=t@t -c user.name=t commit -q --allow-empty -m base \
  && git update-ref refs/remotes/pull/85/merge HEAD \
  && git symbolic-ref HEAD refs/remotes/pull/85/merge ) >/dev/null 2>&1 \
  || fixture_failed "could not build the CI-like HEAD fixture"

ci_head=$(git -C "$SIM/src" rev-parse --abbrev-ref HEAD)
if [ "$ci_head" = "pull/85/merge" ]; then
  ok "cihead/abbrev-ref" "abbrev-ref is '$ci_head' (the CI spelling)"
else
  bad "cihead/abbrev-ref" "expected 'pull/85/merge', got '$ci_head'"
fi

# The OLD form must FAIL. If it ever starts succeeding, this case has stopped
# reproducing CI and the guard against reintroducing --branch is worthless.
if git clone -q --depth 1 --branch "$ci_head" "file://$SIM/src" "$SIM/old" >/dev/null 2>&1; then
  bad "cihead/old-branch-form" "clone --branch '$ci_head' SUCCEEDED; this case no longer reproduces the CI failure"
else
  ok "cihead/old-branch-form" "clone --branch '$ci_head' fails, exactly as it did in CI"
fi

# The NEW form must SUCCEED, and must yield no origin/main — otherwise the shallow
# fixture would not exercise the absent-ref path it exists to test.
if git clone -q --depth 1 "file://$SIM/src" "$SIM/new" >/dev/null 2>&1; then
  if git -C "$SIM/new" rev-parse --verify --quiet origin/main >/dev/null 2>&1; then
    bad "cihead/no-branch-form" "clone succeeded but carries origin/main; not a valid shallow fixture"
  else
    ok "cihead/no-branch-form" "clone succeeds under a CI-like HEAD and has no origin/main"
  fi
else
  bad "cihead/no-branch-form" "clone without --branch failed under a CI-like HEAD"
fi

printf 'guard shallow-clone tests: %s pass, %s fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ]
