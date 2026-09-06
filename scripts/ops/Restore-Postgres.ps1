<#
.SYNOPSIS
    Guarded PowerShell restore for Sigap PostgreSQL (AUDIT-701).

.DESCRIPTION
    Restores a pg_dump --format=custom dump into SIGAP_RESTORE_DATABASE_URL.
    Validates checksum when provided and runs post-restore verification queries.
    Refuses to restore into a URL containing 'prod' without --AllowDestructive.

.PARAMETER Dump
    Path to sigap-*.dump file. If omitted, most recent in SIGAP_BACKUP_DIR.

.PARAMETER Checksum
    Path to .sha256 file for verification.

.PARAMETER AllowDestructive
    Allow restore into a URL that contains 'prod'.

.EXAMPLE
    $env:SIGAP_RESTORE_DATABASE_URL = "postgresql://sigap:pass@127.0.0.1:5433/sigap_restore?sslmode=disable"
    pwsh -NoProfile -File scripts/ops/Restore-Postgres.ps1 -Dump .\backups\sigap\sigap-20260101T000000Z.dump -Checksum .\backups\sigap\sigap-20260101T000000Z.dump.sha256
#>

[CmdletBinding()]
param(
    [string]$Dump = "",
    [string]$Checksum = "",
    [switch]$AllowDestructive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:SIGAP_RESTORE_DATABASE_URL)) {
    Write-Host "[FAIL] SIGAP_RESTORE_DATABASE_URL is required (destination, never production by default)." -ForegroundColor Red
    exit 1
}

$backupDir = if ([string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_DIR)) { Join-Path (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path 'backups\sigap' } else { $env:SIGAP_BACKUP_DIR }

if ([string]::IsNullOrWhiteSpace($Dump)) {
    $latest = Get-ChildItem -Path $backupDir -Filter "sigap-*.dump" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $latest) { Write-Host "[FAIL] no dump found in $backupDir and -Dump not given" -ForegroundColor Red; exit 1 }
    $Dump = $latest.FullName
}

if (-not (Test-Path $Dump)) { Write-Host "[FAIL] dump not found: $Dump" -ForegroundColor Red; exit 1 }

if (-not $AllowDestructive -and $env:SIGAP_RESTORE_DATABASE_URL -match 'prod') {
    Write-Host "[FAIL] refusing to restore into a URL containing 'prod' without -AllowDestructive" -ForegroundColor Red
    exit 1
}

if (-not [string]::IsNullOrWhiteSpace($Checksum)) {
    if (-not (Test-Path $Checksum)) { Write-Host "[FAIL] checksum not found: $Checksum" -ForegroundColor Red; exit 1 }
    $expected = (Get-Content $Checksum -Raw).Split()[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $Dump).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        Write-Host "[FAIL] checksum mismatch for $Dump" -ForegroundColor Red
        Write-Host "  expected: $expected" -ForegroundColor Red
        Write-Host "  actual:   $actual" -ForegroundColor Red
        exit 1
    }
    Write-Host "restore: checksum ok $actual" -ForegroundColor Green
}

$pgRestore = "pg_restore"
if (Test-Path "C:\Program Files\PostgreSQL\18\bin\pg_restore.exe") { $pgRestore = "C:\Program Files\PostgreSQL\18\bin\pg_restore.exe" }

Write-Host "[$(Get-Date -Format o)] restore: starting dump=$Dump allow-destructive=$AllowDestructive"
& $pgRestore --list $Dump 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] dump is not valid custom-format: $Dump" -ForegroundColor Red; exit 1 }

$start = Get-Date
& $pgRestore --clean --if-exists --no-owner --no-acl --verbose --dbname=$env:SIGAP_RESTORE_DATABASE_URL $Dump 2>&1 | ForEach-Object { $_ -replace 'postgresql://[^ ]*', 'postgresql://***REDACTED***' } | Write-Host
if ($LASTEXITCODE -ne 0) { Write-Host "restore: pg_restore returned non-zero" -ForegroundColor Red; exit 1 }

$psql = "psql"
if (Test-Path "C:\Program Files\PostgreSQL\18\bin\psql.exe") { $psql = "C:\Program Files\PostgreSQL\18\bin\psql.exe" }

Write-Host "restore: verifying restored DB"
$queries = @(
    "SELECT count(*) FROM schema_migrations",
    "SELECT count(*) FROM facilities",
    "SELECT count(*) FROM service_units",
    "SELECT count(*) FROM appointments"
)

foreach ($q in $queries) {
    $out = & $psql $env:SIGAP_RESTORE_DATABASE_URL -v ON_ERROR_STOP=1 -t -A -c $q 2>&1
    if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] verification failed: $q`n$out" -ForegroundColor Red; exit 1 }
    $val = ($out | Out-String).Trim()
    Write-Host "  $q -> $val"
}

$duration = [int]((Get-Date) - $start).TotalSeconds
Write-Host "restore: verified duration=${duration}s" -ForegroundColor Green
Write-Host "restore: done" -ForegroundColor Green
