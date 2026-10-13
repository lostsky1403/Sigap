#!/bin/sh
# test-bucket-lock-drill.sh — proves Drill-BucketLockRetention.ps1 fails closed.
#
# The drill's value is entirely in its ASSERTIONS, and every one of them is a
# negative: a deletion that must be refused, a refusal that must name the
# retention reason, a production rule that must survive. A stub executor is the
# only way to exercise those paths without touching a real bucket, so this suite
# runs the drill against one and asserts the exit code and the verdict lines.
#
# The stub is written once in PowerShell and reached through a platform shim, so
# the same suite runs under ubuntu-latest and on a Windows workstation.
#
# Exit codes: 0 = PASS, 1 = FAIL, 2 = MISSING PREREQUISITE.

set -u

if ! command -v pwsh >/dev/null 2>&1; then
    echo "MISSING PREREQUISITE: pwsh is not on PATH" >&2
    exit 2
fi

REPO=$(cd "$(dirname "$0")/../.." && pwd)
DRILL="$REPO/scripts/ops/Drill-BucketLockRetention.ps1"
if [ ! -f "$DRILL" ]; then
    echo "MISSING PREREQUISITE: $DRILL not found" >&2
    exit 2
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

# --- stub executor ----------------------------------------------------------------------
cat > "$TMP/stub.ps1" <<'PS1'
param(
    [Parameter(Mandatory)][string]$Tool,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest
)
# Simulated Cloudflare R2 / Wrangler executor.
# STUB_MODE: normal | no-reason | no-lock | mutate-prod | no-rule-after-add
#            | prod-absent | short-retention
$mode = $env:STUB_MODE; if ([string]::IsNullOrWhiteSpace($mode)) { $mode = 'normal' }
$dir  = $env:STUB_STATE; if ([string]::IsNullOrWhiteSpace($dir)) { $dir = Join-Path ([System.IO.Path]::GetTempPath()) 'stub-state' }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$ruleFlag = Join-Path $dir 'rule'
$objFile  = Join-Path $dir 'objects'
$key = $Rest -join ' '
function Have-Rule { Test-Path $ruleFlag }

if ($Tool -eq 'npx') {
    if ($key -match '^wrangler whoami') {
        Write-Output "Account ID: $($env:R2_DRILL_ACCOUNT_ID)"; exit 0
    }
    if ($key -match 'r2 bucket lock list') {
        Write-Output "Listing lock rules for bucket sigap-backups:"
        $prodGone = ($mode -eq 'prod-absent') -or (Test-Path (Join-Path $dir 'prod-gone'))
        if (-not $prodGone) {
            $ret = if ($mode -eq 'short-retention') { '30' } else { '90' }
            Write-Output "  name: sigap-retention-90d"
            Write-Output "  enabled: Yes"
            Write-Output "  prefix: sigap/"
            Write-Output "  condition: after $ret days"
        }
        if (Have-Rule) {
            Write-Output "  name: lock-drill"
            Write-Output "  enabled: Yes"
            Write-Output "  prefix: lock-drill/x/"
            Write-Output "  condition: after 1 days"
        }
        exit 0
    }
    if ($key -match 'r2 bucket lock add') {
        if ($mode -ne 'no-rule-after-add') { Set-Content -Path $ruleFlag -Value 'present' }
        if ($mode -eq 'mutate-prod') { Set-Content -Path (Join-Path $dir 'prod-gone') -Value 'sticky' }
        Write-Output "Added lock rule 'lock-drill'."; exit 0
    }
    if ($key -match 'r2 bucket lock remove') {
        Remove-Item $ruleFlag -ErrorAction SilentlyContinue
        Write-Output "Removed lock rule 'lock-drill'."; exit 0
    }
    Write-Output "unhandled npx: $key"; exit 3
}

if ($Tool -eq 'aws') {
    if ($key -match 's3api put-object') {
        $k = ($Rest | Where-Object { $_ -match '^lock-drill/' } | Select-Object -First 1)
        Add-Content -Path $objFile -Value $k
        Write-Output '{"ETag":"stub"}'; exit 0
    }
    if ($key -match 's3api list-objects-v2') {
        $n = 0
        if (Test-Path $objFile) { $n = @(Get-Content $objFile | Where-Object { $_ -ne '' }).Count }
        Write-Output ([string]$n); exit 0
    }
    if ($key -match 's3api delete-object') {
        if (Have-Rule) {
            if ($mode -eq 'no-lock') {
                Remove-Item $objFile -ErrorAction SilentlyContinue
                Write-Output 'deleted'; exit 0
            }
            if ($mode -eq 'no-reason') {
                [Console]::Error.WriteLine('An error occurred (AccessDenied) when calling the DeleteObject operation: Access Denied')
                exit 254
            }
            [Console]::Error.WriteLine('An error occurred (AccessDenied) when calling the DeleteObject operation: Bucket is locked: object is under a retention rule (ObjectLock)')
            exit 254
        }
        Remove-Item $objFile -ErrorAction SilentlyContinue
        Write-Output 'deleted'; exit 0
    }
    Write-Output "unhandled aws: $key"; exit 3
}
Write-Output "unknown tool $Tool"; exit 3
PS1

# --- platform shim so pwsh can launch the stub as `aws` / `npx` -------------------------
if command -v cmd.exe >/dev/null 2>&1; then
    printf '@echo off\r\npwsh -NoProfile -ExecutionPolicy Bypass -File "%%~dp0stub.ps1" -Tool aws %%*\r\nexit /b %%errorlevel%%\r\n' > "$TMP/aws.cmd"
    printf '@echo off\r\npwsh -NoProfile -ExecutionPolicy Bypass -File "%%~dp0stub.ps1" -Tool npx %%*\r\nexit /b %%errorlevel%%\r\n' > "$TMP/npx.cmd"
    AWS_BIN="$TMP/aws.cmd"
    NPX_BIN="$TMP/npx.cmd"
else
    printf '#!/bin/sh\nexec pwsh -NoProfile -File "%s" -Tool aws "$@"\n' "$TMP/stub.ps1" > "$TMP/aws"
    printf '#!/bin/sh\nexec pwsh -NoProfile -File "%s" -Tool npx "$@"\n' "$TMP/stub.ps1" > "$TMP/npx"
    chmod +x "$TMP/aws" "$TMP/npx"
    AWS_BIN="$TMP/aws"
    NPX_BIN="$TMP/npx"
fi

failures=0
passes=0

# run_drill <expected_exit> <label> <mode> [extra env assignments...]
# Sets $out to the captured output and $rc to the exit code.
run_drill() {
    expected=$1; label=$2; mode=$3; shift 3
    state="$TMP/state"; rm -rf "$state"; mkdir -p "$state"

    out=$(
        env -u AWS_PROFILE -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY \
            -u AWS_SESSION_TOKEN -u AWS_SHARED_CREDENTIALS_FILE -u AWS_CONFIG_FILE \
            -u AWS_DEFAULT_PROFILE \
            R2_DRILL_ACCOUNT_ID=abc123def \
            R2_DRILL_ENDPOINT=https://abc123def.r2.cloudflarestorage.com \
            R2_DRILL_ACCESS_KEY_ID=stub-access \
            R2_DRILL_SECRET_ACCESS_KEY=stub-secret \
            R2_DRILL_AWS_BIN="$AWS_BIN" \
            R2_DRILL_NPX_BIN="$NPX_BIN" \
            R2_DRILL_EVIDENCE_DIR="$TMP/evidence" \
            STUB_STATE="$state" STUB_MODE="$mode" "$@" \
            pwsh -NoProfile -File "$DRILL" ${DRILL_ARGS:-} 2>&1
    )
    rc=$?

    if [ "$rc" -ne "$expected" ]; then
        echo "FAIL: $label -- expected exit $expected, got $rc"
        printf '%s\n' "$out" | sed 's/^/    | /'
        failures=$((failures + 1))
        return 1
    fi
    echo "PASS: $label (exit $rc)"
    passes=$((passes + 1))
    return 0
}

expect_verdict() {
    label=$1; key=$2; want=$3
    got=$(printf '%s\n' "$out" | grep -a "^$key" | head -1 | sed 's/.*= *//' | tr -d '\r')
    if [ "$got" != "$want" ]; then
        echo "FAIL: $label -- $key expected '$want', got '$got'"
        failures=$((failures + 1))
        return 1
    fi
    passes=$((passes + 1))
    return 0
}

echo "=== happy path ==="
if run_drill 0 "mechanism drill passes" normal; then
    expect_verdict "happy" "BUCKET_LOCK_CONFIG_VERIFIED" "PASS"
    expect_verdict "happy" "DISPOSABLE_LOCK_MECHANISM_TEST" "PASS"
    expect_verdict "happy" "PRODUCTION_RULE_UNCHANGED_AFTER_DRILL" "PASS"
fi

echo "=== negative controls: the drill must FAIL ==="
run_drill 1 "refusal without a retention reason fails" no-reason
if [ "$rc" -eq 1 ]; then expect_verdict "no-reason" "DISPOSABLE_LOCK_MECHANISM_TEST" "FAIL"; fi
run_drill 1 "a deletable locked object fails" no-lock
run_drill 1 "a changed production rule fails" mutate-prod
if [ "$rc" -eq 1 ]; then expect_verdict "mutate-prod" "PRODUCTION_RULE_UNCHANGED_AFTER_DRILL" "FAIL"; fi
run_drill 1 "a rule that never applied fails" no-rule-after-add

echo "=== guards: the drill must REFUSE before mutating ==="
run_drill 2 "ambient AWS credentials refused" normal AWS_ACCESS_KEY_ID=AKIAAMBIENT
run_drill 2 "non-R2 endpoint refused" normal R2_DRILL_ENDPOINT=https://s3.amazonaws.com
run_drill 2 "endpoint/account mismatch refused" normal R2_DRILL_ENDPOINT=https://zzzz.r2.cloudflarestorage.com
run_drill 2 "wrong bucket refused" normal R2_DRILL_BUCKET=someone-elses-bucket
run_drill 2 "missing production rule refused" prod-absent
run_drill 2 "under-90-day retention refused" short-retention

echo "=== pre-existing state must block the run ==="
rm -rf "$TMP/state"; mkdir -p "$TMP/state"; touch "$TMP/state/rule"
out=$(env -u AWS_ACCESS_KEY_ID R2_DRILL_ACCOUNT_ID=abc123def \
        R2_DRILL_ENDPOINT=https://abc123def.r2.cloudflarestorage.com \
        R2_DRILL_ACCESS_KEY_ID=k R2_DRILL_SECRET_ACCESS_KEY=s \
        R2_DRILL_AWS_BIN="$AWS_BIN" R2_DRILL_NPX_BIN="$NPX_BIN" \
        R2_DRILL_EVIDENCE_DIR="$TMP/evidence" STUB_STATE="$TMP/state" STUB_MODE=normal \
        pwsh -NoProfile -File "$DRILL" 2>&1)
rc=$?
if [ "$rc" -eq 2 ]; then echo "PASS: pre-existing drill rule refused (exit 2)"; passes=$((passes + 1));
else echo "FAIL: pre-existing drill rule -- expected exit 2, got $rc"; failures=$((failures + 1)); fi

echo "=== dry run must not mutate ==="
rm -rf "$TMP/state"; mkdir -p "$TMP/state"
DRILL_ARGS="-DryRun"
run_drill 0 "dry run exits 0" normal
unset DRILL_ARGS
if [ -f "$TMP/state/rule" ] || [ -f "$TMP/state/objects" ]; then
    echo "FAIL: dry run mutated state"
    failures=$((failures + 1))
else
    echo "PASS: dry run left no rule and no object"
    passes=$((passes + 1))
fi

echo
echo "checks: $passes passed, $failures failed"
if [ "$failures" -ne 0 ]; then
    exit 1
fi
echo "ALL BUCKET-LOCK DRILL TESTS PASSED"
exit 0
