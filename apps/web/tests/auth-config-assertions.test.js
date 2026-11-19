// Auth configuration assertions for `src/lib/server/auth.better.ts`:
//
//   FIX 1 — registration provisions a SIGAP identity. The registered
//           `databaseHooks.user.create.after` hook must INSERT a ROLELESS
//           `app_users` row keyed by `subject = auth.user.id`, never by email,
//           and must be idempotent on replay.
//   FIX 2 — the build placeholder secret is fail-CLOSED. In production, outside
//           the build, a missing `BETTER_AUTH_SECRET` must throw at module
//           construction instead of signing real sessions with a public
//           constant; `jwt.issuer`/`jwt.audience` must follow
//           SIGAP_AUTH_ISSUER / SIGAP_AUTH_AUDIENCE.
//
// Usage: node tests/auth-config-assertions.test.js
//
// WHY THE TEST CONSTRUCTS THE MODULE ITSELF
// The two behaviours live at module scope (a `throw` fires when the module is
// evaluated, and the hook is an object registered on the Better Auth instance),
// so nothing short of evaluating the module can observe them. `boot the built
// server` cannot: the placeholder gate is a construction-time decision, and the
// provisioning hook is only reachable through a live sign-up. So the test
// spawns child copies of itself and, in each child, imports the real module
// with the two SvelteKit virtual modules swapped for stubs ($app/environment so
// `building` the build flag can be set, $app/server so `getRequestEvent()` is a
// no-request event). Everything else — `better-auth`, the Kysely dialect, the
// `pg` pool, the config itself — is the real thing. `db.ts` is imported by the
// same URL the app resolves, so live assertions read through the app's own pool.
//
// Each scenario runs in its own process because the pool and the resolved
// options are module singletons bound to the env present at import time.
//
// WHAT IS ASSERTED AGAINST A LIVE DB vs STATICALLY
//   - Statically (always): the exact code shapes that carry FIX 1 / FIX 2 —
//     the INSERT statement (columns, `'active'`, `NULL`, `ON CONFLICT DO
//     NOTHING`), the absence of any email-based link, and the fail-closed gate.
//   - Behaviourally, without a DB (always): construction throws / does not
//     throw per env, the resolved jwt issuer/audience follow the env, and the
//     provisioning hook is registered and inert when no pool is configured.
//   - Against a live DB (only when SIGAP_DATABASE_URL is set AND the `auth`
//     schema + `public.app_users` exist): a real sign-up yields exactly one
//     roleless `app_users` row keyed by the auth user id, a replay of the hook
//     does not duplicate it, and a duplicate sign-up adds no second identity.
//     The rows this test creates are deleted before it exits.

import assert from 'node:assert';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');
const authConfigPath = path.join(root, 'src', 'lib', 'server', 'auth.better.ts');
const dbModuleUrl = pathToFileURL(path.join(root, 'src', 'lib', 'server', 'db.ts')).href;
const RESULT_MARKER = '__AUTH_CONFIG_RESULT__';

// ---------------------------------------------------------------------------
// Child mode — one scenario per process, result reported as one JSON line.
// ---------------------------------------------------------------------------

const scenarioArg = process.argv.slice(2).find((arg) => arg.startsWith('--scenario='));
if (scenarioArg) {
	await runChildScenario(scenarioArg.slice('--scenario='.length));
	process.exit(0);
}

/**
 * Replace the two SvelteKit virtual modules with stubs. `$app/environment` is
 * the one that matters: `building` is `true` only while SvelteKit is analysing
 * the app during `vite build`/prerender, which is exactly the window the
 * placeholder secret is allowed in.
 */
async function importAuthModule() {
	const { registerHooks } = await import('node:module');
	const building = process.env.SIGAP_TEST_BUILDING === '1';
	registerHooks({
		resolve(specifier, context, nextResolve) {
			if (specifier === '$app/environment') {
				return {
					url:
						`data:text/javascript,export const building=${building};export const dev=${!building};` +
						'export const browser=false;export const version="auth-config-assertions";',
					shortCircuit: true
				};
			}
			if (specifier === '$app/server') {
				// Outside a request there is no SvelteKit event; returning undefined
				// keeps the `sveltekit-cookies` plugin on its documented
				// `if (!event) return;` path, so cookie mirroring is skipped and the
				// response headers (where the session cookie is carried) stay intact.
				return {
					url: 'data:text/javascript,export function getRequestEvent(){return undefined;}',
					shortCircuit: true
				};
			}
			// The app's own source imports are extensionless TS (`./db`, `./mail`).
			if (specifier.startsWith('.') && !/\.[a-z]+$/i.test(specifier) && context.parentURL) {
				const candidate = new URL(`${specifier}.ts`, context.parentURL);
				if (fs.existsSync(fileURLToPath(candidate)))
					return { url: candidate.href, shortCircuit: true };
			}
			return nextResolve(specifier, context);
		}
	});
	return import(pathToFileURL(authConfigPath).href);
}

async function runChildScenario(name) {
	const result = { scenario: name };
	try {
		const { auth } = await importAuthModule();
		result.constructed = true;
		const jwtPlugin = (auth.options.plugins ?? []).find((plugin) => plugin?.id === 'jwt');
		result.jwt = jwtPlugin?.options?.jwt ?? null;
		result.hookType = typeof auth.options.databaseHooks?.user?.create?.after;
		if (name !== 'live-db' && result.hookType === 'function') {
			// Without a configured pool there is no `auth.user` either, so the hook
			// must be a no-op — not a crash, and not a silent write somewhere.
			try {
				await auth.options.databaseHooks.user.create.after(
					{ id: 'assertion-probe', email: 'assertion-probe@example.test', name: 'Assertion Probe' },
					null
				);
				result.hookWithoutDatabase = 'no-op';
			} catch (error) {
				result.hookWithoutDatabase = `threw: ${error.message}`;
			}
		}
		if (name === 'live-db') await liveDatabaseAssertions(auth, result);
	} catch (error) {
		result.thrown = error?.message ?? String(error);
	}
	// Synchronous write: a piped stdout is not guaranteed to flush on exit().
	fs.writeSync(1, `${RESULT_MARKER} ${JSON.stringify(result)}\n`);
}

/**
 * The live-DB half. Creates one real account through the same call the register
 * action makes (`auth.api.signUpEmail`), asserts the identity it must have
 * produced, and deletes everything it created. Reads go through the app's own
 * pool (`getPool()`), so nothing here can disagree with the runtime about which
 * database or which connection options are in play.
 */
async function liveDatabaseAssertions(auth, result) {
	const { getPool } = await import(dbModuleUrl);
	const pool = getPool();
	// Reachability and migration are environment facts, not config bugs: an
	// environment that merely EXPORTS SIGAP_DATABASE_URL but has no database
	// running (or has not applied the auth migration) skips with the reason.
	// Everything after this point is a genuine assertion — a failure there is
	// the code's, and is reported as one.
	try {
		await pool.query('SELECT 1');
	} catch (error) {
		result.db = {
			skipped: `SIGAP_DATABASE_URL is set but the database is not reachable: ${error.code ?? error.message}`
		};
		return;
	}
	const schema = await pool.query(
		`SELECT to_regclass('public.app_users') AS app_users,
		        to_regclass('public.user_roles') AS user_roles,
		        to_regclass('auth."user"') AS auth_user`
	);
	const present = schema.rows[0];
	if (!present.app_users || !present.user_roles || !present.auth_user) {
		result.db = { skipped: 'the database is reachable but the SIGAP/auth schema is not migrated' };
		return;
	}

	const suffix = crypto.randomBytes(4).toString('hex');
	const email = `auth-config-assertion-${suffix}@example.test`;
	const name = `assertion-${suffix}`;
	const password = `Assertion-${suffix}-Password1`;
	const db = (result.db = { expectedEmail: email, expectedName: name });
	let authUserId = null;
	try {
		const signUp = await auth.api.signUpEmail({ body: { email, password, name } });
		authUserId = signUp?.user?.id ?? null;
		db.authUserId = authUserId;

		const rows = await pool.query(
			'SELECT id, subject, email, display_name, status, deleted_at FROM app_users WHERE subject = $1',
			[authUserId]
		);
		db.appUserRowCount = rows.rowCount;
		db.appUser = rows.rows[0] ?? null;
		db.roleGrants = rows.rows[0]
			? (await pool.query('SELECT count(*)::int AS n FROM user_roles WHERE user_id = $1', [rows.rows[0].id]))
					.rows[0].n
			: null;

		// Replay the REGISTERED hook for the same auth user: idempotency.
		await auth.options.databaseHooks.user.create.after({ id: authUserId, email, name }, null);
		db.appUserRowCountAfterHookReplay = (
			await pool.query('SELECT count(*)::int AS n FROM app_users WHERE subject = $1', [authUserId])
		).rows[0].n;

		// A second sign-up with the same address must not mint a second identity.
		let duplicateThrew = null;
		try {
			await auth.api.signUpEmail({ body: { email, password, name } });
		} catch (error) {
			duplicateThrew = error?.message ?? String(error);
		}
		db.duplicateSignUpThrew = duplicateThrew;
		db.appUserRowsForEmail = (
			await pool.query('SELECT count(*)::int AS n FROM app_users WHERE email = $1', [email])
		).rows[0].n;
		db.authUserRowsForEmail = (
			await pool.query('SELECT count(*)::int AS n FROM auth."user" WHERE email = $1', [email])
		).rows[0].n;

		// Bonus: mint the access token the Go API verifies and read its claims.
		// The JWKS private key is stored in the DB encrypted with THIS deployment's
		// secret, so a DB initialised under a different secret cannot mint a token
		// here; that is an environment limitation, reported as unavailable, not a
		// wiring failure — any other error propagates.
		try {
			const signIn = await auth.api.signInEmail({ body: { email, password }, returnHeaders: true });
			const setCookie = signIn?.headers?.get?.('set-cookie') ?? null;
			const cookie = setCookie ? setCookie.split(';')[0] : '';
			const token = await auth.api.getToken({ headers: new Headers({ cookie }) });
			const claims = JSON.parse(Buffer.from(token.token.split('.')[1], 'base64url').toString('utf8'));
			db.token = {
				iss: claims.iss,
				aud: claims.aud,
				subMatchesAuthUser: claims.sub === authUserId,
				subMatchesAppUserSubject: claims.sub === db.appUser?.subject
			};
		} catch (error) {
			if (/decrypt private key/i.test(error?.message ?? '')) {
				db.token = {
					unavailable: 'the auth.jwks row belongs to a different BETTER_AUTH_SECRET than this run'
				};
			} else {
				throw error;
			}
		}
	} finally {
		if (authUserId) {
			await pool.query('DELETE FROM app_users WHERE subject = $1', [authUserId]);
			await pool.query('DELETE FROM auth."user" WHERE id = $1', [authUserId]);
			db.cleanup = {
				appUserRowsLeft: (
					await pool.query('SELECT count(*)::int AS n FROM app_users WHERE subject = $1', [authUserId])
				).rows[0].n,
				authUserRowsLeft: (
					await pool.query('SELECT count(*)::int AS n FROM auth."user" WHERE id = $1', [authUserId])
				).rows[0].n
			};
		}
	}
}

// ---------------------------------------------------------------------------
// Parent mode — run every scenario, then check the source shapes.
// ---------------------------------------------------------------------------

let failed = 0;
let skipped = 0;
const check = (name, ok, actual) => {
	if (ok) {
		console.log(`  PASS  ${name}`);
	} else {
		failed++;
		console.log(`  FAIL  ${name}  (actual: ${JSON.stringify(actual)})`);
	}
};
const note = (message) => {
	skipped++;
	console.log(`  SKIP  ${message}`);
};

/** Child env with every auth-affecting variable removed, then overrides. */
const childEnv = (overrides) => {
	const env = { ...process.env };
	for (const key of [
		'BETTER_AUTH_SECRET',
		'SIGAP_DATABASE_URL',
		'DATABASE_URL',
		'SIGAP_AUTH_ISSUER',
		'SIGAP_AUTH_AUDIENCE',
		'SIGAP_TEST_BUILDING',
		'NODE_ENV'
	]) {
		delete env[key];
	}
	return { ...env, ...overrides };
};

const runScenario = (name, overrides) => {
	const child = spawnSync(process.execPath, [fileURLToPath(import.meta.url), `--scenario=${name}`], {
		cwd: root,
		env: childEnv(overrides),
		encoding: 'utf8',
		timeout: 180_000
	});
	const line = `${child.stdout ?? ''}`
		.split('\n')
		.find((entry) => entry.startsWith(RESULT_MARKER));
	assert(line, `scenario ${name} produced no result line (stderr: ${(child.stderr ?? '').trim()})`);
	return JSON.parse(line.slice(RESULT_MARKER.length));
};

const SECRET = crypto.randomBytes(32).toString('base64');
const FOREIGN_ISSUER = 'https://issuer.auth-config-assertions.test:8443/sigap';
const FOREIGN_AUDIENCE = 'sigap-api-auth-config-assertions';

console.log('auth-config-assertions: FIX 1 (citizen provisioning) + FIX 2 (fail-closed secret)');

// --- FIX 2: the secret gate and the jwt claims it protects -----------------

console.log('\n[FIX 2] secret resolution');
{
	const prod = runScenario('prod-no-secret', { NODE_ENV: 'production' });
	check(
		'production with no BETTER_AUTH_SECRET throws at module construction',
		prod.constructed !== true && /BETTER_AUTH_SECRET is not set/.test(prod.thrown ?? ''),
		prod
	);
	check(
		'the throw names where the secret must come from, not just that it is missing',
		/production/.test(prod.thrown ?? '') && /BETTER_AUTH_SECRET/.test(prod.thrown ?? ''),
		prod.thrown
	);

	const secret = runScenario('prod-with-secret', { NODE_ENV: 'production', BETTER_AUTH_SECRET: SECRET });
	check('production WITH a secret constructs the real instance', secret.constructed === true, secret);
	check(
		'jwt defaults stay the documented local values when the env is silent',
		secret.jwt?.issuer === 'http://127.0.0.1:5173' && secret.jwt?.audience === 'authenticated',
		secret.jwt
	);

	const fromEnv = runScenario('prod-issuer-from-env', {
		NODE_ENV: 'production',
		BETTER_AUTH_SECRET: SECRET,
		SIGAP_AUTH_ISSUER: FOREIGN_ISSUER,
		SIGAP_AUTH_AUDIENCE: FOREIGN_AUDIENCE
	});
	check(
		'jwt.issuer follows SIGAP_AUTH_ISSUER (the Go API reads the same variable)',
		fromEnv.jwt?.issuer === FOREIGN_ISSUER,
		fromEnv.jwt
	);
	check('jwt.audience follows SIGAP_AUTH_AUDIENCE', fromEnv.jwt?.audience === FOREIGN_AUDIENCE, fromEnv.jwt);
	check(
		'the env override replaces the default rather than being appended to it',
		!/127\.0\.0\.1/.test(fromEnv.jwt?.issuer ?? '') && fromEnv.jwt?.audience !== 'authenticated',
		fromEnv.jwt
	);

	const build = runScenario('build-pass-no-secret', {
		NODE_ENV: 'production',
		SIGAP_TEST_BUILDING: '1'
	});
	check(
		'the SvelteKit build pass (building=true) may still use the placeholder',
		build.constructed === true && build.thrown === undefined,
		build
	);

	const dev = runScenario('dev-no-secret', { NODE_ENV: 'development' });
	check(
		'a non-production launch may still use the placeholder',
		dev.constructed === true && dev.thrown === undefined,
		dev
	);
}

// --- FIX 1: the provisioning hook -----------------------------------------

console.log('\n[FIX 1] registration hook');
{
	const hook = runScenario('prod-with-secret', { NODE_ENV: 'production', BETTER_AUTH_SECRET: SECRET });
	check(
		'databaseHooks.user.create.after is registered on the instance',
		hook.hookType === 'function',
		hook.hookType
	);
	check(
		'with no database configured the hook is an inert no-op, not a crash',
		hook.hookWithoutDatabase === 'no-op',
		hook.hookWithoutDatabase
	);
}

// Static assertions pin the exact code shapes the fix is made of, so a
// behavioural coincidence elsewhere cannot satisfy them.
const source = fs.readFileSync(authConfigPath, 'utf-8');

check(
	'the fail-open form (`BETTER_AUTH_SECRET ?? placeholder`) is gone from the source',
	!/BETTER_AUTH_SECRET\s*\?\?/.test(source) && /throw new Error\(/.test(source),
	source.match(/BETTER_AUTH_SECRET[^\n]*/)?.[0] ?? 'pattern absent'
);
check(
	'the placeholder is gated on `building` OR a non-production launch',
	/if \(building \|\| process\.env\.NODE_ENV !== "production"\) return BUILD_ONLY_SECRET;/.test(source),
	source.match(/if \(building[^\n]*/)?.[0] ?? 'gate not found'
);
check(
	'jwt.issuer/audience read the Go API variable names with local defaults',
	/issuer: process\.env\.SIGAP_AUTH_ISSUER \?\? "http:\/\/127\.0\.0\.1:5173"/.test(source) &&
		/audience: process\.env\.SIGAP_AUTH_AUDIENCE \?\? "authenticated"/.test(source),
	null
);
check(
	'the hook inserts into app_users keyed by subject, active, undeleted',
	/INSERT INTO app_users \(subject, email, display_name, status, deleted_at\)\s+VALUES \(\$1, \$2, \$3, 'active', NULL\)\s+ON CONFLICT DO NOTHING/.test(
		source
	),
	source.match(/INSERT INTO app_users[^\n]*/)?.[0] ?? 'INSERT not found'
);
check(
	'the hook is registered on databaseHooks.user.create.after',
	/databaseHooks:\s*\{[\s\S]{0,2000}?user:\s*\{[\s\S]{0,400}?create:\s*\{[\s\S]{0,1200}?after:\s*async\s*\(user\)/.test(source),
	null
);
check(
	'the hook never links to an existing row by email (no conflict target, no UPDATE)',
	!/ON CONFLICT\s*\(\s*email/i.test(source) &&
		!/WHERE\s+email\s*=/i.test(source) &&
		!/UPDATE\s+app_users/i.test(source),
	null
);
check(
	'no role or grant is written for a citizen (a roleless app_user is the patient account)',
	!/INSERT INTO user_roles/i.test(source) && !/role_id/.test(source),
	null
);

// --- live database, when one is reachable ---------------------------------

console.log('\n[FIX 1] live database');
if (!process.env.SIGAP_DATABASE_URL) {
	note('SIGAP_DATABASE_URL is not set — the live sign-up assertions cannot run');
} else {
	const live = runScenario('live-db', {
		NODE_ENV: 'production',
		BETTER_AUTH_SECRET: process.env.BETTER_AUTH_SECRET ?? SECRET,
		SIGAP_AUTH_ISSUER: FOREIGN_ISSUER,
		SIGAP_AUTH_AUDIENCE: FOREIGN_AUDIENCE,
		SIGAP_DATABASE_URL: process.env.SIGAP_DATABASE_URL
	});
	check('the module constructs against the live database', live.constructed === true, live.thrown ?? live);
	check(
		'the live-database scenario completed without a database error',
		live.thrown === undefined,
		live.thrown ?? 'no error'
	);
	if (live.db?.skipped) {
		note(live.db.skipped);
	} else {
		const db = live.db ?? {};
		check('a real sign-up produced exactly one app_users row', db.appUserRowCount === 1, db.appUserRowCount);
		check(
			'that row is keyed by subject = auth.user.id',
			Boolean(db.authUserId) && db.appUser?.subject === db.authUserId,
			{ authUserId: db.authUserId, subject: db.appUser?.subject }
		);
		check(
			'email and display_name are copied from the auth user, status active, not soft-deleted',
			db.appUser?.email === db.expectedEmail &&
				db.appUser?.display_name === db.expectedName &&
				db.appUser?.status === 'active' &&
				db.appUser?.deleted_at === null,
			db.appUser
		);
		check('the citizen is roleless (no user_roles grants)', db.roleGrants === 0, db.roleGrants);
		check(
			'replaying the registered hook does not duplicate the identity',
			db.appUserRowCountAfterHookReplay === 1,
			db.appUserRowCountAfterHookReplay
		);
		check(
			'a duplicate sign-up adds no second identity and no second auth user',
			db.appUserRowsForEmail === 1 && db.authUserRowsForEmail === 1,
			{ appUsers: db.appUserRowsForEmail, authUsers: db.authUserRowsForEmail, threw: db.duplicateSignUpThrew }
		);
		if (db.token?.unavailable) {
			note(`JWT claims not minted: ${db.token.unavailable}`);
		} else {
			check('the minted token carries the env issuer and audience', db.token?.iss === FOREIGN_ISSUER && db.token?.aud === FOREIGN_AUDIENCE, db.token);
			check(
				'the token subject is the auth user id AND the app_users.subject it resolves to',
				db.token?.subMatchesAuthUser === true && db.token?.subMatchesAppUserSubject === true,
				db.token
			);
		}
		check(
			'the rows this test created were deleted again',
			db.cleanup?.appUserRowsLeft === 0 && db.cleanup?.authUserRowsLeft === 0,
			db.cleanup
		);
	}
}

console.log(
	`\nauth-config-assertions: ${failed === 0 ? 'OK' : 'FAILED'} (${failed} failed, ${skipped} skipped)`
);
process.exit(failed === 0 ? 0 : 1);
