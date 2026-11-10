<#
.SYNOPSIS
    Run one SIGAP V2.2 preview service in the foreground.

.DESCRIPTION
    Internal helper for Start-V22Preview.ps1. It exists so the launcher can
    start each service through WMI `Win32_Process.Create`, which detaches the
    child from the caller's job object.

    Why that matters: `Start-Process` children inherit the parent's job object
    on Windows. When the launching shell exits — or when a supervisor reaps it
    — the job is closed and every process in it is killed. The servers start
    correctly, serve for a while, and then vanish with no error in their own
    logs. Running the service from a WMI-created process breaks that
    inheritance, so the preview survives the terminal that started it.

    Environment is read from .env here, in the child, so the database URL never
    appears on a command line (a command line is readable by any local user via
    the process table and Task Manager).

.PARAMETER Service
    api | web | engine

.PARAMETER AdminActor
    For `web`: the dev RBAC subject the admin screens act as. Passed on the
    command line because it is a seeded subject name, not a credential — the
    database URL, which IS a credential, is read from .env in this process.
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('api', 'web', 'engine')]
    [string]$Service,
    [string]$AdminActor = '',
    [switch]$Jwt
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path
$EnvFile = Join-Path $RepoRoot '.env'
$LogDir = Join-Path $RepoRoot '.local-preview'
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

$envValues = @{}
foreach ($line in Get-Content $EnvFile) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
    $idx = $trimmed.IndexOf('=')
    if ($idx -lt 1) { continue }
    $envValues[$trimmed.Substring(0, $idx).Trim()] =
        $trimmed.Substring($idx + 1).Trim().Trim('"').Trim("'")
}

$dbUrl = $envValues['DATABASE_URL']
if ([string]::IsNullOrWhiteSpace($dbUrl)) { throw "DATABASE_URL missing from .env" }

$ApiPort = 18080
$WebPort = 5173

switch ($Service) {
    'engine' {
        # The engine reads DATABASE_URL (not SIGAP_DATABASE_URL) and panics at
        # startup without it; it has no in-memory mode.
        $env:DATABASE_URL = $dbUrl
        & (Join-Path $RepoRoot 'apps\queue-engine\target\debug\sigap-queue-engine.exe') *>&1 |
            Out-File -FilePath (Join-Path $LogDir 'engine.log') -Encoding utf8
    }
    'api' {
        $env:SIGAP_DATABASE_URL = $dbUrl
        $env:SIGAP_API_PORT = "$ApiPort"
        $env:SIGAP_ENGINE_ADDR = '127.0.0.1:50051'
        $env:SIGAP_ENV = 'local'
        $env:SIGAP_AUTH_MODE = if ($Jwt) { 'jwt' } else { 'dev' }
        $env:SIGAP_AUTH_ISSUER = "http://127.0.0.1:54321/auth/v1"
        $env:SIGAP_AUTH_AUDIENCE = "authenticated"
        $env:SIGAP_AUTH_JWKS_URL = "http://127.0.0.1:54321/auth/v1/.well-known/jwks.json"
        $env:SIGAP_DEV_IDENTITY = if ($Jwt) { '' } else { 'true' }
        $env:SIGAP_LOCAL_RBAC_TEST_IDENTITY = if ($Jwt) { '' } else { 'true' }
        $env:SIGAP_ENGINE_FALLBACK = 'dev'
        $env:SIGAP_WEB_ORIGIN = "http://127.0.0.1:$WebPort"
        $env:SIGAP_API_INTERNAL = "http://127.0.0.1:$ApiPort"
        $env:SIGAP_API_BASE = "http://127.0.0.1:$ApiPort"
        $env:SIGAP_CORS_ORIGINS = "http://127.0.0.1:$WebPort,http://localhost:$WebPort"
        & (Join-Path $RepoRoot 'apps\api\bin\sigap-api.exe') *>&1 |
            Out-File -FilePath (Join-Path $LogDir 'api.log') -Encoding utf8
    }
    'web' {
        $env:SIGAP_API_INTERNAL = "http://127.0.0.1:$ApiPort"
        $env:SIGAP_API_BASE = "http://127.0.0.1:$ApiPort"
        $env:SIGAP_AUTH_MODE = 'dev'
        $env:SIGAP_DEV_IDENTITY = 'true'
        $env:SIGAP_ENV = 'local'
        if (-not [string]::IsNullOrWhiteSpace($AdminActor)) {
            $env:SIGAP_LOCAL_E2E_ACTOR = $AdminActor
        }
        Set-Location (Join-Path $RepoRoot 'apps\web')
        & npm.cmd run dev -- --host 127.0.0.1 --port $WebPort --strictPort *>&1 |
            Out-File -FilePath (Join-Path $LogDir 'web.log') -Encoding utf8
    }
}
