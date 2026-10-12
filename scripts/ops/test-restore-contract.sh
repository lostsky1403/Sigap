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

# A SYNTHETIC 64-hex placeholder, deliberately low-entropy. The previous value was a
# realistic-looking digest and gitleaks flagged it as a generic API key (entropy 3.86)
# in `API=<hex>` shape — a false positive, since it is a test constant and not a
# secret. The fix is to make the fixture OBVIOUSLY a placeholder rather than to
# allowlist the scanner: an allowlist entry would also hide a real secret added to
# this file later, so the scanner keeps full strength and the constant changes.
# The format is what the tests exercise; the value's realism was never load-bearing.
API=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

# Engine IDs the fake docker returns. Real engine IDs come in TWO machine-generated forms —
# 64 lowercase hex on older daemons, and an 8-4-4-4-12 UUID on current ones (Docker 29
# returns the UUID form; [OBSERVED] in CI, where the isolated daemon reported
# `dca7b900-8462-4e5c-a7e6-6c69faafe428`). The helper validates the shape before trusting the
# ambient value, so these must be realistic. The hex form is the default here; a marker
# switches the target to the UUID form so both spellings are covered.
# `TGT_ENGINE` is what the ISOLATED target answers with (DOCKER_HOST pinned);
# `AMB_ENGINE` is what the AMBIENT probe answers with (DOCKER_HOST unset). They differ,
# which is the normal, correctly-isolated case.
TGT_ENGINE=1111111111111111111111111111111111111111111111111111111111111111
AMB_ENGINE=2222222222222222222222222222222222222222222222222222222222222222
OTHER_ENGINE=3333333333333333333333333333333333333333333333333333333333333333
# The SAME isolated daemon as TGT_ENGINE, written the way current Docker reports it. The
# dashless hex MUST differ from AMBIENT_ENGINE: distinctness from the ambient daemon is
# asserted, and an ID equal to the ambient one is refused as "not an isolated target" —
# which is correct behaviour, so the fixture must not trip it.
TGT_ENGINE_UUID=11111111-1111-4111-8111-999999999999
OTHER_ENGINE_UUID=33333333-3333-3333-3333-333333333333
# An image the operator did NOT expect to find in the archive.
EXTRA_IMAGE=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee

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
    # SUBCOMMAND WORDS ARE NOT REFERENCES. `docker image inspect --format X Y` puts
    # `inspect` in $2, and an earlier version of this loop treated the first bare
    # positional as the ref — so it captured the literal string `inspect` and never saw
    # the tag, making the tag-form cases fail for a fixture reason rather than a helper
    # one. Listed explicitly so no other subcommand can be mistaken for a reference.
    inspect|rmi|tag|save|history|ls) ;;
    # A flag plus its value. Under dash (/bin/sh on Ubuntu) `shift 2` is a FATAL
    # error when fewer than two positional parameters remain, so it must be
    # guarded explicitly — `shift 2 2>/dev/null || shift` still fails there,
    # because the error is not suppressible that way.
    -*) if [ $# -ge 2 ]; then shift; fi ;;
    sha256:*|sha256) ref=$1 ;;
    # Any other bare positional is a reference too — a TAG, not a digest. The
    # helper resolves a `Loaded image: <repo:tag>` line by inspecting that tag,
    # so the fake must accept one.
    *) [ -z "$ref" ] && ref=$1 ;;
  esac
  shift
done
case "$sub" in
  # The engine ID depends on whether DOCKER_HOST is set, which is what makes the
  # ambient/target distinction testable. The helper probes the AMBIENT engine with no
  # DOCKER_HOST (to prove the isolated target is a DIFFERENT daemon) and the TARGET with
  # DOCKER_HOST pinned. A fake returning one constant for both would make those two
  # indistinguishable and the distinctness assertion untestable — and would in fact make
  # it fail, since equal IDs are refused.
  info)    [ -f "$TOP_LOG/no_engine" ] && exit 1              # N: no identity
           # Real Docker engine IDs are 64 lowercase hex OR an 8-4-4-4-12 UUID, and the
           # helper VALIDATES that shape before trusting the ambient value as a
           # security-decision input — a whitespace or short string is treated as unreadable
           # and refused. The fake must therefore return realistic IDs, or the happy path
           # would be refused for the wrong reason.
           #
           # `uuid_engine` makes the TARGET report the UUID spelling of the SAME daemon the
           # allowlist describes, proving the helper accepts the format current Docker emits.
           #
           # `same_engine` models a misconfigured run where the allowlisted ID IS the
           # ambient engine's — the case the distinctness assertion exists to catch.
           # `ambient_fails` models a target that answers while the ambient probe does not,
           # which is the only way to reach the ambient-unreadable refusal: the target probe
           # runs first, so `no_engine` alone never gets there.
           # Hardcoded rather than interpolated: the fake is generated from a QUOTED heredoc,
           # so shell variables are not expanded when it is written — and it must stay
           # self-contained to remain a faithful stub.
           # Scoped INSIDE the DOCKER_HOST branch: only the TARGET probe is pinned, so the
           # marker changes the target's answer without touching the ambient probe. Before
           # this, the marker also changed what the ambient probe reported, which made the
           # UUID daemon "the same" as ambient and tripped the distinctness refusal — a
           # fixture bug, not a helper defect.
           if [ -f "$TOP_LOG/same_engine" ]; then echo "1111111111111111111111111111111111111111111111111111111111111111"
           elif [ -n "${DOCKER_HOST:-}" ] && [ -f "$TOP_LOG/uuid_engine" ]; then echo "11111111-1111-4111-8111-999999999999"
           elif [ -n "${DOCKER_HOST:-}" ]; then echo "1111111111111111111111111111111111111111111111111111111111111111"
           elif [ -f "$TOP_LOG/ambient_fails" ]; then exit 1
           elif [ -f "$TOP_LOG/ambient_whitespace" ]; then printf '   \n'
           else echo "2222222222222222222222222222222222222222222222222222222222222222"; fi ;;
  ps)      # `sigap_project` models a target that already carries the production project,
           # which is the last production-specific control for an aliased target.
           # `ps_fails` models an enumeration that ERRORS. The check must distinguish that
           # from a clean daemon: an earlier form piped straight into `grep`, so a failed
           # `docker ps` looked exactly like "no sigap project" and the check failed OPEN.
           [ -f "$TOP_LOG/ps_fails" ] && exit 1
           if [ -f "$TOP_LOG/sigap_project" ]; then printf 'sigap\n'; fi
           exit 0 ;;
  context) echo "unix:///var/run/docker.sock" ;;
  # FIDELITY: real `docker load` prints the ID it assigned, and the helper now READS that
  # line rather than inferring the identity from the live daemon — `docker inspect` reports a
  # config digest that `docker load` does not necessarily reproduce ([OBSERVED] run
  # 38272620285, inspect ff59b31e... vs load d88c5ca4...). The old fake drained stdin and
  # exited 0 with NO output, which would have made the new fail-closed check reject every
  # happy-path case with "docker load reported no image ID" — a fixture failing, not a
  # helper. The literal is the bare-hex form of `API`, matching what the fake's `image`
  # branch resolves. `LOADED_IMAGE_ID` overrides it from the environment for cases that must
  # exercise a load reporting a DIFFERENT ID than the one being looked up.
  load)    cat >/dev/null 2>/dev/null || true                 # drain stdin, never block
           [ -f "$TOP_LOG/load_fails" ] && { echo "load failed" >&2; exit 1; }  # K
           # `load_no_id` models a load that SUCCEEDS but reports no ID — the shape that must be
           # refused rather than trusted. `${VAR:-default}` cannot express "empty" because an
           # empty assignment is indistinguishable from unset, so a marker is used.
           [ -f "$TOP_LOG/load_no_id" ] && exit 0
           # `load_tag_form` models a tar WITH RepoTags: the daemon then prints
           # `Loaded image: <repo:tag>` and NO `Loaded image ID:` line at all, because
           # the ID lines are buffered and written only when the last image had no
           # RepoTags. An earlier helper parser understood only the ID form.
           if [ -f "$TOP_LOG/load_tag_form" ]; then
             echo "Loaded image: sigap-drill:local"
           else
             echo "Loaded image ID: ${LOADED_IMAGE_ID:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
           fi
           # `load_extra_id` models an archive carrying an image the operator did not
           # list — it must be refused, not loaded silently.
           [ -f "$TOP_LOG/load_extra_id" ] && echo "Loaded image ID: eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
           exit 0 ;;
  image)   # `load_tag_form` makes the load report `Loaded image: sigap-drill:local`,
           # so that tag must resolve to the SAME image the ID form names.
           case "$ref" in
             sigap-drill:local) ref="sha256:${LOADED_IMAGE_ID:-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" ;;
           esac
           id=$(printf '%s' "$ref" | sed 's/^sha256://; s/[^0-9a-f]//g')
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
  # Additional images that must be PRESENT, space-separated. A case that lists two
  # DIFFERENT ids — one bare, one `sha256:`-prefixed — needs both registered, and it
  # cannot do that by touching `have_$ID` itself: the `rm -f have_*` above wipes it.
  for _xid in ${KEEP_EXTRA_IDS:-}; do : > "$LOG/have_$_xid"; done
  # Extra arguments are ENVIRONMENT ASSIGNMENTS, so they are passed to `env` as
  # separate arguments — never appended as shell words, which the shell would try to
  # EXECUTE as a program named "ISOLATED_DOCKER_HOST=..." [OBSERVED] rc=127.
  #
  # HERMETIC: the ambient AWS credentials are scrubbed FIRST, so a case that intends
  # `AWS_ACCESS_KEY_ID` to be absent actually gets it absent. `env` otherwise inherits
  # the caller's environment, and a developer with real credentials exported would see
  # `r2-missing-recovery-credential-refused` fail — the helper's `req` would find the
  # ambient key and proceed. [OBSERVED] with a live `AWS_ACCESS_KEY_ID` in the shell.
  # Scrubbing before the explicit assignments means a case that DOES set a variable
  # still wins, so this cannot mask a genuine requirement.
  out=$(env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
        -u AWS_PROFILE -u R2_ENDPOINT \
        TOP_LOG="$LOG" PATH="$NARROW_PATH" \
        ARCHIVE_URI="file://$LOG/$ART" \
        ARCHIVE_KEY=/fake/key STAMP="$TSTAMP" EXPECT_DIGEST="$DIG" \
        ISOLATED_DOCKER_HOST="tcp://isolated:9999" EXPECT_IDS="$API" \
        ALLOWED_ENGINE_ID="$TGT_ENGINE" "$@" \
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
                   "$LOG"/aws_fails "$LOG"/aws_truncated "$LOG"/same_engine \
                   "$LOG"/ambient_fails "$LOG"/ambient_whitespace "$LOG"/sigap_project \
                   "$LOG"/ps_fails "$LOG"/uuid_engine "$LOG"/load_no_id \
                   "$LOG"/load_tag_form "$LOG"/load_extra_id; }

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

# G — the transfer fails. The URI must actually SELECT the ssh path: the default URI is
# `file://`, so without this override `scp` is never invoked, the marker is never read,
# and the restore succeeds for a reason unrelated to the transfer. The case would then
# report a failure that has nothing to do with the guard it names. [OBSERVED] the suite
# reported `want=14 got=0` for exactly this reason.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/scp_fails"
run_case 'G-ssh-transfer-fails' 14 ARCHIVE_URI="ssh://fakehost/srv/arch/$ART"

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

# REGRESSION — a load that reports NO image ID must be refused, not trusted. `docker load`
# printing the ID it assigned is the archive's own identity, and the helper now reads that
# line rather than inferring the ID from a live `docker inspect` (which reports a config
# digest `docker load` does not necessarily reproduce — [OBSERVED] run 38272620285, inspect
# ff59b31e... vs load d88c5ca4...). An unparsable load must therefore be a hard failure (14),
# because passing an empty value on would report a PARTIAL LOAD for a load that succeeded.
# `LOADED_IMAGE_ID=` empty makes the fake print the prefix with nothing after it.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_no_id"
run_case 'load-reports-no-image-id-refused' 14
# ...and a load reporting a DIFFERENT ID than the one archived is a real correspondence
# failure (15), not a crash — the archive restored something, just not what was recorded.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'load-reports-foreign-image-refused' 15 \
         LOADED_IMAGE_ID="sha256:$(printf 'f%.0s' $(seq 1 64))" EXPECT_IDS="$API"

# REGRESSION — the TAG output form. `docker load` prints `Loaded image: <repo:tag>` and NO
# `Loaded image ID:` line when the tar carries RepoTags, which is what `docker save
# <repo:tag>` produces. An earlier parser matched only the ID form and refused a perfectly
# good restore with 14. [OBSERVED] daemon/internal/image/tarexport/load.go buffers the ID
# lines and writes them only when the last image had no RepoTags. The tag must be RESOLVED
# to an image identity, not accepted as a name.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_tag_form"
run_case 'load-tag-form-accepted' 0 EXPECT_IDS="$API"
# ...and the tag form must still fail correspondence when the tag resolves to an image the
# operator did not expect, so accepting the form did not make the check vacuous.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_tag_form"
KEEP_EXTRA_IDS="$EXTRA_IMAGE"
run_case 'load-tag-form-wrong-image-refused' 15 EXPECT_IDS="$EXTRA_IMAGE"
KEEP_EXTRA_IDS=""

# UNEXPECTED IMAGE — an archive carrying an EXTRA image must be refused rather than loaded
# silently. Before this check, 7a confirmed the extra image existed and 7b confirmed the
# expected one existed, and nothing noticed the addition.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_extra_id"
KEEP_EXTRA_IDS="$EXTRA_IMAGE"
run_case 'load-unexpected-image-refused' 15 EXPECT_IDS="$API"
KEEP_EXTRA_IDS=""
# ...and an archive whose reported images are ALL expected is accepted, so the new check
# does not refuse a correct multi-image restore.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/load_extra_id"
KEEP_EXTRA_IDS="$EXTRA_IMAGE"
run_case 'load-multi-image-all-expected-accepted' 0 EXPECT_IDS="$API,$EXTRA_IMAGE"
KEEP_EXTRA_IDS=""

# N — the engine reports no identity.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/no_engine"
run_case 'N-missing-engine-identity' 11

printf '=== Defect C: ALLOWED_ENGINE_ID is mandatory ===\n'
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/no_engine" 2>/dev/null || rm -f "$LOG/no_engine"
run_case 'engine-allowlist-absent-refused' 10 ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
         ALLOWED_ENGINE_ID="" EXPECT_IDS="$API"
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-mismatch-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
         ALLOWED_ENGINE_ID="$OTHER_ENGINE" EXPECT_IDS="$API"

# REGRESSION — the allowlist accepts the `sha256:` spelling, and the comparison is
# symmetric in BOTH places it happens. The pre-load check normalised the allowlist but the
# POST-load recheck compared raw values, so this exact input was refused with "engine ID
# changed after load" — a false failure reporting the wrong cause, since the daemon never
# changed. The happy path must therefore reach rc=0 with a prefixed allowlist; rc=11 here
# means the post-load recheck regressed.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-sha256-prefix-accepted' 0 \
         ALLOWED_ENGINE_ID="sha256:$TGT_ENGINE" EXPECT_IDS="$API"
# ...and a prefixed value that is genuinely a DIFFERENT engine is still a mismatch (11),
# so the normalisation above did not make the comparison vacuous.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-sha256-prefix-mismatch-refused' 11 \
         ALLOWED_ENGINE_ID="sha256:$OTHER_ENGINE" EXPECT_IDS="$API"
# ...and a non-hex allowlist is malformed input (10), not a mismatch (11).
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-malformed-refused' 10 \
         ALLOWED_ENGINE_ID="engineWRONG" EXPECT_IDS="$API"
# ...and the expected `sha256:sha256:<digest>` doubled prefix is malformed too.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'engine-allowlist-doubled-prefix-refused' 10 \
         ALLOWED_ENGINE_ID="sha256:sha256:$TGT_ENGINE" EXPECT_IDS="$API"

# REGRESSION — the UUID spelling of the SAME daemon must be accepted, not refused.
# `docker info --format '{{.ID}}'` returns an 8-4-4-4-12 UUID on current daemons, and the
# earlier image-ID normaliser required exactly 64 hex, so a real isolated daemon was refused
# before the round trip could run. [OBSERVED] run 38268640704, step "Restore":
#   REFUSED: ALLOWED_ENGINE_ID is not a valid engine ID (need 64 hex, ...)
#   'dca7b900-8462-4e5c-a7e6-6c69faafe428' 10
# The allowlist is the SAME daemon as TGT_ENGINE_UUID written WITHOUT dashes — this proves
# the normaliser makes the two spellings interchangeable. If UUID and hex did not
# canonicalise to one value, this would be refused as a mismatch (11) instead of accepted.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/uuid_engine"
run_case 'engine-uuid-spelling-accepted' 0          ALLOWED_ENGINE_ID="11111111111141118111999999999999" EXPECT_IDS="$API"
# ...and a UUID naming a DIFFERENT daemon is still refused, so the widening is not vacuous.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/uuid_engine"
run_case 'engine-uuid-mismatch-refused' 11          ALLOWED_ENGINE_ID="$OTHER_ENGINE_UUID" EXPECT_IDS="$API"
# ...and the two spellings of the SAME daemon must be interchangeable in the denylist too.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/uuid_engine"
# ...and the DENYLIST still works in the UUID family: the target's own UUID spelling is
# listed as a recorded PRODUCTION engine, so the strongest control refuses it even though the
# allowlist describes the same daemon. Listing an unrelated UUID would prove nothing — it
# would be refused by the allowlist before the denylist is ever consulted.
run_case 'engine-uuid-in-production-denylist-refused' 11 \
         ALLOWED_ENGINE_ID="11111111111141118111999999999999" \
         PRODUCTION_ENGINE_ID="$TGT_ENGINE_UUID" EXPECT_IDS="$API"

printf '=== restore-target isolation ===\n'
clear_all; : > "$LOG/KEEP_have_api"
run_case 'M-production-host-refused' 11 ISOLATED_DOCKER_HOST=ssh://fikriserver
run_case 'M2-local-socket-refused'    11 ISOLATED_DOCKER_HOST=/var/run/docker.sock
run_case 'M3-actual-production-path'  11 ISOLATED_DOCKER_HOST=unix:///run/docker.sock
run_case 'M4-empty-endpoint-refused'  10 ISOLATED_DOCKER_HOST=""

printf '=== loopback: refused by default, admitted only with an explicit opt-in ===\n'
# Loopback is how a same-host disposable daemon is reached (the CI `docker:dind` case) AND
# how a developer's local daemon is reached. The default must stay fail-closed, so a
# loopback target is refused unless SIGAP_ALLOW_LOOPBACK_TARGET=1 is set explicitly.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'loopback-refused-without-optin' 11 ISOLATED_DOCKER_HOST="tcp://127.0.0.1:2375"
run_case 'loopback-localhost-refused-without-optin' 11 ISOLATED_DOCKER_HOST="tcp://localhost:2375"
# With the opt-in, and a proven-distinct engine, the same endpoint is admitted.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'loopback-accepted-with-optin' 0 \
  ISOLATED_DOCKER_HOST="tcp://127.0.0.1:2375" SIGAP_ALLOW_LOOPBACK_TARGET=1

printf '=== the target must be a DIFFERENT engine from the ambient one ===\n'
# Without this, ALLOWED_ENGINE_ID could name the ambient engine itself and every other
# guard would pass while loading into the daemon the isolation exists to avoid.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/same_engine"
run_case 'ambient-engine-equals-target-refused' 11 \
  ISOLATED_DOCKER_HOST="tcp://isolated:9999" ALLOWED_ENGINE_ID="$TGT_ENGINE"

printf '=== distinctness must be UNPROVEN => refused, not silently skipped ===\n'
# A plain `[ -n "$AMBIENT" ]` test treated whitespace-only output as readable, so a bad
# probe neutralised the comparison while appearing to pass. The ambient value is now
# validated as an engine ID, and anything else is refused for EVERY target.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/ambient_fails"
run_case 'ambient-unreadable-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999"
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/ambient_whitespace"
run_case 'ambient-whitespace-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999"
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/ambient_fails"
run_case 'ambient-unreadable-loopback-refused' 11 \
  ISOLATED_DOCKER_HOST="tcp://127.0.0.1:2375" SIGAP_ALLOW_LOOPBACK_TARGET=1

printf '=== the production compose project is refused, and the check fails closed ===\n'
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/sigap_project"
run_case 'target-holding-sigap-project-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999"
# An enumeration that ERRORS must be refused, not treated as "no sigap project". This is
# the branch that used to fail OPEN.
clear_all; : > "$LOG/KEEP_have_api"; : > "$LOG/ps_fails"
run_case 'container-enumeration-failure-refused' 11 ISOLATED_DOCKER_HOST="tcp://isolated:9999"

printf '=== a recorded production engine ID is refused (not aliasable) ===\n'
# Every other production control is a string test, and a string can be aliased by a tunnel
# or a DNS name. An engine ID cannot be, so this is the only alias-proof production check.
# NOTE: `same_engine` is deliberately NOT set here — the ambient engine stays distinct, so
# the distinctness check cannot fire and the PRODUCTION denylist is the only thing that can
# refuse this case. Otherwise the test would pass for the wrong reason.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'recorded-production-engine-refused' 11 \
  ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
  PRODUCTION_ENGINE_ID="$TGT_ENGINE"
# A TYPO must not silently disable the denylist. The earlier form used `|| continue`, so a
# malformed entry turned the strongest production control into a no-op with no signal.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'malformed-production-engine-refused' 10 \
  ISOLATED_DOCKER_HOST="tcp://isolated:9999" \
  PRODUCTION_ENGINE_ID="not-an-engine-id"

printf '=== the loopback matcher must be COMPLETE, not a list of favourites ===\n'
# Every one of these reaches a same-host daemon. A matcher that misses them does not
# deliver the fail-closed default it claims, so each must require the opt-in.
clear_all; : > "$LOG/KEEP_have_api"
for ep in "tcp://127.0.0.2:2375" "tcp://127.1:2375" "tcp://127.255.255.255:2375" \
          "tcp://0.0.0.0:2375" "tcp://LOCALHOST:2375" "tcp://[::1]:2375" \
          "tcp://[::]:2375" "tcp://[::ffff:127.0.0.1]:2375" \
          "tcp://[0:0:0:0:0:0:0:1]:2375" "tcp://ip6-localhost:2375" \
          "fd://3" "npipe:////./pipe/docker_engine"; do
  run_case "same-host-refused[$ep]" 11 ISOLATED_DOCKER_HOST="$ep"
done
# Case variants of the production hostname must not slip past a lowercase denylist.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'production-host-case-variant-refused' 11 ISOLATED_DOCKER_HOST="tcp://FIKRISERVER:2375"
run_case 'production-socket-trailing-slash-refused' 11 ISOLATED_DOCKER_HOST="unix:///var/run/docker.sock/"

printf '=== image IDs: one canonical form, normalised once, no doubled prefix ===\n'
# `docker inspect --format '{{.Id}}'` returns `sha256:<digest>`, while `docker image
# inspect` accepts either form. An earlier revision prefixed unconditionally, so a
# prefixed input became `sha256:sha256:<digest>` — not a valid reference — and the
# resulting lookup failure was reported as "image absent", i.e. a false identity failure
# caused by input formatting.
ABSENT=0000000000000000000000000000000000000000000000000000000000000000
# Bare 64-hex digest: accepted.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-bare-digest-accepted' 0 EXPECT_IDS="$API"
# Prefixed form: accepted, and NOT doubled.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-sha256-prefixed-accepted' 0 EXPECT_IDS="sha256:$API"
# A doubled prefix is malformed input, refused as such (10) rather than as absent (15).
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-doubled-prefix-refused-as-invalid' 10 EXPECT_IDS="sha256:sha256:$API"
# Non-hex, wrong length, and uppercase-hex are all invalid.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-non-hex-refused' 10 EXPECT_IDS="notahash"
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-wrong-length-refused' 10 EXPECT_IDS="aaaaaaaa"
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-uppercase-refused-as-non-canonical' 10 \
  EXPECT_IDS="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
# A valid but ABSENT digest is a correspondence failure (15), distinct from malformed (10).
clear_all
run_case 'id-absent-image-fails-correspondence' 15 EXPECT_IDS="$ABSENT"
# PARTIAL LOAD: one present, one absent must be refused, not accepted on the present one.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-partial-load-refused' 15 EXPECT_IDS="$API,$ABSENT"
# Exact correspondence for a multi-ID list, both spellings mixed.
# Genuinely MIXED: two DIFFERENT ids, one bare and one `sha256:`-prefixed, BOTH present.
# The previous form was a duplicate of id-sha256-prefixed-accepted and proved nothing extra.
# The second id needs `KEEP_EXTRA_IDS` rather than a direct `have_` touch, because run_case
# clears `have_*` before registering markers.
clear_all; : > "$LOG/KEEP_have_api"
KEEP_EXTRA_IDS="$ABSENT"
run_case 'id-mixed-spelling-correspondence' 0 EXPECT_IDS="$API,sha256:$ABSENT"
KEEP_EXTRA_IDS=""
# ...and the correspondence check must still FAIL when one of the two mixed-spelling ids is
# genuinely absent, so the case above is not satisfied by a check that ignores ids.
clear_all; : > "$LOG/KEEP_have_api"
run_case 'id-mixed-spelling-missing-one-refused' 15 EXPECT_IDS="$API,sha256:$ABSENT"

printf '=== required input is validated, never defaulted ===\n'
clear_all; : > "$LOG/KEEP_have_api"
# ARCHIVE_URI is omitted entirely. The `req` loop must refuse BEFORE the basename check
# dereferences it, otherwise `set -u` would abort with an undocumented status instead of
# the documented 10.
out=$(TOP_LOG="$LOG" PATH="$NARROW_PATH" \
      ARCHIVE_KEY=/k \
      STAMP="$TSTAMP" EXPECT_DIGEST="$DIG" ISOLATED_DOCKER_HOST="tcp://i:1" \
      EXPECT_IDS="$API" ALLOWED_ENGINE_ID="$TGT_ENGINE" \
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
    ALLOWED_ENGINE_ID="$TGT_ENGINE" \
    timeout 60 "$BASH_BIN" "$HELPER" >/dev/null 2>&1
# Count docker invocations that could READ OR WRITE IMAGES, and how many of those were
# pinned. Two call classes are excluded, because neither can affect an image and both are
# REQUIRED to read the ambient daemon:
#   * `docker context inspect` — reads local configuration by design;
#   * `docker info` — the engine-ID probe. The ambient probe deliberately omits
#     DOCKER_HOST, because its whole purpose is to prove the isolated target is a
#     DIFFERENT engine. Counting it as "unpinned" would penalise the distinctness check.
# Every other call, and the `docker load` above all, must carry the isolated DOCKER_HOST.
checked=0; unpinned=0
while IFS= read -r line; do
  case "$line" in
    DOCKER_HOST=*)
      cur=${line#DOCKER_HOST=} ;;
    docker\ *)
      case "$line" in
        'docker context'*|'docker info'*) ;;
        *)
          checked=$((checked+1))
          [ "$cur" = "tcp://isolated:9999" ] || unpinned=$((unpinned+1)) ;;
      esac ;;
  esac
done < "$LOG/docker.calls" 2>/dev/null
if [ "$checked" -gt 0 ] && [ "$unpinned" -eq 0 ]; then
  ok 'every-docker-call-pinned' "$checked image-affecting calls pinned; $unpinned unpinned"
else
  bad 'every-docker-call-pinned' "$unpinned of $checked image-affecting call(s) unpinned — they target the ambient daemon"
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
  # FAIL CLOSED. This block runs a full successful restore, so `docker load` MUST appear.
  # The earlier form reported PASS when it did not, which made the assertion vacuous in
  # exactly the case it exists to catch — a load that silently never happened.
  bad 'load-is-pinned' 'no docker load recorded in a run that should have loaded — assertion could not be satisfied'
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
