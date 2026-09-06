#!/usr/bin/env bash
# Sigap — production PostgreSQL backup (AUDIT-701).
# Creates a timestamped pg_dump --format=custom dump, validates it,
# computes a SHA-256 checksum, and optionally uploads both files to an
# off-host S3-compatible bucket. Designed for systemd timer execution.
#
# Exit non-zero on any failure so the systemd unit/scheduler detects it.
#
# Required: DATABASE_URL
# Optional: SIGAP_BACKUP_DIR (default ./backups relative to script, or /var/backups/sigap)
# Optional: SIGAP_BACKUP_S3_* (off-host upload; see BACKUP_RESTORE.md)
#   SIGAP_BACKUP_S3_ENDPOINT, SIGAP_BACKUP_BUCKET, SIGAP_BACKUP_ACCESS_KEY,
#   SIGAP_BACKUP_SECRET_KEY, SIGAP_BACKUP_S3_REGION
# Optional: SIGAP_BACKUP_RETENTION_DAYS (default 7; files older are deleted from local dir and, if S3, listed for manual review)
#
# Usage (Linux/VPS):
#   DATABASE_URL="postgresql://sigap:***@postgres:5432/sigap?sslmode=require" \
#   SIGAP_BACKUP_DIR=/var/backups/sigap ./scripts/ops/backup-postgres.sh
#
# For Windows dev verification: run under Git Bash or WSL.

set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required (never log its value)}"

BACKUP_DIR="${SIGAP_BACKUP_DIR:-./backups/sigap}"
RETENTION_DAYS="${SIGAP_BACKUP_RETENTION_DAYS:-7}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DUMP_NAME="sigap-${STAMP}.dump"
DUMP_TMP="${BACKUP_DIR}/.${DUMP_NAME}.tmp"
DUMP_FINAL="${BACKUP_DIR}/${DUMP_NAME}"
CHECKSUM_FINAL="${DUMP_FINAL}.sha256"

mkdir -p "${BACKUP_DIR}"

log() { printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

log "backup: starting (dir=${BACKUP_DIR}, retention=${RETENTION_DAYS}d, stamp=${STAMP})"

start_ts="$(date +%s)"

# Atomic write: pg_dump to temp file, then rename. --format=custom enables pg_restore flex restore.
# Never log DATABASE_URL.
if ! pg_dump --format=custom --no-owner --no-acl --verbose --file="${DUMP_TMP}" "${DATABASE_URL}" 2>&1 | sed 's/postgresql:\/\/[^ ]*/postgresql:\/\/***REDACTED***/g'; then
  rm -f "${DUMP_TMP}"
  log "backup: FAIL pg_dump"
  exit 1
fi

# Validate: non-empty, pg_restore --list must succeed (proves custom-format integrity).
if [ ! -s "${DUMP_TMP}" ]; then
  rm -f "${DUMP_TMP}"
  log "backup: FAIL dump is empty"
  exit 1
fi

if ! pg_restore --list "${DUMP_TMP}" >/dev/null 2>&1; then
  rm -f "${DUMP_TMP}"
  log "backup: FAIL pg_restore --list validation failed"
  exit 1
fi

mv -f "${DUMP_TMP}" "${DUMP_FINAL}"

# SHA-256 checksum (hex lowercase, no path leak).
if command -v sha256sum >/dev/null 2>&1; then
  (cd "${BACKUP_DIR}" && sha256sum "${DUMP_NAME}" > "${DUMP_NAME}.sha256")
elif command -v shasum >/dev/null 2>&1; then
  (cd "${BACKUP_DIR}" && shasum -a 256 "${DUMP_NAME}" > "${DUMP_NAME}.sha256")
else
  log "backup: WARN no sha256sum/shasum found; checksum skipped"
fi

size_bytes="$(wc -c < "${DUMP_FINAL}" | tr -d ' ')"
end_ts="$(date +%s)"
duration="$(( end_ts - start_ts ))"
log "backup: created ${DUMP_FINAL} (${size_bytes} bytes, ${duration}s)"
if [ -f "${CHECKSUM_FINAL}" ]; then
  log "backup: checksum $(cat "${CHECKSUM_FINAL}")"
fi

# Off-host upload if S3 env is configured. Upload both dump and checksum. Requires aws cli or rclone/aws.
if [ -n "${SIGAP_BACKUP_BUCKET:-}" ] && [ -n "${SIGAP_BACKUP_ACCESS_KEY:-}" ] && [ -n "${SIGAP_BACKUP_SECRET_KEY:-}" ]; then
  endpoint="${SIGAP_BACKUP_S3_ENDPOINT:-}"
  region="${SIGAP_BACKUP_S3_REGION:-auto}"
  log "backup: uploading to s3 bucket=${SIGAP_BACKUP_BUCKET} endpoint=${endpoint:-aws} region=${region}"
  export AWS_ACCESS_KEY_ID="${SIGAP_BACKUP_ACCESS_KEY}"
  export AWS_SECRET_ACCESS_KEY="${SIGAP_BACKUP_SECRET_KEY}"
  # shellcheck disable=SC2034
  export AWS_DEFAULT_REGION="${region}"
  # Prefer aws cli v2 if available.
  if command -v aws >/dev/null 2>&1; then
    aws_args=()
    if [ -n "${endpoint}" ]; then aws_args+=(--endpoint-url "${endpoint}"); fi
    aws_args+=(--region "${region}")
    if ! aws s3 cp "${DUMP_FINAL}" "s3://${SIGAP_BACKUP_BUCKET}/${DUMP_NAME}" "${aws_args[@]}"; then
      log "backup: FAIL aws s3 cp dump"
      exit 1
    fi
    if [ -f "${CHECKSUM_FINAL}" ]; then
      if ! aws s3 cp "${CHECKSUM_FINAL}" "s3://${SIGAP_BACKUP_BUCKET}/${DUMP_NAME}.sha256" "${aws_args[@]}"; then
        log "backup: FAIL aws s3 cp checksum"
        exit 1
      fi
    fi
    log "backup: uploaded to s3://${SIGAP_BACKUP_BUCKET}/${DUMP_NAME}"
  elif command -v rclone >/dev/null 2>&1; then
    # rclone requires a remote; use env-provided via RCLONE_CONFIG_* is out of scope here.
    log "backup: FAIL rclone path not configured (use aws cli)"
    exit 1
  else
    log "backup: FAIL SIGAP_BACKUP_BUCKET is set but neither 'aws' nor compatible uploader is available"
    exit 1
  fi
  # Unset to avoid leaking in ps aux / env dumps in logs below (still in shell, but not exported further).
  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_DEFAULT_REGION
else
  if [ -n "${SIGAP_BACKUP_BUCKET:-}" ]; then
    log "backup: WARN SIGAP_BACKUP_BUCKET set but ACCESS_KEY/SECRET_KEY missing — skipping upload (local-only backup is NOT off-host; audit PARTIAL)"
  else
    log "backup: no S3 bucket configured — local-only backup (audit PARTIAL until off-host storage is configured; see BACKUP_RESTORE.md)"
  fi
fi

# Retention: delete local files older than RETENTION_DAYS, only inside BACKUP_DIR and only sigap-*.dump / sigap-*.dump.sha256.
if [ "${RETENTION_DAYS}" -gt 0 ] 2>/dev/null; then
  # shellcheck disable=SC2016
  deleted="$(find "${BACKUP_DIR}" -maxdepth 1 -type f \( -name 'sigap-*.dump' -o -name 'sigap-*.dump.sha256' \) -mtime +"${RETENTION_DAYS}" -print -delete 2>/dev/null | wc -l | tr -d ' ')"
  if [ "${deleted}" != "0" ]; then
    log "backup: retention pruned ${deleted} file(s) older than ${RETENTION_DAYS}d"
  fi
fi

log "backup: done"
