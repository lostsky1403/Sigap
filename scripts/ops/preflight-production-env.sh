#!/bin/sh
# preflight-production-env.sh — fail-closed environment gate for production deploys.
# Read-only: inspects the operator's shell environment, never touches the host.
# Usage: sh scripts/ops/preflight-production-env.sh
# Exit 0 only when every production-inappropriate value is absent.

fail=0
say() { printf '%s\n' "$1"; }

check() { # name, actual, forbidden...
  name=$1; actual=$2; shift 2
  for bad in "$@"; do
    if [ "$actual" = "$bad" ]; then
      say "FAIL: $name is '$actual' — refused for production"
      fail=1
      return
    fi
  done
  say "PASS: $name is ${actual:-<unset>} (accepted: non-local)"
}

# --- SIGAP_ENV: must be set, non-empty, and not any case-variant of local ---
env_val=${SIGAP_ENV:-}
if [ -z "$env_val" ]; then
  say "FAIL: SIGAP_ENV is unset/empty — set it explicitly (staging/production)"
  fail=1
else
  # Trim whitespace then lowercase: the Go guard does TrimSpace + EqualFold
  # (envguard.go), so " local " IS local to the API. Without the trim this gate
  # would pass a value that silently disables the API's TLS guard.
  lowered=$(printf '%s' "$env_val" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
  check "SIGAP_ENV" "$lowered" "local"
fi

# --- SIGAP_DEPLOY_DIR: the deployment directory contract (runbook §7a) ---
deploy_dir=${SIGAP_DEPLOY_DIR:-}
if [ -z "$deploy_dir" ]; then
  say "FAIL: SIGAP_DEPLOY_DIR is unset — the deploy/rollback directory must be recorded before any build or up"
  fail=1
elif [ ! -d "$deploy_dir" ]; then
  say "FAIL: SIGAP_DEPLOY_DIR='$deploy_dir' is not a directory"
  fail=1
elif [ ! -f "$deploy_dir/docker-compose.yml" ]; then
  say "FAIL: SIGAP_DEPLOY_DIR='$deploy_dir' has no docker-compose.yml (not the compose repo)"
  fail=1
else
  say "PASS: SIGAP_DEPLOY_DIR resolves to a compose directory"
fi

# --- SIGAP_AUTH_MODE: canonical modes come from apps/api/internal/auth/config.go ---
mode=${SIGAP_AUTH_MODE:-}
case "$mode" in
  jwt)
    say "PASS: SIGAP_AUTH_MODE is 'jwt' (expected production mode)"
    ;;
  "" )
    say "FAIL: SIGAP_AUTH_MODE is unset — production requires jwt (plus SIGAP_AUTH_ISSUER/SIGAP_AUTH_AUDIENCE)"
    fail=1
    ;;
  dev)
    say "FAIL: SIGAP_AUTH_MODE is 'dev' — dev identity is rejected outside SIGAP_ENV=local"
    fail=1
    ;;
  disabled)
    say "FAIL: SIGAP_AUTH_MODE is 'disabled' — every admin route would 403; production requires jwt"
    fail=1
    ;;
  *)
    say "FAIL: SIGAP_AUTH_MODE is '$mode' — must be one of dev, jwt, disabled (production: jwt)"
    fail=1
    ;;
esac

# --- Dev-only flags must be absent/false; compose never forwards them ---
for v in SIGAP_DEV_IDENTITY SIGAP_LOCAL_RBAC_TEST_IDENTITY SIGAP_LOCAL_E2E_ACTOR SIGAP_ENGINE_FALLBACK; do
  val=$(eval "printf '%s' \"\${$v:-}\"")
  case "$val" in
    "" | 0 | false | False | FALSE | no | No | NO)
      say "PASS: $v is ${val:-<unset>} (safe)"
      ;;
    *)
      say "FAIL: $v is '$val' — dev-only capability, refused outside SIGAP_ENV=local"
      fail=1
      ;;
  esac
done

if [ "$fail" -eq 0 ]; then say "PREFLIGHT ENV: PASS"; else say "PREFLIGHT ENV: FAIL"; fi
exit "$fail"
