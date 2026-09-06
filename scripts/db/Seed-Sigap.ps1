<#
.SYNOPSIS
    Guarded seed runner for Sigap local demo data (AUDIT-607).

.DESCRIPTION
    Applies packages/db/seed/dev.sql, rbac.sql, and demo.sql via psql.
    Seeds are synthetic demo data only — no DDL — and must only run when
    SIGAP_ENV=local. The guard is enforced at the official entrypoint so
    `psql -f ...demo.sql` remains possible but is documented as unsupported
    outside local.

    Direct `psql -f packages/db/seed/demo.sql` is not blocked inside SQL
    itself; the SQL file is data-only and the guard lives here (centralized).

.PARAMETER DatabaseUrl
    PostgreSQL connection string. Defaults to $env:DATABASE_URL.

.EXAMPLE
    $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
    pwsh -NoProfile -File scripts/db/Seed-Sigap.ps1

.EXAMPLE
    SIGAP_ENV=staging pwsh -NoProfile -File scripts/db/Seed-Sigap.ps1
    # → Refusing to run demo seeds unless SIGAP_ENV=local (exit 1)

.NOTES
    Requires psql on PATH. Fails fast if SIGAP_ENV is not local.
#>

[CmdletBinding()]
param(
    [string]$DatabaseUrl = $env:DATABASE_URL
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:SIGAP_ENV) -or $env:SIGAP_ENV -ne 'local') {
    $current = if ([string]::IsNullOrWhiteSpace($env:SIGAP_ENV)) { '(unset)' } else { $env:SIGAP_ENV }
    Write-Host "[FAIL] Refusing to run demo seeds unless SIGAP_ENV=local (current: $current)." -ForegroundColor Red
    Write-Host "       Demo seeds contain synthetic facility/schedule/appointment IDs (d000, SMOKE01, +62-555-01xx)" -ForegroundColor Yellow
    Write-Host "       and must not run against staging/production. Set `$env:SIGAP_ENV='local' to run locally." -ForegroundColor Yellow
    exit 1
}

if ([string]::IsNullOrWhiteSpace($DatabaseUrl)) {
    Write-Host "[FAIL] DATABASE_URL is not set." -ForegroundColor Red
    Write-Host "       Set `$env:DATABASE_URL or pass -DatabaseUrl explicitly." -ForegroundColor Yellow
    exit 1
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path

$seeds = @(
    (Join-Path $RepoRoot 'packages\db\seed\dev.sql'),
    (Join-Path $RepoRoot 'packages\db\seed\rbac.sql'),
    (Join-Path $RepoRoot 'packages\db\seed\demo.sql')
)

foreach ($seed in $seeds) {
    Write-Host "==> Seeding: $seed" -ForegroundColor Cyan
    & psql $DatabaseUrl -v ON_ERROR_STOP=1 -f $seed
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[FAIL] Seed failed: $seed (exit $LASTEXITCODE)" -ForegroundColor Red
        exit 1
    }
}

Write-Host ""
Write-Host "[PASS] All seeds applied (SIGAP_ENV=local, idempotent)." -ForegroundColor Green
