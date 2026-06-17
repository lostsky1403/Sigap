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
