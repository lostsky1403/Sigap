/**
 * SIGAP identity backfill — map Better Auth users onto `app_users.subject`.
 *
 * The one real-data step of the Supabase → Better Auth cutover (see
 * `design/identity-mapping.md` and `design/supabase-dependency-audit.md` §5.2
 * item 12): for every known synthetic citizen + staff account, ensure a Better
 * Auth `auth.user` row exists, then set `public.app_users.subject =
 * <that better-auth user id>` so the Go RBAC resolver (`WHERE u.subject = $1`)
 * maps a verified JWT `sub` to `app_users.id`.
 *
 * Explicit mapping ONLY. It never links an app_user by email equality: each row
 * is a reviewed `{ email, appUserId }` pair from the seed fixtures, and the
 * subject write is always `WHERE id = $appUserId` (never `WHERE email = ...`).
 * The email is used only to locate/create the Better Auth `auth.user` row — which
 * Better Auth's own schema keys on email — never as the app-user join key.
 *
 * Why this mirrors `db.ts` / `auth.better.ts` instead of importing them:
 * both live under `apps/web/src/lib/server` as TypeScript, imported by the Vite
 * build. A bare `node` script cannot resolve the extensionless `./db` import from
 * a stripped `.ts` module, so the two source-of-truth config snippets are repeated
 * below verbatim from `src/lib/server/db.ts` (connection string + local-TLS skip)
 * and `src/lib/server/auth.better.ts` (PostgresDialect + schemaName "auth" +
 * emailAndPassword). The `jwt` plugin is intentionally omitted: it only mints
 * tokens and is irrelevant to creating an `auth.user` row or writing `subject`.
 *
 * Idempotent: the subject write targets a fixed `app_users.id` with the same value
 * on every run, and the Better Auth user is created at most once per email.
 *
 * Usage:
 *   node scripts/backfill-auth-subjects.mjs            # dry-run (no writes)
 *   node scripts/backfill-auth-subjects.mjs --apply    # create users + write subject
 *
 * No NIK. Passwords are synthetic and deterministic.
 */

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import pg from 'pg';
import { betterAuth } from 'better-auth';
import { PostgresDialect } from 'kysely';

const { Pool } = pg;

const here = dirname(fileURLToPath(import.meta.url));

// ---------------------------------------------------------------------------
// Load the repo-root `.env` (source of DATABASE_URL / BETTER_AUTH_*), then
// apps/web/.env, without overwriting anything already exported. Vite does this
// for the app; a bare node script must do it itself.
// ---------------------------------------------------------------------------
for (const envPath of [resolve(here, '../../../.env'), resolve(here, '../.env')]) {
	try {
		const raw = readFileSync(envPath, 'utf8');
		for (const line of raw.split(/\r?\n/)) {
			const m = line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$/);
			if (!m) continue;
			if (process.env[m[1]] !== undefined) continue;
			let value = m[2];
			if (
				(value.startsWith('"') && value.endsWith('"')) ||
				(value.startsWith("'") && value.endsWith("'"))
			) {
				value = value.slice(1, -1);
			}
			process.env[m[1]] = value;
		}
	} catch {
		// no .env at this path — rely on the live environment
	}
}

// ---------------------------------------------------------------------------
// `getPool()` — mirrors src/lib/server/db.ts verbatim.
// Prefer SIGAP_DATABASE_URL (canonical); fall back to DATABASE_URL. A local
// (127.0.0.1/localhost) target uses a self-signed cert, so skip verification
// there only; production always points at a CA-signed server.
// ---------------------------------------------------------------------------
const connectionString = process.env.SIGAP_DATABASE_URL ?? process.env.DATABASE_URL;

let pool;
function getPool() {
	if (!connectionString) return undefined;
	if (!pool) {
		const localTarget = /@(127\.0\.0\.1|localhost):/.test(connectionString);
		const ssl = localTarget ? { rejectUnauthorized: false } : undefined;
		pool = new Pool({ connectionString, ssl });
	}
	return pool;
}

// ---------------------------------------------------------------------------
// `auth` — mirrors src/lib/server/auth.better.ts (minus the token-minting jwt
// plugin, which does not affect user provisioning).
// ---------------------------------------------------------------------------
function buildAuth(pool) {
	return betterAuth({
		baseURL: process.env.BETTER_AUTH_URL || 'http://127.0.0.1:5173',
		database: {
			dialect: new PostgresDialect({ pool }),
			type: 'postgres',
			schemaName: 'auth',
		},
		emailAndPassword: { enabled: true },
	});
}

// ---------------------------------------------------------------------------
// The reviewed, explicit mapping. Never scan by email — every app_users.id is
// hand-listed so the subject write is by id, not by email equality.
//
//   d990-d995  global/facility/zero-scope admins + e2e schedule actors (dev.sql)
//   c301-c306  org/regional/facility staff (regional_tenancy.sql)
//   c307       the citizen patient (regional_tenancy.sql:153)
// ---------------------------------------------------------------------------
const MAPPING = [
	{ email: 'global-admin@sigap.local',         appUserId: '00000000-0000-0000-0000-00000000d990', name: 'Local Global Super Admin (synthetic)' },
	{ email: 'facility-admin@sigap.local',       appUserId: '00000000-0000-0000-0000-00000000d991', name: 'Local Facility Admin (synthetic)' },
	{ email: 'zero-scope-admin@sigap.local',     appUserId: '00000000-0000-0000-0000-00000000d992', name: 'Local Zero-Scope Admin (synthetic)' },
	{ email: 'e2e-schedule-manager@sigap.local', appUserId: '00000000-0000-0000-0000-00000000d993', name: 'Local E2E Schedule Manager (synthetic)' },
	{ email: 'e2e-schedule-mixed@sigap.local',   appUserId: '00000000-0000-0000-0000-00000000d994', name: 'Local E2E Mixed-Provenance Schedule Actor (synthetic)' },
	{ email: 'e2e-schedule-reader@sigap.local',  appUserId: '00000000-0000-0000-0000-00000000d995', name: 'Local E2E Schedule Reader (synthetic)' },
	{ email: 'org-admin-a@sigap.local',          appUserId: '00000000-0000-0000-0000-00000000c301', name: 'Org A Admin (synthetic)' },
	{ email: 'org-admin-b@sigap.local',          appUserId: '00000000-0000-0000-0000-00000000c302', name: 'Org B Admin (synthetic)' },
	{ email: 'regional-admin@sigap.local',       appUserId: '00000000-0000-0000-0000-00000000c303', name: 'Regional Admin (synthetic)' },
	{ email: 'facility-admin-a1@sigap.local',    appUserId: '00000000-0000-0000-0000-00000000c304', name: 'Facility Admin A1 (synthetic)' },
	{ email: 'operator-a1@sigap.local',          appUserId: '00000000-0000-0000-0000-00000000c305', name: 'Operator A1 (synthetic)' },
	{ email: 'operator-b1@sigap.local',          appUserId: '00000000-0000-0000-0000-00000000c306', name: 'Operator B1 (synthetic)' },
	{ email: 'patient-p1@sigap.local',           appUserId: '00000000-0000-0000-0000-00000000c307', name: 'Citizen Patient P1 (synthetic)' },
];

const passwordFor = (email) => `sigap-local-pw-${email.split('@')[0]}`;

const normalizeEmail = (email) => email.trim().toLowerCase();

async function findBetterAuthUser(authPool, email) {
	const { rows } = await authPool.query('SELECT id FROM "auth"."user" WHERE email = $1 LIMIT 1', [
		normalizeEmail(email),
	]);
	return rows[0]?.id ?? null;
}

async function readAppUser(appPool, appUserId) {
	const { rows } = await appPool.query(
		'SELECT id, email, subject, status, deleted_at FROM app_users WHERE id = $1',
		[appUserId]
	);
	return rows[0] ?? null;
}

async function main() {
	const apply = process.argv.includes('--apply');
	const db = getPool();

	if (!db) {
		console.error(
			'backfill-auth-subjects: no Postgres connection configured ' +
				'(SIGAP_DATABASE_URL / DATABASE_URL are both unset).'
		);
		process.exitCode = 1;
		return;
	}

	const auth = buildAuth(db);
	const results = [];
	let created = 0;
	let reused = 0;
	let subjectWritten = 0;
	let skippedMissing = 0;

	for (const { email, appUserId, name } of MAPPING) {
		let betterAuthUserId = await findBetterAuthUser(db, email);
		const appUser = await readAppUser(db, appUserId);

		if (!appUser) {
			console.warn(`[missing app_user] id=${appUserId} email=${email} — skipped (id not found)`);
			skippedMissing += 1;
			results.push({ email, appUserId, betterAuthUserId, action: 'missing-app_user', oldSubject: null, newSubject: null });
			continue;
		}

		if (appUser.deleted_at !== null) {
			console.warn(`[soft-deleted app_user] id=${appUserId} email=${email} — skipped`);
			skippedMissing += 1;
			results.push({ email, appUserId, betterAuthUserId, action: 'soft-deleted', oldSubject: appUser.subject, newSubject: null });
			continue;
		}

		if (apply && normalizeEmail(appUser.email) !== normalizeEmail(email)) {
			// Data-hygiene guard only: never alters the id-based mapping rule. The
			// subject write remains `WHERE id`; this just flags a stale fixture.
			console.warn(
				`[email drift] app_users.id=${appUserId} lists email=${appUser.email}, ` +
				`fixture email=${email} — writing subject by id regardless`
			);
		}

		if (apply) {
			let action = 'reuse';
			if (betterAuthUserId) {
				reused += 1;
			} else {
				try {
					const res = await auth.api.signUpEmail({
						body: { email, password: passwordFor(email), name },
					});
					betterAuthUserId = res.user.id;
					action = 'create';
					created += 1;
				} catch (err) {
					// Race with a concurrent insert, or any "already exists" outcome:
					// fall back to reading the existing row rather than guessing an id.
					betterAuthUserId = await findBetterAuthUser(db, email);
					if (!betterAuthUserId) {
						console.error(`[signUpEmail failed, and no existing user] email=${email}: ${err?.message ?? err}`);
						process.exitCode = 1;
						results.push({ email, appUserId, betterAuthUserId: null, action: 'error', oldSubject: appUser.subject, newSubject: null });
						continue;
					}
					reused += 1;
				}
			}

			await db.query('UPDATE app_users SET subject = $1, updated_at = now() WHERE id = $2', [
				betterAuthUserId,
				appUserId,
			]);
			subjectWritten += 1;

			results.push({
				email,
				appUserId,
				betterAuthUserId,
				action,
				oldSubject: appUser.subject,
				newSubject: betterAuthUserId,
			});
		} else {
			// Dry-run: report what *would* happen without touching either store.
			results.push({
				email,
				appUserId,
				betterAuthUserId,
				action: betterAuthUserId ? 'reuse' : 'create',
				oldSubject: appUser.subject,
				newSubject: betterAuthUserId ?? '(new better-auth id)',
			});
		}
	}

	// -------------------------------------------------------------------------
	// Summary
	// -------------------------------------------------------------------------
	console.log(apply ? '\n=== APPLY SUMMARY ===' : '\n=== DRY-RUN SUMMARY (no writes made; pass --apply to write) ===');
	console.log(
		[
			'email'.padEnd(34),
			'appUserId'.padEnd(18),
			'action'.padEnd(7),
			'old subject'.padEnd(24),
			'new subject (better-auth id)'.padEnd(24),
		].join('')
	);
	for (const r of results) {
		console.log(
			[
				r.email.padEnd(34),
				r.appUserId.replace('00000000-0000-0000-0000-00000000', '…').padEnd(18),
				r.action.padEnd(7),
				String(r.oldSubject ?? '-').padEnd(24),
				String(r.newSubject ?? '-').padEnd(24),
			].join('')
		);
	}

	console.log('\ncounts:');
	console.log(`  mapped entries        : ${results.length}`);
	console.log(`  better-auth created   : ${created}`);
	console.log(`  better-auth reused    : ${reused}`);
	console.log(`  subject written       : ${subjectWritten}`);
	console.log(`  skipped (missing/del) : ${skippedMissing}`);

	if (!apply) {
		console.log('\nDRY RUN — no Better Auth users created, no app_users.subject changed.');
	}

	await db.end();
}

main().catch(async (err) => {
	console.error('backfill-auth-subjects failed:', err);
	process.exitCode = 1;
});