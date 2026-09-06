#!/usr/bin/env bash
# Wrapper installed as /usr/local/bin/sigap-backup-postgres.sh on the VPS.
# Delegates to the versioned script in the deployed checkout; keeps the unit file stable across deploys.
set -euo pipefail
exec /opt/sigap/scripts/ops/backup-postgres.sh "$@"
