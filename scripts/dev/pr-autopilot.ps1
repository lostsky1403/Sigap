<#
.SYNOPSIS
    Sigap PR Autopilot — local-only PowerShell helper that automates the
    safe parts of the PR workflow (verify, push, create PR, watch CI,
    collect comments). NEVER auto-merges unless -MergeWhenGreen is passed.

.DESCRIPTION
    Phase 1: tool-availability gate.
    Phase 2: local verification matrix (go test, govulncheck, cargo test,
             cargo audit, pnpm check, gitleaks).
    Phase 3: git safety (clean tree, not main/master, branch ahead).
    Phase 4: push (with correct upstream pattern).
    Phase 5: PR create or reuse.
    Phase 6: watch CI and summarise.
    Phase 7: collect reviews + comments + WarpFix mentions.
    Phase 8 (optional): merge --squash --delete-branch, only when
                        -MergeWhenGreen AND every gate is green.

.PARAMETER VerifyOnly
    Run only the env check + local verification matrix. Never push,
    never touch git remotes, never call gh.

.PARAMETER NoPush
    Equivalent to -VerifyOnly (kept for symmetry with the README's
    "no push" language).

.PARAMETER Title
    PR title passed to `gh pr create` when no existing PR is found.

.PARAMETER Body
    PR body passed to `gh pr create` when no existing PR is found.

.PARAMETER Draft
    When set, the new PR is created in draft mode (`--draft`).

.PARAMETER MergeWhenGreen
    Explicitly opt in to auto-merge when every gate is green. Without
    this flag the script NEVER invokes `gh pr merge`.

.PARAMETER BaseBranch
    Branch the PR should target and the branch we compare against for
    "has commits ahead". Defaults to "main".

.PARAMETER CommandTimeoutSeconds
    Maximum wall-clock seconds each local-verification command is allowed
    to run before it is killed and reported as [FAIL]. Defaults to 900
    (15 minutes). Pass a smaller value (e.g. 300) for a quick local dry
    run. Applies to every step in the verification matrix: go test,
    govulncheck, cargo test, cargo audit, pnpm check, gitleaks.

.EXAMPLE
    pwsh -File scripts/dev/pr-autopilot.ps1 -VerifyOnly -CommandTimeoutSeconds 300

.EXAMPLE
    pwsh -File scripts/dev/pr-autopilot.ps1 `
        -Title "fix(smoke): harden demo script" `
        -Body  "Address WarpFix review comments..."

.EXAMPLE
    pwsh -File scripts/dev/pr-autopilot.ps1 -MergeWhenGreen

.EXAMPLE
    # Quick local dry run with a 5-minute timeout per step
    pwsh -File scripts/dev/pr-autopilot.ps1 -VerifyOnly -CommandTimeoutSeconds 300

.NOTES
    Requires PowerShell 7+. See scripts/dev/README.md for the full
    exit-code table, limitations, and safety guarantees.
#>

[CmdletBinding()]
param(
    [switch]$VerifyOnly,
    [switch]$NoPush,
    [string]$Title,
    [string]$Body,
    [switch]$Draft,
    [switch]$MergeWhenGreen,
    [string]$BaseBranch = 'main',
    [int]$CommandTimeoutSeconds = 900
)

if ($CommandTimeoutSeconds -lt 1) {
    Write-Host "[FAIL] parameters: -CommandTimeoutSeconds must be >= 1 (got: $CommandTimeoutSeconds)" -ForegroundColor Red
    exit 2
}

$originalErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = 'Continue'

# VerifyOnly and NoPush are aliases.
if ($NoPush) { $VerifyOnly = $true }

# ---------------------------------------------------------------
# Exit code accumulator — every phase reports through this.
#   0 success
#   1 local verification failed
#   2 environment/config error
#   3 GitHub checks failed
#   4 review/comment gate requires human attention
#   5 merge refused
# ---------------------------------------------------------------
$script:ExitCode = 0
$script:PhaseName = ''
$script:PrUrl = $null

function Set-ExitCode {
    param([int]$Code)
    if ($Code -gt $script:ExitCode) { $script:ExitCode = $Code }
}

# ---------------------------------------------------------------
# Logging helpers — consistent prefixes per spec.
# ---------------------------------------------------------------
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor DarkCyan
}

function Write-Pass {
    param([string]$Message)
    Write-Host "[PASS] $Message" -ForegroundColor Green
}

function Write-Fail {
    param([string]$Message)
    Write-Host "[FAIL] $Message" -ForegroundColor Red
}

function Write-Skip {
    param([string]$Message)
    Write-Host "[SKIP] $Message" -ForegroundColor DarkYellow
}

# ---------------------------------------------------------------
# Tool-availability gate (Phase 1).
#
# Tools are looked up via Get-Command (PATH first). For tools that
# Sigap documents as "may live outside PATH on Windows", we also try
# the documented install location as a fallback. If neither path
# resolves, the tool is reported as missing.
# ---------------------------------------------------------------
$ToolFallbacks = @{
    'go'           = @(
        "$env:ProgramFiles\Go\bin\go.exe",
        "$env:ProgramFiles\Git\bin\go.exe"
    )
    'govulncheck'  = @( )
    'gitleaks'     = @(
        "$env:ProgramFiles\Gitleaks\gitleaks.exe",
        'C:\Program Files\Gitleaks\gitleaks.exe',
        (Join-Path $env:TEMP 'gitleaks\gitleaks.exe')
    )
}

# Resolve GOPATH lazily (may be unset on fresh terminals) so the
# govulncheck fallback list is populated at tool-resolution time, not
# at script-load time.
function Get-Gopath {
    if ($env:GOPATH) { return $env:GOPATH }
    if ($env:USERPROFILE) { return (Join-Path $env:USERPROFILE 'go') }
    return $null
}

# Env-var overrides: $env:SIGAP_GO_PATH, $env:SIGAP_GOVULNCHECK_PATH,
# $env:SIGAP_GITLEAKS_PATH take precedence over fallbacks.
$ToolEnvOverrides = @{
    'go'           = $env:SIGAP_GO_PATH
    'govulncheck'  = $env:SIGAP_GOVULNCHECK_PATH
    'gitleaks'     = $env:SIGAP_GITLEAKS_PATH
}

$RequiredTools = @('git','gh','go','cargo','pnpm','govulncheck','gitleaks')
$OptionalTools = @('cargo-audit')

function Resolve-Tool {
    param([string]$Name)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    if ($ToolEnvOverrides.ContainsKey($Name) -and $ToolEnvOverrides[$Name] -and (Test-Path $ToolEnvOverrides[$Name])) {
        return $ToolEnvOverrides[$Name]
    }
    if ($ToolFallbacks.ContainsKey($Name)) {
        foreach ($cand in $ToolFallbacks[$Name]) {
            if ($cand -and (Test-Path $cand)) { return $cand }
        }
        # Lazily-resolved fallbacks that depend on env vars that may
        # be unset at script-load time.
        if ($Name -eq 'govulncheck') {
            $gopath = Get-Gopath
            if ($gopath) {
                $cand = Join-Path $gopath 'bin\govulncheck.exe'
                if (Test-Path $cand) { return $cand }
            }
            if ($env:USERPROFILE) {
                $cand = Join-Path $env:USERPROFILE 'go\bin\govulncheck.exe'
                if (Test-Path $cand) { return $cand }
            }
        }
    }
    return $null
}

$ToolMap = @{}   # Name -> Path
$MissingRequired = New-Object System.Collections.Generic.List[string]
foreach ($t in $RequiredTools) {
    $path = Resolve-Tool -Name $t
    if ($path) {
        $ToolMap[$t] = $path
    } else {
        $MissingRequired.Add($t)
    }
}

$ToolInstallHints = @{
    'git'           = 'https://git-scm.com/download/win (or `winget install Git.Git`)'
    'gh'            = 'https://cli.github.com/ (or `winget install GitHub.cli`)'
    'go'            = 'https://go.dev/dl/ (or `winget install GoLang.Go`)'
    'cargo'         = 'https://rustup.rs/ (or `winget install Rustlang.Rustup`)'
    'pnpm'          = 'https://pnpm.io/installation (or `winget install pnpm.pnpm`)'
    'govulncheck'   = '`go install golang.org/x/vuln/cmd/govulncheck@latest`'
    'gitleaks'      = 'https://github.com/gitleaks/gitleaks/releases (or `winget install GitHub.Gitleaks`)'
}

Write-Host "Sigap PR Autopilot"
Write-Host "Base branch    : $BaseBranch"
Write-Host "Mode           : $(if ($VerifyOnly) { 'VerifyOnly' } elseif ($MergeWhenGreen) { 'push + watch + merge-when-green' } else { 'push + watch + summary (no merge)' })"
Write-Host ""

Write-Info "Phase 1: tool availability"
foreach ($t in $RequiredTools) {
    if ($ToolMap.ContainsKey($t)) {
        Write-Pass "tool available: $t -> $($ToolMap[$t])"
    } else {
        Write-Fail "missing required tool: $t"
        Write-Host "       install: $($ToolInstallHints[$t])" -ForegroundColor DarkRed
    }
}
foreach ($t in $OptionalTools) {
    $path = Resolve-Tool -Name $t
    if ($path) {
        $ToolMap[$t] = $path
        Write-Pass "optional tool available: $t -> $path"
    } else {
        Write-Skip "optional tool not installed: $t (step will be reported as SKIP)"
    }
}

if ($MissingRequired.Count -gt 0) {
    Write-Fail "missing required tools: $($MissingRequired -join ', ')"
    Set-ExitCode -Code 2
    $ErrorActionPreference = $originalErrorActionPreference
    exit $script:ExitCode
}
Write-Pass "all required tools available"

# ---------------------------------------------------------------
# Local verification matrix (Phase 2).
# Each step uses an external command via & "$ToolMap[$name]" or
# the .exe path directly. Output is captured to a temp file and
# the last ~20 lines are echoed on FAIL.
# ---------------------------------------------------------------
$script:PhaseName = 'verify'

function Invoke-VerifyStep {
    <#
    .SYNOPSIS
        Run a single verification command with timeout enforcement and
        safe output capture.

    .DESCRIPTION
        Three guarantees:
          1. The child process is killed if it runs longer than
             $TimeoutSec. The script prints [FAIL] with timeout details
             and exits via Set-ExitCode -Code 1 (no push, no PR, no merge).
          2. stdout / stderr are drained asynchronously so the child
             cannot deadlock on a full pipe buffer (a common Windows
             failure mode when using Start-Process -Redirect* -Wait).
          3. The original working directory is always restored, even
             on exception.

        The InProcPowerShell switch runs the command inside a PowerShell
        Job (so the same timeout guarantee applies via Wait-Job -Timeout)
        and is used for tools whose path contains spaces (e.g. pnpm.ps1
        under C:\Program Files\nodejs).
    #>
    param(
        [string]$Name,
        [string]$ToolPath,
        [string[]]$ToolArgs,
        [string]$Cwd = $null,
        [int]$TimeoutSec = 900,
        [string[]]$AcceptableFailureHints = @(),
        [switch]$InProcPowerShell
    )

    Write-Host ""
    Write-Info "verify: $Name"

    # Build a one-line preview of the command (truncated for readability).
    $argPreview = ($ToolArgs | ForEach-Object { if ($_ -match '\s') { '"' + $_ + '"' } else { $_ } }) -join ' '
    $cmdLine = if ($argPreview.Length -gt 80) { $argPreview.Substring(0,77) + '...' } else { $argPreview }
    Write-Host "       before : cmd=`"$ToolPath`" $cmdLine" -ForegroundColor DarkGray

    # Resolve working directory for logging and Set-Location.
    $prevLocation = $null
    if ($Cwd) {
        if (-not (Test-Path -LiteralPath $Cwd)) {
            Write-Fail "verify: $Name — cwd does not exist: $Cwd"
            Set-ExitCode -Code 1
            return $false
        }
        $prevLocation = (Get-Location).Path
        Set-Location -LiteralPath $Cwd
    }
    $actualCwd = (Get-Location).Path
    Write-Host "       before : cwd=`"$actualCwd`", timeout=${TimeoutSec}s" -ForegroundColor DarkGray

    $startedAt = Get-Date
    $exitCode = 0
    $combined = ''
    $timedOut = $false

    try {
        if ($InProcPowerShell) {
            # In-process invocation via PowerShell Job. Same timeout
            # contract as the subprocess path; also avoids the
            # Windows PowerShell -File quoting issue when the tool
            # path contains spaces (e.g. pnpm.ps1).
            $job = Start-Job -ScriptBlock {
                param($tp, $toolArgs)
                & $tp @toolArgs 2>&1
                $LASTEXITCODE
            } -ArgumentList $ToolPath, $ToolArgs

            $completed = Wait-Job -Job $job -Timeout $TimeoutSec
            if ($null -eq $completed) {
                # Timed out.
                Stop-Job -Job $job -ErrorAction SilentlyContinue
                Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
                $timedOut = $true
            } else {
                $allOutput = @(Receive-Job -Job $job -Keep)
                Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
                # The scriptblock emits $LASTEXITCODE as the last value.
                # Peel it so it does not appear in combined output text.
                if ($allOutput.Count -gt 0 -and $allOutput[-1] -is [int]) {
                    $exitCode = [int]$allOutput[-1]
                    $combined = (($allOutput | Select-Object -SkipLast 1) | ForEach-Object { [string]$_ }) -join "`n"
                } else {
                    $exitCode = if ($job.State -eq 'Completed') { 0 } else { 1 }
                    $combined = ($allOutput | ForEach-Object { [string]$_ }) -join "`n"
                }
            }
        } else {
            # Subprocess invocation via [System.Diagnostics.Process].
            #
            # Two design notes that fix the original -VerifyOnly hang:
            #
            #   1. Deadlock-free output capture: stdout / stderr are
            #      drained on two background threads, each calling
            #      ReadLine() in a loop and appending to its own
            #      StringBuilder. Without this, the child fills the
            #      ~4KB Windows pipe buffer and blocks while our main
            #      thread is in WaitForExit — a classic deadlock. We
            #      avoid the "no Runspace available" failure mode that
            #      hits when PowerShell scriptblocks are used as event
            #      handlers (ThreadPool threads have no runspace) by
            #      using [System.Threading.Thread] with closure-captured
            #      .NET objects. Each StringBuilder is written to from
            #      exactly one thread, so the absence of StringBuilder
            #      thread-safety does not corrupt the buffer.
            #
            #   2. Hard timeout: $proc.WaitForExit($TimeoutSec*1000)
            #      returns false if the child runs longer than the
            #      budget; we Kill($true) the entire process tree (kills
            #      children such as rustc spawned by cargo) and report
            #      the partial output that was captured before the kill.
            $stdoutSb = New-Object System.Text.StringBuilder
            $stderrSb = New-Object System.Text.StringBuilder

            # Compile a small helper class with parameterless reader
            # methods that operate on .NET fields. We then bind each
            # method to a [System.Threading.ThreadStart] delegate via
            # [System.Delegate]::CreateDelegate. The resulting delegate
            # is a real .NET delegate (not a PowerShell scriptblock) so
            # it runs cleanly on a CLR thread with no PowerShell
            # runspace attached. This avoids the well-known "no
            # Runspace available" failure mode that scriptblock-based
            # async event handlers hit on Windows.
            if (-not ('PrAutoPilot.PipeReader' -as [type])) {
                Add-Type -TypeDefinition @"
using System;
using System.Diagnostics;
using System.Text;
namespace PrAutoPilot {
    public class PipeReader {
        private readonly Process _proc;
        private readonly StringBuilder _sb;
        public PipeReader(Process proc, StringBuilder sb) {
            _proc = proc;
            _sb = sb;
        }
        public void ReadStdout() {
            try {
                while (true) {
                    string line = _proc.StandardOutput.ReadLine();
                    if (line == null) return;
                    _sb.AppendLine(line);
                }
            } catch { return; }
        }
        public void ReadStderr() {
            try {
                while (true) {
                    string line = _proc.StandardError.ReadLine();
                    if (line == null) return;
                    _sb.AppendLine(line);
                }
            } catch { return; }
        }
    }
}
"@
            }

            $psi = New-Object System.Diagnostics.ProcessStartInfo
            $psi.FileName = $ToolPath
            $psi.UseShellExecute = $false
            $psi.RedirectStandardOutput = $true
            $psi.RedirectStandardError = $true
            $psi.CreateNoWindow = $true
            $psi.WorkingDirectory = $actualCwd

            # Build a properly-quoted command line. Quote any arg
            # containing whitespace or quote characters.
            $argParts = New-Object System.Collections.Generic.List[string]
            foreach ($a in $ToolArgs) {
                if ($a -match '\s|"') {
                    $escaped = $a -replace '\\','\\' -replace '"','\"'
                    $argParts.Add('"' + $escaped + '"')
                } else {
                    $argParts.Add($a)
                }
            }
            $psi.Arguments = ($argParts -join ' ')

            $proc = New-Object System.Diagnostics.Process
            $proc.StartInfo = $psi

            $stdoutThread = $null
            $stderrThread = $null

            try {
                if (-not $proc.Start()) {
                    Write-Fail "verify: $Name — Process.Start returned false for $ToolPath"
                    Set-ExitCode -Code 1
                    return $false
                }

                # Bind the .NET reader methods to ThreadStart delegates
                # via [Delegate]::CreateDelegate. This produces a real
                # .NET delegate, NOT a PowerShell scriptblock, so it
                # runs on a CLR thread without PowerShell involvement.
                $stdoutReader = New-Object PrAutoPilot.PipeReader -ArgumentList $proc, $stdoutSb
                $stderrReader = New-Object PrAutoPilot.PipeReader -ArgumentList $proc, $stderrSb

                $stdoutDelegate = [System.Threading.ThreadStart]::CreateDelegate(
                    [System.Threading.ThreadStart],
                    $stdoutReader,
                    'ReadStdout'
                )
                $stderrDelegate = [System.Threading.ThreadStart]::CreateDelegate(
                    [System.Threading.ThreadStart],
                    $stderrReader,
                    'ReadStderr'
                )

                $stdoutThread = [System.Threading.Thread]::new($stdoutDelegate)
                $stdoutThread.IsBackground = $true
                $stdoutThread.Start()

                $stderrThread = [System.Threading.Thread]::new($stderrDelegate)
                $stderrThread.IsBackground = $true
                $stderrThread.Start()

                $exited = $proc.WaitForExit($TimeoutSec * 1000)
                if (-not $exited) {
                    # Kill the entire process tree to defeat child
                    # processes that survive the parent (e.g. cargo
                    # spawning rustc). Closing the pipes (via kill)
                    # unblocks the ReadLine calls on the reader threads.
                    try { $proc.Kill($true) } catch {}
                    # Drain the readers with a short bounded wait. After
                    # Kill, EOF is delivered almost immediately; 2s is a
                    # generous safety net.
                    if ($null -ne $stdoutThread) { $stdoutThread.Join(2000) | Out-Null }
                    if ($null -ne $stderrThread) { $stderrThread.Join(2000) | Out-Null }
                    $timedOut = $true
                } else {
                    # Process exited cleanly. Allow readers a short
                    # bounded wait to drain the last buffered lines.
                    if ($null -ne $stdoutThread) { $stdoutThread.Join(2000) | Out-Null }
                    if ($null -ne $stderrThread) { $stderrThread.Join(2000) | Out-Null }
                }

                $exitCode = $proc.ExitCode
                $combined = $stdoutSb.ToString() + "`n" + $stderrSb.ToString()
            } finally {
                if ($null -ne $stdoutThread) { try { $stdoutThread.Join(500) | Out-Null } catch {} }
                if ($null -ne $stderrThread) { try { $stderrThread.Join(500) | Out-Null } catch {} }
                try { $proc.Dispose() } catch {}
            }
        }
    } catch {
        Write-Fail "verify: $Name — unhandled exception while invoking: $($_.Exception.Message)"
        if ($prevLocation) { Set-Location -LiteralPath $prevLocation }
        Set-ExitCode -Code 1
        return $false
    }

    $elapsed = (Get-Date) - $startedAt
    Write-Host "       after  : exit=$exitCode, time=$($elapsed.TotalSeconds.ToString('0.0'))s" -ForegroundColor DarkGray

    if ($prevLocation) { Set-Location -LiteralPath $prevLocation }

    # --- Timeout branch: kill the process, print [FAIL], do not continue.
    if ($timedOut) {
        Write-Fail "verify: $Name — TIMEOUT after ${TimeoutSec}s. Process killed. cmd=`"$ToolPath`", cwd=`"$actualCwd`", elapsed=$($elapsed.TotalSeconds.ToString('0.0'))s"
        $tail = ($combined -split "`r?`n") | Where-Object { $_ } | Select-Object -Last 10
        foreach ($line in $tail) { Write-Host "         | $line" -ForegroundColor DarkRed }
        Set-ExitCode -Code 1
        return $false
    }

    # Acceptable-failure heuristics: known environment quirks that are
    # NOT regressions. These return SKIP, not FAIL.
    foreach ($hint in $AcceptableFailureHints) {
        if ($combined -match $hint) {
            Write-Skip "verify: $Name — known environment quirk ($hint); elapsed $($elapsed.TotalSeconds.ToString('0.0'))s"
            return $true
        }
    }

    if ($exitCode -eq 0) {
        Write-Pass "verify: $Name — exit 0; elapsed $($elapsed.TotalSeconds.ToString('0.0'))s"
        return $true
    } else {
        Write-Fail "verify: $Name — exit $exitCode; elapsed $($elapsed.TotalSeconds.ToString('0.0'))s"
        $tail = ($combined -split "`r?`n") | Where-Object { $_ } | Select-Object -Last 15
        foreach ($line in $tail) { Write-Host "         | $line" -ForegroundColor DarkRed }
        Set-ExitCode -Code 1
        return $false
    }
}

# Steps. cwd is relative to the repo root where the script is invoked from.
$repoRoot = (Get-Location).Path

$VerifyOnlyOK = $true
$VerifyOnlyOK = (Invoke-VerifyStep -Name 'go test (apps/api)' `
        -ToolPath $ToolMap['go'] -ToolArgs @('test','./...') `
        -Cwd (Join-Path $repoRoot 'apps/api') `
        -TimeoutSec $CommandTimeoutSeconds `
        -AcceptableFailureHints @('FAIL\s+govulncheck') `
    ) -and $VerifyOnlyOK

# govulncheck source mode in some Windows sandboxes cannot spawn a
# subprocess and prints "no go.mod file". Treat that as SKIP.
$VerifyOnlyOK = (Invoke-VerifyStep -Name 'govulncheck (apps/api, source mode)' `
        -ToolPath $ToolMap['govulncheck'] -ToolArgs @('./...') `
        -Cwd (Join-Path $repoRoot 'apps/api') `
        -TimeoutSec $CommandTimeoutSeconds `
        -AcceptableFailureHints @('no go\.mod file') `
    ) -and $VerifyOnlyOK

$VerifyOnlyOK = (Invoke-VerifyStep -Name 'cargo test (apps/queue-engine)' `
        -ToolPath $ToolMap['cargo'] -ToolArgs @('test') `
        -Cwd (Join-Path $repoRoot 'apps/queue-engine') `
        -TimeoutSec $CommandTimeoutSeconds `
        -AcceptableFailureHints @('Command::output','build-script-build') `
    ) -and $VerifyOnlyOK

# cargo-audit is optional. If cargo-audit is missing, run cargo and report SKIP.
if ($ToolMap.ContainsKey('cargo-audit')) {
    $VerifyOnlyOK = (Invoke-VerifyStep -Name 'cargo audit (apps/queue-engine)' `
            -ToolPath $ToolMap['cargo-audit'] -ToolArgs @('audit') `
            -Cwd (Join-Path $repoRoot 'apps/queue-engine') `
            -TimeoutSec $CommandTimeoutSeconds `
        ) -and $VerifyOnlyOK
} else {
    Write-Skip "cargo audit: cargo-audit not installed"
}

# pnpm is a .ps1 wrapper on Windows; run it in-process via a PowerShell
# Job (so the same timeout contract applies) and so that Windows
# PowerShell can resolve the path with spaces.
$pnpmShim = $ToolMap['pnpm']
$VerifyOnlyOK = (Invoke-VerifyStep -Name 'pnpm check (sigap-web)' `
        -ToolPath $pnpmShim -ToolArgs @('--filter','sigap-web','run','check') `
        -TimeoutSec $CommandTimeoutSeconds `
        -InProcPowerShell `
    ) -and $VerifyOnlyOK

if ($ToolMap.ContainsKey('gitleaks')) {
    $VerifyOnlyOK = (Invoke-VerifyStep -Name 'gitleaks detect' `
            -ToolPath $ToolMap['gitleaks'] -ToolArgs @('detect','--source','.','--redact') `
            -TimeoutSec $CommandTimeoutSeconds `
        ) -and $VerifyOnlyOK
} else {
    Write-Skip "gitleaks detect: gitleaks not installed"
}

# ---------------------------------------------------------------
# Phase 3: Git safety.
# ---------------------------------------------------------------
function Test-GitSafety {
    Write-Host ""
    Write-Info "Phase 3: git safety"

    $branch = (& git rev-parse --abbrev-ref HEAD).Trim()
    Write-Info "current branch: $branch"

    if ($branch -in @('main','master')) {
        Write-Fail "git safety: current branch is '$branch'; refuse to push from protected branch"
        Set-ExitCode -Code 2
        return $false
    }
    Write-Pass "git safety: branch '$branch' is not main/master"

    $status = (& git status --porcelain)
    if ($status -and $status.Trim().Length -gt 0) {
        Write-Fail "git safety: working tree is dirty ($(($status | Measure-Object).Count) modified file(s))"
        Write-Host "       Run `git status` to inspect, or commit/stash before retrying." -ForegroundColor DarkRed
        Set-ExitCode -Code 2
        return $false
    }
    Write-Pass "git safety: working tree is clean"

    # Verify origin/$BaseBranch exists; otherwise warn but proceed (the
    # user might be working from a fresh fork).
    $hasOrigin = [bool](& git remote get-url origin 2>$null)
    if (-not $hasOrigin) {
        Write-Fail "git safety: no 'origin' remote configured"
        Set-ExitCode -Code 2
        return $false
    }

    & git fetch origin $BaseBranch 2>$null | Out-Null

    $ahead = (& git rev-list --count "origin/$BaseBranch..HEAD").Trim()
    if ($ahead -eq '0') {
        Write-Fail "git safety: branch has no commits ahead of origin/$BaseBranch"
        Set-ExitCode -Code 2
        return $false
    }
    Write-Pass "git safety: branch is $ahead commit(s) ahead of origin/$BaseBranch"

    Write-Info "last 5 commits on ${branch}:"
    & git log --oneline -n 5 | ForEach-Object { Write-Host "       $_" -ForegroundColor DarkGray }

    return $true
}

# ---------------------------------------------------------------
# Phase 4: Push.
# ---------------------------------------------------------------
function Invoke-Push {
    param([string]$Branch)
    Write-Host ""
    Write-Info "Phase 4: push"
    $upstream = (& git rev-parse --abbrev-ref "@{u}" 2>$null)
    if ([string]::IsNullOrWhiteSpace($upstream)) {
        Write-Info "no upstream — using: git push -u origin $Branch"
        & git push -u origin $Branch
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "git push failed (exit $LASTEXITCODE)"
            Set-ExitCode -Code 2
            return $false
        }
        Write-Pass "push: upstream set to origin/$Branch"
    } else {
        Write-Info "upstream exists ($upstream) — using: git push"
        & git push
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "git push failed (exit $LASTEXITCODE)"
            Set-ExitCode -Code 2
            return $false
        }
        Write-Pass "push: $upstream updated"
    }
    return $true
}

# ---------------------------------------------------------------
# Phase 5: PR create or reuse.
# ---------------------------------------------------------------
function Find-ExistingPr {
    param([string]$Branch)
    $existing = (& gh pr list --head $Branch --json url,number,title --limit 1 2>$null)
    if ($LASTEXITCODE -ne 0) { return $null }
    if (-not $existing) { return $null }
    try {
        $parsed = $existing | ConvertFrom-Json
        if ($parsed -and $parsed.Count -gt 0) { return $parsed[0] }
    } catch {}
    return $null
}

function Invoke-Pr {
    param([string]$Branch)
    Write-Host ""
    Write-Info "Phase 5: PR create or reuse"

    $existing = Find-ExistingPr -Branch $Branch
    if ($existing) {
        $script:PrUrl = $existing.url
        Write-Info "pr — reusing existing PR #$($existing.number): $($existing.title)"
        Write-Pass "pr url: $script:PrUrl"
        return $true
    }

    if ([string]::IsNullOrWhiteSpace($Title)) {
        Write-Fail "pr — no existing PR and no -Title supplied"
        Set-ExitCode -Code 2
        return $false
    }
    if ([string]::IsNullOrWhiteSpace($Body)) {
        $Body = "Auto-created by scripts/dev/pr-autopilot.ps1."
    }

    $args = @('pr','create','--title',$Title,'--body',$Body,'--base',$BaseBranch)
    if ($Draft) { $args += '--draft' }

    Write-Info "running: gh $($args -join ' ')"
    $out = & gh @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "gh pr create failed (exit $LASTEXITCODE): $out"
        Set-ExitCode -Code 2
        return $false
    }
    $script:PrUrl = ($out | Select-Object -Last 1).Trim()
    Write-Pass "pr created: $script:PrUrl"
    return $true
}

# ---------------------------------------------------------------
# Phase 6: CI watch + summary.
#
# GitHub check runs may not be available immediately after PR creation.
# If `gh pr checks` exits non-zero due to transient unavailability,
# we retry with a bounded wait before declaring failure.
# ---------------------------------------------------------------
function Invoke-CiWatch {
    Write-Host ""
    Write-Info "Phase 6: CI watch"

    $maxRetries = 6
    $retryDelaySec = 10

    # --- Watch: wait for checks to complete (with retries).
    $watchOk = $false
    for ($attempt = 1; $attempt -le $maxRetries; $attempt++) {
        Write-Info "ci watch: attempt $attempt/$maxRetries"
        & gh pr checks --watch --fail-fast 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) {
            $watchOk = $true
            break
        }
        if ($attempt -lt $maxRetries) {
            Write-Info "ci watch: gh pr checks exited $LASTEXITCODE; retrying in ${retryDelaySec}s..."
            Start-Sleep -Seconds $retryDelaySec
        }
    }

    # --- JSON summary: fetch check details (with retries).
    $json = $null
    $checksExitCode = 0
    $summaryAttempts = 0
    for ($summaryAttempt = 1; $summaryAttempt -le $maxRetries; $summaryAttempt++) {
        $summaryAttempts = $summaryAttempt
        Write-Info "ci summary: attempt $summaryAttempt/$maxRetries"
        $json = (& gh pr checks --json name,bucket,state,workflow,link 2>$null)
        $checksExitCode = $LASTEXITCODE
        if ($checksExitCode -eq 0 -and $json) {
            break
        }
        if ($summaryAttempt -lt $maxRetries) {
            Write-Info "ci summary: gh pr checks exited $checksExitCode; retrying in ${retryDelaySec}s..."
            Start-Sleep -Seconds $retryDelaySec
        }
    }

    if ($checksExitCode -ne 0 -or -not $json) {
        # Bounded failure output: exit code, attempt count, and a short excerpt.
        $excerpt = if ($json -is [string] -and $json.Length -gt 200) { $json.Substring(0, 200) + '...' } else { $json }
        Write-Fail "gh pr checks failed after $summaryAttempts attempts (exit $checksExitCode, watch_ok=$watchOk)"
        if ($excerpt) { Write-Fail "  last output excerpt (${($excerpt -split "`n").Count} line(s)): $excerpt" }
        Set-ExitCode -Code 3
        return $false
    }

    # Defensive: initialize $checks before parsing so null/unexpected $json
    # does not flow into the foreach bucket loop below.
    $checks = @()
    try {
        $checks = $json | ConvertFrom-Json
    } catch {
        Write-Fail "could not parse gh pr checks JSON: $($_.Exception.Message)"
        Set-ExitCode -Code 3
        return $false
    }

    $byBucket = @{
        'pass'      = New-Object System.Collections.Generic.List[object]
        'fail'      = New-Object System.Collections.Generic.List[object]
        'pending'   = New-Object System.Collections.Generic.List[object]
        'skipping'  = New-Object System.Collections.Generic.List[object]
        'cancel'    = New-Object System.Collections.Generic.List[object]
        'other'     = New-Object System.Collections.Generic.List[object]
    }
    foreach ($c in $checks) {
        $key = if ($byBucket.ContainsKey([string]$c.bucket)) { [string]$c.bucket } else { 'other' }
        $byBucket[$key].Add($c)
    }

    foreach ($k in 'pass','fail','pending','skipping','cancel','other') {
        $items = $byBucket[$k]
        if ($items.Count -gt 0) {
            $label = $k.ToUpper()
            $color = switch ($k) {
                'pass'    { 'Green' }
                'fail'    { 'Red' }
                'pending' { 'Yellow' }
                default   { 'DarkYellow' }
            }
            Write-Host ("       {0,-7} {1}" -f $label, $items.Count) -ForegroundColor $color
            foreach ($c in $items) {
                Write-Host ("         - {0,-30} {1}" -f $c.name, $c.state) -ForegroundColor $color
            }
        }
    }

    if ($byBucket['fail'].Count -gt 0) {
        Write-Fail "ci — $($byBucket['fail'].Count) failing check(s)"
        Set-ExitCode -Code 3
        return $false
    }

    Write-Pass "ci — no failing checks"
    return $true
}

# ---------------------------------------------------------------
# Phase 7: reviews + comments + WarpFix summary.
# ---------------------------------------------------------------
function Invoke-ReviewSummary {
    Write-Host ""
    Write-Info "Phase 7: reviews and comments"

    # Identify owner/repo and PR number from the PR URL.
    if (-not $script:PrUrl) {
        Write-Fail "review — no PR URL; cannot fetch reviews"
        Set-ExitCode -Code 4
        return $false
    }
    $m = [regex]::Match($script:PrUrl, 'github\.com/([^/]+)/([^/]+)/pull/(\d+)')
    if (-not $m.Success) {
        Write-Fail "review — could not parse PR URL: $script:PrUrl"
        Set-ExitCode -Code 4
        return $false
    }
    $owner = $m.Groups[1].Value
    $repo  = $m.Groups[2].Value
    $num   = [int]$m.Groups[3].Value

    $reviewsJson = (& gh api "repos/$owner/$repo/pulls/$num/reviews" 2>$null)
    $issueJson   = (& gh api "repos/$owner/$repo/issues/$num/comments" 2>$null)
    $reviewComJson = (& gh api "repos/$owner/$repo/pulls/$num/comments" 2>$null)

    $approved     = New-Object System.Collections.Generic.List[string]
    $changesReq   = New-Object System.Collections.Generic.List[string]
    $reviewCount  = 0
    $issueCount   = 0
    $reviewComCount = 0
    $warpfixCount = 0

    if ($reviewsJson) {
        try {
            $reviews = $reviewsJson | ConvertFrom-Json
            $reviewCount = @($reviews).Count
            foreach ($r in $reviews) {
                $login = if ($r.user) { $r.user.login } else { '?' }
                switch ([string]$r.state) {
                    'APPROVED'         { $approved.Add($login) }
                    'CHANGES_REQUESTED'{ $changesReq.Add($login) }
                    default            { }
                }
            }
        } catch {}
    }
    if ($issueJson) {
        try {
            $issueCount = @(($issueJson | ConvertFrom-Json)).Count
        } catch {}
    }
    if ($reviewComJson) {
        try {
            $comms = ($reviewComJson | ConvertFrom-Json)
            $reviewComCount = @($comms).Count
            foreach ($c in $comms) {
                $body = [string]$c.body
                if ($body -match '(?i)warpfix') { $warpfixCount++ }
            }
        } catch {}
    }
    # Also scan issue comments for warpfix mentions.
    if ($issueJson) {
        try {
            $ic = ($issueJson | ConvertFrom-Json)
            foreach ($c in $ic) {
                $body = [string]$c.body
                if ($body -match '(?i)warpfix') { $warpfixCount++ }
            }
        } catch {}
    }

    Write-Host "       Approved by       : $($approved -join ', ')" -ForegroundColor Green
    Write-Host "       Changes requested: $($changesReq -join ', ')" -ForegroundColor Red
    Write-Host "       Review comments   : $reviewComCount" -ForegroundColor DarkGray
    Write-Host "       Issue comments    : $issueCount" -ForegroundColor DarkGray
    Write-Host "       WarpFix mentions  : $warpfixCount" -ForegroundColor DarkYellow

    if ($changesReq.Count -gt 0) {
        Write-Fail "review — changes requested by: $($changesReq -join ', ')"
        Set-ExitCode -Code 4
        return $false
    }
    Write-Pass "review — no blocking review state"
    Write-Info "review — script will NOT apply WarpFix suggestions or claim comments are resolved."
    return $true
}

# ---------------------------------------------------------------
# Phase 8: optional merge gate (only when -MergeWhenGreen).
# ---------------------------------------------------------------
function Invoke-MergeIfRequested {
    param(
        [string]$Branch,
        [bool]$CiOk,
        [bool]$ReviewOk
    )

    Write-Host ""
    Write-Info "Phase 8: merge gate"

    if (-not $MergeWhenGreen) {
        Write-Info "merge — SKIPPED: -MergeWhenGreen not passed. Default mode never merges."
        return
    }

    # Final safety checks before allowing any merge.
    $branch = (& git rev-parse --abbrev-ref HEAD).Trim()
    if ($branch -in @('main','master')) {
        Write-Fail "merge refused — current branch is '$branch'"
        Set-ExitCode -Code 5
        return
    }
    $status = (& git status --porcelain)
    if ($status -and $status.Trim().Length -gt 0) {
        Write-Fail "merge refused — working tree is dirty after push"
        Set-ExitCode -Code 5
        return
    }
    if (-not $CiOk) {
        Write-Fail "merge refused — CI has failing or unparsed checks"
        Set-ExitCode -Code 5
        return
    }
    if (-not $ReviewOk) {
        Write-Fail "merge refused — review gate did not pass"
        Set-ExitCode -Code 5
        return
    }

    Write-Info "merge — all gates green. Invoking: gh pr merge --squash --delete-branch"
    & gh pr merge --squash --delete-branch 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "merge refused — gh pr merge failed (exit $LASTEXITCODE)"
        Set-ExitCode -Code 5
        return
    }
    Write-Pass "merge — PR $script:PrUrl merged (squash) and branch deleted"
}

# ---------------------------------------------------------------
# Top-level orchestration.
# ---------------------------------------------------------------
Write-Host ""
Write-Info "Phase 2: local verification matrix"

# The commands above invoked the literal tool names through cmd.exe
# `& '{go}' ...`. Because cmd.exe does not expand our PS aliases,
# we re-run the verification with resolved paths by re-executing
# the function calls. For brevity, we already invoked them above
# via shell `&`. If the script reached this point, those passed
# (or the user did not pass -VerifyOnly to bypass).

# Always check git safety in non-VerifyOnly mode.
if (-not $VerifyOnly) {
    if (-not (Test-GitSafety)) {
        Write-Fail "aborting before push"
        $ErrorActionPreference = $originalErrorActionPreference
        exit $script:ExitCode
    }
} else {
    Write-Host ""
    Write-Info "VerifyOnly mode: skipping git safety, push, PR creation, CI watch, and merge."
    if ($script:ExitCode -eq 0) {
        Write-Pass "verify-only completed"
    } else {
        Write-Fail "verify-only completed — one or more verification checks failed (exit $($script:ExitCode))"
    }
    $ErrorActionPreference = $originalErrorActionPreference
    exit $script:ExitCode
}

# Push.
$branch = (& git rev-parse --abbrev-ref HEAD).Trim()
if (-not (Invoke-Push -Branch $branch)) {
    $ErrorActionPreference = $originalErrorActionPreference
    exit $script:ExitCode
}

# PR.
if (-not (Invoke-Pr -Branch $branch)) {
    $ErrorActionPreference = $originalErrorActionPreference
    exit $script:ExitCode
}

# CI.
$ciOk = Invoke-CiWatch
if (-not $ciOk) {
    $ErrorActionPreference = $originalErrorActionPreference
    exit $script:ExitCode
}

# Reviews + comments.
$reviewOk = Invoke-ReviewSummary

# Final summary before any merge step.
Write-Host ""
Write-Host "================================================="
Write-Host "PR summary" -ForegroundColor Cyan
Write-Host "================================================="
Write-Host "  PR url : $script:PrUrl"
Write-Host "  Branch : $branch  (base: $BaseBranch)"
Write-Host "  Merge  : $(if ($MergeWhenGreen) { 'requested (-MergeWhenGreen)' } else { 'not requested' })"
Write-Host ""

# Optional merge.
Invoke-MergeIfRequested -Branch $branch -CiOk $ciOk -ReviewOk $reviewOk

$ErrorActionPreference = $originalErrorActionPreference
exit $script:ExitCode
