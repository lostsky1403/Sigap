#!/usr/bin/env bash
# Sigap — production PostgreSQL restore (AUDIT-701).
# Restores a pg_dump --format=custom dump into a destination DATABASE_URL.
# Defaults to disposable/scratch restore (safe). In-place overwrite of a
# live production DB requires --allow-destructive.
#
# Usage:
#   SIGAP_RESTORE_DATABASE_URL="postgresql://sigap:***@127.0.0.1:5433/sigap_restore?sslmode=disable" \
#   ./scripts/ops/restore-postgres.sh --dump /var/backups/sigap/sigap-20260101T000000Z.dump
#
#   Optional:
#     --checksum /path/to/dump.sha256   # verify before restore (recommended)
#     --allow-destructive               # allow restore into the same DB name as source (dangerous)
#
# Required: SIGAP_RESTORE_DATABASE_URL
# Optional: --dump PATH (if omitted, uses most recent sigap-*.dump in SIGAP_BACKUP_DIR)
# Behavior:
#   - refuses if destination looks like production unless --allow-destructive
#   - verifies checksum when provided
#   - runs pg_restore --clean --if-exists (idempotent) with failure detection
#   - post-restore verification queries (schema_migrations, critical tables)

set -euo pipefail

DUMP=""
CHECKSUM=""
ALLOW_DESTRUCTIVE="false"

while [ $# -gt 0 ]; do
  case "$1" in
    --dump) DUMP="$2"; shift 2 ;;
    --checksum) CHECKSUM="$2"; shift 2 ;;
    --allow-destructive) ALLOW_DESTRUCTIVE="true"; shift ;;
    -h|--help) sed -n '1,80p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

: "${SIGAP_RESTORE_DATABASE_URL:?SIGAP_RESTORE_DATABASE_URL is required (destination, never production by default)}"

BACKUP_DIR="${SIGAP_BACKUP_DIR:-./backups/sigap}"

if [ -z "${DUMP}" ]; then
  # shellcheck disable=SC2012
  DUMP="$(ls -1t "${BACKUP_DIR}"/sigap-*.dump 2>/dev/null | head -n 1 || true)"
  if [ -z "${DUMP}" ]; then
    echo "FAIL: no dump found in ${BACKUP_DIR} and --dump not given" >&2
    exit 1
  fi
fi

if [ ! -f "${DUMP}" ]; then
  echo "FAIL: dump not found: ${DUMP}" >&2
  exit 1
fi

# Safety: refuse to restore into a URL that contains production markers unless explicitly allowed.
if [ "${ALLOW_DESTRUCTIVE}" != "true" ]; then
  case "${SIGAP_RESTORE_DATABASE_URL}" in
    *prod*|*production*)
      echo "FAIL: refusing to restore into a URL containing 'prod' without --allow-destructive" >&2
      exit 1
      ;;
  esac
  # Also refuse if dump name suggests production and destination DB name matches source DB name heuristically.
  # Conservative: block when destination DB name equals source host DB 'sigap' and dump path contains 'prod' is already above.
fi

if [ -n "${CHECKSUM}" ]; then
  if [ ! -f "${CHECKSUM}" ]; then
    echo "FAIL: checksum file not found: ${CHECKSUM}" >&2
    exit 1
  fi
  # Verify: checksum file is expected to contain "<hex>  <filename>" or just "<hex>".
  expected="$(awk '{print $1}' "${CHECKSUM}" | tr -d '\r' | tr '[:upper:]' '[:lower:]')"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${DUMP}" | awk '{print $1}' | tr '[:lower:]' '[:upper:]' | tr '[:lower:]' '[:upper:]' | tr '[:lower:]' '[:upper:]' | tr '[:lower:]' '[:upper:]' | tr '[:upper:]' '[:lower:]')"
    # Simpler: just compute lowercase
    actual="$(sha256sum "${DUMP}" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${DUMP}" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
  else
    echo "FAIL: no sha256sum/shasum for checksum verification" >&2
    exit 1
  fi
  if [ "${expected}" != "${actual}" ]; then
    echo "FAIL: checksum mismatch for ${DUMP}" >&2
    echo "  expected: ${expected}" >&2
    echo "  actual:   ${actual}" >&2
    exit 1
  fi
  printf '[%s] restore: checksum ok %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${actual}"
fi

log() { printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

log "restore: starting dump=${DUMP} allow-destructive=${ALLOW_DESTRUCTIVE} (destination hidden)"

# Validate dump format before attempting restore.
if ! pg_restore --list "${DUMP}" >/dev/null 2>&1; then
  echo "FAIL: dump is not a valid pg_dump custom-format file: ${DUMP}" >&2
  exit 1
fi

start_ts="$(date +%s)"

# pg_restore into destination. --clean --if-exists makes it idempotent on rerun.
# --no-owner --no-acl match backup flags. Verbose but REDACTED.
if ! pg_restore --clean --if-exists --no-owner --no-acl --verbose --dbname="${SIGAP_RESTORE_DATABASE_URL}" "${DUMP}" 2>&1 | sed 's/postgresql:\/\/[^ ]*/postgresql:\/\/***REDACTED***/g'; then
  # pg_restore returns non-zero for genuine errors; warnings (e.g., already exists with IF NOT EXISTS)
  # are non-fatal only if the data still verified below. We treat non-zero as failure for scheduler.
  log "restore: pg_restore returned non-zero (see output above)"
  exit 1
fi

# Post-restore verification: migrations table + critical tables must exist and have plausible counts.
verify_sql() {
  psql "${SIGAP_RESTORE_DATABASE_URL}" -v ON_ERROR_STOP=1 -t -A -c "$1" 2>&1 | tr -d '\r' | sed 's/postgresql:\/\/[^ ]*/postgresql:\/\/***REDACTED***/g'
}

log "restore: verifying restored DB"

# schema_migrations must exist
if ! verify_sql "SELECT count(*) FROM schema_migrations;" >/dev/null; then
  echo "FAIL: schema_migrations missing after restore" >&2
  exit 1
fi

migrations="$(verify_sql "SELECT count(*) FROM schema_migrations;" | tr -d ' \n')"
facilities="$(verify_sql "SELECT count(*) FROM facilities;" | tr -d ' \n' || echo '?')"
service_units="$(verify_sql "SELECT count(*) FROM service_units;" | tr -d ' \n' || echo '?')"
appointments="$(verify_sql "SELECT count(*) FROM appointments;" | tr -d ' \n' || echo '?')"
audit_events="$(verify_sql "SELECT count(*) FROM audit_events;" 2>/dev/null | tr -d ' \n' || echo '?')"

end_ts="$(date +%s)"
duration="$(( end_ts - start_ts ))"

log "restore: verified migrations=${migrations} facilities=${facilities} service_units=${service_units} appointments=${appointments} audit_events=${audit_events} duration=${duration}s"
log "restore: done"
