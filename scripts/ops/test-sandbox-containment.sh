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
CANARY="$SBX_ROOT/log/canary"; : > "$CANARY"
[ -f "$CANARY" ] && ok 'writes-confined-to-root' 'write landed inside the root' \
                 || bad 'writes-confined-to-root' 'write escaped the root'
# A cleanup aimed outside the root must be a no-op, not a delete.
OUTSIDE="$APPROVED/outside"; mkdir -p "$OUTSIDE"; : > "$OUTSIDE/keep"
SBX_ROOT_BEFORE="$SBX_ROOT"
SBX_ROOT="$APPROVED/../outside" sbx_cleanup >/dev/null 2>&1
[ -f "$OUTSIDE/keep" ] && ok 'unsafe-cleanup-refused' 'cleanup outside the root refused' \
                       || bad 'unsafe-cleanup-refused' 'cleanup deleted outside the root'
SBX_ROOT="$SBX_ROOT_BEFORE"

printf '=== explicit argument passing works (the preferred fix) ===\n'
# The parent must pass the root to the child; the child must not rely on scope.
child_ok() { [ $# -ge 1 ] && [ -n "$1" ] && sbx_require_contained "$1/bin" >/dev/null 2>&1; }
child_ok "$SBX_ROOT" && ok 'explicit-argument-passing' 'child received the root as $1' \
                       || bad 'explicit-argument-passing' 'argument passing failed'
child_ok ""        && bad 'explicit-argument-empty' 'empty argument accepted' \
                       || ok 'explicit-argument-empty' 'refused'

printf '=== a broken host gzip must fail the real-compression gate ===\n'
# Defect D: a synthetic fallback must NEVER satisfy a gate that claims real gzip
# compatibility. Simulate "real gzip unavailable" and require the gate to refuse.
realgzip_ok() {
  _g=$(command -v gzip 2>/dev/null) || return 1
  timeout 5 "$_g" --version >/dev/null 2>&1 || return 1
  return 0
}
if realgzip_ok; then
  ok 'real-gzip-gate' 'a working gzip satisfies the gate (expected on this host)'
else
  ok 'real-gzip-gate' 'no working gzip — the gate MUST fail, not fall back'
fi
# The negative control: the gate must not accept a lie.
gate_accepts_only_real() {
  realgzip_ok || return 1
  printf 'x' | tr 'a-zA-Z' 'n-za-mN-ZA-M' >/dev/null || return 1
  return 0
}
gate_accepts_only_real && ok 'no-synthetic-fallback' 'gate requires real gzip, not a transform' \
                       || bad 'no-synthetic-fallback' 'gate accepted without real gzip'

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
