#!/bin/sh
# test-production-deploy-guards.sh — negative controls for the production deploy guards.
# Verifies the fail-closed mechanisms WITHOUT touching any host or container:
# only `docker compose config` (pure interpolation) and the preflight script are run.
# Exit 0 iff every guard fails in the bad case and passes in the good case.
pass=0; failed=0
ok()   { pass=$((pass + 1)); }
bad()  { printf 'GUARD FAIL: %s\n' "$1"; failed=$((failed + 1)); }

CFG="docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml config"
EDGE_CFG="docker compose -f docker-compose.yml -f docker-compose.prod-edge.yml config"
PORTS_CFG="docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml config"
BASE_CREDS="POSTGRES_PASSWORD=x SIGAP_AUTH_MODE=jwt SIGAP_AUTH_ISSUER=i SIGAP_AUTH_AUDIENCE=a"
export SIGAP_DEPLOY_DIR=.   # preflight script requires the directory contract

# 1. Missing SIGAP_ENV through the production overlay set must refuse (before any container starts).
if env -u SIGAP_ENV $BASE_CREDS $CFG >/dev/null 2>&1; then bad "missing SIGAP_ENV was accepted by the prod overlay set"; else ok; fi

# 2. Empty SIGAP_ENV must refuse.
if env SIGAP_ENV= $BASE_CREDS $CFG >/dev/null 2>&1; then bad "empty SIGAP_ENV was accepted by the prod overlay set"; else ok; fi

# 2b. A PARTIAL overlay set (base + prod-edge, omitting prod-ports) must ALSO refuse.
#     Otherwise that subset inherits the base file's SIGAP_ENV=local default and
#     leaves the base public port mappings in force.
if env -u SIGAP_ENV $BASE_CREDS $EDGE_CFG >/dev/null 2>&1; then bad "missing SIGAP_ENV was accepted by base+prod-edge"; else ok; fi

# 3. SIGAP_ENV=staging resolves through the whole overlay set (incl. !override files).
if env SIGAP_ENV=staging $BASE_CREDS $CFG >/dev/null 2>&1; then ok; else bad "SIGAP_ENV=staging was rejected by the prod overlay set"; fi

# 4. SIGAP_ENV=local resolves (overlay is value-agnostic) but the preflight script rejects it.
env SIGAP_ENV=local SIGAP_AUTH_MODE=jwt sh scripts/ops/preflight-production-env.sh >/dev/null 2>&1
if [ $? -eq 0 ]; then bad "SIGAP_ENV=local passed preflight-production-env.sh"; else ok; fi

# 5. Production env gate rejects every non-jwt auth mode.
for m in dev disabled weird ""; do
  if [ -z "$m" ]; then
    env -u SIGAP_AUTH_MODE SIGAP_ENV=staging sh scripts/ops/preflight-production-env.sh >/dev/null 2>&1
  else
    env SIGAP_ENV=staging SIGAP_AUTH_MODE="$m" sh scripts/ops/preflight-production-env.sh >/dev/null 2>&1
  fi
  if [ $? -eq 0 ]; then bad "SIGAP_AUTH_MODE='${m:-<unset>}' passed preflight-production-env.sh"; else ok; fi
done

# 6. Unsupported compose is detectable: the runbook floor is 2.24.4 (!override tag).
#    Only the version can be checked repo-locally; the operator runs the same command.
have=$(docker compose version --short 2>/dev/null || echo "?")
ver_ge() { # ver_ge have floor -> 0 iff have >= floor (suffixes like -desktop.1 ignored)
  clean=${1%%-*}
  [ "$clean" = "$(printf '%s\n%s' "$clean" "$2" | sort -V | tail -1)" ]
}
if [ "$have" = "?" ]; then
  printf 'GUARD SKIP: docker compose unavailable locally (operator checks on host)\n'
else
  if ver_ge "$have" "2.24.4"; then ok; else bad "local compose $have < 2.24.4"; fi
fi
# 7. The overlays all parse together (no YAML/!override drift) with a valid env.
if env SIGAP_ENV=staging $BASE_CREDS $CFG >/dev/null 2>&1; then ok; else bad "prod overlay set does not parse"; fi

# 7b. Edge binding is REAL, not vacuous: web must be loopback-only AND on the edge
#     network WITH the overlay, and NOT on the edge network WITHOUT it. (An unscoped
#     grep matched prod-ports' own host_ip lines and could never fail.)
if command -v python3 >/dev/null 2>&1; then
  wprobe='import json,sys;w=json.load(sys.stdin)["services"]["web"];print(sorted({p.get("host_ip") for p in w.get("ports",[])}),sorted(w.get("networks",{})))'
  with=$(env SIGAP_ENV=staging $BASE_CREDS $CFG --format json 2>/dev/null | python3 -c "$wprobe" 2>/dev/null)
  without=$(env SIGAP_ENV=staging $BASE_CREDS $PORTS_CFG --format json 2>/dev/null | python3 -c "$wprobe" 2>/dev/null)
  case "$with" in
    "['127.0.0.1']"*"edge"*) ok ;;
    *) bad "web is not loopback+edge-bound with the overlay: $with" ;;
  esac
  case "$without" in
    *"'edge'"*) bad "web is on the edge network WITHOUT the edge overlay: $without" ;;
    *) ok ;;
  esac
else
  printf 'GUARD SKIP: python3 unavailable (edge-binding check skipped)\n'
fi

# 8. SIGAP_AUTH_MODE remains required-without-default in the API service (no silent fallback).
if grep -q 'SIGAP_AUTH_MODE: ${SIGAP_AUTH_MODE:?' docker-compose.yml; then ok; else bad "SIGAP_AUTH_MODE lost its :? guard in docker-compose.yml"; fi

# 9. The dev-only flags are NOT forwarded by any compose file (safety rests on absence).
if grep -E '^\s*SIGAP_(DEV_IDENTITY|LOCAL_RBAC_TEST_IDENTITY|LOCAL_E2E_ACTOR|ENGINE_FALLBACK):' docker-compose.yml docker-compose.prod-ports.yml docker-compose.prod-edge.yml 2>/dev/null; then
  bad "a dev-only flag is forwarded by a compose file"
else
  ok
fi

# 10. Rollback target contract is present in the runbook (blocked-on-external evidence).
if grep -q "CURRENT_PRODUCTION_VERSION" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "CURRENT_IMAGE_DIGEST" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "SIGAP_DEPLOY_DIR" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "OPERATOR_EVIDENCE_BUNDLE" docs/operations/DEPLOYMENT_RUNBOOK.md; then
  ok
else
  bad "rollback-target / deploy-dir / evidence-bundle contract missing from the runbook"
fi

printf 'deploy-guard tests: %s pass, %s fail\n' "$pass" "$failed"
exit "$([ "$failed" -eq 0 ] && echo 0 || echo 1)"
