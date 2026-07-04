<#
.SYNOPSIS
    Sigap patient portal smoke suite — validates the public patient status
    lookup API.

.DESCRIPTION
    Exercises the public patient portal endpoint against a running local
    Sigap stack (API on $ApiBase, default http://[::1]:8080). No auth
    required (public endpoint). No Rust engine required.

    Steps:
      1. GET  /health
      2. GET  /api/v1/patient/status?code=SMOKE01                      (valid lookup)
      3. GET  /api/v1/patient/status?code=%3Cscript%3E                  (invalid code, expect 400)
      4. GET  /api/v1/patient/status?code=ZZZZZXXXXX999                 (unknown code, expect 404)
      5. PII field absence — verify step 2 response does NOT contain
         patient_phone, patient_display_name, patient_id,
         recipient_contact_hash, recipient_contact, notification_body

    Each step prints [INFO] / [PASS] / [FAIL]. Exit code is 0 on full
    PASS, 1 on any FAIL, 2 on parameter validation failure.

    Privacy rules — the script NEVER prints:
      - Patient data, phone numbers, or any PII
      - Raw response bodies that may contain patient information
    Only status codes, field presence flags, and non-sensitive metadata
    are printed.

.PARAMETER ApiBase
    Override the API base URL. Defaults to $env:SIGAP_API_BASE or
    'http://[::1]:8080'. MUST be non-empty and start with "http://" or
    "https://".

.EXAMPLE
    pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1

.EXAMPLE
    $env:SIGAP_API_BASE = "http://[::1]:8080"
    pwsh -File scripts/smoke/sigap-patient-portal-smoke.ps1

.NOTES
    Requires PowerShell 7+. The Sigap stack must be running locally.
    Demo seed must be loaded (SMOKE01 appointment).

    Privacy:
      - The script NEVER prints patient data, phone numbers, or any PII.
      - All PII field checks are boolean presence/absence only.
#>

[CmdletBinding()]
param(
    [string]$ApiBase = $(if ($env:SIGAP_API_BASE) { $env:SIGAP_API_BASE } else { 'http://[::1]:8080' })
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

# Forbidden PII field names — must NOT appear in a public response.
$forbiddenFields = @(
    'patient_phone'
    'patient_display_name'
    'patient_id'
    'recipient_contact_hash'
    'recipient_contact'
    'notification_body'
)

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
        [string]$ApiBase
    )
    $errors = New-Object System.Collections.Generic.List[string]

    if ([string]::IsNullOrWhiteSpace($ApiBase)) {
        $errors.Add('ApiBase must not be empty. Pass -ApiBase or set $env:SIGAP_API_BASE.')
    } elseif (-not ($ApiBase -match '^(https?)://')) {
        $errors.Add("ApiBase must start with http:// or https:// (got: '$ApiBase').")
    } elseif ($ApiBase -match '\s') {
        $errors.Add("ApiBase must not contain whitespace (got: '$ApiBase').")
    }

    return $errors
}

# ---------------------------------------------------------------
# HTTP helper — every call site is wrapped in try/catch; the helper
# always returns a consistent result object (never throws).
# Uses Invoke-WebRequest directly — no dev identity headers needed
# since this targets a public endpoint.
# ---------------------------------------------------------------
function Invoke-ApiJson {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [ValidateSet('GET','POST','PATCH','PUT','DELETE')] [string]$Method,
        [Parameter(Mandatory)] [string]$Path,
        [int]$TimeoutSec = 15
    )
    $uri = "$ApiBase$Path"
    $reqHeaders = @{ 'Accept' = 'application/json' }

    $params = @{
        Method             = $Method
        Uri                = $uri
        Headers            = $reqHeaders
        TimeoutSec         = $TimeoutSec
        SkipHttpErrorCheck = $true
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
        $errMsg = $_.Exception.Message
        if ($_.Exception.InnerException) { $errMsg += " | " + $_.Exception.InnerException.Message }
        return [pscustomobject]@{
            StatusCode = 0
            Body       = ''
            Json       = $null
            Success    = $false
            Error      = "Network error: $errMsg"
            NetworkOk  = $false
            CallOk     = $true
        }
    } catch [System.Net.WebException] {
        $errMsg = $_.Exception.Message
        if ($_.Exception.InnerException) { $errMsg += " | " + $_.Exception.InnerException.Message }
        return [pscustomobject]@{
            StatusCode = 0
            Body       = ''
            Json       = $null
            Success    = $false
            Error      = "Network error: $errMsg"
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
$paramErrors = Test-Parameters -ApiBase $ApiBase
if ($paramErrors.Count -gt 0) {
    Write-Host "[FAIL] parameters" -ForegroundColor Red
    foreach ($e in $paramErrors) { Write-Host "       - $e" -ForegroundColor DarkRed }
    $ErrorActionPreference = $originalErrorActionPreference
    exit 2
}

Write-Host "Sigap patient portal smoke suite"
Write-Host "API base       : $ApiBase"
Write-Host ""

$apiHealthy = $false

# ---------------------------------------------------------------
# Step 1: Health check
# ---------------------------------------------------------------
Invoke-Step -Name 'api.health' -Body {
    Write-Step "Step 1/5: GET /health"
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
        $script:apiHealthy = $true
        Add-Result -Name 'api.health' -Pass $true -Detail "HTTP 200"
    } else {
        $errDetail = if ($resp.Error) { $resp.Error } else { "body: $($resp.Body)" }
        Add-Result -Name 'api.health' -Pass $false -Detail "HTTP $($resp.StatusCode) — $errDetail"
    }
}

# ---------------------------------------------------------------
# Step 2: Valid lookup — SMOKE01 deterministic demo code
# ---------------------------------------------------------------
$step2Body = $null
Invoke-Step -Name 'patient.status.valid_lookup' -Body {
    Write-Step "Step 2/5: GET /api/v1/patient/status?code=SMOKE01"
    if (-not $script:apiHealthy) {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail 'Skipped: API health check failed — is the API running?'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/patient/status?code=SMOKE01'
    if (-not $resp.CallOk -or -not $resp.NetworkOk) {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail "Network/transport error: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -ne 200) {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail "Expected HTTP 200 but got HTTP $($resp.StatusCode)"
        return
    }
    if ($null -eq $resp.Json -or -not $resp.Json.success) {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail "HTTP 200 but success!=true"
        return
    }
    # Verify data.facility_name is non-empty.
    $facilityName = [string]$resp.Json.data.facility_name
    if ([string]::IsNullOrWhiteSpace($facilityName)) {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail "data.facility_name is empty or missing"
        return
    }
    # Verify data.found_by is "checkin_code".
    $foundBy = [string]$resp.Json.data.found_by
    if ($foundBy -ne 'checkin_code') {
        Add-Result -Name 'patient.status.valid_lookup' -Pass $false -Detail "Expected data.found_by='checkin_code' but got '$foundBy'"
        return
    }
    # Store the body string for PII check in step 5. Never printed.
    $script:step2Body = $resp.Body
    Add-Result -Name 'patient.status.valid_lookup' -Pass $true -Detail "HTTP 200 success=true facility_name=<non-empty> found_by=checkin_code"
}

# ---------------------------------------------------------------
# Step 3: Invalid code — forbidden characters, expect 400
# ---------------------------------------------------------------
Invoke-Step -Name 'patient.status.invalid_code' -Body {
    Write-Step "Step 3/5: GET /api/v1/patient/status?code=%3Cscript%3E (expect 400)"
    if (-not $script:apiHealthy) {
        Add-Result -Name 'patient.status.invalid_code' -Pass $false -Detail 'Skipped: API health check failed — is the API running?'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/patient/status?code=%3Cscript%3E'
    if (-not $resp.CallOk) {
        Add-Result -Name 'patient.status.invalid_code' -Pass $false -Detail "Could not call API: $($resp.Error)"
        return
    }
    if (-not $resp.NetworkOk) {
        Add-Result -Name 'patient.status.invalid_code' -Pass $false -Detail "Network unreachable: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 400) {
        Add-Result -Name 'patient.status.invalid_code' -Pass $true -Detail "HTTP 400 as expected"
    } else {
        Add-Result -Name 'patient.status.invalid_code' -Pass $false -Detail "Expected HTTP 400 but got HTTP $($resp.StatusCode)"
    }
}

# ---------------------------------------------------------------
# Step 4: Unknown safe code, expect 404
# ---------------------------------------------------------------
Invoke-Step -Name 'patient.status.unknown_code' -Body {
    Write-Step "Step 4/5: GET /api/v1/patient/status?code=ZZZZZXXXXX999 (expect 404)"
    if (-not $script:apiHealthy) {
        Add-Result -Name 'patient.status.unknown_code' -Pass $false -Detail 'Skipped: API health check failed — is the API running?'
        return
    }
    $resp = Invoke-ApiJson -Method GET -Path '/api/v1/patient/status?code=ZZZZZXXXXX999'
    if (-not $resp.CallOk) {
        Add-Result -Name 'patient.status.unknown_code' -Pass $false -Detail "Could not call API: $($resp.Error)"
        return
    }
    if (-not $resp.NetworkOk) {
        Add-Result -Name 'patient.status.unknown_code' -Pass $false -Detail "Network unreachable: $($resp.Error)"
        return
    }
    if ($resp.StatusCode -eq 404) {
        Add-Result -Name 'patient.status.unknown_code' -Pass $true -Detail "HTTP 404 as expected"
    } else {
        Add-Result -Name 'patient.status.unknown_code' -Pass $false -Detail "Expected HTTP 404 but got HTTP $($resp.StatusCode)"
    }
}

# ---------------------------------------------------------------
# Step 5: PII field absence — verify step 2 response does NOT
# contain forbidden field names.
# Privacy: never prints any patient data; only reports which
# forbidden fields were found (if any).
# ---------------------------------------------------------------
Invoke-Step -Name 'patient.status.pii_absence' -Body {
    Write-Step "Step 5/5: PII field absence check on valid lookup response"
    if ($null -eq $script:step2Body) {
        Add-Result -Name 'patient.status.pii_absence' -Pass $false -Detail 'Skipped: step 2 did not produce a response body.'
        return
    }
    $foundFields = New-Object System.Collections.Generic.List[string]
    foreach ($field in $script:forbiddenFields) {
        # Check if the JSON key appears in the raw body string.
        # This matches both "patient_phone" and nested occurrences.
        if ($script:step2Body -match [regex]::Escape("`"$field`"")) {
            $foundFields.Add($field) | Out-Null
        }
    }
    if ($foundFields.Count -eq 0) {
        Add-Result -Name 'patient.status.pii_absence' -Pass $true -Detail "No forbidden PII fields found in response"
    } else {
        # Privacy: list the field names only — never print their values.
        $fieldList = ($foundFields -join ', ')
        Add-Result -Name 'patient.status.pii_absence' -Pass $false -Detail "Forbidden PII fields present in response: $fieldList"
    }
}

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
Write-Host ""
Write-Host "================================================="
Write-Host "Patient portal smoke summary" -ForegroundColor Cyan
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
