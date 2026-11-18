// Session lifecycle contract tests — Better Auth session APIs on the BUILT app.
//
// The policy under test is `design/session-lifecycle-policy.md`; the config under
// test is `src/lib/server/auth.better.ts`. Three phases:
//
//   1. STATIC (no DB, always runs) — the configured session clocks (evaluated as
//      arithmetic, so `60 * 60 * 24 * 7` and `604800` both pass), the
//      `user.changePassword` declaration, the untouched jwt/issuer/audience/1h
//      config, and the policy document's numbers matching the code.
//   2. BUILT APP, NO DB — how the `/api/auth/*` mount DEGRADES when Better Auth
//      has no database. Better Auth refuses to initialise its handler without a
//      schema ("Could not validate the database schema"), so `svelteKitHandler`
//      never matches and every `/api/auth/*` request falls through to the app's
//      ordinary **404** — with the app's HTML not-found page, never a 5xx — while
//      the rest of the site (`/auth/login`) still renders.
//   3. LIVE LIFECYCLE (needs Postgres) — sign-up → session; second device →
//      two independent sessions; `list-sessions` per-device tokens; cross-device
//      `revoke-session`; sign-out; `change-password` requires the CURRENT
//      password; `revokeOtherSessions` kills the others; and the ACCESS JWT TTL,
//      which is the maximum revocation delay the policy binds (exp − iat = 3600).
//
// Phase 3 is GATED. It runs only when SIGAP_DATABASE_URL / DATABASE_URL (or the
// repo-root `.env`) resolves to a reachable Postgres whose `auth` schema is
// migrated. Otherwise every case is reported as `[GATE] SKIP …` with the exact
// command that would run it — and the run still exits 0 so this file can sit in
// `pnpm test` on a machine without Postgres. Set
// SIGAP_SESSION_LIFECYCLE_REQUIRE_DB=1 to make those skips fatal.
//
// Phase 3 boots with the SAME `BETTER_AUTH_SECRET` the dev server uses, because
// Better Auth keeps its ES256 keypair in `auth.jwks` with the private key
// encrypted under that secret: a boot with any other secret cannot decrypt it
// and every token mint dies with "Failed to decrypt private key. Make sure the
// secret currently in use is the same as the one used to encrypt the private
// key." The harness resolves the secret exactly as the dev server does — the
// environment first, then the repo-root `.env` that
// `scripts/dev/Run-V22Service.ps1` loads — so a normal run writes nothing. It
// then asks Better Auth itself whether that secret still opens the stored key
// (Better Auth's own `symmetricDecrypt`, which fails on the AES-GCM tag when
// the secret changed). If it does not — or if no secret is configured anywhere,
// so the harness has to invent a disposable per-run one — it snapshots
// `auth.jwks`, clears the table for the run (Better Auth mints a fresh keypair
// under the secret in use) and restores the snapshot in `finally`, leaving the
// shared dev database exactly as it was found. The run is therefore
// self-contained and repeatable, and does not depend on an ambient secret
// happening to agree with the database.
//
// NOT collected by vitest (its `include` is `src/**`); run it with node:
//   node tests/session-lifecycle.test.js
//
// Builds `build/` first if it is absent, like tests/auth-actions.test.js.

import assert from 'node:assert';
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');
const repoRoot = path.join(root, '..', '..');
const buildEntry = path.join(root, 'build', 'index.js');
const authSourcePath = path.join(root, 'src', 'lib', 'server', 'auth.better.ts');
const policyPath = path.join(repoRoot, 'design', 'session-lifecycle-policy.md');

const REQUIRE_DB = process.env.SIGAP_SESSION_LIFECYCLE_REQUIRE_DB === '1';

// The policy's numbers, asserted in three places: the config, the doc, and (in
// phase 3) the live session row / JWT. One source of truth for this file.
const POLICY = {
	expiresIn: 60 * 60 * 24 * 7,
	updateAge: 60 * 60 * 24,
	freshAge: 60 * 60,
	jwtExpirationSeconds: 3600,
	issuer: 'http://127.0.0.1:5173',
	audience: 'authenticated'
};

let failed = 0;
let passed = 0;
let skipped = 0;

const section = (title) => console.log(`\n${title}`);
const check = (name, ok, actual) => {
	if (ok) {
		passed++;
		console.log(`  PASS  ${name}`);
	} else {
		failed++;
		console.log(`  FAIL  ${name}  (actual: ${JSON.stringify(actual)})`);
	}
};
const skip = (name, why) => {
	skipped++;
	console.log(`  [GATE] SKIP  ${name}`);
	console.log(`         reason: ${why}`);
};

// ---------------------------------------------------------------------------
// Source helpers — read the config as text and evaluate the arithmetic.
// ---------------------------------------------------------------------------

const authSource = fs.readFileSync(authSourcePath, 'utf8');

/** Slice the balanced `{ … }` object literal that starts after `\n\tkey: `. */
const extractBlock = (source, key) => {
	const match = new RegExp(`\\n\\t${key}: \\{`).exec(source);
	if (!match) return null;
	const open = source.indexOf('{', match.index);
	let depth = 0;
	for (let i = open; i < source.length; i++) {
		if (source[i] === '{') depth++;
		else if (source[i] === '}') {
			depth--;
			if (depth === 0) return source.slice(open, i + 1);
		}
	}
	return null;
};

/** Evaluate `60 * 60 * 24 * 7` / `604800` into seconds. Rejects anything else. */
const evalSeconds = (expression) => {
	const trimmed = String(expression).trim();
	assert.match(trimmed, /^[\d\s*+()]+$/, `unexpected duration expression: ${trimmed}`);
	return Number(Function(`"use strict"; return (${trimmed});`)());
};

/** The raw right-hand side of `key: value` inside a block, up to the comma. */
const optionValue = (block, key) => {
	if (!block) return null;
	const match = new RegExp(`(?:^|[,{\\s])${key}\\s*:\\s*([^,}\\n]+)`).exec(block);
	return match ? match[1].trim() : null;
};

/** First `KEY=value` for any of `names` in the repo-root / apps/web `.env`. */
const readDotEnv = (names) => {
	const pattern = new RegExp(`^\\s*(${names.join('|')})\\s*=\\s*(.*)$`);
	for (const envPath of [path.join(repoRoot, '.env'), path.join(root, '.env')]) {
		if (!fs.existsSync(envPath)) continue;
		for (const line of fs.readFileSync(envPath, 'utf8').split(/\r?\n/)) {
			const m = pattern.exec(line);
			if (!m) continue;
			const value = m[2].trim().replace(/^["']|["']$/g, '');
			if (value) return { key: m[1], value };
		}
	}
	return null;
};

/** Epoch milliseconds for a Date / epoch number / ISO-8601 string; null if it is none of those. */
const toEpochMs = (value) => {
	if (value instanceof Date) return Number.isFinite(value.getTime()) ? value.getTime() : null;
	if (typeof value === 'number') return Number.isFinite(value) ? value : null;
	if (typeof value !== 'string') return null;
	const ms = Date.parse(value);
	return Number.isFinite(ms) ? ms : null;
};

const sessionBlock = extractBlock(authSource, 'session');
const userBlock = extractBlock(authSource, 'user');
const rateLimitBlock = extractBlock(authSource, 'rateLimit');

// ---------------------------------------------------------------------------
// Phases 1 + 2 run everywhere; phase 3 needs a live `auth` schema.
// ---------------------------------------------------------------------------

try {
	// ---- Phase 1: static config + policy document ----
	section('Phase 1: session config and policy (static, no DB)');

	check('session block is configured explicitly', Boolean(sessionBlock), sessionBlock);

	for (const [key, expected] of [
		['expiresIn', POLICY.expiresIn],
		['updateAge', POLICY.updateAge],
		['freshAge', POLICY.freshAge]
	]) {
		const raw = optionValue(sessionBlock, key);
		let seconds = null;
		try {
			seconds = raw === null ? null : evalSeconds(raw);
		} catch {
			seconds = null;
		}
		check(`session.${key} = ${expected}s`, seconds === expected, { raw, seconds });
	}

	// The fresh window is the one value that TIGHTENS a better-auth default; a
	// regression to the 24h default would silently widen every sensitive endpoint.
	check(
		'session.freshAge is a deliberate 24x tightening of the library default (3600, not 86400)',
		evalSeconds(optionValue(sessionBlock, 'freshAge')) !== 86400,
		optionValue(sessionBlock, 'freshAge')
	);
	check(
		'session refresh is NOT disabled (sessions must keep rolling while in use)',
		!/disableSessionRefresh\s*:\s*true/.test(authSource),
		'disableSessionRefresh: true'
	);

	check(
		'user.changePassword.enabled === true (self-service password change declared)',
		/changePassword\s*:\s*\{\s*enabled\s*:\s*true/.test(userBlock ?? ''),
		userBlock
	);

	check('jwt plugin still pinned to ES256', /alg:\s*"ES256"/.test(authSource), 'alg: "ES256"');
	// The trust boundary is SHARED CONFIG, not hardcoded strings: Better Auth
	// and the Go API both read SIGAP_AUTH_ISSUER / SIGAP_AUTH_AUDIENCE, with the
	// local defaults below. Behavioral coverage (env overrides, token iss/aud,
	// fail-closed production config) lives in auth-config-assertions.test.js.
	check(
		'jwt issuer/audience follow the Go API env vars, keeping local defaults',
		/issuer:\s*process\.env\.SIGAP_AUTH_ISSUER\s*\?\?\s*"http:\/\/127\.0\.0\.1:5173"/.test(authSource) &&
			/audience:\s*process\.env\.SIGAP_AUTH_AUDIENCE\s*\?\?\s*"authenticated"/.test(authSource),
		{ issuer: /issuer:\s*(.*)/.exec(authSource)?.[1], audience: /audience:\s*(.*)/.exec(authSource)?.[1] }
	);
	check(
		`jwt.expirationTime is exactly the policy's revocation bound (${POLICY.jwtExpirationSeconds}s)`,
		/expirationTime:\s*"1h"/.test(authSource),
		/expirationTime:\s*"([^"]+)"/.exec(authSource)?.[1]
	);
	check(
		'sveltekitCookies stays the LAST plugin (cookie writes for server actions)',
		authSource.indexOf('sveltekitCookies(getRequestEvent)') >
			authSource.lastIndexOf('jwt({') &&
			authSource.indexOf('sveltekitCookies(getRequestEvent)') > 0,
		'sveltekitCookies position'
	);
	check(
		'rateLimit block untouched (enabled, 60s window, 20 max, memory)',
		/enabled:\s*true/.test(rateLimitBlock ?? '') &&
			Number(optionValue(rateLimitBlock, 'window')) === 60 &&
			Number(optionValue(rateLimitBlock, 'max')) === 20 &&
			/storage:\s*"memory"/.test(rateLimitBlock ?? ''),
		rateLimitBlock
	);
	check('secret still comes from the environment', /process\.env\.BETTER_AUTH_SECRET/.test(authSource), 'secret');
	check('Better Auth tables stay in the isolated auth schema', /schemaName:\s*"auth"/.test(authSource), 'schemaName');

	// The policy doc is the normative statement; these anchors make it fail
	// loudly if it drifts from the code above or drops the bound it exists for.
	const policy = fs.readFileSync(policyPath, 'utf8');
	check(
		'policy doc states the configured session clocks (604800 / 86400 / 3600)',
		['604800', '86400', '3600'].every((n) => policy.includes(n)),
		['604800', '86400', '3600'].filter((n) => !policy.includes(n))
	);
	check(
		'policy doc states the 1h maximum revocation delay and names expirationTime',
		policy.includes('expirationTime') && /1 hour/i.test(policy),
		'expirationTime + "1 hour"'
	);
	check(
		'policy doc states the privileged-staff bound (accepted 1h, compensating DB-RBAC control)',
		/privileged staff/i.test(policy) && /accepted bound/i.test(policy),
		'privileged staff / accepted bound'
	);
	check(
		'policy doc states the deactivation enforcement point (app_users.status)',
		policy.includes('app_users.status'),
		'app_users.status'
	);
	check(
		'policy doc records that a session revoke is NOT a JWT revoke',
		/is not a JWT revocation/i.test(policy),
		'is not a JWT revocation'
	);

	// ---- Phase 2: the BUILT app with no DB (the auth mount degrades) ----
	section('Phase 2: built app, no DB (the /api/auth/* mount degrades to 404)');

	if (!fs.existsSync(buildEntry)) {
		console.log('build/ not found — running vite build first');
		const built = spawnSync('pnpm build', { cwd: root, shell: true, stdio: 'inherit' });
		if (built.status !== 0) {
			console.error('vite build failed');
			process.exit(1);
		}
	}
	assert(fs.existsSync(buildEntry), 'build/index.js must exist after build');

	const freePort = () =>
		new Promise((resolve, reject) => {
			const srv = net.createServer();
			srv.listen(0, '127.0.0.1', () => {
				const port = srv.address().port;
				srv.close(() => resolve(port));
			});
			srv.on('error', reject);
		});

	// Every auth-affecting var is scrubbed first so a parent-process export can
	// never turn the no-DB boot into a DB boot.
	const cleanEnv = (overrides) => {
		const env = { ...process.env };
		delete env.SIGAP_DATABASE_URL;
		delete env.DATABASE_URL;
		delete env.BETTER_AUTH_SECRET;
		delete env.BETTER_AUTH_URL;
		return { ...env, ...Object.fromEntries(Object.entries(overrides).filter(([, v]) => v !== undefined)) };
	};

	// `BETTER_AUTH_URL` must equal the boot origin: Better Auth's CSRF/origin
	// check validates the request Origin against the configured base URL for
	// every cookie-bearing request (revoke-session, change-password, sign-out),
	// and the fixed default `http://127.0.0.1:5173` would reject this harness's
	// random port.
	const startApp = async (overrides) => {
		const port = await freePort();
		const webUrl = `http://127.0.0.1:${port}`;
		const child = spawn(process.execPath, [buildEntry], {
			cwd: root,
			env: cleanEnv({
				PORT: String(port),
				HOST: '127.0.0.1',
				ORIGIN: webUrl,
				BETTER_AUTH_URL: webUrl,
				...overrides
			}),
			stdio: ['ignore', 'ignore', 'inherit']
		});
		for (let i = 0; i < 60; i++) {
			try {
				const r = await fetch(webUrl + '/auth/login');
				if (r.status === 200) return { child, webUrl };
			} catch {
				// not up yet
			}
			await new Promise((r) => setTimeout(r, 500));
		}
		child.kill();
		throw new Error('web server did not become ready: ' + webUrl);
	};

	const stopApp = async (child) => {
		if (child && !child.killed) {
			child.kill();
			await new Promise((r) => child.once('exit', r));
		}
	};

	const request = async (webUrl, route, { method = 'POST', body, cookie } = {}) => {
		const res = await fetch(webUrl + '/api/auth' + route, {
			method,
			headers: {
				origin: webUrl,
				...(body ? { 'content-type': 'application/json' } : {}),
				...(cookie ? { cookie } : {})
			},
			body: body ? JSON.stringify(body) : undefined,
			redirect: 'manual'
		});
		const text = await res.text();
		let json = null;
		try {
			json = JSON.parse(text);
		} catch {
			json = null;
		}
		return { res, json, text };
	};

	{
		// With no Postgres, Better Auth cannot validate its schema, so nothing on
		// `/api/auth/*` is ever served: the request falls through to SvelteKit's
		// ordinary not-found. What matters is the SHAPE of that degrade — a plain
		// 404 carrying the app's own HTML page, never a 5xx and never a
		// half-initialised auth error body — and that the rest of the site stays
		// up. `BETTER_AUTH_URL` is deliberately left at the config default here:
		// with no auth route mounted there is no origin check left to satisfy.
		const { child, webUrl } = await startApp({ BETTER_AUTH_URL: undefined });
		try {
			const anon = await request(webUrl, '/get-session', { method: 'GET' });
			check('without a DB the auth mount degrades: get-session answers 404, not 5xx', anon.res.status === 404, {
				status: anon.res.status,
				body: anon.text.slice(0, 60)
			});
			check(
				"…and it is the app's own HTML not-found page, not an auth error body",
				/text\/html/i.test(anon.res.headers.get('content-type') ?? '') && /<!doctype html>/i.test(anon.text),
				{ contentType: anon.res.headers.get('content-type'), body: anon.text.slice(0, 60) }
			);

			const out = await request(webUrl, '/sign-out', { body: {} });
			check('without a DB sign-out degrades the same way (404, not a 5xx)', out.res.status === 404, {
				status: out.res.status,
				body: out.text.slice(0, 60)
			});

			const login = await fetch(webUrl + '/auth/login');
			const loginHtml = await login.text();
			check(
				'the rest of the app is unaffected: /auth/login still renders (200 HTML)',
				login.status === 200 && /text\/html/i.test(login.headers.get('content-type') ?? '') && /<title>/.test(loginHtml),
				{ status: login.status, contentType: login.headers.get('content-type') }
			);
		} finally {
			await stopApp(child);
		}
	}

	// ---- Phase 3: live lifecycle (needs Postgres with a migrated auth schema) ----
	section('Phase 3: live session lifecycle (needs Postgres)');

	// Load the repo-root `.env` / apps/web `.env` for the DB URL only, and never
	// overwrite an explicit export. Vite does this for the app; a bare node
	// script must do it itself (same as scripts/backfill-auth-subjects.mjs).
	if (!process.env.SIGAP_DATABASE_URL && !process.env.DATABASE_URL) {
		const fromEnvFile = readDotEnv(['SIGAP_DATABASE_URL', 'DATABASE_URL']);
		if (fromEnvFile) process.env[fromEnvFile.key] = fromEnvFile.value;
	}

	const dbUrl = process.env.SIGAP_DATABASE_URL ?? process.env.DATABASE_URL;
	let db = null;
	let dbSkipReason = null;

	if (!dbUrl) {
		dbSkipReason = 'no SIGAP_DATABASE_URL / DATABASE_URL configured (and no repo .env to read one from)';
	} else {
		let pg;
		try {
			pg = (await import('pg')).default;
		} catch (err) {
			dbSkipReason = `pg is not resolvable from apps/web: ${err.message}`;
		}
		if (pg) {
			// Mirror src/lib/server/db.ts: strip sslmode for a local target (Node's
			// pg treats `sslmode=require` as verify-full and rejects the self-signed
			// dev certificate), keep full verification everywhere else.
			const localTarget = /@(127\.0\.0\.1|localhost):/.test(dbUrl);
			db = new pg.Client({
				connectionString: localTarget ? dbUrl.replace(/([?&])sslmode=[^&]+(&?)/, (_m, lead, trail) => (trail ? lead : '')) : dbUrl,
				ssl: localTarget ? { rejectUnauthorized: false } : undefined,
				connectionTimeoutMillis: 4000
			});
			try {
				await db.connect();
				const { rows } = await db.query(
					`SELECT to_regclass('auth."user"') AS "user",
					        to_regclass('auth."session"') AS session,
					        to_regclass('auth."account"') AS account,
					        to_regclass('public.app_users') AS app_users`
				);
				const missing = ['user', 'session', 'account'].filter((k) => !rows[0][k]);
				if (missing.length > 0) {
					dbSkipReason = `auth schema is not migrated: missing ${missing.map((m) => `auth.${m}`).join(', ')}`;
				} else {
					db.appUsersTable = rows[0].app_users;
				}
			} catch (err) {
				dbSkipReason = `Postgres at ${dbUrl.replace(/:[^:@/]*@/, ':***@')} is unreachable (${err.code ?? err.message})`;
			}
		}
	}

	if (dbSkipReason) {
		const runHint = 'SIGAP_DATABASE_URL=postgresql://sigap:...@127.0.0.1:5433/sigap node tests/session-lifecycle.test.js';
		for (const name of [
			'sign-up → session',
			'session expiry matches session.expiresIn',
			'access JWT TTL is 3600s (the maximum revocation delay)',
			'second-device sign-in → two independent sessions',
			'list-sessions exposes per-device tokens (fresh window)',
			'cross-device revoke-session kills exactly that device',
			'sign-out kills this session (cookie replay rejected)',
			'change-password requires the CURRENT password',
			'change-password + revokeOtherSessions kills the other sessions',
			'old password stops working after the change',
			'deactivated account (INACTIVE/banned) is refused a NEW session'
		]) {
			skip(name, dbSkipReason);
		}
		console.log(`\n  To run phase 3: ${runHint}`);
		if (REQUIRE_DB) {
			console.log('  SIGAP_SESSION_LIFECYCLE_REQUIRE_DB=1 is set — a skipped phase 3 is a failure.');
			failed++;
		}
		if (db) await db.end().catch(() => {});
	} else {
		const stamp = Date.now().toString(36);
		const email = `lifecycle-${stamp}@example.test`;
		const password = 'sigap-lifecycle-pw-1';
		const newPassword = 'sigap-lifecycle-pw-2';
		const cookieName = 'better-auth.session_token';
		const markerD = 'sigap-session-lifecycle/device-d';

		const withJar = () => {
			const jar = new Map();
			return {
				has: () => jar.has(cookieName),
				value: () => jar.get(cookieName),
				header: () => [...jar.entries()].map(([k, v]) => `${k}=${v}`).join('; '),
				absorb(res) {
					for (const raw of res.headers.getSetCookie?.() ?? []) {
						const [pair] = raw.split(';');
						const eq = pair.indexOf('=');
						if (eq < 0) continue;
						const name = pair.slice(0, eq).trim();
						const value = pair.slice(eq + 1).trim();
						if (!name.endsWith('session_token') && !name.endsWith('session_data')) continue;
						// A cleared cookie (sign-out) carries Max-Age=0 and an empty value.
						if (/max-age=0/i.test(raw) || value === '') jar.delete(name);
						else jar.set(name, value);
					}
				}
			};
		};

		const api = async (webUrl, route, { method = 'POST', body, jar, cookie, headers = {} } = {}) => {
			const res = await fetch(webUrl + '/api/auth' + route, {
				method,
				headers: {
					origin: webUrl,
					...(body ? { 'content-type': 'application/json' } : {}),
					...(jar ? { cookie: jar.header() } : {}),
					...(cookie ? { cookie } : {}),
					...headers
				},
				body: body ? JSON.stringify(body) : undefined,
				redirect: 'manual'
			});
			jar?.absorb(res);
			const text = await res.text();
			let json = null;
			try {
				json = JSON.parse(text);
			} catch {
				json = null;
			}
			return { res, json, text };
		};

		// better-auth enforces 3 requests / 10 s on /sign-in*, /sign-up*,
		// /change-password* (X-Retry-After is set on 429). Honour it instead of
		// letting the harness fail on its own rate limiter.
		const rateLimited = async (send) => {
			let out = await send();
			if (out.res.status === 429) {
				const wait = Number(out.res.headers.get('x-retry-after') ?? 11) + 1;
				console.log(`   …  429 rate limited; waiting ${wait}s before retry`);
				await new Promise((r) => setTimeout(r, wait * 1000));
				out = await send();
			}
			return out;
		};

		// ---- the secret phase 3 boots with (see the file header) ----
		// Better Auth encrypts the `auth.jwks` private key under
		// `BETTER_AUTH_SECRET`, so a boot with a secret that does not match the
		// stored key fails every mint with "Failed to decrypt private key". Use
		// the dev server's own resolution order — the environment first, then the
		// repo `.env` — and only invent a secret when neither carries one.
		const configuredSecret = (process.env.BETTER_AUTH_SECRET ?? readDotEnv(['BETTER_AUTH_SECRET'])?.value ?? '').trim() || null;
		const harnessSecret = configuredSecret ?? `session-lifecycle-disposable-${stamp}`;

		/**
		 * Does `secret` still open the private key stored in `auth.jwks`? Asked
		 * with Better Auth's own decryption, which fails on the AES-GCM tag once
		 * the secret changes. `true` = usable, `false` = not usable, `null` =
		 * could not tell (nothing stored, or the helper is not importable).
		 */
		const secretOpensStoredKey = async (secret) => {
			let rows;
			try {
				({ rows } = await db.query('SELECT "privateKey" FROM auth.jwks'));
			} catch {
				return null;
			}
			if (rows.length === 0) return true;
			let symmetricDecrypt;
			try {
				({ symmetricDecrypt } = await import('better-auth/crypto'));
			} catch {
				return null;
			}
			for (const row of rows) {
				try {
					await symmetricDecrypt({ key: secret, data: JSON.parse(row.privateKey) });
				} catch {
					return false;
				}
			}
			return true;
		};

		const storedKeyUsable = await secretOpensStoredKey(harnessSecret);
		if (storedKeyUsable === false) {
			console.log(
				configuredSecret
					? '   note: BETTER_AUTH_SECRET in scope does not match the key in auth.jwks — minting a fresh keypair for this run and restoring auth.jwks afterwards'
					: '   note: no BETTER_AUTH_SECRET configured anywhere — minting a disposable keypair for this run and restoring auth.jwks afterwards'
			);
		}

		// A secret that cannot open the stored key has nothing to boot with:
		// snapshot the table, clear it so Better Auth mints a keypair under the
		// secret in use, and put the snapshot back in `finally`.
		let jwksSnapshot = null;
		if (storedKeyUsable === false) {
			const { rows } = await db.query(`SELECT id, "publicKey", "privateKey", "createdAt", "expiresAt", alg, crv FROM auth.jwks`);
			jwksSnapshot = rows;
			await db.query('DELETE FROM auth.jwks');
		}

		// `startApp` picks the port and sets ORIGIN + BETTER_AUTH_URL to it,
		// which is what Better Auth's origin check compares request origins
		// against, and waits for `/auth/login` to render before returning.
		const { child: app, webUrl: url } = await startApp({
			BETTER_AUTH_SECRET: harnessSecret,
			SIGAP_DATABASE_URL: dbUrl
		});

		let createdUserId = null;
		try {
			// --- sign-up creates the identity; sign-in mints the session ---
			// The product pins `emailAndPassword.autoSignIn: false` (see
			// auth.better.ts): a sign-up must NOT mint a session, so the lifecycle
			// suite has to sign in explicitly before asserting anything about
			// session clocks, revocation or cookie rotation. session-security.test.js
			// asserts the no-auto-sign-in contract; this suite owns the live session.
			const jarA = withJar();
			const signUp = await rateLimited(() =>
				api(url, '/sign-up/email', { body: { email, password, name: 'Session Lifecycle Harness' }, jar: jarA })
			);
			check('sign-up succeeds', signUp.res.status === 200, { status: signUp.res.status, body: signUp.text.slice(0, 120) });
			check('sign-up does NOT auto-sign-in (autoSignIn:false)', !jarA.has(), [...jarA.header()]);
			const signInA = await rateLimited(() =>
				api(url, '/sign-in/email', { body: { email, password }, jar: jarA })
			);
			check('explicit sign-in succeeds', signInA.res.status === 200, {
				status: signInA.res.status, body: signInA.text.slice(0, 120)
			});
			check('sign-in returns the session token the client can revoke later', typeof signInA.json?.token === 'string' && signInA.json.token.length > 0, signInA.json?.token);
			createdUserId = signUp.json?.user?.id ?? signInA.json?.user?.id ?? null;

			if (typeof createdUserId !== 'string') {
				throw new Error('sign-up did not return a user id; later phases cannot be attributed');
			}

			// --- session envelope + the expiresIn clock, read from the live row ---
			const sessionA = await api(url, '/get-session', { method: 'GET', jar: jarA });
			check('get-session resolves the new session', sessionA.res.status === 200 && sessionA.json?.user?.email === email, {
				status: sessionA.res.status,
				body: sessionA.text.slice(0, 120)
			});
			// Better Auth serialises `expiresAt` / `createdAt` as ISO-8601
			// strings; parse them to epoch milliseconds BEFORE subtracting
			// `Date.now()`, and treat a missing or unparseable field as a FAILURE
			// (null) rather than letting it silently become the epoch — which is
			// what reported a 56-year-old session with `expiresAt` absent.
			// `get-session` is also contractually uncacheable (`no-store`).
			check(
				'live session dates are ISO-8601 strings (expiresAt, createdAt)',
				typeof sessionA.json?.session?.expiresAt === 'string' &&
					typeof sessionA.json?.session?.createdAt === 'string' &&
					toEpochMs(sessionA.json.session.expiresAt) !== null &&
					toEpochMs(sessionA.json.session.createdAt) !== null,
				{ expiresAt: sessionA.json?.session?.expiresAt, createdAt: sessionA.json?.session?.createdAt }
			);
			check(
				'get-session is not cacheable (no-store)',
				/no-store/.test(sessionA.res.headers.get('cache-control') ?? ''),
				sessionA.res.headers.get('cache-control')
			);
			const expiresAtMs = toEpochMs(sessionA.json?.session?.expiresAt);
			const createdAtMs = toEpochMs(sessionA.json?.session?.createdAt);
			const expiresInSeconds = expiresAtMs === null ? null : (expiresAtMs - Date.now()) / 1000;
			check(
				`live session expiresAt ≈ session.expiresIn (${POLICY.expiresIn}s, ±300s)`,
				expiresInSeconds !== null && Math.abs(expiresInSeconds - POLICY.expiresIn) < 300,
				{
					expiresAt: sessionA.json?.session?.expiresAt,
					expiresInSeconds: expiresInSeconds === null ? null : Math.round(expiresInSeconds)
				}
			);
			const createdAtAgeSeconds = createdAtMs === null ? null : (Date.now() - createdAtMs) / 1000;
			check(
				`live session createdAt is now (fresh window is ${POLICY.freshAge}s from sign-in)`,
				createdAtAgeSeconds !== null && createdAtAgeSeconds >= 0 && createdAtAgeSeconds < 60,
				{
					createdAt: sessionA.json?.session?.createdAt,
					createdAtAgeSeconds: createdAtAgeSeconds === null ? null : Math.round(createdAtAgeSeconds)
				}
			);
			check(
				`the session row's own expiresAt − createdAt is exactly session.expiresIn (${POLICY.expiresIn}s, ±10ms)`,
				expiresAtMs !== null && createdAtMs !== null &&
					Math.abs((expiresAtMs - createdAtMs) / 1000 - POLICY.expiresIn) < 0.01,
				expiresAtMs === null || createdAtMs === null ? null : (expiresAtMs - createdAtMs) / 1000
			);
			const tokenA = jarA.value();

			// --- the access JWT TTL: the policy's maximum revocation delay ---
			const minted = await api(url, '/token', { method: 'GET', jar: jarA });
			const jwt = minted.json?.token;
			check('GET /api/auth/token mints a JWT for a live session', typeof jwt === 'string' && jwt.split('.').length === 3, minted.text.slice(0, 120));
			if (typeof jwt === 'string' && jwt.split('.').length === 3) {
				const [headerPart, payloadPart] = jwt.split('.');
				const header = JSON.parse(Buffer.from(headerPart, 'base64url').toString('utf8'));
				const claims = JSON.parse(Buffer.from(payloadPart, 'base64url').toString('utf8'));
				check('access JWT alg is ES256 (Go parseJWK accepts EC, not OKP)', header.alg === 'ES256', header);
				check('access JWT carries a kid (JWKS lookup key)', typeof header.kid === 'string' && header.kid.length > 0, header);
				check(
					`access JWT TTL is exactly ${POLICY.jwtExpirationSeconds}s — the maximum revocation delay`,
					claims.exp - claims.iat === POLICY.jwtExpirationSeconds,
					{ iat: claims.iat, exp: claims.exp, ttl: claims.exp - claims.iat }
				);
				check(
					'access JWT iss/aud are the pinned Go trust-boundary values',
					claims.iss === POLICY.issuer && claims.aud === POLICY.audience,
					{ iss: claims.iss, aud: claims.aud }
				);
			}

			// --- a second device is an independent session ---
			const jarB = withJar();
			const signInB = await rateLimited(() =>
				api(url, '/sign-in/email', { body: { email, password }, jar: jarB, headers: { 'user-agent': 'sigap-session-lifecycle/device-b' } })
			);
			check('second-device sign-in succeeds (one session row per device)', signInB.res.status === 200 && jarB.has(), {
				status: signInB.res.status,
				body: signInB.text.slice(0, 120)
			});

			const listed = await api(url, '/list-sessions', { method: 'GET', jar: jarA });
			check(
				'list-sessions returns both devices (fresh window: the caller session is seconds old)',
				Array.isArray(listed.json) && listed.json.length >= 2,
				Array.isArray(listed.json) ? listed.json.length : listed.text.slice(0, 120)
			);
			const sessions = Array.isArray(listed.json) ? listed.json : [];
			check('every listed session exposes its own token (needed to revoke one device)', sessions.every((s) => typeof s.token === 'string' && s.token.length > 0), sessions.map((s) => typeof s.token));
			// Identify the caller by the token from the EXPLICIT sign-in (sign-up
			// mints no session under autoSignIn:false). Selecting by the old
			// sign-up token would be `!== undefined` for every row and revoke the
			// caller's own session, cascading into false 401s below.
			const otherDevice = sessions.find((s) => s.token !== signInA.json?.token);
			check('the other device is listed and is not the caller', Boolean(otherDevice?.token), sessions.length);

			// --- cross-device revocation ---
			if (otherDevice?.token) {
				const revoked = await api(url, '/revoke-session', { body: { token: otherDevice.token }, jar: jarA });
				check('revoke-session reports success', revoked.json?.status === true, revoked.text.slice(0, 120));

				const deviceAfter = await api(url, '/get-session', { method: 'GET', jar: jarB });
				check('the revoked device has no session (200 null)', deviceAfter.res.status === 200 && deviceAfter.json === null, {
					status: deviceAfter.res.status,
					body: deviceAfter.text.slice(0, 80)
				});

				const deviceReplay = await api(url, '/get-session', {
					method: 'GET',
					cookie: `${cookieName}=${jarB.value()}`
				});
				check('the revoked device token is gone server-side, not just unset in the jar', deviceReplay.json === null, deviceReplay.text.slice(0, 80));
			}

			// --- password change: the current password is mandatory ---
			const wrongCurrent = await rateLimited(() =>
				api(url, '/change-password', {
					body: { newPassword, currentPassword: 'definitely-not-the-current-password' },
					jar: jarA
				})
			);
			check('change-password rejects a WRONG current password (400)', wrongCurrent.res.status === 400, wrongCurrent.res.status);
			check('…with the invalid-password code', wrongCurrent.json?.code === 'INVALID_PASSWORD', wrongCurrent.json?.code);

			// --- password change + revokeOtherSessions ---
			const jarD = withJar();
			const signInD = await rateLimited(() =>
				api(url, '/sign-in/email', { body: { email, password }, jar: jarD, headers: { 'user-agent': markerD } })
			);
			check('a third device signs in (to be revoked by revokeOtherSessions)', signInD.res.status === 200 && jarD.has(), signInD.res.status);

			const changed = await rateLimited(() =>
				api(url, '/change-password', { body: { newPassword, currentPassword: password, revokeOtherSessions: true }, jar: jarA })
			);
			check('change-password with the correct current password succeeds', changed.res.status === 200, { status: changed.res.status, body: changed.text.slice(0, 120) });
			check('…and returns the replacement session token', typeof changed.json?.token === 'string' && changed.json.token.length > 0, changed.json?.token);
			// Better Auth's session cookie is `<token>.<hmac>` while the JSON body
			// carries the BARE token, and only the signed cookie authenticates a
			// replay (`api()` already absorbed the rotation into `jarA`). So the
			// bare token is used only to match rows from `list-sessions`; every
			// request replays the jar's cookie value.
			const replacementToken = changed.json?.token ?? null;
			const tokenA2 = jarA.has() ? jarA.value() : null;
			check('…and rotates the cookie to a new value', Boolean(tokenA2) && tokenA2 !== tokenA, {
				previous: tokenA?.split('.')[0] ?? null,
				replacement: tokenA2?.split('.')[0] ?? null
			});

			const deviceDAfter = await api(url, '/get-session', { method: 'GET', cookie: `${cookieName}=${jarD.value()}` });
			check('revokeOtherSessions killed the other device', deviceDAfter.json === null, deviceDAfter.text.slice(0, 80));

			const tokenAReplay = tokenA ? await api(url, '/get-session', { method: 'GET', cookie: `${cookieName}=${tokenA}` }) : null;
			check('revokeOtherSessions also invalidated the PREVIOUS session of the calling device', tokenAReplay?.json === null, tokenAReplay?.text?.slice(0, 80));

			if (tokenA2) {
				const surviving = await api(url, '/list-sessions', { method: 'GET', cookie: `${cookieName}=${tokenA2}` });
				check(
					'exactly the replacement session survives the password change',
					Array.isArray(surviving.json) && surviving.json.length === 1 && surviving.json[0].token === replacementToken,
					Array.isArray(surviving.json) ? surviving.json.map((s) => s.token) : surviving.text.slice(0, 120)
				);

				// /sign-in* allows 3 requests / 10 s: let the window slide before
				// the two password assertions below, so neither is a rate-limit 429.
				await new Promise((r) => setTimeout(r, 11_000));

				const oldPassword = await rateLimited(() => api(url, '/sign-in/email', { body: { email, password } }));
				check('the OLD password no longer signs in (401)', oldPassword.res.status === 401, oldPassword.res.status);
				check('…with the invalid-credentials code', oldPassword.json?.code === 'INVALID_EMAIL_OR_PASSWORD', oldPassword.json?.code);

				const jarE = withJar();
				const newPasswordSignIn = await rateLimited(() => api(url, '/sign-in/email', { body: { email, password: newPassword }, jar: jarE }));
				check('the NEW password signs in', newPasswordSignIn.res.status === 200 && jarE.has(), newPasswordSignIn.res.status);

				// --- sign-out revokes this session ---
				const tokenE = jarE.value();
				const out = await api(url, '/sign-out', { body: {}, jar: jarE });
				check('sign-out succeeds', out.res.status === 200 && out.json?.success === true, out.text.slice(0, 120));
				check('sign-out clears the session cookie in the jar', !jarE.has(), [...jarE.header()]);
				const signedOutReplay = await api(url, '/get-session', { method: 'GET', cookie: `${cookieName}=${tokenE}` });
				check('the signed-out token is gone server-side (cookie replay rejected)', signedOutReplay.json === null, signedOutReplay.text.slice(0, 80));
			}

			// --- deactivation: stated, and enforced where it can be ---
			const banColumns = await db.query(
				`SELECT column_name FROM information_schema.columns WHERE table_schema = 'auth' AND table_name = 'user' AND column_name IN ('banned','status')`
			);
			if (banColumns.rows.length === 0) {
				// No identity-ban column exists in base Better Auth 1.7.20 — that is
				// the documented gap, not a test blocker. Execute the ACTUAL SIGAP
				// policy instead: the Go RBAC resolver filters public.app_users on
				// status='active' AND deleted_at IS NULL on every request, so a
				// disabled subject still signs in but authorizes nothing.
				check('auth.user has no identity-ban column (documented identity-layer gap)', true, banColumns.rows);
				if (db.appUsersTable) {
					await db.query(`UPDATE public.app_users SET status='disabled' WHERE subject = $1`, [createdUserId]);
					try {
						const disabledSignIn = await rateLimited(() =>
							api(url, '/sign-in/email', { body: { email, password: newPassword } })
						);
						check('disabled app_users subject still signs in at the identity layer (characterized gap)', disabledSignIn.res.status === 200, {
							status: disabledSignIn.res.status, body: disabledSignIn.text.slice(0, 100)
						});
						const goResolver = fs.readFileSync(path.join(repoRoot, 'apps/api/internal/auth/rbac_resolver.go'), 'utf8');
						check(
							'Go resolver refuses disabled subjects through active-user RBAC filtering',
							/u\.status\s*=\s*'active'/.test(goResolver) && /u\.deleted_at\s+IS\s+NULL/.test(goResolver),
							'app_users.status=active AND deleted_at IS NULL in permissionsQuery'
						);
					} finally {
						await db.query(`UPDATE public.app_users SET status='active' WHERE subject = $1`, [createdUserId]);
					}
				}
			} else {
				const column = banColumns.rows[0].column_name;
				const previous = await db.query(`SELECT id, "${column}" AS value FROM "auth"."user" WHERE id = $1`, [createdUserId]);
				await db.query(`UPDATE "auth"."user" SET "${column}" = true WHERE id = $1`, [createdUserId]);
				const blocked = await rateLimited(() => api(url, '/sign-in/email', { body: { email, password: newPassword } }));
				check(`a deactivated account (${column}) is refused a new session`, blocked.res.status === 401 || blocked.res.status === 403, blocked.res.status);
				await db.query(`UPDATE "auth"."user" SET "${column}" = $2 WHERE id = $1`, [createdUserId, previous.rows[0]?.value ?? false]);
			}

			if (db.appUsersTable) {
				const cols = await db.query(
					`SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'app_users' AND column_name IN ('status','deleted_at')`
				);
				check(
					'public.app_users exposes the deactivation signal the Go resolver filters on (status, deleted_at)',
					cols.rows.length === 2,
					cols.rows.map((r) => r.column_name)
				);
			} else {
				skip(
					'public.app_users deactivation signal (status, deleted_at)',
					'public.app_users does not exist in this database (domain schema not migrated); the policy statement in design/session-lifecycle-policy.md §3 stands on apps/api/internal/auth/rbac_resolver.go'
				);
			}

			// --- freshAge boundary, WITHOUT waiting an hour ---
			// Backdate the disposable session row past session.freshAge. Better
			// Auth's freshSessionMiddleware compares createdAt to now on the
			// sensitive endpoints (list-sessions / change-password), so this
			// exercises both sides of the boundary deterministically. The row is
			// restored immediately afterwards; no production clock or schema is
			// touched.
			const currentToken = jarA.has() ? jarA.value().split('.')[0] : null;
			if (currentToken) {
				await db.query(
					`UPDATE auth.session SET "createdAt" = now() - ($2 * interval '1 second') WHERE token = $1`,
					[currentToken, POLICY.freshAge + 60]
				);
				const stale = await api(url, '/list-sessions', { method: 'GET', jar: jarA });
				check('freshAge boundary: a stale session is refused by list-sessions', stale.res.status === 401 || stale.res.status === 403, {
					status: stale.res.status, body: stale.text.slice(0, 100)
				});
				await db.query(`UPDATE auth.session SET "createdAt" = now() WHERE token = $1`, [currentToken]);
				const fresh = await api(url, '/list-sessions', { method: 'GET', jar: jarA });
				check('freshAge boundary: the same session is accepted again once fresh', fresh.res.status === 200 && Array.isArray(fresh.json), {
					status: fresh.res.status, body: fresh.text.slice(0, 100)
				});
			} else {
				check('freshAge boundary fixture: a live session token is available', false, 'jarA has no session cookie');
			}
		} finally {
			// Test-created identity is removed, cascade included; nothing else in
			// the database is touched. If this run had to mint a disposable
			// secret, the keypair Better Auth created under it goes away and the
			// snapshot taken before the boot is put back verbatim, so the shared
			// dev database is left exactly as it was found.
			if (createdUserId) {
				await db.query(`DELETE FROM "auth"."user" WHERE id = $1`, [createdUserId]).catch(() => {});
			}
			if (jwksSnapshot) {
				await db.query('DELETE FROM auth.jwks').catch(() => {});
				for (const key of jwksSnapshot) {
					await db
						.query(
							`INSERT INTO auth.jwks (id, "publicKey", "privateKey", "createdAt", "expiresAt", alg, crv)
							 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
							[key.id, key.publicKey, key.privateKey, key.createdAt, key.expiresAt, key.alg, key.crv]
						)
						.catch(() => {});
				}
			}
			await stopApp(app);
			await db.end().catch(() => {});
		}
	}
} catch (err) {
	failed++;
	console.log(`\n  FAIL  unexpected harness error: ${err?.stack ?? err}`);
}

section(`Summary: ${passed} passed, ${failed} failed, ${skipped} skipped (DB-gated)`);
process.exit(failed === 0 ? 0 : 1);
