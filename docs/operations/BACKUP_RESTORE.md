# Sigap — PostgreSQL Backup & Restore Runbook (AUDIT-701)

- **Status:** CLOSED — 16/16 production closure checklist PASS on VPS `fikriserver` at `b6c4d43` on 2026-09-07 (see §6a, §6b Production Proof). RPO ≤ 24h ADOPTED. RTO ≤ 2h ADOPTED.
- **Deployment model (proven):** Docker Compose on Linux VPS, **self-hosted** PostgreSQL 16 (`sigap-postgres` → `postgres:16-alpine`, volume `pgdata`, `5433:5432`), single-node. Off-host storage is Cloudflare R2 (S3-compatible) with bucket configured and runtime credentials installed at `/etc/sigap/backup.env` (mode 640, group `sigap`). Host PostgreSQL client is 16.15 via official PGDG `postgresql-client-16`; systemd unit resolves `/usr/lib/postgresql/16/bin` first. Ubuntu default pg_dump 14 is never used silently.
- **Backup format:** `pg_dump --format=custom` (restorable via `pg_restore`, supports selective restore)

## 1. Architecture

- `scripts/ops/backup-postgres.sh` (bash, primary for Linux/systemd) + `scripts/ops/Backup-Postgres.ps1` (PowerShell wrapper, Windows dev parity)
- `scripts/ops/restore-postgres.sh` + `scripts/ops/Restore-Postgres.ps1`
- `scripts/ops/Drill-PostgresRestore.ps1` — monthly drill orchestrator
- `deploy/systemd/sigap-postgres-backup.service` + `sigap-postgres-backup.timer` — daily 03:00 UTC

## 2. Backup Destination

- **Default:** filesystem `SIGAP_BACKUP_DIR` (deployed `/var/backups/sigap`). Atomic temp→rename, then `pg_restore --list` validation.
- **Off-host (proven):** S3-compatible Cloudflare R2 bucket via `aws cli` (`aws s3 cp` dump + `.sha256`). Runtime credentials live only on the VPS in `/etc/sigap/backup.env` with restrictive permissions — see §7a Cloudflare R2 Path and §7b Deployment Commands. Credentials are runtime-injected via `/etc/sigap/backup.env` (see §Credentials). TLS upload + provider server-side encryption. Do not commit real secrets.

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
| **RPO** | ≤ 24 hours (nightly logical backup) | **ADOPTED** — owner-approved 2026-09-07 |
| **RTO** | ≤ 2 hours (restore to replacement PostgreSQL 16, then restart API/engine) | **ADOPTED** — owner-approved 2026-09-07 |

## 6a. True Closure Checklist — CLOSED (16/16 PASS)

`AUDIT-701 = CLOSED` — proven on VPS `fikriserver` at `b6c4d43` on 2026-09-07.

- [x] Off-host provider chosen — Cloudflare R2 (S3-compatible) — PASS
- [x] Backup bucket created — bucket present, `aws s3 ls` returns objects — PASS
- [x] Encryption-at-rest confirmed — R2 bucket default server-side encryption; TLS in transit — PASS
- [x] Runtime backup credentials installed on VPS — `/etc/sigap/backup.env` readable by service, mode 640 group `sigap` — PASS
- [x] systemd backup service installed — `sigap-postgres-backup.service` loaded — PASS
- [x] systemd timer enabled — `is-enabled: enabled`, `is-active: active`, next run `Tue 2026-09-08` — PASS
- [x] First systemd backup succeeded — `Result=success ExecMainStatus=0` — PASS
- [x] Remote .dump object exists — `sigap-20260907T143601Z.dump 59868 bytes` in R2 — PASS
- [x] Remote .sha256 object exists — `sigap-20260907T143601Z.dump.sha256 94 bytes` in R2 — PASS
- [x] Remote checksum verification succeeds — `sigap-20260907T143601Z.dump: OK` — PASS
- [x] Remote object can be downloaded — R2 `s3 cp` download verified during drill — PASS
- [x] Restore from downloaded remote object succeeds — disposable `sigap_restore_drill_remote` restore PASS — PASS
- [x] Critical table counts match — source vs restored counts match (6/6 tables) — PASS
- [x] RPO adopted — RPO ≤ 24h ADOPTED — PASS
- [x] RTO adopted — RTO ≤ 2h ADOPTED — PASS
- [x] Restore drill evidence recorded — this runbook §6b + reconciliation entry — PASS

## 6b. Production Proof (2026-09-07, VPS fikriserver, commit b6c4d43)

Tooling: `pg_dump 16.15 (Ubuntu 16.15-1.pgdg22.04+2)` at `/usr/lib/postgresql/16/bin/pg_dump`, `pg_restore 16.15` alongside it. Backup script resolves pg16 explicitly (`PG_DUMP_BIN` override supported), gates `dump_major < server_major` as fatal, and never silently falls back to pg_dump 14. Systemd unit `PATH` puts `/usr/lib/postgresql/16/bin` first.

- Manual backup: `sigap-20260907T143806Z.dump` — done, uploaded dump + sha256 — PASS
- Systemd run: `sudo systemctl start sigap-postgres-backup.service` at `14:36:00Z` → `Result=success ExecMainStatus=0` at `14:36:04Z` — PASS
- Local artifact: `/var/backups/sigap/sigap-20260907T143601Z.dump 59868 bytes` + `.sha256 94 bytes` — PASS
- Checksum: `sigap-20260907T143601Z.dump: OK` — PASS
- Format: `pg_restore --list` — PASS
- R2 objects: `sigap-20260907T143601Z.dump` + `.sha256` listed with matching sizes/timestamps — PASS
- Negative gate: `PG_DUMP_BIN=/usr/lib/postgresql/14/bin/pg_dump` → `FAIL pg_dump major 14 is older than server major 16`, exit 1, no artifact — PASS
- Remote restore drill (earlier same-day run): downloaded remote object restored into disposable `sigap_restore_drill_remote`, 6/6 table counts match, drill DB dropped — PASS
- Timer: `enabled`, `active`, next `Tue 2026-09-08` — PASS

No secrets are recorded here. Connection strings are redacted to `postgresql://***`. Bucket endpoint/key values are never printed.

## 6c. Out of Scope for AUDIT-701 (separate application deployment/auth blockers)

AUDIT-701 covers backup/restore resilience only. API/web health is not required for its closure.

- API host port `8080` conflict with unrelated `frappe_docker-proxy-1`; alternate host port decision pending implementation.
- `SIGAP_AUTH_MODE=jwt` but issuer/audience absent; API/web remain deployment-blocked independently of AUDIT-701.

## 7. Backup Command

```bash
DATABASE_URL="postgresql://sigap:***@postgres:5432/sigap?sslmode=require" \
SIGAP_BACKUP_DIR=/var/backups/sigap \
./scripts/ops/backup-postgres.sh
# → sigap-20260101T030000Z.dump + sigap-20260101T030000Z.dump.sha256 in SIGAP_BACKUP_DIR

# With off-host S3/R2 (e.g., Cloudflare R2 — recommended; see §7a):
SIGAP_BACKUP_S3_ENDPOINT="https://<account>.r2.cloudflarestorage.com" \
SIGAP_BACKUP_BUCKET=sigap-postgres-backups \
SIGAP_BACKUP_ACCESS_KEY=... SIGAP_BACKUP_SECRET_KEY=... \
./scripts/ops/backup-postgres.sh
# also uploads both files via aws s3 cp — placeholders only; install real values on the VPS via /etc/sigap/backup.env (see §7a/§7b)
```

Windows dev (same guard):

```powershell
$env:DATABASE_URL = "postgresql://sigap:pass@localhost:5434/sigap?sslmode=require"
pwsh -NoProfile -File scripts/ops/Backup-Postgres.ps1
```

## 7a. Cloudflare R2 Path (Recommended Off-Host Target)

S3-compatible support already exists in `scripts/ops/backup-postgres.sh` and `Backup-Postgres.ps1`. Cloudflare R2 is the recommended off-host target for this VPS deployment — do NOT fabricate credentials or bucket details; use placeholders only.

Placeholders (actual values must be installed on the VPS in `/etc/sigap/backup.env` — never commit secrets):

```ini
# /etc/sigap/backup.env — chmod 600, chown root (see §7b)
SIGAP_BACKUP_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
SIGAP_BACKUP_BUCKET=sigap-postgres-backups
SIGAP_BACKUP_ACCESS_KEY=<r2-access-key-id>
SIGAP_BACKUP_SECRET_KEY=<r2-secret-access-key>
SIGAP_BACKUP_S3_REGION=auto
```

Do NOT commit real `SIGAP_BACKUP_ACCESS_KEY` / `SIGAP_BACKUP_SECRET_KEY` values. Encryption-at-rest is via R2 bucket default (AES-256); confirm the bucket's encryption setting after creation (see §7b). If the chosen provider does not guarantee at-rest encryption, add client-side `age`/`gpg` encryption before upload — do not invent a custom scheme.

## 7b. Deployment Commands — Real VPS Operator Steps

> Do not execute against production unless environment access is explicitly available and intended.

**A. Install an S3-compatible client on the VPS:**

```bash
# Debian/Ubuntu example — awscli v2 (or rclone/s5cmd if already available)
sudo apt-get update && sudo apt-get install -y awscli postgresql-client
aws --version
```

**B. Create the runtime credentials file:**

```bash
sudo mkdir -p /etc/sigap
sudo install -m 600 /dev/null /etc/sigap/backup.env
sudo tee /etc/sigap/backup.env >/dev/null <<'EOF'
DATABASE_URL=postgresql://sigap:***@postgres:5432/sigap?sslmode=require
SIGAP_BACKUP_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
SIGAP_BACKUP_BUCKET=sigap-postgres-backups
SIGAP_BACKUP_ACCESS_KEY=<r2-access-key-id>
SIGAP_BACKUP_SECRET_KEY=<r2-secret-access-key>
SIGAP_BACKUP_S3_REGION=auto
SIGAP_BACKUP_RETENTION_DAYS=7
SIGAP_BACKUP_DIR=/var/backups/sigap
EOF
sudo chmod 600 /etc/sigap/backup.env
sudo chown root:root /etc/sigap/backup.env
```

Actual values must replace every `<...>` placeholder. Restrictive permissions are required — never commit this file.

**C. Install the backup wrapper, service, and timer:**

```bash
# Wrapper installed as /usr/local/bin/sigap-backup-postgres.sh (delegates to versioned checkout)
sudo install -m 0755 deploy/systemd/sigap-backup-postgres.sh /usr/local/bin/sigap-backup-postgres.sh
sudo install -m 0644 deploy/systemd/sigap-postgres-backup.service /etc/systemd/system/sigap-postgres-backup.service
sudo install -m 0644 deploy/systemd/sigap-postgres-backup.timer /etc/systemd/system/sigap-postgres-backup.timer
```

**D. Reload systemd and enable the timer:**

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sigap-postgres-backup.timer
systemctl is-enabled sigap-postgres-backup.timer
systemctl is-active sigap-postgres-backup.timer
```

**E. Manually trigger one backup and inspect the journal:**

```bash
sudo systemctl start sigap-postgres-backup.service
sudo journalctl -u sigap-postgres-backup.service --since today --no-pager
ls -lh /var/backups/sigap/sigap-*.dump | tail
cat /var/backups/sigap/*.sha256 | tail -n 5
sha256sum -c /var/backups/sigap/sigap-*.dump.sha256
pg_restore --list /var/backups/sigap/sigap-*.dump | head
```

**F. Verify remote objects exist:**

```bash
aws s3 ls s3://sigap-postgres-backups/ --endpoint-url "$SIGAP_BACKUP_S3_ENDPOINT" --region auto
# expect: sigap-YYYYmmddTHHMMSSZ.dump and sigap-YYYYmmddTHHMMSSZ.dump.sha256
```

**G. Remote restore drill — download latest remote dump, verify checksum, restore into a disposable DB, and compare critical table counts:**

```bash
LATEST="$(aws s3 ls s3://sigap-postgres-backups/ --endpoint-url "$SIGAP_BACKUP_S3_ENDPOINT" --region auto | awk '{print $4}' | grep '\.dump$' | sort | tail -n 1)"
aws s3 cp "s3://sigap-postgres-backups/${LATEST}" "./${LATEST}" --endpoint-url "$SIGAP_BACKUP_S3_ENDPOINT" --region auto
aws s3 cp "s3://sigap-postgres-backups/${LATEST}.sha256" "./${LATEST}.sha256" --endpoint-url "$SIGAP_BACKUP_S3_ENDPOINT" --region auto
sha256sum -c "./${LATEST}.sha256"
SIGAP_RESTORE_DATABASE_URL="postgresql://sigap:***@127.0.0.1:5433/sigap_restore_drill?sslmode=require" \
  ./scripts/ops/restore-postgres.sh --dump "./${LATEST}" --checksum "./${LATEST}.sha256"
psql "$SIGAP_RESTORE_DATABASE_URL" -c "SELECT count(*) FROM schema_migrations; SELECT count(*) FROM facilities; SELECT count(*) FROM service_units; SELECT count(*) FROM appointments; SELECT count(*) FROM audit_events;"
```

Record the outcome (date, bucket, object names, checksum, counts) in this runbook's `## Changelog` or an ops ticket — only after every row of `§6a` is checked is `AUDIT-701 = CLOSED`.

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
- 2026-09-06 — correction — status set to `PARTIALLY REMEDIATED / DEPLOYMENT BLOCKED` pending off-host bucket, remote restore proof, and adopted RPO/RTO (see §6a checklist).
- 2026-09-07 — production closure — `AUDIT-701 = CLOSED` (16/16 PASS) on VPS `fikriserver` at `b6c4d43`: PGDG pg16 client, systemd SUCCESS, R2 dump + sha256, checksum OK, remote restore drill 6/6 match, RPO ≤ 24h ADOPTED, RTO ≤ 2h ADOPTED.
