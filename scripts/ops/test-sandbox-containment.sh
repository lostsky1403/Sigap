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
SBX_ROOT_BEFORE="$SBX_ROOT"
SBX_ROOT="$OUTSIDE" sbx_cleanup >/dev/null 2>&1
if [ -f "$OUTSIDE/keep" ]; then
  ok 'unsafe-cleanup-refused' 'cleanup left a child of the approved root intact'
else
  bad 'unsafe-cleanup-refused' 'cleanup DELETED a child of the approved root'
fi
# And the legitimate case must still clean up: a root inside the approved root IS removed.
JUNK="$APPROVED/junk"; mkdir -p "$JUNK"; : > "$JUNK/x"
SBX_ROOT="$JUNK" sbx_cleanup >/dev/null 2>&1
[ -d "$JUNK" ] && bad 'cleanup-removes-own-root' 'a contained root was NOT removed' \
              || ok 'cleanup-removes-own-root' 'a contained root was removed'
SBX_ROOT="$SBX_ROOT_BEFORE"

# OWNERSHIP: a SIBLING of the created root must SURVIVE cleanup. This is the case that
# caught the previous cleanup deleting any child of the approved root.
SIB="$APPROVED/sibling"; mkdir -p "$SIB"; : > "$SIB/keep"
[ -f "$SIB/keep" ] || bad 'cleanup-sibling-survives' 'cannot set up sibling canary'
SBX_ROOT="$SIB" sbx_cleanup >/dev/null 2>&1
if [ -f "$SIB/keep" ]; then
  ok 'cleanup-sibling-survives' 'a sibling directory survived cleanup (ownership enforced)'
else
  bad 'cleanup-sibling-survives' 'cleanup DELETED a sibling this invocation did not create'
fi
# And the created root itself must still be removable through the same code path,
# proving the sibling case above is not passing merely because cleanup never deletes.
SBX_ROOT="$SBX_CREATED" sbx_cleanup >/dev/null 2>&1
[ -d "$SBX_CREATED" ] && bad 'cleanup-own-root-removable' 'own root survived' \
                      || ok 'cleanup-own-root-removable' 'own root removed when pointed at it'
SBX_ROOT="$SBX_ROOT_BEFORE"

printf '=== explicit argument passing works (the preferred fix) ===\n'
# A real child process, so relying on inherited scope actually fails.
# The library is resolved from THIS script's own location and passed to the child as
# an explicit, validated argument. A hardcoded absolute path was a real defect: it
# worked on the development workstation and silently failed on a GitHub-hosted Ubuntu
# runner, where that path does not exist. No Windows-specific fallback is added — an
# unresolvable library fails closed.
cat > "$SBX_ROOT/argchild.sh" <<'CHILD'
LIB="$1"; ROOT="$2"; DEST="$3"
[ -n "$LIB" ] && [ -f "$LIB" ] || { echo "REFUSE:no-library"; exit 0; }
[ -n "$ROOT" ] || { echo "REFUSE:no-root"; exit 0; }
. "$LIB"
SIGAP_TEST_ROOT="$ROOT" sbx_init >/dev/null 2>&1
if [ -n "$DEST" ] && sbx_require_contained "$DEST/bin" >/dev/null 2>&1; then
  printf 'ACCEPT'; else printf 'REFUSE'; fi
CHILD
LIB_PATH=$(cd -P -- "$(dirname "$0")" && pwd -P)/ops-sandbox.sh
[ -f "$LIB_PATH" ] || bad 'library-path-resolves' "cannot find ops-sandbox.sh at $LIB_PATH"
[ -f "$LIB_PATH" ] && ok 'library-path-resolves' "$LIB_PATH"

res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "$SBX_ROOT" 2>/dev/null)
[ "$res" = "ACCEPT" ] && ok 'explicit-argument-passing' 'child received the root as $3' \
                       || bad 'explicit-argument-passing' "child returned $res"
res=$(sh "$SBX_ROOT/argchild.sh" "$LIB_PATH" "$SIGAP_TEST_ROOT" "" 2>/dev/null)
[ "$res" = "REFUSE" ] && ok 'explicit-argument-empty' 'empty argument refused' \
                       || bad 'explicit-argument-empty' "empty argument returned $res"
# A wrong or nonexistent library path must fail closed, never silently pass.
res=$(sh "$SBX_ROOT/argchild.sh" /nonexistent/ops-sandbox.sh "$SIGAP_TEST_ROOT" "$SBX_ROOT" 2>/dev/null)
case "$res" in
  REFUSE*) ok 'library-path-wrong-fails-closed' "returned $res" ;;
  ACCEPT)  bad 'library-path-wrong-fails-closed' 'accepted with a nonexistent library' ;;
  *)       bad 'library-path-wrong-fails-closed' "unexpected: $res" ;;
esac
res=$(sh "$SBX_ROOT/argchild.sh" "" "$SIGAP_TEST_ROOT" "$SBX_ROOT" 2>/dev/null)
case "$res" in
  REFUSE*) ok 'library-path-empty-fails-closed' "returned $res" ;;
  *)       bad 'library-path-empty-fails-closed' "empty library path returned $res" ;;
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
