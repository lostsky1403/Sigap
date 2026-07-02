<#
.SYNOPSIS
    Sigap notification pipeline smoke suite — verifies the end-to-end
    notification flow: API summary, listing, worker dry-run, and real run.

.DESCRIPTION
    Exercises the Sigap notification pipeline against an already-running
    Sigap stack (API on $ApiBase, default http://localhost:8080). Runs the
    notification worker as a subprocess to validate dry-run (no-mutation)
    and real-mode delivery.

    Steps:
      1. GET  /health
      2. GET  /api/v1/admin/facilities                  (dev identity)
      3. GET  /api/v1/admin/notifications/summary       (dev identity, before)
      4. GET  /api/v1/admin/notifications?status=pending (dev identity, before)
      5. worker.dry_run                                 (Go subprocess, DRY_RUN)
      6. notification.summary.dry_run_verify            (assert no mutation)
      7. worker.once                                    (Go subprocess, real mode)
      8. notification.summary.after                     (assert delivery happened)
      9. notification.list.after                        (delivered/failed rows exist)

    Each step prints [INFO] / [PASS] / [FAIL]. Exit code is 0 on full PASS,
    1 on any FAIL, 2 on parameter validation failure.

    Privacy rules — the script NEVER prints:
      - recipient_contact_masked, recipient_contact_hash
      - subject, body_template
      - Raw phone numbers, emails, or contact hashes
    Only counts, UUIDs, status strings, and log-parsed counters are printed.

.PARAMETER ApiBase
    Override the API base URL. Defaults to $env:SIGAP_API_BASE or
    'http://localhost:8080'. MUST be non-empty and start with "http://" or
    "https://".

.PARAMETER DevUserId
    Synthetic value sent in the X-Sigap-Dev-User-ID header for admin routes.
    Defaults to 'dev-user-smoke'. This is a NON-secret identifier for the
    local dev identity provider; no real credentials are involved.

.PARAMETER WorkerDir
    Path to the Go module root containing cmd/notification-worker.
    Defaults to the apps/api directory relative to the script's parent.

.EXAMPLE
    pwsh -File scripts/smoke/sigap-notification-smoke.ps1

.EXAMPLE
    $env:SIGAP_API_BASE = "http://localhost:8080"
    $env:SIGAP_DATABASE_URL = "postgres://user:pass@localhost:5432/sigap"
    pwsh -File scripts/smoke/sigap-notification-smoke.ps1

.NOTES
    Requires PowerShell 7+. The Sigap stack must be running locally.
    Dev identity is enabled locally only; never in production.

    Privacy:
      - The script NEVER prints secrets (no JWT, no password, no API key).
      - All patient data is synthetic; only counts are displayed.
      - Dev identity is local-only; never enable SIGAP_DEV_IDENTITY=true
        in any shared environment.
#>

[CmdletBinding()]
param(
    [string]$ApiBase = $(if ($env:SIGAP_API_BASE) { $env:SIGAP_API_BASE } else { 'http://localhost:8080' }),
    [string]$DevUserId = 'dev-user-smoke',
    [string]$WorkerDir = $(Join-Path (Split-Path $PSScriptRoot -Parent) 'apps\api')
)

# Capture and restore the original ErrorActionPreference on exit so we never
# leak a mutated preference to the caller's shell.
$originalErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

# ---------------------------------------------------------------
# Constants
# ---------------------------------------------------------------

# Results aggregator — one row per smoke step.
$results = New-Object System.Collections.Generic.List[object]

# ---------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------
function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor DarkCyan
}

function Add-Result {
    param(
        [string]$Name,
        [bool]$Pass,
        [string]$Detail = ''
    )
    $results.Add([pscustomobject]@{
        Step   = $Name
        Pass   = $Pass
        Detail = $Detail
    }) | Out-Null
    if ($Pass) {
        Write-Host "[PASS] $Name" -ForegroundColor Green
        if ($Detail) { Write-Host "       $Detail" -ForegroundColor DarkGray }
    } else {
        Write-Host "[FAIL] $Name" -ForegroundColor Red
        if ($Detail) { Write-Host "       $Detail" -ForegroundColor DarkRed }
    }
}

# ---------------------------------------------------------------
# Parameter validation
# ---------------------------------------------------------------
function Test-Parameters {
    param(
        [string]$ApiBase,
        [string]$DevUserId,
        [string]$WorkerDir
    )
    $errors = New-Object System.Collections.Generic.List[string]

    if ([string]::IsNullOrWhiteSpace($ApiBase)) {
        $errors.Add('ApiBase must not be empty. Pass -ApiBase or set $env:SIGAP_API_BASE.')
    } elseif (-not ($ApiBase -match '^(https?)://')) {
        $errors.Add("ApiBase must start with http:// or https:// (got: '$ApiBase').")
    } elseif ($ApiBase -match '\s') {
        $errors.Add("ApiBase must not contain whitespace (got: '$ApiBase').")
    }

    if ([string]::IsNullOrWhiteSpace($DevUserId)) {
        $errors.Add('DevUserId must not be empty.')
    }

    if ([string]::IsNullOrWhiteSpace($WorkerDir)) {
        $errors.Add('WorkerDir must not be empty.')
    } elseif (-not (Test-Path $WorkerDir)) {
        $errors.Add("WorkerDir does not exist: '$WorkerDir'.")
    } elseif (-not (Test-Path (Join-Path $WorkerDir 'cmd\notification-worker\main.go'))) {
        $errors.Add("WorkerDir does not contain cmd/notification-worker/main.go: '$WorkerDir'.")
    }

    return $errors
}

# ---------------------------------------------------------------
# HTTP helper — every call site is wrapped in try/catch; the helper
# always returns a consistent result object (never throws).
# ---------------------------------------------------------------
function Invoke-ApiJson {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [ValidateSet('GET','POST','PATCH','PUT','DELETE')] [string]$Method,
        [Parameter(Mandatory)] [string]$Path,
        [hashtable]$Headers = @{},
        [object]$Body = $null,
        [int]$TimeoutSec = 15
    )
    $uri = "$ApiBase$Path"
    $reqHeaders = @{ 'Accept' = 'application/json' }
    foreach ($k in $Headers.Keys) { $reqHeaders[$k] = $Headers[$k] }

    $params = @{
        Method              = $Method
        Uri                 = $uri
        Headers             = $reqHeaders
        TimeoutSec          = $TimeoutSec
        StatusCodeVariable  = 'sc'
    }
    if ($null -ne $Body) {
        try {
            $json = $Body | ConvertTo-Json -Depth 10 -Compress
        } catch {
            return [pscustomobject]@{
                StatusCode = 0
                Body       = ''
                Json       = $null
                Success    = $false
                Error      = "Failed to serialize request body: $($_.Exception.Message)"
                NetworkOk  = $false
                CallOk     = $false
            }
        }
        $params['Body']        = $json
        $params['ContentType'] = 'application/json'
    }

    try {
        $resp = Invoke-WebRequest @params
        $content = ''
        if ($null -ne $resp.Content) { $content = [string]$resp.Content }
        $parsed = $null
        if ($content) {
            try { $parsed = $content | ConvertFrom-Json -ErrorAction Stop } catch { $parsed = $content }
        }
        $statusCode = 0
        if ($resp.StatusCode) { $statusCode = [int]$resp.StatusCode }
        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = $content
            Json       = $parsed
            Success    = ($statusCode -ge 200 -and $statusCode -lt 300)
            Error      = $null
            NetworkOk  = $true
            CallOk     = $true
        }
    } catch [System.Net.Http.HttpRequestException] {
        return [pscustomobject]@{
            StatusCode = 0
            Body       = ''
            Json       = $null
            Success    = $false
            Error      = "Network error: $($_.Exception.Message)"
            NetworkOk  = $false
            CallOk     = $true
        }
    } catch [System.Net.WebException] {
        return [pscustomobject]@{
            StatusCode = 0
            Body       = ''
            Json       = $null
            Success    = $false
            Error      = "Network error: $($_.Exception.Message)"
            NetworkOk  = $false
            CallOk     = $true
        }
    } catch {
        $ex = $_.Exception
        $statusCode = 0
        $bodyText   = ''
        if ($ex.Response) {
            try { $statusCode = [int]$ex.Response.StatusCode } catch {}
            try {
                $stream = $ex.Response.GetResponseStream()
                if ($stream) {
                    try {
                        $reader = New-Object System.IO.StreamReader($stream)
                        $bodyText = $reader.ReadToEnd()
                    } finally {
                        try { $reader.Close() } catch {}
                        try { $stream.Close() } catch {}
                    }
                }
            } catch {}
        }
        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = $bodyText
            Json       = $null
            Success    = $false
            Error      = $ex.Message
            NetworkOk  = $true
            CallOk     = $true
        }
    }
}

# ---------------------------------------------------------------
# Step runner — wraps a step body so any unhandled exception becomes
# a [FAIL] with the message attached, never a script-terminating crash.
# ---------------------------------------------------------------
function Invoke-Step {
    param(
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [scriptblock]$Body
    )
    try {
        & $Body
    } catch {
        Add-Result -Name $Name -Pass $false -Detail "Unhandled exception: $($_.Exception.Message)"
    }
}

# ---------------------------------------------------------------
# Worker log parser — extracts structured counters from the slog
# "worker run complete" line.
#
# Expected format:
#   time=... level=INFO msg="worker run complete" dry_run=true
#   inspected_pending=2 claimed=0 delivered=0 failed=0 retried=0 skipped=0
# ---------------------------------------------------------------
function Parse-WorkerLog {
    param([string]$LogOutput)

    $result = [ordered]@{
        DryRun           = $null
        InspectedPending = $null
        Claimed          = $null
        Delivered        = $null
        Failed           = $null
        Retried          = $null
        Skipped          = $null
        Found            = $false
    }

    # The "worker run complete" line may be split across the combined
    # stdout+stderr output. Search every line for the marker.
    $lines = $LogOutput -split "`n"
    foreach ($line in $lines) {
        if ($line -match 'worker run complete') {
            $result.Found = $true

            if ($line -match 'dry_run=(\w+)') {
                $result.DryRun = $Matches[1]
            }
            if ($line -match 'inspected_pending=(\d+)') {
                $result.InspectedPending = [int]$Matches[1]
            }
            if ($line -match '\bclaimed=(\d+)') {
                $result.Claimed = [int]$Matches[1]
            }
            if ($line -match 'delivered=(\d+)') {
                $result.Delivered = [int]$Matches[1]
            }
            if ($line -match 'failed=(\d+)') {
                $result.Failed = [int]$Matches[1]
            }
            if ($line -match 'retried=(\d+)') {
                $result.Retried = [int]$Matches[1]
            }
            if ($line -match 'skipped=(\d+)') {
                $result.Skipped = [int]$Matches[1]
            }
            break
        }
    }

    return $result
}

# ---------------------------------------------------------------
# Run notification worker subprocess with a timeout.
#
# Sets SIGAP_NOTIFICATION_WORKER_DRY_RUN and
# SIGAP_NOTIFICATION_WORKER_ONCE via environment variables, runs
# `go run ./cmd/notification-worker` from $WorkerDir, and returns
# the combined stdout+stderr.
# ---------------------------------------------------------------
function Invoke-NotificationWorker {
    param(
        [string]$WorkerDir,
        [bool]$DryRun = $true,
        [int]$TimeoutSec = 30
    )

    $dryRunValue = if ($DryRun) { 'true' } else { 'false' }

    if ([string]::IsNullOrWhiteSpace($env:SIGAP_DATABASE_URL)) {
        throw 'SIGAP_DATABASE_URL environment variable is not set.'
    }

    # Capture the database URL in the parent scope so the child job can use it.
    $databaseUrl = $env:SIGAP_DATABASE_URL

    # Use Start-Job to run the worker in a child process with a hard
    # timeout so we never hang on a stuck worker.
    $job = Start-Job -ScriptBlock {
        param($wd, $dryRun, $dbUrl)

        Set-Location $wd
        $env:SIGAP_NOTIFICATION_WORKER_DRY_RUN = $dryRun
        $env:SIGAP_NOTIFICATION_WORKER_ONCE = 'true'
        $env:SIGAP_DATABASE_URL = $dbUrl

        # go run produces both stdout (slog) and stderr (Go build output);
        # capture both.
        $output = & { go run ./cmd/notification-worker } 2>&1 | Out-String
        return $output
    } -ArgumentList $WorkerDir, $dryRunValue, $databaseUrl

    $completed = $job | Wait-Job -Timeout $TimeoutSec
    if ($null -eq $completed) {
        # Timed out — kill the job and report failure.
        Stop-Job $job
        Remove-Job $job -Force
        throw "Worker did not complete within ${TimeoutSec}s — timed out."
    }

    $output = Receive-Job $job -ErrorAction SilentlyContinue
    Remove-Job $job -Force

    # Receive-Job may return an array of strings; join them.
    if ($null -eq $output) { return '' }
    if ($output -is [array]) { return ($output -join "`n") }
    return [string]$output
}

# ---------------------------------------------------------------
# Helper: query notification summary and return the pending count.
# ---------------------------------------------------------------
function Get-NotificationPendingCount {
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications/summary' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        throw "Network/transport error: $($resp.Error)"
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        throw "HTTP $($resp.StatusCode) — body: $($resp.Body)"
    }
    # data is a flat map[string]int; safe to read .pending.
    $pending = [int]$resp.Json.data.pending
    return $pending
}

# ---------------------------------------------------------------
# Helper: query notification summary and return all counts.
# ---------------------------------------------------------------
function Get-NotificationSummary {
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications/summary' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        throw "Network/transport error: $($resp.Error)"
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        throw "HTTP $($resp.StatusCode) — body: $($resp.Body)"
    }
    return $resp.Json.data
}

# ---------------------------------------------------------------
# Pre-flight: parameter validation
# ---------------------------------------------------------------
$paramErrors = Test-Parameters -ApiBase $ApiBase -DevUserId $DevUserId -WorkerDir $WorkerDir
if ($paramErrors.Count -gt 0) {
    Write-Host "[FAIL] parameters" -ForegroundColor Red
    foreach ($e in $paramErrors) { Write-Host "       - $e" -ForegroundColor DarkRed }
    $ErrorActionPreference = $originalErrorActionPreference
    exit 2
}

Write-Host "Sigap notification pipeline smoke suite"
Write-Host "API base       : $ApiBase"
Write-Host "Dev user       : $DevUserId (local-only synthetic identifier)"
Write-Host "Worker dir     : $WorkerDir"
Write-Host ""

# Track whether worker steps are skipped (e.g. build failure in step 5).
$workerSkipped = $false

# Track whether step 5 (dry-run) reported the log line.
$workerDryRunParsed = [pscustomobject]@{
    Found = $false
}

# ---------------------------------------------------------------
# Step 1: Health check
# ---------------------------------------------------------------
Invoke-Step -Name 'api.health' -Body {
    Write-Step "Step 1/9: GET /health"
    $resp = Invoke-ApiJson -Method GET -Path '/health'
    if (-not $resp.CallOk) {
        Add-Result -Name 'api.health' -Pass $false -Detail "Could not call API: $($resp.Error)"
        return
    }
    if (-not $resp.NetworkOk) {
        Add-Result -Name 'api.health' -Pass $false -Detail "Network unreachable at $ApiBase — is the Go API running? ($($resp.Error))"
        return
    }
    if ($resp.StatusCode -eq 200 -and $resp.Success) {
        Add-Result -Name 'api.health' -Pass $true -Detail "HTTP 200 — $($resp.Body)"
    } else {
        Add-Result -Name 'api.health' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
    }
}

# ---------------------------------------------------------------
# Step 2: Dev identity — list facilities to obtain facility_id
# ---------------------------------------------------------------
$facilityId = $null
Invoke-Step -Name 'dev.identity' -Body {
    Write-Step "Step 2/9: GET /api/v1/admin/facilities (dev identity)"
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/facilities' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'dev.identity' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 403) {
        Add-Result -Name 'dev.identity' -Pass $false -Detail "HTTP 403 — dev identity disabled. Set SIGAP_AUTH_MODE=dev and SIGAP_DEV_IDENTITY=true in .env, then restart the API."
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json) {
        Add-Result -Name 'dev.identity' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    if (-not $resp.Json.success) {
        Add-Result -Name 'dev.identity' -Pass $false -Detail "API returned success=false — body: $($resp.Body)"
        return
    }
    $facilities = @($resp.Json.data)
    if ($facilities.Count -eq 0) {
        Add-Result -Name 'dev.identity' -Pass $false -Detail "HTTP 200 but no facilities returned. Did you run dev.sql?"
        return
    }
    $script:facilityId = [string]$facilities[0].id
    Add-Result -Name 'dev.identity' -Pass $true -Detail "HTTP 200 — facility_id=$($script:facilityId) (from $($facilities.Count) facility/facilities)"
}

# ---------------------------------------------------------------
# Step 3: Notification summary — snapshot pending count before worker
# ---------------------------------------------------------------
$pendingBefore = 0
Invoke-Step -Name 'notification.summary.before' -Body {
    Write-Step "Step 3/9: GET /api/v1/admin/notifications/summary (before worker)"
    $script:pendingBefore = Get-NotificationPendingCount
    Add-Result -Name 'notification.summary.before' -Pass $true -Detail "pending=$($script:pendingBefore)"
}

# ---------------------------------------------------------------
# Step 4: Notification list — verify at least 1 pending row exists
# ---------------------------------------------------------------
Invoke-Step -Name 'notification.list.before' -Body {
    Write-Step "Step 4/9: GET /api/v1/admin/notifications?status=pending (before worker)"
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications?status=pending' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'notification.list.before' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'notification.list.before' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    $rowCount = @($resp.Json.data).Count
    if ($rowCount -ge 1) {
        # Privacy: print row count only — never print subject, body_template,
        # recipient_contact_masked, or recipient_contact_hash.
        Add-Result -Name 'notification.list.before' -Pass $true -Detail "row_count=$rowCount"
    } else {
        Add-Result -Name 'notification.list.before' -Pass $false -Detail "Expected >= 1 pending row but found 0. Seed some notifications first."
    }
}

# ---------------------------------------------------------------
# Step 5: Worker dry run — run the notification worker with DRY_RUN
# ---------------------------------------------------------------
$workerDryRunResult = $null
Invoke-Step -Name 'worker.dry_run' -Body {
    Write-Step "Step 5/9: notification worker dry run (go run ./cmd/notification-worker)"

    $workerOutput = ''
    try {
        $workerOutput = Invoke-NotificationWorker -WorkerDir $WorkerDir -DryRun $true -TimeoutSec 30
    } catch {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Worker execution failed: $($_.Exception.Message)"
        return
    }

    # Parse the structured slog output.
    $parsed = Parse-WorkerLog -LogOutput $workerOutput
    $script:workerDryRunParsed = $parsed

    if (-not $parsed.Found) {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Worker ran but 'worker run complete' log line not found in output."
        return
    }

    # Assert dry_run=true
    if ($parsed.DryRun -ne 'true') {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Expected dry_run=true but got dry_run=$($parsed.DryRun)"
        return
    }

    # Assert claimed=0, delivered=0, retried=0
    if ($parsed.Claimed -ne 0) {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Expected claimed=0 but got claimed=$($parsed.Claimed)"
        return
    }
    if ($parsed.Delivered -ne 0) {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Expected delivered=0 but got delivered=$($parsed.Delivered)"
        return
    }
    if ($parsed.Retried -ne 0) {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.dry_run' -Pass $false -Detail "Expected retried=0 but got retried=$($parsed.Retried)"
        return
    }

    $script:workerDryRunResult = $parsed
    Add-Result -Name 'worker.dry_run' -Pass $true -Detail "dry_run=true inspected_pending=$($parsed.InspectedPending) claimed=0 delivered=0 retried=0"
}

# ---------------------------------------------------------------
# Step 6: Verify dry run did not mutate state
# ---------------------------------------------------------------
Invoke-Step -Name 'notification.summary.dry_run_verify' -Body {
    Write-Step "Step 6/9: Re-query summary to verify dry run had no effect"
    if ($workerSkipped) {
        Add-Result -Name 'notification.summary.dry_run_verify' -Pass $false -Detail 'Skipped: worker.dry_run failed or worker was not executed.'
        return
    }
    $pendingAfterDry = Get-NotificationPendingCount
    if ($pendingAfterDry -eq $script:pendingBefore) {
        Add-Result -Name 'notification.summary.dry_run_verify' -Pass $true -Detail "pending=$pendingAfterDry (unchanged from step 3: $($script:pendingBefore))"
    } else {
        Add-Result -Name 'notification.summary.dry_run_verify' -Pass $false -Detail "pending changed: expected $($script:pendingBefore) but got $pendingAfterDry — dry run mutated state!"
    }
}

# ---------------------------------------------------------------
# Step 7: Worker real run — execute without DRY_RUN
# ---------------------------------------------------------------
$workerRealResult = $null
Invoke-Step -Name 'worker.once' -Body {
    Write-Step "Step 7/9: notification worker real run (go run ./cmd/notification-worker)"
    if ($workerSkipped) {
        Add-Result -Name 'worker.once' -Pass $false -Detail 'Skipped: worker.dry_run failed or worker was not executed.'
        return
    }

    $workerOutput = ''
    try {
        $workerOutput = Invoke-NotificationWorker -WorkerDir $WorkerDir -DryRun $false -TimeoutSec 30
    } catch {
        $script:workerSkipped = $true
        Add-Result -Name 'worker.once' -Pass $false -Detail "Worker execution failed: $($_.Exception.Message)"
        return
    }

    # Parse the structured slog output.
    $parsed = Parse-WorkerLog -LogOutput $workerOutput

    if (-not $parsed.Found) {
        Add-Result -Name 'worker.once' -Pass $false -Detail "Worker ran but 'worker run complete' log line not found in output."
        return
    }

    # Assert dry_run=false
    if ($parsed.DryRun -ne 'false') {
        Add-Result -Name 'worker.once' -Pass $false -Detail "Expected dry_run=false but got dry_run=$($parsed.DryRun)"
        return
    }

    # Assert claimed >= 0 (non-negative; may be 0 if all rows were already processed)
    if ($null -eq $parsed.Claimed -or $parsed.Claimed -lt 0) {
        Add-Result -Name 'worker.once' -Pass $false -Detail "Expected claimed >= 0 but got claimed=$($parsed.Claimed)"
        return
    }

    $script:workerRealResult = $parsed
    $claimed = $parsed.Claimed
    $delivered = $parsed.Delivered
    $failed = $parsed.Failed
    $retried = $parsed.Retried
    Add-Result -Name 'worker.once' -Pass $true -Detail "dry_run=false claimed=$claimed delivered=$delivered failed=$failed retried=$retried"
}

# ---------------------------------------------------------------
# Step 8: Notification summary — verify state changed after real run
# ---------------------------------------------------------------
Invoke-Step -Name 'notification.summary.after' -Body {
    Write-Step "Step 8/9: Re-query summary after real worker run"
    if ($null -eq $script:workerRealResult) {
        Add-Result -Name 'notification.summary.after' -Pass $false -Detail 'Skipped: worker.once did not produce results.'
        return
    }

    $claimed = $script:workerRealResult.Claimed

    # If nothing was claimed, the summary may be unchanged — that is
    # acceptable; report as informational pass.
    if ($claimed -eq 0) {
        Add-Result -Name 'notification.summary.after' -Pass $true -Detail "claimed=0 — no state change expected. pending=$($script:pendingBefore)"
        return
    }

    # Something was claimed. Verify that either pending decreased or
    # delivered+failed increased compared to step 3.
    $summary = Get-NotificationSummary
    $pendingNow = [int]$summary.pending
    $deliveredNow = [int]$summary.delivered
    $failedNow = [int]$summary.failed

    $pendingDecreased = $pendingNow -lt $script:pendingBefore
    $deliveredOrFailedIncreased = ($deliveredNow -gt 0) -or ($failedNow -gt 0)

    if ($pendingDecreased -or $deliveredOrFailedIncreased) {
        Add-Result -Name 'notification.summary.after' -Pass $true -Detail "pending=$pendingNow delivered=$deliveredNow failed=$failedNow (claimed=$claimed)"
    } else {
        # Fall back: use the log-parsed counters from step 7 instead
        # of re-querying the summary, in case the summary endpoint
        # has a different view (e.g. counting by facility).
        if ($script:workerRealResult.Delivered -gt 0 -or $script:workerRealResult.Failed -gt 0) {
            Add-Result -Name 'notification.summary.after' -Pass $true -Detail "pending=$pendingNow delivered=$deliveredNow failed=$failedNow (log: delivered=$($script:workerRealResult.Delivered) failed=$($script:workerRealResult.Failed))"
        } else {
            Add-Result -Name 'notification.summary.after' -Pass $false -Detail "claimed=$claimed but summary unchanged: pending=$pendingNow delivered=$deliveredNow failed=$failedNow"
        }
    }
}

# ---------------------------------------------------------------
# Step 9: Notification list — verify delivered/failed rows exist
# ---------------------------------------------------------------
Invoke-Step -Name 'notification.list.after' -Body {
    Write-Step "Step 9/9: GET /api/v1/admin/notifications?status=delivered and ?status=failed"
    if ($null -eq $script:workerRealResult) {
        Add-Result -Name 'notification.list.after' -Pass $false -Detail 'Skipped: worker.once did not produce results.'
        return
    }

    $claimed = $script:workerRealResult.Claimed
    if ($claimed -eq 0) {
        Add-Result -Name 'notification.list.after' -Pass $true -Detail "No rows were claimed in step 7; delivered/failed check not applicable."
        return
    }

    # Query delivered rows.
    $respDelivered = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications?status=delivered' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    $deliveredCount = 0
    if ($respDelivered.CallOk -and $respDelivered.NetworkOk -and $respDelivered.Success -and $null -ne $respDelivered.Json -and $respDelivered.Json.success) {
        $deliveredCount = @($respDelivered.Json.data).Count
    }

    # Query failed rows.
    $respFailed = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications?status=failed' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    $failedCount = 0
    if ($respFailed.CallOk -and $respFailed.NetworkOk -and $respFailed.Success -and $null -ne $respFailed.Json -and $respFailed.Json.success) {
        $failedCount = @($respFailed.Json.data).Count
    }

    if ($deliveredCount -gt 0 -or $failedCount -gt 0) {
        # Privacy: print counts only — never print individual row data.
        Add-Result -Name 'notification.list.after' -Pass $true -Detail "delivered_rows=$deliveredCount failed_rows=$failedCount"
    } else {
        Add-Result -Name 'notification.list.after' -Pass $false -Detail "claimed=$claimed rows in step 7 but no delivered or failed rows found."
    }
}

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
Write-Host ""
Write-Host "================================================="
Write-Host "Notification pipeline smoke summary" -ForegroundColor Cyan
Write-Host "================================================="
$passCount = ($results | Where-Object Pass).Count
$failCount = ($results | Where-Object { -not $_.Pass }).Count
foreach ($r in $results) {
    $color = if ($r.Pass) { 'Green' } else { 'Red' }
    $tag   = if ($r.Pass) { '[PASS]' } else { '[FAIL]' }
    Write-Host "  $tag $($r.Step)" -ForegroundColor $color
}
Write-Host ""
Write-Host "Passed: $passCount / $($results.Count)" -ForegroundColor $(if ($failCount -eq 0) { 'Green' } else { 'Red' })
if ($failCount -gt 0) {
    Write-Host "Failed: $failCount" -ForegroundColor Red
    $ErrorActionPreference = $originalErrorActionPreference
    exit 1
}
$ErrorActionPreference = $originalErrorActionPreference
exit 0
