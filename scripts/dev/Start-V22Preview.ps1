<#
.SYNOPSIS
    Start (or restart) the SIGAP V2.2 local preview stack.

.DESCRIPTION
    Brings up the three host-run processes the preview needs, in dependency
    order, and prints the URL to open:

      1. Go API       http://127.0.0.1:18080   (SIGAP_API_PORT)
      2. SvelteKit    http://127.0.0.1:5173    (SIGAP_API_INTERNAL -> the API)
      3. Rust engine  127.0.0.1:50051          (started only if not listening)

    PostgreSQL is NOT started here: it runs as the `sigap-postgres` container
    on port 5433. The script verifies it is reachable and refuses to start the
    API against a database it cannot see, because an API with a dead pool
    answers /health but every real read fails.

    Why port 18080 and not 8080
    ---------------------------
    127.0.0.1:8080 is held by `wslrelay.exe` on this machine (the Windows
    IP Helper / WSL relay). A process bound to 0.0.0.0:8080 still answers on
    the LAN address, but every request to 127.0.0.1:8080 reaches wslrelay
    instead, which answers 404 for every path. That looks exactly like a
    broken API and is why this script pins 18080.

    Secrets
    -------
    DATABASE_URL is read from .env and handed to the child process through its
    environment. It is never echoed, never written to a log, and never placed
    on a command line (a command line is visible to every user via Task
    Manager and to the process table).

.PARAMETER Stop
    Stop the preview processes this script started, then exit.

.PARAMETER SkipEngine
    Do not start the Rust queue engine even if it is not listening. The API
    falls back to its in-memory dev queue (SIGAP_ENGINE_FALLBACK=dev).

.PARAMETER AdminActor
    The dev RBAC subject the admin screens act as. Default
    `local-global-super-admin`, which sees every facility and therefore renders
    populated screens for review. Other seeded subjects: `local-facility-admin`
    (facility_admin at the demo facility — sees only that facility's rows, and
    that facility currently has no queue tickets), `local-zero-scope-admin`
    (sees nothing — useful for checking the empty-scope state), and the
    `e2e-schedule-*` pair whose grants deliberately differ per facility.

    This sets SIGAP_LOCAL_E2E_ACTOR on the SvelteKit process, which is the
    documented local selector. It is not a bypass: the Go API resolves the
    subject's roles and facility grants from the database on every request, and
    refuses to arm the selector at all unless SIGAP_ENV=local.

.PARAMETER NoAdminActor
    Do not set SIGAP_LOCAL_E2E_ACTOR. Admin screens then require a real
    Supabase session and will show their unauthenticated state.

.EXAMPLE
    pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Stop
#>

[CmdletBinding()]
param(
    [switch]$Stop,
    [switch]$SkipEngine,
    [switch]$Jwt,
    [string]$AdminActor = 'local-global-super-admin',
    [switch]$NoAdminActor
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path
$EnvFile  = Join-Path $RepoRoot '.env'
$ApiExe   = Join-Path $RepoRoot 'apps\api\bin\sigap-api.exe'
$WebDir   = Join-Path $RepoRoot 'apps\web'
$EngineExe = Join-Path $RepoRoot 'apps\queue-engine\target\debug\sigap-queue-engine.exe'

$ApiPort    = 18080
$WebPort    = 5173
$EnginePort = 50051
$PgPort     = 5433

# Child output goes to files rather than being inherited. An inherited handle
# keeps the parent's stdout pipe open for as long as the server runs, so
# anything that waits for that pipe to close (a shell, a CI step, this script's
# caller) waits forever — the servers start correctly and the command still
# looks hung.
$LogDir    = Join-Path $RepoRoot '.local-preview'
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$ApiLog    = Join-Path $LogDir 'api.log'
$ApiErr    = Join-Path $LogDir 'api.err.log'
$WebLog    = Join-Path $LogDir 'web.log'
$WebErr    = Join-Path $LogDir 'web.err.log'
$EngineLog = Join-Path $LogDir 'engine.log'
$EngineErr = Join-Path $LogDir 'engine.err.log'

function Test-Port {
    param([int]$Port, [string]$HostName = '127.0.0.1')
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $iar = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $iar.AsyncWaitHandle.WaitOne(700)) { return $false }
        $client.EndConnect($iar)
        return $true
    } catch {
        return $false
    } finally {
        $client.Dispose()
    }
}

function Stop-OurProcess {
    param([string]$ExePath)
    $full = [System.IO.Path]::GetFullPath($ExePath)
    Get-Process -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -eq $full } |
        ForEach-Object {
            Write-Host "  stopping PID $($_.Id) $($_.ProcessName)" -ForegroundColor DarkYellow
            Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
        }
}

<#
    Start a service so it OUTLIVES this script.

    `Start-Process` children inherit the caller's job object on Windows, and
    closing that job kills every process in it. That is why an earlier version
    of this script produced servers that started, passed their health checks,
    and then disappeared the moment the launching shell exited — with nothing
    in their own logs, because they were never asked to stop.

    `Win32_Process.Create` goes through WMI, so the child is parented to
    WmiPrvSE instead of to this shell and is not in this shell's job. The
    process id returned is the real service process.
#>
function Start-DetachedService {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string]$AdminActorArg = ''
    )
    $runner = Join-Path $PSScriptRoot 'Run-V22Service.ps1'
    $argLine = "-NoProfile -ExecutionPolicy Bypass -File `"$runner`" -Service $Name"
    if ($AdminActorArg -ne '') {
        $argLine += " -AdminActor `"$AdminActorArg`""
    }
    $pwshPath = (Get-Process -Id $PID).Path
    $cmd = "`"$pwshPath`" $argLine"

    $startup = ([wmiclass]'Win32_ProcessStartup').CreateInstance()
    $startup.ShowWindow = 0
    $result = ([wmiclass]'Win32_Process').Create($cmd, $null, $startup)
    if ($result.ReturnValue -ne 0) {
        throw "Win32_Process.Create failed for $Name (code $($result.ReturnValue))"
    }
    return [int]$result.ProcessId
}

# --- Stop mode -------------------------------------------------------------
if ($Stop) {
    Write-Host "Stopping SIGAP V2.2 preview processes..." -ForegroundColor Yellow
    Stop-OurProcess -ExePath $ApiExe
    Stop-OurProcess -ExePath $EngineExe
    # The web server is `node vite.js dev`; match on the repo path so an
    # unrelated vite project is never touched.
    Get-CimInstance Win32_Process -Filter "Name='node.exe'" |
        Where-Object { $_.CommandLine -and $_.CommandLine -like "*$WebDir*" } |
        ForEach-Object {
            Write-Host "  stopping PID $($_.ProcessId) vite" -ForegroundColor DarkYellow
            Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
        }
    # The detached runners are pwsh processes holding the service. Killing the
    # service binary alone would leave them waiting on a dead child.
    Get-CimInstance Win32_Process -Filter "Name='pwsh.exe'" |
        Where-Object { $_.CommandLine -and $_.CommandLine -like "*Run-V22Service.ps1*" } |
        ForEach-Object {
            Write-Host "  stopping PID $($_.ProcessId) runner" -ForegroundColor DarkYellow
            Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
        }
    Write-Host "Stopped." -ForegroundColor Green
    return
}

Write-Host ""
Write-Host "SIGAP V2.2 local preview" -ForegroundColor Cyan
Write-Host "========================" -ForegroundColor Cyan
Write-Host ""

# --- Preconditions ---------------------------------------------------------
if (-not (Test-Path $EnvFile))    { throw ".env not found at $EnvFile" }
if (-not (Test-Path $ApiExe))     { throw "API binary not built: $ApiExe (run: go build -o bin/sigap-api.exe ./cmd/server in apps/api)" }

# Read only the keys we need. Values are never printed.
$envValues = @{}
foreach ($line in Get-Content $EnvFile) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
    $idx = $trimmed.IndexOf('=')
    if ($idx -lt 1) { continue }
    $key = $trimmed.Substring(0, $idx).Trim()
    $val = $trimmed.Substring($idx + 1).Trim().Trim('"').Trim("'")
    $envValues[$key] = $val
}

if (-not $envValues.ContainsKey('DATABASE_URL')) {
    throw "DATABASE_URL is missing from .env"
}
$dbUrl = $envValues['DATABASE_URL']

# Refuse to point the dev stack at anything that is not the local 5433 dev DB.
if ($dbUrl -notmatch ':5433/') {
    throw "DATABASE_URL does not target port 5433 (the SIGAP dev database). " +
          "Refusing to start: 5434 is an unrelated Odoo instance."
}

if (-not (Test-Port -Port $PgPort)) {
    throw "PostgreSQL is not reachable on 127.0.0.1:$PgPort. " +
          "Start the container first: docker start sigap-postgres"
}
Write-Host "  [ok] PostgreSQL on $PgPort" -ForegroundColor Green

# --- Engine ----------------------------------------------------------------
if (Test-Port -Port $EnginePort) {
    Write-Host "  [ok] queue engine already listening on $EnginePort" -ForegroundColor Green
} elseif ($SkipEngine) {
    Write-Host "  [--] queue engine skipped; API will use the in-memory dev queue" -ForegroundColor DarkYellow
} elseif (Test-Path $EngineExe) {
    Write-Host "  [..] starting queue engine on $EnginePort" -ForegroundColor Yellow
    # The engine reads DATABASE_URL (not SIGAP_DATABASE_URL) and PANICS at
    # startup without it — it has no in-memory mode. It also binds
    # 0.0.0.0:50051 unconditionally, so the port is not configurable.
    Start-DetachedService -Name 'engine' | Out-Null
    $deadline = (Get-Date).AddSeconds(25)
    while (-not (Test-Port -Port $EnginePort) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 400 }
    if (Test-Port -Port $EnginePort) {
        Write-Host "  [ok] queue engine listening" -ForegroundColor Green
    } else {
        throw "Queue engine did not bind $EnginePort. See $EngineErr"
    }
} else {
    Write-Host "  [!!] engine binary not built; API will fall back to the dev queue" -ForegroundColor DarkYellow
}

# --- API -------------------------------------------------------------------
Stop-OurProcess -ExePath $ApiExe
Start-Sleep -Milliseconds 500

if (Test-Port -Port $ApiPort) {
    throw "Port $ApiPort is already in use by another process. Stop it first."
}

$apiEnv = @{
    SIGAP_DATABASE_URL   = $dbUrl
    SIGAP_API_PORT       = "$ApiPort"
    SIGAP_ENGINE_ADDR    = "127.0.0.1:$EnginePort"
    SIGAP_ENV            = 'local'
    SIGAP_AUTH_MODE      = if ($Jwt) { 'jwt' } else { 'dev' }
    SIGAP_AUTH_ISSUER    = "http://127.0.0.1:54321/auth/v1"
    SIGAP_AUTH_AUDIENCE  = "authenticated"
    SIGAP_AUTH_JWKS_URL  = "http://127.0.0.1:54321/auth/v1/.well-known/jwks.json"
    SIGAP_DEV_IDENTITY   = if ($Jwt) { '' } else { 'true' }
    # The local RBAC test-identity selector. Without this the API ignores
    # X-Sigap-Local-Test-Subject entirely and every admin read answers 403,
    # because the web tier's proxyHeaders() has no other way to name an actor.
    # It is inert unless SIGAP_ENV=local, and config.GuardDevCapabilities
    # refuses to let the process start if this flag is set anywhere else.
    SIGAP_LOCAL_RBAC_TEST_IDENTITY = if ($Jwt) { '' } else { 'true' }
    SIGAP_ENGINE_FALLBACK = 'dev'
    SIGAP_WEB_ORIGIN     = "http://127.0.0.1:$WebPort"
    SIGAP_API_INTERNAL   = "http://127.0.0.1:$ApiPort"
    SIGAP_API_BASE       = "http://127.0.0.1:$ApiPort"
    SIGAP_CORS_ORIGINS   = "http://127.0.0.1:$WebPort,http://localhost:$WebPort"
}

# The service runner reads DATABASE_URL from .env in its own process, so the
# credential never appears on a command line.
Write-Host "  [..] starting API on $ApiPort" -ForegroundColor Yellow
Start-DetachedService -Name 'api' | Out-Null

$deadline = (Get-Date).AddSeconds(30)
while (-not (Test-Port -Port $ApiPort) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 400 }
if (-not (Test-Port -Port $ApiPort)) { throw "API did not bind $ApiPort within 30s" }

# Readiness, not just liveness: /health is a constant and says nothing about
# the database or the engine. Invoke-RestMethod throws on a 503, so the failure
# body has to be read explicitly — otherwise the diagnostic is `null`, which is
# the one thing that cannot be acted on.
$ready = $null
$readyDetail = 'no response'
for ($i = 0; $i -lt 30; $i++) {
    try {
        $ready = Invoke-RestMethod -Uri "http://127.0.0.1:$ApiPort/readyz" -TimeoutSec 5
        $readyDetail = $ready.status
        break
    } catch {
        $resp = $_.Exception.Response
        if ($null -ne $resp) {
            try {
                $reader = [System.IO.StreamReader]::new($resp.GetResponseStream())
                $readyDetail = $reader.ReadToEnd()
                $reader.Dispose()
            } catch { $readyDetail = $_.Exception.Message }
        } else {
            $readyDetail = $_.Exception.Message
        }
        Start-Sleep -Milliseconds 500
    }
}
if ($null -eq $ready -or $ready.status -ne 'ready') {
    throw "API is up but not ready: $readyDetail (see $ApiErr)"
}
Write-Host "  [ok] API ready (audit=$($ready.audit))" -ForegroundColor Green

# --- Web -------------------------------------------------------------------
# Restart, do not reuse. A vite that was already running holds the old
# environment, and `--strictPort` makes the new one fail to bind rather than
# steal the port — so the stale server would silently keep serving with a
# missing SIGAP_API_INTERNAL and every /api/v1 proxy would answer 404.
Get-CimInstance Win32_Process -Filter "Name='node.exe'" |
    Where-Object { $_.CommandLine -and $_.CommandLine -like "*$WebDir*" } |
    ForEach-Object {
        Write-Host "  [..] stopping stale vite PID $($_.ProcessId)" -ForegroundColor DarkYellow
        Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
    }
Start-Sleep -Milliseconds 600

if (Test-Port -Port $WebPort) {
    throw "Port $WebPort is still in use after stopping the repo's vite. Stop that process first."
}

Write-Host "  [..] starting SvelteKit on $WebPort" -ForegroundColor Yellow
$actorForWeb = if ($NoAdminActor) { '' } else { $AdminActor }
Start-DetachedService -Name 'web' -AdminActorArg $actorForWeb | Out-Null

$deadline = (Get-Date).AddSeconds(60)
while (-not (Test-Port -Port $WebPort) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 500 }
if (-not (Test-Port -Port $WebPort)) { throw "SvelteKit did not bind $WebPort within 60s" }
Write-Host "  [ok] SvelteKit listening" -ForegroundColor Green

# --- End-to-end proof ------------------------------------------------------
# A port that answers is not a working preview. Prove the browser path:
# SvelteKit proxy -> Go API -> PostgreSQL, using a real seeded check-in code.
Write-Host ""
Write-Host "Verifying the browser path (SvelteKit -> API -> PostgreSQL)..." -ForegroundColor Cyan

try {
    $facilities = Invoke-RestMethod -Uri "http://127.0.0.1:$WebPort/api/v1/public/facilities" -TimeoutSec 15
    $count = @($facilities.data).Count
    if ($count -lt 1) { throw "facilities returned no rows" }
    Write-Host "  [ok] /api/v1/public/facilities -> $count facilities" -ForegroundColor Green
} catch {
    Write-Host "  [!!] facilities proxy failed: $($_.Exception.Message)" -ForegroundColor Red
}

try {
    $status = Invoke-RestMethod -Uri "http://127.0.0.1:$WebPort/api/v1/patient/status?code=K3M9PQ" -TimeoutSec 15
    if (-not $status.data) { throw "no data for K3M9PQ" }
    Write-Host "  [ok] /api/v1/patient/status -> $($status.data.facility_name)" -ForegroundColor Green
} catch {
    Write-Host "  [!!] status proxy failed: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "=================================================" -ForegroundColor Green
Write-Host " SIGAP V2.2 preview: http://127.0.0.1:$WebPort" -ForegroundColor Green
Write-Host " API:                http://127.0.0.1:$ApiPort" -ForegroundColor Green
Write-Host " Admin sign-in:      /auth/login (SIGAP_AUTH_MODE=dev)" -ForegroundColor Green
Write-Host "=================================================" -ForegroundColor Green
Write-Host ""
