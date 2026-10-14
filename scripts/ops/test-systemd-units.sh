#!/bin/sh
# test-systemd-units.sh — static guards for the AUDIT-701 backup systemd units.
#
# The defect this pins, found 2026-10-14: the timer carried TWO OnCalendar directives,
#
#     OnCalendar=daily        # == *-*-* 00:00:00
#     OnCalendar=03:00        # == *-*-* 03:00:00
#
# and systemd.timer(5) says multiple OnCalendar lines are combined with OR — "the timer
# unit will trigger whenever any of the specified timer expressions elapse". So the unit
# fired the database backup TWICE a day, at 00:00 and again at 03:00, while its own
# Description and docs/operations/BACKUP_RESTORE.md both claimed a single 03:00 run.
#
# A second, quieter defect lives in the same setting: an OnCalendar expression is
# evaluated in the HOST's local timezone unless it names one. The documentation promised
# "03:00 UTC", but the unit said `03:00`, so on any host not running UTC the backup would
# have run at 03:00 local — a silent schedule change rather than a loud failure. systemd
# accepts the literal `UTC` as a timezone suffix, so the unit must carry it.
#
# These are static file checks: no systemd, no host, no container, no network.
#
# Usage:  sh scripts/ops/test-systemd-units.sh [unit-dir]
#         (defaults to deploy/systemd)
#
# Exit codes: 0 = PASS, 1 = FAIL.

set -u

REPO=$(cd "$(dirname "$0")/../.." && pwd)
UNIT_DIR=${1:-$REPO/deploy/systemd}

TIMER="$UNIT_DIR/sigap-postgres-backup.timer"
SERVICE="$UNIT_DIR/sigap-postgres-backup.service"
WRAPPER="$UNIT_DIR/sigap-backup-postgres.sh"

pass=0
failed=0
CHECKS_RUN=0
ok()  { pass=$((pass + 1)); }
bad() { printf 'FAIL: %s\n' "$1"; failed=$((failed + 1)); }

# validate_units prints NOTHING on success and counts its own checks in CHECKS_RUN. It must
# not touch the case counters: it is also called against deliberately-defective copies,
# where a passing sub-check is not a passing case.
vchk() { CHECKS_RUN=$((CHECKS_RUN + 1)); }

# --- one directive reader ----------------------------------------------------------------
# Reads a `Key=Value` setting, ignoring comments and blank lines. Prints the value(s), one
# per line, so a caller can COUNT them. Comments must be skipped or the explanatory
# `# OnCalendar=...` lines added alongside the fix would be counted as directives.
directive_values() {
    _file=$1; _key=$2
    [ -f "$_file" ] || return 0
    sed -e 's/\r$//' "$_file" \
      | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' \
      | grep -v '^#' \
      | grep -v '^$' \
      | sed -n "s/^${_key}[[:space:]]*=[[:space:]]*//p"
}

directive_count() { directive_values "$1" "$2" | grep -c . ; }

# --- the checks, as a function so the negative controls can reuse them --------------------
# Returns non-zero if any check fails. All checks are pure text inspection.
validate_units() {
    _d=$1
    _timer="$_d/sigap-postgres-backup.timer"
    _service="$_d/sigap-postgres-backup.service"
    _wrapper="$_d/sigap-backup-postgres.sh"
    _rc=0

    for _f in "$_timer" "$_service" "$_wrapper"; do
        [ -f "$_f" ] || { printf 'FAIL: missing unit file %s\n' "$_f" >&2; return 1; }
    done

    # 1. EXACTLY ONE OnCalendar. This is the defect: two lines fire twice a day.
    _n=$(directive_count "$_timer" OnCalendar)
    if [ "$_n" -eq 1 ]; then
        vchk
    else
        printf 'FAIL: %s has %s OnCalendar directives (must be exactly 1 — systemd ORs multiple lines, so each extra one fires the backup again)\n' "$_timer" "$_n" >&2
        _rc=1
    fi

    # 2. It resolves to 03:00:00, the documented run time.
    _cal=$(directive_values "$_timer" OnCalendar | head -1)
    case "$_cal" in
        *03:00:00*) vchk ;;
        *) printf 'FAIL: OnCalendar is %s, which does not resolve to 03:00:00\n' "'$_cal'" >&2; _rc=1 ;;
    esac

    # 3. It names the timezone. Without this the documented UTC is a host-local claim.
    case "$_cal" in
        *UTC) vchk ;;
        *) printf 'FAIL: OnCalendar %s does not name a timezone (UTC); an unnamed expression is evaluated in the HOST local zone, silently changing the run time\n' "'$_cal'" >&2; _rc=1 ;;
    esac

    # 4. Persistent=true, so a missed run is caught up after downtime.
    if [ "$(directive_values "$_timer" Persistent | head -1)" = "true" ]; then vchk
    else printf 'FAIL: Persistent is not true\n' >&2; _rc=1; fi

    # 5. A randomized delay, so the run does not pile onto the same instant.
    if [ -n "$(directive_values "$_timer" RandomizedDelaySec | head -1)" ]; then vchk
    else printf 'FAIL: RandomizedDelaySec is not set\n' >&2; _rc=1; fi

    # 6. The timer activates the backup service by name.
    if [ "$(directive_values "$_timer" Unit | head -1)" = "sigap-postgres-backup.service" ]; then vchk
    else printf 'FAIL: Unit= does not name sigap-postgres-backup.service\n' >&2; _rc=1; fi

    # 7. The service is oneshot — a long-running Type would hold the timer's unit open.
    if [ "$(directive_values "$_service" Type | head -1)" = "oneshot" ]; then vchk
    else printf 'FAIL: service Type is not oneshot\n' >&2; _rc=1; fi

    # 8. ExecStart invokes the installed wrapper.
    case "$(directive_values "$_service" ExecStart | head -1)" in
        */sigap-backup-postgres.sh*) vchk ;;
        *) printf 'FAIL: ExecStart does not invoke sigap-backup-postgres.sh\n' >&2; _rc=1 ;;
    esac

    # 9. Runtime credentials come from the EnvironmentFile, never from the unit.
    if [ -n "$(directive_values "$_service" EnvironmentFile | head -1)" ]; then vchk
    else printf 'FAIL: service declares no EnvironmentFile\n' >&2; _rc=1; fi

    # 10. The wrapper execs the real backup script.
    if sed -e 's/\r$//' "$_wrapper" | grep -q 'exec .*backup-postgres\.sh'; then vchk
    else printf 'FAIL: wrapper does not exec backup-postgres.sh\n' >&2; _rc=1; fi

    # 11. NO CR. The repository stores these with LF, but a Windows checkout can
    #     reintroduce CR, and a unit file whose values end in `\r` is a real hazard on the
    #     Linux host: the setting value gains a trailing carriage return.
    #
    #     Detected with `tr`, NOT `grep -q "$(printf '\r')"`. That form looks obvious and
    #     silently never matches: the CR produced by command substitution is not usable as
    #     a grep pattern here, so the check passed on every CRLF file while `tr -dc '\r'`
    #     counted the carriage returns correctly. A guard that cannot fail is worse than no
    #     guard, so the count is taken directly.
    for _f in "$_timer" "$_service" "$_wrapper"; do
        _cr=$(tr -dc '\r' < "$_f" | wc -c | tr -d ' ')
        if [ "${_cr:-0}" -gt 0 ]; then
            printf 'FAIL: %s contains %s CR byte(s) (CRLF line endings); units must be LF\n' "$_f" "$_cr" >&2
            _rc=1
        else
            vchk
        fi
    done

    return "$_rc"
}

# --- 1. the real units must pass ---------------------------------------------------------
printf '=== 1. the committed units ===\n'
if validate_units "$UNIT_DIR"; then
    ok
    printf 'ok   committed units satisfy every guard (%s checks)\n' "$CHECKS_RUN"
else
    bad "committed units failed validation"
fi

# --- 2. negative controls: each defect MUST be caught ------------------------------------
# The validator is re-run against mutated COPIES, so the checks are proven non-vacuous.
printf '=== 2. negative controls (each must be caught) ===\n'
TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-units-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

seed() {
    rm -rf "$TMP/case"; mkdir -p "$TMP/case"
    cp "$UNIT_DIR"/* "$TMP/case/" 2>/dev/null || true
}

# Each case: seed, mutate, assert the validator FAILS, and assert it fails for the RIGHT
# reason — a matching exit status alone would pass even if an unrelated check had fired.
expect_reject() {
    _label=$1; _pattern=$2
    _out=$(validate_units "$TMP/case" 2>&1)
    if [ $? -eq 0 ]; then
        bad "$_label -- validator ACCEPTED a defective unit set"
        return 1
    fi
    if printf '%s' "$_out" | grep -qiE "$_pattern"; then
        printf 'ok   rejected: %s\n' "$_label"
        pass=$((pass + 1))
    else
        bad "$_label -- rejected, but not for the expected reason"
        printf '     expected /%s/\n' "$_pattern"
        printf '%s\n' "$_out" | sed 's/^/     | /'
        return 1
    fi
}

# The original defect, exactly: `daily` AND `03:00`.
seed
sed -e 's/^OnCalendar=.*/OnCalendar=daily\nOnCalendar=03:00/' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "two OnCalendar directives (the original defect)" 'has 2 OnCalendar'

# Two identical lines are still two triggers.
seed
sed -e 's/^OnCalendar=.*/OnCalendar=*-*-* 03:00:00 UTC\nOnCalendar=*-*-* 03:00:00 UTC/' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "duplicated identical OnCalendar" 'has 2 OnCalendar'

# No schedule at all.
seed
sed -e '/^OnCalendar=/d' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "no OnCalendar directive" 'has 0 OnCalendar'

# A timezone-less expression: the documented UTC becomes host-local.
seed
sed -e 's/^OnCalendar=.*/OnCalendar=*-*-* 03:00:00/' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "OnCalendar without a timezone" 'does not name a timezone'

# A wrong time, timezone-correct.
seed
sed -e 's/^OnCalendar=.*/OnCalendar=*-*-* 00:00:00 UTC/' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "OnCalendar at the wrong time" 'does not resolve to 03:00:00'

# Persistent disabled.
seed
sed -e 's/^Persistent=.*/Persistent=false/' "$TMP/case/sigap-postgres-backup.timer" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.timer"
expect_reject "Persistent=false" 'Persistent is not true'

# CRLF reintroduced (a Windows checkout hazard).
seed
for f in "$TMP/case"/*; do sed -e 's/$/\r/' "$f" > "$TMP/t" && mv "$TMP/t" "$f"; done
expect_reject "CRLF line endings" 'contains [0-9]+ CR byte'

# A service that no longer runs the wrapper.
seed
sed -e 's#^ExecStart=.*#ExecStart=/bin/true#' "$TMP/case/sigap-postgres-backup.service" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.service"
expect_reject "ExecStart does not run the wrapper" 'does not invoke sigap-backup-postgres.sh'

# A service whose type holds the unit open.
seed
sed -e 's/^Type=.*/Type=simple/' "$TMP/case/sigap-postgres-backup.service" > "$TMP/t" && mv "$TMP/t" "$TMP/case/sigap-postgres-backup.service"
expect_reject "service Type is not oneshot" 'Type is not oneshot'

# A missing unit file.
seed
rm -f "$TMP/case/sigap-postgres-backup.timer"
expect_reject "a missing unit file" 'missing unit file'

# --- 3. the validator must ACCEPT a correct set (guards against over-rejection) ----------
printf '=== 3. a correct set is accepted ===\n'
seed
if validate_units "$TMP/case" >/dev/null 2>&1; then
    printf 'ok   unmodified copy accepted\n'
    pass=$((pass + 1))
else
    bad "the validator rejected an unmodified copy of the committed units"
fi

printf '\nchecks: %s passed, %s failed\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
printf 'ALL SYSTEMD UNIT GUARDS PASSED\n'
exit 0
