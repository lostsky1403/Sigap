/**
 * SIGAP staff provisioning — create/ensure the Better Auth account for each
 * reviewed staff identity, map `app_users.subject` to that Better Auth id, and
 * apply the explicit role/scope grants.
 *
 * Companion to `scripts/backfill-auth-subjects.mjs` (which does the same two
 * stores steps for the synthetic citizen + staff fixtures across the Supabase →
 * Better Auth cutover). The difference is what this script owns:
 *
 *   backfill-auth-subjects.mjs  identity only  — auth.user + app_users.subject
 *   provision-staff.mjs         identity + AUTHORIZATION — the same two steps
 *                               PLUS the `user_roles` grant set, plus the
 *                               offboarding paths (deactivate / revoke-roles).
 *
 * Procedure of record: `design/staff-provisioning.md`.
 *
 * NON-NEGOTIABLE RULES (all four are enforced by the code below, not by
 * convention — see `design/staff-provisioning.md` §2):
 *
 *   1. Roles/scopes are NEVER inferred from the email, the display name, or any
 *      ambient signal. They come from the reviewed `STAFF` table below, which
 *      is static data reviewed like code. There is deliberately NO `--role`
 *      flag: ad-hoc privilege grants from a shell are not a supported path.
 *   2. `app_users.subject` is written strictly `WHERE id = <reviewed UUID>` —
 *      never `WHERE email = ...`. Email is used only to locate/create the
 *      Better Auth row, which Better Auth's own schema keys on email.
 *   3. `--apply` refuses to link an app_user to a Better Auth user that already
 *      existed for that email (the `adopt-existing` path) unless
 *      `--adopt-existing` is passed explicitly. Linking whatever account owns
 *      the address would be an email-only takeover primitive; adoption is a
 *      deliberate, auditable act requiring out-of-band verification.
 *   4. Super-admin is not reachable from the public web form. `POST
 *      /auth/register` mints a Better Auth user with NO `app_users` row, so the
 *      Go RBAC resolver (`WHERE u.subject = $1`) matches no actor and grants
 *      nothing. The `super_admin` grant exists in exactly one place: the
 *      reviewed table below.
 *
 * Usage (dry-run is the default for EVERY mode; nothing is written without
 * `--apply`):
 *
 *   node scripts/provision-staff.mjs                        # plan all staff identities
 *   node scripts/provision-staff.mjs --apply                # create/map/grant
 *   node scripts/provision-staff.mjs --apply --log docs/operations/staff-provisioning-log.md
 *   node scripts/provision-staff.mjs --deactivate <email>   # plan: revoke sessions + disable
 *   node scripts/provision-staff.mjs --deactivate <email> --apply
 *   node scripts/provision-staff.mjs --revoke-roles <email> [--apply]
 *   node scripts/provision-staff.mjs --deactivate <email> --revoke-roles <email> --apply
 *
 * Flags:
 *   --apply                 write (default is dry-run)
 *   --deactivate <email>    revoke the account's Better Auth sessions + set
 *                           `app_users.status = 'disabled'` (repeatable)
 *   --revoke-roles <email>  DELETE the account's `user_roles` rows (repeatable)
 *   --adopt-existing        allow linking to a pre-existing Better Auth user
 *                           found for a provisioned staff email (rule 3)
 *   --log <path>            append the run's audit block (markdown) to <path>
 *   --from-env-file         resolve the DB target from the repo `.env` files
 *                           only, ignoring ambient SIGAP_DATABASE_URL /
 *                           DATABASE_URL (see the interlock note below)
 *   --allow-remote-db       allow a non-loopback DB target (staging/prod)
 *   --help                  print usage
 *
 * A lifecycle flag (`--deactivate` / `--revoke-roles`) switches the run to
 * lifecycle mode: the provisioning pass is NOT executed in the same run, so a
 * single invocation never both grants and revokes.
 *
 * Because both scripts repeat the same two config snippets rather than importing
 * `src/lib/server/*` (a bare `node` script cannot resolve the extensionless `./db`
 * import from a stripped `.ts` module), the connection snippet here mirrors
 * `src/lib/server/db.ts` — including its LOCAL-ONLY `sslmode` strip, which the
 * backfill script omitted: Node's `pg` treats `sslmode=require` as verify-full
 * and rejects the dev server's self-signed certificate, so the parameter is
 * removed for a 127.0.0.1/localhost target and `ssl.rejectUnauthorized=false` is
 * set instead. A non-local target keeps the URL untouched (full verification).
 *
 * Idempotent: re-running `--apply` on an already-provisioned database prints the
 * same plan with every change row marked `unchanged` and writes nothing.
 */

import { appendFileSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { userInfo } from 'node:os';
import pg from 'pg';
import { betterAuth } from 'better-auth';
import { PostgresDialect } from 'kysely';

const { Pool } = pg;

const here = dirname(fileURLToPath(import.meta.url));

// ---------------------------------------------------------------------------
// Env files. Read repo-root `.env` (DATABASE_URL / BETTER_AUTH_*) then
// apps/web/.env, and export only keys the ambient environment does not already
// define — EXCEPT the two database variables, whose file values are kept aside
// so the target resolution below can be explicit about which one it used. The
// ambient environment of a developer machine in this monorepo may carry a
// DATABASE_URL for a DIFFERENT project; silently preferring it over the repo's
// own `.env` is how a provisioning run would point at the wrong database.
// ---------------------------------------------------------------------------
const fileEnv = new Map();
for (const envPath of [resolve(here, '../../../.env'), resolve(here, '../.env')]) {
	let raw;
	try {
		raw = readFileSync(envPath, 'utf8');
	} catch {
		continue; // no .env at this path
	}
	for (const line of raw.split(/\r?\n/)) {
		const m = line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$/);
		if (!m) continue;
		let value = m[2];
		if (
			(value.startsWith('"') && value.endsWith('"')) ||
			(value.startsWith("'") && value.endsWith("'"))
		) {
			value = value.slice(1, -1);
		}
		const key = m[1];
		if (key === 'SIGAP_DATABASE_URL' || key === 'DATABASE_URL') {
			fileEnv.set(key, value);
			continue;
		}
		if (process.env[key] === undefined) process.env[key] = value;
	}
}

// ---------------------------------------------------------------------------
// The neighbouring project's local Postgres. This worktree's dev database is
// 127.0.0.1:5433/sigap; 5434 is a different project's data and MUST NOT be
// touched by SIGAP tooling — not read, not written. The guard has no override.
// ---------------------------------------------------------------------------
const FORBIDDEN_LOCAL_PORTS = new Map([
	['5434', "a different project's Postgres (this repo's dev database is 127.0.0.1:5433/sigap)"]
]);

function describeTarget(connectionString) {
	try {
		const u = new URL(connectionString);
		return `${u.hostname}:${u.port || '5432'}${u.pathname}`;
	} catch {
		return '(unparseable connection string)';
	}
}

/**
 * Resolve the Postgres target for this run. Precedence: an ambient
 * `SIGAP_DATABASE_URL` (the canonical name, what the Go API uses), then an
 * ambient `DATABASE_URL`, then the repo `.env` values. `--from-env-file` skips
 * the ambient layer entirely — the escape hatch for a shell that exports
 * `DATABASE_URL` for another project.
 */
function resolveTarget({ fromEnvFile }) {
	const ambient = () =>
		process.env.SIGAP_DATABASE_URL ?? process.env.DATABASE_URL ?? undefined;
	const fromFile = () => fileEnv.get('SIGAP_DATABASE_URL') ?? fileEnv.get('DATABASE_URL');

	const cs = fromEnvFile ? fromFile() : (ambient() ?? fromFile());
	if (!cs) {
		throw new Error(
			'no Postgres connection configured (SIGAP_DATABASE_URL / DATABASE_URL are both unset).'
		);
	}

	const u = new URL(cs);
	const loopback = u.hostname === '127.0.0.1' || u.hostname === 'localhost';
	const forbidden = loopback ? FORBIDDEN_LOCAL_PORTS.get(u.port) : undefined;
	if (forbidden) {
		throw new Error(
			`refusing to run against ${describeTarget(cs)}: port ${u.port} is ${forbidden}.\n` +
				'  Fix one of:\n' +
				'    * set SIGAP_DATABASE_URL to this repo\'s dev database ' +
				'(see .env → DATABASE_URL), or\n' +
				'    * re-run with --from-env-file to resolve the target from the repo .env files only.'
		);
	}
	return { connectionString: cs, loopback, target: describeTarget(cs) };
}

/**
 * Mirrors `src/lib/server/db.ts`: on a loopback target the `sslmode` parameter
 * is stripped and verification is disabled explicitly (the dev server presents a
 * self-signed certificate). Anywhere else the URL is used untouched.
 */
function createPool({ connectionString, loopback }) {
	const local = loopback || /@(127\.0\.0\.1|localhost):/.test(connectionString);
	// An EXPLICIT sslmode=disable is authoritative even on loopback: CI service
	// containers and other plain-TCP databases are reachable this way. This
	// mirrors src/lib/server/db.ts exactly — only the self-signed local dev path
	// gets relaxed verification. Production URLs are never touched.
	const sslDisabled = /[?&]sslmode=disable(?:&|$)/i.test(connectionString);
	const stripped = local
		? connectionString.replace(/([?&])sslmode=[^&]+(&?)/, (_m, lead, trail) => (trail ? lead : ''))
		: connectionString;
	return new Pool({
		connectionString: stripped,
		ssl: local && !sslDisabled ? { rejectUnauthorized: false } : undefined
	});
}

// ---------------------------------------------------------------------------
// `auth` — mirrors src/lib/server/auth.better.ts (minus the token-minting jwt
// plugin and the SvelteKit cookie plumbing, neither of which affects user
// provisioning, session revocation, or credential creation).
// ---------------------------------------------------------------------------
function buildAuth(pool) {
	return betterAuth({
		baseURL: process.env.BETTER_AUTH_URL || 'http://127.0.0.1:5173',
		database: {
			dialect: new PostgresDialect({ pool }),
			type: 'postgres',
			schemaName: 'auth'
		},
		emailAndPassword: { enabled: true }
	});
}

// ---------------------------------------------------------------------------
// Scope constants, by seed provenance. UUIDs are the deterministic fixture ids
// the seeds use; the labels are what the audit table prints.
//
//   d000  Sigap Demo Facility            packages/db/seed/demo.sql
//   e000  RSUD Kota Sehat                packages/db/seed/dev.sql
//   c001  RS Sehat Nusantara Group (org) packages/db/seed/regional_tenancy.sql
//   c002  Puskesmas Prima Sejahtera Network (org)
//   c101  RS Sehat Nusantara Cimahi      (org A)   packages/db/seed/regional_tenancy.sql
//   c103  Puskesmas Prima Tangerang      (org B)
// ---------------------------------------------------------------------------
const SCOPE = {
	demoFacility: { facility: '00000000-0000-0000-0000-00000000d000', label: 'Sigap Demo Facility' },
	kotaSehatFacility: { facility: '00000000-0000-0000-0000-00000000e000', label: 'RSUD Kota Sehat' },
	orgA: { organization: '00000000-0000-0000-0000-00000000c001', label: 'RS Sehat Nusantara Group' },
	orgB: { organization: '00000000-0000-0000-0000-00000000c002', label: 'Puskesmas Prima Sejahtera Network' },
	cimahiFacility: { facility: '00000000-0000-0000-0000-00000000c101', label: 'RS Sehat Nusantara Cimahi' },
	tangerangFacility: { facility: '00000000-0000-0000-0000-00000000c103', label: 'Puskesmas Prima Tangerang' }
};

const grant = (role, scope) => ({
	role,
	facility: scope?.facility ?? null,
	organization: scope?.organization ?? null,
	scopeLabel: scope?.label ?? 'unscoped'
});

// ---------------------------------------------------------------------------
// THE REVIEWED STAFF TABLE.
//
// Every entry is a reviewed `{ email, appUserId }` pair — `appUserId` is the
// STABLE SIGAP id from the seeds, and the subject write below is keyed on it.
// `grants` mirrors the seed's `user_roles` rows EXACTLY (same role, same
// facility/organization axis), so the script is a no-op against a seeded
// database and the authority for a database that has not been seeded.
//
// Changing a staff member's role or scope = editing this table (code review) and
// re-running `--apply`. There is no other supported path.
// ---------------------------------------------------------------------------
const STAFF = [
	{
		email: 'global-admin@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d990',
		name: 'Local Global Super Admin (synthetic)',
		grants: [grant('super_admin')],
		source: 'seed/dev.sql:41 + seed/demo.sql:95'
	},
	{
		email: 'facility-admin@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d991',
		name: 'Local Facility Admin (synthetic)',
		grants: [grant('facility_admin', SCOPE.demoFacility)],
		source: 'seed/dev.sql:42 + seed/demo.sql:96'
	},
	{
		email: 'zero-scope-admin@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d992',
		name: 'Local Zero-Scope Admin (synthetic)',
		// Deliberate negative fixture: facility_admin with NO scope at all, so
		// the facility-scope resolver returns nothing. Do not "fix" this row by
		// giving it a facility; the empty-scope UI state depends on it.
		grants: [grant('facility_admin')],
		source: 'seed/dev.sql:43 + seed/demo.sql:97'
	},
	{
		email: 'e2e-schedule-manager@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d993',
		name: 'Local E2E Schedule Manager (synthetic)',
		grants: [grant('facility_admin', SCOPE.demoFacility)],
		source: 'seed/dev.sql:114 + seed/demo.sql:101'
	},
	{
		email: 'e2e-schedule-mixed@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d994',
		name: 'Local E2E Mixed-Provenance Schedule Actor (synthetic)',
		// Mixed provenance on purpose: operator (schedule.read, no
		// schedule.manage) at facility A, facility_admin (schedule.manage) at
		// facility B. One row per (user, role) is the whole point of the
		// user_roles primary key; collapsing the two facilities would delete
		// the regression this actor exists to catch.
		grants: [
			grant('operator', SCOPE.kotaSehatFacility),
			grant('facility_admin', SCOPE.demoFacility)
		],
		source: 'seed/dev.sql:115 + seed/demo.sql:116-117'
	},
	{
		email: 'e2e-schedule-reader@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000d995',
		name: 'Local E2E Schedule Reader (synthetic)',
		grants: [grant('operator', SCOPE.demoFacility)],
		source: 'seed/dev.sql:116 + seed/demo.sql:147'
	},
	{
		email: 'org-admin-a@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c301',
		name: 'Org A Admin (synthetic)',
		grants: [grant('organization_admin', SCOPE.orgA)],
		source: 'seed/regional_tenancy.sql:147,186'
	},
	{
		email: 'org-admin-b@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c302',
		name: 'Org B Admin (synthetic)',
		grants: [grant('organization_admin', SCOPE.orgB)],
		source: 'seed/regional_tenancy.sql:148,188'
	},
	{
		email: 'regional-admin@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c303',
		name: 'Regional Admin (synthetic)',
		// Unscoped regional_admin: tenancy directory manage + read-only
		// operational stats, and deliberately no queue/facility/appointment
		// manage (the seed asserts this).
		grants: [grant('regional_admin')],
		source: 'seed/regional_tenancy.sql:149,190'
	},
	{
		email: 'facility-admin-a1@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c304',
		name: 'Facility Admin A1 (synthetic)',
		grants: [grant('facility_admin', SCOPE.cimahiFacility)],
		source: 'seed/regional_tenancy.sql:150,192'
	},
	{
		email: 'operator-a1@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c305',
		name: 'Operator A1 (synthetic)',
		grants: [grant('operator', SCOPE.cimahiFacility)],
		source: 'seed/regional_tenancy.sql:151,194'
	},
	{
		email: 'operator-b1@sigap.local',
		appUserId: '00000000-0000-0000-0000-00000000c306',
		name: 'Operator B1 (synthetic)',
		grants: [grant('operator', SCOPE.tangerangFacility)],
		source: 'seed/regional_tenancy.sql:152,196'
	}
];

/**
 * Synthetic local password. Mirrors `backfill-auth-subjects.mjs`; the account
 * holder is expected to change it (POST /api/auth/change-password is enabled).
 * `SIGAP_STAFF_PASSWORD` overrides it for a non-local bootstrap. NEVER printed.
 */
const passwordFor = (email) =>
	process.env.SIGAP_STAFF_PASSWORD ?? `sigap-local-pw-${email.split('@')[0]}`;

const normalizeEmail = (email) => String(email ?? '').trim().toLowerCase();
const shortId = (id) => (id ? `…${String(id).slice(-4)}` : '(none)');
const shortAuthId = (id) => (id ? `…${String(id).slice(-10)}` : '(absent)');
const grantKey = (role, facility, organization) =>
	`${role}|${String(facility ?? '').toLowerCase()}|${String(organization ?? '').toLowerCase()}`;
const scopeOf = (facility, organization) =>
	organization ? `org:${shortId(organization)}` : facility ? `facility:${shortId(facility)}` : 'global';

// ---------------------------------------------------------------------------
// Small DB helpers. All keyed on ids, never on email equality.
// ---------------------------------------------------------------------------
async function readAppUser(pool, appUserId) {
	const { rows } = await pool.query(
		'SELECT id, email, display_name, subject, status, deleted_at FROM app_users WHERE id = $1',
		[appUserId]
	);
	return rows[0] ?? null;
}

async function readAppUserByEmail(pool, email) {
	const { rows } = await pool.query(
		`SELECT id, email, display_name, subject, status, deleted_at
		 FROM app_users WHERE lower(email) = lower($1) ORDER BY id`,
		[normalizeEmail(email)]
	);
	return rows;
}

/**
 * Grants as stored, keyed for set comparison. Only ACTIVE, non-deleted rows
 * count as grants — the same filter `rbac_resolver.go` applies — but the
 * lifecycle columns are returned too so drift can be reported honestly.
 */
async function readGrants(pool, appUserId) {
	const { rows } = await pool.query(
		`SELECT r.name AS role, ur.facility_id, ur.organization_id, ur.status, ur.deleted_at
		 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		 WHERE ur.user_id = $1
		 ORDER BY r.name, ur.facility_id NULLS FIRST, ur.organization_id NULLS FIRST`,
		[appUserId]
	);
	return rows;
}

async function findBetterAuthUserByEmail(pool, email) {
	const { rows } = await pool.query(
		'SELECT id, email, "createdAt" FROM "auth"."user" WHERE lower(email) = lower($1) LIMIT 1',
		[normalizeEmail(email)]
	);
	return rows[0] ?? null;
}

async function findBetterAuthUserById(pool, id) {
	const { rows } = await pool.query(
		'SELECT id, email, "createdAt" FROM "auth"."user" WHERE id = $1 LIMIT 1',
		[id]
	);
	return rows[0] ?? null;
}

/** The app_user currently claiming this Better Auth id as its subject. */
async function findSubjectClaimant(pool, betterAuthUserId) {
	const { rows } = await pool.query(
		`SELECT id, email, status, deleted_at FROM app_users
		 WHERE subject = $1 AND deleted_at IS NULL`,
		[betterAuthUserId]
	);
	return rows[0] ?? null;
}

async function countSessions(pool, betterAuthUserId) {
	const { rows } = await pool.query(
		'SELECT count(*)::int AS n FROM "auth"."session" WHERE "userId" = $1',
		[betterAuthUserId]
	);
	return rows[0].n;
}

/**
 * Revoke every session of a Better Auth user.
 *
 * `POST /api/auth/revoke-sessions` cannot be used from a CLI: it is
 * `sensitiveSessionMiddleware`-gated and revokes the CALLER's sessions, and a
 * script has no caller session. This calls the exact adapter method that
 * endpoint calls (`ctx.context.internalAdapter.deleteUserSessions(userId)`,
 * better-auth 1.7.20 `dist/api/routes/session.mjs` → `internal-adapter.mjs`), so
 * the semantics are the library's own, not a hand-rolled reimplementation.
 */
async function revokeAllSessions(auth, betterAuthUserId) {
	const ctx = await auth.$context;
	if (typeof ctx?.internalAdapter?.deleteUserSessions !== 'function') {
		throw new Error(
			'internalAdapter.deleteUserSessions is unavailable in this better-auth build; ' +
				'refusing to guess a revocation path.'
		);
	}
	await ctx.internalAdapter.deleteUserSessions(betterAuthUserId);
}

// ---------------------------------------------------------------------------
// Audit surface.
// ---------------------------------------------------------------------------
const changes = [];
const errors = [];

function recordChange(entity, key, oldValue, newValue) {
	changes.push({
		entity,
		key,
		old: oldValue === undefined || oldValue === null || oldValue === '' ? '(none)' : String(oldValue),
		new: newValue === undefined || newValue === null || newValue === '' ? '(none)' : String(newValue),
		changed: String(oldValue ?? '') !== String(newValue ?? '')
	});
}

function fail(message) {
	errors.push(message);
	console.error(`  !! ${message}`);
}

function renderTable(headers, rows) {
	const widths = headers.map((h, i) =>
		Math.min(64, Math.max(h.length, ...rows.map((r) => String(r[i] ?? '').length)))
	);
	const line = (cells) => cells.map((c, i) => String(c ?? '').slice(0, widths[i]).padEnd(widths[i])).join('  ');
	return [line(headers), line(widths.map((w) => '-'.repeat(w))), ...rows.map(line)];
}

function renderChangeLog() {
	const rows = changes.map((c, i) => [String(i + 1), c.entity, c.key, c.old, c.new]);
	return renderTable(['#', 'entity', 'identity', 'old', 'new'], rows);
}

function renderMarkdownLog({ target, mode, operator }) {
	const stamp = new Date().toISOString();
	const lines = [
		'',
		`### ${stamp} — provision-staff (${mode}) — \`${target}\``,
		'',
		`- operator: \`${operator}\``,
		`- reviewed identities: ${STAFF.length} staff`,
		`- result: ${errors.length === 0 ? 'ok' : `${errors.length} error(s)`}`,
		'',
		'| # | entity | identity | old | new |',
		'|---|---|---|---|---|'
	];
	changes.forEach((c, i) => {
		lines.push(`| ${i + 1} | \`${c.entity}\` | ${c.key} | ${c.old} | ${c.new} |`);
	});
	if (errors.length > 0) {
		lines.push('', 'Errors:', ...errors.map((e) => `- ${e}`));
	}
	lines.push('');
	return lines.join('\n');
}

// ---------------------------------------------------------------------------
// Argument parsing.
// ---------------------------------------------------------------------------
function parseArgs(argv) {
	const opts = {
		apply: false,
		deactivate: [],
		revokeRoles: [],
		adoptExisting: false,
		log: null,
		fromEnvFile: false,
		allowRemoteDb: false,
		help: false
	};
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		const needsValue = (name) => {
			const value = argv[i + 1];
			if (value === undefined || value.startsWith('--')) {
				throw new Error(`${name} needs a value (e.g. ${name} user@sigap.local)`);
			}
			i += 1;
			return value;
		};
		switch (arg) {
			case '--apply':
				opts.apply = true;
				break;
			case '--deactivate':
				opts.deactivate.push(needsValue('--deactivate'));
				break;
			case '--revoke-roles':
				opts.revokeRoles.push(needsValue('--revoke-roles'));
				break;
			case '--adopt-existing':
				opts.adoptExisting = true;
				break;
			case '--log':
				opts.log = needsValue('--log');
				break;
			case '--from-env-file':
				opts.fromEnvFile = true;
				break;
			case '--allow-remote-db':
				opts.allowRemoteDb = true;
				break;
			case '--help':
			case '-h':
				opts.help = true;
				break;
			default:
				throw new Error(`unknown argument: ${arg}`);
		}
	}
	return opts;
}

function printUsage() {
	console.log(`SIGAP staff provisioning — see design/staff-provisioning.md for the procedure.

  node scripts/provision-staff.mjs [--apply] [--log <path>] [--from-env-file] [--allow-remote-db]
  node scripts/provision-staff.mjs --deactivate <email> [--apply]
  node scripts/provision-staff.mjs --revoke-roles <email> [--apply]
  node scripts/provision-staff.mjs --deactivate <email> --revoke-roles <email> [--apply]

  --apply              write. Without it every mode is a dry-run.
  --deactivate         revoke Better Auth sessions + app_users.status = 'disabled'
  --revoke-roles       DELETE the account's user_roles rows
  --adopt-existing     allow linking to a pre-existing Better Auth user (audited)
  --log <path>         append the audit block to <path>
  --from-env-file      resolve the DB target from the repo .env files only
  --allow-remote-db    allow a non-loopback DB target

Roles and scopes come from the reviewed STAFF table in this file. There is no
--role flag: privilege grants are never made ad hoc from a shell.`);
}

// ---------------------------------------------------------------------------
// Mode 1 — provisioning.
// ---------------------------------------------------------------------------
async function provisionIdentity(pool, auth, entry, opts, roleIdByName, scopeIndex) {
	const email = normalizeEmail(entry.email);
	console.log(`\n[${entry.email}]`);
	console.log(`  app_user     : ${shortId(entry.appUserId)}  ${entry.name}   (${entry.source})`);

	// (a) The app_users row must already exist. It is created by a seed or a
	// reviewed migration with an app-owned UUID; this script never invents one,
	// because `app_users.id` is the durable id every FK hangs off.
	const appUser = await readAppUser(pool, entry.appUserId);
	if (!appUser) {
		fail(`missing app_user id=${entry.appUserId} email=${entry.email} — run the seeds first; skipped`);
		return;
	}
	if (appUser.deleted_at !== null) {
		fail(`app_user ${shortId(appUser.id)} is soft-deleted — skipped`);
		return;
	}
	const currentLabel = `${appUser.status}${appUser.display_name ? `, ${appUser.display_name}` : ''}`;
	console.log(`  status       : ${currentLabel}`);
	if (normalizeEmail(appUser.email) !== email) {
		// Data hygiene only — never a linking rule. The subject write below is
		// `WHERE id`; a stale fixture email is printed, never trusted.
		console.log(
			`  note         : app_users.email is "${appUser.email}", reviewed table lists "${entry.email}" (email drift; subject is written by id regardless)`
		);
	}
	if (appUser.status !== 'active') {
		console.log('  note         : app_user is not active — subject/grants are written but authorize nothing until it is re-activated');
	}

	// (b) Resolve the reviewed grants against the live role/scope directory.
	const desiredGrants = [];
	for (const g of entry.grants) {
		const roleId = roleIdByName.get(g.role);
		if (!roleId) {
			fail(`role "${g.role}" does not exist — run packages/db/seed/rbac.sql + regional_tenancy.sql first; skipped`);
			return;
		}
		if (g.facility && !scopeIndex.facilities.has(g.facility)) {
			fail(`facility ${g.facility} not found — run packages/db/seed/dev.sql + demo.sql first; skipped`);
			return;
		}
		if (g.organization && !scopeIndex.organizations.has(g.organization)) {
			fail(`organization ${g.organization} not found — run packages/db/seed/regional_tenancy.sql first; skipped`);
			return;
		}
		desiredGrants.push({
			...g,
			roleId,
			scopeLabel: g.organization
				? `org:${shortId(g.organization)} (${g.scopeLabel})`
				: g.facility
					? `facility:${shortId(g.facility)} (${g.scopeLabel})`
					: 'global (unscoped)'
		});
	}

	const storedGrants = await readGrants(pool, entry.appUserId);
	const activeStored = storedGrants.filter((r) => r.status === 'active' && r.deleted_at === null);
	const storedKeys = new Set(activeStored.map((r) => grantKey(r.role, r.facility_id, r.organization_id)));
	const desiredKeys = new Set(desiredGrants.map((g) => grantKey(g.role, g.facility, g.organization)));

	const grantsToAdd = desiredGrants.filter((g) => !storedKeys.has(grantKey(g.role, g.facility, g.organization)));
	const grantsExtra = activeStored.filter((r) => !desiredKeys.has(grantKey(r.role, r.facility_id, r.organization_id)));

	console.log(
		`  grants (now) : ${activeStored.length === 0 ? '(none)' : activeStored.map((r) => `${r.role}@${scopeOf(r.facility_id, r.organization_id)}`).join(', ')}`
	);
	console.log(
		`  grants (new) : ${desiredGrants.length === 0 ? '(none)' : desiredGrants.map((g) => `${g.role}@${scopeOf(g.facility, g.organization)}`).join(', ')}`
	);
	if (entry.grants.some((g) => g.role === 'super_admin')) {
		console.log('  warning      : super_admin grant — full system access; this is the only supported path to it');
	}
	for (const extra of grantsExtra) {
		// Reported, never silently deleted: an extra grant may be a deliberate
		// manual addition that needs review, and de-privileging on the fly is as
		// dangerous as privileging.
		console.log(
			`  drift        : extra active grant ${extra.role}@${scopeOf(extra.facility_id, extra.organization_id)} is NOT in the reviewed table — left in place (use --revoke-roles to remove)`
		);
		recordChange('user_roles', `extra grant ${email} ${shortId(entry.appUserId)}`, `${extra.role}@${scopeOf(extra.facility_id, extra.organization_id)}`, 'kept (drift reported)');
	}

	// (c) Better Auth account.
	let betterAuthUser = await findBetterAuthUserByEmail(pool, email);
	let authAction;
	if (!betterAuthUser) {
		authAction = 'create';
	} else {
		const claimant = await findSubjectClaimant(pool, betterAuthUser.id);
		if (claimant && claimant.id !== entry.appUserId) {
			fail(
				`Better Auth user ${shortAuthId(betterAuthUser.id)} for ${entry.email} is already claimed as the subject of ` +
					`app_user ${shortId(claimant.id)} (${claimant.email ?? 'no email'}) — refusing (fail closed); ` +
					'resolve manually before provisioning'
			);
			return;
		}
		authAction = appUser.subject === betterAuthUser.id ? 'already-linked' : 'adopt-existing';
	}
	console.log(
		`  better-auth  : ${
			authAction === 'create'
				? '(absent — will be created)'
				: `${shortAuthId(betterAuthUser.id)} (${authAction}, created ${new Date(betterAuthUser.createdAt).toISOString()})`
		}`
	);

	const desiredSubject = betterAuthUser?.id ?? '(new better-auth id)';
	console.log(`  subject      : ${appUser.subject ?? '(none)'}  →  ${desiredSubject}`);
	if (!opts.apply && authAction === 'adopt-existing' && !opts.adoptExisting) {
		console.log('  warning      : this is a pre-existing Better Auth account — --apply will REFUSE without --adopt-existing (takeover guard)');
	}

	if (opts.apply && authAction !== 'already-linked') {
		if (authAction === 'adopt-existing' && !opts.adoptExisting) {
			// Rule 3: the account owning the address is whoever registered it
			// first. Linking it silently would be an email-only takeover.
			fail(
				`refusing to adopt the pre-existing Better Auth user ${shortAuthId(betterAuthUser.id)} for ${entry.email}. ` +
					'Verify out of band that this account belongs to the intended person, then re-run with --adopt-existing (the adoption is logged). ' +
					'If that account is junk from an earlier local script run and nobody can sign in with it, delete it from the auth schema (or drop the auth schema) and re-run.'
			);
			return;
		}
		if (authAction === 'adopt-existing') {
			console.log('  warning      : ADOPTING a pre-existing Better Auth account for a provisioned staff email (explicitly requested)');
		}
		if (authAction === 'create') {
			try {
				const res = await auth.api.signUpEmail({
					body: { email: entry.email, password: passwordFor(email), name: entry.name }
				});
				betterAuthUser = res?.user?.id ? res.user : await findBetterAuthUserByEmail(pool, email);
				if (!betterAuthUser) {
					fail(`signUpEmail returned no user id for ${entry.email}`);
					return;
				}
			} catch (err) {
				// Race with a concurrent insert, or an "already exists" outcome:
				// re-read rather than guessing an id.
				betterAuthUser = await findBetterAuthUserByEmail(pool, email);
				if (!betterAuthUser) {
					fail(`signUpEmail failed for ${entry.email} and no user exists: ${err?.message ?? err}`);
					return;
				}
			}
			// sign-up auto-creates a session. The CLI never uses that token, so
			// discard it — provisioning must not leave live sessions behind.
			const straySessions = await countSessions(pool, betterAuthUser.id);
			if (straySessions > 0) {
				await revokeAllSessions(auth, betterAuthUser.id);
				recordChange('auth.session', `sign-up session ${email}`, `${straySessions} live`, '0 live (discarded)');
				console.log(`  note         : discarded ${straySessions} sign-up session(s) for the new account`);
			}
		}
		const res = await pool.query(
			'UPDATE app_users SET subject = $1, updated_at = now() WHERE id = $2',
			[betterAuthUser.id, entry.appUserId]
		);
		if (res.rowCount !== 1) {
			fail(`subject write affected ${res.rowCount} rows for app_user ${shortId(entry.appUserId)} — expected exactly 1`);
			return;
		}
	}

	if (opts.apply && grantsToAdd.length > 0) {
		for (const g of grantsToAdd) {
			await pool.query(
				`INSERT INTO user_roles (user_id, role_id, facility_id, organization_id, status, deleted_at)
				 VALUES ($1, $2, $3, $4, 'active', NULL)
				 ON CONFLICT (user_id, role_id) DO UPDATE SET
				   facility_id = EXCLUDED.facility_id,
				   organization_id = EXCLUDED.organization_id,
				   status = EXCLUDED.status,
				   deleted_at = EXCLUDED.deleted_at`,
				[entry.appUserId, g.roleId, g.facility, g.organization]
			);
		}
	}

	// (d) Audit rows — one per changed fact. Values are STATES (ids), not verbs,
	// so a no-op re-run prints `old == new` for every row it did not change.
	const auditKey = `${email} ${shortId(entry.appUserId)}`;
	recordChange(
		'auth.user',
		auditKey,
		betterAuthUser ? betterAuthUser.id : '(absent)',
		betterAuthUser ? betterAuthUser.id : opts.apply ? '(created by this run)' : '(would be created)'
	);
	if (authAction === 'adopt-existing') {
		recordChange(
			'auth.user.adoption',
			auditKey,
			'pre-existing account, not linked to this app_user',
			opts.apply ? 'linked (--adopt-existing)' : 'would be linked (on --apply --adopt-existing)'
		);
	}
	recordChange(
		'app_users.subject',
		`${email} ${shortId(entry.appUserId)}`,
		appUser.subject ?? '(none)',
		betterAuthUser?.id ?? (opts.apply ? 'created by this run' : 'new better-auth id')
	);
	for (const g of grantsToAdd) {
		recordChange(
			'user_roles',
			`${email} ${shortId(entry.appUserId)}`,
			'(absent)',
			`+${g.role} @ ${g.scopeLabel}${opts.apply ? '' : ' (on --apply)'}`
		);
	}
	if (grantsToAdd.length === 0 && grantsExtra.length === 0) {
		const label = desiredGrants.map((g) => `${g.role}@${scopeOf(g.facility, g.organization)}`).join(', ');
		recordChange('user_roles', `${email} ${shortId(entry.appUserId)}`, label || '(none)', label || '(none)');
	}
	console.log(
		`  action       : ${opts.apply ? 'applied' : 'planned'} — auth=${authAction}, subject=${appUser.subject === betterAuthUser?.id ? 'unchanged' : 'update'}, grants +${grantsToAdd.length}${grantsExtra.length ? `, drift ${grantsExtra.length}` : ''}`
	);
}

async function runProvisioning(pool, auth, opts) {
	const roleRows = await pool.query('SELECT id, name FROM roles');
	const roleIdByName = new Map(roleRows.rows.map((r) => [r.name, r.id]));
	const facilityRows = await pool.query('SELECT id, name FROM facilities');
	const orgRows = await pool.query('SELECT id, name FROM organizations');
	const scopeIndex = {
		facilities: new Set(facilityRows.rows.map((r) => r.id)),
		organizations: new Set(orgRows.rows.map((r) => r.id))
	};

	for (const entry of STAFF) {
		await provisionIdentity(pool, auth, entry, opts, roleIdByName, scopeIndex);
	}
}

// ---------------------------------------------------------------------------
// Mode 2/3 — lifecycle (deactivate / revoke-roles).
//
// Email is an acceptable LOOKUP key here — and only here — because both
// operations strictly remove access. Granting never resolves by email.
// ---------------------------------------------------------------------------
async function resolveLifecycleTarget(pool, email) {
	const norm = normalizeEmail(email);
	const listed = STAFF.find((e) => normalizeEmail(e.email) === norm);
	if (listed) {
		const appUser = await readAppUser(pool, listed.appUserId);
		if (!appUser) throw new Error(`app_user id=${listed.appUserId} (${email}) not found`);
		return { appUser, listed: true };
	}
	const rows = await readAppUserByEmail(pool, email);
	if (rows.length === 0) throw new Error(`no app_users row for ${email}`);
	if (rows.length > 1) throw new Error(`${rows.length} app_users rows for ${email} — refusing to guess`);
	return { appUser: rows[0], listed: false };
}

async function runDeactivate(pool, auth, email, opts) {
	const { appUser, listed } = await resolveLifecycleTarget(pool, email);
	const label = `${normalizeEmail(email)} ${shortId(appUser.id)}${listed ? '' : ' (not in the reviewed staff table; email lookup)'}`;
	console.log(`\n[deactivate] ${label}`);

	const authUser =
		(appUser.subject && (await findBetterAuthUserById(pool, appUser.subject))) ??
		(await findBetterAuthUserByEmail(pool, email));

	if (authUser) {
		const before = await countSessions(pool, authUser.id);
		console.log(`  better-auth  : ${shortAuthId(authUser.id)} — sessions ${before} → ${opts.apply ? '0 (revoked)' : '0 (would be revoked)'}`);
		if (opts.apply) {
			await revokeAllSessions(auth, authUser.id);
			const after = await countSessions(pool, authUser.id);
			if (after !== 0) fail(`session revocation incomplete: ${after} session(s) still present for ${shortAuthId(authUser.id)}`);
			recordChange('auth.session', label, `${before} live`, `${after} live (revoked)`);
		} else {
			recordChange('auth.session', label, `${before} live`, '0 live (would be revoked)');
		}
	} else {
		console.log('  better-auth  : no account found (nothing to revoke)');
		recordChange('auth.session', label, '(no better-auth user)', '(nothing to revoke)');
	}

	console.log(`  status       : ${appUser.status} → disabled`);
	if (opts.apply) {
		await pool.query(
			`UPDATE app_users SET status = 'disabled', updated_at = now() WHERE id = $1`,
			[appUser.id]
		);
	}
	recordChange('app_users.status', label, appUser.status, opts.apply ? 'disabled' : 'disabled (on --apply)');

	const remaining = (await readGrants(pool, appUser.id)).filter((r) => r.status === 'active' && r.deleted_at === null);
	if (remaining.length > 0) {
		console.log(
			`  note         : ${remaining.length} active grant(s) remain (${remaining.map((r) => `${r.role}@${scopeOf(r.facility_id, r.organization_id)}`).join(', ')}) — ` +
				'permissions already resolve to zero for a disabled app_user, but --revoke-roles removes the rows as well'
		);
	}
	console.log(
		'  note         : this deactivation sets app_users.status=disabled and revokes every Better Auth session; the\n' +
			'                 session.create.before gate (identity-status.ts) now refuses ANY new session for a disabled,\n' +
			'                 deleted, missing or unverified identity - so a deactivated account can no longer sign in at all.\n' +
			'                 An already-minted JWT still verifies to its 1 h exp (no zero-second revocation) but authorizes\n' +
			'                 nothing because Go re-resolves app_users.status per request - design/session-lifecycle-policy.md §2.1, §3.'
	);
}

async function runRevokeRoles(pool, email, opts) {
	const { appUser, listed } = await resolveLifecycleTarget(pool, email);
	const label = `${normalizeEmail(email)} ${shortId(appUser.id)}${listed ? '' : ' (not in the reviewed staff table; email lookup)'}`;
	console.log(`\n[revoke-roles] ${label}`);

	const rows = await readGrants(pool, appUser.id);
	if (rows.length === 0) {
		console.log('  user_roles   : (none) — nothing to remove');
		recordChange('user_roles', label, '(none)', '(none)');
		return;
	}
	for (const r of rows) {
		const scope = scopeOf(r.facility_id, r.organization_id);
		console.log(`  user_roles   : ${r.role}@${scope} (${r.status}${r.deleted_at ? ', soft-deleted' : ''}) → removed`);
		recordChange('user_roles', label, `${r.role}@${scope} (${r.status})`, opts.apply ? 'removed' : 'removed on --apply');
	}
	if (opts.apply) {
		const res = await pool.query('DELETE FROM user_roles WHERE user_id = $1', [appUser.id]);
		const left = await readGrants(pool, appUser.id);
		if (left.length !== 0) fail(`role revocation incomplete: ${left.length} row(s) still present for ${shortId(appUser.id)}`);
		console.log(`  user_roles   : ${res.rowCount} row(s) deleted, ${left.length} remaining`);
	}
	console.log(
		'  note         : permission revocation is immediate — the Go API resolves RBAC from the database on every request.\n' +
			'                 Already-minted JWTs keep authenticating the SUBJECT for up to 1 h, so pair this with --deactivate for offboarding.'
	);
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------
async function main() {
	let opts;
	try {
		opts = parseArgs(process.argv.slice(2));
	} catch (err) {
		console.error(`provision-staff: ${err.message}`);
		process.exitCode = 1;
		return;
	}
	if (opts.help) {
		printUsage();
		return;
	}

	let targetInfo;
	try {
		targetInfo = resolveTarget(opts);
		if (!targetInfo.loopback && !opts.allowRemoteDb) {
			throw new Error(
				`refusing a non-loopback target (${targetInfo.target}) without --allow-remote-db`
			);
		}
	} catch (err) {
		console.error(`provision-staff: ${err.message}`);
		process.exitCode = 1;
		return;
	}
	const mode =
		opts.deactivate.length + opts.revokeRoles.length > 0
			? `${opts.apply ? 'APPLY' : 'DRY RUN'} (lifecycle)`
			: `${opts.apply ? 'APPLY' : 'DRY RUN'} (provisioning)`;

	console.log('SIGAP staff provisioning');
	console.log(`  target   : ${targetInfo.target}${targetInfo.loopback ? '  (local)' : '  (REMOTE)'}`);
	console.log(`  mode     : ${mode}${opts.apply ? '' : '  — no writes; pass --apply to write'}`);
	console.log(`  reviewed : ${STAFF.length} staff identities (roles/scopes are static in this script; there is no --role flag)`);

	const pool = createPool(targetInfo);
	const auth = buildAuth(pool);

	try {
		if (opts.deactivate.length + opts.revokeRoles.length > 0) {
			for (const email of opts.revokeRoles) await runRevokeRoles(pool, email, opts);
			for (const email of opts.deactivate) await runDeactivate(pool, auth, email, opts);
		} else {
			await runProvisioning(pool, auth, opts);
		}
	} catch (err) {
		errors.push(err?.message ?? String(err));
		console.error(`\n!! ${err?.stack ?? err}`);
	} finally {
		console.log(`\n=== CHANGE LOG (old → new) — ${mode} ===`);
		for (const line of renderChangeLog()) console.log(line);
		console.log(
			`\ncounts: identities=${STAFF.length}  changed=${changes.filter((c) => c.changed).length}  audit rows=${changes.length}  errors=${errors.length}`
		);
		if (!opts.apply) {
			console.log('DRY RUN — no Better Auth users created, no subject changed, no grants or statuses written.');
		}
		if (opts.log) {
			const operator = process.env.SIGAP_PROVISION_OPERATOR ?? userInfo().username;
			appendFileSync(
				opts.log,
				renderMarkdownLog({ target: targetInfo.target, mode, operator }),
				'utf8'
			);
			console.log(`audit block appended to ${opts.log} (operator: ${operator})`);
		}
		await pool.end();
	}

	if (errors.length > 0) process.exitCode = 1;
}

main().catch(async (err) => {
	console.error('provision-staff failed:', err);
	process.exitCode = 1;
});
