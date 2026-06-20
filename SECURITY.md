# Security Policy

Sigap handles health-adjacent and personal data. We take security and privacy
seriously and appreciate responsible disclosure from the community.

## Supported Versions

Sigap is pre-1.0 and under active development. Security fixes are applied to
the `main` branch. Pin a commit for production and watch releases for advisories.

| Version | Supported          |
| ------- | ------------------ |
| `main`  | :white_check_mark: |
| < 1.0   | best-effort        |

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Please report privately through one of:

1. **GitHub Private Vulnerability Reporting** (preferred): open the repository's
   **Security** tab and choose **Report a vulnerability**. This creates a private
   advisory visible only to maintainers.
2. If that is unavailable, contact the maintainers listed in `CODEOWNERS`
   through a private channel and request a secure reporting address.

Please include:

- A description of the vulnerability and its impact.
- Steps to reproduce (proof-of-concept if possible).
- Affected component (Go API, Rust queue-engine, SvelteKit web, database, deploy).
- **Do not include real patient data, production secrets, or personal
  identifiers** in your report. Use synthetic data only.

## Disclosure Process

1. We acknowledge your report within **5 business days**.
2. We investigate and confirm the issue, and agree on a remediation timeline.
3. We develop and test a fix on a private branch.
4. We publish a security advisory crediting the reporter (unless anonymity is
   requested) once a fix is available.

We aim to remediate critical issues within **30 days** of confirmation. Coordinated
disclosure is requested: please give us reasonable time to ship a fix before any
public disclosure.

## Identity, RBAC & Audit Foundation (Identity Foundation — ✅ Completed)

The foundation for authentication, authorization, and compliance auditing is now in place. This is a **scaffolded foundation only** — it does not provide real user authentication or session management yet.

### What is implemented
- **Identity model** (`app_users`, `roles`, `permissions`, `user_roles`, `role_permissions`) with forward-only SQL migrations.
- **Go `internal/identity` package** with `Actor`, request ID propagation, and `DevIdentity` middleware.
- **Authorization middleware** (`RequirePermission`) that enforces per-route `RequiredPolicy` from `router.Registry`. Protected routes fail closed unless an actor with the correct permission is present.
- **Audit service** (`internal/audit`) that appends sanitized, hashed events to `audit_events`. The schema includes `previous_hash` and `event_hash` columns for future chain-of-custody verification.
- **PII redaction** in audit metadata via a canonical forbid-list (`patient`, `pasien`, `phone`, `telepon`, `nik`, `ktp`, `name`, `nama`, `address`, `alamat`, `email`).

### Dev identity (development only)
Setting `SIGAP_DEV_IDENTITY=true` enables a synthetic actor for local testing when the `X-Sigap-Dev-User-ID` header is present. This **MUST NEVER be enabled in production** — the middleware fails closed when the env var is absent or not `"true"`.

## Authentication Provider Architecture (Auth Provider — ✅ Completed)

The API gateway now supports pluggable authentication providers via the `internal/auth.Provider` interface.

### What is implemented
- **`Provider` interface** (`Authenticate(r *http.Request) (Actor, error)`) decouples identity verification from authorization and audit layers.
- **`DevIdentityProvider`** reads `SIGAP_DEV_IDENTITY` once at construction time; when enabled, injects a synthetic `ActorDev` with the full permission set on requests carrying `X-Sigap-Dev-User-ID`.
- **`JWTProvider`** validates RS256/RS384/RS512 and ES256/ES384/ES512 tokens against a JWKS endpoint with a 15-minute TTL cache and stale-serving fallback. It rejects `alg=none`, validates `exp`/`nbf`/`iat`, and extracts `iss`/`aud`/`sub` using `golang-jwt/jwt/v5`.
- **Factory** (`NewProvider`) selects the provider at boot time based on `SIGAP_AUTH_MODE` (`disabled`, `dev`, `jwt`).
- **Config validation** is fatal at boot: invalid auth config logs an error and exits before `ListenAndServe`.
- **Fail-closed defaults**: `SIGAP_AUTH_MODE=disabled` returns a nil provider (transparent pass-through), but `RequirePermission` still denies requests without a valid actor.

### What is NOT implemented (backlogged)
- Full OIDC discovery flow (the JWT provider accepts a raw JWKS URL; no `.well-known/openid-configuration` support yet).
- Token refresh, logout, or session management.
- Key rotation with graceful cutover (JWKS cache refreshes every 15 minutes; there is no forced eviction API).

### Environment variables
| Variable | Required | Description |
|----------|----------|-------------|
| `SIGAP_AUTH_MODE` | No | `disabled` (default), `dev`, or `jwt`. |
| `SIGAP_AUTH_ISSUER` | For `jwt` | Expected `iss` claim. |
| `SIGAP_AUTH_AUDIENCE` | For `jwt` | Expected `aud` claim. |
| `SIGAP_AUTH_JWKS_URL` | No | JWKS endpoint for key retrieval. |

## Admin Boundary (Admin Boundary — ✅ Completed)

Protected admin routes are now enforced by the existing RBAC permission system.

### What is implemented
- **Route registration**: Admin facility routes require `facility.read` (GET) or `facility.manage` (POST/PATCH) permission. Admin queue routes require `queue.read` (GET) or `queue.manage` (PATCH status).
- **Admin handler** (`internal/handler/admin.go`) queries facilities and queue tickets from the database and returns JSON. Privacy-safe audit events are logged for every access attempt.
- **Integration tests** cover all scenarios: unauthenticated → 403, wrong permission → 403, correct permission → 200, public route (`/health`) → 200.
- **Wiring in `cmd/server/main.go`**: shared DB pool between audit service and admin handler, with nil-safe guards that skip admin route registration when the database is unreachable.
- **Queue operator console**: `GET /api/v1/admin/queues` and `PATCH /api/v1/admin/queues/{id}/status` with state-machine enforced transitions (`waiting→called→in_service→completed`, plus `cancelled`/`skipped`).

### What is NOT implemented (backlogged)
- Facility mutation endpoints (POST, PUT, DELETE) — only list (GET) is available.
- Fine-grained facility scoping (e.g., admin can only manage facilities in a specific province).

## Queue Operator Console Privacy (Queue Console — ✅ Completed)

The queue operator console (`/api/v1/admin/queues`) is designed with **PHI minimization** as a core constraint.

### What is implemented
- **No `patient_id` exposure**: Admin queue responses never include `patient_id`. The allowed fields are strictly limited to: `id`, `facility_id`, `queue_number`, `formatted_number`, `status`, `registered_at`, `called_at`, `completed_at`.
- **State-machine enforcement**: Status transitions are validated against an exact allow-list (`waiting→called`, `called→in_service`, `in_service→completed`, `waiting→cancelled`, `called→cancelled`, `called→skipped`). Invalid transitions return `400`.
- **Audit events**: Every status mutation writes a `queue.status_updated` event with sanitized metadata (no patient data).
- **RBAC enforcement**: `queue.read` for list/detail; `queue.manage` for status updates. The `operator`, `facility_admin`, and `super_admin` roles have these permissions via `rbac.sql` seed.

### What is NOT implemented (backlogged)
- Bulk status updates or batch operations.
- Queue ticket reassignment between facilities.
- Real-time queue updates via SSE for admin console (currently polling-based via SvelteKit UI).

## Bootstrap Admin (Bootstrap — ✅ Completed)

A one-time CLI tool at `cmd/bootstrap` creates a synthetic admin user and assigns the `super_admin` role.

### What is implemented
- **Env-gated**: only runs when `SIGAP_BOOTSTRAP_ADMIN=true`; disabled by default.
- **Idempotent**: safe to rerun; finds the existing `admin@sigap.local` user if already present.
- **Synthetic data only**: creates `admin@sigap.local` with `display_name='Bootstrap Admin'` and `status='active'`.
- **Role assignment**: assigns the existing `super_admin` role (which has all five permissions: `queue.generate`, `queue.read`, `facility.read`, `facility.manage`, `audit.read`).

### How to run
```bash
make bootstrap
```

### DANGER
- Never enable `SIGAP_BOOTSTRAP_ADMIN` in production.
- The bootstrap user has full access. Rotate or delete it after initial setup.
- No hardcoded secrets; `admin@sigap.local` is a synthetic email that cannot receive real mail.

## Known Security Limitations (Auth & Admin Boundary Phase)

Sigap is currently a **foundation-phase MVP with auth scaffolding**. It is **not suitable for production use with real patient data** without significant additional hardening. The following limitations are intentional out-of-scope items for the current phase and will be addressed in subsequent phases:

### Authentication & Authorization
- ✅ **Authentication providers exist** (dev identity, JWT with JWKS), but there is **no user-facing login flow** or password-based authentication. Production deployments must bring their own identity provider (e.g., Keycloak, Auth0, AWS Cognito).
- ✅ **RBAC is implemented** in the database schema and middleware, but there is **no user-role management UI or API** beyond the bootstrap CLI.
- 🔒 **Dev identity (`SIGAP_DEV_IDENTITY=true`) is development-only** and trivially bypassable if enabled in production.

### Data Protection
- **No end-to-end encryption.** Queue submissions and SSE events travel in plaintext over HTTP.†
- **No field-level encryption for PII/PHI.** Patient names and phone numbers are stored in the database as plaintext.†
- ✅ **Audit logging is implemented** (append-only, hash-chained schema) but does **not yet have cryptographic chain verification** or tamper-evidence guarantees.
- **No data retention policies.** Old queue tickets and patient records are never purged.

### JWT / OIDC Provider
- The JWT provider is a **scaffold with real token validation**, but it lacks:
  - OIDC discovery (`.well-known/openid-configuration`).
  - Token refresh, revocation, or logout flows.
  - Graceful key rotation beyond JWKS cache TTL (15 minutes).
  - Audience array or multi-issuer support.

### gRPC Transport
- **gRPC between Go API and Rust engine runs unencrypted in development.** The production CA bundle path is scaffolded but the client defaults to `insecure` when `SIGAP_GRPC_TLS` is not explicitly enabled.
- The `SIGAP_ENGINE_FALLBACK=dev` mode uses a fake in-memory queue service with no persistence or concurrency safety for demonstration purposes only.
- Planned: mTLS with client certificates for gRPC; strict fail-closed TLS in production.

### Input Validation & Rate Limiting
- **Basic validation only.** Phone numbers, facility IDs, and patient data receive minimal validation beyond JSON unmarshaling.
- ✅ **Queue rate limiting exists** (2 per day per phone + facility), but there is **no global API rate limiting** (per-IP, per-user, or burst protection) at the gateway level.
- Planned: comprehensive input validation with a rules engine; API-level rate limiting.

### Infrastructure
- **No secrets management.** Database credentials are passed via environment variables. A real deployment should use a secrets manager (e.g., HashiCorp Vault, AWS Secrets Manager).
- **No infrastructure hardening.** Kubernetes manifests, network policies, and WAF rules are not yet provided. The baseline is single-VM Docker Compose.
- Planned: Helm charts, Kubernetes hardening, and IaC (Terraform/Pulumi) templates.

### Operational Security
- **No log scrubbing.** Application logs could potentially contain patient identifiers if request payloads are logged at high verbosity.
- **No DDoS protection.** There is no WAF, CDN, or request throttling beyond the basic concurrency guardrail in the engine.

† _These items are understood to be critical for HIPAA/GDPR/PDPK compliance and are explicitly out of scope for the current foundation phase._

### What You Can Do Now

You can still run Sigap safely in **non-production environments** by adhering to these rules:

1. **Use only synthetic/test data.** Never enter real patient names, phone numbers, or medical records.
2. **Keep the `.env` file private.** Never commit it. Rotate `POSTGRES_PASSWORD` regularly.
3. **Run behind a reverse proxy with TLS.** Use nginx, Caddy, or Traefik to terminate HTTPS before traffic reaches the Go API.
4. **Do not expose the Rust gRPC port** (50051) to the public internet. Keep it on a private Docker network or localhost only.
5. **Disable `SIGAP_ENGINE_FALLBACK=dev`** in any shared or staging environment.
6. **Monitor the dependency security scan** (`make security`) and apply updates promptly.

## Scope

In scope:

- Authentication / authorization bypass.
- Exposure of personal or health data (PHI/PII), including via logs or errors.
- Injection (SQL, command, etc.), SSRF, insecure deserialization.
- Secrets committed to the repository or leaked at runtime.
- Tampering with the append-only audit log.
- Transport security gaps (missing TLS on gRPC or database connections).

Out of scope:

- Vulnerabilities in third-party dependencies without a demonstrated exploit
  path in Sigap (please report upstream).
- Issues requiring privileged local access to a correctly configured host.
- Findings in example/dev configuration that is clearly documented as insecure
  for local development only.

## Privacy Principles (non-negotiable)

- No real patient data in the repository, seeds, tests, or issue reports.
- No PII, medical records, or patient identifiers are ever stored on-chain.
- PII is minimized, encrypted at rest, and never written to logs.

## License Compliance (SPDX)

Sigap is licensed under the [MIT License](./LICENSE). Contributors must ensure
new source files are compatible with MIT. When practical, annotate source files
with an SPDX identifier:

```
// SPDX-License-Identifier: MIT
```

Do not add dependencies under licenses incompatible with MIT (e.g., GPL/AGPL in
statically linked components) without maintainer approval.
