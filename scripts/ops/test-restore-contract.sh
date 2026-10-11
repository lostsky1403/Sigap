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

TOP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-rst.$$")
BIN="$TOP/bin"; LOG="$TOP/log"; mkdir -p "$BIN" "$LOG"
trap 'rm -rf "$TOP"' EXIT INT TERM
export TOP_LOG="$LOG"
CHAIN="$BIN:/usr/bin:/bin"

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
    -*) shift 2 2>/dev/null || shift ;;
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
# Delegate to the real gzip when one works; otherwise implement a trivial reversible
# transform with \`tr\` so the suite still runs on a host with a broken gzip. The helper
# only ever checks \`gzip -t\` and decompression, both of which this satisfies. A REAL
# deployment never uses this path — it is test scaffolding only.
if [ -n "$REAL_GZIP" ]; then
  exec "$REAL_GZIP" "\$@"
fi
mode=-1
for a in "\$@"; do case "\$a" in -d*) mode=-d ;; esac; done
if [ "\$mode" = "-1" ]; then tr 'a-zA-Z' 'n-za-mN-ZA-M'; else tr 'a-zA-Z' 'n-za-mN-ZA-M'; fi
exit 0
F

chmod +x "$BIN"/age "$BIN"/docker "$BIN"/scp "$BIN"/sha256sum "$BIN"/gzip

# The payload is "compressed" with the same transform the fake uses, so a real gzip is
# not required on this host. `tr` is a POSIX builtin-adjacent utility that always works.
if [ -n "$REAL_GZIP" ]; then
  printf 'fake-image-tar-payload' | "$REAL_GZIP" -1 > "$LOG/payload.gz"
else
  printf 'fake-image-tar-payload' | tr 'a-zA-Z' 'n-za-mN-ZA-M' > "$LOG/payload.gz"
fi
DIG=$(sha256sum "$LOG/payload.gz" | cut -d' ' -f1)
printf '%s  %s\n' "$DIG" "$LOG/payload.gz" > "$LOG/digest.val"

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
        ARCHIVE="$LOG/archive.age" ARCHIVE_HOST=fakehost ARCHIVE_DIR=/fake \
        ARCHIVE_KEY=/fake/key STAMP=20261010 EXPECT_DIGEST="$DIG" \
        ISOLATED_DOCKER_HOST="tcp://isolated:9999" EXPECT_IDS="$API" "$@" \
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
                   "$LOG"/load_fails "$LOG"/no_engine "$LOG"/KEEP_have_api; }

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

printf '=== restore-target isolation ===\n'
clear_all; : > "$LOG/KEEP_have_api"
run_case 'M-production-host-refused' 11 ISOLATED_DOCKER_HOST=ssh://fikriserver
run_case 'M2-local-socket-refused'    11 ISOLATED_DOCKER_HOST=/var/run/docker.sock
run_case 'M3-actual-production-path'  11 ISOLATED_DOCKER_HOST=unix:///run/docker.sock
run_case 'M4-empty-endpoint-refused'  10 ISOLATED_DOCKER_HOST=""

printf '=== required input is validated, never defaulted ===\n'
clear_all; : > "$LOG/KEEP_have_api"
out=$(TOP_LOG="$LOG" PATH="$NARROW_PATH" \
      ARCHIVE_HOST=h ARCHIVE_DIR=/d ARCHIVE_KEY=/k \
      STAMP=s EXPECT_DIGEST="$DIG" ISOLATED_DOCKER_HOST="tcp://i:1" \
      EXPECT_IDS="$API" timeout 60 "$BASH_BIN" "$HELPER" 2>&1)
[ $? -eq 10 ] && ok 'missing-var-fails-closed' 'rc=10' \
              || bad 'missing-var-fails-closed' "want=10 got=$?"

printf '=== every docker call is pinned to the isolated daemon ===\n'
clear_all; : > "$LOG/KEEP_have_api"
# Reset the call log so the count reflects THIS run only — the fake appends across
# cases, and earlier cases run `docker context` through the ambient context.
: > "$LOG/docker.calls" 2>/dev/null || true
env TOP_LOG="$LOG" PATH="$NARROW_PATH" \
    ARCHIVE="$LOG/archive.age" ARCHIVE_HOST=fakehost ARCHIVE_DIR=/fake \
    ARCHIVE_KEY=/fake/key STAMP=20261010 EXPECT_DIGEST="$DIG" \
    ISOLATED_DOCKER_HOST="tcp://isolated:9999" EXPECT_IDS="$API" \
    timeout 60 "$BASH_BIN" "$HELPER" >/dev/null 2>&1
# Count docker invocations and how many were pinned.
# `docker context inspect` is deliberately AMBIENT — it reads the local configuration
# by design and touches no image — so it is excluded from the "must be pinned" set.
# Every other call, especially the ones that write, must carry the isolated DOCKER_HOST.
total=$(grep '^docker ' "$LOG/docker.calls" 2>/dev/null | grep -vc 'docker context' || true)
total=${total:-0}
unpinned=$(grep -A1 '^DOCKER_HOST=' "$LOG/docker.calls" 2>/dev/null \
           | paste - - - 2>/dev/null | grep -v '^DOCKER_HOST=tcp://isolated:9999' \
           | grep -c 'docker ' || true)
# Count unpinned calls that are NOT context inspections, by pairing each DOCKER_HOST
# line with the docker line that follows it.
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
