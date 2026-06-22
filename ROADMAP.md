# Sigap Roadmap

This document outlines the phased evolution of Sigap from its current MVP scaffolding toward a production-ready open-source healthcare superapp. Each phase links to concrete deliverables and success criteria.

> **Current Status**: Auth Provider & Admin Boundary phase completed. JWT/OAuth2 session management and full external OIDC discovery remain backlogged.

---

## Foundation (In Progress)

### Phase 0: Repo Safety & Governance
- [ ] LICENSE (MIT) verified
- [ ] `SECURITY.md` with disclosure path
- [x] `CODEOWNERS` with maintainer entries
- [x] `CONTRIBUTING.md` with conventional commits + PR size cap
- [x] `.env.example` with all vars documented and safe defaults
- [ ] `ROADMAP.md` (this file)
- [ ] Secrets scan with gitleaks

### Phase 1: Operational Hardening for Local Deployment
- [ ] Docker Compose healthchecks for `api` + `rust-engine`
- [ ] Environment-driven CORS (`SIGAP_WEB_ORIGIN`)
- [ ] Makefile targets (`lint`, `security`)
- [ ] Docker Compose config validation

### Phase 2: API & Engine Correctness + Test Baseline
- [ ] `/readyz` endpoint checking engine connectivity
- [ ] FakeQueueService fallback gated behind `SIGAP_ENGINE_FALLBACK=dev`
- [ ] gRPC client with explicit TLS/mTLS or dev-mode documentation
- [ ] Rust engine: graceful DB failure (no `.expect()` panic)
- [ ] Rust concurrency guardrail tests
- [ ] Rust `estimated_wait` regression test
- [ ] Go router tests ≥ 90% coverage

### Phase 3: Minimal Security Foundation Preparation
- [ ] CI: `govulncheck` for Go
- [ ] CI: `cargo audit` for Rust
- [ ] CI: `gitleaks` for secrets scanning
- [ ] Go + Rust tests remain green in CI

### Phase 4: Web Cleanup & Test/Build Baseline
- [ ] Zero `any` types / `@ts-ignore` in web codebase
- [ ] Web build passes (`pnpm run check && pnpm run build`)
- [ ] Basic component test scaffold

### Phase 5: Documentation & Contributor Onboarding
- [ ] `README.md` updated with current setup
- [ ] `docs/DEV_SETUP.md` with step-by-step guide
- [ ] Document security limitations (no auth, demo PHI gates, dev-only gRPC)
- [ ] Document PII policy for development

### Phase 6: Identity/RBAC/Audit Foundation (Identity Foundation — ✅ Completed)
- [x] Forward-only SQL migrations: `app_users`, `roles`, `permissions`, `user_roles`, `role_permissions`, `audit_events`
- [x] Seed data with synthetic system roles and permissions (no real patient data)
- [x] Go `internal/identity` package with `Actor`, request ID, dev identity middleware
- [x] `RequirePermission` middleware enforcing per-route `RequiredPolicy`
- [x] Protected route tests (missing actor, missing permission, correct permission, public routes)
- [x] Go audit service with append-only `audit_events`, hash fields, PII redaction
- [x] Queue handler and authz denied events write privacy-safe audit records
- [x] Security, Dev Setup, and Contributing docs updated
- [x] Go tests pass (41 tests, 10 packages), `go vet` clean

### Phase 6: Final Verification & Summary (Foundation phases complete)
- [x] `make test` passes (Go + Rust + Web)
- [x] Web check/build passes (`svelte-check` 0 errors)
- [x] `docker compose up -d --build` boots healthy
- [x] `docs/FERMENT_REPORT.md` with completed phases

### Phase 7: Appointment Scheduling & Check-In Module (✅ Complete)
- [x] Forward-only migration: `service_units`, `practitioners`, `practitioner_schedules`, `appointments` tables
- [x] Forward-only seed: `appointment.read`, `appointment.manage`, `schedule.read`, `schedule.manage` permissions
- [x] Admin API: `GET/POST/PATCH /api/v1/admin/service-units`, `GET/POST/PATCH /api/v1/admin/schedules`, `GET /api/v1/admin/appointments`, `PATCH /api/v1/admin/appointments/{id}/status`
- [x] Public Booking API: `POST /api/v1/appointments` with check-in code, rate limiting by phone, capacity enforcement
- [x] Check-in API: `POST /api/v1/appointments/{id}/check-in` validates code → gRPC GenerateQueueNumber → updates status `scheduled→checked_in→queued`
- [x] Status transitions enforced: `scheduled→checked_in→queued→completed`, `cancelled`, `no_show`
- [x] Privacy-safe audit: `appointment.created`, `appointment.status_updated`, `appointment.checked_in` with sanitized metadata (no raw phone)
- [x] SvelteKit admin UI: `/admin/schedules`, `/admin/appointments` with loading/error/empty states, dark/light support
- [x] Patient UI: `/appointments/new`, `/appointments/check-in`
- [x] svelte-check 0 errors; production build succeeds
- [x] All CI green (`make test`, `make lint`, `make security`)

---

## Future Modules (Backlog)

> **These are NOT in scope for the current foundation work.** They represent the superapp vision and will be scheduled in subsequent phases.

### Phase 8: Demo Readiness & Smoke Suite (✅ Completed)

This stabilisation phase makes the MVP demonstrable end-to-end on a fresh
local checkout without external services, real patient data, or a heavy
test framework.

- [x] `packages/db/seed/demo.sql` — additive, idempotent synthetic demo data (2 service units, 2 practitioners, 2 schedules tied to facility `f1`, scheduled for "tomorrow")
- [x] `scripts/smoke/sigap-demo-smoke.ps1` — PowerShell 7+ smoke suite covering health → facility list → public booking → check-in → queue list → appointment status update
- [x] `scripts/smoke/README.md` — invocation and troubleshooting for the smoke suite
- [x] `docs/DEMO_FLOW.md` — canonical 10-minute demo walkthrough (prerequisites, three-terminal start, smoke suite, UI tour, troubleshooting)
- [x] `README.md` — "Demo Ready (10 menit)" section above Quickstart
- [x] `docs/DEV_SETUP.md` — top-of-file pointer to the demo flow
- [x] All existing quality gates green: `go test ./...`, `cargo test`, `pnpm --filter sigap-web run check`, `govulncheck`, `cargo audit`, `gitleaks detect --source . --redact`
- [x] No new Go/Rust/Svelte source files modified
- [x] No destructive migrations; no new dependencies

### Phase 7: Authentication, Authorization, Facility Admin & Queue Console (Auth Provider & Admin Boundary — ✅ Completed)

The auth provider interface, JWT scaffold, protected admin routes, facility CRUD, queue operator console, and bootstrap CLI are now complete. Foundation RBAC and audit services from Phase 6 are leveraged for real enforcement.

- [x] Pluggable `auth.Provider` interface (`internal/auth/provider.go`)
- [x] `DevIdentityProvider` with `SIGAP_DEV_IDENTITY` gate and `X-Sigap-Dev-User-ID` header
- [x] `JWTProvider` with JWKS cache, `alg=none` rejection, exp/iss/aud validation, permissions claim extraction
- [x] Auth mode selection at boot (`disabled`, `dev`, `jwt`) via `SIGAP_AUTH_MODE`; fail-closed config validation
- [x] Middleware chain: `DenyByDefault → AuthProvider → RequirePermission → mux`
- [x] **Facility Admin CRUD** (`facility.read` / `facility.manage`):
  - [x] `GET /api/v1/admin/facilities` — list all facilities
  - [x] `GET /api/v1/admin/facilities/{id}` — get facility detail
  - [x] `POST /api/v1/admin/facilities` — create facility with validation
  - [x] `PATCH /api/v1/admin/facilities/{id}` — update facility
  - [x] `PATCH /api/v1/admin/facilities/{id}/deactivate` — soft deactivate
  - [x] Validation: name required, type enum, `available_beds ≤ total_beds`, phone sanitization
- [x] **Queue Operator Console** (`queue.read` / `queue.manage`):
  - [x] `GET /api/v1/admin/queues?facility_id=` — list tickets per facility
  - [x] `GET /api/v1/admin/queues/{id}` — get ticket detail
  - [x] `PATCH /api/v1/admin/queues/{id}/status` — update status with state machine
  - [x] State machine: `waiting→called→in_service→completed`, plus `cancelled`/`skipped`
  - [x] PHI minimization: `patient_id` never exposed in admin responses
- [x] `queue.manage` permission added via forward-only seed (`rbac.sql`)
- [x] Privacy-safe audit events for all mutations (`facility.created`, `facility.updated`, `facility.deactivated`, `queue.status_updated`)
- [x] **SvelteKit Admin UI**: `/admin/facilities` (list/create/edit/deactivate) and `/admin/queues` (status console with badges)
- [x] Admin UI proxy endpoints (`+server.ts`) forwarding to Go API with dev identity headers
- [x] Integration tests: unauthenticated 403, wrong permission 403, correct permission 200, public 200, dev identity enabled/disabled
- [x] Bootstrap admin CLI (`cmd/bootstrap`) env-gated, idempotent, synthetic data only
- [x] Auth denials write privacy-safe audit events via existing audit service
- [x] `svelte-check` 0 errors, 0 warnings; `vite build` succeeds
- [x] Go tests pass (123 tests, 12 packages), Rust tests pass, `make lint` clean, `make security` clean

Backlogged for future phases:
- Full OIDC discovery flow (`.well-known/openid-configuration`)
- Token refresh, revocation, logout, and session management
- OAuth2 / social login integration (Google, etc.)
- Patient identity verification (NIK + phone)
- Full middleware audit coverage for every PHI access path
- Fine-grained facility scoping (e.g., province-level admin)
- Real-time queue SSE for admin console (currently polling-based)

### Phase 8: Live SATUSEHAT Integration
- Bridge module for Kemenkes SATUSEHAT API
- Patient consent management
- Health encounter sync
- Facility master data sync
- Error handling, retries, and circuit breaker

### Phase 9: Clinical Modules
- **Pharmacy**: Digital prescriptions, medication history, stock check
- **Lab**: Test orders, results tracking, digital reports
- **Emergency**: Triage queue, ambulance dispatch, bed reservation
- **Referral**: Smart routing, queue transfer, capacity alerts

### Phase 9b: Notification Outbox & Patient Communication Foundation (✅ Completed)

Privacy-first notification foundation for appointment/check-in communication. No real vendor is integrated in this milestone — the only delivery provider shipped is an offline, deterministic DevProvider. Real vendor integration (Twilio, WhatsApp Cloud API, SMTP, SendGrid, MessageBird, …) is intentionally deferred to a later phase.

- [x] **Forward-only DB**: packages/db/migrations/0006_notifications.sql creates three tables (
otification_templates,
otification_outbox,
otification_delivery_attempts). No destructive SQL; no existing rows modified.
- [x] **RBAC additions**:
otification.read and
otification.manage added to packages/db/seed/rbac.sql. super_admin and acility_admin get both; operator and iewer get
otification.read only.
- [x] **Go service** (pps/api/internal/notification/): Service.Enqueue, Service.List, Service.GetByID, Service.Retry, Service.Cancel. Dev provider (DevProvider) is offline (no network) and deterministic.
- [x] **Privacy model**: mask + SHA-256 dedup, denylist regex (8+ digits), DB CHECK constraints as defence-in-depth, OutboxRow struct has no RecipientContact or RecipientContactHash field (compile-time test guarantees this).
- [x] **API endpoints**: 4 protected admin endpoints under /api/v1/admin/notifications/… — list, get, retry, cancel — gated by
otification.read /
otification.manage.
- [x] **Web UI**: /admin/notifications page with masked-recipient rendering, status badges, retry/cancel actions.
- [x] **Fire-and-forget triggers** in handler/booking.go: BookAppointment enqueues ppointment.booked.confirmation, CheckIn enqueues ppointment.checked_in.confirmation. HTTP response is never blocked by enqueue failures.
- [x] **Audit sanitisation**: metadata keys restricted to
otification_id, acility_id, channel, 	emplate_key, status, outcome. The audit sanitizer's forbidden-key list catches accidental PII leaks.
- [x] **Docs**: docs/NOTIFICATIONS_REPORT.md documents the design, privacy model, dev provider, and future vendor integration seam.
- [x] **No new external dependencies**: go.mod and package.json are unchanged.
- [x] Notification worker + template renderer (DevProvider only):
  `apps/api/internal/notification/{worker,renderer}.go` +
  `apps/api/cmd/notification-worker/main.go`.
  Manual execution; no docker-compose service in this PR.

### Phase 10: Production Deployment Hardening
- Kubernetes manifests and Helm charts
- TLS termination at ingress
- Secrets management (HashiCorp Vault or cloud-native)
- Observability: structured logging, metrics, alerting
- Automated backups for PostgreSQL
- Rate limiting at edge (API gateway)

### Phase 11: Audit & Compliance (full implementation)
- [x] Privacy-safe audit logging for auth denials and queue events
- Immutable audit trail for all PHI access — **foundation schema & basic service done**
- Full local hash-chain append-only log verification — **schema fields present; chain validation backlogged**
- Data retention policies
- GDPR/HIPAA-style compliance documentation

---

## How We Prioritize

1. **Safety before features**: No live PHI without authn/authz.
2. **Test before deploy**: Every module must have tests before entering production.
3. **Incremental delivery**: Prefer 300-line PRs that ship fast over monolithic branches.
4. **Community-driven**: Issues and discussions shape backlog ordering.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for branch conventions, commit format, and PR process. Pick an unchecked item from the current phase and open a draft PR early.

---

*Last updated: 2026-06-19*
