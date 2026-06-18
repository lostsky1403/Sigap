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

### What is NOT implemented (backlogged)
- Real authentication (JWT, OAuth 2.0 / OIDC, sessions).
- Cryptographic hash-chain verification (the schema stores `previous_hash` and `event_hash`; full chain validation is planned for a later compliance phase).
- Production audit log tamper-evidence beyond best-effort append-only inserts.

## Known Security Limitations (Foundation Phase)

Sigap is currently a **foundation-phase MVP**. It is **not suitable for production use with real patient data** without significant additional hardening. The following limitations are intentional out-of-scope items for the current foundation phase and will be addressed in subsequent phases:

### Authentication & Authorization
- **There is no authentication or session management.** All endpoints are public. Anyone can submit queue requests or access dashboard data.
- **No role-based access control (RBAC).** There are no admin, operator, or patient roles.
- Planned: OAuth 2.0 / OIDC integration, JWT-based sessions, and fine-grained RBAC for facility administrators and patients.

### Data Protection
- **No end-to-end encryption.** Queue submissions and SSE events travel in plaintext over HTTP.†
- **No field-level encryption for PII/PHI.** Patient names and phone numbers are stored in the database as plaintext.†
- **No audit logging.** Actions (who created a queue ticket, who updated a bed count) are not logged for accountability.
- **No data retention policies.** Old queue tickets and patient records are never purged.
- Planned: TLS/mTLS for gRPC, field-level encryption, robust audit logging, and configurable retention.

### gRPC Transport
- **gRPC between Go API and Rust engine runs unencrypted in development.** The production CA bundle path is scaffolded but the client defaults to `insecure` when `SIGAP_GRPC_TLS` is not explicitly enabled.
- The `SIGAP_ENGINE_FALLBACK=dev` mode uses a fake in-memory queue service with no persistence or concurrency safety for demonstration purposes only.
- Planned: mTLS with client certificates for gRPC; strict fail-closed TLS in production.

### Input Validation
- **Basic validation only.** Phone numbers, facility IDs, and patient data receive minimal validation beyond JSON unmarshaling.
- **No rate limiting at the API gateway.** The Rust engine has an internal concurrency guardrail, but the HTTP layer does not throttle per-IP or per-phone.
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
