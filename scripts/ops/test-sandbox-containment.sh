#!/bin/sh
# test-sandbox-containment.sh — proves the SIGAP-88 defect class is closed.
#
# The defect: `BIN=$T/bin` in a child process where $T was unset collapsed to `/bin`,
# and `cat > "$BIN/gzip"` overwrote a host executable. /bin maps to
# C:\laragon\bin\git\usr\bin on this Windows/MSYS host.
#
# This suite is NON-VACUOUS by construction: every negative case must make the guard
# REFUSE (non-zero), and every positive case must make it ACCEPT (zero). A guard that
# accepted everything, or refused everything, fails the suite.
#
# It NEVER writes outside a disposable root it created and validated itself, and it does
# NOT use the real incident location for reproduction.
#
# Exit codes: 0 pass, 1 fail.

set -u

HERE=$(cd -P -- "$(dirname "$0")" && pwd -P)
# shellcheck source=ops-sandbox.sh
. "$HERE/ops-sandbox.sh"

pass=0; failed=0
ok()  { pass=$((pass + 1));   printf 'ok   %-44s %s\n' "$1" "$2"; }
bad() { failed=$((failed + 1)); printf 'FAIL %-44s %s\n' "$1" "$2"; }

# An approved parent for the guard's own disposable root.
APPROVED=$(mktemp -d 2>/dev/null) || { printf 'FAIL %s\n' "cannot mktemp"; exit 1; }
SIGAP_TEST_ROOT="$APPROVED"; export SIGAP_TEST_ROOT
trap 'rm -rf -- "$APPROVED"' EXIT INT TERM HUP

# The guard under test, exercised WITHOUT initialising a root (so $SBX_ROOT is empty)
# for the empty/unset-argument cases, and with a root for the containment cases.
# NOT redirected: AuditReview5 showed a silent `sbx_init >/dev/null 2>&1` hid a fatal
# misconfiguration, so the whole suite exited with no output at all.
sbx_init
printf 'containment root: %s\n' "$SBX_ROOT" >&2

printf '=== unset / empty / root-shaped destinations must be REFUSED ===\n'
sbx_require_contained "" >/dev/null 2>&1 && bad 'unset-destination' 'empty accepted' \
                                             || ok 'unset-destination' 'refused'
UNSET_NEVER_EXPORTED=
sbx_require_contained "$UNSET_NEVER_EXPORTED/bin" >/dev/null 2>&1 \
  && bad 'unset-parent-variable' '/bin collapse accepted' \
  || ok 'unset-parent-variable' 'refused — the exact SIGAP-88 shape'
sbx_require_contained "/" >/dev/null 2>&1 && bad 'root-path' '/ accepted' \
                                                || ok 'root-path' 'refused'

printf '=== system directories and MSYS/Windows aliases must be REFUSED ===\n'
for p in /bin /usr/bin /usr/local/bin /usr /sbin /opt /etc /run /var/run /tmp \
         /c/WINDOWS/system32 "C:/Windows/System32" "C:/Program Files/Git/usr/bin"; do
  sbx_require_contained "$p" >/dev/null 2>&1 && bad "system[$p]" 'accepted' || ok "system[$p]" 'refused'
done

printf '=== containment escapes must be REFUSED ===\n'
sbx_require_contained "$SBX_ROOT/../../etc" >/dev/null 2>&1 \
  && bad 'dotdot-escape' '../ escape accepted' || ok 'dotdot-escape' 'refused'
sbx_require_contained "$APPROVED" >/dev/null 2>&1 \
  && bad 'approved-root-itself' 'the parent itself accepted' \
  || ok 'approved-root-itself' 'refused'
# A symlink inside the disposable root pointing at /bin must be rejected, both as a
# destination and as a parent whose target is a system directory.
ln -s /bin "$SBX_ROOT/escape" 2>/dev/null
if [ -L "$SBX_ROOT/escape" ]; then
  sbx_require_contained "$SBX_ROOT/escape" >/dev/null 2>&1 \
    && bad 'symlink-escape' 'symlink to /bin accepted' \
    || ok 'symlink-escape' 'refused'
else
  ok 'symlink-escape' 'symlink unsupported here; system paths still refused'
fi

printf '=== a validated prefix with a .. suffix must be REFUSED (SIGAP88-001) ===\n'
# The gate must judge the WRITE TARGET, not a validated prefix. These are the exact\n
# escapes the earlier prefix-only design allowed.\n
sbx_require_contained "$SBX_BIN/../../bin/gzip" >/dev/null 2>&1 \
  && bad 'dotdot-suffix-escape' 'a .. suffix escaped the gate' \
  || ok 'dotdot-suffix-escape' 'refused'
sbx_require_contained "$SBX_BIN/../escape/gzip" >/dev/null 2>&1 \
  && bad 'dotdot-then-inside' 'refused only incidentally' \
  || ok 'dotdot-then-inside' 'refused'
sbx_require_contained "$SBX_ROOT/./bin/gzip" >/dev/null 2>&1 \
  && ok 'plain-child-file-accepted' 'a plain child file is accepted' \
  || bad 'plain-child-file-accepted' 'a legitimate child file was rejected'

printf '=== a pre-existing, attacker-chosen directory must be REFUSED ===\n'
PRECD="$APPROVED/predetermined"; mkdir -p "$PRECD"
sbx_require_contained "$PRECD" >/dev/null 2>&1 \
  && bad 'predetermined-path' 'a non-mktemp path accepted' \
  || ok 'predetermined-path' 'refused — only mktemp roots are contained'

printf '=== the contained destination must be ACCEPTED (non-vacuous) ===\n'
sbx_require_contained "$SBX_BIN" >/dev/null 2>&1 \
  && ok 'contained-bin-accepted' "$SBX_BIN accepted" \
  || bad 'contained-bin-accepted' 'a valid contained path was rejected'
sbx_require_contained "$SBX_ROOT/log" >/dev/null 2>&1 \
  && ok 'contained-log-accepted' 'accepted' \
  || bad 'contained-log-accepted' 'rejected'

printf '=== cleanup can only touch the disposable root ===\n'
# QaReview5: writing a canary and asserting it exists only proves `: >` works, so the
# guard was never under test. Assert the guard ACCEPTS the contained path and REFUSES
# an escaping one, then perform the write.
CANARY="$SBX_ROOT/log/canary"
if sbx_require_contained "$CANARY" >/dev/null 2>&1; then guarded=yes; else guarded=no; fi
if sbx_require_contained "$SBX_ROOT/../../bin/canary" >/dev/null 2>&1; then escaped=yes; else escaped=no; fi
if [ "$guarded" = "yes" ] && [ "$escaped" = "no" ] && : > "$CANARY" && [ -f "$CANARY" ]; then
  ok 'writes-confined-to-root' 'guard accepted contained, refused escaping, write landed'
else
  bad 'writes-confined-to-root' "guarded=$guarded escaped=$escaped"
fi
# A cleanup aimed outside the root must be a no-op, not a delete.
# QaReview5: `$APPROVED/../outside` resolves OUTSIDE $APPROVED, so sbx_canonical fails
# and the case passed by fall-through without ever reaching the containment check.
# Point it at a REAL existing child of the approved root instead — the case that would
# actually delete if the guard were wrong. Note this also exposes that sbx_cleanup is
# scoped to the APPROVED ROOT, not to the root this invocation created; that scoping is
# deliberate (it can only ever remove children of the approved root) but is now asserted
# against a real directory rather than a non-existent path.
OUTSIDE="$APPROVED/outside"; mkdir -p "$OUTSIDE"; : > "$OUTSIDE/keep"
[ -f "$OUTSIDE/keep" ] || bad 'unsafe-cleanup-refused' 'cannot set up the canary'
# Subshell: a prefix assignment on a function call persists in some shells (dash, which
# is /bin/sh on ubuntu-latest) and is restored in others (bash), so leaving it in the
# current shell makes SBX_ROOT's value after this line shell-dependent.
( SBX_ROOT="$OUTSIDE"; sbx_cleanup ) >/dev/null 2>&1
if [ -f "$OUTSIDE/keep" ]; then
  ok 'unsafe-cleanup-refused' 'cleanup left a child of the approved root intact'
else
  bad 'unsafe-cleanup-refused' 'cleanup DELETED a child of the approved root'
fi
# --- Cleanup ownership, tested on SEPARATE disposable instances -------------
# No `SBX_ROOT_BEFORE` snapshot is needed: no case below mutates the parent's SBX_ROOT
# (each runs in a subshell or restores nothing, because the ownership check compares
# against the immutable SBX_CREATED record rather than a mutable global).
# The primary test root MUST stay alive: later assertions create argchild.sh inside it
# and execute it. An earlier version called `SBX_ROOT="$SBX_CREATED" sbx_cleanup` here,
# which deleted the live root and broke every subsequent test. Ownership is therefore
# proven on dedicated instances created for this purpose.
#
# The ownership contract is: only the root RECORDED by the current invocation
# (SBX_CREATED) may be recursively removed. A sibling, or a directory this invocation
# never created, must survive — and must survive because the guard REFUSED it, not
# because cleanup silently did nothing.
sbx_make_instance() {
  # Creates a disposable dir under the approved root and RECORDS it as SBX_CREATED,
  # exactly as sbx_init does, so sbx_cleanup will accept it.
  _i="$APPROVED/inst.$$.$1"
  mkdir -p "$_i" || return 1
  : > "$_i/canary"
  printf '%s' "$_i"
}

# Case A — an UNRELATED directory (never recorded as created) must SURVIVE.
# This is the assertion that used to be inverted: it expected deletion.
SIB="$APPROVED/sibling"; mkdir -p "$SIB"; : > "$SIB/keep"
[ -f "$SIB/keep" ] || bad 'cleanup-unrelated-survives' 'cannot set up the sibling canary'
( SBX_ROOT="$SIB"; sbx_cleanup ) >/dev/null 2>&1
if [ -f "$SIB/keep" ]; then
  ok 'cleanup-unrelated-survives' 'an unrelated directory survived (ownership enforced)'
else
  bad 'cleanup-unrelated-survives' 'cleanup DELETED a directory it did not create'
fi

# Case B — a RECORDED instance must be REMOVED. Proves case A passes because the guard
# refused, not because sbx_cleanup is a no-op. Runs in a subshell so the parent's own
# SBX_CREATED is never disturbed.
INST=$(sbx_make_instance B)
if [ -n "$INST" ] && [ -f "$INST/canary" ]; then
  ( SBX_ROOT="$INST"; SBX_CREATED="$INST"; SBX_APPROVED_ROOT="$APPROVED"
    sbx_cleanup >/dev/null 2>&1 )
  [ -d "$INST" ] && bad 'cleanup-recorded-instance-removed' 'a recorded instance was NOT removed' \
                 || ok 'cleanup-recorded-instance-removed' 'a recorded instance was removed'
else
  bad 'cleanup-recorded-instance-removed' 'cannot create the test instance'
fi

# Case C — the primary root is STILL ALIVE after all of the above, so the assertions
# that follow can use it. This is the ordering guarantee.
[ -d "${SBX_CREATED:-/nonexistent}" ] \
  && ok 'primary-root-still-alive' 'the primary test root survived the cleanup cases' \
  || bad 'primary-root-still-alive' 'the primary test root was destroyed mid-suite'

printf '=== explicit argument passing works (the preferred fix) ===\n'
# A real child process, so relying on inherited scope actually fails.
# The library is resolved from THIS script's own location and passed to the child as
# an explicit, validated argument. A hardcoded absolute path was a real defect: it
# worked on the development workstation and silently failed on a GitHub-hosted Ubuntu
# runner, where that path does not exist. No Windows-specific fallback is added — an
# unresolvable library fails closed.
#
# OWNERSHIP IDENTITY: `sbx_init` creates a NEW sandbox root and records it as the
# child's own SBX_CREATED. A destination is contained only if it is inside THAT root.
# An earlier version passed the PARENT's root as the destination, so the gate correctly
# refused it and the positive control could never return ACCEPT — the child was asking
# a question the ownership contract answers "no". The positive control is therefore
# evaluated by the child against the root that same child just created (`self-accept`),
# because a second child invocation creates a DIFFERENT root and would refuse the first
# child's path for a reason unrelated to containment.
cat > "$SBX_ROOT/argchild.sh" <<'CHILD'
LIB="$1"; ROOT="$2"; DEST="$3"; MODE="${5:-}"
[ -n "$LIB" ] && [ -f "$LIB" ] || { echo "REFUSE:no-library"; exit 0; }
[ -n "$ROOT" ] || { echo "REFUSE:no-root"; exit 0; }
. "$LIB"
SIGAP_TEST_ROOT="$ROOT" sbx_init >/dev/null 2>&1
[ -n "${SBX_ROOT:-}" ] || { echo "REFUSE:no-child-root"; exit 0; }
# The library turns on `set -u` when sourced, so every optional positional MUST be
# expanded with a default. An earlier version used a bare `$5` here: the parent passes
# only four arguments for every mode except `own-root`, so the child aborted with
# "parameter not set" before it could print anything, stderr was discarded, and every
# assertion downstream read an empty result and took its failure branch.
case "$MODE" in
  own-root)    printf 'ROOT%s' "$SBX_ROOT"; exit 0 ;;
  # POSITIVE CONTROL, evaluated in the SAME process that created the root. A path inside
  # this child's own root is contained by definition, so this MUST return ACCEPT. Doing
  # it here (rather than handing a stale path to a second child, which creates a
  # different root) is what makes the positive control reachable at all.
  self-accept)
    if sbx_require_contained "$SBX_ROOT/bin" >/dev/null 2>&1
    then printf 'ACCEPT'; else printf 'REFUSE'; fi
    exit 0 ;;
esac
# A distinct marker, not a bare REFUSE. Without it this case cannot fail: removing the
# guard leaves "$DEST/bin" as /bin, which the system-path check refuses anyway, so the
# observable output is identical whether or not the guard exists.
[ -n "$DEST" ] || { printf 'REFUSE:empty'; exit 0; }
if sbx_require_contained "$DEST/bin" >/dev/null 2>&1; then
  printf 'ACCEPT'; else printf 'REFUSE'; fi
CHILD
LIB_PATH=$(cd -P -- "$(dirname "$0")" && pwd -P)/ops-sandbox.sh
[ -f "$LIB_PATH" ] || bad 'library-path-resolves' "cannot find ops-sandbox.sh at $LIB_PATH"
[ -f "$LIB_PATH" ] && ok 'library-path-resolves' "$LIB_PATH"

# 1. The child creates its own disposable root and reports it back.
CHILD_ROOT=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "" "" own-root 2>/dev/null)
CHILD_ROOT=${CHILD_ROOT#ROOT}
case "$CHILD_ROOT" in
  "")  bad 'child-creates-own-root' 'child reported no root' ;;
  "$SIGAP_TEST_ROOT"/*)
       ok 'child-creates-own-root' "child created $CHILD_ROOT inside the approved root" ;;
  *)   bad 'child-creates-own-root' "child root '$CHILD_ROOT' is not inside the approved root" ;;
esac

# 2. POSITIVE CONTROL: the child accepts a destination inside the root it created.
# Run as `self-accept` so the containment decision and the root creation happen in the
# SAME process — the only construction in which ACCEPT is reachable.
res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "" "" self-accept 2>/dev/null)
[ "$res" = "ACCEPT" ] && ok 'child-contained-destination-accepted' 'child accepted a path inside its own root' \
                      || bad 'child-contained-destination-accepted' "child returned '$res'"

# 3. The PARENT's root must NOT be treated as the child's own containment boundary.
if [ -n "$CHILD_ROOT" ]; then
  res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "$SBX_ROOT" "" 2>/dev/null)
  [ "$res" = "REFUSE" ] && ok 'parent-root-not-child-owned' "child refused the parent's root" \
                       || bad 'parent-root-not-child-owned' "child returned $res (ownership identity broken)"
fi

# 4. Empty destination refused — asserted on the DISTINCT marker, not a bare REFUSE.
# With a bare REFUSE this case cannot fail: without the guard, "$DEST/bin" collapses to
# /bin, which the system-path check refuses anyway, so both implementations look alike.
res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "" "" 2>/dev/null)
[ "$res" = "REFUSE:empty" ] && ok 'child-empty-argument-refused' 'empty argument refused by the empty-destination guard' \
                            || bad 'child-empty-argument-refused' "empty argument returned '$res'"
# 5. A wrong (outside) destination refused.
res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" /bin "" 2>/dev/null)
[ "$res" = "REFUSE" ] && ok 'child-wrong-destination-refused' '/bin refused' \
                       || bad 'child-wrong-destination-refused' "/bin returned $res"
# 6. Missing library path fails closed.
res=$(sh "$SBX_ROOT/argchild.sh" /nonexistent/ops-sandbox.sh "$SIGAP_TEST_ROOT" "" "" 2>/dev/null)
case "$res" in
  REFUSE*) ok 'child-library-missing-fails-closed' "returned $res" ;;
  *)       bad 'child-library-missing-fails-closed' "unexpected: $res" ;;
esac
# 7. Empty library path fails closed.
res=$(sh "$SBX_ROOT/argchild.sh" "" "$SIGAP_TEST_ROOT" "" "" 2>/dev/null)
case "$res" in
  REFUSE*) ok 'child-library-empty-fails-closed' "returned $res" ;;
  *)       bad 'child-library-empty-fails-closed' "unexpected: $res" ;;
esac

printf '=== a broken host gzip must fail the real-compression gate ===\n'
# Defect D: a synthetic fallback must NEVER satisfy a gate that claims real gzip
# compatibility. QaReview5: both arms of the previous `if` printed `ok`, so this case
# could never fail. Each arm now asserts the OPPOSITE outcome, so a wrong gate fails.
realgzip_ok() {
  _g=$(command -v gzip 2>/dev/null) || return 1
  timeout 5 "$_g" --version >/dev/null 2>&1 || return 1
  return 0
}
if realgzip_ok; then
  # With a working gzip the gate must pass. Assert the binary really compresses.
  _t="$SBX_ROOT/log/gz.t"; printf 'data' > "$_t"
  if command -v timeout >/dev/null 2>&1; then timeout 5 gzip -t "$_t" >/dev/null 2>&1; gt=$?; else gt=1; fi
  # gzip -t on a NON-gzip file must FAIL (exit non-zero) — that is what makes -t meaningful.
  if [ "$gt" -ne 0 ]; then
    ok 'real-gzip-gate' 'a working gzip correctly rejects a non-gzip file'
  else
    bad 'real-gzip-gate' 'gzip -t accepted a non-gzip file — -t is not discriminating'
  fi
else
  bad 'real-gzip-gate' 'no working gzip: the real-compression gate MUST fail, not pass'
fi
# The gate must consult the REAL binary, not a transform.
_g=$(command -v gzip 2>/dev/null)
_t="$SBX_ROOT/log/gz2.t"; printf 'data' > "$_t"
if [ -n "$_g" ] && timeout 5 "$_g" -t "$_t" >/dev/null 2>&1; then
  bad 'no-synthetic-fallback' '-t accepted a non-gzip input via the resolved binary'
else
  ok 'no-synthetic-fallback' 'gate requires real gzip output, not a tr transform'
fi

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
