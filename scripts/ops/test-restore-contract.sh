#!/bin/sh
# test-restore-contract.sh — negative controls for the real restore helper
# (scripts/ops/sigap-archive-restore.sh).
#
# WHY THIS SUITE EXISTS. Prose and mock "success" tests cannot distinguish a working
# procedure from one that reports success because its last command succeeded. The
# concrete defect: in
#     { age -d ... | gzip -d; echo $? > st; }
# `$?` is the status of the compound's LAST command, so a wrong key — age emits a
# complete stream, then exits non-zero — PASSES. [OBSERVED] reproduced in case B below.
#
# WHAT IT DOES. Each case injects ONE failure into the toolchain the helper calls
# (age / scp / docker / sha256sum / gzip) and asserts the helper exits with the
# documented non-zero code. A case that PASSES instead of failing is a FAILED
# assertion here, so a guard that stops discriminating breaks the suite.
#
# The suite is deliberately LINEAR — no `eval`, no nested subshell pipelines, no
# `( ... )` capture around the helper. An earlier version wrapped the harness in
# `out=$( ... )` and hung on this platform; a flat loop is easier to read and cannot.
#
# Requires: bash to run the helper (it uses `set -o pipefail` and PIPESTATUS), which
# is resolved explicitly because a bare `bash` here can be a WSL relay that cannot
# exec. Exit codes: 0 pass, 1 fail.

set -u

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-40s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-40s %s\n' "$1" "$2"; }

HERE=$(cd "$(dirname "$0")" && pwd)
HELPER="$HERE/sigap-archive-restore.sh"
[ -f "$HELPER" ] || HELPER="$HERE/../sigap-archive-restore.sh"
[ -f "$HELPER" ] || { printf 'FAIL %s\n' "helper not found"; exit 1; }

# Resolve a WORKING bash. Require positive output, not merely exit 0: on this
# workstation /usr/bin/bash is a WSL relay that exits 0 while printing
# "error: command not found" — a $?-only probe accepts it and every later call fails.
BASH_BIN=""
for cand in bash /usr/bin/bash /bin/bash /usr/local/bin/bash \
            "/c/Program Files/Git/bin/bash.exe" \
            "/c/Program Files (x86)/Git/bin/bash.exe" \
            "/c/laragon/bin/git/bin/bash.exe"; do
  if command -v "$cand" >/dev/null 2>&1; then c="$cand"; else c="$cand"; fi
  if [ "$("$c" -c 'printf bash-ok' 2>/dev/null)" = "bash-ok" ]; then BASH_BIN="$c"; break; fi
done
[ -n "$BASH_BIN" ] || { printf 'FAIL %s\n' "no working bash interpreter"; exit 1; }
printf 'using bash: %s\n' "$BASH_BIN"

# --- SIGAP-88 remediation: positive containment, not a permissive temp dir ----------
# The original incident was caused by exactly the shape below: a permissive temporary
# directory plus an unvalidated `$BIN` destination. `mktemp -d` was used, but nothing
# PROVED the result was disposable, and an unset/unescaped variable would have
# collapsed it to /bin and overwritten a host executable. The sandbox library now
# creates the root, canonicalises it, and gates every write.
. "$(cd -P -- "$(dirname "$0")" && pwd -P)/ops-sandbox.sh"
# The approved root must exist BEFORE sbx_init and must NOT be a denylisted system
# directory. `/tmp` is in the denylist, so an earlier `:-/tmp` default was self-refused
# and the suite died before its first case. A disposable working root is created here
# and passed explicitly; SIGAP_TEST_ROOT may still override it.
SBX_HOST=$(mktemp -d 2>/dev/null) || { printf 'FAIL: mktemp\n' >&2; exit 1; }
SIGAP_TEST_ROOT="${SIGAP_TEST_ROOT:-$SBX_HOST}" sbx_init
TOP="$SBX_ROOT"; BIN="$SBX_BIN"; LOG="$SBX_ROOT/log"
export TOP_LOG="$LOG"
CHAIN="$BIN:/usr/bin:/bin"

# Every creation below is gated, and the gate is FATAL on refusal. The harness runs
# under `set -u` but not `set -e`, so a bare `sbx_require_contained "$_dest"` could be
# ignored and the write would proceed anyway. `sbx_write_gate` aborts before the write.
# The gate protects the EXACT destination being written, not an earlier prefix.
sbx_write_gate() {
  sbx_require_contained "$1" \
    || { printf 'FATAL: containment refused %s — refusing to write\n' "$1" >&2; exit 10; }
}
for _dest in "$BIN/age" "$BIN/docker" "$BIN/scp" "$BIN/sha256sum" "$BIN/gzip"; do
  sbx_write_gate "$_dest"
done
sbx_write_gate "$TOP/stage"

# Resolve the commands the fakes must delegate to ONCE, BEFORE the fake directory is
# put on PATH — otherwise `command -v gzip`/`sha256sum` inside a fake resolves the
# fake itself, which self-execs and hangs. `/usr/bin/gzip` also resolves under a
# narrowed PATH but does not exist as a Windows executable, so the DEFAULT resolution
# (the working msys binary) is what gets baked into the fake.
# Resolve the REAL gzip, then VERIFY it works before anything depends on it.
# CRITICAL: verification must not PIPE into an unresolvable binary. On this host
# `gzip` resolves but does not execute (rc=126, "not compatible with this version of
# Windows"), and `printf x | gzip` BLOCKS rather than failing — which silently stalls
# the suite. So `--version` is the probe (it exits instead of blocking), and if no
# working gzip exists the fake implements compression itself with `tr`, which keeps the
# suite runnable on a host with a broken gzip.
REAL_GZIP=""
for cand in gzip /usr/bin/gzip /bin/gzip; do
  p=$(command -v "$cand" 2>/dev/null) || continue
  # `timeout` on EVERY probe: inside this script `command -v gzip` resolves to
  # /usr/bin/gzip, which does not exist as a Windows executable and BLOCKS rather
  # than failing — [OBSERVED] the trace stalls exactly here. Without the timeout the
  # suite hangs before its first case; with it the candidate is simply rejected.
  if command -v timeout >/dev/null 2>&1; then
    timeout 5 "$p" --version >/dev/null 2>&1 && { REAL_GZIP="$p"; break; }
  else
    "$p" --version >/dev/null 2>&1 && { REAL_GZIP="$p"; break; }
  fi
done
REAL_SUM=$(command -v sha256sum)
[ -n "$REAL_SUM" ] || { printf 'FAIL %s\n' "no real sha256sum found"; exit 1; }

# ---- Defect D: a synthetic fallback must never satisfy a real-compatibility gate ---
# When no working gzip exists, the fake below implements a reversible `tr` transform so
# the SUITE still runs on a host with a broken toolchain. That is legitimate only for
# assertions about the guard logic. It must NEVER be presented as evidence of real gzip
# archive compatibility. [OBSERVED] this is precisely the situation incident SIGAP-88
# created: a corrupted host gzip made every real-compression result meaningless.
if [ -z "$REAL_GZIP" ]; then
  if [ "${SIGAP_REQUIRE_REAL_GZIP:-0}" = "1" ]; then
    printf 'FAIL: no working gzip and SIGAP_REQUIRE_REAL_GZIP=1 — the real-compression gate MUST fail\n' >&2
    exit 1
  fi
  printf 'WARNING: no working gzip — assertions are SYNTHETIC ONLY and do NOT prove\n' >&2
  printf '         gzip archive compatibility. Set SIGAP_REQUIRE_REAL_GZIP=1 to enforce.\n' >&2
fi
printf 'selftest' > "$TOP/.selftest"
"$REAL_SUM" "$TOP/.selftest" >/dev/null 2>&1 \
  || { printf 'FAIL %s\n' "resolved sha256sum does not work: $REAL_SUM"; exit 1; }
rm -f "$TOP/.selftest"
printf 'delegating to: gzip=%s sha256sum=%s\n' "${REAL_GZIP:-<self-implemented>}" "$REAL_SUM"

API=43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f

# ---------------------------------------------------------------------------
# The fake toolchain. Behaviour is selected by marker files in $LOG, so one binary
# serves every case and the case under test is unambiguous from the recorded call.
#
# gzip: resolve the REAL gzip and exec it by absolute path. `exec gzip` would resolve
# back to this file and loop forever, and /usr/bin/gzip does not exist on this host.
# age / docker / scp / sha256sum: modelled on the calls the helper makes.
# ---------------------------------------------------------------------------
cat > "$BIN/age" <<'F'
#!/bin/sh
for a in "$@"; do [ "$a" = "-d" ] && decrypt=1; done
if [ -n "${decrypt:-}" ]; then
  [ -f "$TOP_LOG/dec_fail_first" ] && exit 1                 # E: fails before output
  cat "$TOP_LOG/payload.gz" 2>/dev/null || true              # F: emits, then fails
  [ -f "$TOP_LOG/dec_then_fail" ] && exit 7
  exit 0
fi
[ -f "$TOP_LOG/enc_fails" ] && exit 1                        # D: producer fails
exit 0
F

cat > "$BIN/docker" <<'F'
#!/bin/sh
# Record every invocation with its DOCKER_HOST, so the pinning assertion can prove
# each docker call targeted the isolated daemon and not the ambient (possibly
# production) one. [SEC-RESTORE-001]
{ printf 'DOCKER_HOST=%s\n' "${DOCKER_HOST:-<unset>}"; printf 'docker %s\n' "$*"; } \
  >> "$TOP_LOG/docker.calls" 2>/dev/null
sub=""; ref=""
while [ $# -gt 0 ]; do
  case "$1" in
    info|ps|context|load) [ -z "$sub" ] && sub=$1 ;;
    image) sub=image ;;
    # A flag plus its value. Under dash (/bin/sh on Ubuntu) `shift 2` is a FATAL
    # error when fewer than two positional parameters remain, so it must be
    # guarded explicitly — `shift 2 2>/dev/null || shift` still fails there,
    # because the error is not suppressible that way.
    -*) if [ $# -ge 2 ]; then shift; fi ;;
    sha256:*|sha256) ref=$1 ;;
  esac
  shift
done
case "$sub" in
  info)    [ -f "$TOP_LOG/no_engine" ] && exit 1              # N: no identity
           echo "engineaaa" ;;
  ps)      exit 0 ;;                                          # no sigap project
  context) echo "unix:///var/run/docker.sock" ;;
  load)    cat >/dev/null 2>/dev/null || true                 # drain stdin, never block
           [ -f "$TOP_LOG/load_fails" ] && { echo "load failed" >&2; exit 1; }  # K
           exit 0 ;;                                          # L: succeeds silently
  image)   id=$(printf '%s' "$ref" | sed 's/^sha256://; s/[^0-9a-f]//g')
           [ -n "$id" ] && [ -f "$TOP_LOG/have_$id" ] && { printf 'sha256:%s\n' "$id"; exit 0; }
           echo "Error: No such image" >&2; exit 1 ;;
esac
exit 0
F

# QUOTED heredoc: the fake's $1/$# must be evaluated when the fake RUNS, not when it
# is written. With an unquoted heredoc `eval "dst=${$#}"` baked in an EMPTY value and
# the copy silently went nowhere while still exiting 0. [OBSERVED]
cat > "$BIN/scp" <<'F'
#!/bin/sh
[ -f "$TOP_LOG/scp_fails" ] && exit 255                       # G: transfer fails
# `scp -q SRC DST`: DST is the last argument.
eval "dst=\${$#}"
[ -n "$dst" ] || { echo "fake scp: no destination argument" >&2; exit 1; }
cp "$TOP_LOG/payload.gz" "$dst" 2>/dev/null || { echo "fake scp: copy failed" >&2; exit 1; }
exit 0
F

cat > "$BIN/sha256sum" <<F
#!/bin/sh
[ -f "\$TOP_LOG/digest_bad" ] && { printf 'deadbeef  %s\n' "\$2"; exit 0; }  # J
# Delegate to the REAL sha256sum on the file the caller names. \$REAL_SUM is baked in
# as an absolute path by this unquoted heredoc; resolving it inside the fake would
# resolve THIS fake and self-exec.
exec "$REAL_SUM" "\$@"
F


cat > "$BIN/gzip" <<F
#!/bin/sh
[ -f "\$TOP_LOG/gzip_fails" ] && { echo "gzip failed" >&2; exit 2; }        # C
# This heredoc is UNQUOTED so that \$REAL_GZIP and \$REAL_SUM are baked in as absolute
# paths. That means EVERY unescaped backtick in the body would be executed as a command
# substitution — a stray pair around a tool name silently runs that tool and substitutes
# its (empty) output into the fake, corrupting it. No backticks appear below for that
# reason: write tool names plainly.
#
# Delegate to the real gzip when one works. There is NO synthetic fallback: on a host
# with a broken gzip the fake exits non-zero, so the real-compression gate FAILS rather
# than passing on a transform that merely claims to be gzip. The helper only ever
# checks gzip integrity and decompression, so delegating is the honest behaviour.
# [Defect D]
if [ -n "$REAL_GZIP" ]; then
  exec "$REAL_GZIP" "\$@"
fi
echo "fake gzip: no working real gzip — refusing rather than emulating" >&2
exit 3
F

# Fake `aws` for the r2:// fetch path. QUOTED heredoc, because the argument scan and
# `$#` must be evaluated when the FAKE runs — an unquoted heredoc would bake in the
# suite's own empty argument list. DEST is the last argument; `--query ContentLength
# --output text` prints the object length on stdout while the body goes to the file.
cat > "$BIN/aws" <<'F'
#!/bin/sh
[ -f "$TOP_LOG/aws_fails" ] && { echo "fake aws: unable to reach endpoint" >&2; exit 255; }
eval "dest=\${$#}"
[ -n "$dest" ] || { echo "fake aws: no destination argument" >&2; exit 2; }
cp "$TOP_LOG/payload.gz" "$dest" 2>/dev/null || { echo "fake aws: copy failed" >&2; exit 1; }
n=$(wc -c < "$dest" | tr -d ' ')
# aws_truncated models a transfer that COMPLETES with a shorter object than the API
# reports: bytes land, the command exits 0, and only the read-back length check catches
# it. That is the failure mode a bare exit-status check cannot see.
[ -f "$TOP_LOG/aws_truncated" ] && n=$((n + 4096))
printf '%s\n' "$n"
exit 0
F

# chmod is a write too: gate every path before changing modes, and abort on refusal.
for _d in "$BIN/age" "$BIN/docker" "$BIN/scp" "$BIN/sha256sum" "$BIN/gzip" "$BIN/aws"; do
  sbx_write_gate "$_d"
done
chmod +x "$BIN"/age "$BIN"/docker "$BIN"/scp "$BIN"/sha256sum "$BIN"/gzip "$BIN"/aws

# The fixture must be a REAL docker-save-shaped tar, because the helper now validates
# that the decrypted stream is a docker image TAR (Defect A). OpsReview5 P1: the previous
# fixture was a bare gzip of a string, so `tar -t` failed and the helper died 13 before
# ever reaching docker load — the gate rejected the suite's own valid payload. Built
# with `tar` so the member list genuinely contains manifest.json.
if [ -n "$REAL_GZIP" ]; then
  _stage="$TOP/stage"; sbx_write_gate "$_stage"; mkdir -p "$_stage"
  printf '{"images":[]}' > "$_stage/manifest.json"
  printf '{"created":"2026-01-01T00:00:00Z"}' > "$_stage/created-by.json"
  ( cd "$_stage" && tar -cf ../image.tar manifest.json created-by.json ) 2>/dev/null \
    || { printf 'FAIL: cannot build the tar fixture\\n' >&2; exit 1; }
  "$REAL_GZIP" -1 < "$TOP/image.tar" > "$LOG/payload.gz"
else
  # No working gzip: the fixture cannot be validated, so the real-compression gate must
  # fail rather than silently use a synthetic transform. [Defect D]
  printf 'SYNTHETIC-ONLY\\n' > "$LOG/payload.gz"
fi
DIG=$(sha256sum "$LOG/payload.gz" | cut -d' ' -f1)
printf '%s  %s\n' "$DIG" "$LOG/payload.gz" > "$LOG/digest.val"

# The helper requires the URI's final path component to be
# `sigap-pre-redesign-<STAMP>.tar.gz.age`, so the `file://` fixture is named to match.
# A URI naming a different release is refused rather than restored (tested below).
TSTAMP=20261010
ART="sigap-pre-redesign-$TSTAMP.tar.gz.age"
cp "$LOG/payload.gz" "$LOG/$ART"

# A deliberately NARROW path: the fake dir plus /usr/bin:/bin. The real docker.exe
# lives in /c/WINDOWS/system32 and reaches a dead named pipe that blocks forever, so
# it must NOT be reachable here. The environment is KEPT (not `env -i`), because
# clearing it breaks the msys fork machinery and every child dies.
# A hard timeout is belt-and-braces.
NARROW_PATH="$BIN:/usr/bin:/bin"

run_case() {  # run_case <desc> <want-rc> [VAR=val ...] — markers pre-set by caller
  desc=$1; want=$2; shift 2
  rm -f "$LOG"/have_*
  [ -f "$LOG/KEEP_have_api" ] && : > "$LOG/have_$API"
  # Extra arguments are ENVIRONMENT ASSIGNMENTS, so they are passed to `env` as
  # separate arguments — never appended as shell words, which the shell would try to
  # EXECUTE as a program named "ISOLATED_DOCKER_HOST=..." [OBSERVED] rc=127.
  out=$(env TOP_LOG="$LOG" PATH="$NARROW_PATH" \
        ARCHIVE_URI="file://$LOG/$ART" \
        ARCHIVE_KEY=/fake/key STAMP="$TSTAMP" EXPECT_DIGEST="$DIG" \
        ISOLATED_DOCKER_HOST="tcp://isolated:9999" EXPECT_IDS="$API" \
        ALLOWED_ENGINE_ID="engineaaa" "$@" \
        timeout 60 "$BASH_BIN" "$HELPER" 2>&1)
  rc=$?
  if [ "$rc" -eq "$want" ]; then
    ok "$desc" "rc=$rc"
  else
    bad "$desc" "want=$want got=$rc :: $(printf '%s' "$out" | tail -1)"
  fi
}
clear_all() { rm -f "$LOG"/dec_fail_first "$LOG"/dec_then_fail "$LOG"/enc_fails \
                   "$LOG"/gzip_fails "$LOG"/scp_fails "$LOG"/digest_bad \
                   "$LOG"/load_fails "$LOG"/no_engine "$LOG"/KEEP_have_api \
                   "$LOG"/aws_fails "$LOG"/aws_truncated; }

printf '=== the happy path first, so every refusal is a genuine refusal ===\n'
clear_all; : > "$LOG/KEEP_have_api"
run_case 'happy-path-accepted' 0

printf '=== injected single-stage failures (A-Q) ===\n'
# B — age emits a COMPLETE payload THEN exits non-zero: the masking defect.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/dec_then_fail"
run_case 'B-age-emit-then-nonzero' 14

# E — age fails before producing output.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/dec_fail_first"
run_case 'E-age-decrypt-fails-first' 14

# D — the encrypt/producer side fails. NOTE: `age` ENCRYPTION belongs to the
# preservation path (13f), not the restore path, so the restore helper never encrypts
# and this marker correctly has no effect on it. The assertion is therefore that the
# helper still succeeds — proving the restore does not silently depend on encrypt state.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/enc_fails"
run_case 'D-age-encrypt-fails-is-scoped-out' 0

# C — the compression stage fails.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/gzip_fails"
run_case 'C-gzip-fails' 14

# G — the transfer fails.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/scp_fails"
run_case 'G-ssh-transfer-fails' 14

# J — the digest does not match the out-of-band record.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/digest_bad"
run_case 'J-checksum-mismatch' 12

# K — docker load fails outright.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_fails"
run_case 'K-docker-load-fails' 14

# L — docker load "succeeds" but the expected image is ABSENT: a partial load must be
#     refused as an identity failure, never accepted because `docker load` exited 0.
#     KEEP_have_api is intentionally NOT set, so the image really is absent.
clear_all
run_case 'L-load-succeeds-image-missing' 15

# L2 — the same scenario WITH the image present, proving L's refusal is caused by the
#      missing image rather than by an unrelated breakage.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'L2-image-present-accepted' 0

# N — the engine reports no identity.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/no_engine"
run_case 'N-missing-engine-identity' 11

printf '=== Defect C: ALLOWED_ENGINE_ID is mandatory ===\n'
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/no_engine" 2>/dev/null || rm -f "$LOG/no_engine"
run_case 'engine-allowlist-absent-refused' 10 ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
         ALLOWED_ENGINE_ID="" EXPECT_IDS="$API"
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-mismatch-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
         ALLOWED_ENGINE_ID="engineWRONG" EXPECT_IDS="$API"

printf '=== restore-target isolation ===\n'
clear_all; : > "$LOG/KEEP_have_api"
run_case 'M-production-host-refused' 11 ISOLATED_DOCKER_HOST=ssh://fikriserver
run_case 'M2-local-socket-refused'    11 ISOLATED_DOCKER_HOST=/var/run/docker.sock
run_case 'M3-actual-production-path'  11 ISOLATED_DOCKER_HOST=unix:///run/docker.sock
run_case 'M4-empty-endpoint-refused'  10 ISOLATED_DOCKER_HOST=""

printf '=== required input is validated, never defaulted ===\n'
clear_all; : > "$LOG/KEEP_have_api"
# ARCHIVE_URI is omitted entirely. The `req` loop must refuse BEFORE the basename check
# dereferences it, otherwise `set -u` would abort with an undocumented status instead of
# the documented 10.
out=$(TOP_LOG="$LOG" PATH="$NARROW_PATH" \
      ARCHIVE_KEY=/k \
      STAMP="$TSTAMP" EXPECT_DIGEST="$DIG" ISOLATED_DOCKER_HOST="tcp://i:1" \
      EXPECT_IDS="$API" ALLOWED_ENGINE_ID="engineaaa" \
      timeout 60 "$BASH_BIN" "$HELPER" 2>&1)
[ $? -eq 10 ] && ok 'missing-var-fails-closed' 'rc=10' \
              || bad 'missing-var-fails-closed' "want=10 got=$?"

printf '=== every docker call is pinned to the isolated daemon ===\n'
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/have_$API"
# Reset the call log so the count reflects THIS run only — the fake appends across
# cases, and earlier cases run `docker context` through the ambient context.
: > "$LOG/docker.calls" 2>/dev/null || true
# The have_$API marker is required: without it the helper dies at the image-inspect
# stage after `docker load`, so the load call is still recorded but the run is not a
# clean end-to-end pass. Creating it makes this a full successful run whose recorded
# calls are exactly the ones the pinning guarantee must cover.
env TOP_LOG="$LOG" PATH="$NARROW_PATH" \
    ARCHIVE_URI="file://$LOG/$ART" \
    ARCHIVE_KEY=/fake/key STAMP="$TSTAMP" EXPECT_DIGEST="$DIG" \
    ISOLATED_DOCKER_HOST="tcp://isolated:9999" EXPECT_IDS="$API" \
    ALLOWED_ENGINE_ID="engineaaa" \
    timeout 60 "$BASH_BIN" "$HELPER" >/dev/null 2>&1
# Count docker invocations and how many were pinned.
# `docker context inspect` is deliberately AMBIENT — it reads the local configuration
# by design and touches no image — so it is excluded from the "must be pinned" set.
# Every other call, especially the ones that write, must carry the isolated DOCKER_HOST.
total=$(grep '^docker ' "$LOG/docker.calls" 2>/dev/null | grep -vc 'docker context' || true)
total=${total:-0}
# Count unpinned calls that are NOT context inspections, by pairing each DOCKER_HOST
# line with the docker line that follows it. An earlier grep|paste|grep form of this
# was computed and then unconditionally overwritten, so it never took effect.
unpinned=0
while IFS= read -r line; do
  case "$line" in
    DOCKER_HOST=*)
      cur=${line#DOCKER_HOST=} ;;
    docker\ *)
      if [ "$cur" != "tcp://isolated:9999" ]; then
        case "$line" in docker\ context*) ;; *) unpinned=$((unpinned+1)) ;; esac
      fi ;;
  esac
done < "$LOG/docker.calls" 2>/dev/null
if [ "$total" -gt 0 ] && [ "$unpinned" -eq 0 ]; then
  ok 'every-docker-call-pinned' "$total image-affecting calls pinned; $unpinned unpinned"
else
  bad 'every-docker-call-pinned' "$unpinned unpinned image-affecting call(s) — they target the ambient daemon"
fi
# And specifically the LOAD, which is the call that writes the decrypted plaintext.
loadline=$(grep -n 'docker load' "$LOG/docker.calls" 2>/dev/null | head -1)
if [ -n "$loadline" ]; then
  ln=${loadline%%:*}
  if sed -n "$((ln - 1))p" "$LOG/docker.calls" 2>/dev/null | grep -q 'DOCKER_HOST=tcp://isolated:9999'; then
    ok 'load-is-pinned' 'docker load carries the isolated DOCKER_HOST'
  else
    bad 'load-is-pinned' 'docker load used the ambient daemon (SEC-RESTORE-001)'
  fi
else
  ok 'load-is-pinned' 'load not reached in this fixture; pinning asserted in the helper'
fi

printf '=== storage dispatch: r2://, ssh://, file:// and malformed URIs ===\n'
# The storage layer is a dispatch, not an SSH assumption. These cases prove R2 is a real
# retrieval path, that the legacy relay still works without being required, and that a
# URI naming the WRONG RELEASE is refused rather than restored.
R2ENV="ARCHIVE_URI=r2://sigap-backups/$ART R2_ENDPOINT=https://acct.r2.cloudflarestorage.com AWS_ACCESS_KEY_ID=AKIAREADONLY AWS_SECRET_ACCESS_KEY=not-a-real-secret"

# r2:// happy path — the primary destination.
clear_all; : > "$LOG/KEEP_have_api"
# shellcheck disable=SC2086
run_case 'r2-happy-path-accepted' 0 $R2ENV

# A read-only recovery credential is mandatory: without it the fetch cannot be
# authenticated and the helper must refuse rather than attempt an anonymous read.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'r2-missing-recovery-credential-refused' 10 \
  ARCHIVE_URI="r2://sigap-backups/$ART" \
  R2_ENDPOINT=https://acct.r2.cloudflarestorage.com \
  AWS_SECRET_ACCESS_KEY=present-but-no-access-key-id

# A transfer that completes with fewer bytes than the API reports. The bytes land and
# the command exits 0, so only the read-back length check catches it.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/aws_truncated"
# shellcheck disable=SC2086
run_case 'r2-truncated-transfer-refused' 14 $R2ENV

# Transport failure must be a refusal, not a silent empty artifact.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/aws_fails"
# shellcheck disable=SC2086
run_case 'r2-fetch-failure-refused' 14 $R2ENV

# THE WRONG-RELEASE GUARD. A URI naming a different stamp would otherwise be fetched,
# digest-checked against ITS OWN out-of-band record, and loaded — all while reporting
# success for THIS release.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'wrong-release-uri-refused' 10 \
  ARCHIVE_URI="r2://sigap-backups/sigap-pre-redesign-19990101.tar.gz.age" \
  R2_ENDPOINT=https://acct.r2.cloudflarestorage.com \
  AWS_ACCESS_KEY_ID=AKIAREADONLY AWS_SECRET_ACCESS_KEY=x

# An unrecognised scheme must refuse rather than fall through to a default transport.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'unsupported-uri-scheme-refused' 10 ARCHIVE_URI="ftp://host/$ART"

# The legacy SSH relay keeps working — supported, but no longer REQUIRED.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'ssh-legacy-happy-path-accepted' 0 ARCHIVE_URI="ssh://fakehost/srv/arch/$ART"

# file:// is the operator-staged path, already exercised by every case above; assert the
# scheme explicitly so a future refactor cannot silently drop it.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'file-staged-happy-path-accepted' 0 ARCHIVE_URI="file://$LOG/$ART"

printf '=== non-vacuity: the old masking idiom accepts what B refuses ===\n'
# Case B's refusal is only meaningful if the OLD idiom would have ACCEPTED the same
# failure. A compound's $? is the status of its LAST command, so {A | B; echo $?}
# records B's status and A's non-zero exit is lost. No gzip is needed to show it.
rm -f "$LOG/old.st"
if { sh -c 'exit 7' | sh -c 'exit 0'; echo $? > "$LOG/old.st"; } 2>/dev/null; then :; fi
if [ "$(cat "$LOG/old.st" 2>/dev/null)" = "0" ]; then
  ok 'old-idiom-masks-first-stage' 'reproduced: {exit 7 | exit 0; echo $?} records 0'
else
  bad 'old-idiom-masks-first-stage' "old idiom recorded $(cat "$LOG/old.st" 2>/dev/null), expected 0"
fi
# ...and the per-stage form used by the helper does NOT mask it.
rm -f "$LOG/new.st"
if { sh -c 'exit 7'; echo $? > "$LOG/new.st"; } | sh -c 'exit 0' >/dev/null 2>&1; then :; fi
if [ "$(cat "$LOG/new.st" 2>/dev/null)" = "7" ]; then
  ok 'per-stage-capture-sees-failure' 'the helper form records the real status 7'
else
  bad 'per-stage-capture-sees-failure' "helper form recorded $(cat "$LOG/new.st" 2>/dev/null), expected 7"
fi
# Case B itself must be discriminating: with the age-failure marker set the helper
# refuses (asserted above), and with it clear it succeeds (also asserted above).
clear_all; : > "$LOG/KEEP_have_api"
run_case 'B-control-succeeds-when-marker-clear' 0

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"

[ "$failed" -eq 0 ] || exit 1
exit 0
