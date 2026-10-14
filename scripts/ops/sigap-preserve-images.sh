#!/usr/bin/env bash
# sigap-preserve-images.sh — source-side encrypted preservation of the three running
# SIGAP application images (runbook §13f, replacing the relay procedure).
#
#   docker image save <immutable-id>... | gzip -1 | age -r <PUBLIC_RECIPIENT> > <ciphertext>
#     -> upload to Cloudflare R2 (or any S3-compatible endpoint)
#     -> read back: ContentLength AND SHA-256 must match the local ciphertext
#     -> record URI, digest, source image IDs and timestamp OUT OF BAND
#
# Only the PUBLIC age recipient is accepted. A private identity (AGE-SECRET-KEY-...) is
# refused, so the private key cannot reach production through this path.
#
# REQUIRES BASH (PIPESTATUS, arrays, pipefail). Run as `bash scripts/ops/sigap-preserve-images.sh`.
#
# SOURCEABLE. Sourcing defines functions only; the test suite sources it to exercise
# verification directly. Nothing runs unless the file is executed.
#
# Required environment. Nothing is defaulted from ambient AWS_* state:
#   PRESERVE_RECIPIENT                public age recipient, `age1...`
#   PRESERVE_R2_ENDPOINT              https endpoint (loopback http is accepted for tests only)
#   PRESERVE_R2_BUCKET                bucket name
#   PRESERVE_UPLOAD_ACCESS_KEY_ID     PRODUCTION UPLOADER credential, write-scoped. NOT the
#   PRESERVE_UPLOAD_SECRET_ACCESS_KEY temporary retention-drill credential.
#   PRESERVE_RECORD_FILE              out-of-band record, outside the repo and the bucket
#   PRESERVE_STAMP                    release stamp, e.g. 20261014T120000Z
# Optional:
#   PRESERVE_DRILL_ACCESS_KEY_ID      the retention-drill key id; refused if it equals the uploader
#   PRESERVE_DOCKER_HOST              source daemon (default: ambient)
#   PRESERVE_CONTAINERS               container names (default: sigap-api sigap-web sigap-rust-engine)
#   PRESERVE_REGION                   signing region (default: auto)
#
# The ambient AWS_* variables are never read. Every AWS call runs with AWS_PROFILE unset and
# the shared config and credentials files pointed at an empty file.
#
# Exit codes:
#   0   preserved: uploaded AND read back (ContentLength and SHA-256 verified)
#   10  required input missing or malformed (including a non-public recipient)
#   11  a source container is missing, stopped, or has a malformed image ID; or duplicates
#   14  a pipeline stage failed (docker save / gzip / age), named per stage
#   16  upload or read-back command failed (endpoint unreachable, auth refused, ...)
#   17  read-back ContentLength did not match the local ciphertext
#   18  read-back SHA-256 did not match the local ciphertext
#   19  ciphertext does not carry the age v1 header
#
# A successful upload is NOT preservation. Preservation is claimed only at exit 0, after the
# read-back has been verified.

# Test seams: binaries may be overridden ONLY so the negative tests can make a real stage fail.
DOCKER_BIN=${PRESERVE_DOCKER_BIN:-docker}
GZIP_BIN=${PRESERVE_GZIP_BIN:-gzip}
AGE_BIN=${PRESERVE_AGE_BIN:-age}
AWS_BIN=${PRESERVE_AWS_BIN:-aws}

_fail() { printf 'REFUSED: %s\n' "$1" >&2; exit "${2:-14}"; }

# Canonical image digest: 64 lowercase hex with an optional `sha256:` prefix, returned prefixed.
_norm_image_id() {
  local v=${1#sha256:}
  [ "${#v}" -eq 64 ] || return 1
  case "$v" in *[!0-9a-f]*) return 1 ;; esac
  printf 'sha256:%s\n' "$v"
}

# Resolve each named container to the IMMUTABLE image ID it is running. Tags are never used,
# so a rebuild that retags cannot change what is saved. One `sha256:<digest>` per line.
resolve_running_images() {
  local name state image norm
  for name in "$@"; do
    state=$("$DOCKER_BIN" inspect --format '{{.State.Running}}' "$name" 2>/dev/null) \
      || _fail "container '$name' not found on the source daemon" 11
    [ "$state" = "true" ] || _fail "container '$name' is not running; refusing to preserve a stopped release" 11
    image=$("$DOCKER_BIN" inspect --format '{{.Image}}' "$name" 2>/dev/null) \
      || _fail "could not read the image ID of '$name'" 11
    norm=$(_norm_image_id "$image") || _fail "'$name' reports a malformed image ID" 11
    printf '%s\n' "$norm"
  done
}

# Two containers resolving to one image would archive the same bytes twice and report a
# three-image preservation that holds fewer. Refuse.
assert_distinct() {
  local dupes
  dupes=$(printf '%s\n' "$@" | sort | uniq -d)
  [ -z "$dupes" ] || _fail "two source containers resolve to the same image ID; refusing" 11
}

# Stream save | gzip | age into $1, capturing EVERY stage's status from ONE read of PIPESTATUS.
#  - Not a `{ ...; }` group: its assignments land in a subshell and are lost.
#  - Not repeated `${PIPESTATUS[n]}` reads: the first read resets the array, so later reads
#    of the same array would not be the other stages' statuses. The array is copied once.
# Sets STAGE_SAVE / STAGE_GZIP / STAGE_AGE. Returns 0 only if all three are 0.
stream_encrypt() {
  local out=$1; shift
  local ps
  set -o pipefail
  set +e
  "$DOCKER_BIN" image save "$@" | "$GZIP_BIN" -1 | "$AGE_BIN" -r "$PRESERVE_RECIPIENT" > "$out"
  ps=("${PIPESTATUS[@]}")
  set -e
  STAGE_SAVE=${ps[0]:-1}; STAGE_GZIP=${ps[1]:-1}; STAGE_AGE=${ps[2]:-1}
  [ "$STAGE_SAVE" -eq 0 ] && [ "$STAGE_GZIP" -eq 0 ] && [ "$STAGE_AGE" -eq 0 ]
}

# Structural check only: non-empty, begins with the age v1 header. Completeness is proven by
# decryption at restore time, the only place the private key exists.
check_age_header() {
  local head
  [ -s "$1" ] || return 1
  head=$(head -c 21 "$1" 2>/dev/null)
  case "$head" in 'age-encryption.org/v1'*) return 0 ;; esac
  return 1
}

sha256_of() { sha256sum "$1" | cut -d' ' -f1; }

# AWS CLI invocation. Only the uploader credential is supplied, and every ambient source is
# blanked, so nothing ambient is adopted.
aws_s3() {
  env -u AWS_PROFILE -u AWS_SESSION_TOKEN \
      AWS_ACCESS_KEY_ID="$PRESERVE_UPLOAD_ACCESS_KEY_ID" \
      AWS_SECRET_ACCESS_KEY="$PRESERVE_UPLOAD_SECRET_ACCESS_KEY" \
      AWS_CONFIG_FILE="$_NOPROFILE" AWS_SHARED_CREDENTIALS_FILE="$_NOPROFILE" \
      AWS_DEFAULT_REGION="${PRESERVE_REGION:-auto}" \
      "$AWS_BIN" s3api "$@" --endpoint-url "$PRESERVE_R2_ENDPOINT"
}

# Read-back verification against the REMOTE object, not the local file. A real HEAD gives the
# stored ContentLength; a real GET gives the bytes R2 returns, which are hashed.
# Sets READBACK_DIGEST. It runs in the PARENT shell and must not be command-substituted: a
# `_fail` inside `$(...)` exits only the subshell, and the caller would then report PRESERVED.
verify_readback() {
  local local_file=$1 bucket=$2 key=$3 dl=$4
  local local_len local_digest remote_len remote_digest
  local_len=$(wc -c < "$local_file" | tr -d ' ')
  local_digest=$(sha256_of "$local_file")

  remote_len=$(aws_s3 head-object --bucket "$bucket" --key "$key" \
                 --query ContentLength --output text 2>"$dl/head.err") \
    || _fail "read-back HEAD failed: $(sed -n '1p' "$dl/head.err" 2>/dev/null)" 16
  [ "$remote_len" = "$local_len" ] \
    || _fail "read-back length mismatch (remote=${remote_len:-none} local=$local_len): object is not the artifact" 17

  aws_s3 get-object --bucket "$bucket" --key "$key" "$dl/readback.age" \
    >/dev/null 2>"$dl/get.err" \
    || _fail "read-back GET failed: $(sed -n '1p' "$dl/get.err" 2>/dev/null)" 16
  remote_digest=$(sha256_of "$dl/readback.age")
  [ "$remote_digest" = "$local_digest" ] \
    || _fail "read-back SHA-256 mismatch: the object in R2 is NOT the ciphertext that was produced" 18
  READBACK_DIGEST=$local_digest
}

# Out-of-band record: identifiers and digest only. Never image contents, never a key.
write_record() {
  local uri=$1 digest=$2 bytes=$3; shift 3
  {
    printf 'stamp=%s\n' "$PRESERVE_STAMP"
    printf 'recorded_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'archive_uri=%s\n' "$uri"
    printf 'sha256=%s\n' "$digest"
    printf 'bytes=%s\n' "$bytes"
    printf 'source_image_ids=%s\n' "$*"
  } > "$PRESERVE_RECORD_FILE"
}

main() {
  umask 077

  local v
  for v in PRESERVE_RECIPIENT PRESERVE_R2_ENDPOINT PRESERVE_R2_BUCKET \
           PRESERVE_UPLOAD_ACCESS_KEY_ID PRESERVE_UPLOAD_SECRET_ACCESS_KEY \
           PRESERVE_RECORD_FILE PRESERVE_STAMP; do
    [ -n "${!v:-}" ] || _fail "$v is required" 10
  done
  case "$PRESERVE_RECIPIENT" in
    AGE-SECRET-KEY-*) _fail "PRESERVE_RECIPIENT is a PRIVATE identity; only the public age1... recipient is accepted" 10 ;;
    age1*) ;;
    *) _fail "PRESERVE_RECIPIENT is not an age1... public recipient" 10 ;;
  esac
  case "$PRESERVE_RECIPIENT" in *[!a-z0-9]*) _fail "PRESERVE_RECIPIENT contains invalid characters" 10 ;; esac
  case "$PRESERVE_R2_ENDPOINT" in
    https://*|http://127.0.0.1:*|http://localhost:*) ;;
    *) _fail "PRESERVE_R2_ENDPOINT must be https:// (loopback http is for tests only)" 10 ;;
  esac
  case "$PRESERVE_STAMP" in *[!0-9TZ]*|'') _fail "PRESERVE_STAMP must look like 20261014T120000Z" 10 ;; esac
  if [ -n "${PRESERVE_DRILL_ACCESS_KEY_ID:-}" ] \
     && [ "$PRESERVE_DRILL_ACCESS_KEY_ID" = "$PRESERVE_UPLOAD_ACCESS_KEY_ID" ]; then
    _fail "PRESERVE_UPLOAD_ACCESS_KEY_ID is the retention-drill credential; use the production uploader" 10
  fi

  local WORK
  WORK=$(mktemp -d) || _fail "mktemp -d failed" 14
  trap 'rm -rf "$WORK"' EXIT INT TERM HUP
  _NOPROFILE="$WORK/no-profile"; : > "$_NOPROFILE"

  if [ -n "${PRESERVE_DOCKER_HOST:-}" ]; then export DOCKER_HOST="$PRESERVE_DOCKER_HOST"; fi

  local names ids=() line
  if [ -n "${PRESERVE_CONTAINERS:-}" ]; then
    read -r -a names <<< "$PRESERVE_CONTAINERS"
  else
    names=(sigap-api sigap-web sigap-rust-engine)
  fi
  while IFS= read -r line; do ids+=("$line"); done < <(resolve_running_images "${names[@]}")
  [ "${#ids[@]}" -eq "${#names[@]}" ] || _fail "resolved ${#ids[@]} of ${#names[@]} images" 11
  assert_distinct "${ids[@]}"
  printf 'source images (running, by immutable ID): %s\n' "${ids[*]}"

  local ART="sigap-pre-redesign-$PRESERVE_STAMP.tar.gz.age"
  local CT="$WORK/$ART"
  if ! stream_encrypt "$CT" "${ids[@]}"; then
    rm -f "$CT"
    _fail "stage failure: docker-save=$STAGE_SAVE gzip=$STAGE_GZIP age=$STAGE_AGE; no artifact produced" 14
  fi
  check_age_header "$CT" || _fail "ciphertext does not carry the age v1 header" 19
  local digest bytes
  digest=$(sha256_of "$CT")
  bytes=$(wc -c < "$CT" | tr -d ' ')
  printf 'ciphertext: %s bytes sha256=%s\n' "$bytes" "$digest"

  local bucket=$PRESERVE_R2_BUCKET key="sigap/$ART" uri="r2://$PRESERVE_R2_BUCKET/sigap/$ART"
  aws_s3 put-object --bucket "$bucket" --key "$key" --body "$CT" >/dev/null 2>"$WORK/put.err" \
    || _fail "upload failed: $(sed -n '1p' "$WORK/put.err" 2>/dev/null)" 16

  verify_readback "$CT" "$bucket" "$key" "$WORK"
  write_record "$uri" "$READBACK_DIGEST" "$bytes" "${ids[@]}"
  printf 'PRESERVED: %s\nSHA256: %s\nRECORD: %s\n' "$uri" "$READBACK_DIGEST" "$PRESERVE_RECORD_FILE"
  exit 0
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
