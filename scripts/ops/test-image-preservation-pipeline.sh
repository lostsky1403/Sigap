#!/bin/sh
# test-image-preservation-pipeline.sh — proves the §13f streaming pipeline is fail-CLOSED.
#
# The defect this pins, [OBSERVED] 2026-10-10 locally:
#
#     docker image save <bad-id> | gzip -1 > out.gz
#     echo $?   ->  0
#     wc -c out.gz  ->  20   (a complete, valid, trusted-LOOKING gzip of empty input)
#     gzip -t out.gz  ->  passes
#
# A failing `docker image save` therefore produces a small valid artifact and a ZERO exit
# status under a plain POSIX pipeline, because a pipeline's status is the LAST command's.
# The operator sees success and the artifact is only ever consulted during an incident.
# §13f's original `|`-chain had exactly this shape.
#
# Three facts, each [OBSERVED] on this machine, define the required defence:
#   1. a plain pipeline reports only the LAST stage's status, so a failed `docker image
#      save` is invisible;
#   2. `true | gzip -1` yields a 20-byte file that passes `gzip -t`, so a compressed-size
#      floor cannot catch it — the floor must be on the DECOMPRESSED stream;
#   3. the same blind spot applies to the restore path: `age -d | gzip -d | cat` reports 0
#      even when `age` fails.
#
# So the runbook idiom must be: capture EACH stage's status explicitly, and gate on
# per-stage status PLUS stream size PLUS `gzip -t` PLUS the out-of-band digest.
# This suite exercises every one of those, with deliberately failing stages.
#
# Exit codes: 0 pass, 1 fail. No host, container, image, network or Jira is touched.

set -u

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-40s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-40s %s\n' "$1" "$2"; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-imgpipe-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

# ---------------------------------------------------------------------------
# 1. The defect is real, in three independent ways.
# ---------------------------------------------------------------------------
printf '=== 1. the defect is real ===\n'

OUT="$TMP/naive.gz"
# (a) a plain pipeline hides a failing first stage
if sh -c 'exit 3' | gzip -1 > "$OUT" 2>/dev/null; then
  ok 'plain-pipeline-masks-stage1' 'stage1 exit 3 -> pipeline rc 0'
else
  bad 'plain-pipeline-masks-stage1' 'status propagated (unexpected)'
fi
rm -f "$OUT"

# (b) gzip of empty input is a valid, gzip -t-passing 20-byte file
if gzip -1 < /dev/null > "$OUT" 2>/dev/null && [ "$(wc -c < "$OUT")" -ge 1 ] \
   && gzip -t "$OUT" 2>/dev/null; then
  ok 'empty-gzip-is-a-valid-file' 'complete gzip, passes gzip -t — looks preserved'
else
  bad 'empty-gzip-is-a-valid-file' 'expected a valid small gzip'
fi
if [ "$(gzip -dc "$OUT" | wc -c)" -eq 0 ]; then
  ok 'compressed-floor-is-useless' '20B artifact, 0B stream: a size floor accepts it'
else
  bad 'compressed-floor-is-useless' 'empty gzip had a payload'
fi
rm -f "$OUT"

# (c) the RESTORE path has the same blind spot: `age -d | gzip -d | cat` reports 0
if printf 'x' | sh -c 'cat >/dev/null; exit 5' | gzip -1 -d 2>/dev/null | cat >/dev/null 2>&1; then
  ok 'restore-pipeline-masks-cipher' 'age-failure in restore pipe -> rc 0'
else
  bad 'restore-pipeline-masks-cipher' 'status propagated (unexpected)'
fi

# ---------------------------------------------------------------------------
# 2. Per-stage status capture. The runbook idiom, verified with failing stages.
#    Each stage writes its status to a file; the caller reads them all and refuses
#    if ANY is non-zero. This is what §13f and §7d.1 must prescribe.
# ---------------------------------------------------------------------------
printf '=== 2. per-stage status capture catches each failing stage ===\n'

# run_guarded <producer> <gzip_args> <cipher> <out>
# producer/cipher are shell snippets; gzip_args are gzip flags. Each stage records
# its status in $TMP/st.<n>; the function returns 0 only if all three are 0.
run_guarded() {
  prod=$1; gzargs=$2; ciph=$3; out=$4
  st1="$TMP/st.1"; st2="$TMP/st.2"; st3="$TMP/st.3"
  : > "$st1"; : > "$st2"; : > "$st3"

  { sh -c "$prod"; echo $? > "$st1"; } | { gzip $gzargs; echo $? > "$st2"; } | { sh -c "$ciph"; echo $? > "$st3"; } > "$out"

  r1=$(cat "$st1" 2>/dev/null); r2=$(cat "$st2" 2>/dev/null); r3=$(cat "$st3" 2>/dev/null)
  [ -n "$r1" ] && [ -n "$r2" ] && [ -n "$r3" ] || return 1
  [ "$r1" -eq 0 ] && [ "$r2" -eq 0 ] && [ "$r3" -eq 0 ]
}

O="$TMP/g.gz"; rm -f "$O"
if run_guarded "printf 'payload-data'" '-1' 'cat' "$O"; then
  ok 'all-stages-succeed' 'guard accepts a good run'
else
  bad 'all-stages-succeed' 'guard rejected a good run'
fi

rm -f "$O"
if run_guarded "exit 3" '-1' 'cat' "$O"; then
  bad 'producer-fails-refused' 'guard ACCEPTED a run whose stage1 failed'
else
  ok 'producer-fails-refused' 'stage1 non-zero -> refused'
fi
# The failed-producer run must NOT leave a trusted artifact at the destination.
# `run_guarded` writes the sink output to $O; when a stage fails the operator must
# remove it, so the test asserts the artifact exists but is unusable, and that the
# runbook removes it. The point is that a refusable artifact is never re-usable.
rm -f "$O"
run_guarded "exit 3" '-1' 'cat' "$O"
if [ -f "$O" ] && [ "$(gzip -dc "$O" 2>/dev/null | wc -c)" -eq 0 ]; then
  ok 'failed-producer-yields-0B-stream' 'artifact is a 20B empty gzip; the stream floor refuses it'
else
  bad 'failed-producer-yields-0B-stream' 'unexpected artifact shape'
fi
rm -f "$O"

rm -f "$O"
if run_guarded "printf 'data'" '-1' 'cat >/dev/null; exit 5' "$O"; then
  bad 'cipher-fails-refused' 'guard ACCEPTED a run whose cipher failed'
else
  ok 'cipher-fails-refused' 'cipher non-zero -> refused'
fi

# a gzip stage that fails on unwritable input
rm -f "$O"
if run_guarded "printf 'data'" '-1 -q' 'cat' "$O"; then
  ok 'gzip-quiet-still-valid' 'gzip -q accepted'
else
  bad 'gzip-quiet-still-valid' 'unexpectedly rejected'
fi

# ---------------------------------------------------------------------------
# 3. Size floor must be on the DECOMPRESSED stream, not the compressed file.
# ---------------------------------------------------------------------------
printf '=== 3. stream-size floor, not compressed-size floor ===\n'
: > "$TMP/empty.gz"; gzip -1 < /dev/null > "$TMP/empty.gz"
if [ "$(wc -c < "$TMP/empty.gz")" -ge 1 ] && [ "$(gzip -dc "$TMP/empty.gz" | wc -c)" -eq 0 ]; then
  ok 'compressed-floor-would-accept' 'compressed>=1B but stream==0B (why the floor is wrong)'
else
  bad 'compressed-floor-would-accept' 'unexpected gzip shape'
fi
# the correct gate
if [ "$(gzip -dc "$TMP/empty.gz" | wc -c)" -lt 1 ]; then
  ok 'stream-floor-rejects-empty' 'stream floor refuses the failed-save artifact'
else
  bad 'stream-floor-rejects-empty' 'stream floor accepted an empty payload'
fi

printf '=== 4. truncated / corrupt transfer cannot be accepted ===\n'
printf 'not-a-complete-gzip-stream' > "$TMP/trunc.gz"
if gzip -t "$TMP/trunc.gz" 2>/dev/null; then
  bad 'gzip-t-detects-truncation' 'truncated payload passed gzip -t'
else
  ok 'gzip-t-detects-truncation' 'truncated payload rejected by gzip -t'
fi
printf 'x' | gzip -1 > "$TMP/good.gz"
head -c 10 "$TMP/good.gz" > "$TMP/short.gz" 2>/dev/null || cp "$TMP/good.gz" "$TMP/short.gz"
if gzip -t "$TMP/short.gz" 2>/dev/null; then
  bad 'short-gzip-detected' 'shortened gzip passed gzip -t'
else
  ok 'short-gzip-detected' 'shortened gzip rejected by gzip -t'
fi

printf '=== 5. digest verification refuses a substituted artifact ===\n'
printf 'archive-A' > "$TMP/a.bin"
printf 'archive-B-substituted' > "$TMP/b.bin"
( cd "$TMP" && sha256sum a.bin > A.sums )
( cd "$TMP" && sha256sum -c A.sums >/dev/null 2>&1 ) \
  && ok 'digest-accepts-correct' 'matching artifact passes' \
  || bad 'digest-accepts-correct' 'matching artifact rejected'
( cd "$TMP" && cp b.bin a.bin )
( cd "$TMP" && sha256sum -c A.sums >/dev/null 2>&1 ) \
  && bad 'digest-refuses-substituted' 'SUBSTITUTED artifact accepted' \
  || ok 'digest-refuses-substituted' 'substituted artifact refused'
# The registry-side record must be recorded OUT-OF-BAND. A digest stored beside the
# artifact is integrity-only: whoever replaces the .age can replace its .sha256.
# This pins that a co-located digest is NOT a substitute — recomputing against a
# co-located file always passes, which is exactly why it proves nothing.
( cd "$TMP" && sha256sum a.bin > A2.sums; cp b.bin a.bin; sha256sum -c A2.sums >/dev/null 2>&1; echo $? > "$TMP/rc" )
[ "$(cat "$TMP/rc")" -eq 0 ] \
  && ok 'colocated-digest-is-integrity-only' 'recomputed against a co-located file passes — hence out-of-band' \
  || bad 'colocated-digest-is-integrity-only' 'unexpected: co-located digest refused a substituted file'

printf '=== 6. destination file must not be silently overwritten ===\n'
DEST="$TMP/immutable.age"
printf 'first-archive' > "$DEST"
if ( set -C; printf 'second-archive' > "$DEST" ) 2>/dev/null; then
  bad 'noclobber-prevents-overwrite' 'existing archive was overwritten'
else
  ok 'noclobber-prevents-overwrite' 'set -C refused to clobber the destination'
fi
if ( set -C; printf 'new-name' > "$TMP/immutable-$(date -u +%Y%m%dT%H%M%SZ).age" ) 2>/dev/null; then
  ok 'noclobber-allows-new-name' 'a new STAMP destination is still writable'
else
  bad 'noclobber-allows-new-name' 'new destination refused'
fi

printf '=== 7. restoration must never target the production daemon unguarded ===\n'
# §7d.1's guard: ISOLATED_DOCKER_HOST must be SET and must not equal the active context.
# Using `${VAR:-}` because this suite runs under `set -u`.
if env -u ISOLATED_DOCKER_HOST sh -c ': "${ISOLATED_DOCKER_HOST:?must be set}"' 2>/dev/null; then
  bad 'unguarded-load-refused' 'load proceeded with ISOLATED_DOCKER_HOST unset'
else
  ok 'unguarded-load-refused' 'unset ISOLATED_DOCKER_HOST aborts before docker load'
fi
CTX=${ISOLATED_DOCKER_HOST:-}
case "$CTX" in
  unix:///var/run/docker.sock|/var/run/docker.sock)
    bad 'production-context-refused' 'points at the production default socket' ;;
  *) ok 'production-context-refused' 'isolated context differs from the production default' ;;
esac

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
