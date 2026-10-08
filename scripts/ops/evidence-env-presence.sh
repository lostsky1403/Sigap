#!/bin/sh
# evidence-env-presence.sh — read-only, three-state environment presence check.
#
# Prints, per container, the non-secret configuration values that are safe to
# return verbatim, and PRESENT / ABSENT / UNKNOWN for every sensitive variable.
# A secret VALUE is never printed — only whether the variable is set.
#
# Usage:  sh scripts/ops/evidence-env-presence.sh [container ...]
#         (defaults to: sigap-api sigap-web)
#
# Exit codes:
#   0  every requested container was inspected
#   1  at least one container was MISSING or its inspect failed
#
# WHY THE EXIT CODE MATTERS. A missing container or a failed `docker inspect`
# is NOT the same as "the variable is unset". Returning 0 in that case would let
# a caller record ABSENT — and, for the dev-only flags, a false ABSENT is a
# false PASS on the exact control this check exists to enforce.
#
# WHY `docker inspect <name>` AND NOT `docker ps --filter name=<name>`.
# The name filter is a SUBSTRING match, so "name=sigap-api" can also select an
# unrelated container such as "sigap-api-old". `docker inspect <name>` matches
# the container NAME exactly.

set -u

CONTAINERS="${*:-sigap-api sigap-web}"

# Safe to print verbatim. Never widen this list without a security review.
NON_SECRET_RE='^SIGAP_ENV=|^SIGAP_AUTH_MODE=|^SIGAP_TLS_TERMINATED=|^SIGAP_TRUSTED_PROXIES='

# Presence only. Values are never emitted.
SENSITIVE='SIGAP_DEV_IDENTITY SIGAP_LOCAL_RBAC_TEST_IDENTITY SIGAP_LOCAL_E2E_ACTOR
SIGAP_ENGINE_FALLBACK PUBLIC_SUPABASE_URL PUBLIC_SUPABASE_ANON_KEY
SIGAP_API_INTERNAL SIGAP_AUTH_ISSUER SIGAP_AUTH_AUDIENCE SIGAP_AUTH_JWKS_URL
SIGAP_DATABASE_URL POSTGRES_PASSWORD'

rc=0

for c in $CONTAINERS; do
    # Capture BOTH streams: if inspect fails we want its reason, and we must not
    # let a failure masquerade as an empty environment.
    if ! dump=$(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$c" 2>&1); then
        printf '%s=CONTAINER_MISSING_OR_INSPECT_FAILED\n' "$c"
        printf '  reason: %s\n' "$(printf '%s' "$dump" | head -n 1)"
        printf '  => every variable for %s is UNKNOWN (NOT absent)\n' "$c"
        rc=1
        continue
    fi

    printf '%s=INSPECTED\n' "$c"

    # Non-secret values, prefixed with the container name.
    printf '%s\n' "$dump" | grep -E "$NON_SECRET_RE" | sed "s|^|$c |" || true

    for v in $SENSITIVE; do
        if printf '%s\n' "$dump" | grep -q "^${v}="; then
            printf '%s %s=PRESENT\n' "$c" "$v"
        else
            printf '%s %s=ABSENT\n' "$c" "$v"
        fi
    done
done

exit "$rc"
