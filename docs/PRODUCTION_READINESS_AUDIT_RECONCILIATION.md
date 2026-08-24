# Production Audit Reconciliation — After PR #40–#51

- Date: 2026-08-25
- Baseline: `main @ d84fed9` (after PR #51 merge)
- Reconciled against: `docs/PRODUCTION_READINESS_AUDIT.md` (original audit at `b47e07d`)

---

## 1. Merged Remediation PRs

| PR | Audit IDs | What | Merged |
|---|---|---|---|
| #40 | AUDIT-1807, 102, 502, 803 | Dev-only environment guard (`GuardDevCapabilities`) | `a22313f` |
| #41 | AUDIT-802 | Compose `SIGAP_AUTH_MODE:?` required-var + boot banner | `cfef087` |
| #42 | AUDIT-803 | Delete demo PHI handlers + remove `SIGAP_ENABLE_DEMO_PHI` | `c756edb` |
| #43 | AUDIT-901 | TLS termination confirmation guard (`GuardTLS`) | `bd512df` |
| #44 | AUDIT-902 | CORS loopback exception restricted to loopback deployments | `dcdb643` |
| #45 | AUDIT-903 | SSE endpoint hardcoded `Access-Control-Allow-Origin: *` removed | `f0dd744` |
| #46 | AUDIT-905, 1803 | Security headers (CSP, XFO, HSTS, XCTO) on API + web | `30790d3` |
| #47 | AUDIT-906 | Trusted-proxy header sanitization middleware | `142dc5d` |
| #48 | AUDIT-1801, 1802, 1805 | Public catalog endpoints + centralized proxy auth + API base guard | `0b8b7c2` |
| #49 | AUDIT-1202 | `/readyz` pings Postgres + surfaces audit state | `48f68be` |
| #50 | AUDIT-1001, 1601, part-of-501 | HTTP server timeouts, body-size limits, graceful shutdown | `7de5952` |
| #51 | AUDIT-601, 602, 1702 | Tracked migration runner + `schema_migrations` table | `d84fed9` |

---

## 2. P0/P1 Finding Status

### CLOSED (15 findings)

| Audit ID | Original Severity | What Closed It |
|---|---|---|
| AUDIT-102 | P1 | PR #40 — `GuardDevCapabilities` fails startup outside `SIGAP_ENV=local` |
| AUDIT-502 | P2→P1-context | PR #40 — engine fallback env-guarded |
| AUDIT-601 | P1 | PR #51 — Go migration runner + `schema_migrations` table |
| AUDIT-602 | P2 | PR #51 — migrations applied via tracked runner (transactional wrapping pending) |
| AUDIT-802 | P2→P1-context | PR #41 — compose `${SIGAP_AUTH_MODE:?}` fails fast |
| AUDIT-803 | P1 | PR #42 — demo PHI handlers deleted entirely |
| AUDIT-901 | P1 | PR #43 — `GuardTLS` requires `SIGAP_TLS_TERMINATED=true` outside local |
| AUDIT-902 | P2 | PR #44 — CORS loopback exception only for loopback origins |
| AUDIT-903 | P2 | PR #45 — SSE wildcard CORS removed |
| AUDIT-905/1803 | P2/P1 | PR #46 — Security headers on API + SvelteKit |
| AUDIT-906 | P2 | PR #47 — TrustedProxy middleware sanitizes forwarding headers |
| AUDIT-1001 | P1 | PR #50 — `LimitRequestBody`, server timeouts, 15s graceful shutdown |
| AUDIT-1202 | P1 | PR #49 — `/readyz` pings DB + surfaces audit state |
| AUDIT-1601 | P1 | PR #50 — `signal.NotifyContext` + `srv.Shutdown` |
| AUDIT-1801 | P1 | PR #48 — Public catalog endpoints (`/api/v1/public/*`) |
| AUDIT-1802 | P1 | PR #48 — Centralized `$lib/server/auth.ts`, zero direct `SIGAP_DEV_IDENTITY` in proxies |
| AUDIT-1805 | P2 | PR #48 — `apiBase()` guard, no `http://api:8080` fallback |
| AUDIT-1702 | P1 | PR #51 — Migration runner covers 0001–0006 application path |

### STILL OPEN — P1 (10 findings)

| Rank | Audit ID | Domain | Finding | Why Still Open |
|---|---|---|---|---|
| 1 | AUDIT-701 | Backup | No backup mechanism, no restore, RPO ∞ / RTO ∞ | Requires deployment decision (hosting model); no pg_dump automation |
| 2 | AUDIT-101 | AuthN | JWT token `permissions` claim trusted directly; DB RBAC dead code | No server-side `sub → user → roles → permissions` resolution implemented |
| 3 | AUDIT-202 | AuthZ | No facility scoping — cross-facility IDOR | Admin queries return all facilities; `user_roles.facility_id` unused at request time |
| 4 | AUDIT-301 | Privacy | Patient status lookup is enumeration oracle | Distinct 404 vs 200; codes never expire; no second factor |
| 5 | AUDIT-302 | Privacy | Check-in: unlimited brute-force attempts, no rate limit | `CheckIn` handler has no `h.limiter.Allow()` call |
| 6 | AUDIT-607 | Config | Seeds contain DDL + UPDATEs; demo IDs runtime-reachable | `demo.sql` has ALTER TABLE + UPDATEs; no env guard on seed execution |
| 7 | AUDIT-801 | Config | Default DB credentials in engine Dockerfile + `main.rs` fallback | `ENV DATABASE_URL=postgresql://sigap:sigap@...` in Dockerfile; `unwrap_or_else` in main.rs |
| 8 | AUDIT-1004 | Data | Check-in/capacity TOCTOU (non-atomic read-then-write) | UPDATE lacks `AND status='scheduled'`; count-then-insert capacity race |
| 9 | AUDIT-1102 | Audit | Request IDs implemented but never injected into middleware chain | `identity/request_id.go` exists; middleware chain never calls it; audit `request_id` always empty |
| 10 | AUDIT-1701 | CI | Integration/smoke suites never run in CI | No Postgres service container in CI; web job runs only `svelte-check` |

### STILL OPEN — P2 (selected high-impact)

| Audit ID | Domain | Finding |
|---|---|---|
| AUDIT-103/104 | AuthN | No max token lifetime; JWKS URL free-form |
| AUDIT-201 | AuthZ | No policy-completeness test (every route has policy or explicit `Public: true`) |
| AUDIT-303/304 | Privacy | Admin list ships live check-in codes; booking under arbitrary phones |
| AUDIT-401/402 | Rate Limit | Shared-bucket behind proxy; in-memory non-distributed limiter |
| AUDIT-501 | Queue | gRPC client lacks timeout/retry/keepalive; conn never closed |
| AUDIT-604 | DB | Missing UNIQUE constraints on `facilities.short_code` / `appointments.checkin_code` |
| AUDIT-605 | Config | Compose `DATABASE_URL` vs Go `SIGAP_DATABASE_URL` drift (partially addressed) |
| AUDIT-1101 | Logging | No JSON log handler |
| AUDIT-1203 | Observability | No `/metrics` endpoint; no Prometheus/OTel |
| AUDIT-1302 | Notifications | No enqueue dedup; terminal failures accumulate; worker absent from compose |
| AUDIT-1501 | SSE | In-memory hub single-instance-only; no metrics |
| AUDIT-1703 | CI | `cargo-audit` and `clippy` non-blocking (`|| true`) |
| AUDIT-1704 | CI | Actions pinned by tag not SHA; no `permissions:` block |

---

## 3. Top 10 Remaining Risks

| Rank | Audit ID | Severity | Finding | Recommended Action |
|---|---|---|---|---|
| 1 | AUDIT-101 | P1 | JWT claims trusted directly; DB RBAC unused at request time | Server-side `sub → user → roles → permissions` resolution; ignore claim permissions in jwt mode |
| 2 | AUDIT-202 | P1 | No facility scoping — cross-facility IDOR | Enforce `user_roles.facility_id` in every admin query; depends on AUDIT-101 |
| 3 | AUDIT-1004 | P1 | Check-in TOCTOU: non-atomic status transition + capacity count-then-insert | Atomic `UPDATE … WHERE status='scheduled' RETURNING`; single-tx check-in+ticket |
| 4 | AUDIT-701 | P0* | No backup/restore capability | pg_dump automation + restore drill; requires deployment decision |
| 5 | AUDIT-301 | P1 | Patient status enumeration oracle | Second factor (code+phone-last-4), failure counters, code expiry |
| 6 | AUDIT-302 | P1 | Check-in unlimited brute-force | Per-IP + per-appointment attempt limits with hard lock |
| 7 | AUDIT-801 | P1 | Default DB credentials in engine Dockerfile + main.rs | Remove baked-in DSN; require runtime injection |
| 8 | AUDIT-1102 | P1 | Request IDs implemented but never wired | Middleware injecting/generating request ID; feed to audit service |
| 9 | AUDIT-1701 | P1 | Integration/smoke tests never run in CI | CI job with Postgres service container; run handler integration tests |
| 10 | AUDIT-607 | P1 | Seeds contain DDL; demo IDs runtime-reachable | Env-guard seed execution; move ALTER to migration |

---

## 4. Maturity After Remediation

| Level | Before (original audit) | After PR #40–#51 |
|---|---|---|
| Demo | ~95% | ~95% (unchanged; all changes are behind flags) |
| Staging | ~45% | ~65% (+20pp) — env guards, TLS guard, migration runner, public catalog, security headers, trusted proxy, body limits, graceful shutdown all land |
| Production | ~25% | ~40% (+15pp) — same gains; backup/restore and RBAC resolution remain absolute blockers |

---

## 5. Recommended Next PR

**Title:** fix(api): atomic check-in transitions and rate-limited attempts

**Audit ID(s):** AUDIT-1004, AUDIT-302

**Why this one:**
1. **Data integrity**: Duplicate tickets per appointment and overbooked slots from TOCTOU races are correctness bugs, not just security issues.
2. **Security**: Unlimited brute-force on check-in code (6 chars = ~19B combinations) allows visit hijacking.
3. **Contained scope**: Fix is in `booking.go` CheckIn handler (~100 lines) — atomic UPDATE + rate limiter call. No cross-cutting concerns.
4. **Validated by existing tests**: Handler tests can be extended to verify atomic transitions and rate limiting.
5. **No architecture dependencies**: Unlike AUDIT-101/202 (RBAC), this fix is self-contained.

**Files likely affected:**
- `apps/api/internal/handler/booking.go` — atomic UPDATE RETURNING + limiter.Allow in CheckIn
- `apps/api/internal/handler/booking_test.go` — new tests for race conditions and rate limiting

**Acceptance criteria:**
- `UPDATE appointments SET status='checked_in' … WHERE id=$1 AND status='scheduled' RETURNING` — rows=0 returns 409
- Check-in code verification + status update in single query (eliminates separate SELECT)
- `h.limiter.Allow()` called on check-in path (per-IP + per-appointment)
- `go test ./...` green; existing tests unbroken

**Validation:**
- Unit tests for concurrent check-in attempts
- Rate limit test (rapid retries → 429)
- `go vet ./...` clean
- SvelteKit check clean

---

*Reconciliation performed at `d84fed9`. Original audit findings at `b47e07d` — see `docs/PRODUCTION_READINESS_AUDIT.md`.*
