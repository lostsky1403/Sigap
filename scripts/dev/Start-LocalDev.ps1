<#
.SYNOPSIS
    One-command local dev bootstrap for bare-metal Windows development.

.DESCRIPTION
    Sets all required environment variables for a reliable bare-metal Windows
    development session and starts the queue engine, API, and web frontend in
    separate terminal windows.

    Fixes two known Windows/WSL port-conflict problems:
      1. Port 8080 is often occupied by wslrelay or the Windows IP Helper
         service (iphlpsvc). This script uses port 18080 as the local API
         port so bare-metal dev never conflicts with those system services.
      2. "localhost" resolves to ::1 (IPv6) on many Windows configurations,
         but the Rust queue engine binds 0.0.0.0:50051 (IPv4 only). This
         script forces SIGAP_ENGINE_ADDR=127.0.0.1:50051 to guarantee the
         Go API reaches the engine over IPv4 loopback.

    Docker, docker-compose, and CI are completely unaffected — this script
    only sets process-scoped environment variables in the current shell and
    child processes it spawns.

    Environment variables set:
      SIGAP_API_PORT=18080
      SIGAP_ENGINE_ADDR=127.0.0.1:50051
      SIGAP_AUTH_MODE=dev
      SIGAP_DEV_IDENTITY=true
      SIGAP_ENGINE_FALLBACK=dev
      SIGAP_WEB_ORIGIN=http://localhost:5173
      SIGAP_API_INTERNAL=http://127.0.0.1:18080
      SIGAP_API_BASE=http://127.0.0.1:18080
      SIGAP_DATABASE_URL=<value of DATABASE_URL>

    DATABASE_URL must already be set in the calling shell. The script never
    reads or writes .env files, and never commits or logs any secret value.

.PARAMETER NoStart
    Configure environment only; do not launch engine, API, or web processes.
    Use this when you want to start services manually in separate terminals.

.PARAMETER SkipEngine
    Skip launching the Rust queue engine. The API will fall back to the
    in-memory dev queue (SIGAP_ENGINE_FALLBACK=dev is always set).

.EXAMPLE
    # Full startup (engine + API + web) in separate windows:
    $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
    pwsh -NoProfile -File scripts/dev/Start-LocalDev.ps1

.EXAMPLE
    # Configure env only — start processes manually:
    $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
    . scripts/dev/Start-LocalDev.ps1 -NoStart

.NOTES
    Requires PowerShell 7+.
    Do not use in Docker, CI, or production environments.
    Dev identity (SIGAP_AUTH_MODE=dev, SIGAP_DEV_IDENTITY=true) is local-only.
    Never commit DATABASE_URL or any credential to version control.
#>

[CmdletBinding()]
param(
    [switch]$NoStart,
    [switch]$SkipEngine
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path

Write-Host ""
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host " Sigap local dev bootstrap (bare-metal Windows)" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host ""

if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL)) {
    Write-Host "[FAIL] DATABASE_URL is not set." -ForegroundColor Red
    Write-Host ""
    Write-Host "  Set it before running this script:" -ForegroundColor Yellow
    Write-Host '  $env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"' -ForegroundColor Yellow
    Write-Host ""
    exit 1
}

$env:SIGAP_DATABASE_URL   = $env:DATABASE_URL
$env:SIGAP_API_PORT        = '18080'
$env:SIGAP_ENGINE_ADDR     = '127.0.0.1:50051'
$env:SIGAP_AUTH_MODE       = 'dev'
$env:SIGAP_DEV_IDENTITY    = 'true'
$env:SIGAP_ENGINE_FALLBACK = 'dev'
$env:SIGAP_WEB_ORIGIN      = 'http://localhost:5173'
$env:SIGAP_API_INTERNAL    = 'http://127.0.0.1:18080'
$env:SIGAP_API_BASE        = 'http://127.0.0.1:18080'

Write-Host "Environment configured:" -ForegroundColor Green
Write-Host "  SIGAP_API_PORT        = $env:SIGAP_API_PORT"
Write-Host "  SIGAP_ENGINE_ADDR     = $env:SIGAP_ENGINE_ADDR"
Write-Host "  SIGAP_AUTH_MODE       = $env:SIGAP_AUTH_MODE"
Write-Host "  SIGAP_DEV_IDENTITY    = $env:SIGAP_DEV_IDENTITY"
Write-Host "  SIGAP_ENGINE_FALLBACK = $env:SIGAP_ENGINE_FALLBACK"
Write-Host "  SIGAP_WEB_ORIGIN      = $env:SIGAP_WEB_ORIGIN"
Write-Host "  SIGAP_API_INTERNAL    = $env:SIGAP_API_INTERNAL"
Write-Host "  SIGAP_API_BASE        = $env:SIGAP_API_BASE  <-- smoke scripts will use this"
Write-Host "  SIGAP_DATABASE_URL    = (set from DATABASE_URL)"
Write-Host ""
Write-Host "  API endpoint: http://127.0.0.1:18080" -ForegroundColor Cyan
Write-Host "  Web frontend: http://localhost:5173" -ForegroundColor Cyan
Write-Host ""

if ($NoStart) {
    Write-Host "(-NoStart) Environment set. Start services manually:" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  # Terminal 1 — queue engine:"
    Write-Host "  cd $RepoRoot\apps\queue-engine"
    Write-Host "  cargo run"
    Write-Host ""
    Write-Host "  # Terminal 2 — Go API:"
    Write-Host "  cd $RepoRoot\apps\api"
    Write-Host "  go run ./cmd/server"
    Write-Host ""
    Write-Host "  # Terminal 3 — SvelteKit web (inherits env from this shell):"
    Write-Host "  cd $RepoRoot\apps\web"
    Write-Host "  pnpm dev"
    Write-Host ""
    exit 0
}

$EngineDir = Join-Path $RepoRoot 'apps\queue-engine'
$ApiDir    = Join-Path $RepoRoot 'apps\api'
$WebDir    = Join-Path $RepoRoot 'apps\web'

if (-not $SkipEngine) {
    Write-Host "Starting queue engine in new window..." -ForegroundColor Yellow
    Start-Process pwsh -ArgumentList @(
        '-NoProfile', '-NoExit', '-Command',
        "Set-Location '$EngineDir'; Write-Host 'Queue engine — 0.0.0.0:50051' -ForegroundColor Cyan; cargo run"
    ) -WindowStyle Normal
}

Write-Host "Starting Go API in new window (port 18080)..." -ForegroundColor Yellow
$apiEnvBlock = @"
`$env:SIGAP_DATABASE_URL   = '$env:SIGAP_DATABASE_URL'
`$env:SIGAP_API_PORT        = '$env:SIGAP_API_PORT'
`$env:SIGAP_ENGINE_ADDR     = '$env:SIGAP_ENGINE_ADDR'
`$env:SIGAP_AUTH_MODE       = '$env:SIGAP_AUTH_MODE'
`$env:SIGAP_DEV_IDENTITY    = '$env:SIGAP_DEV_IDENTITY'
`$env:SIGAP_ENGINE_FALLBACK = '$env:SIGAP_ENGINE_FALLBACK'
`$env:SIGAP_WEB_ORIGIN      = '$env:SIGAP_WEB_ORIGIN'
Set-Location '$ApiDir'
Write-Host 'Go API — 127.0.0.1:18080' -ForegroundColor Cyan
go run ./cmd/server
"@
Start-Process pwsh -ArgumentList @('-NoProfile', '-NoExit', '-Command', $apiEnvBlock) -WindowStyle Normal

Write-Host "Starting SvelteKit web in new window..." -ForegroundColor Yellow
$webEnvBlock = @"
`$env:SIGAP_API_INTERNAL = '$env:SIGAP_API_INTERNAL'
`$env:SIGAP_API_BASE     = '$env:SIGAP_API_BASE'
Set-Location '$WebDir'
Write-Host 'SvelteKit web — http://localhost:5173 (proxies API to http://127.0.0.1:18080)' -ForegroundColor Cyan
pnpm dev
"@
Start-Process pwsh -ArgumentList @('-NoProfile', '-NoExit', '-Command', $webEnvBlock) -WindowStyle Normal

Write-Host ""
Write-Host "=================================================" -ForegroundColor Green
Write-Host " Three windows started. Wait ~5s for services." -ForegroundColor Green
Write-Host ""
Write-Host " Health check:  curl http://127.0.0.1:18080/health" -ForegroundColor Cyan
Write-Host " Readiness:     curl http://127.0.0.1:18080/readyz" -ForegroundColor Cyan
Write-Host " Demo smoke:    pwsh -NoProfile -File scripts/smoke/sigap-demo-smoke.ps1" -ForegroundColor Cyan
Write-Host " Full demo:     pwsh -NoProfile -File scripts/smoke/sigap-full-local-demo.ps1" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Green
Write-Host ""
