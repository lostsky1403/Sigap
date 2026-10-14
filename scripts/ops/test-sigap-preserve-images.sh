#!/usr/bin/env bash
# test-sigap-preserve-images.sh — proves sigap-preserve-images.sh cannot report PRESERVED
# for a failure, and that a real round trip restores the archived image IDs.
#
# Uses a REAL Docker daemon, REAL gzip and REAL age. The object store is a LOCAL S3-compatible
# endpoint on loopback supplied by the caller (MinIO), never a production endpoint:
#   TEST_S3_ENDPOINT     e.g. http://127.0.0.1:9000
#   TEST_S3_ACCESS_KEY   local test user
#   TEST_S3_SECRET_KEY   local test secret
# The suite creates a unique bucket and removes it, and creates and removes only containers
# and images named with this run's id. It never touches a container or image it did not create.
# Age identities are generated in a temp dir and removed on exit; none is committed.
#
# Fault injection replaces ONE tool with a shim that wraps the real tool, so every other stage
# stays real. Every negative asserts the specific exit code AND the specific failure reason.
#
# Exit codes: 0 all pass, 1 any assertion failed, 2 environment unusable (refuses to run).

set -u

HERE=$(cd "$(dirname "$0")" && pwd)
PRESERVE="$HERE/sigap-preserve-images.sh"

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-42s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-42s %s\n' "$1" "$2"; }

# ---------------------------------------------------------------------------
# Environment gate. Refuse rather than skip: a skipped preservation test proves nothing.
# ---------------------------------------------------------------------------
for t in docker gzip age age-keygen go sha256sum tar; do
  command -v "$t" >/dev/null 2>&1 || { echo "ENV: required tool '$t' is missing" >&2; exit 2; }
done
[ -n "${TEST_S3_ENDPOINT:-}" ] && [ -n "${TEST_S3_ACCESS_KEY:-}" ] && [ -n "${TEST_S3_SECRET_KEY:-}" ] \
  || { echo "ENV: TEST_S3_ENDPOINT / TEST_S3_ACCESS_KEY / TEST_S3_SECRET_KEY are required (local MinIO)" >&2; exit 2; }
case "$TEST_S3_ENDPOINT" in
  http://127.0.0.1:*|http://localhost:*) ;;
  *) echo "ENV: TEST_S3_ENDPOINT must be loopback; refusing a non-local endpoint" >&2; exit 2 ;;
esac

RUN=$(date -u +%H%M%S)$$
# Native Windows binaries (docker.exe, age.exe) cannot resolve MSYS /tmp paths, so the work
# directory is given in mixed Windows form when cygpath is available.
W_RAW=$(mktemp -d) || { echo "ENV: mktemp failed" >&2; exit 2; }
if command -v cygpath >/dev/null 2>&1; then W=$(cygpath -m "$W_RAW"); else W=$W_RAW; fi
mkdir -p "$W_RAW/tmp" "$W_RAW/rootfs"
BUCKET="sigap-preserve-test-$RUN"
C_API="sigap-preserve-test-$RUN-api"
C_WEB="sigap-preserve-test-$RUN-web"
C_ENG="sigap-preserve-test-$RUN-engine"
IMG_REPO="sigap-preserve-test/$RUN"

# Real AWS CLI with NO ambient profile. Prefer `aws`; fall back to the python module.
if command -v aws >/dev/null 2>&1; then
  REAL_AWS=$(command -v aws)
else
  printf '#!/bin/sh\nexec python -m awscli "$@"\n' > "$W_RAW/aws-real"; chmod +x "$W_RAW/aws-real"
  REAL_AWS="$W/aws-real"
fi
REAL_DOCKER=$(command -v docker)

cleanup() {
  for c in "$C_API" "$C_WEB" "$C_ENG"; do docker rm -f "$c" >/dev/null 2>&1; done
  for id in $(docker images -q "$IMG_REPO/*" 2>/dev/null) ${ALL_IDS:-}; do docker rmi -f "$id" >/dev/null 2>&1; done
  env AWS_ACCESS_KEY_ID="$TEST_S3_ACCESS_KEY" AWS_SECRET_ACCESS_KEY="$TEST_S3_SECRET_KEY" \
      AWS_CONFIG_FILE="$W/none" AWS_SHARED_CREDENTIALS_FILE="$W/none" AWS_DEFAULT_REGION=us-east-1 \
      "$REAL_AWS" s3 rb "s3://$BUCKET" --force --endpoint-url "$TEST_S3_ENDPOINT" >/dev/null 2>&1
  rm -rf "$W_RAW"
}
trap cleanup EXIT INT TERM HUP

# Test-side aws, independent of the script: used to inspect the store directly.
t_aws() {
  env AWS_ACCESS_KEY_ID="$TEST_S3_ACCESS_KEY" AWS_SECRET_ACCESS_KEY="$TEST_S3_SECRET_KEY" \
      AWS_CONFIG_FILE="$W/none" AWS_SHARED_CREDENTIALS_FILE="$W/none" AWS_DEFAULT_REGION=us-east-1 \
      "$REAL_AWS" s3api "$@" --endpoint-url "$TEST_S3_ENDPOINT"
}

t_aws create-bucket --bucket "$BUCKET" >/dev/null 2>"$W_RAW/mb.err" \
  || { echo "ENV: could not create test bucket on $TEST_S3_ENDPOINT: $(head -1 "$W_RAW/mb.err")" >&2; exit 2; }

# ---------------------------------------------------------------------------
# Fixtures: three DISTINCT synthetic images from a real `docker import`, each run as a real
# container. The process is a static Go binary, so no shell or sleep is needed in the image.
# ---------------------------------------------------------------------------
printf 'package main\nimport "time"\nfunc main(){ for { time.Sleep(time.Hour) } }\n' > "$W_RAW/sleeper.go"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$W_RAW/sleeper" "$W_RAW/sleeper.go" 2>"$W_RAW/build.err" \
  || { echo "ENV: go build of fixture failed: $(head -1 "$W_RAW/build.err")" >&2; exit 2; }

ALL_IDS=""
make_image() {  # make_image <tag> <marker> — prints the image ID on success
  local dir="$W_RAW/rootfs/$1"
  rm -rf "$dir"; mkdir -p "$dir"
  cp "$W_RAW/sleeper" "$dir/sleeper"; chmod 755 "$dir/sleeper"
  printf '%s\n' "$2" > "$dir/marker.txt"
  # Build the tar with an explicit 0755 mode for the binary. A host chmod is a no-op on Windows
  # and the container would then fail with `permission denied`.
  tar --mode=0755 -C "$dir" -cf "$W_RAW/$1.tar" sleeper marker.txt 2>"$W_RAW/tar.err" || return 1
  docker import --change 'CMD ["/sleeper"]' "$W/$1.tar" "$IMG_REPO-$1:s" 2>"$W_RAW/imp.err" >/dev/null || return 1
  docker image inspect -f '{{.Id}}' "$IMG_REPO-$1:s"
}
ID_API=$(make_image api "api-$RUN")    || { echo "ENV: fixture import failed: $(head -1 "$W_RAW/imp.err")" >&2; exit 2; }
ID_WEB=$(make_image web "web-$RUN")    || { echo "ENV: fixture import failed" >&2; exit 2; }
ID_ENG=$(make_image engine "engine-$RUN") || { echo "ENV: fixture import failed" >&2; exit 2; }
ALL_IDS="$ID_API $ID_WEB $ID_ENG"
[ "$ID_API" != "$ID_WEB" ] && [ "$ID_WEB" != "$ID_ENG" ] && [ "$ID_API" != "$ID_ENG" ] \
  || { echo "ENV: fixture images are not distinct" >&2; exit 2; }

start_fixtures() {
  for c in "$C_API" "$C_WEB" "$C_ENG"; do docker rm -f "$c" >/dev/null 2>&1; done
  docker run -d --name "$C_API" "$ID_API" >/dev/null || return 1
  docker run -d --name "$C_WEB" "$ID_WEB" >/dev/null || return 1
  docker run -d --name "$C_ENG" "$ID_ENG" >/dev/null || return 1
}
start_fixtures || { echo "ENV: could not start fixture containers" >&2; exit 2; }
PRESERVE_CONTAINERS="$C_API $C_WEB $C_ENG"

# Identities: A is the recovery identity; B is a second, unrelated identity for the
# wrong-recipient negative. Both are throwaway test keys.
age-keygen -o "$W_RAW/id-a.txt" 2>/dev/null; age-keygen -o "$W_RAW/id-b.txt" 2>/dev/null
RECIP_A=$(age-keygen -y "$W_RAW/id-a.txt"); RECIP_B=$(age-keygen -y "$W_RAW/id-b.txt")

# ---------------------------------------------------------------------------
# Shims. Each wraps ONE real tool. Selected via PRESERVE_*_BIN or PRESERVE_AWS_BIN.
# ---------------------------------------------------------------------------
# docker: `image save` emits a PARTIAL stream and exits 7; everything else is real docker.
cat > "$W_RAW/docker-save-fail" <<EOF
#!/bin/sh
if [ "\$1" = image ] && [ "\$2" = save ]; then
  "$REAL_DOCKER" "\$@" | head -c 4096
  exit 7
fi
exec "$REAL_DOCKER" "\$@"
EOF
cat > "$W_RAW/gzip-fail" <<'EOF'
#!/bin/sh
cat >/dev/null
exit 2
EOF
cat > "$W_RAW/age-fail" <<'EOF'
#!/bin/sh
cat >/dev/null
exit 3
EOF
# aws: one injected fault per FAULT value, otherwise real. Each fault is a fixed, bounded
# branch; there is no argument-rewriting loop.
#   truncate-put: the uploaded body is replaced with its first 1000 bytes.
#   wrong-len:    HEAD reports one byte more than the stored object.
#   wrong-digest: GET writes the real bytes with the first byte altered.
cat > "$W_RAW/aws-fault" <<EOF
#!/bin/sh
REAL="$REAL_AWS"
if [ "\$FAULT" = truncate-put ] && [ "\$2" = put-object ]; then
  # Forward every argument unchanged EXCEPT the value after --body, which becomes a 1000-byte
  # prefix of the real file. Rebuilt in place: one pass over argv, no splitting, no re-entry.
  take=0; n=\$#; i=0
  while [ "\$i" -lt "\$n" ]; do
    a=\$1; shift; i=\$((i+1))
    if [ "\$take" = 1 ]; then head -c 1000 "\$a" > "\$FAULT_TMP"; a="\$FAULT_TMP"; take=0
    elif [ "\$a" = "--body" ]; then take=1; fi
    set -- "\$@" "\$a"
  done
  exec "\$REAL" "\$@"
fi
if [ "\$FAULT" = wrong-len ] && [ "\$2" = head-object ]; then
  n=\$("\$REAL" "\$@") || exit \$?
  echo \$((n+1)); exit 0
fi
if [ "\$FAULT" = wrong-digest ] && [ "\$2" = get-object ]; then
  "\$REAL" "\$@" >/dev/null || exit \$?
  # The destination is the argument immediately before --endpoint-url (aws_s3 appends that last).
  dest=""; prev=""
  for a in "\$@"; do
    if [ "\$a" = "--endpoint-url" ]; then dest="\$prev"; break; fi
    prev="\$a"
  done
  printf 'X' | dd of="\$dest" bs=1 seek=0 conv=notrunc 2>/dev/null || exit 1
  exit 0
fi
exec "\$REAL" "\$@"
EOF
chmod +x "$W_RAW/docker-save-fail" "$W_RAW/gzip-fail" "$W_RAW/age-fail" "$W_RAW/aws-fault"
export FAULT_TMP="$W/trunc.part"

# run_case <stamp> [VAR=value ...] — runs the script with a clean, explicit environment.
# Sets RC and OUT. The script's TMPDIR is redirected so residue is visible to the test.
run_case() {
  local stamp=$1; shift
  RECORD="$W/record-$stamp.txt"
  OUT=$(env -u AWS_PROFILE -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY \
        PRESERVE_RECIPIENT="$RECIP_A" PRESERVE_R2_ENDPOINT="$TEST_S3_ENDPOINT" \
        PRESERVE_R2_BUCKET="$BUCKET" PRESERVE_UPLOAD_ACCESS_KEY_ID="$TEST_S3_ACCESS_KEY" \
        PRESERVE_UPLOAD_SECRET_ACCESS_KEY="$TEST_S3_SECRET_KEY" PRESERVE_RECORD_FILE="$RECORD" \
        PRESERVE_STAMP="$stamp" PRESERVE_CONTAINERS="$PRESERVE_CONTAINERS" \
        PRESERVE_AWS_BIN="$W/aws-fault" TMPDIR="$W/tmp" FAULT="${FAULT:-}" "$@" \
        bash "$PRESERVE" 2>&1)
  RC=$?
}

object_key() { printf 'sigap/sigap-pre-redesign-%s.tar.gz.age' "$1"; }
object_exists() { t_aws head-object --bucket "$BUCKET" --key "$(object_key "$1")" >/dev/null 2>&1; }

expect_rc() {   # expect_rc <name> <want>
  if [ "$RC" -eq "$2" ]; then ok "$1" "exit $RC"; else bad "$1" "exit $RC, want $2: $OUT"; fi
}
expect_msg() {  # expect_msg <name> <ERE>
  if printf '%s\n' "$OUT" | grep -Eq "$2"; then ok "$1" "matched /$2/"
  else bad "$1" "no /$2/ in: $(printf '%s' "$OUT" | tr '\n' ' ')"; fi
}
expect_no_success() {  # a refused run must never claim preservation or leave a record
  if printf '%s\n' "$OUT" | grep -q '^PRESERVED:' || [ -e "$RECORD" ]; then
    bad "$1" "a failing run reported PRESERVED or wrote a record"
  else ok "$1" "no PRESERVED line, no record"; fi
}
expect_not_uploaded() { # the object must not exist after a pre-upload failure
  if object_exists "$2"; then bad "$1" "object was uploaded despite the failure"
  else ok "$1" "object not uploaded"; fi
}

# ===========================================================================
# POSITIVE: the real round trip.
# ===========================================================================
printf '=== positive: save -> gzip -> age -> store -> read-back -> decrypt -> load ===\n'
STAMP_OK=20261014T120000Z
run_case "$STAMP_OK"
expect_rc  "positive-preserve" 0
expect_msg "positive-claims-preserved" '^PRESERVED: r2://'
REC_SHA=$(sed -n 's/^sha256=//p' "$RECORD" 2>/dev/null)
REC_IDS=$(sed -n 's/^source_image_ids=//p' "$RECORD" 2>/dev/null)
REC_URI=$(sed -n 's/^archive_uri=//p' "$RECORD" 2>/dev/null)
if [ -n "$REC_SHA" ] && [ -n "$REC_IDS" ]; then ok "record-written-out-of-band" "uri/sha256/ids recorded"
else bad "record-written-out-of-band" "record incomplete: $(cat "$RECORD" 2>/dev/null)"; fi
[ "$REC_URI" = "r2://$BUCKET/$(object_key "$STAMP_OK")" ] && ok "record-uri-matches-key" "$REC_URI" \
  || bad "record-uri-matches-key" "uri=$REC_URI"

# Three distinct IDs, selected by running container.
n=$(printf '%s\n' $REC_IDS | wc -l | tr -d ' ')
u=$(printf '%s\n' $REC_IDS | sort -u | wc -l | tr -d ' ')
if [ "$n" -eq 3 ] && [ "$u" -eq 3 ]; then ok "three-distinct-source-ids" "3 distinct immutable IDs"
else bad "three-distinct-source-ids" "got $n ids, $u distinct"; fi

# The stored object, fetched independently by the TEST, must match the recorded digest.
if t_aws get-object --bucket "$BUCKET" --key "$(object_key "$STAMP_OK")" "$W/fetched.age" >/dev/null 2>"$W_RAW/fetch.err"; then
  GOT=$(sha256sum "$W_RAW/fetched.age" | cut -d' ' -f1)
  if [ "$GOT" = "$REC_SHA" ]; then ok "independent-digest-matches-record" "sha256 $GOT"
  else bad "independent-digest-matches-record" "stored=$GOT recorded=$REC_SHA"; fi
else
  bad "independent-fetch" "test could not fetch the stored object: $(head -1 "$W_RAW/fetch.err")"
fi

# Decrypt with the recovery identity, decompress, and load into the REAL daemon.
if age -d -i "$W/id-a.txt" -o "$W/image.tar.gz" "$W/fetched.age" 2>"$W_RAW/dec.err" \
   && gzip -dc "$W_RAW/image.tar.gz" > "$W_RAW/image.tar" 2>>"$W_RAW/dec.err"; then
  ok "decrypt-and-decompress" "age -d | gzip -d succeeded"
else
  bad "decrypt-and-decompress" "$(head -1 "$W_RAW/dec.err")"
fi

# Remove the source containers and images, so the load CANNOT succeed by finding the image
# already present: the identity compared is the one the archive restores.
for c in "$C_API" "$C_WEB" "$C_ENG"; do docker rm -f "$c" >/dev/null 2>&1; done
docker rmi -f "$ID_API" "$ID_WEB" "$ID_ENG" >/dev/null 2>&1
still=0
for id in $REC_IDS; do docker image inspect "$id" >/dev/null 2>&1 && still=$((still+1)); done
if [ "$still" -eq 0 ]; then ok "source-removed-before-load" "all three source images absent from daemon"
else bad "source-removed-before-load" "$still source image(s) still present; load would prove nothing"; fi

docker load -i "$W/image.tar" > "$W_RAW/load.out" 2>&1 || true
LOADED=$(sed -n 's/^Loaded image ID: //p' "$W_RAW/load.out")
GOT_SET=$(printf '%s\n' $LOADED | sort | tr '\n' ' ')
WANT_SET=$(printf '%s\n' $REC_IDS | sort | tr '\n' ' ')
if [ -n "$LOADED" ] && [ "$GOT_SET" = "$WANT_SET" ]; then
  ok "round-trip-identity" "load restored exactly the recorded IDs"
else
  bad "round-trip-identity" "loaded=[$GOT_SET] recorded=[$WANT_SET] load=$(head -2 "$W_RAW/load.out")"
fi

# Recreate the fixtures from the restored IDs so the negatives have live sources.
# The restored images ARE the recorded IDs, so each container name maps to the same ID.
ID_API=$(printf '%s\n' $REC_IDS | sed -n 1p)
ID_WEB=$(printf '%s\n' $REC_IDS | sed -n 2p)
ID_ENG=$(printf '%s\n' $REC_IDS | sed -n 3p)
start_fixtures || { echo "ENV: could not recreate fixtures from restored IDs" >&2; exit 2; }

left=$(find "$W_RAW/tmp" -type f 2>/dev/null | wc -l | tr -d ' ')
if [ "$left" -eq 0 ]; then ok "no-temp-residue" "script TMPDIR empty after run"
else bad "no-temp-residue" "$left file(s) left in script TMPDIR"; fi

# ===========================================================================
# NEGATIVE: each must refuse with its SPECIFIC reason.
# ===========================================================================
printf '=== negative: every failure is refused with its specific reason ===\n'

# 1. docker save fails. gzip and age SUCCEED, so an artifact is genuinely produced from the
#    partial stream. This is the masking case: it must still refuse.
run_case 20261014T120001 PRESERVE_DOCKER_BIN="$W/docker-save-fail"
expect_rc  "neg-docker-save-fails" 14
expect_msg "neg-docker-save-reason" 'docker-save=7 gzip=0 age=0'
expect_no_success "neg-docker-save-no-success"
expect_not_uploaded "neg-docker-save-not-uploaded" 20261014T120001

# 2. gzip fails.
run_case 20261014T120002 PRESERVE_GZIP_BIN="$W/gzip-fail"
expect_rc  "neg-gzip-fails" 14
expect_msg "neg-gzip-reason" 'gzip=2'
expect_no_success "neg-gzip-no-success"
expect_not_uploaded "neg-gzip-not-uploaded" 20261014T120002

# 3. age fails.
run_case 20261014T120003 PRESERVE_AGE_BIN="$W/age-fail"
expect_rc  "neg-age-fails" 14
expect_msg "neg-age-reason" 'age=3'
expect_no_success "neg-age-no-success"
expect_not_uploaded "neg-age-not-uploaded" 20261014T120003

# 4. Truncated object in the store: the upload is cut short, but the command exits 0.
FAULT=truncate-put run_case 20261014T120004
expect_rc  "neg-truncated-stored-object" 17
expect_msg "neg-truncated-stored-reason" 'read-back length mismatch .*not the artifact'
expect_no_success "neg-truncated-no-success"

# 5. Read-back length mismatch (HEAD reports a different length than was uploaded).
FAULT=wrong-len run_case 20261014T120005
expect_rc  "neg-readback-length" 17
expect_msg "neg-readback-length-reason" 'read-back length mismatch'
expect_no_success "neg-readback-length-no-success"

# 6. Read-back digest mismatch: same length, one byte altered on read-back.
FAULT=wrong-digest run_case 20261014T120006
expect_rc  "neg-readback-digest" 18
expect_msg "neg-readback-digest-reason" 'read-back SHA-256 mismatch'
expect_no_success "neg-readback-digest-no-success"

# 7. Unreachable endpoint (nothing listens on the port).
run_case 20261014T120007 PRESERVE_R2_ENDPOINT=http://127.0.0.1:1
expect_rc  "neg-unreachable-endpoint" 16
expect_msg "neg-unreachable-reason" 'upload failed'
expect_no_success "neg-unreachable-no-success"

# 8. A PRIVATE identity supplied as the recipient is refused before any work, and not echoed.
PRIV=$(grep '^AGE-SECRET-KEY-' "$W_RAW/id-a.txt")
run_case 20261014T120008 PRESERVE_RECIPIENT="$PRIV"
expect_rc  "neg-private-key-as-recipient" 10
expect_msg "neg-private-key-reason" 'PRIVATE identity'
if printf '%s\n' "$OUT" | grep -q 'AGE-SECRET-KEY-1'; then bad "neg-private-key-not-echoed" "refusal echoed a secret"
else ok "neg-private-key-not-echoed" "refusal does not echo the secret"; fi

# 9. The drill credential reused as the uploader is refused.
run_case 20261014T120009 PRESERVE_DRILL_ACCESS_KEY_ID="$TEST_S3_ACCESS_KEY"
expect_rc  "neg-drill-credential-reused" 10
expect_msg "neg-drill-credential-reason" 'retention-drill credential'

# 10. Duplicate source: two names resolving to one image must not report three images.
run_case 20261014T120010 PRESERVE_CONTAINERS="$C_API $C_API $C_ENG"
expect_rc  "neg-duplicate-source" 11
expect_msg "neg-duplicate-reason" 'same image ID'

# 11. Stopped source: a stopped container must not be preserved as if it were the release.
docker stop "$C_WEB" >/dev/null 2>&1
run_case 20261014T120011
expect_rc  "neg-stopped-source" 11
expect_msg "neg-stopped-reason" "'$C_WEB' is not running"
docker start "$C_WEB" >/dev/null 2>&1

# ---- restore-side age failures, on REAL ciphertext ---------------------------
printf '=== negative: restore-side age failures on real ciphertext ===\n'

# 12. Truncated ciphertext: age must refuse and must not produce a complete plaintext.
SIZE=$(wc -c < "$W_RAW/fetched.age" | tr -d ' ')
head -c $((SIZE / 2)) "$W_RAW/fetched.age" > "$W_RAW/trunc.age"
age -d -i "$W/id-a.txt" -o "$W_RAW/trunc.out" "$W/trunc.age" 2>"$W_RAW/trunc.err"; rc_t=$?
if [ "$rc_t" -ne 0 ]; then ok "neg-truncated-ciphertext" "age exit $rc_t: $(head -1 "$W_RAW/trunc.err")"
else bad "neg-truncated-ciphertext" "age accepted a truncated archive (rc=0)"; fi

# 13. Wrong recipient: an archive encrypted to B must not decrypt with identity A.
run_case 20261014T120013 PRESERVE_RECIPIENT="$RECIP_B"
if [ "$RC" -eq 0 ]; then ok "wrong-recipient-archive-made" "archive encrypted to recipient B"
else bad "wrong-recipient-archive-made" "exit $RC: $OUT"; fi
if t_aws get-object --bucket "$BUCKET" --key "$(object_key 20261014T120013)" "$W/wrong.age" >/dev/null 2>&1; then
  age -d -i "$W/id-a.txt" -o "$W_RAW/wrong.out" "$W/wrong.age" 2>"$W_RAW/wrong.err"; rc_w=$?
  if [ "$rc_w" -ne 0 ] && grep -q 'no identity matched' "$W_RAW/wrong.err"; then
    ok "neg-wrong-recipient" "age exit $rc_w: no identity matched"
  else bad "neg-wrong-recipient" "rc=$rc_w err=$(head -1 "$W_RAW/wrong.err")"; fi
else
  bad "neg-wrong-recipient" "could not fetch the wrong-recipient object"
fi

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
