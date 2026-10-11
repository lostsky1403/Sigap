#!/bin/sh
# image-config-secret-classifier.sh — non-disclosing classification of baked image env.
#
# PURPOSE. `docker image save` embeds the whole image config, so an image built with an
# `ENV DATABASE_URL=...` ships that value into the archive. This script decides whether
# a preserved image carries a baked credential, WITHOUT ever printing a value.
#
# SCOPE — STATED HONESTLY. This inspects `Config.Env` only. It checks KEY NAMES, so it
# classifies "is a credential-shaped variable baked in". It does NOT:
#   - read image LABELS (a label value can hold a token)
#   - read image HISTORY or build arguments (`--build-arg SECRET=...` is recorded)
#   - scan layers or embedded files for credential material
#   - detect a secret baked into a file inside the image
# A verdict of SAFE_WITH_DEFINED_SCOPE therefore means "no credential-shaped env KEY is
# baked", NOT "this image contains no secrets anywhere". That is a real limitation and is
# printed with the verdict so it cannot be mistaken for a full secret scan.
#
# Exit codes: 0 SAFE_WITH_DEFINED_SCOPE, 2 REVIEW_REQUIRED, 1 BLOCKED or UNREADABLE.

set -u

pass=0; review=0; stop=0; unread=0

# Secret-shaped key names. Presence of the NAME is the signal; the value is never read.
SECRET_KEYS='PASSWORD|PASSWD|SECRET|TOKEN|CREDENTIAL|PRIVATE_KEY|API_KEY|ACCESS_KEY|_URL$|_URL=|DSN|DATABASE_URL|REDIS_URL|AMQP_URL|SMTP_|JWT_SECRET|SIGNING_KEY|ENCRYPTION_KEY|WEBHOOK_SECRET|BASICAUTH|_AUTH|_AUTH$|CERT|CERTIFICATE|KEYSTORE'

# Keys known-benign: this repo's Dockerfiles, plus base-image variables Docker bakes.
BENIGN_KEYS='^(PATH|NODE_VERSION|YARN_VERSION|NPM_VERSION|PNPM_VERSION|GPG_KEY|SIGAP_API_PORT|SIGAP_ENGINE_ADDR|PORT|HOST|RUST_LOG|NODE_ENV|SIGAP_ENV|GIN_MODE|TZ|LANG|HOME|SSL_CERT_FILE|PNPM_HOME|DEBIAN_FRONTEND|APT_KEY_DONT_WARN_ON_DANGEROUS_USAGE)$'

classify_key() {
  k=$1
  if printf '%s' "$k" | grep -Eq "$BENIGN_KEYS"; then printf 'SAFE'; return 0; fi
  if printf '%s' "$k" | grep -Eiq "$SECRET_KEYS"; then printf 'STOP'; return 1; fi
  printf 'REVIEW'; return 2
}

for svc in "$@"; do
  printf 'image %s\n' "$svc"

  # ---- SCOPE 1: Config.Env (what this script is designed to classify) -------------
  n=$(docker image inspect "$svc" --format '{{len .Config.Env}}' 2>/dev/null) || n=''
  # A count that does not parse MUST abort. `docker inspect` can print an empty string
  # and exit 0 for a wrong format directive, which would make n=0 and the verdict read
  # SAFE — a false PASS in a security gate. [OBSERVED] the first version did exactly that.
  case "$n" in
    ''|*[!0-9]*) printf '  env: UNREADABLE (count not an integer: "%s")\n' "$n"
                 unread=$((unread+1)); continue ;;
    0) printf '  env: 0 baked entries\n' ;;
  esac
  [ "$n" -gt 0 ] || continue

  i=0
  while [ "$i" -lt "$n" ]; do
    entry=$(docker image inspect "$svc" --format "{{index .Config.Env $i}}" 2>/dev/null) || entry=''
    # cut -d= -f1 discards the value BEFORE it can reach stdout.
    case "$entry" in
      *'='*) key=$(printf '%s' "$entry" | cut -d= -f1) ;;
      *) printf '  entry %s: UNREADABLE\n' "$i"; unread=$((unread+1)); i=$((i+1)); continue ;;
    esac
    v=$(classify_key "$key")
    case "$v" in
      SAFE)   pass=$((pass+1)) ;;
      REVIEW) review=$((review+1)) ;;
      STOP)   stop=$((stop+1)) ;;
    esac
    printf '  env key=%s verdict=%s\n' "$key" "$v"
    i=$((i+1))
  done

  # ---- SCOPE DISCLOSURE: what this script does NOT cover -------------------------
  # Pasted from LABELS and HISTORY would risk printing a token, so only the COUNT is
  # reported. A non-zero label/history count is a REVIEW signal, never a pass.
  labels=$(docker image inspect "$svc" --format '{{len .Config.Labels}}' 2>/dev/null) || labels=''
  hist=$(docker image inspect "$svc" --format '{{len .History}}' 2>/dev/null) || hist=''
  case "$labels" in ''|*[!0-9]*) labels='?' ;; esac
  case "$hist" in ''|*[!0-9]*) hist='?' ;; esac
  printf '  not-inspected: labels=%s history=%s (values deliberately NOT read)\n' "$labels" "$hist"
  if [ "$labels" != "0" ] && [ "$labels" != "?" ]; then review=$((review+1)); fi
done

printf '\n--- env keys: %d safe, %d review, %d stop, %d unreadable ---\n' \
  "$pass" "$review" "$stop" "$unread"

if [ "$unread" -gt 0 ]; then
  printf 'VERDICT: BLOCKED — inspection incomplete; do not preserve\n'
  exit 1
fi
if [ "$stop" -gt 0 ]; then
  printf 'VERDICT: BLOCKED — a baked credential-shaped env key is present. Do NOT preserve.\n'
  printf '         Request owner disposition.\n'
  exit 1
fi
if [ "$review" -gt 0 ]; then
  printf 'VERDICT: REVIEW_REQUIRED — an unrecognised env key, or labels/history not inspected.\n'
  exit 2
fi
printf 'VERDICT: SAFE_WITH_DEFINED_SCOPE — no credential-shaped env key is baked.\n'
printf '         SCOPE: Config.Env key names only. This is NOT a full image secret scan:\n'
printf '         labels, history/build args, and layer contents were NOT examined.\n'
exit 0
