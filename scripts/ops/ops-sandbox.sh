#!/bin/sh
# ops-sandbox.sh — fail-closed containment for disposable test roots and test doubles.
#
# This library is the direct remediation for incident SIGAP-88. In that incident a
# reviewer-generated helper ran
#
#     BIN=$T/bin            # $T empty in the child process
#     cat > "$BIN/gzip" ... # => cat > "/bin/gzip"  — A HOST EXECUTABLE
#
# $T was assigned in the parent but never exported, so it was empty in the child script,
# `BIN` collapsed to `/bin`, and `/bin` maps to C:\laragon\bin\git\usr\bin on this
# Windows/MSYS host — a real, writable, system-adjacent directory. The write therefore
# SUCCEEDED SILENTLY and corrupted the host gzip.
#
# The control below is POSITIVE CONTAINMENT, not a denylist. A denylist is necessary
# but insufficient: it can only reject shapes someone thought of. This library instead
# requires the destination to be PROVEN to be a child of a disposable root created by
# this very invocation, after symlink canonicalisation.
#
# Intended use, at the top of any harness that creates test doubles:
#
#     . "$(dirname "$0")/ops-sandbox.sh"
#     sbx_init                     # creates + validates the disposable root
#     sbx_require_contained "$BIN" # before ANY cat > / chmod / rm -rf
#
# Exit codes: 10 containment failure, 11 tool/environment failure.

set -u

# The ONLY acceptable parent for a disposable test root. A harness must set this to an
# explicitly approved location; the library refuses to invent a default, because a
# default that looks plausible is exactly the failure mode being remedied.
SBX_APPROVED_ROOT="${SIGAP_TEST_ROOT:-}"

# Refusals are RETURNED, not exited. AuditReview5 (P0): an earlier version ended with
# `exit`, so a refused destination terminated the CALLING shell — which made every
# negative assertion untestable (`sbx_require_contained x && bad || ok` could never
# observe a refusal, because the test process died first). `return` keeps the refusal
# observable; genuine setup failures use `sbx_fatal`, which does exit.
sbx_die() { printf 'REFUSED: %s\n' "$*" >&2; return "${2:-10}"; }
sbx_fatal() { printf 'FATAL: %s\n' "$*" >&2; exit "${2:-11}"; }

# ---------------------------------------------------------------------------
# 1. Create the disposable root. `mktemp -d` must succeed; a fallback string is NOT
#    acceptable, because `/tmp/sigap-rst.$$` is predictable and predictable paths are
#    hijackable. Abort on any failure rather than proceeding with an unverified path.
# ---------------------------------------------------------------------------
sbx_new_root() {
  # The disposable root must be created INSIDE the approved root, not in $TMPDIR.
  # QaReview5: a bare `mktemp -d` lands the root in $TMPDIR, which makes it a SIBLING
  # of the approved root rather than a child — so `sbx_is_child_of root approved`
  # is never true and sbx_init fails unconditionally for every caller. Both suites were
  # therefore dead before their first assertion.
  _parent=$SBX_APPROVED_ROOT
  [ -d "$_parent" ] || sbx_fatal "the approved root disappeared: $_parent" 11
  _root=$(mktemp -d "$_parent/sbx.XXXXXXXX" 2>/dev/null) \
    || sbx_fatal "mktemp -d inside $_parent failed" 11
  [ -d "$_root" ] || sbx_fatal "mktemp returned a non-directory" 11
  [ -L "$_root" ] && sbx_fatal "the new root is a symlink" 10
  printf '%s' "$_root"
}

# ---------------------------------------------------------------------------
# 2. Canonicalise. Resolve the PHYSICAL directory, so a symlink cannot point the
#    containment check at a directory other than the one it will write to.
# ---------------------------------------------------------------------------
sbx_canonical() {
  _p=$1
  [ -n "$_p" ] || return 1
  _c=$(cd -P -- "$_p" 2>/dev/null && pwd -P) || return 1
  printf '%s' "$_c"
}

# ---------------------------------------------------------------------------
# 3. Positive containment: $1 must canonicalise to a child of $2.
# ---------------------------------------------------------------------------
sbx_is_child_of() {
  _path=$1; _parent=$2
  [ -n "$_path" ]   || return 1
  [ -n "$_parent" ] || return 1
  # Deny the parent itself, and deny the root of the filesystem outright.
  [ "$_path" = "$_parent" ] && return 1
  case "$_path" in
    /|//|"") return 1 ;;
    /*/*) : ;;                     # an absolute path with at least one component
    *) return 1 ;;                # relative or malformed
  esac
  case "$_path" in
    "$_parent"/*) return 0 ;;
    *) return 1 ;;
  esac
}

# System directories and their MSYS/Windows aliases. Defense in depth ONLY — every one
# of these is also rejected by positive containment, because none is inside the
# disposable root. Kept explicit so a future reader sees the intent.
sbx_is_system_path() {
  case "$1" in
    /|/bin|/usr|/usr/bin|/usr/local|/usr/local/bin|/sbin|/lib|/lib32|/lib64|/opt|/etc|/run|/var|/var/run|/tmp|/dev|/proc|/sys|/root|/home)
      return 0 ;;
    /bin/*|/usr/bin/*|/usr/local/bin/*|/sbin/*|/opt/*|/etc/*|/run/*|/proc/*|/sys/*|/dev/*)
      return 0 ;;
    /c/WINDOWS/*|/c/Windows/*|/c/ProgramFiles/*|/c/Program\ Files/*|C:/WINDOWS/*|C:/Windows/*|C:/ProgramFiles/*|C:/Program\ Files/*)
      return 0 ;;
    # The toolchain damaged in SIGAP-88, in both MSYS and native Windows spellings.
    /c/laragon/*|C:/laragon/*|C:\\laragon\\*|/c/laragon|C:/laragon)
      return 0 ;;
    # UNC paths and drive-relative forms that can resolve onto host volumes.
    //*|\\\\*) return 0 ;;
    # Drive-relative (C:foo.exe) has no leading slash and so cannot be contained.
    [A-Za-z]:[!/\\]*) return 0 ;;
    *) return 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# 4. The gate every write must pass.
#
# SIGAP88-001: an earlier version contained only its ARGUMENT, so a write such as
# `cat > "$BIN/../../bin/gzip"` escaped — the guard validated $BIN and never saw the
# `..`. The gate therefore now takes the EXACT write target and canonicalises its
# PARENT, so any `..` or symlink in the write path is resolved before the decision.
# A missing final component is fine (the file is about to be created); a missing
# PARENT is not.
# ---------------------------------------------------------------------------
sbx_require_contained() {
  # The guard MUST fail closed. An earlier version called `sbx_die` (which does
  # `return 10`) as the last command of an `if`/`&&` list and then ran another
  # statement, which resets $? — so the function returned 0 for every input,
  # including `/`. Every negative test then reported `bad`, and, worse, every
  # caller that trusted the guard's exit status would have proceeded with the
  # write. `sbx_fatal` exits the process instead of returning, so the process
  # itself dies and the status can never be skipped.
  _dest=$1
  [ -n "${SBX_ROOT:-}" ] || sbx_fatal "sbx_init was never called" 10
  [ -n "$_dest" ] || sbx_fatal "destination is empty — the unset-variable shape from SIGAP-88" 10
  # Any `..` component in the RAW argument is rejected outright. Canonicalising the
  # parent would resolve it, but rejecting it here is clearer and defence in depth.
  case "$_dest" in
    *..*) sbx_fatal "destination contains a '..' component: $_dest" 10 ;;
  esac
  [ -L "$_dest" ] && sbx_fatal "destination is a symlink: $_dest" 10

  # Canonicalise the PARENT (which must already exist), then re-attach the final
  # component, so the path actually written is the path judged.
  _parent=$(dirname -- "$_dest")
  _base=$(basename -- "$_dest")
  _cp=$(sbx_canonical "$_parent") || sbx_fatal "destination's parent does not resolve: $_parent" 10
  _c="$_cp/$_base"
  [ -L "$_c" ] && sbx_fatal "destination is a symlink after canonicalisation: $_c" 10
  sbx_is_system_path "$_c" && sbx_fatal "destination is a system directory: $_c" 10
  sbx_is_child_of "$_c" "$SBX_ROOT" || sbx_fatal "destination escapes the disposable root: $_c" 10
  SBX_LAST_OK="$_c"
  return 0
}

# ---------------------------------------------------------------------------
# 5. Full initialisation: create the root, canonicalise, require the approved parent,
#    and expose SBX_ROOT / SBX_BIN for the caller.
# ---------------------------------------------------------------------------
# Read the approved root AT CALL TIME, not at source time. AuditReview5 (P0): the
# library captured `${SIGAP_TEST_ROOT:-}` when sourced, so a caller that set the variable
# after sourcing it had no effect — the suite then aborted with no output. Reading it
# here makes `SIGAP_TEST_ROOT=x sbx_init` and `SIGAP_TEST_ROOT=x; . lib; sbx_init` both
# work, which is the whole point of testing an unexported-variable failure mode.
sbx_init() {
  SBX_APPROVED_ROOT="${SIGAP_TEST_ROOT:-}"
  [ -n "$SBX_APPROVED_ROOT" ] || sbx_fatal "SIGAP_TEST_ROOT is not set; refusing to invent a default" 10
  _ar=$(sbx_canonical "$SBX_APPROVED_ROOT") || sbx_fatal "SIGAP_TEST_ROOT does not resolve: $SBX_APPROVED_ROOT" 10
  sbx_is_system_path "$_ar" && sbx_fatal "SIGAP_TEST_ROOT is a system directory: $_ar" 10
  [ -w "$_ar" ] || sbx_fatal "SIGAP_TEST_ROOT is not writable: $_ar" 11
  SBX_APPROVED_ROOT="$_ar"

  _r=$(sbx_new_root)
  _cr=$(sbx_canonical "$_r") || sbx_fatal "the new root does not resolve" 10
  sbx_is_child_of "$_cr" "$SBX_APPROVED_ROOT" || sbx_fatal "the new root is not inside the approved root: $_cr" 10

  SBX_ROOT="$_cr"
  SBX_CREATED="$_cr"          # ownership record: cleanup may remove ONLY this
  SBX_BIN="$SBX_ROOT/bin"
  mkdir "$SBX_BIN" "$SBX_ROOT/log" || sbx_fatal "cannot create bin/log inside the disposable root" 11

  # Ownership invariant: only the disposable directory CREATED by this invocation may
  # be recursively removed. QaReview5 and AuditReview5 both found that the previous
  # cleanup would `rm -rf` ANY child of the approved root, so a caller who replaced
  # SBX_ROOT with a sibling could delete a directory this invocation did not create.
  # The created root is recorded at init time and cleanup compares against that
  # recorded value, never against a mutable global.
  trap 'sbx_cleanup' EXIT INT TERM HUP
  printf 'sandbox root: %s\n' "$SBX_ROOT" >&2
}

# Cleanup is safe exactly when SBX_ROOT resolves to the root THIS invocation created.
sbx_cleanup() {
  [ -n "${SBX_ROOT:-}" ] || return 0
  [ -n "${SBX_CREATED:-}" ] || return 0          # never recorded → refuse
  _c=$(sbx_canonical "$SBX_ROOT") || return 0
  _created=$(sbx_canonical "$SBX_CREATED") || return 0
  # Both the current root and the recorded root must be children of the approved
  # parent AND must be the SAME directory. A sibling with a valid-looking path fails
  # the second test and survives.
  sbx_is_child_of "$_c" "${SBX_APPROVED_ROOT:-/nonexistent}" || return 0
  [ "$_c" = "$_created" ] || return 0
  [ -d "$_c" ] && rm -rf -- "$_c"
  return 0
}
