<#
.SYNOPSIS
    Sigap full local demo verification — seeds + smoke suites in one run.

.DESCRIPTION
    Orchestrates the complete local demo readiness flow:
      1. Seed dev.sql    (facilities, service units, dev user)
      2. Seed rbac.sql   (roles, permissions, role_permissions)
      3. Seed demo.sql   (schedules, demo appointments, notification outbox)
      4. sigap-demo-smoke.ps1          (8-step booking/check-in flow)
      5. sigap-notification-smoke.ps1  (9-step notification pipeline)
      6. sigap-patient-portal-smoke.ps1 (5-step public status lookup)

    Fails fast on any error. The API must already be running at
    $env:SIGAP_API_BASE (default http://127.0.0.1:8080).

.PARAMETER SkipSeed
    Skip the three psql seed commands (assume DB is already seeded).

.EXAMPLE
    pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1 -SkipSeed

.NOTES
    Requires PowerShell 7+, psql, and a running Sigap API.
    Environment variables:
      - DATABASE_URL        (required) PostgreSQL connection string
      - SIGAP_API_BASE      (optional)  API base URL, default http://127.0.0.1:8080
#>

[CmdletBinding()]
param(
    [switch]$SkipSeed
)

# --- Pre-flight: DATABASE_URL ---
if (-not $env:DATABASE_URL) {
    Write-Host "[FAIL] DATABASE_URL is not set. Export it before running this script." -ForegroundColor Red
    exit 1
}

if ([string]::IsNullOrWhiteSpace($env:SIGAP_ENV) -or $env:SIGAP_ENV -ne 'local') {
    Write-Host "[FAIL] Refusing to run demo seeds unless SIGAP_ENV=local (current: $(if([string]::IsNullOrWhiteSpace($env:SIGAP_ENV)){'unset'}else{$env:SIGAP_ENV})). Set `$env:SIGAP_ENV='local' to run locally." -ForegroundColor Red
    exit 1
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path

# --- Helper ---
function Invoke-Phase {
    param(
        [string]$Label,
        [scriptblock]$Block
    )
    Write-Host ""
    Write-Host "=================================================" -ForegroundColor Cyan
    Write-Host $Label -ForegroundColor Cyan
    Write-Host "=================================================" -ForegroundColor Cyan
    & $Block
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[FAIL] $Label failed with exit code $LASTEXITCODE" -ForegroundColor Red
        exit 1
    }
}

# --- Seed phase ---
if (-not $SkipSeed) {
    Invoke-Phase -Label 'SEED: dev.sql' -Block { psql $env:DATABASE_URL -f (Join-Path $RepoRoot 'packages\db\seed\dev.sql') }
    Invoke-Phase -Label 'SEED: rbac.sql' -Block { psql $env:DATABASE_URL -f (Join-Path $RepoRoot 'packages\db\seed\rbac.sql') }
    Invoke-Phase -Label 'SEED: demo.sql' -Block { psql $env:DATABASE_URL -f (Join-Path $RepoRoot 'packages\db\seed\demo.sql') }
} else {
    Write-Host ""
    Write-Host "Skipping seed phase (-SkipSeed)" -ForegroundColor Yellow
}

# --- Smoke phase ---
Invoke-Phase -Label 'SMOKE: demo' -Block { pwsh -NoProfile -File (Join-Path $PSScriptRoot 'sigap-demo-smoke.ps1') }
Invoke-Phase -Label 'SMOKE: notification' -Block { pwsh -NoProfile -File (Join-Path $PSScriptRoot 'sigap-notification-smoke.ps1') }
Invoke-Phase -Label 'SMOKE: patient portal' -Block { pwsh -NoProfile -File (Join-Path $PSScriptRoot 'sigap-patient-portal-smoke.ps1') }

# --- Summary ---
Write-Host ""
Write-Host "=================================================" -ForegroundColor Green
Write-Host "FULL LOCAL DEMO: PASS" -ForegroundColor Green
Write-Host "=================================================" -ForegroundColor Green
exit 0
