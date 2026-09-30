<#
.SYNOPSIS
    Deterministic local seeded-stack runner for the GATE 2/GATE 3 Playwright suite.

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
      4. A missing queue engine. `POST /api/v1/queues/generate` is the one
         transactional route with no request-time dev fallback: booking and
         check-in retry against FakeQueueService when the Rust engine is absent
         (booking.go, "Dev fallback"), but the queue handler only has the
         STARTUP-time fallback in main.go. `grpc.Dial` is lazy, so it succeeds
         against a dead address, the fallback is never selected, and every
         walk-in then fails at request time with a 500.

         This is a local infrastructure gap, not a product defect, so the fix
         belongs here and not in committed Go code. The script now starts the
         real Rust engine and waits for it BEFORE the Go API, which matters for
         a second reason that is easy to miss: gRPC connections are lazy AND the
         API's first Generate is its first real use of the channel, so an API
         started against a not-yet-listening engine keeps a stale channel and
         stays broken even after the engine comes up. Starting the engine first
         is what makes the whole chain work, and it is why the API readiness
         check below polls /readyz rather than /health.

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

.PARAMETER EnginePort
    Port for the Rust queue engine. Defaults to 50051, which is the address
    hardcoded in apps/queue-engine/src/main.rs and the default SIGAP_ENGINE_ADDR.
    The engine binary binds a fixed 0.0.0.0:50051 and reads no port variable, so
    the effective port is the first free one at or above this value and is
    reported before the run. If the port is occupied by a process this script did
    not start, it refuses rather than killing an unrelated process.

.PARAMETER SkipEngineBuild
    Reuse an existing release binary instead of rebuilding it. A cold
    `cargo build --release` of the engine is by far the slowest step in this
    script, and it only needs rerunning when the engine source or its proto
    changes.

.EXAMPLE
    pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1

.EXAMPLE
    # Iterate on the smoke suite with the stack left up.
    pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1 -KeepRunning
    # ... then in another shell:
    pnpm --filter sigap-web exec playwright test e2e/smoke.spec.ts

.EXAMPLE
    # Re-run without paying for a cold Rust rebuild.
    pwsh -NoProfile -File scripts/dev/Start-LocalE2E.ps1 -SkipEngineBuild

.NOTES
    Requires PowerShell 7+, Go, Node/pnpm, a Rust toolchain, protoc, and a local
    PostgreSQL install (initdb/pg_ctl/createdb/psql on PATH or under Program
    Files). Never contacts production. The dev identity flags used here are gated
    by SIGAP_ENV=local in apps/api/internal/config/envguard.go and will refuse to
    start anywhere else.
#>

[CmdletBinding()]
param(
    [switch]$KeepRunning,
    [switch]$SkipEngineBuild,
    [int]$DatabasePort = 55433,
    [int]$ApiPort = 18080,
    [int]$WebPort = 4173,
    [int]$EnginePort = 50051,
    # Phase 3B5 §2. The DB-seeded subject the local RBAC selector authenticates
    # as, and the one the web tier names in X-Sigap-Local-Test-Subject.
    #
    # Defaults to the authorized schedule actor (facility_admin at the demo
    # facility, so it genuinely holds schedule.manage THERE). Pass
    # `e2e-schedule-mixed` to run the mixed-provenance case, where
    # schedule.read@A and schedule.manage@B must yield B only, or
    # `local-global-super-admin` for the facility-create case, which
    # AdminHandler.CreateFacility restricts to an unscoped super_admin.
    [string]$LocalActor = 'e2e-schedule-manager',
    # Phase 3B5 T-3B5-04. Which actors to run the browser suite as, in order.
    #
    # TWO ACTORS ARE REQUIRED, and the reason is a real authorization property
    # rather than a desire for extra coverage. Facility CREATE answers 404 unless
    # the actor's facility scope is Unrestricted, while facility UPDATE and
    # DEACTIVATE are authorized per-facility. One subject therefore cannot cover
    # both halves of T-3B5-04:
    #
    #   - `e2e-schedule-manager` is facility_admin at the demo facility: it can
    #     update and deactivate there, and it is the actor whose refusal to
    #     create a facility is worth asserting, because a fully rendered,
    #     submittable create form is refused anyway.
    #   - `local-global-super-admin` holds super_admin with a NULL facility, so
    #     it is the only seeded subject that can create a facility. That matters
    #     because deactivation is ONE-WAY: the deactivation test needs a
    #     disposable facility, and the demo facility is load-bearing for the
    #     queue, appointment, and schedule suites.
    #
    # Each spec skips the halves its actor cannot perform, so running the matrix
    # is additive rather than a way of making one green actor cover both.
    [string[]]$ActorMatrix = @('e2e-schedule-manager', 'local-global-super-admin')
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

<#
    Stop a process TREE, not just the handle it was started with.

    WHY THIS EXISTS. The web preview is launched as `vite.cmd`, a .cmd shim that
    PowerShell can only start through cmd.exe. So the handle returned by
    Start-Process is a cmd.exe process, while the process that actually HOLDS THE
    PORT is a node.exe grandchild of it. Stopping only the tracked handle left the
    grandchild alive and still listening: observed as 127.0.0.1:4173 answering
    HTTP 200 for minutes after teardown, with its parent already reaped.

    That is a real cleanup-safety defect, and an indirect one — a leaked preview
    occupies the very port the next run wants, so `Get-FreePort` hands the
    following run a DIFFERENT port and it then tests something other than what
    the banner printed.

    Descendants are collected by parent pid BEFORE anything is stopped, then
    killed deepest-first, because a dying parent reparents its children and the
    link needed to find them disappears with it. Strictly scoped to this run's
    own subtree, so an engine or preview someone started by hand is never killed.
#>
function Stop-OwnedTree([System.Diagnostics.Process]$Root, [string]$Label) {
    if (-not $Root) { return }

    $ids = New-Object System.Collections.Generic.List[int]
    $queue = New-Object System.Collections.Generic.Queue[int]
    if (-not $Root.HasExited) {
        $ids.Add($Root.Id)
        $queue.Enqueue($Root.Id)
    }
    while ($queue.Count -gt 0) {
        $parent = $queue.Dequeue()
        $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$parent" -ErrorAction SilentlyContinue)
        foreach ($child in $children) {
            $childId = [int]$child.ProcessId
            if (-not $ids.Contains($childId)) {
                $ids.Add($childId)
                $queue.Enqueue($childId)
            }
        }
    }

    $ordered = $ids.ToArray() | Sort-Object -Descending
    foreach ($processId in $ordered) {
        try { Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue } catch { }
    }
    if ($ordered) { Write-Host "    stopped $Label (pid $($ordered -join ', '))" -ForegroundColor DarkGray }
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

<#
    The engine is a fixed-address binary, so the "find a free port" treatment the
    other services get does not apply to it: main.rs binds a literal
    0.0.0.0:50051 and reads no port from the environment, so a busy 50051 cannot
    be moved.

    The rule here is therefore refuse, never take over. A listener on the engine
    port is either a stale process from a previous run that a human can judge, or
    something unrelated that must not be killed to make a test pass. Auto-killing
    either is exactly the kind of "make the gate green" move that hides a real
    problem, so the script names the owner and stops. `-KeepRunning` reuses a port
    this script's own previous run left open, which is the common case, so the
    message tells the user how to clear it.
#>
$EnginePortBusy = Get-NetTCPConnection -State Listen -LocalPort $EnginePort -ErrorAction SilentlyContinue
if ($EnginePortBusy) {
    $owners = ($EnginePortBusy | Select-Object -ExpandProperty OwningProcess -Unique)
    $described = ($owners | ForEach-Object {
        try {
            $p = Get-Process -Id $_ -ErrorAction Stop
            "$($p.ProcessName) (pid $($p.Id))"
        } catch { "pid $_" }
    }) -join ', '
    Write-Fail @"
Port $EnginePort is already in use by: $described
The Rust queue engine binds a fixed address (apps/queue-engine/src/main.rs), so it
cannot be moved to a different port. Stop that process and re-run, or pass
-EnginePort if a previous run of THIS script left it open.
"@
}

Write-Host ""
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host " Sigap local seeded stack (GATE 2 / GATE 3 E2E)" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host "  web        http://127.0.0.1:$WebPort"
Write-Host "  Go API     http://127.0.0.1:$ApiPort"
Write-Host "  Engine     127.0.0.1:$EnginePort (Rust, real gRPC)"
Write-Host "  Postgres   postgresql://$DbUser@127.0.0.1:$DbPort/$DbName (disposable)"
Write-Host "  SIGAP_ENV  local"
Write-Host "=================================================" -ForegroundColor Cyan
Write-Host ""

$apiProcess = $null
$webProcess = $null
$engineProcess = $null

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

    # --- 3. Rust queue engine ---------------------------------------------------
    # This is the real engine, not a fake, and that is the entire point: a
    # FakeQueueService would return a plausible ticket without proving anything,
    # because the walk-in E2E exists to prove the gRPC hop and the transactional
    # row-locking counter actually work end to end.
    #
    # Order is load-bearing and cannot be rearranged. The engine must be listening
    # BEFORE the Go API starts, for two independent reasons:
    #   - grpc.Dial is lazy, so main.go's startup fallback never triggers against
    #     a dead address and the API would silently hold a broken channel.
    #   - The API's channel does not reconnect usefully for a Generate that
    #     happens too early, so an API started first stays broken even after the
    #     engine appears. Observed directly: the API had to be restarted by hand
    #     after the engine came up before /readyz turned ready.
    Write-Step "Building the Rust queue engine"
    $engineDir = Join-Path $RepoRoot 'apps\queue-engine'
    $engineExe = Join-Path $engineDir 'target\release\sigap-queue-engine.exe'
    if ($SkipEngineBuild -and (Test-Path $engineExe)) {
        Write-Host "    reusing existing engine binary (-SkipEngineBuild)" -ForegroundColor DarkGray
    } else {
        # `cargo build --release` is the slowest step here by a wide margin, and
        # build.rs shells out to protoc, so both are preconditions rather than
        # surprises discovered 200 lines into a failed run. Kept as a separate
        # check so the failure names the missing tool instead of a codegen error.
        if (-not (Get-Command 'cargo' -ErrorAction SilentlyContinue)) {
            Write-Fail "cargo not found on PATH. Install the Rust toolchain (https://rustup.rs) to run the local E2E stack."
        }
        if (-not (Get-Command 'protoc' -ErrorAction SilentlyContinue)) {
            Write-Fail "protoc not found on PATH. build.rs compiles protos/sigap/queue_engine.proto with tonic-build, which requires it."
        }
        Invoke-Native -FilePath (Get-Command 'cargo').Source -Label 'cargo build --release' -TimeoutSeconds 1800 `
            -WorkingDirectory $engineDir `
            -Arguments @('build', '--release') | Out-Null
    }
    if (-not (Test-Path $engineExe)) {
        Write-Fail "Queue engine binary not found at $engineExe after the build."
    }

    Write-Step "Starting the Rust queue engine on 127.0.0.1:$EnginePort"
    # DATABASE_URL, not SIGAP_DATABASE_URL: main.rs reads that exact name and
    # exits immediately if it is missing, so the engine shares the same
    # disposable Postgres the API and migrations just used.
    $env:DATABASE_URL = $dbUrl
    $engineProcess = Start-Process -FilePath $engineExe -PassThru -WindowStyle Hidden

    # Readiness is a TCP connect, and it is a sound signal here specifically
    # because of the engine's own startup order: PgPool::connect completes BEFORE
    # Server::serve is called, so a listening socket means the pool is already
    # open against the migrated, seeded database. A "listening but not yet
    # migrated" window does not exist in this binary. Verified against the real
    # response: the first successful generate returned "PMI-0001" for the seeded
    # Puskesmas Melati Indah, a short code that only the engine reads from the
    # facilities table.
    $engineReady = $false
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        if ($engineProcess.HasExited) {
            Write-Fail "Queue engine exited during startup (code $($engineProcess.ExitCode)). Run it by hand to see the error: $engineExe"
        }
        try {
            $probe = [System.Net.Sockets.TcpClient]::new()
            $probe.Connect('127.0.0.1', $EnginePort)
            $engineReady = $probe.Connected
            $probe.Close()
            if ($engineReady) { break }
        } catch { $engineReady = $false }
    }
    if (-not $engineReady) {
        Write-Fail "Queue engine did not start listening on 127.0.0.1:$EnginePort. Run it by hand to see the error: $engineExe"
    }
    Write-Host "    engine listening on 127.0.0.1:$EnginePort" -ForegroundColor DarkGray

    # --- 4. Go API ---------------------------------------------------------------
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
    $env:SIGAP_ENGINE_ADDR     = "127.0.0.1:$EnginePort"
    $env:SIGAP_ENV             = 'local'
    $env:SIGAP_AUTH_MODE       = 'dev'
    $env:SIGAP_DEV_IDENTITY    = 'true'
    # Phase 3B5 §2: arm the local DB-backed identity selector.
    #
    # This makes the API resolve `e2e-schedule-manager` through the REAL RBAC
    # resolver on every request, which is what lets the mutation E2E run as an
    # actor whose schedule.manage genuinely comes from user_roles rather than
    # from a synthetic permission list.
    #
    # It REPLACES the dev identity provider (cmd/server/main.go), so from here
    # on `X-Sigap-Dev-User-ID` is inert and `X-Sigap-Local-Test-Subject` is the
    # only thing that selects an actor. The web tier mirrors that precedence in
    # `localE2eActorHeader`, which is why both sides set SIGAP_ENV=local.
    #
    # Override with -LocalActor to run the mixed-provenance or denied-actor
    # cases. Both are refused by the API outside SIGAP_ENV=local, and
    # GuardDevCapabilities refuses to START such a process at all.
    $env:SIGAP_LOCAL_RBAC_TEST_IDENTITY = 'true'
    $env:SIGAP_LOCAL_E2E_ACTOR          = $LocalActor
    # Left unset on purpose for the authoritative run. The engine is really
    # running, so the fallback is not needed — and keeping it set would mean a
    # future engine outage silently produced plausible-looking tickets instead of
    # failing, which is precisely the failure mode this script exists to remove.
    # Booking and check-in keep their own request-time retry in committed code;
    # that is untouched. This is about not letting the E2E stack depend on it.
    Remove-Item Env:SIGAP_ENGINE_FALLBACK -ErrorAction SilentlyContinue
    $env:SIGAP_WEB_ORIGIN      = "http://127.0.0.1:$WebPort"
    $env:SIGAP_API_INTERNAL    = "http://127.0.0.1:$ApiPort"
    $env:SIGAP_API_BASE        = "http://127.0.0.1:$ApiPort"

    $apiProcess = Start-Process -FilePath (Join-Path $WorkDir 'sigap-api.exe') `
        -PassThru -WindowStyle Hidden

    # /readyz, NOT /health. This is the check that would have caught the original
    # problem: /health only proves the HTTP server is up and returns 200 with a
    # dead engine behind it, so a /health poll declared a stack ready that could
    # not complete a walk-in. /readyz probes the queue service and reports
    # "engine unreachable" when it is not connected, which is the exact state
    # that made every walk-in 500.
    $apiReady = $false
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Milliseconds 500
        if ($apiProcess.HasExited) {
            Write-Fail "Go API exited during startup (code $($apiProcess.ExitCode)). Start it by hand to see the error: $WorkDir\sigap-api.exe"
        }
        try {
            $health = Invoke-WebRequest "http://127.0.0.1:$ApiPort/readyz" -UseBasicParsing -TimeoutSec 2
            if ($health.StatusCode -eq 200) { $apiReady = $true; break }
        } catch { }
    }
    if (-not $apiReady) {
        Write-Fail @"
Go API did not report ready on 127.0.0.1:$ApiPort.
/readyz is the check because /health returns 200 with a dead engine behind it.
If it reports "engine unreachable", the queue engine on 127.0.0.1:$EnginePort is
not reachable. Start it by hand to see the error: $engineExe
"@
    }
    Write-Host "    API ready (queue engine connected)" -ForegroundColor DarkGray

    # --- 5. Build BEFORE the preview starts -------------------------------------
    # Ordering is load-bearing. The build replaces the hashed assets a running
    # preview is streaming, so a build during preview is what kills it with ENOENT.
    Write-Step "Building web app (must complete before the preview starts)"
    Invoke-Native -FilePath $pwshExe -Label 'vite build (web)' -TimeoutSeconds 600 `
        -WorkingDirectory (Join-Path $RepoRoot 'apps\web') `
        -Arguments @('-NoProfile', '-Command', 'pnpm run build; exit $LASTEXITCODE') | Out-Null

    # --- 6. Web preview ----------------------------------------------------------
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
    # The web tier needs the SAME actor the API is armed with, because
    # `localE2eActorHeader` emits the subject header and the API's local
    # provider is the one that reads it. Setting only one side is the
    # interesting failure mode: the API would receive no subject and answer with
    # a zero actor, surfacing as a 401 on every admin read.
    $env:SIGAP_LOCAL_E2E_ACTOR = $LocalActor

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

    # --- 7. Prove the proxy path before handing over to Playwright ----------------
    Write-Step "Verifying web -> proxy -> API -> DB"
    $facilities = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/public/facilities" -UseBasicParsing -TimeoutSec 15
    $parsed = $facilities.Content | ConvertFrom-Json
    Write-Host "    proxy returned $($parsed.data.Count) facilities" -ForegroundColor DarkGray
    if ($parsed.data.Count -lt 1) { Write-Fail "Proxy returned no facilities; the seeded stack is not live." }

    # --- 8. Prove the local DB-backed actor chain before any mutation E2E -----
    #
    # Phase 3B5 §2 requires this BEFORE mutation E2E, because a mutation suite
    # running as the dev identity would prove nothing about facility-scoped
    # schedule.manage: the dev actor carries a synthetic permission list with
    # unrestricted grants, so every affordance would trivially appear.
    #
    # Two things are checked here, and the second matters more than the first.
    Write-Step "Verifying the local DB-backed actor chain (T-3B5 §2)"
    Write-Host "    actor: $LocalActor" -ForegroundColor DarkGray

    # (a) The chain works: browser -> web -> proxy -> Go -> DB RBAC resolver.
    # The options endpoint is the one read whose content is derived from
    # facility-scoped schedule.manage, so a non-empty result is proof the real
    # resolver ran rather than a synthetic permission set.
    $options = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/admin/schedules/options" `
        -UseBasicParsing -TimeoutSec 15
    $manageable = ($options.Content | ConvertFrom-Json).data.facilities
    if ($manageable.Count -lt 1) {
        Write-Fail @"
The schedule options endpoint returned no manageable facilities for '$LocalActor'.

The local RBAC selector is armed but resolved to zero schedule.manage grants, so
either the subject is not seeded or its role is not attached. Check that
packages/db/seed/dev.sql created '$LocalActor' and that demo.sql granted it a
role at a facility.
"@
    }
    $manageableNames = ($manageable | ForEach-Object { $_.name }) -join ', '
    Write-Host "    manageable facilities: $manageableNames" -ForegroundColor DarkGray

    # (b) The browser CANNOT bypass the proxy or select an arbitrary identity.
    # A browser-supplied X-Sigap-* header is not forwarded by the proxy, which
    # reconstructs the upstream header set from proxyHeaders() alone. This is
    # asserted here as well as in the unit suite because it is the property the
    # whole §2 argument rests on, and an HTTP-level probe is the only way to
    # show it against a running stack rather than a mocked one.
    $forged = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/admin/schedules/options" `
        -Headers @{ 'X-Sigap-Local-Test-Subject' = 'e2e-schedule-mixed' } `
        -UseBasicParsing -TimeoutSec 15
    $forgedFacilities = ($forged.Content | ConvertFrom-Json).data.facilities
    $forgedNames = ($forgedFacilities | ForEach-Object { $_.name }) -join ', '

    if ($forgedNames -ne $manageableNames) {
        Write-Fail @"
A browser-supplied X-Sigap-Local-Test-Subject header CHANGED the resolved identity.

  configured actor: $LocalActor -> $manageableNames
  forged header:    e2e-schedule-mixed -> $forgedNames

The proxy must reconstruct upstream headers from proxyHeaders() and never forward
caller-supplied X-Sigap-* headers. If this fired, identity is browser-controllable.
"@
    }
    Write-Host "    browser-supplied X-Sigap-* header correctly ignored" -ForegroundColor DarkGray

    # --- 9. Prove the walk-in chain before the browser suite --------------------
    # The full transactional hop, exercised once here so a broken engine fails in
    # ONE line with a readable cause instead of surfacing as a Playwright
    # timeout on a selector 40 seconds into a browser run.
    #
    # This also guards against the exact regression that motivated the change: a
    # 200 here is only meaningful if the ticket came from the Rust engine. A
    # FakeQueueService answer is always "RSK-0001" with a fixed "123μs"
    # processing time regardless of facility, and the facility-driven prefix
    # check below catches it — a fake response cannot agree with the facility
    # that was actually requested.
    Write-Step "Verifying walk-in against the real queue engine"
    $catalog = $parsed.data | Where-Object { $_.is_active } | Select-Object -First 1
    if (-not $catalog) { Write-Fail "No active facility in the catalog to test the walk-in with." }

    # Unique per attempt so the in-memory daily limiter (2 per phone per facility
    # per day, keyed on the API process) cannot reject a repeat run.
    $walkinPhone = '081' + (Get-Random -Minimum 10000000 -Maximum 99999999)
    $walkinBody = @{
        facilityId = $catalog.id
        patient    = @{ fullName = 'Local Stack Walk-in Probe'; phone = $walkinPhone }
    } | ConvertTo-Json -Depth 5

    try {
        $walkin = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/queues/generate" `
            -Method POST -ContentType 'application/json' -Body $walkinBody -UseBasicParsing -TimeoutSec 20
    } catch {
        $code = try { [int]$_.Exception.Response.StatusCode } catch { 'unknown' }
        Write-Fail @"
Walk-in failed through the full chain (HTTP $code).
browser -> web -> proxy -> Go API -> Rust engine ($EnginePort) -> Postgres
Run the engine by hand to see the error: $engineExe
"@
    }

    $ticket = ($walkin.Content | ConvertFrom-Json).data
    Write-Host "    HTTP $($walkin.StatusCode)  ticket=$($ticket.formatted_number)  wait=$($ticket.estimated_wait_minutes)m" -ForegroundColor DarkGray

    if ($walkin.StatusCode -ne 200) { Write-Fail "Walk-in returned HTTP $($walkin.StatusCode); the engine chain is not serving." }
    if (-not $ticket.formatted_number) { Write-Fail "Walk-in succeeded but returned no formatted_number; the success path is not renderable." }
    if ($ticket.formatted_number -ne "$($catalog.short_code)-0001") {
        Write-Fail @"
Walk-in returned '$($ticket.formatted_number)' for facility short_code '$($catalog.short_code)'.
Expected '$($catalog.short_code)-0001'. A mismatch means the ticket did not come from
the real engine reading the facilities table (the dev fake always returns RSK-0001
regardless of facility), so the stack is not exercising the gRPC hop it claims to.
"@
    }
    if ($ticket.formatted_number -eq 'RSK-0001' -and $catalog.short_code -ne 'RSK') {
        Write-Fail "Walk-in returned the FakeQueueService constant RSK-0001. The real engine is not serving requests."
    }
    Write-Host "    walk-in verified end to end" -ForegroundColor DarkGray

    # --- 10. Playwright, once per actor -------------------------------------------
    #
    # WHY THE MATRIX EXISTS. Facility create is authorized globally (unrestricted
    # scope) while facility update and deactivate are authorized per-facility, so
    # no single seeded subject can exercise both halves of T-3B5-04. The suite is
    # therefore run once per actor, and each spec skips the halves its actor
    # genuinely cannot perform. See -ActorMatrix.
    #
    # ONLY THE WEB TIER IS RESTARTED. The Go API resolves the subject from the
    # request header on every call, so it is already actor-agnostic; the web tier
    # is the side that pins the subject at process start (localE2eActorHeader
    # reads process.env), which is precisely the property that stops a browser
    # from choosing its own identity. Re-spawning it per actor keeps that
    # property intact instead of weakening it to make a matrix convenient.
    $env:SIGAP_E2E_BASE_URL = "http://127.0.0.1:$WebPort"

    $actorFailures = @()
    foreach ($actor in $ActorMatrix) {
        Write-Step "Running Playwright as actor: $actor"

        if ($actor -ne $LocalActor) {
            # Re-pin the web tier. The preview is stopped first so the new
            # process is the only listener, rather than racing the old one for
            # the port with --strictPort.
            Stop-OwnedTree $webProcess 'web preview (previous actor)'
            $env:SIGAP_LOCAL_E2E_ACTOR = $actor
            $webProcess = Start-Process -FilePath $viteCmd `
                -ArgumentList @('preview', '--host', '127.0.0.1', '--port', "$WebPort", '--strictPort') `
                -WorkingDirectory $WebDir `
                -PassThru -WindowStyle Hidden

            $actorReady = $false
            for ($i = 0; $i -lt 60; $i++) {
                Start-Sleep -Milliseconds 500
                try {
                    $probe = Invoke-WebRequest "http://127.0.0.1:$WebPort/" -UseBasicParsing -TimeoutSec 2
                    if ($probe.StatusCode -eq 200) { $actorReady = $true; break }
                } catch { }
            }
            if (-not $actorReady) {
                Write-Fail "Web preview did not come back up for actor '$actor'."
            }
            Write-Host "    web re-pinned to $actor" -ForegroundColor DarkGray
        }

        # Re-prove the identity chain per actor rather than trusting the first
        # actor's result. The whole point of the matrix is that each actor
        # resolves differently, so asserting it once would assert nothing about
        # the second.
        $actorOptions = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/admin/schedules/options" `
            -UseBasicParsing -TimeoutSec 15
        $actorFacilities = ($actorOptions.Content | ConvertFrom-Json).data.facilities
        $actorNames = ($actorFacilities | ForEach-Object { $_.name }) -join ', '
        Write-Host "    manageable facilities: $actorNames" -ForegroundColor DarkGray

        $actorForged = Invoke-WebRequest "http://127.0.0.1:$WebPort/api/v1/admin/schedules/options" `
            -Headers @{ 'X-Sigap-Local-Test-Subject' = 'e2e-schedule-mixed' } `
            -UseBasicParsing -TimeoutSec 15
        $actorForgedNames = (($actorForged.Content | ConvertFrom-Json).data.facilities |
            ForEach-Object { $_.name }) -join ', '
        if ($actorForgedNames -ne $actorNames) {
            Write-Fail @"
A browser-supplied X-Sigap-Local-Test-Subject header CHANGED the resolved identity for actor '$actor'.

  configured actor: $actor -> $actorNames
  forged header:    e2e-schedule-mixed -> $actorForgedNames

The proxy must reconstruct upstream headers from proxyHeaders() and never forward
caller-supplied X-Sigap-* headers. If this fired, identity is browser-controllable.
"@
        }
        Write-Host "    browser-supplied X-Sigap-* header correctly ignored" -ForegroundColor DarkGray

        # SIGAP_E2E_ACTOR lets a spec know WHICH subject it is running as, so it
        # can assert the half it is authorized for and skip the half it is not.
        # It is the same value the web tier is already pinned to; passing it
        # here keeps the test process and the server from ever disagreeing.
        $env:SIGAP_E2E_ACTOR = $actor

        # Playwright writes progress to stderr, so it goes through Invoke-Native
        # for the same reason as every other native child: an unbuffered pipe
        # would stall the run with no visible output.
        #
        # Pre-seeded because Invoke-Native THROWS on a non-zero exit, and under
        # Set-StrictMode reading an unset variable is itself a terminating error.
        # The catch block below prints $_.Exception.Message, which already
        # carries the reporter's stdout and stderr; without this seed a failing
        # suite would replace its own failure report with "variable cannot be
        # retrieved".
        $playwrightOut = ''
        $e2eExit = 0
        try {
            $playwrightOut = Invoke-Native -FilePath $pwshExe -Label "Playwright ($actor)" -TimeoutSeconds 1800 `
                -WorkingDirectory (Join-Path $RepoRoot 'apps\web') `
                -Arguments @('-NoProfile', '-Command', 'pnpm exec playwright test --reporter=line; exit $LASTEXITCODE')
        } catch {
            Write-Host $_.Exception.Message -ForegroundColor Red
            $e2eExit = 1
        }
        if ($playwrightOut) { Write-Host $playwrightOut }

        if ($e2eExit -ne 0) {
            # Recorded rather than fatal, so one actor's failure does not hide
            # the other's result. The run still fails at the end, and it fails
            # with BOTH actors named.
            $actorFailures += $actor
        }
    }

    # Leave the web tier pinned to the actor the caller asked for, so a
    # -KeepRunning stack is the one they expect to poke at by hand.
    if ($ActorMatrix[-1] -ne $LocalActor) {
        Stop-OwnedTree $webProcess 'web preview (actor matrix)'
        $env:SIGAP_LOCAL_E2E_ACTOR = $LocalActor
        $webProcess = Start-Process -FilePath $viteCmd `
            -ArgumentList @('preview', '--host', '127.0.0.1', '--port', "$WebPort", '--strictPort') `
            -WorkingDirectory $WebDir `
            -PassThru -WindowStyle Hidden
        for ($i = 0; $i -lt 60; $i++) {
            Start-Sleep -Milliseconds 500
            try {
                $probe = Invoke-WebRequest "http://127.0.0.1:$WebPort/" -UseBasicParsing -TimeoutSec 2
                if ($probe.StatusCode -eq 200) { break }
            } catch { }
        }
    }

    if ($actorFailures.Count -ne 0) {
        Write-Host ""
        Write-Fail @"
Playwright reported failures for: $($actorFailures -join ', ')

The local seeded stack was verified live before the run, and each actor's identity
chain was re-proven immediately before its suite.
"@
    }

    Write-Host ""
    Write-Host "=================================================" -ForegroundColor Green
    Write-Host " GATE 2 / GATE 3 E2E: PASS against the local seeded stack" -ForegroundColor Green
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
        Write-Host "  Engine   127.0.0.1:$EnginePort" -ForegroundColor Yellow
        Write-Host "  Postgres postgresql://$DbUser@127.0.0.1:$DbPort/$DbName" -ForegroundColor Yellow
        Write-Host ""
        # Only printed when the engine actually started. Under Set-StrictMode a
        # failed run reaches this finally block with $engineProcess still $null,
        # and reading .Id off that would throw and bury the real failure message.
        if ($engineProcess) {
            Write-Host "  The engine keeps holding port $EnginePort, so a re-run needs that process" -ForegroundColor DarkGray
            Write-Host "  stopped first. Its pid is $($engineProcess.Id)." -ForegroundColor DarkGray
        }
        exit 0
    }

    Write-Host ""
    Write-Step "Stopping local seeded stack"
    Stop-OwnedTree $webProcess 'web preview'
    Stop-OwnedTree $apiProcess 'Go API'
    # Engine last, so the API is never left talking to a dead channel mid-teardown.
    # The engine is a single .exe with no shim, so a plain stop is exact here;
    # the tree walk is harmless and keeps one teardown path.
    Stop-OwnedTree $engineProcess 'queue engine'
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
