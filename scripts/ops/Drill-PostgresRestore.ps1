<#
.SYNOPSIS
    Monthly restore drill for Sigap PostgreSQL (AUDIT-701).

.DESCRIPTION
    Backup → disposable DB → restore → verify critical tables → optional smoke.

.PARAMETER Keep
    Keep the disposable restore DB after the drill.

.EXAMPLE
    pwsh -NoProfile -File scripts/ops/Drill-PostgresRestore.ps1
    pwsh -NoProfile -File scripts/ops/Drill-PostgresRestore.ps1 -Keep
#>

[CmdletBinding()]
param(
    [switch]$Keep
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    Write-Host "[FAIL] DATABASE_URL is required (source)." -ForegroundColor Red
    exit 1
}

$restoreDb = "sigap_restore_drill"
$restoreUrl = $env:DATABASE_URL -replace '/sigap(\?|$)', "/$restoreDb`$1"

Write-Host "=================================================" -ForegroundColor Cyan
Write-Host " Sigap restore drill (AUDIT-701)" -ForegroundColor Cyan
Write-Host " Source : (redacted)" -ForegroundColor Cyan
Write-Host " Restore: $restoreDb (disposable)" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan

$psql = "psql"
if (Test-Path "C:\Program Files\PostgreSQL\18\bin\psql.exe") { $psql = "C:\Program Files\PostgreSQL\18\bin\psql.exe" }

# 1. Backup
Write-Host ""
Write-Host "[1/5] Backup source" -ForegroundColor Cyan
pwsh -NoProfile -File (Join-Path $PSScriptRoot 'Backup-Postgres.ps1')
if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] backup" -ForegroundColor Red; exit 1 }

# Resolve latest dump
$backupDir = if ([string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_DIR)) { Join-Path (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path 'backups\sigap' } else { $env:SIGAP_BACKUP_DIR }
$latest = Get-ChildItem -Path $backupDir -Filter "sigap-*.dump" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $latest) { Write-Host "[FAIL] no dump found in $backupDir" -ForegroundColor Red; exit 1 }
$latestSha = "$($latest.FullName).sha256"
Write-Host "  latest: $($latest.Name) ($([int]($latest.Length/1024)) KB)"

# 2. Create disposable DB
Write-Host ""
Write-Host "[2/5] Create disposable DB $restoreDb" -ForegroundColor Cyan
$adminUrl = $env:DATABASE_URL -replace '/sigap(\?|$)', '/postgres$1'
& $psql $adminUrl -c "DROP DATABASE IF EXISTS $restoreDb;" 2>&1 | Write-Host
& $psql $adminUrl -c "CREATE DATABASE $restoreDb OWNER sigap;" 2>&1 | Write-Host
if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] create disposable DB" -ForegroundColor Red; exit 1 }

# 3. Restore
Write-Host ""
Write-Host "[3/5] Restore into $restoreDb" -ForegroundColor Cyan
$env:SIGAP_RESTORE_DATABASE_URL = $restoreUrl
$checksumArg = @()
if (Test-Path $latestSha) { $checksumArg = @("-Checksum", $latestSha) }
pwsh -NoProfile -File (Join-Path $PSScriptRoot 'Restore-Postgres.ps1') -Dump $latest.FullName @checksumArg 2>&1 | Write-Host
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] restore" -ForegroundColor Red
    if (-not $Keep) { & $psql $adminUrl -c "DROP DATABASE IF EXISTS $restoreDb;" 2>&1 | Out-Null }
    exit 1
}

# 4. Verify critical tables + counts
Write-Host ""
Write-Host "[4/5] Verify critical data" -ForegroundColor Cyan
$tables = @("schema_migrations", "facilities", "service_units", "practitioner_schedules", "appointments", "audit_events")
$fail = $false
foreach ($t in $tables) {
    $cnt = & $psql $restoreUrl -t -A -c "SELECT count(*) FROM $t;" 2>&1
    if ($LASTEXITCODE -ne 0) { Write-Host "  $t : FAIL $cnt" -ForegroundColor Red; $fail = $true } else { Write-Host "  $t : $($cnt.Trim())" }
}
if ($fail) { Write-Host "[FAIL] verification" -ForegroundColor Red; if (-not $Keep) { & $psql $adminUrl -c "DROP DATABASE IF EXISTS $restoreDb;" 2>&1 | Out-Null }; exit 1 }

# 5. Optional: application smoke against restored DB (health check via migrations status is sufficient for drill)
Write-Host ""
Write-Host "[5/5] Drill summary" -ForegroundColor Green
Write-Host "  dump: $($latest.Name)"
Write-Host "  restore DB: $restoreDb"
Write-Host "  result: PASS"

if (-not $Keep) {
    Write-Host "  cleaning up $restoreDb"
    & $psql $adminUrl -c "DROP DATABASE IF EXISTS $restoreDb;" 2>&1 | Out-Null
} else {
    Write-Host "  kept $restoreDb (--Keep)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=================================================" -ForegroundColor Green
Write-Host " DRILL: PASS" -ForegroundColor Green
Write-Host "=================================================" -ForegroundColor Green
