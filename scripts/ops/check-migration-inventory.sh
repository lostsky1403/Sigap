#!/bin/sh
# check-migration-inventory.sh — fail-closed migration inventory guard with four explicit branch contexts.
#
# Contexts (the ONLY supported interpretations; any other shape fails closed):
#   A  historical design release  — design/ui-ux-overhaul, anchor d7705f09, 0006 hardening + 0011
#   B  ordinary hotfix/other      — no migration change relative to origin/main
#   C  regional development       — feat/sigap-selfhosted-auth, baseline 4813de64, exactly 0012-0014 added
#   D  future dedicated release   — release/sigap-regional-v1, baseline 4813de64, exactly 0012-0014 added
#   E  explicit stacked child     — PR #98 child of C: parent validated against the
#                                  canonical baseline, child must change zero migrations.
#                                  Active ONLY on a real pull_request event whose
#                                  base/head/repository match the declared contract.
#
# A feature branch is NEVER a release branch: forcing context D on the regional
# development branch, or overriding the release branch name to it, is a violation.
# No branch, tag, file or ref is modified by this script.
#
# Usage:
#   sh scripts/ops/check-migration-inventory.sh [--branch NAME] [--baseline SHA] [--context A|B|C|D|E]
# Exit: 0 pass | 1 inventory/context violation | 2 fail-closed (missing prerequisite / bad config)

set -u
fail() { printf 'MIGRATION-GUARD FAIL: %s\n' "$1"; exit 1; }
stop() { printf 'MIGRATION-GUARD STOP (fail-closed): %s\n' "$1"; exit 2; }
ok()   { printf 'ok: %s\n' "$1"; }

HIST_ANCHOR=d7705f09
HIST_RELEASE_BRANCH=design/ui-ux-overhaul
REGIONAL_BASE=4813de64fcbac89ae9ce419f003a9120177c6867
REGIONAL_DEV_BRANCH=feat/sigap-selfhosted-auth
REGIONAL_RELEASE_BRANCH=release/sigap-regional-v1
# Context E is bound to EXACTLY this stacked child and its parent. These are
# constants, not environment-supplied declarations: an arbitrary base/head in
# the environment can never select the stacked exemption.
STACKED_CHILD_BRANCH=feat/sigap-identity-deactivation
STACKED_BASE_BRANCH=feat/sigap-selfhosted-auth
EXPECTED_REPOSITORY=lostsky1403/Sigap
MIG_DIR=packages/db/migrations
EXPECTED_REGIONAL="packages/db/migrations/0012_service_day_wib.sql packages/db/migrations/0013_organizations.sql packages/db/migrations/0014_patient_account.sql"
EXPECTED_SHA256="0012_service_day_wib.sql:618154620aa57c9b044b2a19423ff24358ec050bd1a8d7a220076e7646157663 0013_organizations.sql:6bf14b56079e4a252bc366e57977b1b3daed5399dbd68c34371479a660c4d5b1 0014_patient_account.sql:001e7784b420649732970f963df2f24eca388b0e5d64cc83fd505ed925e21624"

branch='' baseline='' context=''
while [ $# -gt 0 ]; do
  case "$1" in
    --branch)   branch=${2:-}; shift 2 ;;
    --baseline) baseline=${2:-}; shift 2 ;;
    --context)  context=${2:-}; shift 2 ;;
    *) stop "unknown argument: $1" ;;
  esac
done

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || stop 'not inside a git work tree'
[ -d "$MIG_DIR" ] || stop "$MIG_DIR is missing"
[ -f docs/operations/DEPLOYMENT_RUNBOOK.md ] || stop 'deployment runbook is missing'

if [ -z "$branch" ]; then
  branch=${GITHUB_HEAD_REF:-${GITHUB_REF_NAME:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '')}}
fi
[ -n "$branch" ] || stop 'cannot determine branch context'

# An override that renames the development branch into a release branch is forbidden.
override=${SIGAP_RELEASE_BRANCH:-}
if [ -n "$override" ] && { [ "$override" = "$REGIONAL_DEV_BRANCH" ] || [ "$override" = 'main' ]; }; then
  fail "unauthorized branch-name override: '$override' cannot be declared a release branch"
fi

if [ -z "$context" ]; then
  if [ "$branch" = "$HIST_RELEASE_BRANCH" ] || [ "$branch" = "$override" ] && [ -n "$override" ]; then context=A
  elif [ "$branch" = "$REGIONAL_DEV_BRANCH" ]; then context=C
  elif [ "$branch" = "$REGIONAL_RELEASE_BRANCH" ]; then context=D
  else context=B
  fi
fi

# Never allow the feature branch to be relabelled as the release branch.
if [ "$branch" = "$REGIONAL_DEV_BRANCH" ] && [ "$context" = 'D' ]; then
  fail "context D (release) cannot be applied to the development branch '$REGIONAL_DEV_BRANCH'"
fi
# --- Context E helpers: stacked child of the verified regional dev branch ---
# Verify the checksums of the declared 0012-0014 at an arbitrary git ref, not
# just in the working tree, so inherited content is validated at the parent.
check_regional_checksums_at() {
  ref=$1
  for pair in $EXPECTED_SHA256; do
    file=${pair%%:*}; want=${pair#*:}
    actual=$(git show "$ref:$MIG_DIR/$file" 2>/dev/null | sha256sum | awk '{print $1}')
    [ -n "$actual" ] || fail "E: declared migration missing at $ref: $file"
    [ "$actual" = "$want" ] || fail "E: checksum drift at $ref for $file: $actual"
  done
  ok "E: declared 0012-0014 checksums match at $ref"
}

validate_stacked_context() {
  # All identity inputs are constants checked against the GitHub event; an
  # arbitrary environment-supplied base/head can never select this exemption.
  [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ] || stop "E: requires pull_request event"
  [ "${GITHUB_BASE_REF:-}" = "$STACKED_BASE_BRANCH" ] || stop "E: base ref must be $STACKED_BASE_BRANCH"
  [ "${GITHUB_HEAD_REF:-}" = "$STACKED_CHILD_BRANCH" ] || stop "E: head ref must be $STACKED_CHILD_BRANCH"
  [ "${GITHUB_REPOSITORY:-}" = "$EXPECTED_REPOSITORY" ] || stop "E: repository must be $EXPECTED_REPOSITORY"
  # HEAD must be the PR merge commit: exactly two parents, parent = base tip,
  # second parent = head tip, and the base must be an ancestor of the head.
  np=$(git rev-list --parents -n 1 HEAD | awk '{print NF-1}')
  [ "$np" -eq 2 ] || stop "E: HEAD must be a PR merge commit with 2 parents (got $np)"
  parent=$(git rev-parse --verify --quiet "HEAD^1^{commit}") || stop "E: parent commit (HEAD^1) unavailable"
  child=$(git rev-parse --verify --quiet "HEAD^2^{commit}") || stop "E: child commit (HEAD^2) unavailable"
  git merge-base --is-ancestor "$parent" "$child" || fail "E: verified parent is not an ancestor of the child"
  # Cross-check the merge parents against the fetched branch refs when present.
  if git rev-parse --verify --quiet "origin/$STACKED_BASE_BRANCH^{commit}" >/dev/null 2>&1; then
    [ "$(git rev-parse "origin/$STACKED_BASE_BRANCH")" = "$parent" ] \
      || fail "E: merge parent does not match origin/$STACKED_BASE_BRANCH"
  fi
  if git rev-parse --verify --quiet "origin/$STACKED_CHILD_BRANCH^{commit}" >/dev/null 2>&1; then
    [ "$(git rev-parse "origin/$STACKED_CHILD_BRANCH")" = "$child" ] \
      || fail "E: merge second parent does not match origin/$STACKED_CHILD_BRANCH"
  fi
  ok "E: PR event/base/head/repository and merge ancestry verified"
}

validate_stacked_parent() {
  # The parent must carry EXACTLY the authorized regional migration set
  # relative to the canonical baseline, with unchanged applied history.
  p_ns=$(git diff --name-status "$base".."$parent" -- "$MIG_DIR")
  p_added=$(printf '%s\n' "$p_ns" | awk '$1=="A"{print $2}' | sort)
  p_destroyed=$(printf '%s\n' "$p_ns" | awk '$1=="D"||$1~/^R/{print $0}')
  p_modified=$(printf '%s\n' "$p_ns" | awk '$1=="M"{print $2}')
  [ -z "$p_destroyed" ] || fail "E: parent deletes/renames an applied migration: $(printf '%s' "$p_destroyed" | tr '\n' ' ')"
  [ -z "$p_modified" ] || fail "E: parent modifies an applied migration: $(printf '%s' "$p_modified" | tr '\n' ' ')"
  [ "$(printf '%s' "$p_added" | tr '\n' ' ' | sed 's/ $//')" = "$EXPECTED_REGIONAL" ] \
    || fail "E: parent inventory mismatch (got: $(printf '%s' "$p_added" | tr '\n' ' '))"
  reg=$base
  reg_ns=$(git diff --name-status "$reg".."$parent" -- "$MIG_DIR")
  reg_added=$(printf '%s\n' "$reg_ns" | awk '$1=="A"{print $2}' | sort)
  [ "$(printf '%s' "$reg_added" | tr '\n' ' ' | sed 's/ $//')" = "$EXPECTED_REGIONAL" ] \
    || fail "E: parent inventory vs canonical regional baseline mismatch"
  check_regional_checksums_at "$parent"
  ok "E: parent carries exactly declared 0012-0014 vs $base"
}

validate_stacked_child() {
  # The child must change ZERO migrations relative to the verified parent.
  c_ns=$(git diff --name-status "$parent".."$child" -- "$MIG_DIR")
  [ -z "$c_ns" ] || fail "E: child changes migrations vs parent: $(printf '%s' "$c_ns" | tr '\n' ' ')"
  # Also verify the actual tree content the run will use (defence in depth).
  c_work=$(git diff --name-status "$parent"..HEAD -- "$MIG_DIR")
  [ -z "$c_work" ] || fail "E: checkout tree changes migrations vs parent"
  check_regional_checksums_at "$child"
  check_forward_order
  ok "E: child changes zero migrations vs verified parent"
}

case "$context" in
  A|B|C|D|E) ;;
  *) stop "unknown context '$context'" ;;
esac

# Each context is bound to its branch; a mismatched context is a violation, never a pass.
case "$context" in
  A) [ "$branch" = "$HIST_RELEASE_BRANCH" ] || [ -n "$override" ] \
       || fail "A: historical release context requires '$HIST_RELEASE_BRANCH' (got '$branch')" ;;
  B) case "$branch" in
       "$HIST_RELEASE_BRANCH"|"$REGIONAL_DEV_BRANCH"|"$REGIONAL_RELEASE_BRANCH")
         fail "B: ordinary-branch context cannot be used on '$branch'" ;;
     esac ;;
  C) [ "$branch" = "$REGIONAL_DEV_BRANCH" ] \
       || fail "C: development context requires '$REGIONAL_DEV_BRANCH' (got '$branch')" ;;
  D) [ "$branch" = "$REGIONAL_RELEASE_BRANCH" ] \
       || fail "D: release context requires '$REGIONAL_RELEASE_BRANCH' (got '$branch')" ;;
   E) [ "$branch" = "$STACKED_CHILD_BRANCH" ] \
        || fail "E: stacked-child context requires '$STACKED_CHILD_BRANCH' (got '$branch')" ;;
esac

case "$context" in
  A) base=$HIST_ANCHOR ;;
  B) base=origin/main ;;
  C|D|E) base=${baseline:-$REGIONAL_BASE} ;;
esac

git rev-parse --verify --quiet "$base^{commit}" >/dev/null 2>&1 || stop "baseline '$base' is not available in this checkout — fetch full history"
git rev-parse --verify --quiet HEAD^{commit} >/dev/null 2>&1 || stop 'HEAD is not a commit'

name_status=$(git diff --name-status "$base"..HEAD -- "$MIG_DIR")
added=$(printf '%s\n' "$name_status" | awk '$1=="A"{print $2}' | sort)
destroyed=$(printf '%s\n' "$name_status" | awk '$1=="D"||$1~/^R/{print $0}')
modified=$(printf '%s\n' "$name_status" | awk '$1=="M"{print $2}')

check_forward_order() {
  versions=$(find "$MIG_DIR" -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]_*.sql' -printf '%f\n' 2>/dev/null \
    | sed 's/^\([0-9]*\).*/\1/' | sort -n)
  [ -n "$versions" ] || fail 'no ordered migrations found'
  expected=1
  for v in $versions; do
    [ "$v" -eq "$expected" ] || fail "incorrect migration order/version: expected $expected, found $v"
    expected=$((expected + 1))
  done
  ok "forward-only migration versions 1..$((expected - 1))"
}

check_regional_checksums() {
  for pair in $EXPECTED_SHA256; do
    file=${pair%%:*}; want=${pair#*:}
    actual=$(sha256sum "$MIG_DIR/$file" 2>/dev/null | awk '{print $1}')
    [ -n "$actual" ] || fail "declared migration missing or unreadable: $file"
    [ "$actual" = "$want" ] || fail "checksum mismatch for $file: $actual"
  done
  ok 'declared 0012-0014 SHA-256 checksums match'
}

check_never_applied_changed() {
  applied=$(git diff --name-only "$base"..HEAD -- "$MIG_DIR/0001_init.sql" "$MIG_DIR/0002_medical_records.sql" \
    "$MIG_DIR/0003_identity_rbac.sql" "$MIG_DIR/0004_audit_events.sql" "$MIG_DIR/0005_appointments.sql" \
    "$MIG_DIR/0006_notifications.sql" "$MIG_DIR/0007_checkin_constraints.sql" "$MIG_DIR/0008_identity_subject.sql" \
    "$MIG_DIR/0009_user_roles_lifecycle.sql" "$MIG_DIR/0010_demo_notes_column.sql" \
    "$MIG_DIR/0011_notification_outbox_phone_constraints.sql")
  [ -z "$applied" ] || fail "applied migrations 0001-0011 changed: $(printf '%s' "$applied" | tr '\n' ' ')"
  ok 'applied migrations 0001-0011 unchanged'
}

case "$context" in
  A)
    if [ -z "$(git rev-parse --verify --quiet "$HIST_ANCHOR^{commit}" 2>/dev/null)" ]; then stop 'historical anchor d7705f09 unavailable'; fi
    want="packages/db/migrations/0006_notifications.sql packages/db/migrations/0011_notification_outbox_phone_constraints.sql"
    got=$(printf '%s\n' "$name_status" | sort | awk '{print $2}' | tr '\n' ' ' | sed 's/ $//')
    [ "$got" = "$want" ] || fail "A: historical release inventory differs from declared set (got: $got)"
    grep -q "$HIST_ANCHOR" docs/operations/DEPLOYMENT_RUNBOOK.md \
      && grep -qi 'non-empty by design' docs/operations/DEPLOYMENT_RUNBOOK.md \
      || fail 'A: runbook does not classify the non-empty historical diff'
    check_forward_order
    ok "A: historical design-release contract preserved (anchor $HIST_ANCHOR)"
    ;;
  B)
    [ -z "$destroyed" ] || fail "B: branch deletes/renames an applied migration: $(printf '%s' "$destroyed" | tr '\n' ' ')"
    [ -z "$modified" ] || fail "B: branch modifies an applied migration: $(printf '%s' "$modified" | tr '\n' ' ')"
    [ -z "$added" ] || fail "B: undeclared migration added on an ordinary branch: $(printf '%s' "$added" | tr '\n' ' ')"
    check_forward_order
    ok 'B: ordinary branch ships no migration change'
    ;;
  C)
    [ -z "$destroyed" ] || fail "C: branch deletes/renames an applied migration: $(printf '%s' "$destroyed" | tr '\n' ' ')"
    [ -z "$modified" ] || fail "C: branch modifies an applied migration: $(printf '%s' "$modified" | tr '\n' ' ')"
    [ "$(printf '%s' "$added" | tr '\n' ' ' | sed 's/ $//')" = "$EXPECTED_REGIONAL" ] \
      || fail "C: declared 0012-0014 inventory mismatch (got: $(printf '%s' "$added" | tr '\n' ' '))"
    check_never_applied_changed
    check_forward_order
    check_regional_checksums
    case "$branch" in release/*|hotfix/*) fail "C: development branch relabelled as '$branch'" ;; esac
    ok "C: regional development branch permits ONLY declared 0012-0014 (baseline $base)"
    ;;
  D)
    [ "$branch" = "$REGIONAL_RELEASE_BRANCH" ] || fail "D: only '$REGIONAL_RELEASE_BRANCH' may use the release contract (got '$branch')"
    [ "$base" = "$REGIONAL_BASE" ] || fail "D: release baseline must be $REGIONAL_BASE (got $base)"
    [ -z "$destroyed" ] || fail "D: branch deletes/renames an applied migration"
    [ -z "$modified" ] || fail "D: branch modifies an applied migration"
    [ "$(printf '%s' "$added" | tr '\n' ' ' | sed 's/ $//')" = "$EXPECTED_REGIONAL" ] \
      || fail "D: declared 0012-0014 inventory mismatch"
    check_never_applied_changed
    check_forward_order
    check_regional_checksums
    grep -q "$REGIONAL_BASE" docs/operations/DEPLOYMENT_RUNBOOK.md \
      || fail 'D: regional baseline is not declared in the deployment runbook'
    ok "D: dedicated regional release contract satisfied (baseline $base)"
    ;;
  E)
    validate_stacked_context
    validate_stacked_parent
    validate_stacked_child
    ok "E: stacked child adds zero migrations; parent 0012-0014 inventory and checksums enforced"
    ;;
esac

printf 'PASS: migration inventory context %s on branch %s (baseline %s)\n' "$context" "$branch" "$base"
exit 0
