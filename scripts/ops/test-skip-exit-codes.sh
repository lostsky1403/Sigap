#!/bin/sh
# test-skip-exit-codes.sh — proves the documented CI skip idiom actually works.
#
# The defect this pins: the runbook used to recommend
#
#     sh suite.sh; rc=$?; [ "$rc" -eq 2 ] || exit "$rc"
#
# Under `set -e` — which is what a GitHub Actions `run:` block uses — the suite's
# non-zero exit terminates the shell BEFORE the guard runs, so the guard is dead
# code and a clean skip (2) becomes a hard failure. The corrected form is the
# `if` form. This suite executes BOTH forms against synthetic exit codes 0, 1 and
# 2, under `set -e`, `set -eu`, and an ordinary shell, and asserts the outcome.
#
# Exit codes: 0 pass, 1 fail.

set -u

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-skip-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-34s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-34s %s\n' "$1" "$2"; }

# Three synthetic suites with the documented exit-code contract.
for code in 0 1 2; do
  cat > "$TMP/suite_$code.sh" <<EOF
#!/bin/sh
exit $code
EOF
  chmod +x "$TMP/suite_$code.sh"
done

# The CORRECTED idiom. Prints PASS/SKIP and returns 0; a genuine failure returns
# the suite's non-zero status. Uses `return`, not `exit`: `exit` inside a
# function would terminate the calling shell, which is exactly the class of
# mistake this file exists to catch.
correct_form() {
  if sh "$1"; then
    echo PASS
  else
    rc=$?
    if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; return "$rc"; fi
  fi
}

# The OLD (broken) idiom, kept only to demonstrate the defect.
broken_form() {
  sh "$1"; rc=$?; [ "$rc" -eq 2 ] || return "$rc"; echo REACHED
}

# --- 1. correct form, plain shell -------------------------------------------
r=$(correct_form "$TMP/suite_0.sh"); [ "$r" = "PASS" ] && ok "correct/plain/exit0" "PASS" || bad "correct/plain/exit0" "$r"
r=$(correct_form "$TMP/suite_2.sh"); [ "$r" = "SKIP" ] && ok "correct/plain/exit2" "SKIP" || bad "correct/plain/exit2" "$r"
correct_form "$TMP/suite_1.sh" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 1 ] && ok "correct/plain/exit1" "propagates 1" || bad "correct/plain/exit1" "rc=$rc want 1"

# --- 2. correct form under set -e -------------------------------------------
r=$(sh -e -c 'if sh '"$TMP"'/suite_0.sh; then echo PASS; else rc=$?; if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; exit "$rc"; fi; fi')
[ "$r" = "PASS" ] && ok "correct/set-e/exit0" "PASS" || bad "correct/set-e/exit0" "$r"

r=$(sh -e -c 'if sh '"$TMP"'/suite_2.sh; then echo PASS; else rc=$?; if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; exit "$rc"; fi; fi')
[ "$r" = "SKIP" ] && ok "correct/set-e/exit2" "SKIP (not a failure)" || bad "correct/set-e/exit2" "$r"

sh -e -c 'if sh '"$TMP"'/suite_1.sh; then echo PASS; else rc=$?; if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; exit "$rc"; fi; fi' >/dev/null 2>&1
rc=$?
[ "$rc" -ne 0 ] && ok "correct/set-e/exit1" "fails (rc=$rc)" || bad "correct/set-e/exit1" "exit 1 was swallowed"

# --- 3. correct form under set -eu ------------------------------------------
r=$(sh -eu -c 'if sh '"$TMP"'/suite_2.sh; then echo PASS; else rc=$?; if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; exit "$rc"; fi; fi')
[ "$r" = "SKIP" ] && ok "correct/set-eu/exit2" "SKIP" || bad "correct/set-eu/exit2" "$r"

sh -eu -c 'if sh '"$TMP"'/suite_1.sh; then echo PASS; else rc=$?; if [ "$rc" -eq 2 ]; then echo SKIP; else echo FAIL; exit "$rc"; fi; fi' >/dev/null 2>&1
rc=$?
[ "$rc" -ne 0 ] && ok "correct/set-eu/exit1" "fails (rc=$rc)" || bad "correct/set-eu/exit1" "exit 1 was swallowed"

# --- 4. the OLD form is demonstrably broken under set -e --------------------
# It must NOT report SKIP, because the guard after the failing command never runs.
r=$(sh -e -c 'sh '"$TMP"'/suite_2.sh; rc=$?; [ "$rc" -eq 2 ] || exit "$rc"; echo REACHED' 2>/dev/null)
rc=$?
if [ "$rc" -eq 2 ] && [ "$r" != "REACHED" ]; then
  ok "broken/set-e/exit2" "reproduced: exits 2, guard never runs"
else
  bad "broken/set-e/exit2" "expected exit 2 with no REACHED, got rc=$rc out=$r"
fi

# --- 5. anti-patterns must not be used --------------------------------------
# `|| true` turns a real failure into a pass; assert that is true, so the
# prohibition in the runbook is grounded in observed behaviour.
sh -e -c 'sh '"$TMP"'/suite_1.sh || true' >/dev/null 2>&1
[ "$?" -eq 0 ] && ok "antipattern/exit1-swallowed" "|| true masks exit 1 (do not use)" \
  || bad "antipattern/exit1-swallowed" "expected masking to succeed"

# --- 6. the runbook documents the CORRECT idiom and does not recommend the broken one
#
# Two assertions, because either alone is porous:
#   (a) the guarded if-form must actually be PRESENT — grepping only for the
#       fragment `[ "$rc" -eq 2 ] || exit "$rc"` passes whenever that fragment
#       appears anywhere, including inside the prohibition sentence; and
#   (b) every line that shows `; rc=$?` must be a PROHIBITION. A future edit that
#       recommended the broken form for a different script path would otherwise
#       slip through a check hard-coded to one filename.
RB=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)/docs/operations/DEPLOYMENT_RUNBOOK.md
if [ -f "$RB" ]; then
  if ! grep -q 'if sh scripts/ops/' "$RB"; then
    bad "runbook/idiom" "no guarded 'if sh ...; then' skip form present"
  else
    ok "runbook/idiom-present" "documents the guarded if-form"
  fi

  # `; rc=$?` on a line that is not a prohibition = the broken idiom recommended.
  broken=$(grep -n '; rc=\$?' "$RB" | grep -vi 'not use\|never\|anti-pattern\|broken\|dead code')
  if [ -n "$broken" ]; then
    bad "runbook/idiom-recommended" "the broken '; rc=\$?' idiom appears outside a prohibition"
    printf '  %s\n' "$broken"
  else
    ok "runbook/idiom-not-recommended" "the broken idiom appears only as a prohibition"
  fi
else
  bad "runbook/idiom" "runbook not found at $RB"
fi

printf 'skip-exit-code tests: %s pass, %s fail\n' "$pass" "$failed"
[ "$failed" -eq 0 ]
