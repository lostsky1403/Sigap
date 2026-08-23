<#
.SYNOPSIS
    Sigap local demo smoke suite — verifies the happy path end-to-end.

.DESCRIPTION
    Runs the documented local demo flow against an already-running Sigap
    stack (API on $API_BASE, default http://[::1]:8080).

    Steps:
      1. GET  /health
      2. GET  /api/v1/admin/facilities                  (dev identity)
      3. GET  /api/v1/admin/service-units               (dev identity, discover service_unit_id)
      4. GET  /api/v1/admin/schedules                   (dev identity, discover schedule_id for tomorrow)
      5. POST /api/v1/appointments                      (public booking, uses discovered IDs)
      6. POST /api/v1/appointments/{id}/check-in        (public check-in)
      7. GET  /api/v1/admin/queues?facility_id=...      (dev identity)
      8. PATCH /api/v1/admin/appointments/{id}/status   (dev identity, queued->completed)

    Steps 3–4 discover the service_unit_id and practitioner_schedule_id that
    belong to the selected facility, so the booking payload always contains
    valid cross-references. If discovery finds nothing and $SkipSeed is not
    set, booking proceeds without those fields (the API accepts optional
    capacity validation).

    Each step prints [INFO] / [PASS] / [FAIL]. Exit code is 0 on full PASS,
    non-zero on any FAIL or on a network error / invalid response / null
    response field.

.PARAMETER ApiBase
    Override the API base URL. Defaults to http://[::1]:8080 or $env:SIGAP_API_BASE.
    MUST be non-empty and start with "http://" or "https://".

.PARAMETER FacilityShortCode
    The short_code of the facility to book against. Defaults to 'RSK'.
    MUST be non-empty.

.PARAMETER ServiceUnitCode
    The code of the seeded service unit. Defaults to 'DEMO-UMUM'.
    MUST be non-empty. (Informational — the actual UUID is discovered from
    the admin API; the hardcoded fallback is only used when -SkipSeed is set.)

.PARAMETER PractitionerScheduleId
    Override practitioner schedule id. Normally discovered automatically
    from the admin schedules API (step 4). Only used as a fallback when
    -SkipSeed is set. Defaults to the demo seed UUID
    '00000000-0000-0000-0000-00000000d021'.

.PARAMETER DevUserId
    Synthetic value sent in the X-Sigap-Dev-User-ID header for admin routes.
    Defaults to a deterministic synthetic UUID. This is a NON-secret identifier
    for the local dev identity provider; no real credentials are involved.

.PARAMETER SkipSeed
    Set to $true to skip auto-discovery and use only the hardcoded fallback
    IDs for service_unit_id and practitioner_schedule_id in the booking
    payload (booking still works, but capacity may not be validated).

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
    [string]$ApiBase = $(if ($env:SIGAP_API_BASE) { $env:SIGAP_API_BASE } else { 'http://127.0.0.1:8080' }),
    [string]$FacilityShortCode = 'RSK',
    [string]$ServiceUnitCode = 'DEMO-UMUM',
    [string]$PractitionerScheduleId = '00000000-0000-0000-0000-00000000d021',
    [string]$DevUserId = '00000000-0000-0000-0000-00000000d999',
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
$DemoFacilityId    = '00000000-0000-0000-0000-00000000d000'

# Discovered IDs — populated by steps 3–4 from the admin API.
$script:discoveredServiceUnitId  = $null
$script:discoveredScheduleId     = $null
$script:discoveredScheduleUnitId = $null  # service_unit_id from the matched schedule

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
        Method              = $Method
        Uri                 = $uri
        Headers             = $reqHeaders
        TimeoutSec          = $TimeoutSec
        SkipHttpErrorCheck  = $true
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
Write-Host "Service unit   : $ServiceUnitCode (fallback: $DemoServiceUnitId)"
Write-Host "Schedule id    : $PractitionerScheduleId (fallback; auto-discovered)"
Write-Host "Dev user       : $DevUserId (local-only synthetic identifier)"
Write-Host ""

# ---------------------------------------------------------------
# Step 1: Health check
# ---------------------------------------------------------------
Invoke-Step -Name 'health' -Body {
    Write-Step "Step 1/8: GET /health"
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
    Write-Step "Step 2/8: GET /api/v1/admin/facilities (dev identity)"
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
    # Store facilities for cross-referencing with service units in step 3.
    $script:allFacilities = $facilities
    # Initial selection by short_code; step 3 may override if DEMO-UMUM
    # belongs to a different facility.
    $matched = $facilities | Where-Object { $_.short_code -eq $FacilityShortCode } | Select-Object -First 1
    if (-not $matched) {
        $matched = $facilities[0]
        Write-Info "No facility matched short_code='$FacilityShortCode'; falling back to first facility (id=$($matched.id))."
    }
    $script:facilityId = [string]$matched.id
    Add-Result -Name 'admin.facilities.list' -Pass $true -Detail "HTTP 200 — found $($facilities.Count) facilities; selected id=$($script:facilityId) short_code='$($matched.short_code)' (may be refined in step 3)"
}

# ---------------------------------------------------------------
# Step 3: Discover service_unit_id (dev identity)
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.service-units.discover' -Body {
    Write-Step "Step 3/8: GET /api/v1/admin/service-units (discover service_unit_id)"
    if (-not $facilityId) {
        Add-Result -Name 'admin.service-units.discover' -Pass $false -Detail 'Skipped: no facility id from step 2.'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/service-units' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'admin.service-units.discover' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'admin.service-units.discover' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    $units = @($resp.Json.data)
    if ($units.Count -eq 0) {
        Add-Result -Name 'admin.service-units.discover' -Pass $true -Detail "HTTP 200 — no service units returned; will use fallback."
        return
    }
    # Search for DEMO-UMUM across ALL facilities (not just the one
    # from step 2) so we always find the canonical demo facility (d000)
    # even when step 2 picked a duplicate RSK row.
    # Do NOT fall back to an arbitrary active service unit — booking
    # must always target the known demo service unit to stay deterministic.
    $demoUnit = $units | Where-Object { $_.is_active -eq $true -and $_.code -eq $ServiceUnitCode } | Select-Object -First 1
    if ($demoUnit) {
        $script:discoveredServiceUnitId = [string]$demoUnit.id
        # Prefer the facility that owns DEMO-UMUM — this is the canonical
        # demo facility (d000) and is always the correct target for booking.
        if ($demoUnit.facility_id -ne $facilityId) {
            Write-Info "DEMO-UMUM belongs to facility $($demoUnit.facility_id), overriding step 2 selection ($facilityId)."
            $script:facilityId = [string]$demoUnit.facility_id
        }
        Add-Result -Name 'admin.service-units.discover' -Pass $true -Detail "HTTP 200 — discovered service_unit id=$($script:discoveredServiceUnitId) name='$($demoUnit.name)' for facility=$facilityId"
    } else {
        Add-Result -Name 'admin.service-units.discover' -Pass $true -Detail "HTTP 200 — DEMO-UMUM not found; will use deterministic fallback in step 5."
    }
}

# ---------------------------------------------------------------
# Step 4: Discover schedule_id for tomorrow (dev identity)
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.schedules.discover' -Body {
    Write-Step "Step 4/8: GET /api/v1/admin/schedules (discover schedule_id for tomorrow)"
    if (-not $facilityId) {
        Add-Result -Name 'admin.schedules.discover' -Pass $false -Detail 'Skipped: no facility id from step 2.'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/schedules' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'admin.schedules.discover' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if (-not $resp.Success -or $null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'admin.schedules.discover' -Pass $false -Detail "HTTP $($resp.StatusCode) — body: $($resp.Body)"
        return
    }
    $schedules = @($resp.Json.data)
    if ($schedules.Count -eq 0) {
        Add-Result -Name 'admin.schedules.discover' -Pass $true -Detail "HTTP 200 — no schedules returned; will use fallback."
        return
    }
    $tomorrowStr = (Get-Date).ToUniversalTime().Date.AddDays(1).ToString('yyyy-MM-dd')
    $match = $schedules | Where-Object {
        $_.facility_id -eq $facilityId -and $_.is_active -eq $true -and $_.schedule_date -eq $tomorrowStr
    } | Select-Object -First 1
    if ($match) {
        $script:discoveredScheduleId = [string]$match.id
        if ($match.service_unit_id) {
            $script:discoveredScheduleUnitId = [string]$match.service_unit_id
        }
        Add-Result -Name 'admin.schedules.discover' -Pass $true -Detail "HTTP 200 — discovered schedule id=$($script:discoveredScheduleId) date=$tomorrowStr service_unit=$($script:discoveredScheduleUnitId) for facility=$facilityId"
    } else {
        Add-Result -Name 'admin.schedules.discover' -Pass $true -Detail "HTTP 200 — no active schedule for facility=$facilityId on $tomorrowStr; will use fallback."
    }
}

# ---------------------------------------------------------------
# Step 5: Public booking
# ---------------------------------------------------------------
$apptId = $null
$checkinCode = $null
Invoke-Step -Name 'public.booking' -Body {
    Write-Step "Step 5/8: POST /api/v1/appointments (public booking)"
    if (-not $facilityId) {
        Add-Result -Name 'public.booking' -Pass $false -Detail 'Skipped: no facility id from step 2 (facility list failed or returned empty).'
        return
    }
    # Synthetic phone in the ITU-T reserved-for-testing +62-555-01xx range.
    # Random suffix avoids the per-phone daily rate limit across re-runs.
    $rand = Get-Random -Minimum 1000 -Maximum 9999
    $phone = "+62-555-01$($rand.ToString().Substring(0,3))"
    $patientName = "Pasien Demo $rand"
    $apptTime = (Get-Date).ToUniversalTime().Date.AddDays(1).AddHours(9).ToString('yyyy-MM-ddTHH\:mm\:ssZ')

    # Resolve service_unit_id with strict fallback tiers:
    #   1. Schedule's service_unit_id (consistent with the matched schedule)
    #   2. Independently discovered DEMO-UMUM from step 3
    #   3. Deterministic fallback: d001 with canonical facility d000
    #   4. Fail — never silently book against an arbitrary active unit
    #      from the wrong facility.
    $resolvedServiceUnitId = $null
    if ($script:discoveredScheduleUnitId) {
        # Schedule carries its own service_unit_id — use it for consistency.
        $resolvedServiceUnitId = $script:discoveredScheduleUnitId
        if ($script:discoveredServiceUnitId -and $script:discoveredServiceUnitId -ne $script:discoveredScheduleUnitId) {
            Write-Info "Note: independently discovered service_unit ($($script:discoveredServiceUnitId)) differs from schedule's service_unit ($($script:discoveredScheduleUnitId)); using schedule's unit for consistency."
        }
    } elseif ($script:discoveredServiceUnitId) {
        $resolvedServiceUnitId = $script:discoveredServiceUnitId
    } else {
        # Last resort: deterministic demo service unit (d001) always belongs
        # to the canonical demo facility (d000). Force facility to d000 so
        # the booking never targets an arbitrary active service unit from a
        # duplicate RSK row or the wrong facility.
        $resolvedServiceUnitId = $DemoServiceUnitId
        $script:facilityId = $DemoFacilityId
        Write-Info "Using deterministic service unit $DemoServiceUnitId with canonical facility $DemoFacilityId."
    }

    # Resolve schedule_id: prefer discovered, then fallback when SkipSeed is set.
    $resolvedScheduleId = $null
    if ($script:discoveredScheduleId) {
        $resolvedScheduleId = $script:discoveredScheduleId
    } elseif ($SkipSeed) {
        $resolvedScheduleId = $PractitionerScheduleId
    }

    Write-Info "Synthetic booking payload: facility=$facilityId, service_unit=$resolvedServiceUnitId, schedule=$resolvedScheduleId, time=$apptTime (UTC), phone=$phone (reserved test range)."

    $bookingBody = @{
        facility_id          = $facilityId
        patient_display_name = $patientName
        patient_phone        = $phone
        appointment_time     = $apptTime
    }
    if ($resolvedServiceUnitId) {
        $bookingBody.service_unit_id = $resolvedServiceUnitId
    }
    if ($resolvedScheduleId) {
        $bookingBody.practitioner_schedule_id = $resolvedScheduleId
    }
    $resp = Invoke-ApiJson -Method POST -Path '/api/v1/appointments' -Body $bookingBody
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'public.booking' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 400) {
        $excerpt = if ($resp.Body) { $resp.Body.Substring(0, [Math]::Min(300, $resp.Body.Length)) } else { '(empty body)' }
        Add-Result -Name 'public.booking' -Pass $false -Detail "HTTP 400 — $excerpt"
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
# Step 6: Public check-in
# ---------------------------------------------------------------
$queueTicketId = $null
$queueEngineUnavailable = $false
Invoke-Step -Name 'public.checkin' -Body {
    Write-Step "Step 6/8: POST /api/v1/appointments/{id}/check-in"
    if (-not $apptId -or -not $checkinCode) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail 'Skipped: no appointment id or checkin_code from step 5.'
        return
    }
    $resp = Invoke-ApiJson -Method POST -Path "/api/v1/appointments/$apptId/check-in" -Body @{ checkin_code = $checkinCode }
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 500 -and ($resp.Body -match 'Layanan antrean tidak tersedia' -or $resp.Body -match 'Gagal mengambil nomor antrean')) {
        $script:queueEngineUnavailable = $true
        Add-Result -Name 'public.checkin' -Pass $false -Detail "HTTP 500 — Queue engine unavailable. Start it: cargo run in apps/queue-engine, or set SIGAP_ENGINE_FALLBACK=dev in .env and restart API."
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
# Step 7: Admin queue list
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.queues.list' -Body {
    Write-Step "Step 7/8: GET /api/v1/admin/queues?facility_id={id}"
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
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail "HTTP 200 but zero tickets found for facility (expected ≥1 after step 6 check-in)."
    }
}

# ---------------------------------------------------------------
# Step 8: Admin appointment status update (queued -> completed)
# ---------------------------------------------------------------
Invoke-Step -Name 'admin.appointments.status' -Body {
    Write-Step "Step 8/8: PATCH /api/v1/admin/appointments/{id}/status"
    if ($queueEngineUnavailable) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail 'Skipped: queue engine was unavailable at check-in; appointment did not reach queued status.'
        return
    }
    if (-not $apptId) {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail 'Skipped: no appointment id from step 5.'
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
