// Identity-deactivation DB-backed integration test (adopted ADR Option 1).
//
// This suite drives REAL Better Auth session creation against a disposable
// Postgres with a migrated `auth` schema and a `public.app_users` table, using
// SYNTHETIC identities only. It verifies actual session rows and responses —
// not source-code assertions. It is DB-gated exactly like the other auth
// suites: it skips ([GATE] SKIP ...) when no DB is reachable, and with
// SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB=1 a skip is fatal (CI sets it).
//
// Usage:
//   node tests/identity-deactivation.integration.test.js
//   SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB=1 node tests/identity-deactivation.integration.test.js
//
// NOTE: no secrets, password hashes, tokens or PII are ever printed. All
// identities are per-run synthetic values and are deleted in `finally`.

import assert from 'node:assert';
import crypto from 'node:crypto';

const REQUIRE_DB = process.env.SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB === '1';
const DATABASE_URL = process.env.DATABASE_URL || process.env.SIGAP_DATABASE_URL || '';

const runId = Date.now().toString(36) + crypto.randomBytes(3).toString('hex');
const suffix = `${runId}`;

function skip(reason) {
	if (REQUIRE_DB) {
		console.error(`[GATE] FAIL (REQUIRE_DB=1): ${reason}`);
		process.exit(1);
	}
	console.log(`[GATE] SKIP (${reason})`);
	process.exit(0);
}

async function main() {
	if (!DATABASE_URL) skip('DATABASE_URL / SIGAP_DATABASE_URL not set');

	// Dynamic imports so the no-DB skip path needs neither pg nor the built app.
	let pg;
	let authModule;
	try {
		pg = await import('pg');
		authModule = await import('../src/lib/server/auth.better.ts');
	} catch (err) {
		skip(`deps unavailable (${err?.code || err?.name || 'unknown'})`);
	}

	const pool = new pg.Pool({ connectionString: DATABASE_URL });
	const created = { userIds: [], appUserIds: [] };
	try {
		await pool.query('select 1');
	} catch (err) {
		await pool.end();
		skip(`DB unreachable (${err?.code || 'unknown'})`);
	}

	const auth = authModule.auth;
	const pass = 'Str0ng!TestPass' + suffix;

	async function makeUser(label, status = 'active', createIdentity = true) {
		const email = `idtest-${label}-${suffix}@example.test`;
		const signup = await auth.api.signUpEmail({
			body: { email, password: pass, name: label }
		}).catch((e) => ({ error: e }));
		if (signup.error) throw new Error(`signup failed for ${label}: ${signup.error?.status || 'unknown'}`);
		const authUser = signup.user || (await auth.api.getSession ? null : null);
		// Resolve the Better Auth user id via a trusted server query (no PII printed).
		const { rows } = await pool.query(
			'SELECT id FROM "auth"."user" WHERE email = $1',
			[email]
		);
		const userId = rows[0]?.id;
		assert.ok(userId, 'auth user id must exist after signup');
		created.userIds.push(userId);

		if (createIdentity) {
			const appId = crypto.randomUUID();
			await pool.query(
				`INSERT INTO public.app_users (id, subject, email, display_name, status, deleted_at)
				 VALUES ($1, $2, $3, $4, $5, NULL)`,
				[appId, userId, email, label, status]
			);
			created.appUserIds.push(appId);
		}
		return { email, userId };
	}

	async function signIn(email) {
		return auth.api.signInEmail({ body: { email, password: pass } })
			.then((r) => ({ ok: true, result: r }))
			.catch((e) => ({ ok: false, status: e?.status, statusCode: e?.statusCode }));
	}

	async function sessionCount(userId) {
		const { rows } = await pool.query(
			'SELECT count(*)::int AS n FROM "auth"."session" WHERE "userId" = $1',
			[userId]
		);
		return rows[0].n;
	}

	try {
		// 1. Active citizen signs in → session created.
		const active = await makeUser('active', 'active');
		const s1 = await signIn(active.email);
		assert.equal(s1.ok, true, 'active identity must be able to sign in');
		assert.ok((await sessionCount(active.userId)) >= 1, 'active sign-in must create a session row');

		// 2. Disabled identity cannot create a session.
		const disabled = await makeUser('disabled', 'disabled');
		const s2 = await signIn(disabled.email);
		assert.equal(s2.ok, false, 'disabled identity must be denied');
		assert.equal(
			s2.status === 'UNAUTHORIZED' || s2.statusCode === 401, true,
			'denial must be a generic credential error (anti-enumeration)'
		);
		assert.equal(await sessionCount(disabled.userId), 0, 'disabled identity must create no session row');

		// 3. Soft-deleted identity denied.
		const del = await makeUser('softdel', 'active');
		await pool.query('UPDATE public.app_users SET deleted_at = now() WHERE subject = $1', [del.userId]);
		const s3 = await signIn(del.email);
		assert.equal(s3.ok, false, 'soft-deleted identity must be denied');
		assert.equal(await sessionCount(del.userId), 0, 'soft-deleted identity must create no session row');

		// 4. Missing application identity denied.
		const missing = await makeUser('missing', 'active', false);
		const s4 = await signIn(missing.email);
		assert.equal(s4.ok, false, 'missing app_users row must be denied');
		assert.equal(await sessionCount(missing.userId), 0, 'missing identity must create no session row');

		// 5. Repeated attempts stay denied.
		for (let i = 0; i < 3; i++) {
			const s = await signIn(disabled.email);
			assert.equal(s.ok, false, 'repeated disabled sign-in must stay denied');
		}
		assert.equal(await sessionCount(disabled.userId), 0);

		// 6. Reactivation allows a fresh session; old revoked session not resurrected.
		await pool.query("UPDATE public.app_users SET status = 'active', deleted_at = NULL WHERE subject = $1", [disabled.userId]);
		const s6 = await signIn(disabled.email);
		assert.equal(s6.ok, true, 'reactivated identity must be able to sign in');
		assert.ok((await sessionCount(disabled.userId)) >= 1);

		console.log('IDENTITY_DEACTIVATION_INTEGRATION=PASS');
	} finally {
		// Cleanup in reverse FK order; never print row values.
		for (const id of created.appUserIds) {
			await pool.query('DELETE FROM public.app_users WHERE id = $1', [id]).catch(() => {});
		}
		for (const id of created.userIds) {
			await pool.query('DELETE FROM "auth"."session" WHERE "userId" = $1', [id]).catch(() => {});
			await pool.query('DELETE FROM "auth"."user" WHERE id = $1', [id]).catch(() => {});
		}
		await pool.end();
	}
}

main().catch((err) => {
	console.error('IDENTITY_DEACTIVATION_INTEGRATION=FAIL', err?.message || String(err));
	process.exit(1);
});
