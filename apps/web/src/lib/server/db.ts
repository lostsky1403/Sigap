// apps/web/src/lib/server/db.ts
//
// Shared PostgreSQL connection for the SIGAP web app's server-side identity
// layer (Better Auth). We connect to the SAME Postgres instance that the SIGAP
// Go API and the `packages/db` domain tables use — but Better Auth's tables are
// schema-qualified to the `auth` schema (see `auth.better.ts`
// `database.schemaName`), so they never collide with the `public` domain
// tables or the `packages/db` migration ledger.

import { Pool } from "pg";

/**
 * Connection target, resolved once at import. Prefer `SIGAP_DATABASE_URL` (the
 * canonical name used by the Go API and notification worker); fall back to the
 * generic `DATABASE_URL` the host-run Makefile / migration tooling uses.
 */
const connectionString = process.env.SIGAP_DATABASE_URL ?? process.env.DATABASE_URL;

let pool: Pool | undefined;

/** True when a Postgres connection string is configured. */
export function databaseConfigured(): boolean {
	return Boolean(connectionString);
}

/**
 * Lazily-created module singleton. `pg.Pool` does not open a socket until the
 * first query, so importing this module never forces a DB connection.
 */
export function getPool(): Pool | undefined {
	if (!connectionString) return undefined;
	if (!pool) {
		// The local Docker Postgres serves a self-signed TLS certificate. Go's
		// pgx (sslmode=require) encrypts without verifying, but Node's pg (v9)
		// treats sslmode=require as verify-full and rejects it. For a LOCAL
		// (127.0.0.1/localhost) target only, skip certificate verification.
		// Production always points at a CA-signed server, so this branch does
		// not weaken it.
		const localTarget = /@(127\.0\.0\.1|localhost):/.test(connectionString);
		const ssl = localTarget ? { rejectUnauthorized: false } : undefined;
		pool = new Pool({ connectionString, ssl });
	}
	return pool;
}