// apps/web/src/lib/server/identity-status.ts
//
// Fail-closed identity-status gate for Better Auth session creation.
//
// `public.app_users` is the single authoritative application-identity status
// source (ADR `design/identity-deactivation-policy-adr.md`, adopted policy
// 2026-11-19). This module refuses to mint a session unless it can positively
// confirm that the authenticated subject maps to an ACTIVE, non-deleted
// application identity.
//
// Every refusal uses the SAME generic credential error so the response cannot
// be used to distinguish "disabled", "deleted", "missing identity" or "DB
// outage" from an ordinary wrong password (anti-enumeration, policy item 5).
// No SQL text, status value or account-existence signal is ever exposed.

import { APIError } from "better-auth";
import { getPool } from "./db";

/**
 * Generic authentication failure. Mirrors better-auth's own
 * `INVALID_EMAIL_OR_PASSWORD` response so a refused identity is
 * indistinguishable from a bad credential at the login form.
 *
 * `login/+page.server.ts` classifies `status === "UNAUTHORIZED"` (or
 * `statusCode === 401`) as a credential rejection and renders
 * "Email atau kata sandi salah." for it — identical to a wrong password.
 */
function authenticationRefused(): APIError {
	return APIError.from("UNAUTHORIZED", {
		message: "Email atau kata sandi salah.",
		code: "INVALID_EMAIL_OR_PASSWORD",
	});
}

const ACTIVE_STATUS = "active";

/**
 * Assert that `subject` (the verified Better Auth user id, used as
 * `app_users.subject`) currently maps to an ACTIVE application identity.
 *
 * Fail-closed outcomes (all throw the same generic error):
 *   - no database configured / pool unavailable
 *   - the status query throws (DB outage during identity check)
 *   - no `app_users` row for the subject (missing application identity)
 *   - status <> 'active' (explicitly disabled)
 *   - deleted_at IS NOT NULL (soft-deleted)
 *   - a blank/mismatched subject (no authenticated identity to check)
 *
 * Returns void only for a confirmed-active identity, so the caller may
 * proceed with session creation. Never caches across subjects.
 */
export async function assertActiveIdentity(
	subject: unknown,
	injectedPool?: { query: (sql: string, params: unknown[]) => Promise<{ rows: unknown[] }> }
): Promise<void> {
	// A blank or non-string subject cannot be a verified identity. Refuse rather
	// than let a falsy value fall through into a query that might match nothing
	// or, worse, be skipped.
	if (typeof subject !== "string" || subject.trim() === "") {
		throw authenticationRefused();
	}

	// `injectedPool` is test-only (disposable fake); production always uses the
	// real configured pool from getPool(). No caching across subjects.
	const pool = injectedPool ?? getPool();
	if (!pool) {
		// No DB means we cannot confirm identity status. Fail closed.
		throw authenticationRefused();
	}

	let result;
	try {
		// Read status + deletion marker only; no secrets, no PII, no session data.
		// This reads the SAME `public.app_users` table the Go RBAC resolver uses,
		// never a cached copy or a second source of truth.
		result = await pool.query(
			"SELECT status, deleted_at FROM public.app_users WHERE subject = $1 LIMIT 1",
			[subject]
		);
	} catch {
		// DB unavailable during the check → fail closed (never mint on error).
		throw authenticationRefused();
	}

	const row = (result?.rows ?? [])[0] as
		| { status?: unknown; deleted_at?: unknown }
		| undefined;

	if (!row) {
		// Missing application identity → fail closed.
		throw authenticationRefused();
	}
	if (row.status !== ACTIVE_STATUS) {
		throw authenticationRefused();
	}
	if (row.deleted_at != null) {
		throw authenticationRefused();
	}

	// Confirmed active: allow session creation.
}
