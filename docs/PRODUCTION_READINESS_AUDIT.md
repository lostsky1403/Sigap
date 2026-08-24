# Sigap Production Readiness Audit

- Date: 2026-08-25
- Baseline: `main @ b47e07d` ("Add final demo runbook (#38)")
- Scope: full repository across 20 mandated domains (authentication through operations/runbooks)
- Method: read-only investigation; no source files modified during investigation. Evidence cites exact paths and current behavior. Unknowns are marked UNKNOWN / REQUIRES DEPLOYMENT DECISION.

Read-only validation executed at baseline:

| Command | Result |
|---|---|
| `go test ./...` (apps/api) | PASS |
| `govulncheck ./...` (apps/api) | PASS — 0 reachable vulnerabilities (1 unreachable advisory in required modules) |
| `cargo test` (apps/queue-engine) | PASS — unit + 2 integration tests (`concurrency_guardrail`, `estimated_wait_regression`) |
| `cargo audit` (apps/queue-engine) | PASS with 3 allow-listed warnings (anyhow unsound, event-listener unsound, spin yanked) |
| `pnpm --filter sigap-web run check` | PASS — 0 errors / 0 warnings |
| `gitleaks detect --source . --redact` | PASS — 114 commits scanned, no leaks |

---

## 1. Current Maturity

- Demo: ~95%. End-to-end stable (smoke 8/8, full local demo PASS). Excellent runbook culture.
- Staging: ~45%. Core flows work, but promotion is blocked by auth trust-model flaws, booking UI coupled to dev identity, no migration runner, env-var drift that silently disables audit in the compose deploy path, unenforced TLS, thin CI integration coverage, and public-endpoint abuse surfaces.
- Production: ~25%. Everything above plus zero backup/restore, no metrics/tracing/JSON logs/request correlation, no graceful shutdown, weak supply-chain gates, and missing operational runbooks. UNKNOWN / REQUIRES DEPLOYMENT DECISION: hosting model, managed Postgres vs self-managed, IdP choice for JWT issuance.

---

## 2. Top 10 Risks

| Rank | Severity | Domain | Finding | Staging blocker | Production blocker |
|---|---|---|---|---|---|
| 1 | P0* | Backup/Restore | No backup mechanism, no restore procedure, never tested; RPO ∞ / RTO unbounded (*latent until first real-data deployment) | No | Yes (absolute) |
| 2 | P1 | AuthN | Authorization trusts token `permissions` claim; DB RBAC tables unused at request time; no user-status check | Yes | Yes |
| 3 | P1 | AuthZ | No facility scoping — cross-facility IDOR incl. patient names + live check-in codes | Yes | Yes |
| 4 | P1 | AuthN | Dev auth mode has no environment guard; copied `.env` = anonymous superuser via header | Yes | Yes |
| 5 | P1 | Frontend | Public booking page fetches permission-gated admin endpoints; journey depends on dev identity | Yes | Yes |
| 6 | P1 | Database | No migration runner / applied-version tracking past 0001 | Yes | Yes |
| 7 | P1 | Config | Compose exports `DATABASE_URL` but Go reads `SIGAP_DATABASE_URL`; 1-click deploy boots audit-blind behind static `/health` | Yes | Yes |
| 8 | P1 | Privacy | Patient status lookup is an enumeration oracle; check-in unlimited attempts, no expiry | Yes | Yes |
| 9 | P1 | Validation | No body-size limits or HTTP server timeouts on unauthenticated endpoints | Yes | Yes |
| 10 | P1 | Network | Plain-HTTP listener; TLS termination relies entirely on undocumented reverse-proxy discipline | Yes | Yes |

---

## 3. Full Findings

### Domain 1 — Authentication

**AUDIT-101 · Token claims are the authorization source; DB RBAC is dead code** — P1
- Files: [jwt_provider.go](../apps/api/internal/auth/jwt_provider.go) L266–270; consumed at [authz.go](../apps/api/internal/identity/authz.go) L82.
- Current behavior: `Authenticate()` copies `claims.Permissions` into the Actor; `RequirePermission` exact-matches strings. No lookup against `roles`/`role_permissions`/`user_roles`; no `app_users.status`/`deleted_at` check.
- Risk: any issuer misconfiguration or leaked long-lived token yields full admin access; revocation latency = token TTL.
- Staging impact: masks missing role plumbing. Production impact: privilege forgery via token contents.
- Remediation: resolve `sub` → active user → roles → permissions server-side (short cache); ignore the claim in jwt mode. Scope M; deps none.

**AUDIT-102 · Dev auth mode lacks an environment guard** — P1
- Files: [config.go](../apps/api/internal/auth/config.go) L78–93; [factory.go](../apps/api/internal/auth/factory.go) L8–9; [dev_provider.go](../apps/api/internal/auth/dev_provider.go) L32–36, L39–70.
- Current behavior: dev mode selected purely from env vars; header `X-Sigap-Dev-User-ID` mints an actor with attacker-chosen UserID + 11 permissions; only signal is a per-request warn log; nothing refuses startup outside local.
- Staging impact: likely enabled for convenience → effectively unauthenticated staging. Production impact: catastrophic if leaked via copied `.env`.
- Remediation: fail startup when dev/dev-identity/demo-PHI/engine-fallback flags are set unless explicit `SIGAP_ENV=local`; strip dev paths from prod builds; CI lint deploy manifests. Scope S; deps none.

**AUDIT-103 · No max token lifetime; `exp` not required** — P2
- File: jwt_provider.go L235–242. Tokens without `exp`, or with very long validity, accepted.
- Remediation: `jwt.WithExpirationRequired()` + max-TTL validator (≤24h) + reject far-future iat. Scope S.

**AUDIT-104 · JWKS URL free-form; unknown-kid never triggers refresh; stale keys served indefinitely on refresh failure** — P2
- Files: config.go L65; jwt_provider.go L91–95, L98–133 (15-min TTL).
- Risk: rotation outages up to 15 min; prolonged acceptance of revoked-key signatures; mispointed JWKS neutralizes signature checking.
- Remediation: derive JWKS from issuer discovery; require https; single-flight refresh on unknown kid; cap stale window. Scope M.

**AUDIT-105 · `iss == "sigap-dev"` heuristic types tokens as dev** — P3 (latent; `IsDev` currently unchecked anywhere). Remove heuristic. Scope S.

Positive (INFO): alg allow-list blocks `none`/HMAC confusion twice ([L236](../apps/api/internal/auth/jwt_provider.go#L236), L277–279); iss/aud enforced; expired/wrong-sig → zero actor; deny-by-default router 401s undeclared routes ([router.go](../apps/api/internal/router/router.go) L94–112); `disabled` mode fails closed.

### Domain 2 — Authorization / RBAC

**AUDIT-201 · Empty-policy routes pass authorization by design** — P2
- Files: router.go L31–60 vs [authz.go](../apps/api/internal/identity/authz.go) L63–67. Public-by-intent routes exist (queues/generate, appointments, events/beds, facilities/nearby, patient/status) but nothing asserts intent explicitly.
- Risk: future endpoints silently ship ungated. Remediation: unit test asserting every route has non-empty policy or explicit `Public: true`; eventually deny empty. Scope S.

**AUDIT-202 · No facility scoping — cross-facility IDOR** — P1
- Files: authz.go L82 (permission-only); [admin.go](../apps/api/internal/handler/admin.go) L88–122 (all facilities), L489–513 (tickets by arbitrary `facility_id`), L1643–1667 (appointments — all facilities, includes `checkin_code`); [notifications.go](../apps/api/internal/handler/notifications.go) L105–113.
- Current behavior: migration 0003 models facility-scoped roles (`user_roles.facility_id`) but no request-time code reads them; handlers never intersect resource ↔ actor scope.
- Staging impact: cross-tenant visibility. Production impact: multi-tenant privacy breach (health data).
- Remediation: resolve actor facility scope server-side; enforce in every query (or Postgres RLS); reject out-of-scope rather than filter silently. Depends on AUDIT-101 work. Scope L.

**AUDIT-203 · Over-broad seed grants; duplicate divergent dev implementations** — P3 (P1-equivalent if demo-PHI reaches shared infra)
- Files: [rbac.sql](../packages/db/seed/rbac.sql) L37–57 (operator gets `facility.manage`; viewer reads appointments); [auth/dev_provider.go](../apps/api/internal/auth/dev_provider.go) L57–69 vs unwired [identity/dev_middleware.go](../apps/api/internal/identity/dev_middleware.go) L40–46.
- Remediation: strip `facility.manage` from operator; delete duplicate middleware; seed matrix tests. Scope S.

### Domain 3 — Public API / Privacy

**AUDIT-301 · Patient status lookup is an enumeration oracle** — P1
- Files: [patient.go](../apps/api/internal/handler/patient.go) L57–142 (public at router.go L37; proxied at [patient/status/+server.ts](../apps/web/src/routes/api/v1/patient/status/+server.ts) L21–25).
- Current behavior: matches case-insensitive `checkin_code` OR bare `formatted_number` (e.g. `PMI-0042`, ≈10⁴–10⁵ space/facility); distinct 404 vs 200 = clean oracle; historical rows matched regardless of status; codes never expire/rotate.
- Risk: harvested codes correlate citizen ↔ facility ↔ visit timeline.
- Staging: demonstrable. Production: tracking/doxxing vector; regulatory exposure.
- Remediation: second factor (code+phone-last-4), failure counters + lockout, hide terminal statuses after N days, stop bare-number lookups, uniform responses. Scope M.

**AUDIT-302 · Check-in: single-factor, unlimited attempts, no expiry** — P1
- Files: [booking.go](../apps/api/internal/handler/booking.go) L398–589 — **no `h.limiter.Allow(...)` anywhere in `CheckIn`** (unlike booking L90, status L70).
- Current behavior: knowledge of `(appointment UUID, 6-char code)` authorizes; wrong code → 401, unlimited retries; success consumes the visit.
- Risk: leaked appointment ID → unthrottled brute force hijacks visits; engine load amplification.
- Remediation: per-IP + per-appointment attempt limits with hard lock; generic errors; consider 8-char codes; alerting on repeated failures. Scope M.

**AUDIT-303 · Admin appointment list ships live check-in codes + names in bulk** — P2
- Files: admin.go L1584–1598 (JSON tag exposes code), L1643–1667 (unpaginated).
- Remediation: mask in list responses; strict-permission single-item reveal; paginate. Scope S–M.

**AUDIT-304 · Booking under arbitrary third-party phone numbers** — P2
- Files: booking.go L67–214, fan-out L301–345. Anyone books with any phone; confirmation (with code) enqueued to that number; quota burnable by attackers.
- Remediation: OTP proof-of-possession or pending-verification state. Scope M.

### Domain 4 — Rate Limiting / Abuse

**AUDIT-401 · Shared-bucket lockout behind proxy; spoofable helper as latent trap** — P1
- Files: [limiter.go](../apps/api/internal/limiter/limiter.go); patient limiter keyed on `RemoteAddr` (patient.go L70, L145–151); web proxies forward no client headers → all users share one bucket (30 req/min site-wide on patient status); dead-code `clientIP()` ([queue.go](../apps/api/internal/handler/queue.go) L129–141) blindly trusts `X-Forwarded-For`.
- Remediation: forward real client IP via overwritten internal header from the SvelteKit hop; delete/gate `clientIP()`. Scope M.

**AUDIT-402 · In-memory limiter: non-distributed, reset-on-restart, unbounded map growth** — P2
- Files: limiter.go L18–63, L42–63 (entries never deleted despite comment), L79–81 (25h windows).
- Impact: N replicas ⇒ limit×N; deploys wipe counters; slow memory growth → OOM pressure.
- Remediation: Redis/Postgres-backed atomic counters; periodic sweep; Asia/Jakarta date keys. Scope M.

**AUDIT-403 · Booking quota phone-key-only; comment/code mismatch** — P2
- Files: booking.go L88–93 (limit 2/day per phone) vs comment claiming 3.
- Positive contrast: `/queues/generate` keys phone+facility+date — immune to shared-WiFi collisions (queue.go L53–62).
- Remediation: layered per-IP hourly + per-phone daily; align constants; captcha on public form. Scope S–M.

### Domain 5 — Queue Engine Reliability

**AUDIT-501 · gRPC client lacks timeout/retry/keepalive; connection never closed** — P2
- Files: [grpc/client.go](../apps/api/internal/grpc/client.go) L38–47 (insecure default with loud warning — good), L52–53 (conn intentionally unclosed), Probe L90–112.
- Impact: transient engine blips become user-facing 500s; leaked TCP conns.
- Remediation: per-call deadline (3–5s), retry w/ backoff on Unavailable, keepalive, `Close()` in shutdown. Scope S–M.

**AUDIT-502 · Dev fallback can serve non-dev traffic if env leaks** — P2 (boot-guard family, see AUDIT-102)
- Files: [main.go](../apps/api/cmd/server/main.go) L78–91; fallback writes real ticket rows without the engine ([booking.go](../apps/api/internal/handler/booking.go) L505–546, fixed DOB `2000-01-01` upsert keyed by phone).
- Fail-closed otherwise (`os.Exit(1)` when unreachable without fallback) — correct directionally.
- Split-brain: none observed — numbering authority solely in the engine tx (Domain 14). Reconnect behavior beyond Probe: UNKNOWN / REQUIRES DEPLOYMENT DECISION (no soak evidence in repo).

### Domain 6 — Database / Migrations

**AUDIT-601 · No migration runner, no applied-version tracking** — P1
- Files: [Makefile](../Makefile) L32–36 (`db-migrate` applies only 0001); [docker-compose.yml](../docker-compose.yml) L55–58 (init-dir mount works only on fresh volumes); [DEV_SETUP.md](./DEV_SETUP.md) L574–586 (manual 0006 application).
- Impact: partial application undetectable; cannot answer "what version is this DB?"; silent missing-table failures under load.
- Remediation: adopt tracked migrator (goose/golang-migrate/dbmate); interim `make db-migrate-all`; CI fresh-schema convergence job. Scope M.

**AUDIT-602 · Migrations 0001–0005 not transactional/idempotent (0006 is)** — P2
- Files: e.g. [0001_init.sql](../packages/db/migrations/0001_init.sql) L1–60 bare DDL vs [0006_notifications.sql](../packages/db/migrations/0006_notifications.sql) L28–151 (BEGIN…COMMIT + IF NOT EXISTS).
- Remediation: wrap in transactions; idempotent forms. Scope S–M.

**AUDIT-603 · No rollback strategy; "forward-only" asserted, unenforced** — P1 (compounds AUDIT-701)
- Evidence: migration headers only; no down files anywhere.
- Remediation: document rollback contract (= PITR once backups exist); down-migrations for additive changes. Scope S (doc)/M (tooling).

**AUDIT-604 · Missing UNIQUE constraints on `facilities.short_code` and `appointments.checkin_code`** — P2 (P1 for production check-in correctness)
- Files: [0001_init.sql](../packages/db/migrations/0001_init.sql) L22; [0005_appointments.sql](../packages/db/migrations/0005_appointments.sql) L57/L80; seeds ship defensive repairs proving duplicates occurred ([demo.sql](../packages/db/seed/demo.sql) L50–55, L302–308).
- Remediation: dedupe then unique indexes (partial `WHERE is_active` if needed). Scope S.

**AUDIT-605 · Env-var drift: compose exports `DATABASE_URL`, Go reads `SIGAP_DATABASE_URL`** — P1
- Files: docker-compose.yml L96–101 vs [server/main.go](../apps/api/cmd/server/main.go) L99–121 and [notification-worker/main.go](../apps/api/cmd/notification-worker/main.go) L72–75; `.env.example` defines only `DATABASE_URL`.
- Current behavior: 1-click stack boots API with no DB pool → audit silently off, admin nil; `/health` still 200 (static).
- Remediation: align names (or accept both); promote disabled-DB log to error; fold DB ping into `/readyz`. Scope S.

**AUDIT-606 · Pool defaults; no timeouts; hardcoded fallback DSN in engine binary** — P2
- Files: server/main.go L100–113; worker L77–81; [queue-engine main.rs](../apps/queue-engine/src/main.rs) L37–40.
- Remediation: explicit pool config knobs via env; connect/statement timeouts; delete fallback DSN. Scope S–M.

**AUDIT-607 · Seeds contain DDL + UPDATEs; deterministic demo IDs runtime-reachable** — P1-if-ever-run-outside-dev (guarded today by convention only)
- Files: demo.sql L39 (ALTER TABLE in a seed!), L119–126/L285–294/L304–308 (UPDATEs contradicting own header), L332–360 (`SMOKE01`, phone `085550000001`); dev.sql L31–37 (synthetic user `d999`); smoke constants [sigap-demo-smoke.ps1](../scripts/smoke/sigap-demo-smoke.ps1) L82–96.
- Remediation: move ALTER into numbered migration; env-guard seeds (`SIGAP_ENV != local` refuses). Scope S.

### Domain 7 — Backup / Restore

**AUDIT-701 · No backup mechanism, no restore procedure, never tested; RPO/RTO undefined** — **P0*** (activates at first real-data deployment)
- Evidence of absence: repo grep hits only [ROADMAP.md](../ROADMAP.md) L191 (Phase 10 backlog); single unnamed volume `pgdata`, no WAL archiving; no Makefile targets; no drill in CI/smoke.
- Effective RPO ∞; RTO = full rebuild. `docker compose down -v` or disk loss permanently destroys patients/appointments/medical_records/audit_events.
- Staging: tolerable (disposable data). Production: absolute blocker; retention-law exposure.
- Remediation: nightly encrypted `pg_dump` + WAL archiving or managed Postgres PITR; scripted restore + monthly scratch-DB drill; declare RPO ≤5 min / RTO ≤1 h as testable criteria. Mechanism UNKNOWN / REQUIRES DEPLOYMENT DECISION (hosting model). Scope M.

**AUDIT-702 · No retention/archival plan for `audit_events`** — P2
- Files: [0004_audit_events.sql](../packages/db/migrations/0004_audit_events.sql) L8–24 (append-only, six indexes, unbounded); ROADMAP Phase 11 placeholder.
- Remediation: monthly partitions; regulation-aligned retention tiers. Scope M.

### Domain 8 — Config / Secrets

**AUDIT-801 · Default DB credentials baked into engine image and binary** — P1
- Files: [queue-engine Dockerfile](../apps/queue-engine/Dockerfile) L38 (`ENV DATABASE_URL=postgresql://sigap:sigap@...`); main.rs L37–38 fallback DSN.
- Impact: `docker history` reveals credentials; standalone runs use baked-in defaults.
- Remediation: remove both; require runtime injection (matches Go API pattern). Scope S.

**AUDIT-802 · `SIGAP_AUTH_MODE` unset-defaults to `disabled`; compose lacks required-var guard** — P2
- Files: config.go L24; compose api service (var absent). Forgotten var → locked-but-running API (availability footgun; fail-closed direction correct).
- Remediation: `${SIGAP_AUTH_MODE:?}` interpolation in compose; boot banner. Scope S.

**AUDIT-803 · `SIGAP_ENABLE_DEMO_PHI=true` serves fabricated PHI with zero authentication** — P1
- Files: server/main.go L259–278 (env-check guard; default 404 fail-closed), handler L356–375.
- Remediation: include in `SIGAP_ENV` boot-guard; delete demo handlers before production rather than trusting flags. Scope S.

Positive (INFO): `.env` gitignored with safe placeholder defaults ([.env.example](../.env.example) L7); bootstrap admin gated by `SIGAP_BOOTSTRAP_ADMIN=true`, idempotent; gitleaks clean across 114 commits.

### Domain 9 — Network / TLS / CORS / CSRF

**AUDIT-901 · Plain HTTP listener; TLS unenforced** — P1
- Files: server/main.go L251; SECURITY.md instructs reverse-proxy TLS but nothing verifies it.
- Remediation: staging profile mandates terminating proxy; optional boot assertion requiring explicit `SIGAP_TLS_TERMINATED=true` outside local. Scope S + deployment decision.

**AUDIT-902 · CORS loopback exception survives production origins** — P2
- File: server/main.go L39–42. `http://127.0.0.1:3005` always allowed even when `SIGAP_WEB_ORIGIN=https://app.example.com`.
- Remediation: parenthesize and restrict exception to loopback deployments. Scope S.

**AUDIT-903 · SSE endpoint hardcodes `Access-Control-Allow-Origin: *`** — P2
- File: [events/hub.go](../apps/api/internal/events/hub.go) L63. Any website can subscribe to operational bed/queue events.
- Remediation: delegate to shared CORS wrapper. Scope S.

**AUDIT-904 · CSRF posture** — P3 documentation. Header-based auth + no cookies ⇒ not exploitable today; becomes critical if cookie sessions land. Document in SECURITY.md.

**AUDIT-905 · No security headers anywhere (API or web)** — P2
- Evidence: no header middleware; no `hooks.server.ts`; zero CSP/XFO/HSTS/XCTO matches in apps/web.
- Risk: clickjacking of admin state transitions; XSS without containment; MIME sniffing.
- Remediation: `hooks.server.ts` (CSP `default-src 'self'`, relaxed connect-src for SSE/map tiles, `frame-ancestors 'none'`) + API baseline headers; verify SSE streaming + MapLibre workers. Scope M.

**AUDIT-906 · No trusted-proxy header handling** — P2. Audit/logs lose real client IPs; interlocks with AUDIT-401. Define exactly one trusted hop; overwrite forwarding headers. Scope S–M.

**AUDIT-907 · gRPC transport insecure by default** — P2 (acceptable single-host; exposure multi-node). grpc/client.go L38–47 warns loudly. mTLS/mesh for K8s. Scope M.

### Domain 10 — Input Validation / Error Handling

**AUDIT-1001 · No body-size limits, no HTTP server timeouts** — P1
- Files: server/main.go L251 (no ReadHeaderTimeout/ReadTimeout/WriteTimeout/MaxHeaderBytes); raw body decodes booking L73–77, queue L42–45, admin L171–176; uncapped `request.text()` in every web proxy.
- Risk: trivial memory/CPU exhaustion + slowloris on unauthenticated endpoints.
- Remediation: `http.MaxBytesReader` (64KB public/256KB admin) + configured `http.Server`; mirror cap in proxies. Scope S.

**AUDIT-1002 · SSE event JSON assembled via `fmt.Sprintf` with unvalidated `facility_id`** — P2
- File: queue.go L97; `/queues/generate` checks presence only (L48), no `uuid.Parse` (booking parses at L218–227).
- Risk: event-stream poisoning — injected members drive false dashboard state.
- Remediation: `uuid.Parse` + emit via `json.Marshal` on typed struct. Scope S.

**AUDIT-1003 · `/queues/generate` skips format/length/normalization validation** — P2
- Files: queue.go L41–68, L118–126. Raw phone becomes limiter key (format variants bypass quota); oversized fields fail late with opaque 500s.
- Remediation: reuse booking validators (uuid.Parse, normalizePhone 10–15 digits, name ≤100, gender enum, strict DOB parse). Scope S.

**AUDIT-1004 · Check-in/capacity TOCTOU races (non-atomic read-then-write)** — P1
- Files: booking.go L437–454 (status read), L470–476 (update lacks `AND status='scheduled'`), L564–570 (ticket attach), L149–164 (count-then-insert capacity).
- Impact: duplicate tickets per appointment; overbooked slots; orphaned tickets.
- Strong contrast: Rust engine does this right — single tx + `SELECT … FOR UPDATE` counter ([engine/queue.rs](../apps/queue-engine/src/engine/queue.rs) L35–111, L170–172) with passing concurrency test.
- Remediation: guarded transition `UPDATE … WHERE id=$1 AND status='scheduled' RETURNING` (rows=0 ⇒ 409); one tx for check-in+ticket; lock schedule row or per-slot uniqueness for capacity. Scope M.

**AUDIT-1005 · Length-cap gaps** — P2. `notes` unlimited (booking.go L55, insert L177–183); facility text fields uncapped (admin.go L382–433); gender/DOB free-text in queue flow. Mirror schema widths; field-level 400s. Scope S.

**AUDIT-1006 · Status/log hygiene cluster** — P3. Wrong-code check-in returns 401 on public route + echoes internal status enums (booking.go L452, L465); booking success returns 200 not 201 (L195–203); DB driver error strings persisted into audit `detail` (admin.go L100, L155; sanitizer filters keys only — [audit/service.go](../apps/api/internal/audit/service.go) L220–227); non-sargable `LOWER(col)=LOWER($1)` lookups (patient.go L92, L109); most admin `[id]` proxy params lack `encodeURIComponent`.

Verified clean (INFO): malformed JSON → uniform generic 400s everywhere; **no SQL injection paths** (parameterized `$n` throughout; dynamic SET concatenates only hardcoded column names; enums whitelisted for transitions); no stack traces/raw internal errors to clients; UUID parsing consistent on `{id}` paths except AUDIT-1002/1003.

### Domain 11 — Logging / Audit

**AUDIT-1101 · No JSON log handler** — P2. No `NewJSONHandler`/`SetDefault` anywhere; default text slog degrades aggregation/alerting. Add JSON handler + `SIGAP_LOG_LEVEL`. Scope S.

**AUDIT-1102 · Request IDs implemented but never injected** — P1
- Files: [request_id.go](../apps/api/internal/identity/request_id.go) L14–39 (implemented + tested) vs middleware chain that never calls it → `RequestIDFromContext` always `""`; audit rows store empty request_id.
- Impact: impossible to correlate logs ↔ audit ↔ requests (compliance gap).
- Remediation: middleware honoring `X-Request-ID`/generating; echo response header; feed audit service. Scope S.

**AUDIT-1103 · Metadata sanitization filters keys only; nested maps leak; values carry DB error strings** — P2. service.go L218–260 (nested limitation acknowledged in service_test.go L65–73); IP/UA SHA-256 hashed before storage (good). Recursive sanitize or reject non-flat metadata. Scope S.

**AUDIT-1104 · Audit coverage gaps; old/new state unstructured** — P2
- Covered: queue.generate/list/status_updated, all admin CRUD, notification ops, authz.denied. Missing: authn.success/authn.failed, patient-status lookups, SSE subscribes, bootstrap creation. Old→new only as free-form `"old->new"` string (admin.go L1760–1768).
- Remediation: add authn events; structured old/new JSON fields. Scope M.

**AUDIT-1105 · `audit_events` lacks DB-level write protection despite hash chain** — P2 (P1 for regulated production)
- File: 0004_audit_events.sql — chain columns exist; no trigger/REVOKE preventing UPDATE/DELETE; no chain-verification function; concurrent-writer gaps acknowledged (service.go L157).
- Remediation: anti-mutation trigger; `verify_audit_chain()`; REVOKE from app role. Scope M.

### Domain 12 — Observability

**AUDIT-1201 · `/health` static liveness probe** — INFO, correct design (server/main.go L134–138).

**AUDIT-1202 · `/readyz` checks only the engine, not the database** — P1
- File: server/main.go L140–152 (3s `svc.Probe` only).
- Impact: LB routes traffic to pods whose DB-backed endpoints all fail → cascading 500s.
- Remediation: `dbPool.Ping(ctx)` in readiness; surface audit-enabled state. Scope S.

**AUDIT-1203 · No metrics endpoint; no Prometheus/OTel SDK** — P1
- Evidence: no client_golang imports, no `/metrics`, no OTel anywhere.
- Impact: no quantitative visibility into rates/errors/latency/outbox depth/SSE clients; every reliability effort blind.
- Remediation: instrument HTTP by route/status, gRPC probe latency, outbox depth by status, SSE gauges; expose `/metrics`. Scope M.

**AUDIT-1204 · No distributed tracing** — P2. Cross-service latency analysis impossible; interlocks with AUDIT-1102. OTel SDK + gRPC propagation. Scope M.

### Domain 13 — Notifications

**AUDIT-1301 · Only a simulated provider exists** — INFO/by-design. [provider.go](../apps/api/internal/notification/provider.go) L21–176 (deterministic fnv32a simulation, zero sockets). Real delivery impossible until vendors are implemented behind the existing clean interface; secrets must come from env/vault. Scope per vendor M.

**AUDIT-1302 · Enqueue has no dedup; terminal failures accumulate; worker invisible externally** — P2 (each)
- Files: worker.go L215–262 — claim uses `FOR UPDATE SKIP LOCKED` (correct), crash recovery via 15-min safety window (correct), MaxAttempts=3 fixed backoff 1/5/15 min; no dedup on `(contact_hash, template_key, related_resource)`; `failed` rows never cleaned; no dead-letter distinction; no worker health/metrics ([worker/main.go](../apps/api/cmd/notification-worker/main.go) L55–138); worker absent from compose ([DEV_SETUP.md](./DEV_SETUP.md) L621–626) — a deploy forgetting it silently stops all notifications.
- Remediation: partial unique index for pending/processing dedup; archive job + terminal-failure alert; heartbeat/metrics; include worker in deploy topology. Scope M.

### Domain 14 — Concurrency / Consistency

Strong (INFO): queue numbering authoritative and atomic in the engine (FOR UPDATE counter; single transaction spanning ticket + signed medical record; concurrency integration test green locally and in CI). Outbox claiming `SKIP LOCKED`. Admin status transitions enum-whitelisted (admin.go L682–689, L1606–1613).

Weak spots already filed: AUDIT-1004 (double check-in/ticket duplication), booking capacity count-then-insert (same remediation), notification enqueue dedup (AUDIT-1302).

### Domain 15 — SSE / Realtime

**AUDIT-1501 · In-memory hub is single-instance-only; no metrics; upstream fetch unhandled** — P2
- Files: [hub.go](../apps/api/internal/events/hub.go); web relay [events/beds/+server.ts](../apps/web/src/routes/api/v1/events/beds/+server.ts) (fetch without try/catch → generic 500 on API restart, P3).
- Multi-instance deploys silently partition subscribers; no client/drop gauges; wildcard CORS (AUDIT-903).
- Privacy positive (INFO): payloads are aggregate counts, non-PHI (verified).
- Remediation: document single-instance constraint for staging; LISTEN/NOTIFY or pub/sub backbone for multi-instance; gauges; try/catch + 502 retry guidance. Scope M.

### Domain 16 — Deployment

Strong (INFO): all three images non-root (`sigap` / uid 10001 / `node`); multi-stage minimal builds; `restart: unless-stopped`; healthchecks on postgres/engine/api; named volume persistence; api waits `condition: service_healthy` on postgres.

**AUDIT-1601 · No graceful shutdown in Go API** — P1
- File: server/main.go L251. SIGTERM kills in-flight bookings/check-ins; gRPC conn never closed (client.go L52–53). Notification worker does this correctly (`signal.NotifyContext`) — INFO-positive contrast.
- Impact: aborted transactions, inconsistent states on every deploy/restart.
- Remediation: `http.Server` + `signal.NotifyContext` + `Shutdown(ctx)` + close pool/gRPC conn (~20 lines). Scope S.

**AUDIT-1602 · Web waits on `service_started` not `service_healthy`; no web healthcheck** — P2/P3
- File: docker-compose.yml L131 (web.depends_on), web service block (no healthcheck). First requests can hit a still-booting API; orchestrators cannot detect web crashes.
- Remediation: `condition: service_healthy`; add web healthcheck (+ tiny `/health` route). Scope S.

**AUDIT-1603 · Host port exposure: Postgres 5433, gRPC 50051, API 8080** — P2
- File: docker-compose.yml L54, L82, L108. SECURITY.md itself says never expose 50051 publicly.
- Remediation: `expose` instead of `ports` for non-staging profiles; reverse proxy fronts API only. Scope S.

**AUDIT-1604 · Postgres TLS is self-signed dev cert; clients use `sslmode=require`** — P2. docker-compose.yml L18–48 (encrypt-without-authentication). Production must use CA cert + `verify-full` — REQUIRES DEPLOYMENT DECISION. Scope S (config).

### Domain 17 — CI/CD / Supply Chain

Current pipeline ([ci.yml](../.github/workflows/ci.yml)): Go tests + router-coverage ≥90% gate; Rust test+clippy; svelte-check; blocking govulncheck; cargo-audit **non-blocking**; blocking gitleaks. Trigger: push/PR to main.

**AUDIT-1701 · Integration/smoke suites never run in CI** — P1
- Files: ci.yml (no Postgres service container; web job runs only svelte-check, never `pnpm test`; engine integration tests self-skip without DB); smoke suites (22 assertions) run only on developer machines.
- Impact: merges pass green while the end-to-end booking→check-in→queue→notification path may be broken; "demo-green" enforced manually.
- Remediation: integration job with `services: postgres:16`; run full local demo suite (or bash port); web job runs `pnpm --filter sigap-web test`. Scope M.

**AUDIT-1702 · Migration testing absent; 0002–0006 have no scripted application path** — P1 (see AUDIT-601). CI step: apply all migrations to ephemeral Postgres, run handler tests. Scope M.

**AUDIT-1703 · cargo-audit and clippy non-blocking (`|| true`)** — P1
- Files: ci.yml L64, L119, L124; Makefile L73. New RustSec advisories/clippy warnings cannot fail CI.
- Local evidence: repo currently passes both clean (cargo audit exit 0 with allow-listed warnings).
- Remediation: remove `|| true`; pin cargo-audit; `--locked`. Scope S.

**AUDIT-1704 · Actions pinned by tag not SHA; mutable scanner install; no `permissions:` restriction** — P2
- Files: ci.yml L14–135 (`@v4/@v5/@stable`, `govulncheck@latest`); no top-level permissions block.
- Remediation: SHA-pin everything; `permissions: { contents: read }`; pin govulncheck version. Scope S.

**AUDIT-1705 · No dependency update tooling (Renovate/Dependabot absent)** — P2. Zero config matches repo-wide. Enable Dependabot/Renovate (gomod, cargo, npm ecosystems, weekly). Scope S.

**AUDIT-1706 · No image builds, SBOM, or container scanning in pipeline** — P2 (P1 once any real deployment target exists). Dockerfiles never built/scanned by CI. Remediation: build-scan job (Syft SBOM, Trivy fail-on-HIGH, GHCR push tagged by SHA). Scope M.

**AUDIT-1707 · npm ecosystem unaudited** — P2. No `pnpm audit`/osv-scanner anywhere; caret ranges (maplibre-gl, vite, svelte) unmonitored. Add audit job. Scope S.

Positive (INFO): router-coverage gate on the security seam; Go pinned via `go-version-file`; frozen-lockfile install in CI; Cargo.lock committed; gitleaks full-history blocking; truthful PASS/FAIL semantics in `make security`.

**AUDIT-1708 · Doc/toolchain drift** — P3. Reports cite go 1.25.x; actual [go.mod](../apps/api/go.mod) is 1.26.6; smoke README describes 6-step suite while script is 8 steps. Mark historical docs as such. Scope S.

**AUDIT-1709 · Duplicate CODEOWNERS** (.github/ vs root, contents diverge) — P3. Keep `.github/` copy only.

**AUDIT-1710 · No deployment stage/environments/release artifacts** — P2 structural. Expected at this phase; required for staging (environment protection + post-deploy smoke using the patient-portal suite, which needs no dev identity).

### Domain 18 — Frontend Production Readiness

**AUDIT-1801 · Public booking page calls permission-gated admin endpoints** — P1
- Files: [appointments/new/+page.svelte](../apps/web/src/routes/appointments/new/+page.svelte) L58–61 fetches `/api/v1/admin/facilities` and `/api/v1/admin/service-units` (policies `facility.read`/`schedule.read`, router.go L40/L47); silent catch at L86–88 falls back to manual UUID entry.
- Works today **only** because proxies self-inject the dev identity header (see AUDIT-1802). Under JWT auth the dropdowns die — the flagship patient journey is structurally coupled to dev-only authentication.
- Remediation: public catalog endpoints (`GET /api/v1/facilities`, `/api/v1/service-units?facility_id=`) returning id/name/code only; repoint page; keep admin lists privileged. Scope M.

**AUDIT-1802 · Dev-identity header injection embedded in ~17 proxy modules (64 occurrences)** — P1 pre-staging-gate item
- Representative: [facilities/+server.ts](../apps/web/src/routes/api/v1/admin/facilities/+server.ts) L3–10. Gate `SIGAP_DEV_IDENTITY === 'true'` mirrors the API's fail-closed check — correct behavior today, pervasive coupling.
- Remediation: centralize in `$lib/server/auth.ts` with startup throw if enabled in production builds; replace with session cookie → JWT relay later. Scope S–M.

**AUDIT-1803 · Zero security headers (shared with AUDIT-905)** — P1 for sign-off. See Domain 9 remediation.

**AUDIT-1804 · adapter-node chosen correctly; build reproducibility weak** — P2
- Files: [svelte.config.js](../apps/web/svelte.config.js) L8 (adapter-node ✅); [web Dockerfile](../apps/web/Dockerfile) L10–11 (`pnpm-lock.yaml*` glob makes lockfile optional; plain `pnpm install`), L24–27 (devDeps shipped to runtime). `USER node` ✅.
- Remediation: mandatory lockfile copy + `--frozen-lockfile`; prune devDeps. Scope S.

**AUDIT-1805 · Proxy target falls back to Docker hostname `http://api:8080` outside Docker** — P2. Every proxy: `process.env.SIGAP_API_INTERNAL || 'http://api:8080'`. Bare-metal deploys get DNS failures everywhere. Remove fallback; fail fast at boot. Scope S.

**AUDIT-1806 · No root `+error.svelte`; SSE relay lacks upstream error handling** — P3. Per-page loading/error states otherwise well covered. Add global error boundary; wrap SSE fetch with 502 + retry guidance.

**AUDIT-1807 · Environment separation gaps** — P2. Compose api service lacks `SIGAP_WEB_ORIGIN` (CORS won't match a real domain); stale `PUBLIC_SIGAP_API_BASE` kept "for reference". Create `docker-compose.staging.yml`: `SIGAP_AUTH_MODE=jwt`, real origin, TLS proxy, fallback/demo flags absent.

### Domain 19 — Demo-Only Couplings Inventory

| # | Occurrence | Key evidence | Classification |
|---|---|---|---|
| 1 | Dev identity provider + header injection | dev_provider.go L12–71; all web proxies | Code REMOVE BEFORE PRODUCTION (build-strip); flag activation REMOVE BEFORE STAGING |
| 2 | Engine fallback `SIGAP_ENGINE_FALLBACK=dev` | server/main.go L78–91; Start-LocalDev.ps1 forces ON | Flag REMOVE BEFORE STAGING; code SAFE DEV-ONLY (fail-closed) |
| 3 | Demo PHI gate + fake records | server/main.go L259–278, L356–375 | REMOVE BEFORE PRODUCTION (delete handlers); default-closed ✅ |
| 4 | Hardcoded nearby-facilities list f1…f6 | server/main.go L303–310, served on public route | REMOVE BEFORE STAGING (misleading operational data, public endpoint) |
| 5 | Chaos load-test button in dashboard | BedAvailabilityDashboard.svelte L100–137 (bulk generate triggering 429s) | REMOVE BEFORE STAGING |
| 6 | Geolocation mock fallback (Jakarta coords) | BedAvailabilityDashboard.svelte L142–158 | SAFE DEV-ONLY (legitimate UX fallback; document) |
| 7 | Deterministic demo seeds d000–d999/e000–e005 | demo.sql, dev.sql | SAFE DEV-ONLY files; data REMOVE BEFORE STAGING (never execute on shared DBs; env-guard per AUDIT-607) |
| 8 | Reserved ITU-T test phones +62-555-01xx | demo.sql L32; README; runbook | SAFE DEV-ONLY. Nuance: presenter phone `08555001999` normalizes onto a real Indonesian mobile prefix — swap to 555 range (P3) |
| 9 | SMOKE01 sentinel appointment | demo.sql L302–346; portal smoke Step 2 | SAFE DEV-ONLY |
| 10 | Smoke-script fallback IDs d999/d001/d000/d021 | sigap-demo-smoke.ps1 L82–96 | SAFE DEV-ONLY (well-commented tier logic) |
| 11 | Offline deterministic notification provider | provider.go L19–176 | SAFE DEV-ONLY (only provider shipped; vendor seam deferred by design) |
| 12 | Wallet UI coupled to demo-PHI endpoint | wallet/+page.svelte L15 (404s when flag off) | REMOVE BEFORE STAGING (hide route or wire to real authenticated records) |
| 13 | Test synthetic identities | *_test.go fixtures | SAFE DEV-ONLY (contained) |
| 14 | MVP disclaimers in layout/footer | +layout.svelte L10–28; app.html | SAFE DEV-ONLY (honest signaling) |

Net: no demo coupling is reachable in a default production configuration — every dangerous path requires explicit `"true"` env vars and the deny-by-default router blocks undeclared routes. Residual risk is operational (flags) plus functional (AUDIT-1801 makes demo couplings load-bearing for booking).

### Domain 20 — Operations / Runbooks

Coverage matrix:

| Scenario | Status | Evidence | Gap severity |
|---|---|---|---|
| Startup/shutdown (local) | Excellent | Start-LocalDev.ps1; make dev targets; runbook §1 | Prod lifecycle docs MISSING (P2) |
| Health/readiness | Good | `/health`+`/readyz` procedures; compose healthchecks | No monitoring/on-call wiring (P2); readyz gap AUDIT-1202 |
| Migration execution | Partial | forward-only policy stated | Runner absent (AUDIT-601) — P1 |
| Backup/restore | Absent | only destructive volume-reset documented; ROADMAP Phase 10 | P1 |
| DB outage | Partial | readyz reflects DB; nil-safe skip | reconnect/data-integrity procedure MISSING (P2) |
| Engine outage | Good (local) | dedicated runbook row; fail-fast default correct | prod paragraph needed (P3) |
| Notification outage | Best-covered | full ops runbook: dry-run preview, summary endpoint, retry/cancel APIs, troubleshooting matrix | worker absent from compose topology (P2) |
| Credential rotation | Advice only | SECURITY.md one-liners | procedure MISSING (P2; P1 at first real deployment) |
| Incident response | Disclosure process only | SECURITY.md SLAs (5-day ack/30-day fix) | operational IR runbook MISSING (P2) |

Positive discipline worth extending: runbook carries source-of-truth governance, dual-language sync rule, "last verified @ commit" stamping; DEMO_FLOW.md properly retired as redirect stub; notification dry-run is verified no-mutation.

---

## 4. What Is Already Good

Verified strengths (evidence-backed):

1. Deny-by-default route registry returning 401 for undeclared routes, with per-route policy registry and router-coverage CI gate ≥90% ([router.go](../apps/api/internal/router/router.go), ci.yml).
2. Fail-closed posture everywhere: `disabled` auth mode locks all gated routes; engine-unreachable exits unless fallback explicitly set; demo PHI defaults to 404; JWT wrong-sig/expired → zero actor, not panic.
3. JWT signature hygiene: double alg confusion defense (allow-list + keyFunc), iss/aud enforcement, comprehensive provider tests.
4. Rust queue engine correctness: FOR UPDATE counter, single atomic transaction spanning ticket + signed record, passing concurrency integration test in CI.
5. Outbox pattern done right: `FOR UPDATE SKIP LOCKED` claiming, crash-recovery safety window, masked contacts + SHA-256 hashes, DB CHECK constraints forbidding raw phone digits in message content.
6. Tamper-evident audit foundation: hash chain columns + sanitized metadata key redaction + hashed IP/UA.
7. Supply-chain basics: blocking govulncheck + gitleaks full-history; Cargo.lock committed; frozen-lockfile CI installs; zero leaks across 114 commits.
8. Deployment hygiene: all containers non-root, multi-stage minimal images, healthchecks on 3/4 services, named-volume persistence, restart policies.
9. No SQL injection paths; uniform generic error messages; consistent UUID parsing on `{id}` routes.
10. Documentation culture: honest SECURITY.md limitations, dual-language runbook with verification stamps, retired-doc stubbing, reserved ITU-T phone ranges for synthetic data.
11. All six permitted validations green at baseline.

---

## 5. Environment Matrix

| Capability | Dev | Staging | Production |
|---|---|---|---|
| Authentication | SUPPORTED (`dev` mode + header identity) | REQUIRED (`jwt` mode; currently DEV ONLY path) | REQUIRED (`jwt`; claims-trust fix AUDIT-101 prerequisite) |
| Dev identity | SUPPORTED (env-gated) | DEV ONLY — flag must be unset | MISSING by requirement (build-strip recommended) |
| Engine fallback | SUPPORTED (`SIGAP_ENGINE_FALLBACK=dev`) | DEV ONLY — absent | DEV ONLY — absent; fail-fast instead |
| Notification provider | SUPPORTED (simulated DevProvider) | SUPPORTED (simulated) | MISSING (real vendors not implemented) |
| Seeds | SUPPORTED (demo/dev/rbac) | DEV ONLY (must not execute) | MISSING (bootstrap-admin only) |
| TLS | SUPPORTED (Postgres sslmode=require, self-signed; app plain HTTP behind nothing) | REQUIRED (terminating proxy; currently MISSING) | REQUIRED (proxy + CA certs; `verify-full` DB) |
| Logging | SUPPORTED (text slog) | PARTIAL (text; JSON handler MISSING) | REQUIRED (JSON + request IDs; currently MISSING) |
| Metrics | MISSING | MISSING | REQUIRED (none exists) |
| Backups | MISSING | MISSING | REQUIRED (absolute production blocker) |

---

## 6. Roadmap

### PHASE 0 — Critical blockers (before any shared environment)

1. **PR: "feat(api): environment guards for dev-only capabilities"**
   - Goal: fail startup when `SIGAP_AUTH_MODE=dev`, `SIGAP_DEV_IDENTITY=true`, `SIGAP_ENABLE_DEMO_PHI=true`, or `SIGAP_ENGINE_FALLBACK=dev` appear without explicit `SIGAP_ENV=local`.
   - Files: apps/api/internal/auth/config.go, cmd/server/main.go, cmd/notification-worker/main.go (+tests).
   - Addresses: AUDIT-102, 502, 803. Validation: new unit tests + manual negative boots; `go test ./...`.
   - Deps: none. ← Recommended next PR (see §7).

2. **PR: "fix(compose): align database env var and harden compose defaults"**
   - Goal: export `SIGAP_DATABASE_URL` (or accept both in Go); `${SIGAP_AUTH_MODE:?}` guard; remove baked-in engine DSN (Dockerfile ENV + main.rs fallback); switch exposed ports to `expose`.
   - Files: docker-compose.yml, .env.example, apps/queue-engine/Dockerfile, apps/queue-engine/src/main.rs, docs/DEV_SETUP.md.
   - Addresses: AUDIT-605, 801, 802, 1603. Validation: fresh `docker compose up` shows DB connected + audit enabled; `go test ./...`, `cargo test`.

3. **PR: "feat(api): readiness checks database"**
   - Goal: `/readyz` pings Postgres; surfaces audit-disabled state.
   - Files: apps/api/cmd/server/main.go (+test).
   - Addresses: AUDIT-1202. Validation: unit tests; kill DB → readyz 503.

### PHASE 1 — Staging foundations

4. **PR: "feat(db): tracked migrations applied in order"** — migrator + `schema_migrations` table; `make db-migrate-all`; CI ephemeral-Postgres convergence step; wrap 0001–0005 transactions. Addresses AUDIT-601/602/1702.
5. **PR: "feat(web): public catalog endpoints for booking flow"** — public facilities/service-units endpoints (id/name/code only); repoint booking page; keep admin lists privileged. Addresses AUDIT-1801. Validation: booking flow works with `SIGAP_AUTH_MODE=jwt`.
6. **PR: "feat(api): http server hardening"** — MaxBytesReader + server timeouts + graceful shutdown + gRPC Close(). Addresses AUDIT-1001, 1601, part of 501. Validation: load/timeout tests, SIGTERM drain test.
7. **PR: "chore(ci): make rust gates blocking and pin actions"** — drop `|| true`, SHA-pin actions, `permissions:` block, govulncheck version pin. Addresses AUDIT-1703/1704.
8. **PR: "feat(db): unique indexes for short_code/checkin_code"** — dedupe then create; guarded transitions for check-in/capacity (single UPDATE … AND status='scheduled'). Addresses AUDIT-604/1004.
9. **PR: "docs(ops): backup/restore + rotation runbooks"** — nightly pg_dump script, restore drill procedure, credential rotation steps, IR skeleton. Addresses AUDIT-701 doc-half, 20-gaps. Depends on Phase 0 #2 (deployment decision).

### PHASE 2 — Production security

10. Server-side authorization resolution (permissions + facility scope from DB) — AUDIT-101 + 202 together; then enforce scoping in every admin query; policy-completeness test (AUDIT-201).
11. Public-endpoint abuse hardening: second factor + attempt lockouts on status/check-in; masked codes in admin lists; trusted client-IP forwarding; layered rate limiting (AUDIT-301/302/303/401/403).
12. Security headers (hooks.server.ts CSP etc.) + CORS fixes (loopback exception, SSE wildcard) — AUDIT-905/902/903.
13. Token lifetime caps + JWKS discovery pinning + kid-refresh — AUDIT-103/104.
14. Delete demo-PHI handlers, chaos button, hardcoded nearby list, wallet demo coupling — Domain 19 🔴 items.
15. Supply chain: Dependabot/Renovate; build-scan-SBOM job; npm audit — AUDIT-1705/1706/1707.

### PHASE 3 — Reliability / observability

16. JSON logging + request-ID middleware wired into audit — AUDIT-1101/1102.
17. Prometheus `/metrics` + OTel tracing (HTTP, gRPC, outbox depth, SSE gauges) — AUDIT-1203/1204.
18. Distributed limiter (Redis/PG) with eviction — AUDIT-402.
19. Notification dedup + archive/dead-letter + worker metrics + compose topology entry — AUDIT-1302.
20. gRPC resilience (deadlines/retries/keepalive) + SSE pub/sub backbone decision — AUDIT-501/1501.

### PHASE 4 — Operational maturity

21. Backup automation + scheduled restore drills + declared RPO/RTO — AUDIT-701 completion.
22. audit_events partitioning/retention + DB write protection + chain verification — AUDIT-702/1105.
23. Structured old/new audit fields + authn events — AUDIT-1104.
24. Staging/prod environments in GitHub with protection rules + post-deploy smoke (patient-portal suite needs no dev identity) — AUDIT-1710.
25. Hygiene sweep: doc drift, duplicate CODEOWNERS, `+error.svelte`, encodeURIComponent in proxies, 555-range phone consistency — P3 cluster.

---

## 7. Recommended Next PR

**Title:** feat(api): fail-fast environment guards for dev-only capabilities

**Why first:** highest risk reduction per line changed; minimal blast radius (boot-time checks + tests only); directly closes the single most catastrophic conditional risk (anonymous superuser / unauthenticated PHI / silent fake-engine writes reaching a shared environment through one copied env var). Everything downstream assumes this tripwire exists.

**Acceptance criteria:**
- Boot refuses (non-zero exit + clear error) when any of `SIGAP_AUTH_MODE=dev`, `SIGAP_DEV_IDENTITY=true`, `SIGAP_ENABLE_DEMO_PHI=true`, `SIGAP_ENGINE_FALLBACK=dev` is set unless `SIGAP_ENV=local` is explicitly present.
- With `SIGAP_ENV=local` the flags behave exactly as today (zero behavioral change to the demo workflow).
- Unit tests cover every guard branch; `go test ./...` green; `gitleaks detect --source . --redact` clean.

**Files likely affected:** apps/api/internal/auth/config.go, apps/api/cmd/server/main.go, apps/api/cmd/notification-worker/main.go, plus new tests; optionally docs/DEV_SETUP.md note.

**Validation:** `go test ./...`; manual matrix boots (guard combos × local/non-local); existing smoke suite unchanged and green locally.

---

*End of audit. Findings reference the working tree at `b47e07d`; re-validate against newer commits before acting.*
