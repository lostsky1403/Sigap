# Better Auth isolated-schema migration plan (SIGAP self-hosted identity)

**Status:** Implementation plan for `apps/web/src/lib/server/auth.better.ts`.
**Date:** 2026-11-11

## Decision

Better Auth stores its tables in an **isolated `auth` schema**, never in SIGAP's
`public` schema. The SIGAP domain tables, RBAC tables, and the `packages/db`
migration ledger (`0001_init.sql` … `0014_patient_account.sql`, plus the
`schema_migrations` tracking table used by `SIGAP_AUTO_MIGRATE`) are
**untouched**.

- Same Postgres instance / database as SIGAP (`:5433`), same connection string.
- Isolation is via `database.schemaName: "auth"` in `auth.better.ts`, which the
  runtime Kysely dialect and the CLI both honor.
- Tables created: `auth.user`, `auth.session`, `auth.account`,
  `auth.verification`, plus the `jwt` plugin's `auth.jwks`. (`auth.rateLimit`
  is added only if `rateLimit.storage === "database"`, which we do not set.)
- The private signing key in `auth.jwks` is AES-256-GCM encrypted by default.
- The Postgres role Better Auth uses must be able to `CREATE SCHEMA`/tables, or
  the `auth` schema must be pre-created and `GRANT`ed before migrating.

## Migration command

### CLI (recommended, Kysely adapter)

```
npx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts
```

Or with pnpm: `pnpm dlx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts`

The CLI reads `database.schemaName: "auth"` from the config, emits
`create schema if not exists "auth";`, and applies schema-qualified DDL
(`"auth"."user"`, …). It only touches the `auth` schema. Run it from the repo
root with the DB URL present in the environment:

```
SIGAP_DATABASE_URL="postgresql://sigap:...@localhost:5433/sigap?sslmode=require" \
  npx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts
```

(The CLI searches for an `auth.ts` config by default; our config is
`auth.better.ts`, so pass `--config` explicitly.)

### Programmatic (Kysely adapter)

```ts
import { getMigrations } from "better-auth/db/migration";

const { toBeCreated, toBeAdded, runMigrations } = await getMigrations(auth.options);
await runMigrations();
```

`getMigrations` requires the Kysely adapter (which we use) and applies into the
`auth` schema because `database.schemaName` is part of `auth.options`.

### Older CLI alias

The pre-1.7 CLI was `@better-auth/cli` (`npx @better-auth/cli migrate`). On
1.7.x use the `auth@latest` CLI above.

## Env vars needed

| Var | Purpose |
|---|---|
| `SIGAP_DATABASE_URL` | Canonical Postgres URL (Go API name) — read first |
| `DATABASE_URL` | Fallback Postgres URL (host-run Makefile tooling) |
| `BETTER_AUTH_SECRET` | Better Auth secret; required, auto-read, never hardcoded |
| `BETTER_AUTH_BASE_URL` | (optional) app origin for the `auth` instance |
| `SIGAP_AUTH_ISSUER` / `SIGAP_AUTH_AUDIENCE` / `SIGAP_AUTH_JWKS_URL` | Go-side env, unchanged shape. Keep in sync with `jwt`: `issuer` is pinned to `http://127.0.0.1:5173`, `audience` to `authenticated` in `auth.better.ts`. |

## Rollback

```sql
DROP SCHEMA auth CASCADE;
```

This drops every Better Auth table (`user`, `session`, `account`,
`verification`, `jwks`, …) and the schema itself. It **cannot** touch `public`
or the SIGAP migration ledger, because those live in a different schema.

## Caveats

- `kysely` is only a transitive dependency of `better-auth`; under pnpm's
  strict node isolation, `import { PostgresDialect } from "kysely"` may need
  `kysely` added to `apps/web` deps (or a hoist setting).
- The `jwt` plugin import is `better-auth/plugins` — there is no top-level
  `better-auth/jwt` export in `better-auth@1.7.20`.
- Do NOT flip `jwks.keyPairConfig.alg` back to the EdDSA default: the Go
  `parseJWK` rejects `OKP`/Ed25519 keys (it only accepts `RSA`/`EC`).