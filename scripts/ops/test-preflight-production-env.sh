#!/bin/sh
# test-preflight-production-env.sh — RED/GREEN guard for scripts/ops/preflight-production-env.sh.
# Zero dependencies beyond /bin/sh. Exit 0 iff every case behaves as specified.
S=scripts/ops/preflight-production-env.sh
pass=0; failed=0
ok()   { pass=$((pass + 1)); }
bad()  { printf 'CASE FAIL: %s (want exit %s, got %s)\n' "$1" "$2" "$3"; failed=$((failed + 1)); }
run() { # name, want_exit, VAR=val...
  name=$1; want=$2; shift 2
  # SIGAP_DEPLOY_DIR defaults to the repo root so a case fails for ITS reason,
  # not because the directory contract is unset.
  env -u SIGAP_ENV -u SIGAP_AUTH_MODE -u SIGAP_DEV_IDENTITY \
      -u SIGAP_LOCAL_RBAC_TEST_IDENTITY -u SIGAP_LOCAL_E2E_ACTOR -u SIGAP_ENGINE_FALLBACK \
      SIGAP_DEPLOY_DIR="${SIGAP_DEPLOY_DIR_TEST:-.}" \
      "$@" sh "$S" >/dev/null 2>&1
  got=$?
  if [ "$got" -eq "$want" ]; then ok; else bad "$name" "$want" "$got"; fi
}

run unset-env-fails        1
run empty-env-fails        1 "SIGAP_ENV="
run local-fails            1 "SIGAP_ENV=local"
run Local-case-fails       1 "SIGAP_ENV=Local"
run LOCAL-case-fails       1 "SIGAP_ENV=LOCAL"
run padded-local-fails     1 "SIGAP_ENV= local "
run padded-local2-fails    1 "SIGAP_ENV=local "
run missing-mode-fails     1 "SIGAP_ENV=staging"
run dev-mode-fails         1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=dev"
run disabled-mode-fails    1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=disabled"
run unknown-mode-fails     1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=weird"
run dev-flag-fails         1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt" "SIGAP_DEV_IDENTITY=true"
run rbac-flag-fails        1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt" "SIGAP_LOCAL_RBAC_TEST_IDENTITY=true"
run jwt-mode-passes         0 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt"
run production-mode-passes  0 "SIGAP_ENV=production" "SIGAP_AUTH_MODE=jwt"

# Deployment-directory contract (runbook §7a) - must be enforced, not just documented.
run missing-deploy-dir-fails 1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt" "SIGAP_DEPLOY_DIR="
run bad-deploy-dir-fails     1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt" "SIGAP_DEPLOY_DIR=/nonexistent-sigap-dir"
run non-repo-deploy-dir-fails 1 "SIGAP_ENV=staging" "SIGAP_AUTH_MODE=jwt" "SIGAP_DEPLOY_DIR=/tmp"

printf 'preflight-env tests: %s pass, %s fail\n' "$pass" "$failed"
exit "$([ "$failed" -eq 0 ] && echo 0 || echo 1)"
