#!/bin/sh
# image-config-secret-classifier.sh — read-only classification of baked image env.
#
# Purpose: decide whether a preserved image carries a BAKED CREDENTIAL, without ever
# printing an environment value. `docker image save` embeds the whole image config,
# including its `Env` array, so an image built with an `ENV DATABASE_URL=…` ships that
# value into the archive and to anyone who later restores it. This repo has done exactly
# that before — `docs/PRODUCTION_READINESS_AUDIT.md` records AUDIT-801, an
# `ENV DATABASE_URL=postgresql://sigap:sigap@postgres:5432/...` in the queue-engine
# Dockerfile, removed in `70f8d03`.
#
# The defect this closes: the runbook used to prescribe
#
#     docker image inspect "sigap-$s" --format '{{json .Config.Env}}'
#
# which prints every baked value in cleartext — into the transcript, the CI log, the
# evidence bundle and any Jira comment it is pasted into. Classifying a secret is not
# the same as disclosing it.
#
# THIS SCRIPT NEVER PRINTS A VALUE. It emits, per image:
#   - the image reference it read (safe: it is a name/ID the operator already has)
#   - the COUNT of baked env entries
#   - each env KEY NAME, and a per-key verdict: SAFE | REVIEW | STOP
#   - one overall verdict line
#
# Runtime-injected env is DISTINCT from baked env and is NOT inspected: compose passes
# `SIGAP_AUTH_ISSUER`, `DATABASE_URL`, `POSTGRES_PASSWORD` etc. at container start, so
# they live in the container's runtime env, not in the image config. `docker image
# inspect` reads the image config only. This script therefore answers "does the IMAGE
# carry a credential", which is the question that determines whether the archive is
# safe to preserve.
#
# Exit codes: 0 no baked credential suspected (SAFE), 2 at least one REVIEW key,
# 1 at least one STOP key or the inspection could not be read.

set -u

pass=0; failed=0; review=0; stop=0

# Keys whose VALUES are secrets if they were ever baked. Presence of the KEY in the
# image config is itself the signal; the value is never read or printed.
SECRET_KEYS='PASSWORD|SECRET|TOKEN|CREDENTIAL|PRIVATE_KEY|API_KEY|ACCESS_KEY|_URL|DSN|DATABASE_URL|REDIS_URL|AMQP_URL|SMTP_|JWT_SECRET|SIGNING_KEY|ENCRYPTION_KEY|WEBHOOK_SECRET|BASICAUTH|_AUTH|CERT|CERTIFICATE|KEYSTORE'

# Keys known-benign at build time in this repo's Dockerfiles (ports, log level, addr),
# plus the base-image variables Docker always bakes in when a Dockerfile FROMs an image
# that sets them. [OBSERVED] on the production host 2026-10-10: sigap-api bakes PATH,
# sigap-web bakes PATH/NODE_VERSION/YARN_VERSION, sigap-rust-engine bakes PATH. Without
# these in the allowlist every image could only ever reach REVIEW, which makes the gate
# noise and trains the operator to click past it.
BENIGN_KEYS='^(PATH|NODE_VERSION|YARN_VERSION|NPM_VERSION|PNPM_VERSION|GPG_KEY|SIGAP_API_PORT|SIGAP_ENGINE_ADDR|PORT|HOST|RUST_LOG|NODE_ENV|SIGAP_ENV|GIN_MODE|TZ|LANG|HOME|SSL_CERT_FILE|PNPM_HOME|DEBIAN_FRONTEND|APT_KEY_DONT_WARN_ON_DANGEROUS_USAGE)$'

classify_key() {
  k=$1
  if printf '%s' "$k" | grep -Eq "$BENIGN_KEYS"; then
    printf 'SAFE'
    return 0
  fi
  if printf '%s' "$k" | grep -Eiq "$SECRET_KEYS"; then
    printf 'STOP'
    return 1
  fi
  printf 'REVIEW'
  return 2
}

for svc in "$@"; do
  printf 'image %s\n' "$svc"

  # Count baked env entries without printing them.
  # `docker image inspect` prints an empty string and exits 0 for a wrong format
  # directive on some Docker versions, so an unparsable count would silently
  # become 0 and the verdict would read SAFE. That is a false PASS in a security
  # gate, so the count is validated as a positive integer and anything else aborts.
  n=$(docker image inspect "$svc" --format '{{len .Config.Env}}' 2>/dev/null) || n=''
  case "$n" in
    ''|*[!0-9]*)
      printf '  UNREADABLE (env count not an integer: "%s")\n' "$n"
      failed=$((failed+1)); continue ;;
    0)
      printf '  baked_env_count=0\n'
      printf '  VERDICT: SAFE (no baked env at all)\n'
      pass=$((pass+1)); continue ;;
  esac
  printf '  baked_env_count=%s\n' "$n"

  # Emit KEY NAMES ONLY. `cut -d= -f1` discards every value before it reaches stdout.
  # A failed per-key read must also abort rather than be counted as a key named "".
  i=0
  while [ "$i" -lt "$n" ]; do
    entry=$(docker image inspect "$svc" \
              --format "{{index .Config.Env $i}}" 2>/dev/null) || entry=''
    case "$entry" in
      *'='*) key=$(printf '%s' "$entry" | cut -d= -f1) ;;
      *) printf '  UNREADABLE (entry %s could not be read)\n' "$i"
         failed=$((failed+1)); i=$((i+1)); continue ;;
    esac
    v=$(classify_key "$key")
    case "$v" in
      SAFE)   pass=$((pass+1)) ;;
      REVIEW) review=$((review+1)) ;;
      STOP)   stop=$((stop+1)) ;;
    esac
    printf '  key=%s verdict=%s\n' "$key" "$v"
    i=$((i+1))
  done
done

printf '\n--- classification: %d safe, %d review, %d stop, %d unreadable ---\n' \
  "$pass" "$review" "$stop" "$failed"

if [ "$failed" -gt 0 ]; then
  printf 'VERDICT: UNREADABLE — cannot classify; STOP before preservation\n'
  exit 1
fi
if [ "$stop" -gt 0 ]; then
  printf 'VERDICT: STOP — a baked credential is suspected. Do NOT preserve. Request owner disposition.\n'
  exit 1
fi
if [ "$review" -gt 0 ]; then
  printf 'VERDICT: REVIEW — an unrecognised baked key is present. Have the owner classify it before preservation.\n'
  exit 2
fi
printf 'VERDICT: SAFE — no baked credential suspected. Preservation may proceed subject to the other gates.\n'
exit 0
