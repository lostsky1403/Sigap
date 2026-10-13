<#
.SYNOPSIS
    Real Cloudflare R2 Bucket Lock retention drill (runbook 13g.14, Part 1).

.DESCRIPTION
    Proves the retention MECHANISM on the real service, on a disposable prefix,
    without touching sigap/. It creates a short-lived rule scoped to
    lock-drill/<STAMP>/, uploads one throwaway object, asserts that deletion is
    refused FOR THE RETENTION REASON, removes the rule, asserts the object then
    becomes deletable, and proves cleanup.

    It NEVER modifies sigap-retention-90d. That is enforced mechanically: every
    mutating invocation is screened and refused if it names the production rule.

    This mutates a real bucket. Owner authorization is required. A passing run
    proves the disposable-prefix mechanism only -- it is NOT proof that deletion
    under the production sigap/ prefix was exercised.

.PARAMETER DryRun
    Run every preflight guard and print the exact plan. Performs no mutation.

.EXAMPLE
    pwsh -NoProfile -File scripts/ops/Drill-BucketLockRetention.ps1 -DryRun
    pwsh -NoProfile -File scripts/ops/Drill-BucketLockRetention.ps1

.NOTES
    Required environment (drill-specific names, deliberately NOT AWS_*):
      R2_DRILL_ACCOUNT_ID         Cloudflare account id; must match `wrangler whoami`
      R2_DRILL_ENDPOINT           https://<accountid>.r2.cloudflarestorage.com
      R2_DRILL_ACCESS_KEY_ID      bucket-scoped Object Read & Write token (A5)
      R2_DRILL_SECRET_ACCESS_KEY  its secret

    Optional:
      R2_DRILL_BUCKET             default sigap-backups
      R2_DRILL_AWS_BIN            default: aws resolved from PATH
      R2_DRILL_NPX_BIN            default: npx resolved from PATH
      R2_DRILL_EVIDENCE_DIR       default: ./lock-drill-evidence/<STAMP>

    Exit codes:
      0  drill PASSED (all three verdicts PASS)
      1  drill FAILED (an assertion did not hold)
      2  refused before mutating (preflight or guard)
#>

[CmdletBinding()]
param(
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# PowerShell 7.3+ turns native-command stderr into a terminating error when this is on.
# The drill depends on capturing an EXPECTED non-zero delete, so it must be off.
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$EXIT_PASS     = 0
$EXIT_FAIL     = 1
$EXIT_REFUSED  = 2

# The disposable namespace. Both are asserted below and must never be production values.
$DRILL_PREFIX   = 'lock-drill'
$DRILL_RULE     = 'lock-drill'
$PROD_RULE      = 'sigap-retention-90d'
$PROD_PREFIX    = 'sigap/'
$PROD_MIN_DAYS  = 90

$script:Evidence = New-Object System.Collections.ArrayList

function Say  { param([string]$m, [string]$c = 'Gray')  Write-Host $m -ForegroundColor $c }
function Ok   { param([string]$m) Say "ok: $m"   'Green' }
function Warn { param([string]$m) Say "warn: $m" 'Yellow' }
function Fail { param([string]$m) Say "FAIL: $m" 'Red' }
function Note { param([string]$m) Say "  $m"     'DarkGray' }
function Refuse {
    param([string]$m)
    Say "REFUSED: $m" 'Red'
    Say "No mutation was performed." 'Red'
    exit $EXIT_REFUSED
}

function Record {
    param([string]$Name, [string]$Text)
    [void]$script:Evidence.Add("[==== $Name ====]`n$Text`n")
    if ($script:EvidenceDir -and (Test-Path $script:EvidenceDir)) {
        Set-Content -Path (Join-Path $script:EvidenceDir $Name) -Value $Text -Encoding UTF8
    }
}

# --- native invocation that never terminates the script on a non-zero exit ---------------
# The drill's central assertion is an EXPECTED failure, so the exit status must be data,
# not control flow. This captures stdout+stderr and the exit code, and restores the
# caller's error preference so a native stderr write cannot abort the run.
function Invoke-Native {
    param(
        [Parameter(Mandatory)][string]$File,
        [Parameter(Mandatory)][string[]]$Arguments
    )
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $rc = 0
    try {
        $raw = & $File @Arguments 2>&1
        # StrictMode makes an unset $LASTEXITCODE a terminating error; a launch failure
        # (wrong path, not executable) leaves it unset, so it is read defensively and a
        # launch failure is reported as a non-zero status rather than aborting the run.
        if (Test-Path Variable:LASTEXITCODE) { $rc = $LASTEXITCODE }
        $text = (($raw | ForEach-Object { $_.ToString() }) -join "`n").Trim()
    } catch {
        $text = "failed to launch '$File': $($_.Exception.Message)"
        $rc = -1
    } finally {
        $ErrorActionPreference = $prev
    }
    [pscustomobject]@{ Output = $text; ExitCode = $rc }
}

# --- mechanical guard: no mutating command may name the production rule ------------------
# Property 8 of 13g.14 ("no command can remove or overwrite sigap-retention-90d") is
# asserted structurally rather than trusted, so a later edit cannot quietly drop it.
function Invoke-Guarded {
    param(
        [Parameter(Mandatory)][string]$File,
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$Purpose
    )
    foreach ($a in $Arguments) {
        if ($a -eq $PROD_RULE) {
            Refuse "a mutating command ($Purpose) names the production rule '$PROD_RULE'"
        }
        if ($a -like "$PROD_PREFIX*") {
            Refuse "a mutating command ($Purpose) names the production prefix '$PROD_PREFIX'"
        }
    }
    Say "  -> $Purpose" 'DarkGray'
    Note ($File + ' ' + ($Arguments -join ' '))
    if ($DryRun) {
        return [pscustomobject]@{ Output = '(dry-run: not executed)'; ExitCode = 0 }
    }
    $r = Invoke-Native -File $File -Arguments $Arguments
    if ($r.Output) { Note $r.Output }
    return $r
}

# =========================================================================================
Say "================================================================" 'Cyan'
Say " Sigap Bucket Lock retention drill  (runbook 13g.14 Part 1)" 'Cyan'
if ($DryRun) { Say " MODE: DRY RUN -- no mutation will be performed" 'Yellow' }
Say "================================================================" 'Cyan'

# --- Guard 7: every key is confined to the disposable prefix -----------------------------
if ($DRILL_PREFIX -ne 'lock-drill') { Refuse "drill prefix is '$DRILL_PREFIX', expected 'lock-drill'" }
if ($DRILL_PREFIX -like 'sigap*')   { Refuse "drill prefix '$DRILL_PREFIX' must never be the archive prefix" }
if ($DRILL_RULE   -eq $PROD_RULE)   { Refuse "drill rule name collides with the production rule" }
if ($DRILL_RULE   -like 'sigap*')   { Refuse "drill rule name '$DRILL_RULE' must never be a sigap name" }

# --- Guard 4: credentials must be supplied explicitly, under drill-specific names ---------
foreach ($v in 'R2_DRILL_ACCOUNT_ID','R2_DRILL_ENDPOINT','R2_DRILL_ACCESS_KEY_ID','R2_DRILL_SECRET_ACCESS_KEY') {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($v))) {
        Refuse "$v is not set. Export the bucket-scoped Object Read & Write token under the drill's own variable names."
    }
}

# --- Guard 5: ambient AWS credentials must not be selectable -----------------------------
$ambient = @(@('AWS_PROFILE','AWS_ACCESS_KEY_ID','AWS_SECRET_ACCESS_KEY','AWS_SESSION_TOKEN',
              'AWS_SHARED_CREDENTIALS_FILE','AWS_CONFIG_FILE','AWS_DEFAULT_PROFILE') |
    Where-Object { -not [string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($_)) })
if ($ambient.Count -gt 0) {
    Refuse ("ambient AWS credential variables are set: " + ($ambient -join ', ') +
            ". The drill must not be able to fall back to them. Clear them in this shell and re-run.")
}

$BUCKET = [Environment]::GetEnvironmentVariable('R2_DRILL_BUCKET')
if ([string]::IsNullOrWhiteSpace($BUCKET)) { $BUCKET = 'sigap-backups' }
if ($BUCKET -ne 'sigap-backups') {
    Refuse "R2_DRILL_BUCKET is '$BUCKET'; the verified production bucket is 'sigap-backups'"
}

# --- Guard 6: the endpoint must genuinely be Cloudflare R2 -------------------------------
$endpoint = $env:R2_DRILL_ENDPOINT
if ($endpoint -notmatch '^https://[a-z0-9]+\.r2\.cloudflarestorage\.com/?$') {
    Refuse "R2_DRILL_ENDPOINT is '$endpoint'; it must be https://<accountid>.r2.cloudflarestorage.com"
}
$endpoint = $endpoint.TrimEnd('/')
if ($endpoint -notmatch [regex]::Escape($env:R2_DRILL_ACCOUNT_ID)) {
    Refuse "the endpoint does not contain R2_DRILL_ACCOUNT_ID -- wrong account or wrong endpoint"
}

$accountId = $env:R2_DRILL_ACCOUNT_ID

# --- resolve the two binaries ------------------------------------------------------------
function Resolve-Bin {
    param([string]$OverrideVar, [string]$Name)
    $ov = [Environment]::GetEnvironmentVariable($OverrideVar)
    if (-not [string]::IsNullOrWhiteSpace($ov)) {
        if (-not (Test-Path $ov)) { Refuse "$OverrideVar points at '$ov', which does not exist" }
        return (Resolve-Path $ov).Path
    }
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $cmd) {
        Refuse "'$Name' was not found on PATH. Set $OverrideVar to its full path."
    }
    return $cmd.Source
}
$aws = Resolve-Bin -OverrideVar 'R2_DRILL_AWS_BIN' -Name 'aws'
$npx = Resolve-Bin -OverrideVar 'R2_DRILL_NPX_BIN' -Name 'npx'

$STAMP = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' +
         (-join ((1..6) | ForEach-Object { '{0:x}' -f (Get-Random -Minimum 0 -Maximum 16) }))
$KEY   = "$DRILL_PREFIX/$STAMP/probe.txt"
$RULE_PREFIX = "$DRILL_PREFIX/$STAMP/"

# --- Guard 9: temporary names cannot collide with production resources -------------------
if (-not $KEY.StartsWith("$DRILL_PREFIX/")) { Refuse "object key '$KEY' escaped the drill prefix" }
if ($RULE_PREFIX -eq $PROD_PREFIX)           { Refuse "rule prefix collides with production" }

$script:EvidenceDir = [Environment]::GetEnvironmentVariable('R2_DRILL_EVIDENCE_DIR')
if ([string]::IsNullOrWhiteSpace($script:EvidenceDir)) {
    $script:EvidenceDir = Join-Path (Get-Location).Path "lock-drill-evidence/$STAMP"
}
if (-not $DryRun) {
    New-Item -ItemType Directory -Force -Path $script:EvidenceDir | Out-Null
}

Say ""
Say "account  : $accountId"
Say "bucket   : $BUCKET"
Say "endpoint : $endpoint"
Say "object   : $KEY"
Say "rule     : $DRILL_RULE  -> prefix '$RULE_PREFIX'  (1 day)"
Say "evidence : $script:EvidenceDir"
Say ""

# Child-process credential environment. AWS_SHARED_CREDENTIALS_FILE / AWS_CONFIG_FILE are
# pointed at an empty file so `~/.aws` can never be consulted, even indirectly.
$emptyCreds = Join-Path ([System.IO.Path]::GetTempPath()) ("r2drill-empty-" + [guid]::NewGuid().ToString('n'))
Set-Content -Path $emptyCreds -Value '' -Encoding ASCII
$env:AWS_ACCESS_KEY_ID         = $env:R2_DRILL_ACCESS_KEY_ID
$env:AWS_SECRET_ACCESS_KEY     = $env:R2_DRILL_SECRET_ACCESS_KEY
$env:AWS_SHARED_CREDENTIALS_FILE = $emptyCreds
$env:AWS_CONFIG_FILE             = $emptyCreds
$env:AWS_DEFAULT_REGION          = 'auto'
Remove-Item Env:\AWS_SESSION_TOKEN -ErrorAction SilentlyContinue
Remove-Item Env:\AWS_PROFILE       -ErrorAction SilentlyContinue

$verdicts = [ordered]@{
    BUCKET_LOCK_CONFIG_VERIFIED             = 'NOT_RUN'
    DISPOSABLE_LOCK_MECHANISM_TEST          = 'NOT_RUN'
    PRODUCTION_RULE_UNCHANGED_AFTER_DRILL   = 'NOT_RUN'
}
$cleanupRequired = $false
$bodyFile = Join-Path ([System.IO.Path]::GetTempPath()) ("r2drill-body-" + [guid]::NewGuid().ToString('n') + '.txt')

function Invoke-LockList {
    $r = Invoke-Native -File $npx -Arguments @('wrangler','r2','bucket','lock','list',$BUCKET)
    if ($r.ExitCode -ne 0) { Refuse "`wrangler r2 bucket lock list` failed (rc=$($r.ExitCode)): $($r.Output)" }
    return $r.Output
}

function Invoke-ListObjects {
    $r = Invoke-Native -File $aws -Arguments @('s3api','list-objects-v2','--bucket',$BUCKET,
        '--prefix',"$DRILL_PREFIX/",'--endpoint-url',$endpoint,'--query','KeyCount','--output','text')
    return $r
}

function Invoke-Cleanup {
    # Fail-closed teardown. Idempotent, and it runs even on the failure paths.
    Say ""
    Say "--- cleanup ---" 'Cyan'
    if ($DryRun) { Note '(dry-run: cleanup skipped)'; return $true }
    $ok = $true

    # Remove the drill rule if it is still present. Never the production rule.
    $list = (Invoke-Native -File $npx -Arguments @('wrangler','r2','bucket','lock','list',$BUCKET)).Output
    if ($list -match [regex]::Escape($DRILL_RULE)) {
        $rm = Invoke-Guarded -File $npx -Purpose "remove the drill rule" `
              -Arguments @('wrangler','r2','bucket','lock','remove',$BUCKET,'--name',$DRILL_RULE)
        if ($rm.ExitCode -ne 0) { Warn "drill-rule removal returned rc=$($rm.ExitCode)"; $ok = $false }
    }

    # Delete the probe object if it survived.
    $del = Invoke-Native -File $aws -Arguments @('s3api','delete-object','--bucket',$BUCKET,
        '--key',$KEY,'--endpoint-url',$endpoint)
    if ($del.ExitCode -ne 0) { Warn "probe-object delete returned rc=$($del.ExitCode): $($del.Output)" }

    # Prove absence of both.
    $list2 = (Invoke-Native -File $npx -Arguments @('wrangler','r2','bucket','lock','list',$BUCKET)).Output
    if ($list2 -match [regex]::Escape($DRILL_RULE)) {
        Fail "the drill rule survived cleanup"; $ok = $false
    } else { Ok "no drill rule remains" }
    Record -Name '07-lock-after-cleanup.txt' -Text $list2

    $objs = Invoke-ListObjects
    $count = ($objs.Output).Trim()
    if ($objs.ExitCode -ne 0) {
        Fail "could not list objects under $DRILL_PREFIX/ (rc=$($objs.ExitCode))"; $ok = $false
    } elseif ($count -eq '0') { Ok "no drill objects remain under $DRILL_PREFIX/" }
    else { Fail "$count drill object(s) remain under $DRILL_PREFIX/"; $ok = $false }
    Record -Name '08-cleanup.txt' -Text "rule_remains=$($list2 -match [regex]::Escape($DRILL_RULE))`nobject_count=$count"

    return $ok
}

$exit = $EXIT_PASS
try {
    # -------------------------------------------------------------------------------------
    # PREFLIGHT
    # -------------------------------------------------------------------------------------
    Say "--- preflight ---" 'Cyan'

    # Guard 1: the wrangler session must be the expected account.
    $who = Invoke-Native -File $npx -Arguments @('wrangler','whoami')
    if ($who.ExitCode -ne 0) { Refuse "`wrangler whoami` failed (rc=$($who.ExitCode)): $($who.Output)" }
    if ($who.Output -notmatch [regex]::Escape($accountId)) {
        Refuse "wrangler is not authenticated to account '$accountId'. Run 'npx wrangler whoami' and compare."
    }
    Ok "wrangler session matches account $accountId"
    Record -Name '00-whoami.txt' -Text $who.Output

    # Guard 2: the production rule must match the established inventory, BEFORE anything.
    $before = Invoke-LockList
    Record -Name '01-lock-before.txt' -Text $before
    if ($before -notmatch [regex]::Escape($PROD_RULE)) {
        Refuse "the production rule '$PROD_RULE' is not present in 'lock list'. Re-verify F14 before drilling."
    }
    if ($before -notmatch [regex]::Escape($PROD_PREFIX)) {
        Refuse "the production rule's prefix '$PROD_PREFIX' was not seen in 'lock list'."
    }
    if ($before -notmatch '(?i)\b9[0-9]+\b\s*day|\b9[0-9]+\b.*retention|retention.*\b9[0-9]+\b') {
        Refuse "could not confirm a >=$PROD_MIN_DAYS-day retention in 'lock list'. Inspect the output above; refusing rather than assuming."
    }
    Ok "production rule '$PROD_RULE' present, prefix '$PROD_PREFIX', >=$PROD_MIN_DAYS days"
    $verdicts['BUCKET_LOCK_CONFIG_VERIFIED'] = 'PASS'

    # Guard 3: no pre-existing drill rule or object may interfere.
    if ($before -match [regex]::Escape($DRILL_RULE)) {
        Refuse "a rule named '$DRILL_RULE' already exists. Remove it before re-running; the drill will not adopt an unknown rule."
    }
    $pre = Invoke-ListObjects
    if ($pre.ExitCode -ne 0) { Refuse "could not list $DRILL_PREFIX/ (rc=$($pre.ExitCode)): $($pre.Output)" }
    if (($pre.Output).Trim() -ne '0') {
        Refuse "$(($pre.Output).Trim()) pre-existing object(s) under $DRILL_PREFIX/. Clear them before drilling."
    }
    Ok "no pre-existing drill rule or object"

    if ($DryRun) {
        Say ""
        Say "--- dry-run plan (nothing below will execute) ---" 'Yellow'
        Note "1. put-object  s3://$BUCKET/$KEY"
        Note "2. lock add    $BUCKET $DRILL_RULE $RULE_PREFIX --retention-days 1 --force"
        Note "3. delete-object $KEY            (expect REFUSAL naming lock/retention)"
        Note "4. lock remove $BUCKET --name $DRILL_RULE"
        Note "5. delete-object $KEY            (expect SUCCESS)"
        Note "6. lock list -> no '$DRILL_RULE'; list $DRILL_PREFIX/ -> KeyCount 0"
        Note "7. diff 01-lock-before.txt against 07-lock-after-cleanup.txt"
        Say ""
        Say "DRY RUN complete. Re-run without -DryRun to execute (owner authorization required)." 'Yellow'
        Remove-Item $emptyCreds -ErrorAction SilentlyContinue
        exit $EXIT_PASS
    }

    # -------------------------------------------------------------------------------------
    # PART 1 -- the mechanism, on the disposable prefix
    # -------------------------------------------------------------------------------------
    Say ""
    Say "--- Part 1: disposable-prefix mechanism ---" 'Cyan'
    $cleanupRequired = $true

    # 1. Upload one throwaway object.
    Set-Content -Path $bodyFile -Value 'lock-drill' -Encoding ASCII -NoNewline
    $put = Invoke-Guarded -File $aws -Purpose "upload the probe object" -Arguments @(
        's3api','put-object','--bucket',$BUCKET,'--key',$KEY,'--body',$bodyFile,'--endpoint-url',$endpoint)
    if ($put.ExitCode -ne 0) { Fail "probe upload failed (rc=$($put.ExitCode)): $($put.Output)"; $exit = $EXIT_FAIL; throw 'upload' }
    Ok "probe object uploaded"
    Record -Name '02-upload.txt' -Text $put.Output

    # 2. Apply a 1-day rule to that prefix ONLY. NAME and PREFIX are POSITIONAL on `add`.
    $add = Invoke-Guarded -File $npx -Purpose "add the 1-day drill rule" -Arguments @(
        'wrangler','r2','bucket','lock','add',$BUCKET,$DRILL_RULE,$RULE_PREFIX,'--retention-days','1','--force')
    if ($add.ExitCode -ne 0) { Fail "adding the drill rule failed (rc=$($add.ExitCode)): $($add.Output)"; $exit = $EXIT_FAIL; throw 'lock-add' }
    Ok "drill rule added: $DRILL_RULE -> $RULE_PREFIX (1 day)"
    Record -Name '03-lock-add.txt' -Text $add.Output

    # 3. The refusal. EXPECTED to be non-zero -- captured, never terminating.
    Say ""
    Say "  attempting deletion (a refusal is the expected result)..." 'DarkGray'
    $del = Invoke-Native -File $aws -Arguments @(
        's3api','delete-object','--bucket',$BUCKET,'--key',$KEY,'--endpoint-url',$endpoint)
    Record -Name '04-delete-refused.txt' -Text "exit=$($del.ExitCode)`n$($del.Output)"
    if ($del.ExitCode -eq 0) {
        Fail "a locked object was deletable -- retention is NOT in force"
        Note "output was: $($del.Output)"
        $verdicts['DISPOSABLE_LOCK_MECHANISM_TEST'] = 'FAIL'
        $exit = $EXIT_FAIL; throw 'not-locked'
    }
    # The refusal must name the retention condition. A bare non-zero exit is ALSO what an
    # expired credential, a wrong bucket or an unreachable endpoint produce -- all three
    # would read as "retention is working" while nothing was tested.
    if ($del.Output -match '(?i)lock|retention|ObjectLock') {
        Ok "deletion refused for the retention reason"
        Note $del.Output
    } else {
        Fail "deletion failed (rc=$($del.ExitCode)) but NOT for a retention reason"
        Note "output was: $($del.Output)"
        Note "An expired credential, a wrong bucket, or an unreachable endpoint also exits non-zero."
        $verdicts['DISPOSABLE_LOCK_MECHANISM_TEST'] = 'FAIL'
        $exit = $EXIT_FAIL; throw 'wrong-reason'
    }

    # 4. Remove the drill rule, then confirm deletion SUCCEEDS. Both directions are
    #    asserted: presence refused the delete, removal permitted it. Without the second
    #    half, a rule that silently never applied looks identical to one that worked.
    $rm = Invoke-Guarded -File $npx -Purpose "remove the drill rule" -Arguments @(
        'wrangler','r2','bucket','lock','remove',$BUCKET,'--name',$DRILL_RULE)
    if ($rm.ExitCode -ne 0) { Fail "removing the drill rule failed (rc=$($rm.ExitCode)): $($rm.Output)"; $exit = $EXIT_FAIL; throw 'lock-remove' }
    Ok "drill rule removed"
    Record -Name '05-lock-remove.txt' -Text $rm.Output

    $del2 = Invoke-Native -File $aws -Arguments @(
        's3api','delete-object','--bucket',$BUCKET,'--key',$KEY,'--endpoint-url',$endpoint)
    Record -Name '06-delete-after-removal.txt' -Text "exit=$($del2.ExitCode)`n$($del2.Output)"
    if ($del2.ExitCode -ne 0) {
        Fail "the object is still not deletable after removing the rule (rc=$($del2.ExitCode))"
        Note "Either the rule was not removed, or the refusal above had an unrelated cause."
        $verdicts['DISPOSABLE_LOCK_MECHANISM_TEST'] = 'FAIL'
        $exit = $EXIT_FAIL; throw 'still-locked'
    }
    Ok "after rule removal the object IS deletable -- the refusal came from the rule"
    Note "RETENTION = ADMINISTRATIVELY_REMOVABLE (behaviour, observed not assumed)"
    $verdicts['DISPOSABLE_LOCK_MECHANISM_TEST'] = 'PASS'

    # 5. Cleanup + the production-rule diff.
    if (-not (Invoke-Cleanup)) { $exit = $EXIT_FAIL }
    $cleanupRequired = $false

    $after = Invoke-LockList
    Record -Name '09-lock-after.txt' -Text $after
    $normBefore = ($before -replace '\s+',' ').Trim()
    $normAfter  = ($after  -replace '\s+',' ').Trim()
    if ($normBefore -eq $normAfter) {
        Ok "production rule unchanged by the drill (before == after)"
        $verdicts['PRODUCTION_RULE_UNCHANGED_AFTER_DRILL'] = 'PASS'
    } else {
        Fail "the drill changed the lock configuration"
        Note "before: $normBefore"
        Note "after : $normAfter"
        $verdicts['PRODUCTION_RULE_UNCHANGED_AFTER_DRILL'] = 'FAIL'
        $exit = $EXIT_FAIL
    }
}
catch {
    if ($_.Exception.Message -notin @('upload','lock-add','not-locked','wrong-reason','lock-remove','still-locked')) {
        Fail "unexpected error: $($_.Exception.Message)"
    }
    $exit = $EXIT_FAIL
}
finally {
    if ($cleanupRequired) {
        try { if (-not (Invoke-Cleanup)) { $exit = $EXIT_FAIL } }
        catch { Fail "cleanup itself failed: $($_.Exception.Message)"; $exit = $EXIT_FAIL }
    }
    Remove-Item $bodyFile  -ErrorAction SilentlyContinue
    Remove-Item $emptyCreds -ErrorAction SilentlyContinue
}

# -----------------------------------------------------------------------------------------
Say ""
Say "==================== VERDICTS ====================" 'Cyan'
foreach ($k in $verdicts.Keys) {
    $v = $verdicts[$k]
    $c = if ($v -eq 'PASS') { 'Green' } elseif ($v -eq 'FAIL') { 'Red' } else { 'Yellow' }
    Say ("{0,-42} = {1}" -f $k, $v) $c
}
Say "=================================================" 'Cyan'
Note "A PASS on DISPOSABLE_LOCK_MECHANISM_TEST proves the mechanism on a throwaway prefix."
Note "It is NOT proof that deletion under the production '$PROD_PREFIX' prefix was exercised."
Note "Record the evidence files out-of-band, alongside Part 2's inventory. Never a credential."

$summary = ($verdicts.Keys | ForEach-Object { "$_ = $($verdicts[$_])" }) -join "`n"
Record -Name '99-summary.txt' -Text $summary

exit $exit
