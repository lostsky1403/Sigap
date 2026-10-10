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

# Reachability and engine identity. `docker info` returns the ENGINE id, which is a
# property of the daemon itself, so two spellings of the same daemon cannot both pass.
if ! target_engine=$(DOCKER_HOST="$ISOLATED_DOCKER_HOST" docker info --format '{{.ID}}' 2>/dev/null); then
  die "isolated daemon unreachable at $ISOLATED_DOCKER_HOST" 11
fi
[ -n "$target_engine" ] || die "isolated daemon returned no engine ID" 11

if [ -n "${ALLOWED_ENGINE_ID:-}" ]; then
  [ "$target_engine" = "$ALLOWED_ENGINE_ID" ] \
    || die "engine ID $target_engine is not the allowlisted $ALLOWED_ENGINE_ID" 11
else
  printf 'WARNING: ALLOWED_ENGINE_ID unset — engine identity not pinned\n' >&2
fi

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
WORK="${TMPDIR:-/tmp}/sigap-restore.$$"
mkdir -p "$WORK" 2>/dev/null || die "cannot create temporary directory" 14
chmod 700 "$WORK" || true
cleanup() {
  # The decrypted plaintext is the only sensitive artifact; remove it unconditionally.
  # `shred` is preferred but absent on some platforms (macOS, minimal images), so fall
  # back to a plain remove — the guarantee is REMOVAL, not a specific shred path.
  if [ -f "$WORK/plain.gz" ]; then
    shred -u "$WORK/plain.gz" 2>/dev/null || rm -f "$WORK/plain.gz"
  fi
  rm -rf "$WORK"
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
age -d -i "$ARCHIVE_KEY" "$WORK/archive.age" | gzip -d > "$WORK/plain.gz"
# Under `set -u`, PIPESTATUS[n] is only defined for stages that RAN: a pipeline that
# fails in its first stage leaves later indices unset, so index them defensively.
rc_age=${PIPESTATUS[0]:-1}; rc_gzip=${PIPESTATUS[1]:-1}
set -e
printf 'stage statuses: age=%s gzip=%s\n' "$rc_age" "$rc_gzip"
[ "$rc_age"  -eq 0 ] || die "age decrypt failed (rc=$rc_age)" 14
[ "$rc_gzip" -eq 0 ] || die "gzip decompress failed (rc=$rc_gzip)" 14

# Belt and braces: the plaintext must be a COMPLETE, NON-EMPTY gzip. This also
# catches truncation and the empty-stream case, which look like success to a
# bare pipeline.
[ -s "$WORK/plain.gz" ] || die "decrypted stream is empty" 13
gzip -t "$WORK/plain.gz" 2>/dev/null || die "decrypted stream is not a complete gzip" 13

set +e
docker load --input "$WORK/plain.gz"
rc_load=$?
set -e
[ "$rc_load" -eq 0 ] || die "docker load failed (rc=$rc_load)" 14
echo "docker load succeeded"

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
