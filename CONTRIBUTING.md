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

## Questions?

Feel free to open an issue with the `question` label or start a discussion.

Thank you again for helping make Sigap better for communities!
