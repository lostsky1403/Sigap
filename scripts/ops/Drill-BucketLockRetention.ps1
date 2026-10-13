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
      3  INTERNAL ERROR in the drill itself -- not a bucket result, and no
         conclusion about retention may be drawn from it
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
$EXIT_INTERNAL = 3

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
        if ($a -ceq $PROD_RULE) {
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

# --- lock-list parsing: one rule record at a time ----------------------------------------
# Wrangler prints each rule as a block of `key: value` lines, delimited by its `name:` line.
# The earlier preflight matched NAME, PREFIX and RETENTION independently against the WHOLE
# output, which cannot tell which rule a value came from: a second rule could supply the
# retention while the production rule supplied only its name. These functions parse the
# output into records first, so every attribute is read from the same rule.
function Get-LockRuleRecords {
    param([string]$Text)
    $records = New-Object System.Collections.ArrayList
    $cur = $null
    foreach ($line in ($Text -split "`r?`n")) {
        # A `key: value` line only. Anything else (headers, blank lines) is not a field, so
        # a stray colon in a human-readable line cannot invent a record.
        $m = [regex]::Match($line, '^\s*([A-Za-z][A-Za-z0-9_-]*)\s*:\s*(.*)$')
        if (-not $m.Success) { continue }
        $key = $m.Groups[1].Value.ToLowerInvariant()
        $val = $m.Groups[2].Value.Trim()
        if ($key -eq 'name') {
            $cur = @{}
            [void]$records.Add($cur)
        }
        if ($null -eq $cur) { continue }   # a field before any `name:` belongs to no rule
        if ($cur.ContainsKey($key)) { $cur[$key] = @($cur[$key]) + @($val) }
        else                        { $cur[$key] = @($val) }
    }
    # Returned without the `,` prefix: PowerShell enumerates the ArrayList into the
    # pipeline, and the caller's @() normalises it to an array. Using `return ,$records`
    # here AND @() at the call site double-wraps, so the loop variable becomes the
    # ArrayList itself and `$rec['name']` tries to index it by string.
    return $records
}

# Reads exactly one value for a key, or refuses. A field that appears twice with different
# values is contradictory and must not be resolved by taking the first one.
function Get-SingleRuleField {
    param($Record, [string]$Key, [string]$RuleName)
    if (-not $Record.ContainsKey($Key)) { return $null }
    $vals = @($Record[$Key])
    if ($vals.Count -gt 1) {
        $distinct = @($vals | Select-Object -Unique)
        if ($distinct.Count -gt 1) {
            Refuse "rule '$RuleName' has $($vals.Count) '$Key' fields with differing values ($($distinct -join ' / ')) -- contradictory record"
        }
    }
    return $vals[0]
}

# Validates the production rule as ONE record: NAME, PREFIX, ENABLED and RETENTION all read
# from the same rule, with no attribute able to be satisfied by a different one.
function Assert-ProductionRule {
    param([string]$Text)

    $records = @(Get-LockRuleRecords -Text $Text)

    # A record with no name cannot be attributed to any rule, so it is refused rather than
    # silently ignored -- ignoring it could hide the production rule inside a malformed block.
    foreach ($rec in $records) {
        if ([string]::IsNullOrWhiteSpace(@($rec['name'])[0])) {
            Refuse "a rule record in 'lock list' has an empty 'name' field -- malformed output, refusing rather than guessing which rule it is"
        }
    }

    $prod = @($records | Where-Object {
        $_['name'] -and (@($_['name'])[0] -ceq $PROD_RULE)
    })

    if ($prod.Count -eq 0) {
        Refuse "the production rule '$PROD_RULE' is not present in 'lock list'. Re-verify the recorded inventory before drilling."
    }
    if ($prod.Count -gt 1) {
        Refuse "found $($prod.Count) rules named '$PROD_RULE' in 'lock list' -- the inventory is ambiguous, so no single rule can be validated"
    }
    $r = $prod[0]

    # NAME -- already exact-matched above, but a record whose name is empty is malformed.
    $name = Get-SingleRuleField -Record $r -Key 'name' -RuleName $PROD_RULE
    if ([string]::IsNullOrWhiteSpace($name)) {
        Refuse "the production rule record has an empty 'name' field -- malformed record"
    }

    # PREFIX -- must be exactly the recorded scope.
    $prefix = Get-SingleRuleField -Record $r -Key 'prefix' -RuleName $PROD_RULE
    if ($null -eq $prefix) {
        Refuse "the production rule '$PROD_RULE' has no 'prefix' field -- cannot confirm it is scoped to '$PROD_PREFIX'"
    }
    if ($prefix -ne $PROD_PREFIX) {
        Refuse "the production rule's prefix is '$prefix', not the recorded '$PROD_PREFIX' -- the archive may be uncovered, or covered differently than recorded"
    }

    # ENABLED -- a rule that exists but is disabled enforces nothing. This is the defect the
    # name/prefix/retention-only preflight could not catch.
    $enabled = Get-SingleRuleField -Record $r -Key 'enabled' -RuleName $PROD_RULE
    if ($null -eq $enabled) {
        Refuse "the production rule '$PROD_RULE' has no 'enabled' field -- a rule whose enabled state is unknown cannot be treated as enforcing"
    }
    if ($enabled -notmatch '(?i)^(yes|true)$') {
        Refuse "the production rule '$PROD_RULE' is NOT enabled (enabled: '$enabled') -- a disabled rule enforces nothing, so the drill would prove nothing about the archive"
    }

    # RETENTION -- a numeric comparison, not a shape match. Exactly one retention field.
    $retKeys = @(@('condition','retention','retention-days') | Where-Object { $r.ContainsKey($_) })
    if ($retKeys.Count -eq 0) {
        Refuse "the production rule '$PROD_RULE' has no retention field ('condition'/'retention') -- cannot confirm >= $PROD_MIN_DAYS days"
    }
    if ($retKeys.Count -gt 1) {
        Refuse "the production rule '$PROD_RULE' has multiple retention fields ($($retKeys -join ', ')) -- ambiguous record"
    }
    $retKey = $retKeys[0]
    $cond = Get-SingleRuleField -Record $r -Key $retKey -RuleName $PROD_RULE
    if ($cond -match '(?i)indefinite') {
        Refuse "the production rule's retention is 'indefinite', which is not the recorded >= $PROD_MIN_DAYS-day duration -- reconcile the inventory before drilling"
    }
    # Accepted forms: `after N day(s)` and `N day(s)`. Anything else is refused rather than
    # guessed at -- a date-bounded or unrecognised form would otherwise slip through.
    #
    # The match is captured explicitly rather than read from $Matches: $Matches is set by
    # whatever regex ran last in scope, and the record parser matches on every line, so
    # reading it here picked up the parser's capture ('name') instead of the duration.
    $retMatch = [regex]::Match($cond, '(?i)^(?:after\s+)?(\d+)\s+days?$')
    if (-not $retMatch.Success) {
        Refuse "could not parse the production rule's retention '$cond' -- expected 'after N days' or 'N days', and an unrecognised form is refused rather than assumed"
    }
    $days = [int]$retMatch.Groups[1].Value
    if ($days -lt $PROD_MIN_DAYS) {
        Refuse "the production rule's retention is $days days, below the required $PROD_MIN_DAYS -- this is the failure the check exists to catch"
    }

    return [pscustomobject]@{ Days = $days; Prefix = $prefix; Enabled = $enabled; Condition = $cond }
}

# A rule named like the drill's must not already exist. Read from the parsed records so a
# prefix that merely CONTAINS the name cannot be mistaken for a rule.
function Test-DrillRulePresent {
    param([string]$Text)
    $records = @(Get-LockRuleRecords -Text $Text)
    return @($records | Where-Object {
        $_['name'] -and (@($_['name'])[0] -ceq $DRILL_RULE)
    }).Count -gt 0
}

# =========================================================================================
Say "================================================================" 'Cyan'
Say " Sigap Bucket Lock retention drill  (runbook 13g.14 Part 1)" 'Cyan'
if ($DryRun) { Say " MODE: DRY RUN -- no mutation will be performed" 'Yellow' }
Say "================================================================" 'Cyan'

# --- Guard 7: every key is confined to the disposable prefix -----------------------------
if ($DRILL_PREFIX -ne 'lock-drill') { Refuse "drill prefix is '$DRILL_PREFIX', expected 'lock-drill'" }
if ($DRILL_PREFIX -like 'sigap*')   { Refuse "drill prefix '$DRILL_PREFIX' must never be the archive prefix" }
if ($DRILL_RULE   -ceq $PROD_RULE)   { Refuse "drill rule name collides with the production rule" }
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

# --- Guard 6: the endpoint must be EXACTLY this account's Cloudflare R2 host -------------
# A substring test accepts `<id>.evil.example`, `<id>.r2.cloudflarestorage.com.attacker.tld`
# and a longer id that merely contains this one. The host must be the account id and nothing
# else, so the comparison is on the parsed host with an exact, case-sensitive equality.
$accountId = $env:R2_DRILL_ACCOUNT_ID
# -cnotmatch, not -notmatch: PowerShell's -match is case-INSENSITIVE by default, so
# `[0-9a-f]{32}` would accept an uppercase id and the strict host comparison below would
# then reject it for the wrong reason. Cloudflare account ids are lowercase hex.
if ($accountId -cnotmatch '^[0-9a-f]{32}$') {
    Refuse "R2_DRILL_ACCOUNT_ID is not a 32-character lowercase hex Cloudflare account id (got '$accountId')"
}
$endpoint = $env:R2_DRILL_ENDPOINT.Trim()
$epMatch = [regex]::Match($endpoint, '^https://([^/]+)/?$')
if (-not $epMatch.Success) {
    Refuse "R2_DRILL_ENDPOINT is '$endpoint'; it must be a bare https:// URL with no path, e.g. https://<accountid>.r2.cloudflarestorage.com"
}
# Captured from the explicit Match, not from $Matches: the automatic variable is set by
# whichever regex ran last, so reading it here is fragile in exactly the way that already
# produced one defect in this file.
$endpointHost = $epMatch.Groups[1].Value
$expectedHost = "$accountId.r2.cloudflarestorage.com"
if ($endpointHost -cne $expectedHost) {
    Refuse "R2_DRILL_ENDPOINT host is '$endpointHost'; it must be exactly '$expectedHost' for account $accountId -- an extra label, a different domain, or a longer id that merely contains the account id is refused"
}
$endpoint = "https://$expectedHost"

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

# --- listing the drill prefix without trusting KeyCount -----------------------------------
# `--query KeyCount --output text` renders a MISSING key as the literal string `None`, which
# is not `0`. On a real empty prefix R2 omits KeyCount entirely, so the old form refused a
# correct bucket with "None pre-existing object(s)". The count is now taken from the
# response's `Contents` -- the actual keys -- and every uncertain case is reported rather
# than read as "empty".
#
# Returns a result object rather than calling Refuse directly, because it is also used from
# the teardown path, where a refusal must be a drill FAILURE and not a pre-mutation refusal.
function Get-DrillObjectKeys {
    $keys = New-Object System.Collections.ArrayList
    $token = $null
    $page = 0
    $maxPages = 100
    while ($true) {
        $page++
        if ($page -gt $maxPages) {
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "listing $DRILL_PREFIX/ exceeded $maxPages pages -- refusing rather than reporting an incomplete count" }
        }
        # --no-paginate stops the CLI merging pages, so IsTruncated and
        # NextContinuationToken are meaningful and the page boundary is ours to control.
        $awsArgs = @('s3api','list-objects-v2','--bucket',$BUCKET,'--prefix',"$DRILL_PREFIX/",
                     '--endpoint-url',$endpoint,'--output','json','--no-paginate')
        if ($token) { $awsArgs += @('--continuation-token',$token) }
        $r = Invoke-Native -File $aws -Arguments $awsArgs
        if ($r.ExitCode -ne 0) {
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "could not list $DRILL_PREFIX/ (rc=$($r.ExitCode)): $($r.Output)" }
        }

        $text = ($r.Output).Trim()
        # An empty body, or the CLI's literal `None`/`null`, is NOT an empty result. This is
        # the exact value that produced the false refusal, so it is called out by name.
        if ([string]::IsNullOrWhiteSpace($text) -or $text -eq 'None' -or $text -eq 'null') {
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "list-objects-v2 returned no usable output for $DRILL_PREFIX/ ('$text') -- refusing rather than assuming the prefix is empty" }
        }

        $doc = $null
        try { $doc = ConvertFrom-Json -InputObject $text } catch {
            $preview = $text.Substring(0, [Math]::Min(120, $text.Length))
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "list-objects-v2 returned output that is not valid JSON -- refusing rather than assuming the prefix is empty. Output began: $preview" }
        }
        if ($null -eq $doc -or $doc -is [string] -or $doc -is [array]) {
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "list-objects-v2 returned a JSON value that is not an object -- malformed response, refusing" }
        }
        $names = @(Get-JsonFieldNames -Object $doc)

        # Contents is the authoritative list of keys. Its absence means there are none.
        $pageKeys = @()
        if ($names -contains 'Contents') {
            $contents = $doc.Contents
            if ($null -ne $contents) {
                foreach ($c in @($contents)) {
                    if ($null -eq $c -or -not (Test-JsonHasField -Object $c -Field 'Key')) {
                        return [pscustomobject]@{ Ok = $false; Keys = @()
                            Error = "a Contents entry in list-objects-v2 has no Key field -- malformed response, refusing" }
                    }
                    $k = [string]$c.Key
                    if ([string]::IsNullOrWhiteSpace($k)) {
                        return [pscustomobject]@{ Ok = $false; Keys = @()
                            Error = "a Contents entry in list-objects-v2 has an empty Key -- malformed response, refusing" }
                    }
                    $pageKeys += $k
                }
            }
        }

        # KeyCount is a HINT, not the count -- the keys above are. It is still validated when
        # present, because a malformed or contradictory value means the response cannot be
        # trusted at all. It is deliberately NOT compared for equality against the page size:
        # its exact semantics (page vs total) are not guaranteed, and a false mismatch here
        # would re-break a correct bucket, which is the failure being fixed.
        if ($names -contains 'KeyCount') {
            $kc = $doc.KeyCount
            if ($null -ne $kc) {
                $kcInt = 0
                if (-not [int]::TryParse([string]$kc, [ref]$kcInt) -or $kcInt -lt 0) {
                    return [pscustomobject]@{ Ok = $false; Keys = @()
                        Error = "list-objects-v2 returned a non-numeric KeyCount ('$kc') -- malformed response, refusing" }
                }
                if ($pageKeys.Count -eq 0 -and $kcInt -gt 0) {
                    return [pscustomobject]@{ Ok = $false; Keys = @()
                        Error = "list-objects-v2 reported KeyCount=$kcInt but listed no Contents -- contradictory response, refusing rather than guessing" }
                }
            }
        }

        foreach ($k in $pageKeys) { [void]$keys.Add($k) }

        $truncated = $false
        if ($names -contains 'IsTruncated') { $truncated = [bool]$doc.IsTruncated }
        if (-not $truncated) { break }
        $next = $null
        if ($names -contains 'NextContinuationToken') { $next = [string]$doc.NextContinuationToken }
        if ([string]::IsNullOrWhiteSpace($next)) {
            return [pscustomobject]@{ Ok = $false; Keys = @()
                Error = "list-objects-v2 reported IsTruncated with no NextContinuationToken -- refusing rather than reporting a partial count" }
        }
        $token = $next
    }
    return [pscustomobject]@{ Ok = $true; Keys = @($keys); Error = '' }
}

# Field names of a JSON object, as an array. Explicit iteration rather than
# `$obj.PSObject.Properties.Name`: under Set-StrictMode an object with ZERO properties makes
# that member-enumeration form throw, and an empty JSON object `{}` is exactly that case --
# a valid "no objects" response that must not be reported as an internal error.
function Get-JsonFieldNames {
    param($Object)
    $names = New-Object System.Collections.ArrayList
    if ($null -ne $Object) {
        foreach ($p in $Object.PSObject.Properties) { [void]$names.Add($p.Name) }
    }
    return $names
}

function Test-JsonHasField {
    param($Object, [string]$Field)
    foreach ($n in @(Get-JsonFieldNames -Object $Object)) {
        if ($n -ceq $Field) { return $true }
    }
    return $false
}

# Used from the preflight, where uncertainty must stop the run before any mutation.
function Get-DrillObjectKeysOrRefuse {
    $res = Get-DrillObjectKeys
    if (-not $res.Ok) { Refuse $res.Error }
    return $res.Keys
}

function Invoke-Cleanup {
    # Fail-closed teardown. Idempotent, and it runs even on the failure paths.
    Say ""
    Say "--- cleanup ---" 'Cyan'
    if ($DryRun) { Note '(dry-run: cleanup skipped)'; return $true }
    $ok = $true

    # Remove the drill rule if it is still present. Never the production rule.
    $list = (Invoke-Native -File $npx -Arguments @('wrangler','r2','bucket','lock','list',$BUCKET)).Output
    if (Test-DrillRulePresent -Text $list) {
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
    if (Test-DrillRulePresent -Text $list2) {
        Fail "the drill rule survived cleanup"; $ok = $false
    } else { Ok "no drill rule remains" }
    Record -Name '07-lock-after-cleanup.txt' -Text $list2

    $objs = Get-DrillObjectKeys
    if (-not $objs.Ok) {
        # Teardown cannot prove the prefix is clean, so it reports a drill FAILURE rather
        # than a pre-mutation refusal -- nothing was refused, the drill ran and could not
        # confirm its own cleanup.
        Fail "could not confirm $DRILL_PREFIX/ is empty: $($objs.Error)"
        $ok = $false
        $count = 'unknown'
    } elseif ($objs.Keys.Count -eq 0) {
        Ok "no drill objects remain under $DRILL_PREFIX/"
        $count = '0'
    } else {
        Fail "$($objs.Keys.Count) drill object(s) remain under $DRILL_PREFIX/: $($objs.Keys -join ', ')"
        $ok = $false
        $count = [string]$objs.Keys.Count
    }
    Record -Name '08-cleanup.txt' -Text "rule_remains=$(Test-DrillRulePresent -Text $list2)`nobject_count=$count"

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
    # Validated as ONE record, so NAME, PREFIX, ENABLED and RETENTION all come from the same
    # rule and none can be satisfied by a different one.
    $before = Invoke-LockList
    Record -Name '01-lock-before.txt' -Text $before
    $prod = Assert-ProductionRule -Text $before
    Ok "production rule '$PROD_RULE' present: prefix '$($prod.Prefix)', enabled '$($prod.Enabled)', retention '$($prod.Condition)' (>= $PROD_MIN_DAYS days)"
    $verdicts['BUCKET_LOCK_CONFIG_VERIFIED'] = 'PASS'

    # Guard 3: no pre-existing drill rule or object may interfere.
    if (Test-DrillRulePresent -Text $before) {
        Refuse "a rule named '$DRILL_RULE' already exists. Remove it before re-running; the drill will not adopt an unknown rule."
    }
    $preKeys = @(Get-DrillObjectKeysOrRefuse)
    if ($preKeys.Count -gt 0) {
        Refuse "$($preKeys.Count) pre-existing object(s) under $DRILL_PREFIX/: $($preKeys -join ', '). Clear them before drilling."
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
        Note "6. lock list -> no '$DRILL_RULE'; list $DRILL_PREFIX/ -> no keys"
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
    # A thrown label is an expected failure path: the assertion ran and failed.
    # Anything else is a defect in the DRILL, and must not be reported as a drill
    # failure -- exit 1 means "retention was not proven", which would be a false
    # statement about the bucket when the real cause is a bug in this script.
    $known = @('upload','lock-add','not-locked','wrong-reason','lock-remove','still-locked')
    if ($_.Exception.Message -notin $known) {
        $line = $_.InvocationInfo.ScriptLineNumber
        $code = ''
        if ($_.InvocationInfo.Line) { $code = $_.InvocationInfo.Line.Trim() }
        Say ""
        Fail "INTERNAL ERROR in the drill (not a bucket result): $($_.Exception.Message)"
        Note "at line $line : $code"
        Note "This is a defect in the script, not evidence about the bucket. Do not read it as a retention failure."
        Note "No conclusion may be drawn from this run."
        $exit = $EXIT_INTERNAL
    } else {
        $exit = $EXIT_FAIL
    }
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
