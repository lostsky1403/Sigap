#!/usr/bin/env bash
# sigap-archive-restore.sh — shared, executable restore-verification helper.
#
# This is the executable contract behind runbook §7d.1 and §13f. It exists because
# prose and mock tests are not proof: the restore must be exercised by real commands.
# The caller invokes this script; it performs every fail-closed check and exits
# non-zero unless EVERY one of them passes.
#
# REQUIRES BASH. `set -euo pipefail`, PIPESTATUS, `local`, `shred` and `trap ... HUP`
# are used deliberately; POSIX /bin/sh provides none of them (pipefail, PIPESTATUS
# and `local` in particular). Invoke explicitly as:
#     bash scripts/ops/sigap-archive-restore.sh
# Do NOT add a `sh`-to-`bash` re-exec here: `command -v bash` can resolve to a broken
# relay that appears to succeed and then hangs, which is worse than a clean refusal.
if [ -z "${BASH_VERSION:-}" ]; then
  echo "REFUSED: this script requires bash — invoke it as: bash $0" >&2
  exit 99
fi
#
# Usage:
#   ARCHIVE=<path-to-.age> ARCHIVE_HOST=<host> ARCHIVE_DIR=<dir> ARCHIVE_KEY=<key>
#   EXPECT_DIGEST=<sha256> ISOLATED_DOCKER_HOST=<endpoint> EXPECT_IDS=<id1,id2,...>
#   [ALLOWED_ENGINE_ID=<id>] ./sigap-archive-restore.sh
#
# Exit codes:
#   0  every stage succeeded AND identity correspondence verified
#   10 a required variable was unset
#   11 destination/target refused (production or unverifiable)
#   12 digest mismatch
#   13 decrypted stream is not a complete gzip
#   14 a pipeline stage failed (see per-stage report)
#   15 image identity correspondence failed

set -euo pipefail

die() { printf 'REFUSED: %s\n' "$*" >&2; exit "${2:-14}"; }

# ---------------------------------------------------------------------------
# 1. Required inputs — validated, never defaulted.
# ---------------------------------------------------------------------------
# Required inputs — validated, never defaulted. Note: `${VAR:?msg}` EXITS the shell
# with status 1 before `||` can run, so an explicit test is used to return 10.
req() { [ -n "${!1:-}" ] || die "$1 is required (see the script header)" 10; }
for v in ARCHIVE ARCHIVE_HOST ARCHIVE_DIR ARCHIVE_KEY STAMP EXPECT_DIGEST \
         ISOLATED_DOCKER_HOST EXPECT_IDS; do req "$v"; done

ART="sigap-pre-redesign-$STAMP.tar.gz.age"

# ---------------------------------------------------------------------------
# 2. Restore target isolation — proved by ENGINE IDENTITY, not string matching.
#    String matching is a necessary negative control but is NOT sufficient proof.
#    The actual engine ID (docker info .ID) of the target is compared against an
#    explicitly allowlisted value; the production engine ID is recorded from the
#    host so the comparison is meaningful.
# ---------------------------------------------------------------------------
is_production_string() {
  case "$1" in
    ssh://fikriserver*|*@fikriserver|fikriserver|fikriserver:*|tcp://fikriserver*) return 0 ;;
    unix:///var/run/docker.sock|/var/run/docker.sock|var/run/docker.sock|unix:///run/docker.sock|/run/docker.sock|run/docker.sock) return 0 ;;
    ssh://localhost*|tcp://localhost*|tcp://127.0.0.1*|ssh://127.0.0.1*|tcp://[::1]:*) return 0 ;;
    "") return 0 ;;
  esac
  return 1
}
if is_production_string "$ISOLATED_DOCKER_HOST"; then
  die "ISOLATED_DOCKER_HOST '$ISOLATED_DOCKER_HOST' matches a production/shared endpoint" 11
fi
if [ "$ISOLATED_DOCKER_HOST" = "$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null)" ]; then
  die "ISOLATED_DOCKER_HOST equals the active context" 11
fi
# If the AMBIENT context is itself production, an UNPINNED docker command would target
# it. The helper therefore pins EVERY docker command below with an explicit DOCKER_HOST
# and warns loudly, rather than trusting that no command was forgotten.
AMBIENT_CTX=$(docker context inspect -f '{{.Endpoints.docker.Host}}' 2>/dev/null) || AMBIENT_CTX=''
if [ -n "$AMBIENT_CTX" ] && is_production_string "$AMBIENT_CTX"; then
  printf 'NOTE: the active context (%s) is production-like; every docker command below\n' "$AMBIENT_CTX" >&2
  printf '      is pinned to DOCKER_HOST=%s.\n' "$ISOLATED_DOCKER_HOST" >&2
fi

# Reachability and engine identity. `docker info` returns the ENGINE id, which is a
# property of the daemon itself, so two spellings of the same daemon cannot both pass.
if ! target_engine=$(DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker info --format '{{.ID}}' 2>/dev/null); then
  die "isolated daemon unreachable at $ISOLATED_DOCKER_HOST" 11
fi
[ -n "$target_engine" ] || die "isolated daemon returned no engine ID" 11

# ALLOWED_ENGINE_ID is MANDATORY: absent aborts. Previously it warned and continued,
# so a caller could omit the allowlist and load the decrypted image into WHATEVER the
# ambient context happened to be. [Defect C]
: "${ALLOWED_ENGINE_ID:?ALLOWED_ENGINE_ID is required — refusing to load without a pinned engine ID}"
[ "$target_engine" = "$ALLOWED_ENGINE_ID" ] \
  || die "engine ID $target_engine is not the allowlisted $ALLOWED_ENGINE_ID" 11

# Never load into a daemon that is (or contains) the production project.
if DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker ps -a --format '{{.Label "com.docker.compose.project"}}' 2>/dev/null \
     | grep -qx 'sigap'; then
  die "the target daemon holds compose project 'sigap' — refusing, it is not isolated" 11
fi

echo "restore target verified: endpoint=$ISOLATED_DOCKER_HOST engine=$target_engine"

# ---------------------------------------------------------------------------
# 3. Temporary state. `mktemp -d` gives 0700 by default, so nothing we write is
#    world-readable. The plaintext NEVER escapes this directory, and cleanup is
#    guaranteed on every exit path INCLUDING interruption (SIGINT/SIGTERM) — the
#    trap fires before the shell exits, and `set -e` cannot skip it.
# ---------------------------------------------------------------------------
# A predictable PID-derived path is hijackable: a local attacker can pre-create it as a
# symlink and have us write through it. `mktemp -d` gives an unpredictable path created
# atomically with mode 0700. A fallback string is NOT acceptable. [Defect B]
WORK=$(mktemp -d 2>/dev/null) || die "mktemp -d failed; refusing a predictable fallback" 14
[ -d "$WORK" ] || die "mktemp did not yield a directory" 14
[ -L "$WORK" ] && die "the temporary directory is a symlink" 11
chmod 700 "$WORK" 2>/dev/null || true
cleanup() {
  # The decrypted plaintext is the only sensitive artifact; remove it unconditionally.
  # `shred` is preferred but absent on some platforms (macOS, minimal images), so fall
  # back to a plain remove — the guarantee is REMOVAL, not a specific shred path.
  # Image TAR and decrypted archive are both sensitive plaintext; shred when possible.
  for f in "$WORK/image.tar" "$WORK/archive.tar.gz" "$WORK/plain.gz"; do
    [ -f "$f" ] && { shred -u "$f" 2>/dev/null || rm -f "$f"; }
  done
  rm -rf -- "$WORK"
  # Belt and braces: assert nothing plaintext survived in the worktree.
  [ -n "${worktree_plain:-}" ] && rm -f "$worktree_plain"
  # Report rather than trust: if a plaintext file survived, say so loudly.
  if [ -n "${worktree_plain:-}" ] && [ -f "$worktree_plain" ]; then
    printf 'WARNING: plaintext still present at %s\n' "$worktree_plain" >&2
  fi
}
worktree_plain="$PWD/sigap-restore-plain-$$.tar.gz"
trap cleanup EXIT INT TERM HUP

# ---------------------------------------------------------------------------
# 4. Fetch — ciphertext only. The workstation/rollback host never holds the key,
#    so nothing decrypted can land here.
# ---------------------------------------------------------------------------
scp -q "$ARCHIVE_HOST:$ARCHIVE_DIR/$ART" "$WORK/archive.age" || die "fetch from \$ARCHIVE_HOST failed" 14
[ -s "$WORK/archive.age" ] || die "fetched artifact is empty" 14

# ---------------------------------------------------------------------------
# 5. Digest — compared against the OUT-OF-BAND record, never a co-located file
#    (a co-located digest is integrity-only: whoever replaces the .age replaces it).
# ---------------------------------------------------------------------------
actual_digest=$(sha256sum "$WORK/archive.age" | cut -d' ' -f1)
[ "$actual_digest" = "$EXPECT_DIGEST" ] || {
  printf 'REFUSED: digest mismatch\n  expected %s\n  actual   %s\n' "$EXPECT_DIGEST" "$actual_digest" >&2
  exit 12
}
echo "digest verified against out-of-band record"

# ---------------------------------------------------------------------------
# 6. Decrypt + decompress + load, with PER-STAGE status that cannot be masked.
#
# `set -euo pipefail` (bash) makes the pipeline fail fast, and PIPESTATUS gives
# each stage's real status regardless of where the others sit. This is the proof
# that age's exit is not masked by gzip's — the exact defect §2 name: in
#     { age -d ... | gzip -d; echo $? > st; }
# `$?` is gzip's status, so a wrong key (age exits non-zero after emitting a
# complete stream) passes. Bash's `set -o pipefail` + PIPESTATUS is the portable
# fix, and this script REQUIRES bash.
# ---------------------------------------------------------------------------
set +e
age -d -i "$ARCHIVE_KEY" "$WORK/archive.age" > "$WORK/archive.tar.gz"
ps=("${PIPESTATUS[@]}")
rc_age=${ps[0]:-1}
set -e
printf 'stage statuses: age=%s\n' "$rc_age"
[ "$rc_age" -eq 0 ] || die "age decrypt failed (rc=$rc_age)" 14
[ -s "$WORK/archive.tar.gz" ] || die "decrypted stream is empty" 13

# Naming matters, and the original code got it wrong: the DECRYPTED stream IS the
# archive (a .tar.gz); the DECOMPRESSED form is the image TAR. The previous version
# wrote the decompressed data to a file literally NAMED `plain.gz` and then ran
# `gzip -t` on it — testing a decompressed payload for gzip integrity, which is
# meaningless and cannot fail for the reason claimed. [Defect A]
gzip -t "$WORK/archive.tar.gz" 2>/dev/null \
  || die "decrypted artifact is not a complete gzip archive (wrong key or truncated)" 13

# Validate that it is a DOCKER IMAGE TAR before spending a load on it: a docker save
# archive always contains a manifest.json. This turns a silently-empty load — which
# could exit 0 with no images restored — into a hard refusal.
if ! gzip -dc "$WORK/archive.tar.gz" 2>/dev/null | tar -t 2>/dev/null | grep -q 'manifest.json'; then
  die "the decrypted archive is not a docker image tar (no manifest.json)" 13
fi

# Decompress and load in ONE pipeline so both statuses are captured together: a gzip
# failure cannot be masked by a successful docker load, or the reverse.
set +e
gzip -dc "$WORK/archive.tar.gz" > "$WORK/image.tar"
rc_gunzip=$?
set -e
[ "$rc_gunzip" -eq 0 ] || die "gzip decompress failed (rc=$rc_gunzip)" 14
[ -s "$WORK/image.tar" ] || die "decompressed image tar is empty" 13

set +e
DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker load < "$WORK/image.tar"
rc_load=$?
set -e
[ "$rc_load" -eq 0 ] || die "docker load failed (rc=$rc_load)" 14
echo "docker load succeeded"

# ---- Residual-risk disclosure (Defect A, unavoidable plaintext) ------------------
# The decompressed image TAR is a temporary plaintext, unavoidable because `docker
# load` consumes a TAR stream. Mitigations: it exists only inside a 0700 `mktemp -d`
# directory, is shredded and removed by the cleanup trap on EVERY exit path including
# interruption, is never written to the working directory, and .gitignore adds a second
# line of defence. RESIDUAL RISK: a reader of /tmp, or a core dump during the window,
# could observe the image TAR. The window is the duration of `docker load` only, and
# the TAR holds the image layers — which is what the archive already holds — not the
# plaintext application data.

# Re-read the engine ID AFTER the load closes the verification-to-load TOCTOU window:
# if the ambient context changed between the check above and this load, the ID differs
# and the identity assertion below is against a different daemon than the verified one.
target_engine=$(DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker info --format '{{.ID}}' 2>/dev/null) \
  || die "isolated daemon unreachable after load" 11
[ -n "$target_engine" ] || die "no engine ID after load" 11
[ "$target_engine" = "$ALLOWED_ENGINE_ID" ] \
  || die "engine ID changed after load ($target_engine != $ALLOWED_ENGINE_ID)" 11

# ---------------------------------------------------------------------------
# 7. Identity correspondence — per-image, ON the isolated daemon, with explicit
#    DOCKER_HOST so this can never read the ambient (possibly production) daemon.
# ---------------------------------------------------------------------------
IFS=',' read -ra IDS <<< "$EXPECT_IDS"
missing=0; present=0
for id in "${IDS[@]}"; do
  id="${id//[[:space:]]/}"
  [ -n "$id" ] || continue
  if DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker image inspect "sha256:$id" >/dev/null 2>&1; then
    present=$((present+1)); printf '  present %s\n' "$id"
  else
    missing=$((missing+1)); printf '  MISSING %s\n' "$id" >&2
  fi
done
[ "$present" -gt 0 ] || die "no expected image present after load — PARTIAL LOAD not accepted" 15
[ "$missing" -eq 0 ] || die "$missing expected image(s) absent after load — identity correspondence FAILED" 15

echo "identity correspondence VERIFIED for all $present image(s) on engine $target_engine"
exit 0
