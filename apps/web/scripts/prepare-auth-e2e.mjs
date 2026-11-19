/**
 * SIGAP self-hosted-auth E2E preparation — put the local database into the exact
 * state `e2e/selfhosted-auth-journey.spec.ts` asserts against.
 *
 * WHY THIS SCRIPT EXISTS
 *
 * Under `scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor` the Go API takes
 * its identity from the verified Better Auth JWT ONLY (no dev actor, no
 * X-Sigap-Local-Test-Subject). The RBAC resolver keys on `app_users.subject`
 * (`apps/api/internal/auth/rbac_resolver.go::permissionsQuery` →
 * `WHERE u.subject = $1`), so a signed-in Better Auth account authorizes
 * nothing until its `sub` (the `auth.user.id`) is written into the
 * `app_users` row that owns the identity. There are four reviewed actors in
 * the journey spec, and three of the four need that write:
 *
 *   smoke@sigap.local        → patient-a row   (2 owned appointments)
 *   smoke-b@sigap.local      → smoke-b row     (already mapped; 0 appointments)
 *   org-a-admin@sigap.local  → c301 org A admin
 *   global-admin@sigap.local → d990 super admin
 *
 * The second job is the CITIZEN FIXTURE PRE-STATE. The spec cancels a real
 * appointment through the UI, which is a one-way transition: after a run, that
 * appointment is `cancelled` and a second run has nothing eligible to cancel
 * (the page renders a status badge, not a `Batalkan` button). So the two
 * reviewed fixture appointments owned by patient A are restored to their
 * deterministic pre-state — `scheduled`, `cancelled_at = NULL` — before each
 * run. Nothing else about them (id, facility, time, check-in code) is touched.
 *
 * NON-NEGOTIABLE RULES (enforced by code below, not by convention):
 *
 *   1. The script is a FIXTURE script for one local database. It refuses port
 *      5434 (a different project's Postgres), refuses a non-loopback target
 *      without `--allow-remote-db`, and never writes anything without
 *      `--apply`. Dry run is the default for every mode.
 *   2. `app_users.subject` is written STRICTLY `WHERE id = <reviewed UUID>` —
 *      never `WHERE email = ...`. Email is used only to locate the Better Auth
 *      account, which Better Auth's own schema keys on email; every SIGAP-side
 *      row is addressed by the reviewed id, and a fixture whose row email
 *      differs from the login email (patient-a@sigap.local vs
 *      smoke@sigap.local) is reported, never "resolved".
 *   3. A Better Auth account that is already the subject of a DIFFERENT
 *      app_user is refused, and so is an app_user row that is missing or
 *      soft-deleted. No guessing, no adoption.
 *   4. The fixture reset refuses to rewrite any appointment that left the
 *      citizen-controlled states (`scheduled` / `cancelled` with no check-in
 *      and no completion). A checked-in, queued, completed or no-show
 *      appointment is a real visit record, not fixture state.
 *
 * Usage (dry run is the default; nothing is written without `--apply`):
 *
 *   node scripts/prepare-auth-e2e.mjs                     # plan only
 *   node scripts/prepare-auth-e2e.mjs --apply             # write
 *   node scripts/prepare-auth-e2e.mjs --apply --log <path>  # + audit block
 *   node scripts/prepare-auth-e2e.mjs --from-env-file     # ignore an ambient DATABASE_URL
 *
 * Flags:
 *   --apply              write (default is dry run)
 *   --from-env-file      resolve the DB target from the repo `.env` files only,
 *                        ignoring ambient SIGAP_DATABASE_URL / DATABASE_URL
 *   --allow-remote-db    allow a non-loopback DB target (not needed locally)
 *   --log <path>         append the run's audit block (markdown) to <path>
 *   --help               print usage
 *
 * Procedure of record: `design/selfhosted-auth-e2e.md`.
 *
 * Idempotent: re-running `--apply` on a prepared database prints the same plan
 * with every row `unchanged` and writes nothing (the password probe still runs
 * — it is a read of credential state, and it is what turns "the spec failed to
 * sign in" into "the password no longer matches the reviewed fixture").
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
// own `.env` is how a fixture run would point at the wrong database.
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
				"    * set SIGAP_DATABASE_URL to this repo's dev database " +
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
	const stripped = local
		? connectionString.replace(/([?&])sslmode=[^&]+(&?)/, (_m, lead, trail) => (trail ? lead : ''))
		: connectionString;
	return new Pool({ connectionString: stripped, ssl: local ? { rejectUnauthorized: false } : undefined });
}

// ---------------------------------------------------------------------------
// `auth` — mirrors src/lib/server/auth.better.ts (minus the token-minting jwt
// plugin and the SvelteKit cookie plumbing, neither of which affects user
// provisioning or credential verification). Same construction as
// provision-staff.mjs, so the two scripts cannot disagree about where the auth
// tables live.
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
// THE REVIEWED TABLE.
//
// Every entry is a reviewed `{ email, password, appUserId }` tuple:
//   - `email` is the Better Auth login the spec types into the form;
//   - `password` is the reviewed local fixture password (synthetic, local-only
//     — never printed, never used outside `*.sigap.local` accounts);
//   - `appUserId` is the STABLE SIGAP id the subject is written to.
//
// The app_users ROW EMAIL is expected to drift for the citizens
// (smoke@sigap.local signs in to the row whose own email is
// patient-a@sigap.local). That drift is printed and is NOT a linking rule —
// the link is the reviewed id, exactly like `backfill-auth-subjects.mjs`.
//
//   a62e05c1…   patient-a row — owns the two fixture appointments the spec cancels
//   949eae99…   smoke-b row   — patient B (0 appointments; the isolation side)
//   d990        super admin   (seed/dev.sql:40)
//   c301        org A admin   (seed/regional_tenancy.sql:147)
// ---------------------------------------------------------------------------
const IDENTITIES = [
	{
		email: 'smoke@sigap.local',
		password: 'Smoke12345!',
		appUserId: 'a62e05c1-43b4-4b58-a51d-6950e5ae742e',
		label: 'Patient A (synthetic)',
		role: 'citizen A — the two appointment cards'
	},
	{
		email: 'smoke-b@sigap.local',
		password: 'SmokeB12345!',
		appUserId: '949eae99-edfd-4c5b-ab13-2a9bc7b7a482',
		label: 'Smoke B (synthetic citizen)',
		role: 'citizen B — the empty list and the denied cancel'
	},
	{
		email: 'org-a-admin@sigap.local',
		password: 'Admin12345!',
		appUserId: '00000000-0000-0000-0000-00000000c301',
		label: 'Org A Admin (synthetic)',
		role: 'staff — one organization in the directory'
	},
	{
		email: 'global-admin@sigap.local',
		password: 'Admin12345!',
		appUserId: '00000000-0000-0000-0000-00000000d990',
		label: 'Local Global Super Admin (synthetic)',
		role: 'staff — both organizations in the directory'
	}
];

// ---------------------------------------------------------------------------
// THE CITIZEN FIXTURE PRE-STATE.
//
// The spec's cancellation is real and one-way: it drives the citizen's own
// `scheduled → cancelled` transition through the UI, so a second run would find
// nothing to cancel. These are the two hand-built appointments owned by the
// reviewed patient-A row, restored to `scheduled` before every run. The guard
// inside `planFixtureAppointments` refuses any row that left the
// citizen-controlled states.
//
// `facility` is the human label the spec asserts on the card; the script only
// prints it (and refuses when a row's facility does not match what the spec
// was written against — a seed/fixture edit should fail loudly here rather
// than as a mystery UI failure).
// ---------------------------------------------------------------------------
const CITIZEN_FIXTURE_APPOINTMENTS = [
	{
		id: '5d2a54cb-221b-45c6-9871-29e0ca7a3056',
		appUserId: 'a62e05c1-43b4-4b58-a51d-6950e5ae742e',
		facility: 'RS Sehat Nusantara Cimahi'
	},
	{
		id: '730ec946-65d4-4c5b-9346-18435de47f1e',
		appUserId: 'a62e05c1-43b4-4b58-a51d-6950e5ae742e',
		facility: 'Puskesmas Prima Tangerang'
	}
];

const normalizeEmail = (email) => String(email ?? '').trim().toLowerCase();
const shortId = (id) => (id ? `…${String(id).slice(-4)}` : '(none)');
const shortAuthId = (id) => (id ? `…${String(id).slice(-10)}` : '(absent)');

// ---------------------------------------------------------------------------
// Small DB helpers. The SIGAP-side reads are keyed on ids, never on email.
// ---------------------------------------------------------------------------
async function readAppUser(pool, appUserId) {
	const { rows } = await pool.query(
		'SELECT id, email, display_name, subject, status, deleted_at FROM app_users WHERE id = $1',
		[appUserId]
	);
	return rows[0] ?? null;
}

async function findBetterAuthUserByEmail(pool, email) {
	const { rows } = await pool.query(
		'SELECT id, email, "createdAt" FROM "auth"."user" WHERE lower(email) = lower($1) LIMIT 1',
		[normalizeEmail(email)]
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
 * Read one fixture appointment with the facility label the spec asserts on.
 * Every column the reset guard needs is read in the same query, so the guard
 * and the write cannot disagree about the row's state.
 */
async function readFixtureAppointment(pool, appointmentId) {
	const { rows } = await pool.query(
		`SELECT a.id, a.status, a.cancelled_at, a.checkin_at, a.completed_at,
		        a.patient_account_id, a.checkin_code, f.name AS facility_name
		 FROM appointments a
		 JOIN facilities f ON f.id = a.facility_id
		 WHERE a.id = $1`,
		[appointmentId]
	);
	return rows[0] ?? null;
}

// ---------------------------------------------------------------------------
// Audit surface. Same shape as provision-staff.mjs so the two logs read alike.
// ---------------------------------------------------------------------------
const changes = [];
const errors = [];

function recordChange(entity, key, oldValue, newValue) {
	changes.push({
		entity,
		key,
		old:
			oldValue === undefined || oldValue === null || oldValue === ''
				? '(none)'
				: String(oldValue),
		new:
			newValue === undefined || newValue === null || newValue === ''
				? '(none)'
				: String(newValue),
		changed: String(oldValue ?? '') !== String(newValue ?? '')
	});
}

function fail(message) {
	errors.push(message);
	console.error(`  !! ${message}`);
}

function renderTable(headers, rows) {
	const widths = headers.map((h, i) =>
		Math.min(72, Math.max(h.length, ...rows.map((r) => String(r[i] ?? '').length)))
	);
	const line = (cells) =>
		cells.map((c, i) => String(c ?? '').slice(0, widths[i]).padEnd(widths[i])).join('  ');
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
		`### ${stamp} — prepare-auth-e2e (${mode}) — \`${target}\``,
		'',
		`- operator: \`${operator}\``,
		`- reviewed identities: ${IDENTITIES.length}`,
		`- reviewed fixture appointments: ${CITIZEN_FIXTURE_APPOINTMENTS.length}`,
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
	const opts = { apply: false, fromEnvFile: false, allowRemoteDb: false, log: null, help: false };
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		const needsValue = (name) => {
			const value = argv[i + 1];
			if (value === undefined || value.startsWith('--')) {
				throw new Error(`${name} needs a value`);
			}
			i += 1;
			return value;
		};
		switch (arg) {
			case '--apply':
				opts.apply = true;
				break;
			case '--from-env-file':
				opts.fromEnvFile = true;
				break;
			case '--allow-remote-db':
				opts.allowRemoteDb = true;
				break;
			case '--log':
				opts.log = needsValue('--log');
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
	console.log(`SIGAP self-hosted-auth E2E preparation — see design/selfhosted-auth-e2e.md.

  node scripts/prepare-auth-e2e.mjs [--apply] [--log <path>] [--from-env-file] [--allow-remote-db]

  --apply              write. Without it the run is a dry run.
  --log <path>         append the audit block to <path>
  --from-env-file      resolve the DB target from the repo .env files only
  --allow-remote-db    allow a non-loopback DB target

Writes (id-keyed, never email-keyed):
  * auth.<user>          — created for an absent fixture login (sign-up)
  * app_users.subject    — set to that Better Auth id, WHERE id = <reviewed id>
  * appointments         — the two reviewed citizen fixture rows, restored to
                           'scheduled' (only from citizen-controlled states)

The reviewed identities and the reviewed fixture appointments are static data
in this file, reviewed like code. There is no flag to add an actor or a row.`);
}

// ---------------------------------------------------------------------------
// Identity preparation — one reviewed entry.
// ---------------------------------------------------------------------------
async function prepareIdentity(pool, auth, entry, opts) {
	const email = normalizeEmail(entry.email);
	console.log(`\n[${entry.email}]  ${entry.role}`);
	console.log(`  app_user     : ${shortId(entry.appUserId)}  ${entry.label}`);

	// (a) The app_users row must already exist. app_users.id is the durable id
	// every FK (appointments.patient_account_id, user_roles.user_id) hangs off;
	// this script never invents one.
	const appUser = await readAppUser(pool, entry.appUserId);
	if (!appUser) {
		fail(
			`missing app_user id=${entry.appUserId} (${entry.email}) — run the seeds (or the fixture insert) first; skipped`
		);
		return;
	}
	if (appUser.deleted_at !== null) {
		fail(`app_user ${shortId(appUser.id)} is soft-deleted — skipped`);
		return;
	}
	console.log(`  row email    : ${appUser.email}${normalizeEmail(appUser.email) !== email ? '  (drift — the subject write is by id, not by email)' : ''}`);
	console.log(`  status       : ${appUser.status}${appUser.display_name ? `, ${appUser.display_name}` : ''}`);
	if (appUser.status !== 'active') {
		// The Go resolver filters status='active', so a disabled row authorizes
		// nothing even with a correct subject. Reported as an error because the
		// spec cannot pass — this is the whole point of the pre-flight.
		fail(`app_user ${shortId(appUser.id)} is '${appUser.status}' — the resolver would refuse it; re-activate before running the spec`);
	}

	// (b) The Better Auth login. Email is the lookup key HERE because that is
	// what Better Auth's own schema keys accounts on; no SIGAP write below uses
	// this email.
	let authUser = await findBetterAuthUserByEmail(pool, email);
	const existedBefore = Boolean(authUser);
	if (!authUser && opts.apply) {
		try {
			const res = await auth.api.signUpEmail({
				body: { email: entry.email, password: entry.password, name: entry.label }
			});
			authUser = res?.user?.id ? res.user : await findBetterAuthUserByEmail(pool, email);
		} catch (err) {
			authUser = await findBetterAuthUserByEmail(pool, email);
			if (!authUser) {
				fail(`signUpEmail failed for ${entry.email} and no account exists: ${err?.message ?? err}`);
				return;
			}
		}
		if (!authUser) {
			fail(`signUpEmail returned no user id for ${entry.email}`);
			return;
		}
		// Sign-up auto-creates a session; the CLI never uses that token, so it is
		// discarded — preparation must not leave live sessions behind.
		const stray = await countSessions(pool, authUser.id);
		if (stray > 0) {
			await revokeAllSessions(auth, authUser.id);
			recordChange('auth.session', `${email} ${shortId(entry.appUserId)}`, `${stray} live`, '0 live (sign-up session discarded)');
		}
	}
	console.log(
		`  better-auth  : ${
			authUser
				? `${shortAuthId(authUser.id)} (${existedBefore ? 'pre-existing' : 'created by this run'}, created ${new Date(authUser.createdAt).toISOString()})`
				: '(absent — would be created on --apply)'
		}`
	);

	// (c) Claimant interlock: one Better Auth account owns one app_user, always.
	if (authUser) {
		const claimant = await findSubjectClaimant(pool, authUser.id);
		if (claimant && claimant.id !== entry.appUserId) {
			fail(
				`Better Auth user ${shortAuthId(authUser.id)} for ${entry.email} is already the subject of ` +
					`app_user ${shortId(claimant.id)} (${claimant.email ?? 'no email'}) — refusing (fail closed)`
			);
			return;
		}
	}

	const desiredSubject = authUser?.id ?? '(new better-auth id)';
	const subjectChanged = appUser.subject !== authUser?.id;
	console.log(`  subject      : ${appUser.subject ?? '(none)'}  →  ${desiredSubject}${subjectChanged ? '' : '  (unchanged)'}`);

	if (opts.apply && authUser && subjectChanged) {
		const res = await pool.query(
			'UPDATE app_users SET subject = $1, updated_at = now() WHERE id = $2',
			[authUser.id, entry.appUserId]
		);
		if (res.rowCount !== 1) {
			fail(`subject write affected ${res.rowCount} rows for app_user ${shortId(entry.appUserId)} — expected exactly 1`);
			return;
		}
	}

	// (d) Credential probe (apply only). A dry run promises no writes, and a
	// sign-in writes a session row, so the probe is skipped there and the table
	// says so. When the probe fails, the spec cannot pass either — report it now
	// rather than as a mystery 401.
	let probeLabel = opts.apply ? '(not probed)' : '(not probed in dry run)';
	if (opts.apply && authUser) {
		try {
			const res = await auth.api.signInEmail({ body: { email: entry.email, password: entry.password } });
			if (!res?.user?.id) {
				probeLabel = 'FAILED (no user returned)';
				fail(`password probe failed for ${entry.email}: sign-in returned no user`);
			} else {
				// The probe creates a real session. A CLI has no cookie jar, and
				// better-auth 1.7.20's `signOut` wants a request's `Headers`, so the
				// session is discarded through the library's own adapter call instead
				// (the same one provision-staff uses) — preparation must not leave
				// live sessions behind.
				const stray = await countSessions(pool, authUser.id);
				probeLabel = 'verified';
				if (stray > 0) {
					await revokeAllSessions(auth, authUser.id);
					probeLabel = `verified (probe session discarded; ${stray} session(s) revoked)`;
				}
				console.log(`  sessions     : ${stray} live${stray > 0 ? ' → 0 (revoked)' : ''}`);
			}
		} catch (err) {
			probeLabel = `FAILED: ${err?.message ?? err}`;
			fail(
				`password probe failed for ${entry.email} — the reviewed password does not sign this account in; ` +
					'the spec would fail at the login form. Reset the fixture password deliberately (delete the auth row and re-run --apply, or change it through the app) before running the spec.'
			);
		}
	}
	console.log(`  password     : ${probeLabel}`);

	recordChange(
		'auth.user',
		`${email} ${shortId(entry.appUserId)}`,
		authUser ? shortAuthId(authUser.id) : '(absent)',
		authUser
			? shortAuthId(authUser.id)
			: opts.apply
				? '(created by this run)'
				: '(would be created)'
	);
	recordChange(
		'app_users.subject',
		`${email} → ${shortId(entry.appUserId)} (row email ${appUser.email})`,
		appUser.subject ?? '(none)',
		authUser?.id ?? (opts.apply ? '(written by this run)' : '(would be written)')
	);
	// A credential probe is an OBSERVATION, not a change, so both columns carry
	// the observed state and a no-op re-run reports `old == new` like every
	// other unchanged row (the convention provision-staff.mjs uses).
	recordChange('credentials.probe', `${email} ${shortId(entry.appUserId)}`, probeLabel, probeLabel);
}

/**
 * Revoke every session of a Better Auth user.
 *
 * Same call `provision-staff.mjs` uses: the exact adapter method behind
 * `POST /api/auth/revoke-sessions` (better-auth 1.7.20
 * `internalAdapter.deleteUserSessions`). A CLI has no caller session, so the
 * endpoint itself is not reachable from here.
 */
async function revokeAllSessions(auth, betterAuthUserId) {
	const ctx = await auth.$context;
	if (typeof ctx?.internalAdapter?.deleteUserSessions !== 'function') {
		throw new Error(
			'internalAdapter.deleteUserSessions is unavailable in this better-auth build; refusing to guess a revocation path.'
		);
	}
	await ctx.internalAdapter.deleteUserSessions(betterAuthUserId);
}

// ---------------------------------------------------------------------------
// Citizen fixture pre-state — one reviewed appointment.
//
// The ONLY appointment columns ever written are `status` and `cancelled_at`,
// and only on the reviewed ids, only from a citizen-controlled state. The
// reset is one-way OUT of `cancelled` (and a no-op for an already-`scheduled`
// row), so a re-run after the spec cancelled the fixture restores exactly the
// state the spec asserts.
// ---------------------------------------------------------------------------
function planFixtureAppointment(row, entry) {
	if (!row) {
		fail(`fixture appointment ${entry.id} (${entry.facility}) does not exist — the fixture is incomplete`);
		return null;
	}
	if (row.patient_account_id !== entry.appUserId) {
		fail(
			`fixture appointment ${entry.id} is owned by app_user ${row.patient_account_id ?? '(none)'} — ` +
				`the reviewed owner is ${entry.appUserId}; refusing to touch it`
		);
		return null;
	}
	if (row.facility_name !== entry.facility) {
		fail(
			`fixture appointment ${entry.id} is at "${row.facility_name}" but the reviewed fixture says "${entry.facility}" — ` +
				'the spec asserts on the facility name; update the fixture table and the spec together'
		);
		return null;
	}
	if (row.status === 'scheduled') return { action: 'unchanged', row };
	if (row.status === 'cancelled') {
		if (row.checkin_at !== null || row.completed_at !== null) {
			fail(
				`fixture appointment ${entry.id} is cancelled but carries check-in/completion timestamps — ` +
					'that is a real visit record, not fixture state; refusing to rewrite it'
			);
			return null;
		}
		return { action: 'reset', row };
	}
	fail(
		`fixture appointment ${entry.id} is '${row.status}' — only the citizen-controlled states ` +
			"(scheduled / cancelled) are fixture state; refusing to rewrite a visit in progress"
	);
	return null;
}

async function prepareFixtureAppointments(pool, opts) {
	console.log('\n[citizen fixture — the two reviewed appointments]');
	const seenOwners = new Set();
	let plannedResets = 0;

	for (const entry of CITIZEN_FIXTURE_APPOINTMENTS) {
		const row = await readFixtureAppointment(pool, entry.id);
		const plan = planFixtureAppointment(row, entry);
		if (!plan) continue;
		seenOwners.add(entry.appUserId);

		const label = `${entry.facility} (${entry.id.slice(-4)})`;
		if (plan.action === 'unchanged') {
			console.log(`  ${label}: scheduled  (unchanged)`);
		} else {
			console.log(`  ${label}: ${plan.row.status} → scheduled  (cancelled_at ${plan.row.cancelled_at ? 'cleared' : '(none)'})`);
			plannedResets += 1;
			if (opts.apply) {
				const res = await pool.query(
					`UPDATE appointments
					 SET status = 'scheduled', cancelled_at = NULL, updated_at = now()
					 WHERE id = $1 AND status = 'cancelled'`,
					[entry.id]
				);
				if (res.rowCount !== 1) {
					fail(`fixture reset affected ${res.rowCount} rows for appointment ${entry.id} — expected exactly 1`);
					plannedResets -= 1;
					continue;
				}
			}
		}
		const stateLabel = `${plan.row.status}${plan.row.cancelled_at ? ' (cancelled_at set)' : ''}`;
		recordChange(
			'appointments',
			`${label} owner ${shortId(entry.appUserId)}`,
			stateLabel,
			plan.action === 'unchanged'
				? stateLabel
				: opts.apply
					? 'scheduled (cancelled_at NULL)'
					: 'scheduled (on --apply)'
		);
	}

	for (const owner of seenOwners) {
		const { rows } = await pool.query(
			`SELECT count(*)::int AS total,
			        count(*) FILTER (WHERE status = 'scheduled')::int AS scheduled
			 FROM appointments WHERE patient_account_id = $1`,
			[owner]
		);
		const total = rows[0].total;
		// In a dry run the write has not happened yet, so the projected count is
		// today's count plus the rows this run would restore.
		const scheduled = opts.apply ? rows[0].scheduled : rows[0].scheduled + plannedResets;
		console.log(
			`  owner ${shortId(owner)}: ${total} appointment(s), ${scheduled} scheduled  (the spec asserts ${total} cards, then 1 cancelled + ${total - 1} scheduled)`
		);
		if (total !== CITIZEN_FIXTURE_APPOINTMENTS.length) {
			fail(
				`owner ${shortId(owner)} holds ${total} appointments but the reviewed fixture lists ${CITIZEN_FIXTURE_APPOINTMENTS.length} — ` +
					'the spec asserts EXACTLY the reviewed cards; update the fixture table and the spec together'
			);
		}
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------
async function main() {
	let opts;
	try {
		opts = parseArgs(process.argv.slice(2));
	} catch (err) {
		console.error(`prepare-auth-e2e: ${err.message}`);
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
		console.error(`prepare-auth-e2e: ${err.message}`);
		process.exitCode = 1;
		return;
	}

	const mode = opts.apply ? 'APPLY' : 'DRY RUN';
	console.log('SIGAP self-hosted-auth E2E preparation');
	console.log(`  target   : ${targetInfo.target}${targetInfo.loopback ? '  (local)' : '  (REMOTE)'}`);
	console.log(`  mode     : ${mode}${opts.apply ? '' : '  — no writes; pass --apply to write'}`);
	console.log(`  reviewed : ${IDENTITIES.length} identities, ${CITIZEN_FIXTURE_APPOINTMENTS.length} citizen fixture appointments`);
	console.log('  spec     : apps/web/e2e/selfhosted-auth-journey.spec.ts (requires the -Jwt -NoAdminActor preview)');

	const pool = createPool(targetInfo);
	const auth = buildAuth(pool);

	try {
		for (const entry of IDENTITIES) await prepareIdentity(pool, auth, entry, opts);
		await prepareFixtureAppointments(pool, opts);
	} catch (err) {
		errors.push(err?.message ?? String(err));
		console.error(`\n!! ${err?.stack ?? err}`);
	} finally {
		console.log(`\n=== CHANGE LOG (old → new) — ${mode} ===`);
		for (const line of renderChangeLog()) console.log(line);
		console.log(
			`\ncounts: identities=${IDENTITIES.length}  appointments=${CITIZEN_FIXTURE_APPOINTMENTS.length}  changed=${changes.filter((c) => c.changed).length}  audit rows=${changes.length}  errors=${errors.length}`
		);
		if (!opts.apply) {
			console.log(
				'DRY RUN — no Better Auth users created, no subject changed, no appointment state restored. Re-run with --apply.'
			);
		} else if (errors.length === 0) {
			console.log(
				'Prepared. Next: SIGAP_E2E_BASE_URL=http://127.0.0.1:5173 npx playwright test e2e/selfhosted-auth-journey.spec.ts\n' +
					'  (PowerShell: $env:SIGAP_E2E_BASE_URL=\'http://127.0.0.1:5173\'; npx playwright test e2e/selfhosted-auth-journey.spec.ts)\n' +
					'  Re-run `--apply` before each spec run: the spec cancels a real appointment, and this is what restores it.'
			);
		}
		if (opts.log) {
			const operator = process.env.SIGAP_PREPARE_E2E_OPERATOR ?? userInfo().username;
			appendFileSync(opts.log, renderMarkdownLog({ target: targetInfo.target, mode, operator }), 'utf8');
			console.log(`audit block appended to ${opts.log} (operator: ${operator})`);
		}
		await pool.end();
	}

	if (errors.length > 0) process.exitCode = 1;
}

main().catch(async (err) => {
	console.error('prepare-auth-e2e failed:', err);
	process.exitCode = 1;
});
