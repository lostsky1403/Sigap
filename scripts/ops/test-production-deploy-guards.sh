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
#    NOTE: implemented in pure shell (no external `sort`) — `sort` is a shadowed builtin
#    in some restricted shells, and `sort -V` is not POSIX. The comparison itself is
#    self-tested below so it cannot silently pass everything.
_norm_ver() (
  set -f
  IFS=.
  set -- $1
  IFS=' '
  printf '%d%02d%02d' "${1:-0}" "${2:-0}" "${3:-0}"
)
ver_ge() { [ "$(_norm_ver "${1%%-*}")" -ge "$(_norm_ver "$2")" ]; }

# 6a. The comparator itself must discriminate (otherwise test 6 is vacuous).
if ver_ge 2.24.4 2.24.4 && ver_ge 2.25.0 2.24.4 && ! ver_ge 2.24.3 2.24.4 && ! ver_ge 1.29.2 2.24.4; then
  ok
else
  bad "version comparator is broken (it must accept >=floor and reject <floor)"
fi

have=$(docker compose version --short 2>/dev/null || echo "?")
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
if grep -q "CURRENT_PRODUCTION_IMAGE_ID" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "PRESERVED_ROLLBACK_IMAGE_REF" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "SIGAP_DEPLOY_DIR" docs/operations/DEPLOYMENT_RUNBOOK.md \
   && grep -q "OPERATOR_EVIDENCE_BUNDLE" docs/operations/DEPLOYMENT_RUNBOOK.md; then
  ok
else
  bad "rollback-target / deploy-dir / evidence-bundle contract missing from the runbook"
fi

# ---------------------------------------------------------------------------
# 11-16: negative controls for the NEW blocking gates.
# ---------------------------------------------------------------------------
RB=docs/operations/DEPLOYMENT_RUNBOOK.md
CL=docs/operations/RELEASE_CHECKLIST.md

# 11. A NON-EMPTY historical migration diff must NOT be treated as a blocker.
#     Negative control: the old runbook claimed "migration diff empty" for 6d7f940..HEAD
#     (it is NOT empty). Assert (a) the diff really is non-empty, and (b) the runbook
#     documents it as expected/classified rather than as an abort condition.
hist=$(git diff --stat 6d7f940..HEAD -- packages/db/migrations 2>/dev/null)
maindiff=$(git diff --stat origin/main..HEAD -- packages/db/migrations 2>/dev/null)
if [ -n "$maindiff" ]; then
  bad "origin/main..HEAD migration diff is NOT empty (release source diverges from main)"
elif [ -z "$hist" ]; then
  bad "6d7f940..HEAD migration diff is empty — expected the inherited 0006 hardening"
elif grep -q "6d7f940..HEAD" "$RB" && grep -qi "non-empty by design" "$RB"; then
  ok
else
  bad "runbook does not document the non-empty 6d7f940..HEAD diff as expected/classified"
fi

# 12. An UNKNOWN DB schema can never be a PASS.
#     Assert the inspection package exists, that the classification enum is documented,
#     and that UNKNOWN is explicitly marked blocking / never a PASS.
if [ ! -f scripts/ops/db-metadata-inspection.sql ]; then
  bad "scripts/ops/db-metadata-inspection.sql is missing"
elif ! grep -q "BEGIN READ ONLY" scripts/ops/db-metadata-inspection.sql; then
  bad "db-metadata-inspection.sql is not wrapped in a read-only transaction"
elif grep -q "UNKNOWN" "$RB" \
  && grep -qi "never a PASS" "$RB" \
  && grep -q "DATABASE_SCHEMA_COMPATIBILITY = UNKNOWN" "$RB"; then
  ok
else
  bad "runbook does not establish that UNKNOWN DB schema is blocking and never a PASS"
fi

# 12b. The inspection must be SELECT-only: no DDL/DML keywords anywhere in the file.
if grep -Eiq '^[[:space:]]*(INSERT|UPDATE|DELETE|ALTER|CREATE|DROP|TRUNCATE|GRANT|COPY)[[:space:]]' scripts/ops/db-metadata-inspection.sql; then
  bad "db-metadata-inspection.sql contains a mutating statement"
else
  ok
fi

# 13. A MISSING rollback archive must fail CLOSED.
#     The documented restore step must abort non-zero when the checksum check fails.
if grep -q "sha256sum -c" "$RB" && grep -q "ABORT: archive checksum failed" "$RB"; then
  ok
else
  bad "rollback restore does not fail closed on a failed archive checksum"
fi

# 14. An UNKNOWN current image ID must fail CLOSED (abort before build/up).
#     Scoped to the §7a rollback-target table: the `CURRENT_PRODUCTION_IMAGE_ID` row
#     itself must list "unknown" as a reject value. A whole-file grep would be vacuous
#     (the phrase also appears in §2.9 and §11).
sec7a=$(sed -n '/^### 7a\./,/^### 7b\./p' "$RB")
if printf '%s' "$sec7a" | grep -q "CURRENT_PRODUCTION_IMAGE_ID.*unknown" \
  && printf '%s' "$sec7a" | grep -q "ABORT before \`build\`/\`up\`"; then
  ok
else
  bad "an unknown current image ID does not fail closed in the §7a rollback contract"
fi

# 15. An UNSAFE .env staging must be DETECTABLE.
#     Functional: the ignore rules must match the sensitive backup and must NOT match
#     the tracked template. (A blanket `*.env*` rule would break the second assertion.)
if git check-ignore -q .env.bak-phase5; then
  ok
else
  bad ".env.bak-phase5 is NOT ignored — it could be staged into a public remote"
fi
if git check-ignore -q .env.example; then
  bad ".env.example is ignored — the tracked template would become untrackable"
else
  ok
fi

# 16. A MISSING edge overlay must be DETECTED in the ROLLBACK path.
#     The rollback COMPOSE definition must name all three files (a subset detaches web
#     from Traefik), and the restore must run through it with --no-deps.
sec7c=$(sed -n '/^### 7c\./,/^### 7d\./p' "$RB")
sec7d=$(sed -n '/^### 7d\./,/^### 7e\./p' "$RB")
c7c=$(printf '%s\n' "$sec7c" | grep -m1 '^COMPOSE=')
c7d=$(printf '%s\n' "$sec7d" | grep -m1 '^COMPOSE=')
if [ -z "$c7c" ] || [ -z "$c7d" ]; then
  bad "the rollback path does not define a COMPOSE overlay set"
elif printf '%s' "$c7c" | grep -q 'docker-compose.yml.*prod-ports.yml.*prod-edge.yml' \
  && printf '%s' "$c7d" | grep -q 'docker-compose.yml.*prod-ports.yml.*prod-edge.yml'; then
  if printf '%s' "$sec7d" | grep -q '\$COMPOSE up -d --no-deps rust-engine api web'; then
    ok
  else
    bad "the rollback path does not document a --no-deps application-only restore"
  fi
else
  bad "a rollback COMPOSE definition omits the prod-edge overlay (web would leave Traefik)"
fi

# 17. §7d must be SELF-CONTAINED: it runs in a fresh shell days after §7c, so it must
#     define STAMP and ARCHIVE_DIR (or abort), not silently inherit them.
if printf '%s' "$sec7d" | grep -q 'STAMP:?' && printf '%s' "$sec7d" | grep -q 'ARCHIVE_DIR:?'; then
  ok
else
  bad "§7d consumes STAMP/ARCHIVE_DIR without defining or requiring them (not executable standalone)"
fi

# 18. §13a must NOT advise pruning before preservation is verified (§13b forbids it).
#     After a build retags :latest, the orphaned running release IS a dangling image.
sec13a=$(sed -n '/^### 13a\./,/^### 13b\./p' "$RB")
if printf '%s' "$sec13a" | grep -qi 'docker image prune' \
  && ! printf '%s' "$sec13a" | grep -qi 'only after\|permitted .*only after\|do not run'; then
  bad "§13a advises pruning without the §13b ordering constraint (can destroy the rollback image)"
else
  ok
fi

# 19. The §6b anchor must not claim 6d7f940 is NOT an ancestor of origin/main (it is).
if grep -q 'not an ancestor of' "$RB"; then
  bad "runbook still claims a commit is 'not an ancestor' (verify with git merge-base --is-ancestor)"
else
  ok
fi

printf 'deploy-guard tests: %s pass, %s fail\n' "$pass" "$failed"
exit "$([ "$failed" -eq 0 ] && echo 0 || echo 1)"
