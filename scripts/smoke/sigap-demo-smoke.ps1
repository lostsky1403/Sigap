<#
.SYNOPSIS
    Sigap local demo smoke suite — verifies the happy path end-to-end.

.DESCRIPTION
    Runs the documented local demo flow against an already-running Sigap
    stack (API on $API_BASE, default http://localhost:8080).

    Steps:
      1. GET  /health
      2. GET  /api/v1/admin/facilities                (dev identity)
      3. POST /api/v1/appointments                    (public booking)
      4. POST /api/v1/appointments/{id}/check-in      (public check-in)
      5. GET  /api/v1/admin/queues?facility_id=...    (dev identity)
      6. PATCH /api/v1/admin/appointments/{id}/status (dev identity, queued->completed)

    Each step prints [INFO] / [PASS] / [FAIL]. Exit code is 0 on full PASS,
    non-zero on any FAIL or on a network error / invalid response / null
    response field.

.PARAMETER ApiBase
    Override the API base URL. Defaults to http://localhost:8080 or $env:SIGAP_API_BASE.
    MUST be non-empty and start with "http://" or "https://".

.PARAMETER FacilityShortCode
    The short_code of the facility to book against. Defaults to 'f1'.
    MUST be non-empty.

.PARAMETER ServiceUnitCode
    The code of the seeded service unit. Defaults to 'DEMO-UMUM'.
    MUST be non-empty. (Informational — the actual UUID is hardcoded below.)

.PARAMETER PractitionerScheduleId
    Optional seeded schedule id. Defaults to the demo seed UUID
    '00000000-0000-0000-0000-00000000d021'. If not present in the DB,
    booking still succeeds but skips capacity validation.

.PARAMETER DevUserId
    Synthetic value sent in the X-Sigap-Dev-User-ID header for admin routes.
    Defaults to 'dev-user-smoke'. This is a NON-secret identifier for the
    local dev identity provider; no real credentials are involved.

.PARAMETER SkipSeed
    Set to $true to skip sending practitioner_schedule_id in the booking
    payload (booking still works, but capacity is not validated).

.EXAMPLE
    pwsh -File scripts/smoke/sigap-demo-smoke.ps1

.EXAMPLE
    $env:SIGAP_API_BASE = "http://localhost:8080"
    pwsh -File scripts/smoke/sigap-demo-smoke.ps1 -FacilityShortCode 'f2'

.NOTES
    Requires PowerShell 7+. The Sigap stack must be running locally.
    Dev identity is enabled locally only — never in production.

    Privacy:
      - The script NEVER prints secrets (no JWT, no password, no API key).
      - All patient data is synthetic: random name like "Pasien Demo 4711"
        and a random phone in the ITU-T reserved +62-555-01xx range.
      - Dev identity is local-only; never enable SIGAP_DEV_IDENTITY=true
        in any shared environment.
#>

[CmdletBinding()]
param(
    [string]$ApiBase = $(if ($env:SIGAP_API_BASE) { $env:SIGAP_API_BASE } else { 'http://localhost:8080' }),
    [string]$FacilityShortCode = 'f1',
    [string]$ServiceUnitCode = 'DEMO-UMUM',
    [string]$PractitionerScheduleId = '00000000-0000-0000-0000-00000000d021',
    [string]$DevUserId = 'dev-user-smoke',
    [switch]$SkipSeed
)

# Capture and restore the original ErrorActionPreference on exit so we never
# leak a mutated preference to the caller's shell.
$originalErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

# ---------------------------------------------------------------
# Constants (synthetic identifiers only — never replace with real data)
# ---------------------------------------------------------------
$DemoServiceUnitId = '00000000-0000-0000-0000-00000000d001'

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
        [string]$FacilityShortCode,
        [string]$ServiceUnitCode,
        [string]$PractitionerScheduleId,
        [string]$DevUserId
    )
    $errors = New-Object System.Collections.Generic.List[string]

    if ([string]::IsNullOrWhiteSpace($ApiBase)) {
        $errors.Add('ApiBase must not be empty. Pass -ApiBase or set $env:SIGAP_API_BASE.')
    } elseif (-not ($ApiBase -match '^(https?)://')) {
        $errors.Add("ApiBase must start with http:// or https:// (got: '$ApiBase').")
    } elseif ($ApiBase -match '\s') {
        $errors.Add("ApiBase must not contain whitespace (got: '$ApiBase').")
    }

    if ([string]::IsNullOrWhiteSpace($FacilityShortCode)) {
        $errors.Add('FacilityShortCode must not be empty.')
    }
    if ([string]::IsNullOrWhiteSpace($ServiceUnitCode)) {
        $errors.Add('ServiceUnitCode must not be empty.')
    }
    if ([string]::IsNullOrWhiteSpace($PractitionerScheduleId)) {
        $errors.Add('PractitionerScheduleId must not be empty.')
    } elseif ($PractitionerScheduleId -notmatch '^[0-9a-fA-F-]{36}$') {
        $errors.Add("PractitionerScheduleId must be a UUID (got: '$PractitionerScheduleId').")
    }
    if ([string]::IsNullOrWhiteSpace($DevUserId)) {
        $errors.Add('DevUserId must not be empty.')
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
        Method  = $Method
        Uri     = $uri
        Headers = $reqHeaders
        TimeoutSec = $TimeoutSec
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
# Pre-flight: parameter validation
# ---------------------------------------------------------------
$paramErrors = Test-Parameters -ApiBase $ApiBase -FacilityShortCode $FacilityShortCode -ServiceUnitCode $ServiceUnitCode -PractitionerScheduleId $PractitionerScheduleId -DevUserId $DevUserId
if ($paramErrors.Count -gt 0) {
    Write-Host "[FAIL] parameters" -ForegroundColor Red
    foreach ($e in $paramErrors) { Write-Host "       - $e" -ForegroundColor DarkRed }
    $ErrorActionPreference = $originalErrorActionPreference
    exit 2
}

Write-Host "Sigap demo smoke suite"
Write-Host "API base       : $ApiBase"
Write-Host "Facility code  : $FacilityShortCode"
Write-Host "Service unit   : $ServiceUnitCode ($DemoServiceUnitId)"
Write-Host "Schedule id    : $PractitionerScheduleId"
Write-Host "Dev user       : $DevUserId (local-only synthetic identifier)"
Write-Host ""

# ---------------------------------------------------------------
# Step 1: Health check
# ---------------------------------------------------------------
Invoke-Step -Name 'health' -Body {
    Write-Step "Step 1/6: GET /health"
    $resp = Invoke-ApiJson -Method GET -Path '/health'
    if (-not $resp.CallOk) {
        Add-Result -Name 'health' -Pass $false -Detail "Could not call API: $($resp.Error)"
        return
    }
    if (-not $resp.NetworkOk) {
        Add-Result -Name 'health' -Pass $false -Detail "Network unreachable at $ApiBase — is the Go API running? ($($resp.Error))"
        return
    }
    if ($resp.StatusCode -eq 200 -and $resp.Success) {
        Add-Result -Name 'health' -Pass $true -Detail "HTTP 200 — $($resp.Body)"
    } else {
        Add-Result -Name 'health' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
    }
}

# ---------------------------------------------------------------
# Step 2: Admin facility list (dev identity)
# ---------------------------------------------------------------
$facilityId = $null
Invoke-Step -Name 'admin.facilities.list' -Body {
    Write-Step "Step 2/6: GET /api/v1/admin/facilities (dev identity)"
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/facilities' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 403) {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "HTTP 403 — dev identity disabled. Set SIGAP_AUTH_MODE=dev and SIGAP_DEV_IDENTITY=true in .env, then restart the API."
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json) {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    if (-not $resp.Json.success) {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "API returned success=false — body: $($resp.Body)"
        return
    }
    $facilities = @($resp.Json.data)
    if ($facilities.Count -eq 0) {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "HTTP 200 but no facilities returned. Did you run dev.sql?"
        return
    }
    $script:facilityId = ($facilities | Where-Object { $_.short_code -eq $FacilityShortCode } | Select-Object -First 1).id
    if (-not $script:facilityId) {
        $script:facilityId = $facilities[0].id
        Write-Info "No facility matched short_code='$FacilityShortCode'; falling back to first facility (id=$($script:facilityId))."
        Add-Result -Name 'admin.facilities.list' -Pass $true -Detail "HTTP 200 — found $($facilities.Count) facilities; using first (id=$($script:facilityId))"
    } else {
        Add-Result -Name 'admin.facilities.list' -Pass $true -Detail "HTTP 200 — found $($facilities.Count) facilities; matched short_code='$FacilityShortCode' (id=$($script:facilityId))"
    }
}

# ---------------------------------------------------------------
# Step 3: Public booking
# ---------------------------------------------------------------
$apptId = $null
$checkinCode = $null
Invoke-Step -Name 'public.booking' -Body {
    Write-Step "Step 3/6: POST /api/v1/appointments (public booking)"
    if (-not $facilityId) {
        Add-Result -Name 'public.booking' -Pass $false -Detail 'Skipped: no facility id from step 2 (facility list failed or returned empty).'
        return
    }
    # Synthetic phone in the ITU-T reserved-for-testing +62-555-01xx range.
    # Random suffix avoids the per-phone daily rate limit across re-runs.
    $rand = Get-Random -Minimum 1000 -Maximum 9999
    $phone = "+62-555-01$($rand.ToString().Substring(0,3))"
    $patientName = "Pasien Demo $rand"
    $apptTime = (Get-Date).ToUniversalTime().Date.AddDays(1).AddHours(9).ToString('yyyy-MM-ddTHH:mm:ssZ')
    Write-Info "Synthetic booking payload: facility=$facilityId, service_unit=$DemoServiceUnitId, time=$apptTime (UTC), phone=$phone (reserved test range)."

    $bookingBody = @{
        facility_id          = $facilityId
        service_unit_id      = $DemoServiceUnitId
        patient_display_name = $patientName
        patient_phone        = $phone
        appointment_time     = $apptTime
    }
    if (-not $SkipSeed) {
        $bookingBody.practitioner_schedule_id = $PractitionerScheduleId
    }
    $resp = Invoke-ApiJson -Method POST -Path '/api/v1/appointments' -Body $bookingBody
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'public.booking' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 400 -and $resp.Body -match 'Waktu janji temu harus di masa depan') {
        Add-Result -Name 'public.booking' -Pass $false -Detail "HTTP 400 — appointment_time is in the API's past. Check timezone/clock (see docs/DEMO_FLOW.md § Troubleshooting)."
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'public.booking' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    if ($null -eq $resp.Json.data -or $null -eq $resp.Json.data.id -or $null -eq $resp.Json.data.checkin_code) {
        Add-Result -Name 'public.booking' -Pass $false -Detail "HTTP 200 but response missing id or checkin_code — body: $($resp.Body)"
        return
    }
    $script:apptId      = [string]$resp.Json.data.id
    $script:checkinCode = [string]$resp.Json.data.checkin_code
    Add-Result -Name 'public.booking' -Pass $true -Detail "HTTP 200 — appointment=$($script:apptId) code=$($script:checkinCode) time=$apptTime"
}

# ---------------------------------------------------------------
# Step 4: Public check-in
# ---------------------------------------------------------------
$queueTicketId = $null
Invoke-Step -Name 'public.checkin' -Body {
    Write-Step "Step 4/6: POST /api/v1/appointments/{id}/check-in"
    if (-not $apptId -or -not $checkinCode) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail 'Skipped: no appointment id or checkin_code from step 3.'
        return
    }
    $resp = Invoke-ApiJson -Method POST -Path "/api/v1/appointments/$apptId/check-in" -Body @{ checkin_code = $checkinCode }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 500 -and $resp.Body -match 'Layanan antrean tidak tersedia') {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "HTTP 500 — Rust queue engine is unavailable. Start Terminal 1 (cargo run in apps/queue-engine)."
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    if ($null -eq $resp.Json.data) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "HTTP 200 but response missing data — body: $($resp.Body)"
        return
    }
    $script:queueTicketId = [string]$resp.Json.data.queue_ticket_id
    Add-Result -Name 'public.checkin' -Pass $true -Detail "HTTP 200 — queue_ticket=$($script:queueTicketId) number=$($resp.Json.data.formatted_number)"
}

# ---------------------------------------------------------------
# Step 5: Admin queue list
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.queues.list' -Body {
    Write-Step "Step 5/6: GET /api/v1/admin/queues?facility_id={id}"
    if (-not $facilityId) {
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail 'Skipped: no facility id from step 2.'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path "/api/v1/admin/queues?facility_id=$facilityId" -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    $ticketCount = @($resp.Json.data).Count
    if ($ticketCount -gt 0) {
        Add-Result -Name 'admin.queues.list' -Pass $true -Detail "HTTP 200 — found $ticketCount ticket(s) for facility"
    } else {
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail "HTTP 200 but zero tickets found for facility (expected ≥1 after step 4 check-in)."
    }
}

# ---------------------------------------------------------------
# Step 6: Admin appointment status update (queued -> completed)
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.appointments.status' -Body {
    Write-Step "Step 6/6: PATCH /api/v1/admin/appointments/{id}/status"
    if (-not $apptId) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail 'Skipped: no appointment id from step 3.'
        return
    }
    $resp = Invoke-ApiJson -Method PATCH -Path "/api/v1/admin/appointments/$apptId/status" -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId } -Body @{ status = 'completed' }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    if ($resp.Json.PSObject.Properties['success'] -and -not $resp.Json.success) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail "API returned success=false — body: $($resp.Body)"
        return
    }
    Add-Result -Name 'admin.appointments.status' -Pass $true -Detail "HTTP 200 — appointment $apptId -> completed"
}

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
Write-Host ""
Write-Host "================================================="
Write-Host "Smoke summary" -ForegroundColor Cyan
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
