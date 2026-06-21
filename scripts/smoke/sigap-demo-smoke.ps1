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

    Each step prints [PASS] or [FAIL]. Exit code is 0 on full PASS,
    non-zero on any FAIL.

.PARAMETER ApiBase
    Override the API base URL. Defaults to http://localhost:8080 or $env:SIGAP_API_BASE.

.PARAMETER FacilityShortCode
    The short_code of the facility to book against. Defaults to 'f1'.

.PARAMETER ServiceUnitCode
    The code of the seeded service unit. Defaults to 'DEMO-UMUM'.

.PARAMETER PractitionerScheduleId
    Optional seeded schedule id. Defaults to the demo seed UUID
    '00000000-0000-0000-0000-00000000d021'. If not present in the DB,
    booking still succeeds but skips capacity validation.

.PARAMETER SkipSeed
    Set to $true to skip loading the demo seed IDs (booking still works).

.EXAMPLE
    pwsh -File scripts/smoke/sigap-demo-smoke.ps1

.EXAMPLE
    $env:SIGAP_API_BASE = "http://localhost:8080"
    pwsh -File scripts/smoke/sigap-demo-smoke.ps1 -FacilityShortCode 'f2'

.NOTES
    Requires PowerShell 7+. The Sigap stack must be running locally.
    Dev identity is enabled locally only — never in production.
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

$ErrorActionPreference = 'Continue'
$DemoServiceUnitId = '00000000-0000-0000-0000-00000000d001'
$results = New-Object System.Collections.Generic.List[object]

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
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

function Invoke-ApiJson {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        [object]$Body = $null,
        [int]$TimeoutSec = 15
    )
    $uri = "$ApiBase$Path"
    $reqHeaders = @{
        'Accept' = 'application/json'
    }
    foreach ($k in $Headers.Keys) { $reqHeaders[$k] = $Headers[$k] }

    $params = @{
        Method          = $Method
        Uri             = $uri
        Headers         = $reqHeaders
        TimeoutSec      = $TimeoutSec
        StatusCodeVariable = 'sc'
    }
    if ($PSVersionTable.PSVersion.Major -ge 7 -and $Method -ne 'GET') {
        # SkipCertificateCheck not needed for localhost http; keep simple.
    }
    if ($Body -ne $null) {
        $json = $Body | ConvertTo-Json -Depth 10 -Compress
        $params['Body'] = $json
        $params['ContentType'] = 'application/json'
    }
    try {
        $resp = Invoke-WebRequest @params
        $content = $resp.Content
        $parsed = $null
        if ($content) {
            try { $parsed = $content | ConvertFrom-Json -ErrorAction Stop } catch { $parsed = $content }
        }
        return [pscustomobject]@{
            StatusCode = [int]$resp.StatusCode
            Body       = $content
            Json       = $parsed
            Success    = ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 300)
        }
    } catch {
        $ex = $_.Exception
        $statusCode = 0
        $bodyText = ''
        if ($ex.Response) {
            try { $statusCode = [int]$ex.Response.StatusCode } catch {}
            try {
                $stream = $ex.Response.GetResponseStream()
                if ($stream) {
                    $reader = New-Object System.IO.StreamReader($stream)
                    $bodyText = $reader.ReadToEnd()
                    $reader.Close()
                }
            } catch {}
        }
        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = $bodyText
            Json       = $null
            Success    = $false
            Error      = $ex.Message
        }
    }
}

Write-Host "Sigap demo smoke suite"
Write-Host "API base       : $ApiBase"
Write-Host "Facility code  : $FacilityShortCode"
Write-Host "Service unit   : $ServiceUnitCode ($DemoServiceUnitId)"
Write-Host "Schedule id    : $PractitionerScheduleId"

# ---------------------------------------------------------------
# Step 1: Health check
# ---------------------------------------------------------------
Write-Step "Step 1/6: GET /health"
$healthResp = Invoke-ApiJson -Method GET -Path '/health'
if ($healthResp.Success -and $healthResp.StatusCode -eq 200) {
    Add-Result -Name 'health' -Pass $true -Detail "HTTP 200 — $($healthResp.Body)"
} else {
    Add-Result -Name 'health' -Pass $false -Detail "HTTP $($healthResp.StatusCode) — $($healthResp.Body) $($healthResp.Error)"
}

# ---------------------------------------------------------------
# Step 2: Admin facility list (dev identity)
# ---------------------------------------------------------------
Write-Step "Step 2/6: GET /api/v1/admin/facilities (dev identity)"
$facilitiesResp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/facilities' -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
$facilityId = $null
if ($facilitiesResp.Success -and $facilitiesResp.Json -and $facilitiesResp.Json.success) {
    $facilities = @($facilitiesResp.Json.data)
    $facilityId = ($facilities | Where-Object { $_.short_code -eq $FacilityShortCode } | Select-Object -First 1).id
    if (-not $facilityId -and $facilities.Count -gt 0) {
        # Fallback: first facility
        $facilityId = $facilities[0].id
        Add-Result -Name 'admin.facilities.list' -Pass $true -Detail "HTTP 200 — found $($facilities.Count) facilities; using first (id=$facilityId)"
    } elseif ($facilityId) {
        Add-Result -Name 'admin.facilities.list' -Pass $true -Detail "HTTP 200 — found $($facilities.Count) facilities; matched short_code='$FacilityShortCode'"
    } else {
        Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "HTTP 200 but no facilities returned"
    }
} else {
    Add-Result -Name 'admin.facilities.list' -Pass $false -Detail "HTTP $($facilitiesResp.StatusCode) — $($facilitiesResp.Body)"
}

# ---------------------------------------------------------------
# Step 3: Public booking
# ---------------------------------------------------------------
Write-Step "Step 3/6: POST /api/v1/appointments (public booking)"
$apptId = $null
$checkinCode = $null
if ($facilityId) {
    # Build a unique fake phone per run to avoid daily rate limit collisions.
    $rand = Get-Random -Minimum 1000 -Maximum 9999
    $phone = "+62-555-01$($rand.ToString().Substring(0,3))"   # reserved test range
    $patientName = "Pasien Demo $rand"
    $apptTime = (Get-Date).ToUniversalTime().Date.AddDays(1).AddHours(9).ToString('yyyy-MM-ddTHH:mm:ssZ')

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
    $bookingResp = Invoke-ApiJson -Method POST -Path '/api/v1/appointments' -Body $bookingBody
    if ($bookingResp.Success -and $bookingResp.Json -and $bookingResp.Json.success) {
        $apptId = $bookingResp.Json.data.id
        $checkinCode = $bookingResp.Json.data.checkin_code
        Add-Result -Name 'public.booking' -Pass $true -Detail "HTTP 200 — appointment=$apptId code=$checkinCode time=$apptTime"
    } else {
        Add-Result -Name 'public.booking' -Pass $false -Detail "HTTP $($bookingResp.StatusCode) — $($bookingResp.Body) $($bookingResp.Error)"
    }
} else {
    Add-Result -Name 'public.booking' -Pass $false -Detail 'Skipped: no facility id from step 2'
}

# ---------------------------------------------------------------
# Step 4: Public check-in
# ---------------------------------------------------------------
Write-Step "Step 4/6: POST /api/v1/appointments/{id}/check-in"
$queueTicketId = $null
if ($apptId -and $checkinCode) {
    $checkinResp = Invoke-ApiJson -Method POST -Path "/api/v1/appointments/$apptId/check-in" -Body @{ checkin_code = $checkinCode }
    if ($checkinResp.Success -and $checkinResp.Json -and $checkinResp.Json.success) {
        $queueTicketId = $checkinResp.Json.data.queue_ticket_id
        Add-Result -Name 'public.checkin' -Pass $true -Detail "HTTP 200 — queue_ticket=$queueTicketId number=$($checkinResp.Json.data.formatted_number)"
    } else {
        Add-Result -Name 'public.checkin' -Pass $false -Detail "HTTP $($checkinResp.StatusCode) — $($checkinResp.Body) $($checkinResp.Error)"
    }
} else {
    Add-Result -Name 'public.checkin' -Pass $false -Detail 'Skipped: no appointment id from step 3'
}

# ---------------------------------------------------------------
# Step 5: Admin queue list
# ---------------------------------------------------------------
Write-Step "Step 5/6: GET /api/v1/admin/queues?facility_id={id}"
if ($facilityId) {
    $queuesResp = Invoke-ApiJson -Method GET -Path "/api/v1/admin/queues?facility_id=$facilityId" -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId }
    if ($queuesResp.Success -and $queuesResp.Json -and $queuesResp.Json.success) {
        $ticketCount = @($queuesResp.Json.data).Count
        if ($ticketCount -gt 0) {
            Add-Result -Name 'admin.queues.list' -Pass $true -Detail "HTTP 200 — found $ticketCount ticket(s) for facility"
        } else {
            Add-Result -Name 'admin.queues.list' -Pass $false -Detail "HTTP 200 but zero tickets found"
        }
    } else {
        Add-Result -Name 'admin.queues.list' -Pass $false -Detail "HTTP $($queuesResp.StatusCode) — $($queuesResp.Body)"
    }
} else {
    Add-Result -Name 'admin.queues.list' -Pass $false -Detail 'Skipped: no facility id from step 2'
}

# ---------------------------------------------------------------
# Step 6: Admin appointment status update (queued -> completed)
# ---------------------------------------------------------------
Write-Step "Step 6/6: PATCH /api/v1/admin/appointments/{id}/status"
if ($apptId) {
    $statusResp = Invoke-ApiJson -Method PATCH -Path "/api/v1/admin/appointments/$apptId/status" -Headers @{ 'X-Sigap-Dev-User-ID' = $DevUserId } -Body @{ status = 'completed' }
    if ($statusResp.Success -and $statusResp.Json -and $statusResp.Json.success) {
        Add-Result -Name 'admin.appointments.status' -Pass $true -Detail "HTTP 200 — appointment $apptId -> completed"
    } else {
        Add-Result -Name 'admin.appointments.status' -Pass $false -Detail "HTTP $($statusResp.StatusCode) — $($statusResp.Body)"
    }
} else {
    Add-Result -Name 'admin.appointments.status' -Pass $false -Detail 'Skipped: no appointment id from step 3'
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
    $tag = if ($r.Pass) { '[PASS]' } else { '[FAIL]' }
    Write-Host "  $tag $($r.Step)" -ForegroundColor $color
}
Write-Host ""
Write-Host "Passed: $passCount / $($results.Count)" -ForegroundColor $(if ($failCount -eq 0) { 'Green' } else { 'Red' })
if ($failCount -gt 0) {
    Write-Host "Failed: $failCount" -ForegroundColor Red
    exit 1
}
exit 0
