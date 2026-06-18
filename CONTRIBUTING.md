# Contributing to Sigap

First off, thank you for considering contributing to Sigap! 🎉

Sigap is an open-source Civic-Tech framework for regional health information and queue services. We welcome contributions of all kinds: bug fixes, features, documentation, translations, and ideas.

## Code of Conduct

This project and everyone participating in it is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the maintainers.

## How to Run the Project (Recommended)

The easiest and most reliable way to run the entire stack (PostgreSQL + Rust Engine + Go API + SvelteKit) is with Docker Compose.

```bash
# 1. Clone the repository
git clone https://github.com/lostsky1403/Sigap.git
cd Sigap

# 2. Start everything (builds images automatically)
docker compose up -d --build

# 3. Apply database migrations (run once)
docker compose exec -T postgres psql -U sigap -d sigap -f /docker-entrypoint-initdb.d/0001_init.sql

# 4. Open the application
# Web UI:   http://localhost:3000
# API:      http://localhost:8080
# SSE:      http://localhost:8080/api/v1/events/beds
```

See the [README.md](README.md) "1-Click Deploy" section for more details and example curl commands.

## Development Workflow

We use **conventional commits** and prefer small, focused pull requests.

### Branch Naming & Commits

- Use feature branches: `feat/xxx`, `fix/xxx`, `docs/xxx`, `chore/xxx`
- Commit messages must follow **Conventional Commits**:
  - `feat: add SSE real-time bed updates`
  - `fix(api): correct rate limit key for phone+facility`
  - `docs: improve Docker 1-Click instructions`
  - `refactor: extract gRPC client into internal/grpc`

### Code Style (High Level)

- **English** for all code and code comments.
- **Indonesian** for user-facing messages and errors.
- Prefer early returns over deep nesting.
- Keep functions small and focused (aim for < 50 lines when reasonable).
- No `any` (TypeScript) or unsafe code without strong justification.
- For behavior changes, API changes, or user-visible features: follow TDD principles (write failing test first when practical).
- Update or add tests for new functionality.
- All changes must pass `go test`, `cargo check`, and `pnpm --filter sigap-web run check`.

### Running Tests Locally (without full Docker)

- Go API: `cd apps/api && go test ./... -v`
- Rust Engine: `cd apps/queue-engine && cargo check && cargo clippy`
- Web: `cd apps/web && pnpm install && pnpm run check`

## Pull Request Process

1. Fork the repository and create your branch from `main`.
2. Make your changes + add/update tests.
3. Ensure the project builds and tests pass (`docker compose build` is also acceptable).
4. Update documentation (README, comments, etc.) if needed.
5. Open a Pull Request against `main`.
6. Fill out the PR template completely.
7. Be responsive to review feedback.

We use squash merges. Your PR title will become the commit message (so make it good!).

### PR Size Cap

Prefer small, focused pull requests. Aim for **under 300 lines of code** per PR when possible. Large changes are harder to review, riskier to deploy, and harder to roll back. If a feature requires more than 300 lines, split it into stacked or incremental PRs (e.g., scaffold → logic → tests → integration).

## Data Privacy & PII Prohibition

**Never use real patient data in development, tests, seeds, fixtures, or pull requests.**

- All test data must be synthetic. Use fictional names like "Test Patient" and phone numbers like `081234567890`.
- Do not include real names, medical record numbers, national IDs, addresses, or any personally identifiable information (PII) in code, commits, issues, documentation, **logs**, or **audit events**.
- If you find real patient data anywhere in the repository, report it immediately as a security issue per `SECURITY.md`.
- Reviewers will reject any PR that introduces real patient data or insufficiently anonymized test fixtures.

### Canonical PII Forbid-List

The audit service maintains a canonical list of forbidden metadata keys that **must never** appear in logs, audit events, seeds, fixtures, or tests. This is the single source of truth for PII redaction:

- See `apps/api/internal/audit/service.go` (`forbidList`) for the authoritative list.
- Keys matching these substrings are automatically redacted to `[REDACTED]` before they reach the audit log.
- **Do not add new metadata keys** that overlap with these forbidden substrings without updating `forbidList` and this section.

The current forbidden substrings include identities (`patient`, `pasien`, `nik`, `ktp`), contact (`phone`, `telepon`, `email`), personal names (`name`, `nama`), and addresses (`address`, `alamat`). Any PR that introduces metadata keys containing these substrings must include corresponding `forbidList` entries.

## Questions?

Feel free to open an issue with the `question` label or start a discussion.

Thank you again for helping make Sigap better for communities!
