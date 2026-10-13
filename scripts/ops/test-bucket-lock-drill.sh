#!/bin/sh
# test-bucket-lock-drill.sh — proves Drill-BucketLockRetention.ps1 fails closed.
#
# The drill's value is entirely in its ASSERTIONS, and every one of them is a
# negative: a deletion that must be refused, a refusal that must name the
# retention reason, a production rule that must survive. A stub executor is the
# only way to exercise those paths without touching a real bucket, so this suite
# runs the drill against one and asserts the exit code, the specific refusal
# REASON, and the absence of mutation.
#
# A matching exit code is NOT treated as proof that the intended guard was
# reached: every refusal case asserts the reason text it must produce, because
# an unrelated guard exiting 2 would otherwise pass as the case under test.
#
# The stub is written once in PowerShell and reached through a platform shim, so
# the same suite runs under ubuntu-latest and on a Windows workstation.
#
# Exit codes: 0 = PASS, 1 = FAIL, 2 = MISSING PREREQUISITE.

set -u

# Overridable so the same suite can be run against Windows PowerShell 5.1
# (PWSH=powershell) as well as pwsh 7. The drill must work on both.
PWSH=${PWSH:-pwsh}

if ! command -v "$PWSH" >/dev/null 2>&1; then
    echo "MISSING PREREQUISITE: $PWSH is not on PATH" >&2
    exit 2
fi

REPO=$(cd "$(dirname "$0")/../.." && pwd)
DRILL="$REPO/scripts/ops/Drill-BucketLockRetention.ps1"
if [ ! -f "$DRILL" ]; then
    echo "MISSING PREREQUISITE: $DRILL not found" >&2
    exit 2
fi

# A real Cloudflare account id is 32 lowercase hex characters. The endpoint must be exactly
# <id>.r2.cloudflarestorage.com, so the harness uses a well-formed id to exercise the strict
# comparison rather than a short placeholder that would fail for the wrong reason.
ACCT=0123456789abcdef0123456789abcdef
ENDPOINT="https://$ACCT.r2.cloudflarestorage.com"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT INT TERM

# --- stub executor ----------------------------------------------------------------------
cat > "$TMP/stub.ps1" <<'PS1'
param(
    [Parameter(Mandatory)][string]$Tool,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest
)
# Simulated Cloudflare R2 / Wrangler executor.
$mode = $env:STUB_MODE; if ([string]::IsNullOrWhiteSpace($mode)) { $mode = 'normal' }
$dir  = $env:STUB_STATE; if ([string]::IsNullOrWhiteSpace($dir)) { $dir = Join-Path ([System.IO.Path]::GetTempPath()) 'stub-state' }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$ruleFlag = Join-Path $dir 'rule'
$objFile  = Join-Path $dir 'objects'
$key = $Rest -join ' '
function Have-Rule { Test-Path $ruleFlag }

# One lock rule record, with individual fields suppressible so a record can be made
# malformed, disabled, mis-scoped or missing a field.
function Emit-Rule {
    param(
        [string]$Name = 'sigap-retention-90d',
        [string]$Enabled = 'Yes',
        [string]$Prefix = 'sigap/',
        [string]$Condition = 'after 90 days',
        [switch]$NoEnabled, [switch]$NoPrefix, [switch]$NoCondition
    )
    if ($Name) { Write-Output "  name: $Name" }
    if (-not $NoEnabled)   { Write-Output "  enabled: $Enabled" }
    if (-not $NoPrefix)    { Write-Output "  prefix: $Prefix" }
    if (-not $NoCondition) { Write-Output "  condition: $Condition" }
}

if ($Tool -eq 'npx') {
    if ($key -match '^wrangler whoami') {
        Write-Output "Account ID: $($env:R2_DRILL_ACCOUNT_ID)"; exit 0
    }
    if ($key -match 'r2 bucket lock list') {
        Write-Output "Listing lock rules for bucket sigap-backups:"
        $prodGone = ($mode -eq 'prod-absent') -or (Test-Path (Join-Path $dir 'prod-gone'))
        if (-not $prodGone) {
            switch ($mode) {
                'prod-disabled'       { Emit-Rule -Enabled 'No' }
                'prod-enabled-false'  { Emit-Rule -Enabled 'false' }
                'enabled-missing'     { Emit-Rule -NoEnabled }
                'enabled-empty'       { Emit-Rule -Enabled '' }
                'prefix-wrong'        { Emit-Rule -Prefix 'sigap' }
                'prefix-other'        { Emit-Rule -Prefix 'other/' }
                'prefix-empty'        { Emit-Rule -Prefix '' }
                'prefix-missing'      { Emit-Rule -NoPrefix }
                'retention-missing'   { Emit-Rule -NoCondition }
                'retention-empty'     { Emit-Rule -Condition '' }
                'retention-malformed' { Emit-Rule -Condition 'soon' }
                'retention-date'      { Emit-Rule -Condition 'after 2027-01-01' }
                'retention-indef'     { Emit-Rule -Condition 'indefinite' }
                'retention-89'        { Emit-Rule -Condition 'after 89 days' }
                'retention-1'         { Emit-Rule -Condition 'after 1 days' }
                'retention-100'       { Emit-Rule -Condition 'after 100 days' }
                'retention-365'       { Emit-Rule -Condition 'after 365 days' }
                'retention-plain'     { Emit-Rule -Condition '90 days' }
                'retention-day'       { Emit-Rule -Condition 'after 90 day' }
                'prod-duplicate'      { Emit-Rule; Emit-Rule }
                'prod-name-case'      { Emit-Rule -Name 'SIGAP-RETENTION-90D' }
                'prod-name-superstr'  { Emit-Rule -Name 'sigap-retention-90d-extra' }
                'contradictory'       { Emit-Rule; Write-Output "  enabled: No" }
                'prod-empty-name'     { Write-Output "  name:"; Write-Output "  enabled: Yes"; Write-Output "  prefix: sigap/"; Write-Output "  condition: after 90 days" }
                # Attributes split across TWO rules: the production rule supplies name and
                # prefix, a different rule supplies the retention. The old whole-output
                # matching could not tell them apart.
                'mixed'               { Emit-Rule -NoCondition; Emit-Rule -Name 'other-rule' -Prefix 'other/' -Condition 'after 90 days' }
                default               { Emit-Rule }
            }
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
out=''
rc=0

# invoke_drill <mode> [env assignments...] ; sets $out and $rc, fresh stub state
invoke_drill() {
    mode=$1; shift
    rm -rf "$TMP/state"; mkdir -p "$TMP/state"
    out=$(
        env -u AWS_PROFILE -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY \
            -u AWS_SESSION_TOKEN -u AWS_SHARED_CREDENTIALS_FILE -u AWS_CONFIG_FILE \
            -u AWS_DEFAULT_PROFILE \
            R2_DRILL_ACCOUNT_ID="$ACCT" \
            R2_DRILL_ENDPOINT="$ENDPOINT" \
            R2_DRILL_ACCESS_KEY_ID=stub-access \
            R2_DRILL_SECRET_ACCESS_KEY=stub-secret \
            R2_DRILL_AWS_BIN="$AWS_BIN" \
            R2_DRILL_NPX_BIN="$NPX_BIN" \
            R2_DRILL_EVIDENCE_DIR="$TMP/evidence" \
            STUB_STATE="$TMP/state" STUB_MODE="$mode" "$@" \
            "$PWSH" -NoProfile -File "$DRILL" ${DRILL_ARGS:-} 2>&1
    )
    rc=$?
}

ok()   { passes=$((passes + 1));   echo "PASS: $1"; }
bad()  { failures=$((failures + 1)); echo "FAIL: $1"; }
dump() { printf '%s\n' "$out" | sed 's/^/    | /'; }

check_exit() {
    want=$1; label=$2
    if [ "$rc" -ne "$want" ]; then bad "$label -- expected exit $want, got $rc"; dump; return 1; fi
    ok "$label (exit $rc)"
    return 0
}

# The specific refusal REASON, not merely a non-zero exit. An unrelated guard exiting 2
# would otherwise satisfy a case that never reached the guard it names.
check_reason() {
    label=$1; re=$2
    if printf '%s' "$out" | grep -aqE "$re"; then
        ok "$label -- refused for the expected reason"
        return 0
    fi
    bad "$label -- refusal reason did not match /$re/"
    dump
    return 1
}

# No bucket mutation may have occurred on a refusal path.
check_no_mutation() {
    label=$1
    if [ -f "$TMP/state/rule" ] || [ -f "$TMP/state/objects" ]; then
        bad "$label -- refused but MUTATED stub state"
        return 1
    fi
    ok "$label -- no mutation"
    return 0
}

check_verdict() {
    key=$1; want=$2
    got=$(printf '%s\n' "$out" | grep -a "^$key" | head -1 | sed 's/.*= *//' | tr -d '\r')
    if [ "$got" != "$want" ]; then bad "$key expected '$want', got '$got'"; return 1; fi
    ok "$key = $want"
    return 0
}

# Reads the verdict block from a file (used where the run is not via invoke_drill).
check_verdict_file() {
    file=$1; key=$2; want=$3
    got=$(grep -a "^$key" "$file" | head -1 | sed 's/.*= *//' | tr -d '\r')
    if [ "$got" != "$want" ]; then bad "$key expected '$want', got '$got'"; return 1; fi
    ok "$key = $want"
    return 0
}

# An internal error (exit 3) is never an acceptable outcome for any case.
check_not_internal() {
    label=$1
    if printf '%s' "$out" | grep -aq "INTERNAL ERROR"; then
        bad "$label -- the drill hit an INTERNAL ERROR (exit $rc); no bucket conclusion is valid"
        dump
        return 1
    fi
    return 0
}

# refusal <label> <mode> <reason-regex> [env assignments...]
refusal() {
    label=$1; mode=$2; re=$3; shift 3
    invoke_drill "$mode" "$@"
    check_exit 2 "$label" || return 1
    check_reason "$label" "$re" || return 1
    check_no_mutation "$label"
}

echo "=== happy path ==="
invoke_drill normal
if check_exit 0 "mechanism drill passes"; then
    check_verdict "BUCKET_LOCK_CONFIG_VERIFIED" "PASS"
    check_verdict "DISPOSABLE_LOCK_MECHANISM_TEST" "PASS"
    check_verdict "PRODUCTION_RULE_UNCHANGED_AFTER_DRILL" "PASS"
fi

echo "=== retention duration is compared numerically, not shape-matched ==="
# The old regex was `\b9[0-9]+\b`, which matches only 90-99. It therefore REJECTED a
# stronger 100- or 365-day retention while accepting the recorded band -- measured against
# the pre-fix drill: retention-100 and retention-365 both exited 2 there.
for m in retention-100 retention-365 retention-plain retention-day; do
    invoke_drill "$m"
    check_exit 0 "retention accepted: $m"
done
# 89 is below the floor and must be REFUSED under the numeric comparison.
refusal "retention 89 days refused"      retention-89        "below the required"
refusal "retention 1 day refused"        retention-1         "below the required"

echo "=== retention must be present, parseable and unambiguous ==="
refusal "retention missing refused"      retention-missing   "no retention field"
refusal "retention empty refused"        retention-empty     "could not parse"
refusal "retention malformed refused"    retention-malformed "could not parse"
refusal "retention date refused"         retention-date      "could not parse"
refusal "retention indefinite refused"   retention-indef     "indefinite"

echo "=== the production rule must be ENABLED ==="
refusal "disabled production rule refused"   prod-disabled      "is NOT enabled"
refusal "enabled=false refused"              prod-enabled-false "is NOT enabled"
refusal "enabled missing refused"            enabled-missing    "no 'enabled' field"
refusal "enabled empty refused"              enabled-empty      "is NOT enabled"

echo "=== attributes must come from the SAME rule ==="
# The production rule supplies name and prefix; a different rule supplies the retention.
# Whole-output matching could not tell them apart and PASSED this.
refusal "mixed-rule retention refused"   mixed               "no retention field"

echo "=== exactly one production rule, with a coherent record ==="
refusal "duplicate production rule refused"  prod-duplicate   "found 2 rules named"
refusal "contradictory record refused"       contradictory    "contradictory record"
refusal "empty rule name refused"            prod-empty-name  "empty 'name' field"
refusal "production rule absent refused"     prod-absent      "is not present"
# A differently-cased name is NOT the production rule: R2 rule names are case-sensitive,
# so treating it as a match would validate the wrong rule's attributes.
refusal "differently-cased name refused"     prod-name-case     "is not present"
refusal "superstring name refused"           prod-name-superstr "is not present"

echo "=== prefix must be the recorded scope ==="
refusal "prefix 'sigap' refused"         prefix-wrong   "not the recorded"
refusal "prefix 'other/' refused"        prefix-other   "not the recorded"
refusal "prefix empty refused"           prefix-empty   "not the recorded"
refusal "prefix missing refused"         prefix-missing "no 'prefix' field"

echo "=== endpoint must be EXACTLY <account>.r2.cloudflarestorage.com ==="
refusal "non-R2 endpoint refused"        normal "must be exactly"  R2_DRILL_ENDPOINT=https://s3.amazonaws.com
refusal "extra host label refused"       normal "must be exactly"  R2_DRILL_ENDPOINT="https://$ACCT.r2.cloudflarestorage.com.evil.example"
refusal "subdomain prefix refused"       normal "must be exactly"  R2_DRILL_ENDPOINT="https://evil.$ACCT.r2.cloudflarestorage.com"
refusal "longer id refused"              normal "must be exactly"  R2_DRILL_ENDPOINT="https://${ACCT}00.r2.cloudflarestorage.com"
refusal "trailing dot refused"           normal "must be exactly"  R2_DRILL_ENDPOINT="https://$ACCT.r2.cloudflarestorage.com."
refusal "endpoint with path refused"     normal "must be a bare"   R2_DRILL_ENDPOINT="https://$ACCT.r2.cloudflarestorage.com/bucket"
refusal "http endpoint refused"          normal "must be a bare"   R2_DRILL_ENDPOINT="http://$ACCT.r2.cloudflarestorage.com"
refusal "short account id refused"       normal "32-character"     R2_DRILL_ACCOUNT_ID=abc123def
refusal "uppercase account id refused"   normal "32-character"     R2_DRILL_ACCOUNT_ID=0123456789ABCDEF0123456789ABCDEF
refusal "non-hex account id refused"     normal "32-character"     R2_DRILL_ACCOUNT_ID=0123456789abcdef0123456789abcdeg
refusal "non-hex endpoint host refused"  normal "32-character"     R2_DRILL_ACCOUNT_ID=zzzz456789abcdef0123456789abcdef
# A trailing slash on an otherwise exact endpoint is the one accepted variant.
invoke_drill normal R2_DRILL_ENDPOINT="$ENDPOINT/"
check_exit 0 "endpoint with trailing slash accepted"

echo "=== credentials and bucket ==="
refusal "ambient AWS credentials refused"  normal "ambient AWS credential"  AWS_ACCESS_KEY_ID=AKIAAMBIENT
refusal "ambient AWS profile refused"      normal "ambient AWS credential"  AWS_PROFILE=default
refusal "wrong bucket refused"             normal "verified production bucket"  R2_DRILL_BUCKET=someone-elses-bucket

echo "=== negative controls: the drill must FAIL ==="
invoke_drill no-reason
if check_exit 1 "refusal without a retention reason fails"; then
    check_not_internal "no-reason"
    check_verdict "DISPOSABLE_LOCK_MECHANISM_TEST" "FAIL"
fi
invoke_drill no-lock
if check_exit 1 "a deletable locked object fails"; then
    check_not_internal "no-lock"
    check_verdict "DISPOSABLE_LOCK_MECHANISM_TEST" "FAIL"
fi
invoke_drill mutate-prod
if check_exit 1 "a changed production rule fails"; then
    check_not_internal "mutate-prod"
    check_verdict "PRODUCTION_RULE_UNCHANGED_AFTER_DRILL" "FAIL"
fi
invoke_drill no-rule-after-add
if check_exit 1 "a rule that never applied fails"; then
    check_not_internal "no-rule-after-add"
    check_verdict "DISPOSABLE_LOCK_MECHANISM_TEST" "FAIL"
fi

echo "=== pre-existing state must block the run ==="
rm -rf "$TMP/state"; mkdir -p "$TMP/state"; touch "$TMP/state/rule"
out=$(
    env -u AWS_PROFILE -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY \
        -u AWS_SESSION_TOKEN -u AWS_SHARED_CREDENTIALS_FILE -u AWS_CONFIG_FILE \
        -u AWS_DEFAULT_PROFILE \
        R2_DRILL_ACCOUNT_ID="$ACCT" R2_DRILL_ENDPOINT="$ENDPOINT" \
        R2_DRILL_ACCESS_KEY_ID=k R2_DRILL_SECRET_ACCESS_KEY=s \
        R2_DRILL_AWS_BIN="$AWS_BIN" R2_DRILL_NPX_BIN="$NPX_BIN" \
        R2_DRILL_EVIDENCE_DIR="$TMP/evidence" STUB_STATE="$TMP/state" STUB_MODE=normal \
        "$PWSH" -NoProfile -File "$DRILL" 2>&1
)
rc=$?
if check_exit 2 "pre-existing drill rule refused"; then
    check_reason "pre-existing drill rule refused" "already exists"
fi

echo "=== dry run must not mutate ==="
rm -rf "$TMP/state"; mkdir -p "$TMP/state"
DRILL_ARGS="-DryRun"
invoke_drill normal
unset DRILL_ARGS
if check_exit 0 "dry run exits 0"; then check_no_mutation "dry run"; fi

echo
echo "checks: $passes passed, $failures failed"
if [ "$failures" -ne 0 ]; then
    exit 1
fi
echo "ALL BUCKET-LOCK DRILL TESTS PASSED"
exit 0
