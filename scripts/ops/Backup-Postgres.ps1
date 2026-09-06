<#
.SYNOPSIS
    Guarded PowerShell wrapper for Sigap PostgreSQL backup (AUDIT-701).

.DESCRIPTION
    Calls scripts/ops/backup-postgres.sh via bash (WSL/Git Bash) or
    falls back to a native PowerShell pg_dump path when bash is absent.
    Prefers the bash script on Linux/VPS; this wrapper exists so Windows
    dev machines can also prove backup/restore without manual bash.

    Requires: DATABASE_URL
    Optional: SIGAP_BACKUP_DIR, SIGAP_BACKUP_S3_* / SIGAP_BACKUP_BUCKET etc.
    See docs/operations/BACKUP_RESTORE.md.

.EXAMPLE
    $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
    pwsh -NoProfile -File scripts/ops/Backup-Postgres.ps1

.EXAMPLE
    $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
    $env:SIGAP_BACKUP_DIR = "F:\backups\sigap"
    pwsh -NoProfile -File scripts/ops/Backup-Postgres.ps1
#>

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    Write-Host "[FAIL] DATABASE_URL is required." -ForegroundColor Red
    exit 1
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$sh = Join-Path $RepoRoot 'scripts\ops\backup-postgres.sh'

$bash = $null
# Only use bash if it is real MSYS/Git bash with the script's deps; WSL bash fails on Windows paths.
# Prefer native PowerShell path on Windows to avoid WSL path translation issues.
$gitBash = "C:\Program Files\Git\usr\bin\bash.exe"
if (Test-Path $gitBash) { $bash = Get-Item $gitBash }
if ($bash -and (Test-Path $sh)) {
    # Use MSYS bash explicitly so /bin/bash deps (sha256sum, pg_dump) resolve via Git's usr/bin
    & $gitBash $sh
    if ($LASTEXITCODE -eq 0) { exit 0 }
    Write-Host "backup: bash path failed ($LASTEXITCODE), falling back to native PowerShell" -ForegroundColor Yellow
}

# Native PowerShell fallback (custom-format pg_dump, checksum, validation).
$backupDir = if ([string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_DIR)) { Join-Path $RepoRoot 'backups\sigap' } else { $env:SIGAP_BACKUP_DIR }
$retentionDays = if ([string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_RETENTION_DAYS)) { 7 } else { [int]$env:SIGAP_BACKUP_RETENTION_DAYS }
$stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$dumpName = "sigap-$stamp.dump"
$dumpTmp = Join-Path $backupDir ".$dumpName.tmp"
$dumpFinal = Join-Path $backupDir $dumpName
$checksumFinal = "$dumpFinal.sha256"

New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
Write-Host "[$(Get-Date -Format o)] backup: starting (dir=$backupDir, retention=${retentionDays}d, stamp=$stamp)"

$start = Get-Date
$pgDump = "pg_dump"
if (Test-Path "C:\Program Files\PostgreSQL\18\bin\pg_dump.exe") { $pgDump = "C:\Program Files\PostgreSQL\18\bin\pg_dump.exe" }

# Atomic: dump to temp, validate, rename. Never log DATABASE_URL.
$dumpArgs = @("--format=custom", "--no-owner", "--no-acl", "--verbose", "--file=$dumpTmp", $env:DATABASE_URL)
& $pgDump @dumpArgs 2>&1 | ForEach-Object { $_ -replace 'postgresql://[^ ]*', 'postgresql://***REDACTED***' } | Write-Host
if ($LASTEXITCODE -ne 0) { Remove-Item -Force -ErrorAction SilentlyContinue $dumpTmp; Write-Host "[FAIL] pg_dump" -ForegroundColor Red; exit 1 }

if (-not (Test-Path $dumpTmp) -or (Get-Item $dumpTmp).Length -eq 0) {
    Remove-Item -Force -ErrorAction SilentlyContinue $dumpTmp
    Write-Host "[FAIL] dump empty" -ForegroundColor Red; exit 1
}

$pgRestore = "pg_restore"
if (Test-Path "C:\Program Files\PostgreSQL\18\bin\pg_restore.exe") { $pgRestore = "C:\Program Files\PostgreSQL\18\bin\pg_restore.exe" }
& $pgRestore --list $dumpTmp 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) { Remove-Item -Force -ErrorAction SilentlyContinue $dumpTmp; Write-Host "[FAIL] pg_restore --list" -ForegroundColor Red; exit 1 }

Move-Item -Force $dumpTmp $dumpFinal

# Checksum
try {
    $hash = (Get-FileHash -Algorithm SHA256 $dumpFinal).Hash.ToLowerInvariant()
    $line = "$hash  $dumpName"
    Set-Content -NoNewline -Path $checksumFinal -Value $line
    Write-Host "backup: checksum $line"
} catch {
    Write-Host "backup: WARN checksum skipped: $_" -ForegroundColor Yellow
}

$size = (Get-Item $dumpFinal).Length
$duration = [int]((Get-Date) - $start).TotalSeconds
Write-Host "backup: created $dumpFinal ($size bytes, ${duration}s)"

# Off-host upload (aws cli if configured)
if (-not [string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_BUCKET) -and -not [string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_ACCESS_KEY) -and -not [string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_SECRET_KEY)) {
    $endpoint = $env:SIGAP_BACKUP_S3_ENDPOINT
    $region = if ([string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_S3_REGION)) { "auto" } else { $env:SIGAP_BACKUP_S3_REGION }
    $aws = Get-Command aws -ErrorAction SilentlyContinue
    if ($aws) {
        $env:AWS_ACCESS_KEY_ID = $env:SIGAP_BACKUP_ACCESS_KEY
        $env:AWS_SECRET_ACCESS_KEY = $env:SIGAP_BACKUP_SECRET_KEY
        $env:AWS_DEFAULT_REGION = $region
        $args = @("s3", "cp", $dumpFinal, "s3://$env:SIGAP_BACKUP_BUCKET/$dumpName", "--region", $region)
        if (-not [string]::IsNullOrWhiteSpace($endpoint)) { $args += @("--endpoint-url", $endpoint) }
        & aws @args 2>&1 | Write-Host
        if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] aws s3 cp dump" -ForegroundColor Red; exit 1 }
        if (Test-Path $checksumFinal) {
            $args2 = @("s3", "cp", $checksumFinal, "s3://$env:SIGAP_BACKUP_BUCKET/$dumpName.sha256", "--region", $region)
            if (-not [string]::IsNullOrWhiteSpace($endpoint)) { $args2 += @("--endpoint-url", $endpoint) }
            & aws @args2 2>&1 | Write-Host
            if ($LASTEXITCODE -ne 0) { Write-Host "[FAIL] aws s3 cp checksum" -ForegroundColor Red; exit 1 }
        }
        Remove-Item Env:\AWS_ACCESS_KEY_ID -ErrorAction SilentlyContinue
        Remove-Item Env:\AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue
        Write-Host "backup: uploaded to s3://$env:SIGAP_BACKUP_BUCKET/$dumpName"
    } else {
        Write-Host "[FAIL] SIGAP_BACKUP_BUCKET set but 'aws' not available" -ForegroundColor Red; exit 1
    }
} elseif (-not [string]::IsNullOrWhiteSpace($env:SIGAP_BACKUP_BUCKET)) {
    Write-Host "backup: WARN SIGAP_BACKUP_BUCKET set but ACCESS_KEY/SECRET_KEY missing — skipping upload (local-only, audit PARTIAL)" -ForegroundColor Yellow
} else {
    Write-Host "backup: no S3 bucket configured — local-only backup (audit PARTIAL until off-host storage is configured; see BACKUP_RESTORE.md)" -ForegroundColor Yellow
}

# Retention
if ($retentionDays -gt 0) {
    $cutoff = (Get-Date).AddDays(-$retentionDays)
    $old = Get-ChildItem -Path $backupDir -File -Filter "sigap-*.dump*" -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -lt $cutoff }
    foreach ($f in $old) { Remove-Item -Force $f.FullName; Write-Host "backup: retention pruned $($f.Name)" }
}

Write-Host "backup: done" -ForegroundColor Green
