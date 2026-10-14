#!/bin/sh
# test-operator-evidence-bundle.sh — deterministic negative controls for the
# read-only environment-presence check used by
# docs/operations/OPERATOR_EVIDENCE_BUNDLE.md.
#
# WHY THIS EXISTS
#   The first revision of that bundle built ONE id string by concatenating two
#   container ids with no separator:
#       IDS=$(docker ps -q --filter "name=sigap-api")$(docker ps -q --filter "name=sigap-web")
#   Command substitution concatenates, so two 12-character ids became a single
#   24-character token. `docker inspect` then failed with "no such object", the
#   error was swallowed by 2>/dev/null, and EVERY variable was reported ABSENT —
#   including SIGAP_DEV_IDENTITY, the very flag the check exists to catch. A
#   false ABSENT is a false PASS, so this is a control failure, not cosmetics.
#
#   The shim below reproduces that failure mode locally and deterministically.
#   No Docker daemon, no host and no production access is involved.
#
# Exit 0 iff every control behaves correctly.

set -u
pass=0; failed=0
ok()  { pass=$((pass + 1)); }
bad() { printf 'BUNDLE FAIL: %s\n' "$1"; failed=$((failed + 1)); }

TMP=$(mktemp -d 2>/dev/null || mktemp -d -t sigapbundle)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/broken"

# --- fake docker -------------------------------------------------------------
# Models the three behaviours that matter here:
#   * `ps -q --filter name=X` is a SUBSTRING match (so it also returns decoys)
#   * `inspect <name>` matches the NAME exactly
#   * `inspect <unknown>` fails with a non-zero status and a diagnostic
cat > "$TMP/bin/docker" <<'SHIM'
#!/bin/sh
api_env() { printf '%s\n' \
  SIGAP_ENV=production SIGAP_AUTH_MODE=jwt SIGAP_TLS_TERMINATED=true \
  SIGAP_TRUSTED_PROXIES=2 SIGAP_DEV_IDENTITY=false \
  SIGAP_AUTH_JWKS_URL=FAKE_JWKS_URL_VALUE DATABASE_URL=FAKE_DB_URL_VALUE; }
web_env() { printf '%s\n' \
  SIGAP_ENV=production SIGAP_API_INTERNAL=http://api:8080 \
  PUBLIC_SUPABASE_URL=FAKE_SUPABASE_URL_VALUE \
  PUBLIC_SUPABASE_ANON_KEY=FAKE_SUPABASE_KEY_VALUE; }
old_env() { printf '%s\n' SIGAP_ENV=production SIGAP_DEV_IDENTITY=true; }

short_id() {
  case "$1" in
    sigap-api)     echo 40bf694b849d ;;
    sigap-web)     echo 4f316e3cdca7 ;;
    sigap-api-old) echo aaaaaaaaaaaa ;;
  esac
}

cmd=$1; shift
case "$cmd" in
  ps)
    filt=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --filter) filt=${2#name=}; shift 2 ;;
        *) shift ;;
      esac
    done
    for n in sigap-api sigap-web sigap-api-old; do
      case "$n" in *"$filt"*) short_id "$n" ;; esac
    done
    ;;
  inspect)
    name=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --format) shift 2 ;;
        *) name=$1; shift ;;
      esac
    done
    case "$name" in
      sigap-api)     api_env; exit 0 ;;
      sigap-web)     web_env; exit 0 ;;
      sigap-api-old) old_env; exit 0 ;;
      *) printf 'Error: No such object: %s\n' "$name" >&2; exit 1 ;;
    esac
    ;;
  *) exit 0 ;;
esac
SHIM
chmod +x "$TMP/bin/docker"

# A docker that always fails, to prove an inspect failure is never read as ABSENT.
cat > "$TMP/broken/docker" <<'SHIM'
#!/bin/sh
printf 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock.\n' >&2
exit 1
SHIM
chmod +x "$TMP/broken/docker"

SCRIPT=scripts/ops/evidence-env-presence.sh

# ============================================================================
# A. Reproduce the original defect.
# ============================================================================
PATH="$TMP/bin:$PATH" export PATH

IDS=$(docker ps -q --filter "name=sigap-api")$(docker ps -q --filter "name=sigap-web")
# The two command substitutions are jammed together with no separator, producing
# a single over-long token rather than two arguments.
if [ "${#IDS}" -ge 24 ]; then
    ok
else
    bad "expected the concatenated-id form to yield an over-long token, got ${#IDS} chars"
fi

# The concatenated token is not a container, so inspect fails and every variable
# silently reads as unset. This is the false-ABSENT the fix removes.
found=$(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' $IDS 2>/dev/null | grep -c '^SIGAP_DEV_IDENTITY=' || true)
if [ "$found" -eq 0 ]; then
    ok
else
    bad "the concatenated-id form unexpectedly resolved SIGAP_DEV_IDENTITY"
fi

# ============================================================================
# B. The corrected implementation resolves the same variable correctly.
# ============================================================================
out=$(sh "$SCRIPT" sigap-api 2>&1); rc=$?
case "$out" in
    *"sigap-api SIGAP_DEV_IDENTITY=PRESENT"*) ok ;;
    *) bad "corrected script did not report sigap-api SIGAP_DEV_IDENTITY=PRESENT" ;;
esac
if [ "$rc" -eq 0 ]; then ok; else bad "corrected script exited $rc on a present container"; fi

# ============================================================================
# C. A missing container is reported as MISSING, never as a variable being ABSENT.
# ============================================================================
out=$(sh "$SCRIPT" sigap-does-not-exist 2>&1); rc=$?
case "$out" in
    *"sigap-does-not-exist=CONTAINER_MISSING_OR_INSPECT_FAILED"*) ok ;;
    *) bad "missing container was not reported as MISSING" ;;
esac
case "$out" in
    *"=PRESENT"*) bad "a missing container produced a PRESENT result" ;;
    *) ok ;;
esac
case "$out" in
    *"is UNKNOWN (NOT absent)"*) ok ;;
    *) bad "missing container did not mark its variables UNKNOWN" ;;
esac
if [ "$rc" -ne 0 ]; then ok; else bad "missing container exited 0 (failure hidden)"; fi

# ============================================================================
# D. A Docker inspect FAILURE is never converted into ABSENT.
# ============================================================================
out=$(PATH="$TMP/broken:$PATH" sh "$SCRIPT" sigap-api 2>&1); rc=$?
case "$out" in
    *"sigap-api=CONTAINER_MISSING_OR_INSPECT_FAILED"*) ok ;;
    *) bad "inspect failure was not surfaced as MISSING_OR_INSPECT_FAILED" ;;
esac
case "$out" in
    *"Docker daemon"*) ok ;;
    *) bad "inspect failure reason was not reported" ;;
esac
if [ "$rc" -ne 0 ]; then ok; else bad "inspect failure exited 0 (failure hidden)"; fi

# ============================================================================
# E. No secret VALUE is ever printed — only PRESENT/ABSENT.
# ============================================================================
out=$(sh "$SCRIPT" sigap-api sigap-web 2>&1)
leak=0
for secret in FAKE_JWKS_URL_VALUE FAKE_DB_URL_VALUE FAKE_SUPABASE_URL_VALUE FAKE_SUPABASE_KEY_VALUE; do
    case "$out" in *"$secret"*) leak=1 ;; esac
done
if [ "$leak" -eq 0 ]; then ok; else bad "a secret value was printed by the presence check"; fi
case "$out" in
    *"sigap-web PUBLIC_SUPABASE_ANON_KEY=PRESENT"*) ok ;;
    *) bad "a present sensitive variable was not reported PRESENT" ;;
esac
case "$out" in
    *"sigap-api PUBLIC_SUPABASE_ANON_KEY=ABSENT"*) ok ;;
    *) bad "an absent sensitive variable was not reported ABSENT" ;;
esac

# ============================================================================
# F. Exact-name matching: the decoy `sigap-api-old` must not be selected.
# ============================================================================
# The substring filter DOES see the decoy (that is the hazard the fix avoids).
ids=$(docker ps -q --filter "name=sigap-api")
n=$(printf '%s\n' "$ids" | grep -c . || true)
if [ "$n" -eq 2 ]; then ok; else bad "expected the substring filter to match 2 ids, got $n"; fi

# The corrected script inspects by exact name, so it sees the safe value.
out=$(sh "$SCRIPT" sigap-api 2>&1)
case "$out" in
    *"sigap-api SIGAP_DEV_IDENTITY=PRESENT"*) ok ;;
    *) bad "exact-name inspect did not resolve sigap-api" ;;
esac
# And the decoy's dangerous value never appears for sigap-api.
if printf '%s\n' "$out" | grep -q 'SIGAP_DEV_IDENTITY=true'; then
    bad "the decoy container's SIGAP_DEV_IDENTITY=true leaked into sigap-api's result"
else
    ok
fi

printf 'evidence-bundle tests: %s pass, %s fail\n' "$pass" "$failed"
exit "$([ "$failed" -eq 0 ] && echo 0 || echo 1)"
