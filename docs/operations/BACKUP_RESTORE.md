# Sigap — PostgreSQL Backup & Restore Runbook (AUDIT-701)

- **Status:** IMPLEMENTED — real backup/restore proven (disposable-DB drill)
- **Deployment model (discovered):** Docker Compose on Linux VPS, **self-hosted** PostgreSQL 16 (`sigap-postgres` → `postgres:16-alpine`, volume `pgdata`, `5433:5432`), no managed Postgres, no S3/R2/WAL in stack. Single-node. No staging/production split in repo. Remote storage is **UNKNOWN** until configured — local backup is production-safe baseline; off-host upload is optional and proven when env is set.
- **Backup format:** `pg_dump --format=custom` (restorable via `pg_restore`, supports selective restore)

## 1. Architecture

- `scripts/ops/backup-postgres.sh` (bash, primary for Linux/systemd) + `scripts/ops/Backup-Postgres.ps1` (PowerShell wrapper, Windows dev parity)
- `scripts/ops/restore-postgres.sh` + `scripts/ops/Restore-Postgres.ps1`
- `scripts/ops/Drill-PostgresRestore.ps1` — monthly drill orchestrator
- `deploy/systemd/sigap-postgres-backup.service` + `sigap-postgres-backup.timer` — daily 03:00 UTC

## 2. Backup Destination

- **Default (implemented):** filesystem `SIGAP_BACKUP_DIR` (defaults to `./backups/sigap` or `/var/backups/sigap` when deployed). Atomic temp→rename, then `pg_restore --list` validation.
- **Off-host (proven when configured):** S3-compatible bucket via `aws cli` (`aws s3 cp` dump + `.sha256`). Env: `SIGAP_BACKUP_S3_ENDPOINT`, `SIGAP_BACKUP_BUCKET`, `SIGAP_BACKUP_ACCESS_KEY`, `SIGAP_BACKUP_SECRET_KEY`, `SIGAP_BACKUP_S3_REGION`. Credentials are runtime-injected via `/etc/sigap/backup.env` (see §Credentials). TLS upload + provider server-side encryption if available.

## 3. Schedule

- `deploy/systemd/sigap-postgres-backup.timer`: `OnCalendar=daily 03:00` UTC, `Persistent=true`, `RandomizedDelaySec=600`.
- Install: copy scripts + units, create `/etc/sigap/backup.env` (see §Credentials), `systemctl daemon-reload && systemctl enable --now sigap-postgres-backup.timer`.

## 4. Retention

- `SIGAP_BACKUP_RETENTION_DAYS` (default 7). Pruner deletes only `sigap-*.dump` / `sigap-*.dump.sha256` in `SIGAP_BACKUP_DIR` older than N days. Safe namespace, no broad wildcard.

## 5. Encryption

- **In transit:** TLS for DB (`DATABASE_URL` `sslmode=require` in compose) and for S3 upload.
- **At rest (off-host):** S3 server-side encryption (bucket default, e.g., AES-256/SSE-S3 or KMS) if the provider offers it. If the chosen provider does not guarantee at-rest encryption, add client-side `age`/`gpg` encryption before upload (not pre-implemented; documented here).
- No custom crypto, no keys in code or logs.

## 6. Recovery Objectives

| Objective | Value | Status |
|---|---|---|
| **RPO** | ≤ 24 hours (nightly logical backup) | **PROPOSED** — product-approved target not yet declared in repo; 24h is the minimum production-safe baseline for the current self-hosted Compose VPS. With WAL/PITR or managed PITR the target tightens to ≤ 5–15 min (see §WAL/PITR). |
| **RTO** | ≤ 1 hour (restore to replacement PostgreSQL 16, then restart API/engine) | **PROPOSED** — verified duration in drill is minutes, but the declared target is proposed until product approves. |

Classify as **PROPOSED** until a product owner pins them in `ROADMAP.md` or an ADR. Closure of AUDIT-701 requires explicit targets.

## 7. Backup Command

```bash
DATABASE_URL="postgresql://sigap:***@postgres:5432/sigap?sslmode=require" \
SIGAP_BACKUP_DIR=/var/backups/sigap \
./scripts/ops/backup-postgres.sh
# → sigap-20260101T030000Z.dump + sigap-20260101T030000Z.dump.sha256 in SIGAP_BACKUP_DIR

# With off-host S3/R2 (e.g., Cloudflare R2):
SIGAP_BACKUP_S3_ENDPOINT="https://<account>.r2.cloudflarestorage.com" \
SIGAP_BACKUP_BUCKET=sigap-postgres-backups \
SIGAP_BACKUP_ACCESS_KEY=... SIGAP_BACKUP_SECRET_KEY=... \
./scripts/ops/backup-postgres.sh
# also uploads both files via aws s3 cp
```

Windows dev (same guard):

```powershell
$env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
pwsh -NoProfile -File scripts/ops/Backup-Postgres.ps1
```

## 8. Verify Latest Backup

```bash
ls -lh /var/backups/sigap/sigap-*.dump | tail
cat /var/backups/sigap/sigap-*.dump.sha256 | tail -n 5
sha256sum -c /var/backups/sigap/sigap-*.dump.sha256   # integrity
pg_restore --list /var/backups/sigap/sigap-*.dump | head  # format validation
journalctl -u sigap-postgres-backup.service --since today  # scheduler logs
systemctl is-active sigap-postgres-backup.timer
```

## 9. Restore to Scratch (Safe Default)

```bash
# Restores the most recent dump into a disposable DB sigap_restore_drill — never overwrites prod.
SIGAP_RESTORE_DATABASE_URL="postgresql://sigap:***@127.0.0.1:5433/sigap_restore_drill?sslmode=require" \
./scripts/ops/restore-postgres.sh --dump /var/backups/sigap/sigap-20260101T030000Z.dump --checksum /var/backups/sigap/sigap-20260101T030000Z.dump.sha256
```

Or explicit file:

```bash
SIGAP_RESTORE_DATABASE_URL="postgresql://sigap:***@127.0.0.1:5433/sigap_restore?sslmode=require" \
./scripts/ops/restore-postgres.sh --dump ./backups/sigap/sigap-20260101T030000Z.dump
```

## 10. Disaster Recovery (Production Restore)

1. Provision replacement PostgreSQL 16 (same major version).
2. Restore the **latest verified** dump into the new instance with the command above (use `--allow-destructive` only if the destination URL is intentionally production and you have taken a second backup).
3. Verify: `SELECT count(*) FROM schema_migrations` + `facilities` / `service_units` / `appointments` / `audit_events`.
4. Rotate `DATABASE_URL` / `POSTGRES_PASSWORD` if compromise is suspected.
5. Restart `sigap-api` / `sigap-rust-engine` / `sigap-web`.
6. Run `sigap-full-local-demo.ps1 -SkipSeed` or targeted smoke against the restored DB.

## 11. Credentials

- `DATABASE_URL` is **never** printed; dump/restore scripts redact `postgresql://` in logs.
- Off-host S3 keys come from `/etc/sigap/backup.env` (systemd `EnvironmentFile`):

```ini
# /etc/sigap/backup.env — chmod 600, chown root
DATABASE_URL=postgresql://sigap:***@postgres:5432/sigap?sslmode=require
# optional off-host
SIGAP_BACKUP_S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com
SIGAP_BACKUP_BUCKET=sigap-postgres-backups
SIGAP_BACKUP_ACCESS_KEY=...
SIGAP_BACKUP_SECRET_KEY=...
SIGAP_BACKUP_S3_REGION=auto
SIGAP_BACKUP_RETENTION_DAYS=7
SIGAP_BACKUP_DIR=/var/backups/sigap
```

## 12. Monthly Restore Drill

```bash
# Creates a disposable sigap_restore_drill, restores latest backup, validates tables, drops it (or --keep).
DATABASE_URL="postgresql://sigap:***@postgres:5432/sigap?sslmode=require" \
pwsh -NoProfile -File scripts/ops/Drill-PostgresRestore.ps1
# keep: pwsh -NoProfile -File scripts/ops/Drill-PostgresRestore.ps1 -Keep
```

Schedule a calendar reminder; record drill outcome (date, dump name, durations, counts) in this file's changelog or a dedicated ops ticket.

## 13. Failure Handling

- **Missing `DATABASE_URL` / `SIGAP_RESTORE_DATABASE_URL`:** scripts exit non-zero, no DB mutation.
- **Missing dump / checksum mismatch:** non-zero, no restore.
- **Upload failure (S3):** backup script exits non-zero; local dump remains (retry on next timer).
- **Restore safety:** refuses URLs containing `prod` without `--allow-destructive`.

## 14. WAL / PITR (Future Tightening)

The current baseline is nightly logical `pg_dump` custom format (RPO 24h). For tighter RPO, enable one of:

- **Self-hosted WAL archiving** (`archive_mode=on`, `archive_command` to S3/R2, `pg_basebackup` + `PITR`), or
- **Managed Postgres** (e.g., Cloud SQL / RDS / Neon) with PITR.

Either choice tightens RPO to minutes; declare it explicitly before claiming it.

## 15. Troubleshooting

- `pg_dump: command not found` — install `postgresql-client` (Linux) or PostgreSQL 18 bin (Windows).
- `pg_restore: not a valid custom-format file` — dump is corrupted or was taken with incompatible version; check checksum and re-backup.
- `psql: could not connect` — verify `DATABASE_URL` vs `POSTGRES_*` in compose env; check `pg_isready`.
- Timer not firing — `systemctl status sigap-postgres-backup.timer` + `journalctl -u sigap-postgres-backup.service`.

## 16. Changelog

- 2026-09-06 — initial workflow (backup/restore/drill + systemd + runbook) — backup + restore verified on disposable DB `sigap_restore_drill` (see PR evidence).
