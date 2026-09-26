<#
.SYNOPSIS
    Deterministic local seeded-stack runner for the Phase 3B1 GATE 2 Playwright suite.

.DESCRIPTION
    Brings up everything the baseline E2E smoke needs, in the one order that
    works, then tears it down. Exists because the naive sequence fails in three
    separate ways that all surface as the same confusing error:

      1. Address family. On Windows `localhost` resolves to the IPv6 loopback
         ::1, so a bare `vite preview` binds ::1 only and every request to
         127.0.0.1:4173 fails with ERR_CONNECTION_REFUSED. The preview host is
         pinned to 127.0.0.1 to match the Playwright baseURL.
      2. Build ordering. `pnpm --filter sigap-web test` runs `vite build`, which
         replaces the hashed assets under .svelte-kit/output that a running
         preview is streaming. The preview then dies with ENOENT on a stale
         asset hash. So the build runs BEFORE the preview starts, never during.
      3. A dead backend. The page-load smoke tests pass against a static shell
         even when the Go API is off, which is not evidence the stack is real.
         This script starts the API and the database, and the smoke suite has a
         backend-backed scenario that fails without them.

    The database is a DISPOSABLE cluster created with initdb under .local-e2e/,
    bound to a free loopback port. It is intentionally separate from any Postgres
    instance already on the machine: this repo's .env.example contains only
    placeholder credentials, and on this host port 5434 is an unrelated Odoo
    business database that must not be touched. The cluster uses trust auth on
    loopback and holds only synthetic demo data, so no secret is involved.

.PARAMETER KeepRunning
    Leave the API and database running after the suite instead of stopping them.
    Useful when iterating on the smoke tests by hand.

.PARAMETER DatabasePort
    Loopback port for the disposable cluster. Defaults to the first free port at
    or above 55433.

.PARAMETER ApiPort
    Port for the Go API. Defaults to 18080, matching Start-LocalDev.ps1.

.PARAMETER WebPort
    Port for the web preview. Defaults to 4173, matching playwright.config.ts.

.EXAMPLE
    pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1

.EXAMPLE
    # Iterate on the smoke suite with the stack left up.
    pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1 -KeepRunning
    # ... then in another shell:
    pnpm --filter sigap-web exec playwright test e2e/smoke.spec.ts

.NOTES
    Requires PowerShell 7+, Go, Node/pnpm, and a local PostgreSQL install
    (initdb/pg_ctl/createdb/psql on PATH or under Program Files).
    Never contacts production. The dev identity flags used here are gated by
    SIGAP_ENV=local in apps/api/internal/config/envguard.go and will refuse to
    start anywhere else.
#>

[CmdletBinding()]
param(
    [switch]$KeepRunning,
    [int]$DatabasePort = 55433,
    [int]$ApiPort = 18080,
    [int]$WebPort = 4173
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..')).Path
$WorkDir = Join-Path $RepoRoot '.local-e2e'
$DataDir = Join-Path $WorkDir 'pgdata'
$DbName = 'sigap_e2e'
$DbUser = 'sigap'
$DbPassword = 'sigap'

function Write-Step([string]$Message) {
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Fail([string]$Message) {
    Write-Host "[FAIL] $Message" -ForegroundColor Red
    exit 1
}

<#
    Runs a native executable to completion and returns its exit code.

    WHY THIS EXISTS — do not "simplify" these call sites back to `& $tool ... | Out-Null`.

    On this host a native child process whose stdout is consumed by a PowerShell
    pipeline (or redirected to a file) can block forever: the child fills the
    pipe buffer, PowerShell blocks reading it, and the script stops making
    progress with no output and no error. initdb alone reproduced it three times
    here, which presents as "the script hangs on step 1" while the actual cause
    is the pipe. The same shape bit Start-Process -RedirectStandardOutput.

    Two properties keep it deterministic:
      1. stdout and stderr are redirected to real files, so the child always has
         somewhere to drain and can never block on a full buffer.
      2. the parent waits with a bounded timeout, so a genuinely stuck child
         fails loudly with a diagnosable message instead of hanging the gate.

    Arguments are pre-quoted by Quote-Arg. Start-Process rejoins -ArgumentList
    with plain spaces and does NOT add quotes, so an unquoted SQL string such as
    "select count(*) from facilities" reaches the child as four separate argv
    tokens. psql then silently runs only "select" and prints an empty result with
    exit code 0 — a failure that looks exactly like an empty table. That is why
    the seed-count check has to go through this helper.
#>
function Quote-Arg([string]$Value) {
    if ($Value -match '[\s"]') { return '"' + ($Value -replace '"', '\"') + '"' }
    return $Value
}

function Invoke-Native {
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [string[]]$Arguments = @(),
        [int]$TimeoutSeconds = 300,
        [string]$Label = $FilePath,
        [string]$WorkingDirectory
    )

    if (-not $WorkingDirectory) { $WorkingDirectory = $WorkDir }
    $stamp = [Guid]::NewGuid().ToString('N')
    $outFile = Join-Path $WorkDir "$stamp.out"
    $errFile = Join-Path $WorkDir "$stamp.err"
    $quoted = ($Arguments | ForEach-Object { Quote-Arg $_ }) -join ' '

    $proc = Start-Process -FilePath $FilePath -ArgumentList $quoted `
        -WorkingDirectory $WorkingDirectory -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $outFile -RedirectStandardError $errFile

    if (-not $proc.WaitForExit($TimeoutSeconds * 1000)) {
        try { $proc.Kill() } catch { }
        throw "$Label did not finish within $TimeoutSeconds s. stderr: $(Get-Content $errFile -Raw -ErrorAction SilentlyContinue)"
    }

    $code = $proc.ExitCode
    if ($code -ne 0) {
        $tail = (Get-Content $errFile -ErrorAction SilentlyContinue |
            Select-Object -Last 15) -join "`n"
        $tailOut = (Get-Content $outFile -ErrorAction SilentlyContinue |
            Select-Object -Last 15) -join "`n"
        throw "$Label failed (exit $code).`n--- stderr ---`n$tail`n--- stdout ---`n$tailOut"
    }

    # stdout is read AFTER the process exits, so nothing here can block a child.
    $stdout = (Get-Content $outFile -Raw -ErrorAction SilentlyContinue) ?? ''
    Remove-Item $outFile, $errFile -Force -ErrorAction SilentlyContinue
    return $stdout
}

# --- Locate PostgreSQL binaries -------------------------------------------------
function Resolve-PgTool([string]$Name) {
    $onPath = Get-Command "$Name.exe" -ErrorAction SilentlyContinue
    if ($onPath) { return $onPath.Source }
    # Sort by VERSION, not by Name. A plain name sort puts "psqlODBC" (a driver
    # directory that ships none of these tools) ahead of the real install
    # "18", and the tool lookup then fails even though Postgres is installed.
    $candidates = Get-ChildItem 'C:\Program Files\PostgreSQL' -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^\d+(\.\d+)*$' } |
        Sort-Object { [version]$_.Name } -Descending
    foreach ($dir in $candidates) {
        $probe = Join-Path $dir.FullName "bin\$Name.exe"
        if (Test-Path $probe) { return $probe }
    }
    throw "PostgreSQL tool '$Name' not found on PATH or under C:\Program Files\PostgreSQL."
}

$initdb  = Resolve-PgTool 'initdb'
$pgctl   = Resolve-PgTool 'pg_ctl'
$createdb = Resolve-PgTool 'createdb'
$psql    = Resolve-PgTool 'psql'

# `pnpm` on Windows resolves to pnpm.ps1/pnpm.cmd, neither of which Start-Process
# can launch directly: with redirection PowerShell forces UseShellExecute=false,
# which refuses .cmd ("%1 is not a valid Win32 application"). Going through the
# PowerShell host is the supported way to run those shims, and the reason the
# build and Playwright steps below invoke pwsh rather than pnpm.
$pwshExe = (Get-Command 'pwsh' -ErrorAction SilentlyContinue)?.Source
if (-not $pwshExe) {
    $pwshExe = Join-Path $PSHOME 'pwsh.exe'
}

# --- Port selection -------------------------------------------------------------
function Get-FreePort([int]$Preferred) {
    $port = $Preferred
    while (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { $port++ }
    return $port
}

# The cluster is persistent, so its port has to be too. Re-probing a free port on
# every run means a leftover postmaster from a killed run keeps the old port,
# pg_ctl then refuses to start against a different port with a stale pid file
# ("another server might be running"), and the run fails for a reason that has
# nothing to do with the code under test. Persisting the port keeps the cluster
# and the port that owns it paired.
$PortFile = Join-Path $WorkDir 'port'
if ((Test-Path $PortFile) -and (Test-Path (Join-Path $DataDir 'PG_VERSION'))) {
    $recorded = (Get-Content $PortFile -Raw -ErrorAction SilentlyContinue).Trim()
    if ($recorded -match '^\d+$') { $DbPort = [int]$recorded }
} else {
    $DbPort = Get-FreePort $DatabasePort
}
$WebPort = Get-FreePort $WebPort
$ApiPort = Get-FreePort $ApiPort

Write-Host ""
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host " Sigap local seeded stack (GATE 2 E2E)" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host "  web        http://127.0.0.1:$WebPort"
Write-Host "  Go API     http://127.0.0.1:$ApiPort"
Write-Host "  Postgres   postgresql://$DbUser@127.0.0.1:$DbPort/$DbName (disposable)"
Write-Host "  SIGAP_ENV  local"
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host ""

$apiProcess = $null
$webProcess = $null

# NOTE on process startup: the long-running children (the Go API and the Vite
# preview) are started WITHOUT stream redirection on purpose. They never exit, so
# a redirect handle would stay open for the life of the run and the launching
# shell can block on it indefinitely, presenting as a script that hangs with no
# output. Their readiness is verified over HTTP instead, and a failure message
# names the exact command to run by hand to see the real log. Short-lived tools
# (initdb, psql, go build) DO go through Invoke-Native, which redirects to real
# files and waits with a timeout, so they can neither block nor hang.

try {
    # --- 1. Disposable database cluster -----------------------------------------
    Write-Step "Preparing disposable Postgres cluster on 127.0.0.1:$DbPort"
    New-Item -ItemType Directory -Force -Path $WorkDir | Out-Null

    if (-not (Test-Path (Join-Path $DataDir 'PG_VERSION'))) {
        # trust auth on loopback only: no secret is created, and the cluster is
        # disposable demo data, not a real store.
        Invoke-Native -FilePath $initdb -Label 'initdb' -TimeoutSeconds 180 `
            -Arguments @('-D', $DataDir, '-U', $DbUser, '--auth=trust', '--encoding=UTF8', '--no-sync') | Out-Null
    }

    # Probe the port instead of trusting `pg_ctl status`. On Windows that check
    # reports "another server might be running" even for a cluster this script
    # started moments earlier, because the pid file check races the postmaster.
    # A completed TCP connect is unambiguous.
    $dbListening = $false
    try {
        $probe = [System.Net.Sockets.TcpClient]::new()
        $probe.Connect('127.0.0.1', $DbPort)
        $dbListening = $probe.Connected
        $probe.Close()
    } catch { $dbListening = $false }

    if (-not $dbListening) {
        # A postmaster.pid left behind by a killed run makes pg_ctl refuse to
        # start ("another server might be running") even though nothing is
        # listening. Nothing is listening — that was just proven by the probe
        # above — so the pid file is definitionally stale and safe to clear.
        $pidFile = Join-Path $DataDir 'postmaster.pid'
        if (Test-Path $pidFile) {
            Write-Host "    clearing stale postmaster.pid" -ForegroundColor DarkGray
            Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
        }
        # Quote-Arg keeps this as a single argv entry; pg_ctl would otherwise see
        # bare tokens and reject the unknown -h.
        $pgOptions = "-p $DbPort -h 127.0.0.1"
        Invoke-Native -FilePath $pgctl -Label 'pg_ctl start' -TimeoutSeconds 120 `
            -Arguments @('-D', $DataDir, '-l', (Join-Path $WorkDir 'pg.log'), '-o', $pgOptions, '-w', 'start') | Out-Null
        Set-Content -Path $PortFile -Value $DbPort -Encoding ascii
    }

    $env:PGPASSWORD = $DbPassword
    $exists = (Invoke-Native -FilePath $psql -Label 'psql (check database)' -TimeoutSeconds 60 `
        -Arguments @('-h', '127.0.0.1', '-p', "$DbPort", '-U', $DbUser, '-d', 'postgres', '-w', '-tAc',
            "select 1 from pg_database where datname='$DbName'")).Trim()

    # The migrations are NOT idempotent — replaying 0001_init.sql against an
    # already-migrated database fails with "type facility_type already exists".
    # A retained cluster from a previous run would therefore fail the gate for a
    # reason unrelated to the code under test. Since this database is disposable
    # by construction (a throwaway cluster holding only synthetic demo data), the
    # correct move is to recreate it so every run starts from the same empty
    # state. -Force drops any lingering connections first.
    if ($exists -eq '1') {
        Write-Host "    dropping previous $DbName for a clean migration run" -ForegroundColor DarkGray
        Invoke-Native -FilePath $psql -Label 'psql (drop database)' -TimeoutSeconds 60 `
            -Arguments @('-h', '127.0.0.1', '-p', "$DbPort", '-U', $DbUser, '-d', 'postgres', '-w', '-c',
                "drop database if exists $DbName with (force)") | Out-Null
    }
    Invoke-Native -FilePath $createdb -Label 'createdb' -TimeoutSeconds 60 `
        -Arguments @('-h', '127.0.0.1', '-p', "$DbPort", '-U', $DbUser, $DbName) | Out-Null

    $dbUrl = "postgresql://$DbUser`:$DbPassword@127.0.0.1:$DbPort/$DbName`?sslmode=disable"
    $env:DATABASE_URL = $dbUrl

    # --- 2. Migrations and seed --------------------------------------------------
    # The seed is gated on SIGAP_ENV=local by the Makefile db-seed target; the
    # synthetic demo IDs must never reach a shared environment.
    Write-Step "Applying migrations"
    foreach ($file in (Get-ChildItem (Join-Path $RepoRoot 'packages\db\migrations\*.sql') | Sort-Object Name)) {
        Invoke-Native -FilePath $psql -Label "migration $($file.Name)" -TimeoutSeconds 120 `
            -Arguments @($dbUrl, '-v', 'ON_ERROR_STOP=1', '-q', '-f', $file.FullName) | Out-Null
    }

    Write-Step "Loading local seed data (SIGAP_ENV=local)"
    $env:SIGAP_ENV = 'local'
    foreach ($seed in 'dev.sql', 'rbac.sql', 'demo.sql') {
        $path = Join-Path $RepoRoot "packages\db\seed\$seed"
        Invoke-Native -FilePath $psql -Label "seed $seed" -TimeoutSeconds 120 `
            -Arguments @($dbUrl, '-v', 'ON_ERROR_STOP=1', '-q', '-f', $path) | Out-Null
    }

    $facilityCount = (Invoke-Native -FilePath $psql -Label 'psql (count facilities)' -TimeoutSeconds 60 `
        -Arguments @($dbUrl, '-tAc', 'select count(*) from facilities')).Trim()
    Write-Host "    seeded facilities: $facilityCount" -ForegroundColor DarkGray
    if ([int]$facilityCount -eq 0) { Write-Fail "seed produced no facilities." }

    # --- 3. Go API ---------------------------------------------------------------
    Write-Step "Building the Go API binary"
    # `go run` compiles first, and a cold compile can outlast the readiness poll
    # below, which looks exactly like a server that refuses to start. Building a
    # real binary first separates "still compiling" from "genuinely broken" and
    # makes the readiness check meaningful.
    #
    # The build output is captured rather than piped: go can pause on a toolchain
    # or module prompt, and a child holding the parent's stdin makes the whole
    # script look hung with no error and no output.
    # -WorkingDirectory rather than Push-Location: Start-Process pins the child's
    # cwd explicitly, so the go module root is honoured regardless of what this
    # script's own location happens to be.
    $goExe = (Get-Command 'go' -ErrorAction SilentlyContinue).Source
    Invoke-Native -FilePath $goExe -Label 'go build' -TimeoutSeconds 600 `
        -WorkingDirectory (Join-Path $RepoRoot 'apps\api') `
        -Arguments @('build', '-o', (Join-Path $WorkDir 'sigap-api.exe'), './cmd/server') | Out-Null

    Write-Step "Starting Go API on 127.0.0.1:$ApiPort"
    # The API is launched DIRECTLY rather than through a wrapper `pwsh -Command`.
    # Spawning pwsh with a multi-statement env block plus redirected streams is
    # fragile on Windows: depending on the flags it either blocks on the host or
    # exits without ever running the command, and it produces no output either
    # way. Setting the variables on THIS process and starting the exe means the
    # child simply inherits them, which needs no quoting and no wrapper.
    $env:SIGAP_DATABASE_URL   = $dbUrl
    $env:SIGAP_API_PORT        = "$ApiPort"
    $env:SIGAP_ENGINE_ADDR     = '127.0.0.1:50051'
    $env:SIGAP_ENV             = 'local'
    $env:SIGAP_AUTH_MODE       = 'dev'
    $env:SIGAP_DEV_IDENTITY    = 'true'
    $env:SIGAP_ENGINE_FALLBACK = 'dev'
    $env:SIGAP_WEB_ORIGIN      = "http://127.0.0.1:$WebPort"
    $env:SIGAP_API_INTERNAL    = "http://127.0.0.1:$ApiPort"
    $env:SIGAP_API_BASE        = "http://127.0.0.1:$ApiPort"

    $apiProcess = Start-Process -FilePath (Join-Path $WorkDir 'sigap-api.exe') `
        -PassThru -WindowStyle Hidden

    $apiReady = $false
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $health = Invoke-WebRequest "http://127.0.0.1:$ApiPort/health" -UseBasicParsing -TimeoutSec 2
            if ($health.StatusCode -eq 200) { $apiReady = $true; break }
        } catch { }
    }
    if (-not $apiReady) {
        Write-Fail "Go API did not become healthy on 127.0.0.1:$ApiPort. Start it by hand to see the error: $WorkDir\sigap-api.exe"
    }
    Write-Host "    API healthy" -ForegroundColor DarkGray

    # --- 4. Build BEFORE the preview starts -------------------------------------
    # Ordering is load-bearing. The build replaces the hashed assets a running
    # preview is streaming, so a build during preview is what kills it with ENOENT.
    Write-Step "Building web app (must complete before the preview starts)"
    Invoke-Native -FilePath $pwshExe -Label 'vite build (web)' -TimeoutSeconds 600 `
        -WorkingDirectory (Join-Path $RepoRoot 'apps\web') `
        -Arguments @('-NoProfile', '-Command', 'pnpm run build; exit $LASTEXITCODE') | Out-Null

    # --- 5. Web preview ----------------------------------------------------------
    Write-Step "Starting web preview on 127.0.0.1:$WebPort"
    # Same reasoning as the API: set the proxy target on this process and launch
    # the preview binary directly instead of wrapping it in `pwsh -Command`.
    # --host 127.0.0.1 is mandatory, not cosmetic: Windows resolves `localhost`
    # to the IPv6 loopback, so a preview bound by default answers only on ::1 and
    # the Playwright baseURL (127.0.0.1) gets ERR_CONNECTION_REFUSED.
    $env:SIGAP_API_INTERNAL = "http://127.0.0.1:$ApiPort"
    $env:SIGAP_API_BASE     = "http://127.0.0.1:$ApiPort"
    $env:SIGAP_ENV          = 'local'
    $env:SIGAP_DEV_IDENTITY = 'true'

    $WebDir = Join-Path $RepoRoot 'apps\web'
    # pnpm keeps the package's binaries under apps\web\node_modules\.bin, NOT the
    # workspace root, so the path has to be resolved from the package directory.
    $viteCmd = Join-Path $WebDir 'node_modules\.bin\vite.cmd'
    if (-not (Test-Path $viteCmd)) { Write-Fail "vite not found at $viteCmd. Run 'pnpm install' first." }

    $webProcess = Start-Process -FilePath $viteCmd `
        -ArgumentList @('preview', '--host', '127.0.0.1', '--port', "$WebPort", '--strictPort') `
        -WorkingDirectory $WebDir `
        -PassThru -WindowStyle Hidden

    $webReady = $false
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $page = Invoke-WebRequest "http://127.0.0.1:$WebPort/" -UseBasicParsing -TimeoutSec 2
            if ($page.StatusCode -eq 200) { $webReady = $true; break }
        } catch { }
    }
    if (-not $webReady) {
        Write-Fail "Web preview did not become ready on 127.0.0.1:$WebPort. Run it by hand to see the error: pnpm --filter sigap-web preview:e2e"
    }
    Write-Host "    web ready" -ForegroundColor DarkGray

    # --- 6. Prove the proxy path before handing over to Playwright ----------------
    Write-Step "Verifying web -> proxy -> API -> DB"
    $facilities = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/public/facilities" -UseBasicParsing -TimeoutSec 15
    $parsed = $facilities.Content | ConvertFrom-Json
    Write-Host "    proxy returned $($parsed.data.Count) facilities" -ForegroundColor DarkGray
    if ($parsed.data.Count -lt 1) { Write-Fail "Proxy returned no facilities; the seeded stack is not live." }

    # --- 7. Playwright -----------------------------------------------------------
    Write-Step "Running Playwright against the local seeded stack"
    $env:SIGAP_E2E_BASE_URL = "http://127.0.0.1:$WebPort"
    # Playwright writes progress to stderr, so it goes through Invoke-Native for
    # the same reason as every other native child: an unbuffered pipe would stall
    # the run with no visible output.
    try {
        $playwrightOut = Invoke-Native -FilePath $pwshExe -Label 'Playwright' -TimeoutSeconds 900 `
            -WorkingDirectory (Join-Path $RepoRoot 'apps\web') `
            -Arguments @('-NoProfile', '-Command', 'pnpm exec playwright test --reporter=line; exit $LASTEXITCODE')
        $e2eExit = 0
    } catch {
        Write-Host $_.Exception.Message -ForegroundColor Red
        $e2eExit = 1
    }
    if ($playwrightOut) { Write-Host $playwrightOut }

    if ($e2eExit -ne 0) {
        Write-Host ""
        Write-Fail "Playwright reported failures. The local seeded stack was verified live before the run."
    }

    Write-Host ""
    Write-Host "=================================================" -ForegroundColor Green
    Write-Host " GATE 2 E2E: PASS against the local seeded stack" -ForegroundColor Green
    Write-Host "=================================================" -ForegroundColor Green
}
catch {
    # Invoke-Native throws with the child's stderr attached. Surfacing the
    # message and exiting non-zero keeps a real failure distinguishable from the
    # silent hang this script previously suffered.
    Write-Host ""
    Write-Fail $_.Exception.Message
}
finally {
    if ($KeepRunning) {
        Write-Host ""
        Write-Host "(-KeepRunning) Stack left running:" -ForegroundColor Yellow
        Write-Host "  web      http://127.0.0.1:$WebPort" -ForegroundColor Yellow
        Write-Host "  API      http://127.0.0.1:$ApiPort" -ForegroundColor Yellow
        Write-Host "  Postgres postgresql://$DbUser@127.0.0.1:$DbPort/$DbName" -ForegroundColor Yellow
        exit 0
    }

    Write-Host ""
    Write-Step "Stopping local seeded stack"
    if ($webProcess -and -not $webProcess.HasExited) { Stop-Process -Id $webProcess.Id -Force -ErrorAction SilentlyContinue }
    if ($apiProcess -and -not $apiProcess.HasExited) { Stop-Process -Id $apiProcess.Id -Force -ErrorAction SilentlyContinue }
    # Stop by PID file only when one exists, so a teardown that follows an early
    # failure does not emit a confusing "Is server running?" error.
    if (Test-Path (Join-Path $DataDir 'postmaster.pid')) {
        try {
            Invoke-Native -FilePath $pgctl -Label 'pg_ctl stop' -TimeoutSeconds 60 `
                -Arguments @('-D', $DataDir, '-m', 'fast', '-w', 'stop') | Out-Null
        } catch {
            Write-Host "    (pg_ctl stop reported: $($_.Exception.Message))" -ForegroundColor DarkGray
        }
    }
    Write-Step "Stopped. The .local-e2e cluster is retained; delete it to reset."
}
