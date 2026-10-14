<#
.SYNOPSIS
    Sigap production-readiness smoke — new routes, admin mutations, and the P0 proofs.

.DESCRIPTION
    Post-deploy / pre-production verification for Phase 3B7 (T-3B7-02). Covers the
    parts of the canonical smoke list (plan Section 15.8) that the API-only smoke
    scripts do not reach:

      ROUTES (availability — 200 + a stable SSR marker)
      1.  web.route.root              GET /                        200 + Beranda
      2.  web.route.faskes            GET /faskes                  200 + catalog
      3.  web.route.queues_new        GET /queues/new              200 + walk-in form
      4.  web.route.admin             GET /admin                   200 + admin shell
      5.  web.route.appointments_new  GET /appointments/new        200 + booking form
      6.  web.route.checkin           GET /appointments/check-in   200 + check-in form
      7.  web.route.patient_status    GET /patient/status          200 + status form
      8.  api.public_catalog          GET /api/v1/public/facilities 200 + data array

      P0 PROOFS (plan Section 15.8)
      9.  p0.retry.in_scope.status    in-scope retry -> 200            (POSITIVE CONTROL)
      10. p0.retry.in_scope.mutates   the in-scope retry DID write     (POSITIVE CONTROL)
      11. p0.retry.out_of_scope.status cross-facility retry -> 404
      12. p0.retry.out_of_scope.no_write the row is UNCHANGED (before == after)
      13. p0.summary.zero_assignment  zero-assignment non-super_admin -> all-zero
      14. p0.summary.global_super_admin active global super_admin -> global counts

      ADMIN MUTATIONS (plan Section 15.8)
      15. admin.queue_status_transition  waiting -> called, row persists
      16. admin.facility_deactivate      is_active becomes false (disposable facility)

    WHY THE P0-1 PROOF HAS A POSITIVE CONTROL.
    "Returns 404" is not the canonical proof — "returns 404 AND the row is
    unchanged" is. But even that pair can pass vacuously: the handler also
    answers 404 for an unrecognised path suffix, so an endpoint that had been
    broken or renamed would yield 404 for the same request, the no-write step
    would pass because nothing ran, and the suite would be green over a missing
    endpoint. Steps 9-10 close that hole: the SAME subject, against a row inside
    its own facility, must get 200 AND the row must actually change. Only with
    that positive control does the 404 in step 11 mean "denied by scope" rather
    than "endpoint gone".

    WHY THE OUT-OF-SCOPE CASE IS CROSS-FACILITY.
    The plan's narrative (Section 15.3 / 4.5) is about an actor that genuinely
    manages one facility attempting a mutation at ANOTHER. So step 11 drives a
    facility-scoped subject (manage at the demo facility) against a probe row
    belonging to a DIFFERENT facility. Driving it with the zero-assignment
    subject would collapse the cross-facility proof and the zero-scope proof
    onto one actor, and would still pass if cross-facility isolation regressed.

    WHY 13/14 ARE A MATCHED PAIR.
    Asserting only "zero actor sees zeros" would also pass if the summary
    endpoint were simply broken for everyone. Step 14 requires a DB-resolved
    active global super_admin to still see global counts, which is the contrast
    that makes step 13 meaningful. Step 13 requires every declared status key to
    be PRESENT and zero, so a regression that dropped key initialisation (and
    returned `{}`) fails.

    IDENTITY. The P0 and admin-mutation steps act as specific DB-seeded subjects
    via the LOCAL test identity selector (X-Sigap-Local-Test-Subject), which the
    API arms only under SIGAP_ENV=local + SIGAP_LOCAL_RBAC_TEST_IDENTITY=true.
    Each subject's permission is resolved from the database, never from a header
    claim. If the selector is not armed the script reports that as a PARAMETER
    failure (exit 2), not as a misleading assertion failure.

    PROBE FIXTURES. The two retry probes and the deactivation facility are created
    at run time and removed in a `finally` block. The walk-in minted by the admin
    queue-transition check is also removed (its ticket, patient and medical record).
    The probe phone is RANDOMISED per run: the walk-in endpoint allows only 2
    tickets per (date, phone, facility), so a fixed phone would make a third run in
    one day return 429 and fail the check. The engine's `daily_queue_counters` row
    is deliberately left alone — it is monotonic engine state, not a probe.

    This is a REHEARSAL tool: it is meant for a production-equivalent local/staging
    stack, not for a live production deployment (see DEPLOYMENT_RUNBOOK.md §4a).

.PARAMETER ApiBase
    API root. Defaults to $env:SIGAP_API_BASE or http://127.0.0.1:8080.

.PARAMETER WebBase
    Web origin. Defaults to $env:SIGAP_WEB_BASE or http://127.0.0.1:4173.

.PARAMETER DatabaseUrl
    PostgreSQL connection string, used to snapshot probe rows for the P0-1
    no-write proof. Defaults to $env:DATABASE_URL.

.EXAMPLE
    pwsh -NoProfile -File scripts/smoke/sigap-production-readiness-smoke.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/smoke/sigap-production-readiness-smoke.ps1 `
        -ApiBase http://127.0.0.1:18080 -WebBase http://127.0.0.1:4173

.NOTES
    Requires PowerShell 7+, psql, and a running production-equivalent LOCAL stack
    (real Go API + real database + real web build/preview + real Rust engine).
    The dev/local identity selector must be armed (Start-LocalE2E.ps1 arms it).

    SAFETY. Both targets MUST be loopback. A non-loopback target is rejected
    before any request is made (fail closed), so this script can never be pointed
    at production by accident. It never contacts https://sigap.chaerulchalik.web.id.

    Exit codes: 0 all checks pass; 1 an assertion failed; 2 parameter/precondition
    failure (bad target, missing DATABASE_URL, or the local identity selector is
    not armed).
#>

[CmdletBinding()]
param(
    [string]$ApiBase = $(if ($env:SIGAP_API_BASE) { $env:SIGAP_API_BASE } else { 'http://127.0.0.1:8080' }),
    [string]$WebBase = $(if ($env:SIGAP_WEB_BASE) { $env:SIGAP_WEB_BASE } else { 'http://127.0.0.1:4173' }),
    [string]$DatabaseUrl = $env:DATABASE_URL,
    [string]$ZeroScopeSubject = 'local-zero-scope-admin',
    [string]$GlobalSuperAdminSubject = 'local-global-super-admin',
    [string]$ScopedSubject = 'local-facility-admin',
    # The facility the scoped subject MANAGES (seeded: facility_admin at d000).
    [string]$ScopedFacilityId = '00000000-0000-0000-0000-00000000d000',
    # A DIFFERENT facility the scoped subject does NOT manage (seeded dev facility).
    [string]$OtherFacilityId = '00000000-0000-0000-0000-00000000e000',
    # Escape hatch for an explicitly authorized staging rehearsal. Off by default:
    # the script refuses a non-loopback database, because it writes probe rows.
    [switch]$AllowNonLocalDatabase
)

$originalErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

$results = New-Object System.Collections.Generic.List[object]

# Declared notification status keys — mirrors notification.AllStatuses().
$script:NotificationStatuses = @('pending', 'processing', 'delivered', 'failed', 'cancelled')

# Probe identifiers (deterministic, distinctive, never part of any seed).
$script:ProbeInScope  = '00000000-0000-0000-0000-0000000d0f01'
$script:ProbeOutScope = '00000000-0000-0000-0000-0000000d0f02'
$script:ProbeShortCode = 'G7PROBE'
$script:ProbeFacilityId = $null
$script:ProbeTicketId = $null
# Randomised per run: the walk-in endpoint caps tickets per (date, phone, facility),
# so a fixed probe phone would 429 on the third run of the day.
$script:ProbePhone = '0812' + (Get-Random -Minimum 10000000 -Maximum 99999999)

# ---------------------------------------------------------------
# Logging helpers (same shape as the other scripts in this suite)
# ---------------------------------------------------------------
function Write-Step { param([string]$Message); Write-Host ""; Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Info { param([string]$Message); Write-Host "[INFO] $Message" -ForegroundColor DarkCyan }

function Add-Result {
    param([string]$Name, [bool]$Pass, [string]$Detail = '')
    $results.Add([pscustomobject]@{ Step = $Name; Pass = $Pass; Detail = $Detail }) | Out-Null
    if ($Pass) {
        Write-Host "[PASS] $Name" -ForegroundColor Green
        if ($Detail) { Write-Host "       $Detail" -ForegroundColor DarkGray }
    } else {
        Write-Host "[FAIL] $Name" -ForegroundColor Red
        if ($Detail) { Write-Host "       $Detail" -ForegroundColor DarkRed }
    }
}

function Write-ParamFail {
    param([string]$Message)
    Write-Host "[FAIL] parameters" -ForegroundColor Red
    Write-Host "       $Message" -ForegroundColor DarkRed
    $ErrorActionPreference = $originalErrorActionPreference
    exit 2
}

# ---------------------------------------------------------------
# Production-target guard — mirrors apps/web/playwright.config.ts.
# ---------------------------------------------------------------
$script:AllowedHosts = @('localhost', '127.0.0.1', '::1', '[::1]')

function Assert-LoopbackTarget {
    param([string]$Label, [string]$Target)
    if ([string]::IsNullOrWhiteSpace($Target)) { Write-ParamFail "$Label must not be empty." }
    if ($Target -match '\s') { Write-ParamFail "$Label must not contain whitespace (got: '$Target')." }
    $uri = $null
    try { $uri = [System.Uri]$Target } catch { Write-ParamFail "$Label is not a valid URL (got: '$Target')." }
    if ($uri.Scheme -ne 'http' -and $uri.Scheme -ne 'https') {
        Write-ParamFail "$Label must be http:// or https:// (got: '$Target')."
    }
    if (-not ($script:AllowedHosts -contains $uri.Host)) {
        Write-ParamFail "$Label must be a LOOPBACK host ($($script:AllowedHosts -join ', ')); refusing '$($uri.Host)'. Smoke runs never target production."
    }
}

# The database is the target of real DML (probe INSERT/DELETE), so it gets the
# same loopback guard as the HTTP targets. Without this, a production
# DATABASE_URL left in the shell would be written to even though the HTTP
# targets are loopback, and the "never targets production" claim would be false.
function Assert-LoopbackDatabase {
    param([string]$Dsn, [switch]$AllowNonLocalDatabase)
    if ([string]::IsNullOrWhiteSpace($Dsn)) {
        Write-ParamFail 'DatabaseUrl is required (probe fixtures + the P0-1 no-write proof). Pass -DatabaseUrl or set $env:DATABASE_URL.'
    }
    $dbHost = ''
    try {
        # postgresql://user:pass@host:port/db  ->  [System.Uri].Host
        $dbHost = ([System.Uri]$Dsn).Host
    } catch {
        Write-ParamFail "DatabaseUrl is not a parseable connection string."
    }
    if ($AllowNonLocalDatabase) {
        Write-Info "database host '$dbHost' allowed by -AllowNonLocalDatabase (probe rows WILL be written there)"
        return $dbHost
    }
    if (-not ($script:AllowedHosts -contains $dbHost)) {
        Write-ParamFail "DatabaseUrl must point at a LOOPBACK host ($($script:AllowedHosts -join ', ')); refusing '$dbHost'. Pass -AllowNonLocalDatabase to override for an explicitly authorized staging rehearsal."
    }
    return $dbHost
}

# ---------------------------------------------------------------
# HTTP helpers — never throw; always return a consistent object.
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

    $params = @{ Method = $Method; Uri = $uri; Headers = $reqHeaders; TimeoutSec = $TimeoutSec; SkipHttpErrorCheck = $true }
    if ($null -ne $Body) {
        try { $json = $Body | ConvertTo-Json -Depth 10 -Compress }
        catch {
            return [pscustomobject]@{ StatusCode = 0; Body = ''; Json = $null; Success = $false
                Error = "Failed to serialize request body: $($_.Exception.Message)"; NetworkOk = $false; CallOk = $false }
        }
        $params['Body'] = $json
        $params['ContentType'] = 'application/json'
    }
    try {
        $resp = Invoke-WebRequest @params
        $content = ''
        if ($null -ne $resp.Content) { $content = [string]$resp.Content }
        $parsed = $null
        if ($content) { try { $parsed = $content | ConvertFrom-Json -ErrorAction Stop } catch { $parsed = $content } }
        $statusCode = 0
        if ($resp.StatusCode) { $statusCode = [int]$resp.StatusCode }
        return [pscustomobject]@{ StatusCode = $statusCode; Body = $content; Json = $parsed
            Success = ($statusCode -ge 200 -and $statusCode -lt 300); Error = $null; NetworkOk = $true; CallOk = $true }
    } catch {
        return [pscustomobject]@{ StatusCode = 0; Body = ''; Json = $null; Success = $false
            Error = $_.Exception.Message; NetworkOk = $false; CallOk = $false }
    }
}

function Invoke-WebGet {
    param([Parameter(Mandatory)] [string]$Path, [int]$TimeoutSec = 15)
    $uri = "$WebBase$Path"
    try {
        $resp = Invoke-WebRequest -Method GET -Uri $uri -TimeoutSec $TimeoutSec -SkipHttpErrorCheck
        $content = ''
        if ($null -ne $resp.Content) { $content = [string]$resp.Content }
        return [pscustomobject]@{ StatusCode = [int]$resp.StatusCode; Content = $content; NetworkOk = $true; Error = $null }
    } catch {
        return [pscustomobject]@{ StatusCode = 0; Content = ''; NetworkOk = $false; Error = $_.Exception.Message }
    }
}

function Invoke-Step {
    param([Parameter(Mandatory)] [string]$Name, [Parameter(Mandatory)] [scriptblock]$Body)
    try { & $Body } catch { Add-Result -Name $Name -Pass $false -Detail "Unhandled exception: $($_.Exception.Message)" }
}

# ---------------------------------------------------------------
# psql helpers (probe fixtures + the no-write snapshot).
# ---------------------------------------------------------------
function Invoke-Psql {
    param([Parameter(Mandatory)] [string]$Sql)
    $out = & psql $DatabaseUrl -tAc $Sql 2>&1
    if ($LASTEXITCODE -ne 0) { throw "psql failed ($LASTEXITCODE): $out" }
    if ($null -eq $out) { return '' }
    return ($out | Out-String).Trim()
}

# Whole-row snapshot: compares EVERY column, so a write that touched any field
# is caught. The canonical Go proof compares row_to_json for the same reason.
function Get-RowJson {
    param([Parameter(Mandatory)] [string]$Table, [Parameter(Mandatory)] [string]$Id)
    return (Invoke-Psql -Sql "SELECT row_to_json(t)::text FROM $Table t WHERE t.id = '$Id'")
}

function New-ProbeRow {
    param([string]$Id, [string]$FacilityId, [string]$TemplateKey)
    $hash = "decode(repeat('ab', 32), 'hex')"
    $sql = @"
INSERT INTO notification_outbox
    (id, facility_id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at, related_resource_type, related_resource_id)
VALUES ('$Id'::uuid, '$FacilityId'::uuid, 'dev', '$TemplateKey', 'Gate 7 probe', 'probe',
        'patient', '+62****0000', $hash, 'failed', 1, NOW(), 'smoke_probe', '$Id'::uuid)
ON CONFLICT (id) DO UPDATE SET status='failed', attempt_count=1, next_attempt_at=NOW(), updated_at=NOW()
"@
    Invoke-Psql -Sql $sql | Out-Null
}

function Remove-ProbeRow {
    param([string]$Id)
    Invoke-Psql -Sql "DELETE FROM notification_outbox WHERE id = '$Id'" | Out-Null
}

function Remove-ProbeFacility {
    if ($script:ProbeFacilityId) {
        Invoke-Psql -Sql "DELETE FROM facilities WHERE id = '$($script:ProbeFacilityId)'" | Out-Null
        $script:ProbeFacilityId = $null
    }
}

function Test-WebRoute {
    param([string]$Name, [string]$Path, [string]$Marker)
    Invoke-Step -Name $Name -Body {
        $resp = Invoke-WebGet -Path $Path
        if (-not $resp.NetworkOk) { Add-Result -Name $Name -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name $Name -Pass $false -Detail "expected 200, got $($resp.StatusCode)"; return }
        if ($Marker -and ($resp.Content -notmatch [regex]::Escape($Marker))) {
            Add-Result -Name $Name -Pass $false -Detail "200 but body did not contain marker '$Marker'"
            return
        }
        Add-Result -Name $Name -Pass $true -Detail "200$(if($Marker){" + marker '$Marker'"})"
    }
}

# ---------------------------------------------------------------
# Pre-flight
# ---------------------------------------------------------------
Assert-LoopbackTarget -Label 'ApiBase' -Target $ApiBase
Assert-LoopbackTarget -Label 'WebBase' -Target $WebBase
$dbHost = Assert-LoopbackDatabase -Dsn $DatabaseUrl -AllowNonLocalDatabase:$AllowNonLocalDatabase

Write-Host ""
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host " Sigap production-readiness smoke (T-3B7-02)" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host "  API   $ApiBase"
Write-Host "  Web   $WebBase"
Write-Host "  DB    $dbHost (probe fixtures + row snapshots)"
Write-Host "=================================================" -ForegroundColor Cyan

$probesReady = $false
try {
    # --- 1-7. Web routes (availability + a stable SSR marker) ----------------
    Write-Step 'Web routes (availability)'
    Test-WebRoute -Name 'web.route.root'             -Path '/'                      -Marker 'id="sigap-beranda-title"'
    Test-WebRoute -Name 'web.route.faskes'           -Path '/faskes'                -Marker 'id="sigap-faskes-title"'
    Test-WebRoute -Name 'web.route.queues_new'       -Path '/queues/new'            -Marker 'id="sigap-walkin-title"'
    Test-WebRoute -Name 'web.route.admin'            -Path '/admin'                 -Marker 'sigap-admin-shell'
    Test-WebRoute -Name 'web.route.appointments_new' -Path '/appointments/new'      -Marker 'id="sigap-booking-title"'
    Test-WebRoute -Name 'web.route.checkin'          -Path '/appointments/check-in' -Marker 'id="sigap-checkin-title"'
    Test-WebRoute -Name 'web.route.patient_status'   -Path '/patient/status'        -Marker 'id="sigap-status-title"'

    # --- 8. Public catalog (API) ---------------------------------------------
    Write-Step 'Public catalog'
    Invoke-Step -Name 'api.public_catalog' -Body {
        $resp = Invoke-ApiJson -Method GET -Path '/api/v1/public/facilities'
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'api.public_catalog' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name 'api.public_catalog' -Pass $false -Detail "expected 200, got $($resp.StatusCode)"; return }
        if ($null -eq $resp.Json -or -not $resp.Json.success) { Add-Result -Name 'api.public_catalog' -Pass $false -Detail 'body was not a success envelope'; return }
        $count = @($resp.Json.data).Count
        if ($count -lt 1) { Add-Result -Name 'api.public_catalog' -Pass $false -Detail 'data array is empty'; return }
        Add-Result -Name 'api.public_catalog' -Pass $true -Detail "200, data rows=$count"
    }

    # --- Identity precondition ------------------------------------------------
    Write-Step 'Identity precondition (local test identity selector)'
    $selectorArmed = $true
    Invoke-Step -Name 'precondition.selector_armed' -Body {
        $probe = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications/summary' -Headers @{ 'X-Sigap-Local-Test-Subject' = $ZeroScopeSubject }
        if (-not $probe.CallOk -or -not $probe.NetworkOk) { $script:selectorArmed = $false; Add-Result -Name 'precondition.selector_armed' -Pass $false -Detail "transport error: $($probe.Error)"; return }
        # Contract (apps/api/internal/auth/local_test_identity_provider.go): every failure
        # path returns a ZERO actor -> the request falls through to the deny-by-default
        # handler, which answers 403 "Akses ditolak: autentikasi diperlukan."
        # (internal/identity/authz.go). A usable armed selector resolves the subject from
        # the real DB RBAC and answers 200. So 200 = armed+resolvable; anything else,
        # including 403, means the selector is NOT usable here. 401 is only the
        # unrecognised-route fallback and is not this endpoint's contract.
        if ($probe.StatusCode -eq 200) {
            Add-Result -Name 'precondition.selector_armed' -Pass $true -Detail "subject '$ZeroScopeSubject' resolved (HTTP 200)"
            return
        }
        $script:selectorArmed = $false
        Add-Result -Name 'precondition.selector_armed' -Pass $false -Detail "subject '$ZeroScopeSubject' was not resolved (HTTP $($probe.StatusCode), expected 200); the local test identity selector is not armed"
    }

    if (-not $selectorArmed) {
        Write-ParamFail "The local test identity selector is not armed. Start the API with SIGAP_ENV=local and SIGAP_LOCAL_RBAC_TEST_IDENTITY=true (scripts/dev/Start-LocalE2E.ps1 does this)."
    }

    # --- Probe fixtures -------------------------------------------------------
    Write-Step 'Probe fixtures (disposable; removed in cleanup)'
    Invoke-Step -Name 'fixtures.create' -Body {
        New-ProbeRow -Id $script:ProbeInScope  -FacilityId $ScopedFacilityId -TemplateKey 'gate7.probe.in_scope'
        New-ProbeRow -Id $script:ProbeOutScope -FacilityId $OtherFacilityId  -TemplateKey 'gate7.probe.out_of_scope'
        if ([string]::IsNullOrWhiteSpace((Get-RowJson -Table 'notification_outbox' -Id $script:ProbeInScope)) -or
            [string]::IsNullOrWhiteSpace((Get-RowJson -Table 'notification_outbox' -Id $script:ProbeOutScope))) {
            Add-Result -Name 'fixtures.create' -Pass $false -Detail 'probe rows were not created (check DATABASE_URL / psql)'
            return
        }
        $script:probesReady = $true
        Add-Result -Name 'fixtures.create' -Pass $true -Detail 'two probe notification rows created'
    }

    # --- P0-1: in-scope retry (POSITIVE CONTROL) ------------------------------
    Write-Step 'P0-1 in-scope retry (positive control for the 404 proof)'
    Invoke-Step -Name 'p0.retry.in_scope.status' -Body {
        if (-not $script:probesReady) { Add-Result -Name 'p0.retry.in_scope.status' -Pass $false -Detail 'skipped: probe fixtures were not created'; return }
        $script:inScopeBefore = Get-RowJson -Table 'notification_outbox' -Id $script:ProbeInScope
        $resp = Invoke-ApiJson -Method POST -Path "/api/v1/admin/notifications/$($script:ProbeInScope)/retry" -Headers @{ 'X-Sigap-Local-Test-Subject' = $ScopedSubject }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'p0.retry.in_scope.status' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) {
            Add-Result -Name 'p0.retry.in_scope.status' -Pass $false -Detail "expected 200 for an IN-SCOPE retry as '$ScopedSubject', got $($resp.StatusCode). Without this, a 404 below cannot be distinguished from a missing endpoint."
            return
        }
        Add-Result -Name 'p0.retry.in_scope.status' -Pass $true -Detail "in-scope retry answered 200 as '$ScopedSubject'"
    }

    Invoke-Step -Name 'p0.retry.in_scope.mutates' -Body {
        if (-not $script:inScopeBefore) { Add-Result -Name 'p0.retry.in_scope.mutates' -Pass $false -Detail 'no before-snapshot (the in-scope status step did not run)'; return }
        $after = Get-RowJson -Table 'notification_outbox' -Id $script:ProbeInScope
        if ($after -eq $script:inScopeBefore) {
            Add-Result -Name 'p0.retry.in_scope.mutates' -Pass $false -Detail "the 200 did NOT change the row; the retry endpoint may be a no-op. before='$($script:inScopeBefore)'"
            return
        }
        Add-Result -Name 'p0.retry.in_scope.mutates' -Pass $true -Detail 'the in-scope retry genuinely mutated the row'
    }

    # --- P0-1: cross-facility out-of-scope retry -> 404 AND no write -----------
    Write-Step 'P0-1 cross-facility out-of-scope retry (404 + no write)'
    Invoke-Step -Name 'p0.retry.out_of_scope.status' -Body {
        if (-not $script:probesReady) { Add-Result -Name 'p0.retry.out_of_scope.status' -Pass $false -Detail 'skipped: probe fixtures were not created'; return }
        $script:outScopeBefore = Get-RowJson -Table 'notification_outbox' -Id $script:ProbeOutScope
        $resp = Invoke-ApiJson -Method POST -Path "/api/v1/admin/notifications/$($script:ProbeOutScope)/retry" -Headers @{ 'X-Sigap-Local-Test-Subject' = $ScopedSubject }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'p0.retry.out_of_scope.status' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 404) {
            Add-Result -Name 'p0.retry.out_of_scope.status' -Pass $false -Detail "expected 404 for a cross-facility retry as '$ScopedSubject', got $($resp.StatusCode)"
            return
        }
        Add-Result -Name 'p0.retry.out_of_scope.status' -Pass $true -Detail "cross-facility retry answered 404 as '$ScopedSubject'"
    }

    Invoke-Step -Name 'p0.retry.out_of_scope.no_write' -Body {
        if (-not $script:outScopeBefore) { Add-Result -Name 'p0.retry.out_of_scope.no_write' -Pass $false -Detail 'no before-snapshot (the out-of-scope status step did not run)'; return }
        $after = Get-RowJson -Table 'notification_outbox' -Id $script:ProbeOutScope
        Write-Info "row before: $($script:outScopeBefore)"
        Write-Info "row after:  $after"
        if ($after -ne $script:outScopeBefore) {
            Add-Result -Name 'p0.retry.out_of_scope.no_write' -Pass $false -Detail 'ROW CHANGED after a denied retry'
            return
        }
        Add-Result -Name 'p0.retry.out_of_scope.no_write' -Pass $true -Detail 'whole-row state identical before and after the denied retry'
    }

    # --- P0-3 + Section 4.5: summary contrast ---------------------------------
    Write-Step 'P0-3 zero-assignment summary + global super_admin contrast'
    Invoke-Step -Name 'p0.summary.zero_assignment' -Body {
        $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications/summary' -Headers @{ 'X-Sigap-Local-Test-Subject' = $ZeroScopeSubject }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'p0.summary.zero_assignment' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name 'p0.summary.zero_assignment' -Pass $false -Detail "expected 200, got $($resp.StatusCode)"; return }
        if ($null -eq $resp.Json -or -not $resp.Json.success) { Add-Result -Name 'p0.summary.zero_assignment' -Pass $false -Detail 'body was not a success envelope'; return }
        $counts = $resp.Json.data
        $missing = @()
        $nonZero = @()
        foreach ($status in $script:NotificationStatuses) {
            $prop = $counts.PSObject.Properties[$status]
            if ($null -eq $prop) { $missing += $status; continue }
            if ([int]$prop.Value -ne 0) { $nonZero += "$status=$($prop.Value)" }
        }
        if ($missing.Count -gt 0) { Add-Result -Name 'p0.summary.zero_assignment' -Pass $false -Detail "summary omitted declared status key(s): $($missing -join ', ')"; return }
        if ($nonZero.Count -gt 0) { Add-Result -Name 'p0.summary.zero_assignment' -Pass $false -Detail "zero-assignment actor saw non-zero counts: $($nonZero -join ', ')"; return }
        Add-Result -Name 'p0.summary.zero_assignment' -Pass $true -Detail 'all five declared status keys present and zero'
    }

    Invoke-Step -Name 'p0.summary.global_super_admin' -Body {
        $resp = Invoke-ApiJson -Method GET -Path '/api/v1/admin/notifications/summary' -Headers @{ 'X-Sigap-Local-Test-Subject' = $GlobalSuperAdminSubject }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'p0.summary.global_super_admin' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name 'p0.summary.global_super_admin' -Pass $false -Detail "expected 200, got $($resp.StatusCode)"; return }
        if ($null -eq $resp.Json -or -not $resp.Json.success) { Add-Result -Name 'p0.summary.global_super_admin' -Pass $false -Detail 'body was not a success envelope'; return }
        $total = 0
        foreach ($p in $resp.Json.data.PSObject.Properties) { $total += [int]$p.Value }
        if ($total -lt 1) { Add-Result -Name 'p0.summary.global_super_admin' -Pass $false -Detail 'global super_admin saw an all-zero summary; expected global counts'; return }
        Add-Result -Name 'p0.summary.global_super_admin' -Pass $true -Detail "global super_admin saw non-zero counts (total=$total)"
    }

    # --- Admin mutations ------------------------------------------------------
    Write-Step 'Admin mutations'

    # Queue status transition: mint a real ticket through the engine, then
    # advance it waiting -> called. The seed ships no tickets, so this is also
    # proof the board mutates rows the real engine produced.
    Invoke-Step -Name 'admin.queue_status_transition' -Body {
        $gen = Invoke-ApiJson -Method POST -Path '/api/v1/queues/generate' -Body @{
            facilityId = $ScopedFacilityId
            patient    = @{ fullName = 'Gate 7 Walk-In Probe'; phone = $script:ProbePhone }
        }
        if (-not $gen.CallOk -or -not $gen.NetworkOk) { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail "walk-in transport error: $($gen.Error)"; return }
        if ($gen.StatusCode -ne 200) { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail "walk-in generate expected 200, got $($gen.StatusCode)"; return }
        $ticketId = $gen.Json.data.id
        if (-not $ticketId) { $ticketId = $gen.Json.data.TicketID }
        if (-not $ticketId) { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail 'walk-in response carried no ticket id'; return }
        $script:ProbeTicketId = $ticketId

        $before = Invoke-Psql -Sql "SELECT status FROM queue_tickets WHERE id = '$ticketId'"
        $resp = Invoke-ApiJson -Method PATCH -Path "/api/v1/admin/queues/$ticketId/status" -Headers @{ 'X-Sigap-Local-Test-Subject' = $ScopedSubject } -Body @{ status = 'called' }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail "expected 200 for waiting->called, got $($resp.StatusCode)"; return }
        $after = Invoke-Psql -Sql "SELECT status FROM queue_tickets WHERE id = '$ticketId'"
        if ($after -ne 'called') { Add-Result -Name 'admin.queue_status_transition' -Pass $false -Detail "row did not persist: before='$before' after='$after'"; return }
        Add-Result -Name 'admin.queue_status_transition' -Pass $true -Detail "waiting -> called persisted (before='$before' after='$after')"
    }

    # Facility deactivate: create a DISPOSABLE facility as the unrestricted
    # global super_admin, deactivate it, and confirm is_active became false.
    # Deactivation is one-way, so it must never target a seeded facility.
    Invoke-Step -Name 'admin.facility_deactivate' -Body {
        $create = Invoke-ApiJson -Method POST -Path '/api/v1/admin/facilities' -Headers @{ 'X-Sigap-Local-Test-Subject' = $GlobalSuperAdminSubject } -Body @{
            name           = 'Gate 7 Disposable Probe'
            type           = 'puskesmas'
            address        = 'Jl. Probe No. 1'
            kecamatan      = 'Probe'
            kabupaten_kota = 'Kota Probe'
            provinsi       = 'Jawa Barat'
            phone          = '000-000000'
            total_beds     = 5
            available_beds = 5
            short_code     = $script:ProbeShortCode
        }
        if (-not $create.CallOk -or -not $create.NetworkOk) { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail "create transport error: $($create.Error)"; return }
        if ($create.StatusCode -ne 200 -and $create.StatusCode -ne 201) { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail "disposable facility create expected 200/201, got $($create.StatusCode)"; return }
        $facilityId = $create.Json.data.id
        if (-not $facilityId) { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail 'create response carried no facility id'; return }
        $script:ProbeFacilityId = $facilityId

        $resp = Invoke-ApiJson -Method PATCH -Path "/api/v1/admin/facilities/$facilityId/deactivate" -Headers @{ 'X-Sigap-Local-Test-Subject' = $GlobalSuperAdminSubject }
        if (-not $resp.CallOk -or -not $resp.NetworkOk) { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail "transport error: $($resp.Error)"; return }
        if ($resp.StatusCode -ne 200) { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail "expected 200 for deactivate, got $($resp.StatusCode)"; return }
        $isActive = Invoke-Psql -Sql "SELECT is_active::text FROM facilities WHERE id = '$facilityId'"
        if ($isActive -ne 'false') { Add-Result -Name 'admin.facility_deactivate' -Pass $false -Detail "is_active did not become false (got '$isActive')"; return }
        Add-Result -Name 'admin.facility_deactivate' -Pass $true -Detail 'disposable facility deactivated (is_active=false)'
    }
}
finally {
    # --- Cleanup: always remove the probe fixtures ---------------------------
    Write-Step 'Cleanup'
    try { Remove-ProbeRow -Id $script:ProbeInScope } catch { Write-Info "cleanup in-scope probe failed: $($_.Exception.Message)" }
    try { Remove-ProbeRow -Id $script:ProbeOutScope } catch { Write-Info "cleanup out-of-scope probe failed: $($_.Exception.Message)" }
    try { Remove-ProbeFacility } catch { Write-Info "cleanup probe facility failed: $($_.Exception.Message)" }
    # The walk-in minted by the queue-transition check: remove the ticket, then
    # the patient (FK is RESTRICT, so the ticket must go first), then the record.
    if ($script:ProbeTicketId) {
        try { Invoke-Psql -Sql "DELETE FROM queue_tickets WHERE id = '$($script:ProbeTicketId)'" | Out-Null } catch { Write-Info "cleanup ticket failed: $($_.Exception.Message)" }
    }
    try { Invoke-Psql -Sql "DELETE FROM medical_records WHERE patient_phone = '$($script:ProbePhone)'" | Out-Null } catch { Write-Info "cleanup medical record failed: $($_.Exception.Message)" }
    try { Invoke-Psql -Sql "DELETE FROM patients WHERE phone = '$($script:ProbePhone)'" | Out-Null } catch { Write-Info "cleanup patient failed: $($_.Exception.Message)" }
    # Counts are best-effort: a throw here would escape the finally block and
    # replace the real exit code, so each is wrapped.
    try {
        $leftIn  = Invoke-Psql -Sql "SELECT count(*) FROM notification_outbox WHERE id IN ('$($script:ProbeInScope)','$($script:ProbeOutScope)')"
        $leftFac = Invoke-Psql -Sql "SELECT count(*) FROM facilities WHERE short_code = '$($script:ProbeShortCode)'"
        Write-Info "probe rows remaining: $leftIn; probe facilities remaining: $leftFac"
    } catch {
        Write-Info "probe residue count unavailable: $($_.Exception.Message)"
    }
}

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
Write-Host ""
Write-Host "================================================="
Write-Host "Production-readiness smoke summary" -ForegroundColor Cyan
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
