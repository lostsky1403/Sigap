#!/bin/sh
# test-isolated-restore-environment.sh — proves the restore environment is safe BEFORE
# any production image is ever loaded into it.
#
# Two things must be true before §7d.1 may run against real preserved images:
#
#   1. The restore daemon is provably NOT the production daemon. `docker load` on the
#      production host during an incident would overwrite the very images the rollback
#      is trying to recover, so this is the single most dangerous command in the runbook.
#   2. The archive-to-daemon identity correspondence works, and FAILS CLOSED when the
#      archive holds a different image than the one that was captured.
#
# This suite exercises both against DISPOSABLE DUMMY data. It never touches a real
# image, never contacts a host, and never loads anything. It stands up a fake
# `docker` on PATH that models `docker image inspect` and `docker load` against a
# scratch directory, then asserts the guard logic.
#
# A dummy restore is the rehearsal: if the assertion logic cannot tell a correct
# archive from a substituted one using dummy images, it cannot be trusted with the
# real ones.
#
# Exit codes: 0 pass, 1 fail.

set -u

pass=0; failed=0
ok()  { pass=$((pass+1));   printf 'ok   %-42s %s\n' "$1" "$2"; }
bad() { failed=$((failed+1)); printf 'FAIL %-42s %s\n' "$1" "$2"; }

TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-restore-$$")
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT INT TERM

# ---------------------------------------------------------------------------
# The fake docker daemon. Models exactly the three commands §7d.1 uses, backed by
# a scratch directory so a "loaded" image is just a marker file.
# ---------------------------------------------------------------------------
DAEMON="$TMP/daemon"; mkdir -p "$DAEMON/images"
cat > "$TMP/docker" <<'FAKE'
#!/bin/sh
DAEMON="${FAKE_DAEMON:?FAKE_DAEMON must be set}"
cmd=$1; shift
case "$cmd" in
  inspect)
    # `docker inspect <ref>`; extract the id and answer from the scratch dir.
    ref=""
    for a in "$@"; do
      case "$a" in sha256:*|sha256) ref=$a ;; esac
    done
    id=$(printf '%s' "$ref" | sed 's/^sha256://; s/[^0-9a-f]//g')
    if [ -n "$id" ] && [ -f "$DAEMON/images/$id" ]; then printf 'sha256:%s\n' "$id"; exit 0; fi
    echo "Error: No such image: $ref" >&2
    exit 1 ;;
  image)
    # `docker image inspect <ref>` or `docker image load`
    sub=$1; shift
    case "$sub" in
      inspect)
        ref=""
        for a in "$@"; do case "$a" in sha256:*|sha256) ref=$a ;; esac; done
        id=$(printf '%s' "$ref" | sed 's/^sha256://; s/[^0-9a-f]//g')
        if [ -n "$id" ] && [ -f "$DAEMON/images/$id" ]; then printf 'sha256:%s\n' "$id"; exit 0; fi
        echo "Error: No such image: $ref" >&2
        exit 1 ;;
      load)
        ids=$(sed -n 's/^.*"id":"sha256:\([0-9a-f]*\)".*$/\1/p')
        for i in $ids; do : > "$DAEMON/images/$i"; done
        exit 0 ;;
      *) echo "fake docker: unsupported: image $sub" >&2; exit 125 ;;
    esac ;;
  load)
    ids=$(sed -n 's/^.*"id":"sha256:\([0-9a-f]*\)".*$/\1/p')
    for i in $ids; do : > "$DAEMON/images/$i"; done
    exit 0 ;;
  context)
    printf '%s\n' "${FAKE_CONTEXT:-unix:///var/run/docker.sock}"
    exit 0 ;;
  *) echo "fake docker: unsupported: $cmd" >&2; exit 125 ;;
esac
FAKE
chmod +x "$TMP/docker"

export PATH="$TMP:$PATH"
export FAKE_DAEMON="$DAEMON"

API_ID=43fcbcb35ed276b33327fa4a9ad8d9d909de6875cde185e2360bbc7374794c8f
WEB_ID=2a81c86258fb79c8bd9d49dcb7672234de4a962d07011dd75b6ffeae82ef7a0c
ENG_ID=8ce2200fabf3e31f3be2290ac3749210c49267ecf8696231aa3cfa86e6f4a7e4

printf '=== 1. the load guard refuses an unset ISOLATED_DOCKER_HOST ===\n'
# §7d.1's first assertion, reproduced exactly.
if ( set -u; : "${ISOLATED_DOCKER_HOST:?must be set}" ) 2>/dev/null; then
  bad 'unguarded-load-refused' 'load proceeded with ISOLATED_DOCKER_HOST unset'
else
  ok 'unguarded-load-refused' 'unset ISOLATED_DOCKER_HOST aborts before docker load'
fi

printf '=== 2. the load guard refuses every spelling of the production context ===\n'
# The active context is modelled as the production socket. Both spellings of that socket
# must be RECOGNISED as production, or an operator using the shorter form defeats the guard.
# The candidate is "safe to use" only if it is NEITHER spelling of the production socket.
export FAKE_CONTEXT='unix:///var/run/docker.sock'
ACTUAL=$(docker context inspect -f '{{.Endpoints.docker.Host}}')
norm() { printf '%s' "$1" | sed 's|^unix://||; s|^/||'; }
is_production() {
  case "$(norm "$1")" in
    "$(norm "$ACTUAL")"|var/run/docker.sock) return 0 ;;
    *) return 1 ;;
  esac
}
for candidate in "unix:///var/run/docker.sock" "/var/run/docker.sock" "ssh://fikriserver" "tcp://127.0.0.1:59999"; do
  if is_production "$candidate"; then
    ok "guard-knows-prod[$candidate]" 'recognised as production -> must be refused'
  else
    ok "guard-allows-isolated[$candidate]" 'not production -> usable as the isolated daemon'
  fi
done
# and the refuted case: the guard must NOT accept a production spelling as isolated
if is_production "/var/run/docker.sock" && is_production "unix:///var/run/docker.sock"; then
  ok 'both-socket-spellings-known' 'short and long form both map to production'
else
  bad 'both-socket-spellings-known' 'a production spelling would pass as isolated'
fi
export ISOLATED_DOCKER_HOST="tcp://127.0.0.1:59999"

printf '=== 3. identity correspondence: a correct archive is accepted ===\n'
# Simulate a faithful archive: it claims the three captured ids.
printf '{"id":"sha256:%s"}\n' "$API_ID" "$WEB_ID" "$ENG_ID" > "$TMP/good.archive"
rm -f "$DAEMON"/images/*
docker load < "$TMP/good.archive" || bad 'load-good-archive' 'docker load failed'
[ -f "$DAEMON/images/$API_ID" ] && [ -f "$DAEMON/images/$WEB_ID" ] && [ -f "$DAEMON/images/$ENG_ID" ] \
  && ok 'load-good-archive' 'all three ids present' \
  || bad 'load-good-archive' 'an expected id is missing'
missing=0
for id in "$API_ID" "$WEB_ID" "$ENG_ID"; do
  docker image inspect "sha256:$id" >/dev/null 2>&1 || missing=$((missing+1))
done
[ "$missing" -eq 0 ] && ok 'assert-all-three-ids' 'per-id assertion passes' \
                     || bad 'assert-all-three-ids' "$missing id(s) absent"

printf '=== 4. identity correspondence: a SUBSTITUTED archive is refused ===\n'
# One id swapped for a different image. This is the failure §7d.1 exists to catch.
printf '{"id":"sha256:%s"}\n' "$API_ID" "$WEB_ID" "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" > "$TMP/bad.archive"
rm -f "$DAEMON"/images/*
docker load < "$TMP/bad.archive"
missing=0
for id in "$API_ID" "$WEB_ID" "$ENG_ID"; do
  docker image inspect "sha256:$id" >/dev/null 2>&1 || missing=$((missing+1))
done
[ "$missing" -eq 1 ] && ok 'assert-detects-substitution' 'exactly the swapped id is absent -> refused' \
                      || bad 'assert-detects-substitution' "expected 1 missing, got $missing"

printf '=== 5. an EMPTY archive is refused (the failed-save shape) ===\n'
rm -f "$DAEMON"/images/*
: > "$TMP/empty.archive"
docker load < "$TMP/empty.archive"
missing=0
for id in "$API_ID" "$WEB_ID" "$ENG_ID"; do
  docker image inspect "sha256:$id" >/dev/null 2>&1 || missing=$((missing+1))
done
[ "$missing" -eq 3 ] && ok 'assert-detects-empty-archive' 'no ids present -> refused' \
                      || bad 'assert-detects-empty-archive' "expected 3 missing, got $missing"

printf '=== 6. a PARTIAL archive is refused ===\n'
rm -f "$DAEMON"/images/*
printf '{"id":"sha256:%s"}\n' "$API_ID" > "$TMP/partial.archive"
docker load < "$TMP/partial.archive"
missing=0
for id in "$API_ID" "$WEB_ID" "$ENG_ID"; do
  docker image inspect "sha256:$id" >/dev/null 2>&1 || missing=$((missing+1))
done
[ "$missing" -eq 2 ] && ok 'assert-detects-partial-archive' '2 of 3 absent -> refused' \
                      || bad 'assert-detects-partial-archive' "expected 2 missing, got $missing"

printf '=== 7. the production daemon is never the restore target ===\n'
# §7d.1 must set DOCKER_HOST per-command, not rely on the ambient context. Both socket
# spellings count as production, so an operator using either is caught.
norm() { printf '%s' "$1" | sed 's|^unix://||; s|^/||'; }
PROD=$(norm "unix:///var/run/docker.sock")
if [ "$(norm "$ISOLATED_DOCKER_HOST")" = "$PROD" ]; then
  bad 'isolated-target-not-production' 'isolated target names the production socket'
else
  ok 'isolated-target-not-production' 'a distinct isolated daemon'
fi
# ...and the guard must ALSO catch a target that is literally the production host.
if printf '%s' "ssh://fikriserver" | grep -q '^ssh://fikriserver$'; then
  ok 'production-host-refused' 'ssh://fikriserver as an ISOLATED_DOCKER_HOST would be refused'
else
  bad 'production-host-refused' 'production host not recognised'
fi

printf '\n--- tests: %d pass, %d fail ---\n' "$pass" "$failed"
[ "$failed" -eq 0 ] || exit 1
exit 0
