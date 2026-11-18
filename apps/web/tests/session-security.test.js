// Session security contract tests — Better Auth session security negatives on
// the BUILT app, complementary to `session-lifecycle.test.js` (which owns the
// lifecycle happy paths: sign-up, multi-device sessions, revoke-session,
// sign-out, change-password + revokeOtherSessions, the access JWT TTL).
//
// This file owns the SECURITY negatives and the trust-boundary contract:
//
//   Phase 1 (static, no DB, always runs) — cross-language trust-boundary
//   assertions read from the source:
//     - `jwks.rotationInterval` is set (the ES256 signing key cannot live
//       forever) and the Go verifier's 15-min JWKS cache bound is documented
//       next to it;
//     - the Go verifier enforces issuer + audience with ZERO leeway (no
//       `jwt.WithLeeway` in `apps/api/internal/auth/jwt_provider.go`);
//     - the Go JWKS cache TTL constant is 15 minutes (the retired-key
//       acceptance bound);
//     - Go jwt mode requires `SIGAP_AUTH_ISSUER` + `SIGAP_AUTH_AUDIENCE`
//       (config.go Validate);
//     - the Go RBAC resolver filters on `app_users.status = 'active' AND
//       deleted_at IS NULL` (the deactivation enforcement point);
//     - the auth schema has no `banned`/`status` column on `auth.user` (the
//       identity-boundary gap, recorded in session-lifecycle-policy.md §3).
//
//   Phase 2 (live, needs Postgres with a migrated auth schema) — behavioural
//   security negatives. SELF-CONTAINED: the suite owns its fixture — per-run
//   accounts (timestamp + random suffix), devices signed in explicitly
//   (sign-up mints none: the product pins `autoSignIn: false`), and every row
//   it creates deleted in the `finally` (asserted by a residue check). Re-runs
//   neither depend on nor leave fixture state; a device may serve more than one
//   case, which is stated where it happens.
//     - cross-USER revocation isolation: a caller can only revoke ITS OWN
//       sessions; revoking another user's session token is a silent no-op and
//       the target session survives;
//     - session switching isolation: `/revoke-other-sessions` from device B
//       kills device A and leaves device B signed in;
//     - revocation: `/revoke-session` with one's own token deletes that row and
//       its cookie is dead afterwards;
//     - logout: `/sign-out` deletes the session row and the SIGNED cookie the
//       server issued before sign-out cannot be replayed for `get-session`
//       (200 null) or `/token` (401);
//     - refresh: a session whose `updatedAt` is older than `updateAge` has
//       `expiresAt` rolled forward on `get-session` (the 1-day cadence write);
//     - expiration: a session row with `expiresAt` in the past answers
//       `get-session` with 200 null and `/token` refuses to mint (401);
//     - JWT/JWKS live shape: the minted access token's kid matches a key
//       actually served by `/api/auth/jwks`, with alg ES256 / crv P-256, and
//       iss/aud are the pinned trust-boundary values;
//     - password-change default (no revokeOtherSessions): other devices'
//       sessions survive, the OLD password stops working and the NEW one works;
//     - deactivation gap-characterization: with `app_users.status = 'disabled'`
//       the identity layer STILL signs the account in (recorded gap — the Go
//       API is the enforcement point, per the static phase and the policy doc),
//       and the `app_users` columns the resolver filters on exist.
//
// Note on cookies: better-auth signs the session cookie
// (`setSignedCookie` / `getSignedCookie`), so a session is ONLY presentable
// through the cookie jar the server itself filled — a raw `auth.session.token`
// used as a cookie value is not a session.
//
// Usage: node tests/session-security.test.js
// Phase 2 is DB-gated exactly like session-lifecycle.test.js: it skips with
// `[GATE] SKIP …` when no reachable Postgres with a migrated `auth` schema is
// found, and exits 0. Set
// SIGAP_SESSION_SECURITY_REQUIRE_DB=1 to make a skip fatal.

import assert from 'node:assert';
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');
const repoRoot = path.join(root, '..', '..');
const buildEntry = path.join(root, 'build', 'index.js');
const authSourcePath = path.join(root, 'src', 'lib', 'server', 'auth.better.ts');
const policyPath = path.join(repoRoot, 'design', 'session-lifecycle-policy.md');
const secretJwtProviderPath = path.join(repoRoot, 'apps', 'api', 'internal', 'auth', 'jwt_provider.go');
const goConfigPath = path.join(repoRoot, 'apps', 'api', 'internal', 'auth', 'config.go');
const rbacResolverPath = path.join(repoRoot, 'apps', 'api', 'internal', 'auth', 'rbac_resolver.go');

const REQUIRE_DB = process.env.SIGAP_SESSION_SECURITY_REQUIRE_DB === '1';

// The trust-boundary numbers, asserted in the config source, the Go source, and
// (phase 2) the live JWKS / JWT. One source of truth for this file.
const TRUST = {
	rotationIntervalSeconds: 60 * 60 * 24 * 30, // 30 days
	goJwksCacheTtlMinutes: 15,
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
// Source helpers (same shape as session-lifecycle.test.js)
// ---------------------------------------------------------------------------

const authSource = fs.readFileSync(authSourcePath, 'utf8');
const goJwtProvider = fs.readFileSync(secretJwtProviderPath, 'utf8');
const goConfig = fs.readFileSync(goConfigPath, 'utf8');
const rbacResolver = fs.readFileSync(rbacResolverPath, 'utf8');

const extractBlock = (source, key) => {
	// `\t+`, not `\t`: the block this file reads (`jwks`) is nested three levels
	// deep in the config (betterAuth → plugins → jwt → jwks), so a single-tab
	// anchor never matched it.
	const match = new RegExp(`\\n\\t+${key}: \\{`).exec(source);
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

const evalSeconds = (expression) => {
	const trimmed = String(expression).trim();
	assert.match(trimmed, /^[\d\s*+()]+$/, `unexpected duration expression: ${trimmed}`);
	return Number(Function(`"use strict"; return (${trimmed});`)());
};

const optionValue = (block, key) => {
	if (!block) return null;
	const match = new RegExp(`(?:^|[,{\\s])${key}\\s*:\\s*([^,}\\n]+)`).exec(block);
	return match ? match[1].trim() : null;
};

const readDotEnv = (names) => {
	const pattern = new RegExp(`^\\s*(${names.join('|')})\\s*=\\s*(.*)$`);
	for (const envPath of [path.join(repoRoot, '.env'), path.join(root, '.env')]) {
		if (!fs.existsSync(envPath)) continue;
		for (const line of fs.readFileSync(envPath, 'utf8').split(/\r?\n/)) {
			const m = pattern.exec(line);
			if (m) return { key: m[1], value: m[2].trim().replace(/^["']|["']$/g, '') };
		}
	}
	return null;
};

const toEpochMs = (value) => {
	if (value instanceof Date) return Number.isFinite(value.getTime()) ? value.getTime() : null;
	if (typeof value === 'number') return Number.isFinite(value) ? value : null;
	if (typeof value !== 'string') return null;
	const ms = Date.parse(value);
	return Number.isFinite(ms) ? ms : null;
};

const jwtwksBlock = extractBlock(authSource, 'jwks');

// ---------------------------------------------------------------------------
// Phase 1: static, cross-language trust-boundary assertions
// ---------------------------------------------------------------------------

try {
	section('Phase 1: trust-boundary config (static, no DB)');

	check('jwks block is configured explicitly', Boolean(jwtwksBlock), jwtwksBlock);
	check(
		'jwks.rotationInterval is set, so the ES256 signing key cannot live forever',
		Boolean(jwtwksBlock) && !/rotationInterval:\s*(undefined)?[,}\n]/.test(jwtwksBlock) && /rotationInterval:\s*[^,}\n]+/.test(jwtwksBlock),
		optionValue(jwtwksBlock, 'rotationInterval')
	);
	if (jwtwksBlock) {
		const rotationExpression = optionValue(jwtwksBlock, 'rotationInterval');
		check(
			`rotationInterval evaluates to ${TRUST.rotationIntervalSeconds}s (30 days)`,
			rotationExpression !== null && evalSeconds(rotationExpression) === TRUST.rotationIntervalSeconds,
			rotationExpression
		);
	}
	// The rationale is written ABOVE `rotationInterval:` in auth.better.ts (and
	// partly below it), so the assertion is a window around the option rather
	// than "text after it". The window must name the Go verifier and the
	// 15-minute cache bound the retired-key pickup is capped by.
	const rotationIndex = authSource.indexOf('rotationInterval:');
	const rotationComment = rotationIndex < 0 ? '' : authSource.slice(Math.max(0, rotationIndex - 1500), rotationIndex + 300);
	check(
		'the rotation comment documents the Go verifier 15-min cache bound on retired-key pickup',
		/15 minutes/.test(rotationComment) && /jwt_provider\.go/.test(rotationComment),
		{
			mentions15Minutes: /15 minutes/.test(rotationComment),
			namesGoVerifier: /jwt_provider\.go/.test(rotationComment)
		}
	);
	check(
		'the Go verifier enforces issuer AND audience (WithIssuer / WithAudience)',
		goJwtProvider.includes('jwt.WithIssuer') && goJwtProvider.includes('jwt.WithAudience'),
		{ withIssuer: goJwtProvider.includes('jwt.WithIssuer'), withAudience: goJwtProvider.includes('jwt.WithAudience') }
	);
	check(
		'the Go verifier has ZERO leeway (jwt.WithLeeway is absent)',
		!goJwtProvider.includes('jwt.WithLeeway') && !goJwtProvider.includes('WithLeeway'),
		goJwtProvider.match(/WithLeeway[^\n]*/)?.[0] ?? 'absent'
	);
	check(
		`the Go JWKS cache TTL is exactly ${TRUST.goJwksCacheTtlMinutes} minutes (retired-key bound)`,
		/ttl:\s*15\s*\*\s*time\.Minute/.test(goJwtProvider),
		goJwtProvider.match(/ttl:\s*[^\n]+/)?.[0] ?? 'ttl not found'
	);
	check(
		'Go jwt mode requires SIGAP_AUTH_ISSUER and SIGAP_AUTH_AUDIENCE (config.go Validate)',
		goConfig.includes('SIGAP_AUTH_ISSUER is not set') && goConfig.includes('SIGAP_AUTH_AUDIENCE is not set'),
		'Validate() issuer/audience'
	);
	check(
		'the Go RBAC resolver enforces deactivation at authorization (status = active, deleted_at IS NULL)',
		/u\.status = 'active'/.test(rbacResolver) && /deleted_at IS NULL/.test(rbacResolver),
		rbacResolver.match(/u\.status = 'active'[^\n]*/)?.[0] ?? 'filter not found'
	);
	check(
		'policy doc records the missing auth.user banned/status column (identity-boundary gap)',
		fs.readFileSync(policyPath, 'utf8').includes('banned'),
		'policy §3 banned'
	);

	// ---- Phase 2: live security negatives (needs Postgres) ----
	section('Phase 2: live session security negatives (needs Postgres)');

	if (!fs.existsSync(buildEntry)) {
		console.log('build/ not found — running vite build first');
		const built = spawnSync('pnpm build', { cwd: root, shell: true, stdio: 'inherit' });
		if (built.status !== 0) {
			console.error('vite build failed');
			process.exit(1);
		}
	} else {
		// The live phase must exercise the CURRENT config (including the new
		// rotationInterval), so the built app is rebuilt even when build/ exists.
		console.log('rebuilding so the live phase exercises the current auth config…');
		const rebuilt = spawnSync('pnpm build', { cwd: root, shell: true, stdio: 'inherit' });
		if (rebuilt.status !== 0) {
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

	const cleanEnv = (overrides) => {
		const env = { ...process.env };
		delete env.SIGAP_DATABASE_URL;
		delete env.DATABASE_URL;
		delete env.BETTER_AUTH_SECRET;
		delete env.BETTER_AUTH_URL;
		return { ...env, ...Object.fromEntries(Object.entries(overrides).filter(([, v]) => v !== undefined)) };
	};

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
				// Pinned, not inherited: the live `iss`/`aud` assertions below
				// compare against TRUST, so the server must be configured from the
				// same constants (an ambient SIGAP_AUTH_ISSUER would silently
				// redefine what "the pinned trust-boundary values" means).
				SIGAP_AUTH_ISSUER: TRUST.issuer,
				SIGAP_AUTH_AUDIENCE: TRUST.audience,
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

	// Load the repo-root `.env` / apps/web `.env` for the DB URL only.
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
			const localTarget = /@(127\.0\.0\.1|localhost):/.test(dbUrl);
			// An EXPLICIT sslmode=disable must be honored even for a loopback
			// target: CI service containers accept plain TCP only, while local
			// dev clusters use a self-signed certificate with sslmode=require.
			const sslDisabled = /[?&]sslmode=disable(?:&|$)/i.test(dbUrl);
			db = new pg.Client({
				connectionString: localTarget ? dbUrl.replace(/([?&])sslmode=[^&]+(&?)/, (_m, lead, trail) => (trail ? lead : '')) : dbUrl,
				ssl: localTarget && !sslDisabled ? { rejectUnauthorized: false } : undefined,
				connectionTimeoutMillis: 4000
			});
			try {
				await db.connect();
				const { rows } = await db.query(
					`SELECT to_regclass('auth."user"') AS "user",
					        to_regclass('auth."session"') AS session,
					        to_regclass('auth."jwks"') AS jwks,
					        to_regclass('public.app_users') AS app_users`
				);
				const missing = ['user', 'session', 'jwks'].filter((k) => !rows[0][k]);
				if (missing.length > 0) {
					dbSkipReason = `auth schema is not migrated or jwks table missing: ${missing.map((m) => `auth.${m}`).join(', ')}`;
				} else {
					db.appUsersTable = rows[0].app_users;
				}
			} catch (err) {
				dbSkipReason = `Postgres at ${dbUrl.replace(/:[^:@/]*@/, ':***@')} is unreachable (${err.code ?? err.message})`;
			}
		}
	}

	if (dbSkipReason) {
		const runHint = 'SIGAP_DATABASE_URL=postgresql://sigap:...@127.0.0.1:5433/sigap node tests/session-security.test.js';
		for (const name of [
			'cross-user revocation isolation (foreign token is a no-op)',
			'session switching: revoke-other-sessions kills the other device only',
			'refresh: stale updatedAt rolls expiresAt forward (updateAge)',
			'expiration: past expiresAt ⇒ get-session null and /token 401',
			'live JWKS/JWT shape: kid matches a served key, ES256/P-256, iss/aud',
			'password change without revokeOtherSessions: other device survives, old password dead',
			'deactivation gap-characterization: disabled app_users row still signs in (Go API enforces)'
		]) {
			skip(name, dbSkipReason);
		}
		console.log(`\n  To run phase 2: ${runHint}`);
		if (REQUIRE_DB) {
			console.log('  SIGAP_SESSION_SECURITY_REQUIRE_DB=1 is set — a skipped phase 2 is a failure.');
			failed++;
		}
		if (db) await db.end().catch(() => {});
	} else {
		// Per-run fixture accounts: timestamp + random suffix, so a re-run can
		// never collide with another run's residue — and every row this run
		// creates is deleted in the `finally` below.
		const stamp = `${Date.now().toString(36)}-${randomUUID().slice(0, 8)}`;
		const emailA = `sec-a-${stamp}@example.test`;
		const emailB = `sec-b-${stamp}@example.test`;
		const password = 'sigap-security-pw-1';
		const newPassword = 'sigap-security-pw-2';
		const cookieName = 'better-auth.session_token';
		const markerA = 'sigap-session-security/device-a';
		const markerB = 'sigap-session-security/device-b';
		const markerC = 'sigap-session-security/device-c';
		const markerE = 'sigap-session-security/device-e';

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

		// Better Auth's per-path buckets are TIGHT on the credential paths
		// (`/sign-in/email` refuses the 4th request inside its 10 s window once a
		// few devices have signed in), and this harness deliberately creates many
		// devices — so a 429 is expected fixture noise, not a product failure.
		// Retry up to four times, honouring the advertised retry-after.
		const rateLimited = async (send) => {
			let out = await send();
			for (let attempt = 0; attempt < 4 && out.res.status === 429; attempt++) {
				const wait = Number(out.res.headers.get('x-retry-after') ?? 11) + 1;
				console.log(`   …  429 rate limited; waiting ${wait}s before retry ${attempt + 2}`);
				await new Promise((r) => setTimeout(r, wait * 1000));
				out = await send();
			}
			return out;
		};

		const configuredSecret = (process.env.BETTER_AUTH_SECRET ?? readDotEnv(['BETTER_AUTH_SECRET'])?.value ?? '').trim() || null;
		const harnessSecret = configuredSecret ?? `session-security-disposable-${stamp}`;

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
		let jwksSnapshot = null;
		if (storedKeyUsable === false) {
			console.log(
				configuredSecret
					? '   note: BETTER_AUTH_SECRET in scope does not match the key in auth.jwks — minting a fresh keypair for this run and restoring auth.jwks afterwards'
					: '   note: no BETTER_AUTH_SECRET configured anywhere — minting a disposable keypair for this run and restoring auth.jwks afterwards'
			);
			const { rows } = await db.query(`SELECT id, "publicKey", "privateKey", "createdAt", "expiresAt", alg, crv FROM auth.jwks`);
			jwksSnapshot = rows;
			await db.query('DELETE FROM auth.jwks');
		}

		const { child: app, webUrl: url } = await startApp({
			BETTER_AUTH_SECRET: harnessSecret,
			SIGAP_DATABASE_URL: dbUrl
		});

		// A "device" is one independent sign-in with its OWN cookie jar. Sign-up
		// does NOT mint a session (`emailAndPassword.autoSignIn: false`, asserted
		// below), so every device in this file is an explicit /sign-in/email.
		//
		// The cookie the server sets is SIGNED (`setSignedCookie` →
		// `getSignedCookie` on read in better-auth 1.7.20), so a raw
		// `auth.session.token` from JSON is NOT usable as a cookie: the only way
		// to present a session is the jar the server itself filled.
		const device = async (email, pw, userAgent) => {
			const jar = withJar();
			const out = await rateLimited(() =>
				api(url, '/sign-in/email', { body: { email, password: pw }, jar, headers: { 'user-agent': userAgent } })
			);
			return { ...out, jar, token: out.json?.token ?? jar.value() ?? null };
		};

		const createdUserIds = [];
		try {
			// --- two independent accounts; sign-up provisions, sign-in mints ---
			const signUpA = await rateLimited(() => api(url, '/sign-up/email', { body: { email: emailA, password, name: 'Session Security Harness A' } }));
			check(
				'account A sign-up succeeds and returns a user id',
				signUpA.res.status === 200 && typeof signUpA.json?.user?.id === 'string',
				{ status: signUpA.res.status, body: signUpA.text.slice(0, 120) }
			);
			const userIdA = signUpA.json?.user?.id ?? null;
			if (typeof userIdA !== 'string') throw new Error('sign-up A did not return a user id');
			createdUserIds.push(userIdA);
			// The product pins `autoSignIn: false` (anti-enumeration: a duplicate
			// address gets the same generic 200 + synthetic user). The fixture
			// design above depends on the consequence: sign-up leaves NO session.
			check(
				'sign-up does NOT mint a session (autoSignIn: false — token null, no session cookie)',
				signUpA.json?.token === null && !(signUpA.res.headers.getSetCookie?.() ?? []).some((c) => c.startsWith(`${cookieName}=`)),
				{ token: signUpA.json?.token ?? null, setCookie: signUpA.res.headers.getSetCookie?.() ?? [] }
			);

			const signUpB = await rateLimited(() => api(url, '/sign-up/email', { body: { email: emailB, password, name: 'Session Security Harness B' } }));
			check(
				'account B sign-up succeeds and returns a user id',
				signUpB.res.status === 200 && typeof signUpB.json?.user?.id === 'string',
				{ status: signUpB.res.status, body: signUpB.text.slice(0, 120) }
			);
			const userIdB = signUpB.json?.user?.id ?? null;
			if (typeof userIdB !== 'string') throw new Error('sign-up B did not return a user id');
			createdUserIds.push(userIdB);

			// device A1 / device B1 — one device per account, one session each.
			const devA1 = await device(emailA, password, markerA);
			check('account A device 1 signs in', devA1.res.status === 200 && devA1.jar.has(), devA1.res.status);
			const sessionTokenA = devA1.token;
			check('account A has a session token', typeof sessionTokenA === 'string' && sessionTokenA.length > 0, typeof sessionTokenA);

			const devB1 = await device(emailB, password, markerB);
			check('account B device 1 signs in', devB1.res.status === 200 && devB1.jar.has(), devB1.res.status);
			const sessionTokenB = devB1.token;
			check('account B has a session token', typeof sessionTokenB === 'string' && sessionTokenB.length > 0, typeof sessionTokenB);

			// --- cross-USER revocation isolation (IDOR negative) ---
			const listedA = await api(url, '/list-sessions', { method: 'GET', jar: devA1.jar });
			check('account A list-sessions shows only its own session', Array.isArray(listedA.json) && listedA.json.length === 1, Array.isArray(listedA.json) ? listedA.json.length : listedA.text.slice(0, 120));
			check(
				"account B's session token is NOT visible to account A",
				Array.isArray(listedA.json) && !listedA.json.some((s) => s.token === sessionTokenB),
				Array.isArray(listedA.json) ? listedA.json.map((s) => s.token) : listedA.text.slice(0, 120)
			);
			const foreignRevoke = await api(url, '/revoke-session', { body: { token: sessionTokenB }, jar: devA1.jar });
			check('revoking a FOREIGN session token answers success (no information leak)', foreignRevoke.res.status === 200 && foreignRevoke.json?.status === true, foreignRevoke.text.slice(0, 120));
			// Presented through B's OWN jar: only the signed cookie the server set
			// is a session, so this is a real replay of B's session, not a no-op.
			const deviceBAfterForeign = await api(url, '/get-session', { method: 'GET', jar: devB1.jar });
			check('the foreign session SURVIVES the cross-user revoke attempt (isolation holds)', deviceBAfterForeign.res.status === 200 && deviceBAfterForeign.json !== null, deviceBAfterForeign.text.slice(0, 120));

			// --- session switching: device B of account A, revoke-other-sessions ---
			const devA2 = await device(emailA, password, markerC);
			check('account A signs in on a second device', devA2.res.status === 200 && devA2.jar.has(), devA2.res.status);
			const listedA2 = await api(url, '/list-sessions', { method: 'GET', jar: devA2.jar });
			check('device B sees both of account A sessions', Array.isArray(listedA2.json) && listedA2.json.length === 2, Array.isArray(listedA2.json) ? listedA2.json.length : listedA2.text.slice(0, 120));

			const revokeOthers = await api(url, '/revoke-other-sessions', { body: {}, jar: devA2.jar });
			check('revoke-other-sessions succeeds', revokeOthers.res.status === 200 && revokeOthers.json?.status === true, revokeOthers.text.slice(0, 120));
			// Replaying device A1's own signed cookie (still in its jar) must fail:
			// the row is gone, the cookie alone cannot resurrect the session.
			const deviceAAfter = await api(url, '/get-session', { method: 'GET', jar: devA1.jar });
			check('the OTHER device session is revoked', deviceAAfter.json === null, deviceAAfter.text.slice(0, 80));
			const revokedRow = await db.query(`SELECT count(*)::int AS n FROM auth."session" WHERE token = $1`, [sessionTokenA]);
			check('the revoked device row is gone from auth.session', revokedRow.rows[0].n === 0, revokedRow.rows[0].n);
			const deviceCAfter = await api(url, '/get-session', { method: 'GET', jar: devA2.jar });
			check('the CALLING device session survives (session switching) ', deviceCAfter.res.status === 200 && deviceCAfter.json !== null, deviceCAfter.text.slice(0, 80));
			const listedC = await api(url, '/list-sessions', { method: 'GET', jar: devA2.jar });
			check('exactly the calling device session remains', Array.isArray(listedC.json) && listedC.json.length === 1, Array.isArray(listedC.json) ? listedC.json.length : listedC.text.slice(0, 120));

			// --- refresh: stale updatedAt rolls expiresAt forward (updateAge) ---
			// device A1's row was deleted by revoke-other-sessions, so the refresh
			// test signs in a FRESH device and drives it straight to SQL.
			const devA3 = await device(emailA, password, markerA);
			check('account A signs in again (fresh session for the refresh test)', devA3.res.status === 200 && devA3.jar.has(), devA3.res.status);
			const refreshToken = devA3.token;
			await db.query(
				`UPDATE auth."session" SET "updatedAt" = $2, "expiresAt" = $3 WHERE token = $1`,
				[refreshToken, new Date(Date.now() - 2 * 24 * 60 * 60 * 1000), new Date(Date.now() + 60 * 60 * 1000)]
			);
			const beforeRefresh = await db.query(`SELECT "expiresAt" FROM auth."session" WHERE token = $1`, [refreshToken]);
			const preExpiresMs = toEpochMs(beforeRefresh.rows[0]?.expiresAt);
			check('the session row is staged with a 1h expiry and a 2-day-old updatedAt', preExpiresMs !== null && Math.abs(preExpiresMs - (Date.now() + 3600_000)) < 60_000, preExpiresMs);

			const refreshed = await api(url, '/get-session', { method: 'GET', jar: devA3.jar });
			check('get-session still resolves the refresh-staged session', refreshed.res.status === 200 && refreshed.json !== null, refreshed.text.slice(0, 80));
			const afterRefresh = await db.query(`SELECT "expiresAt", "updatedAt" FROM auth."session" WHERE token = $1`, [refreshToken]);
			const postExpiresMs = toEpochMs(afterRefresh.rows[0]?.expiresAt);
			const postUpdatedMs = toEpochMs(afterRefresh.rows[0]?.updatedAt);
			check(
				`a stale session (updatedAt older than updateAge) has expiresAt rolled forward to now + 7d`,
				postExpiresMs !== null && Math.abs(postExpiresMs - (Date.now() + 7 * 24 * 3600_000)) < 300_000,
				postExpiresMs
			);
			check('the refresh write also rolled updatedAt forward', postUpdatedMs !== null && Date.now() - postUpdatedMs < 60_000, postUpdatedMs);

			// --- live JWKS / JWT shape (the Go trust boundary's input) ---
			const jwksRes = await fetch(url + '/api/auth/jwks');
			let jwksKeys = [];
			if (jwksRes.status === 200) {
				try {
					jwksKeys = (await jwksRes.json())?.keys ?? [];
				} catch {
					jwksKeys = [];
				}
			}
			check('GET /api/auth/jwks serves at least one key', jwksRes.status === 200 && jwksKeys.length >= 1, { status: jwksRes.status, count: jwksKeys.length });
			check(
				'every served JWKS key is ES256 / P-256 (Go parseJWK accepts EC, not OKP)',
				jwksKeys.every((k) => k.alg === 'ES256' && k.crv === 'P-256' && k.kty === 'EC'),
				jwksKeys.map((k) => ({ alg: k.alg, crv: k.crv, kty: k.kty }))
			);
			check('served keys have a kid and a use of sig', jwksKeys.every((k) => typeof k.kid === 'string' && k.kid.length > 0 && (k.use ?? 'sig') === 'sig'), jwksKeys.map((k) => k.kid));

			const minted = await api(url, '/token', { method: 'GET', jar: devA3.jar });
			const jwt = minted.json?.token;
			check('a live session can mint an access JWT', typeof jwt === 'string' && jwt.split('.').length === 3, minted.text.slice(0, 120));
			if (typeof jwt === 'string' && jwt.split('.').length === 3) {
				const header = JSON.parse(Buffer.from(jwt.split('.')[0], 'base64url').toString('utf8'));
				const claims = JSON.parse(Buffer.from(jwt.split('.')[1], 'base64url').toString('utf8'));
				check('access JWT alg is ES256 and it carries a kid', header.alg === 'ES256' && typeof header.kid === 'string' && header.kid.length > 0, header);
				check('the minted token kid matches a key ACTUALLY served by /jwks', jwksKeys.some((k) => k.kid === header.kid), { tokenKid: header.kid, served: jwksKeys.map((k) => k.kid) });
				check('access JWT TTL is the 1h bound (exp − iat)', claims.exp - claims.iat === TRUST.jwtExpirationSeconds, { iat: claims.iat, exp: claims.exp });
				check('access JWT iss/aud are the pinned trust-boundary values', claims.iss === TRUST.issuer && claims.aud === TRUST.audience, { iss: claims.iss, aud: claims.aud });
			}

			// --- password change DEFAULT (no revokeOtherSessions): other devices survive,
			//     old password stops working ---
			const devB2 = await device(emailB, password, markerB);
			check('account B signs in on a fresh device (for the password-change default)', devB2.res.status === 200 && devB2.jar.has(), devB2.res.status);

			const changedDefault = await rateLimited(() =>
				api(url, '/change-password', { body: { newPassword, currentPassword: password }, jar: devB2.jar })
			);
			check('change-password without revokeOtherSessions succeeds', changedDefault.res.status === 200, { status: changedDefault.res.status, body: changedDefault.text.slice(0, 120) });
			const deviceBAfterDefault = await api(url, '/get-session', { method: 'GET', jar: devB1.jar });
			check('the OTHER device session survives when revokeOtherSessions is not requested', deviceBAfterDefault.res.status === 200 && deviceBAfterDefault.json !== null, deviceBAfterDefault.text.slice(0, 80));

			await new Promise((r) => setTimeout(r, 11_000));
			const oldPasswordAfter = await rateLimited(() => api(url, '/sign-in/email', { body: { email: emailB, password } }));
			check('the OLD password stops working after the change (401)', oldPasswordAfter.res.status === 401, oldPasswordAfter.res.status);
			// Positive control for the line above: the NEW password must work, so a
			// 401 from a lockout or a rate limit cannot masquerade as "old password
			// dead".
			const newPasswordAfter = await device(emailB, newPassword, markerB);
			check('the NEW password works after the change', newPasswordAfter.res.status === 200 && newPasswordAfter.jar.has(), newPasswordAfter.res.status);

			// --- deactivation gap-characterization: disabled app_users row still signs in.
			//     The Go API (not the identity layer) is the enforcement point. ---
			const appUserCols = await db.query(
				`SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'app_users' AND column_name IN ('status','deleted_at','subject')`
			);
			check(
				'public.app_users exposes the deactivation columns the Go resolver filters on (status, deleted_at, subject)',
				appUserCols.rows.length === 3,
				appUserCols.rows.map((r) => r.column_name)
			);
			const prevStatus = await db.query(`SELECT status FROM public.app_users WHERE subject = $1`, [userIdA]);
			await db.query(`UPDATE public.app_users SET status = 'disabled' WHERE subject = $1`, [userIdA]);
			const devA4 = await device(emailA, password, markerE);
			check(
				'GAP CHARACTERIZATION: a deactivated account still obtains a Better Auth session (identity layer does not gate; the Go RBAC resolver does)',
				devA4.res.status === 200 && devA4.jar.has() && typeof devA4.token === 'string' && devA4.token.length > 0,
				{ status: devA4.res.status, body: devA4.text.slice(0, 120) }
			);
			const deactivatedSession = await api(url, '/get-session', { method: 'GET', jar: devA4.jar });
			check(
				'GAP CHARACTERIZATION: that session resolves to the deactivated user at the identity layer',
				deactivatedSession.res.status === 200 && deactivatedSession.json?.user?.id === userIdA,
				{ status: deactivatedSession.res.status, userId: deactivatedSession.json?.user?.id ?? null }
			);
			await db.query(`UPDATE public.app_users SET status = $2 WHERE subject = $1`, [userIdA, prevStatus.rows[0]?.status ?? 'active']);

			// --- expiration: past expiresAt ⇒ get-session null / /token 401.
			//     Reuses device A4 (the deactivation device): one sign-in serves the
			//     identity-gap characterization, then the clock test. ---
			const expireToken = devA4.token;
			// Live BEFORE the clock moves: otherwise "expired ⇒ null" would also be
			// satisfied by a session that never worked at all.
			const beforeExpiry = await api(url, '/get-session', { method: 'GET', jar: devA4.jar });
			check('the expiration device is live before the clock is moved', beforeExpiry.res.status === 200 && beforeExpiry.json !== null, beforeExpiry.text.slice(0, 80));
			await db.query(`UPDATE auth."session" SET "expiresAt" = $2 WHERE token = $1`, [expireToken, new Date(Date.now() - 1000)]);

			const expiredGet = await api(url, '/get-session', { method: 'GET', jar: devA4.jar });
			check('an expired session answers get-session with 200 null (never a 5xx)', expiredGet.res.status === 200 && expiredGet.json === null, { status: expiredGet.res.status, body: expiredGet.text.slice(0, 80) });
			const expiredMint = await api(url, '/token', { method: 'GET', jar: devA4.jar });
			check('the expired session can no longer mint an access JWT (401)', expiredMint.res.status === 401, { status: expiredMint.res.status, body: expiredMint.text.slice(0, 80) });

			// --- revocation: a caller ends ONE of its OWN sessions. Reuses device A3:
			//     its session has already minted the JWT above, so revoking it now
			//     proves a previously-valid session dies. ---
			const selfRevoke = await api(url, '/revoke-session', { body: { token: devA3.token }, jar: devA3.jar });
			check('revoking your OWN session token succeeds', selfRevoke.res.status === 200 && selfRevoke.json?.status === true, selfRevoke.text.slice(0, 120));
			const selfRevoked = await api(url, '/get-session', { method: 'GET', jar: devA3.jar });
			check('the self-revoked session is dead for its own cookie', selfRevoked.res.status === 200 && selfRevoked.json === null, selfRevoked.text.slice(0, 80));
			const selfRevokedRow = await db.query(`SELECT count(*)::int AS n FROM auth."session" WHERE token = $1`, [devA3.token]);
			check('the self-revoked row is deleted from auth.session', selfRevokedRow.rows[0].n === 0, selfRevokedRow.rows[0].n);

			// --- logout: sign-out ends THIS session; the old cookie is not a key.
			//     Reuses device A2, signed in since the revoke-other-sessions case —
			//     which also proves a long-lived device stays valid until it signs
			//     out. ---
			const stillLive = await api(url, '/get-session', { method: 'GET', jar: devA2.jar });
			check('device A2 is still live before sign-out', stillLive.res.status === 200 && stillLive.json !== null, stillLive.text.slice(0, 80));
			const signedOutCookie = devA2.jar.value();
			const signedOut = await api(url, '/sign-out', { body: {}, jar: devA2.jar });
			check('sign-out succeeds', signedOut.res.status === 200, { status: signedOut.res.status, body: signedOut.text.slice(0, 80) });
			// Replay the SIGNED cookie the server issued before sign-out (a fresh
			// empty jar would prove nothing).
			const afterSignOut = await api(url, '/get-session', { method: 'GET', cookie: `${cookieName}=${signedOutCookie}` });
			check('the signed-out session answers get-session with 200 null (cookie replay rejected)', afterSignOut.res.status === 200 && afterSignOut.json === null, { status: afterSignOut.res.status, body: afterSignOut.text.slice(0, 80) });
			const signOutMint = await api(url, '/token', { method: 'GET', cookie: `${cookieName}=${signedOutCookie}` });
			check('the signed-out session can no longer mint an access JWT (401)', signOutMint.res.status === 401, { status: signOutMint.res.status, body: signOutMint.text.slice(0, 80) });
			const signedOutRow = await db.query(`SELECT count(*)::int AS n FROM auth."session" WHERE token = $1`, [devA2.token]);
			check('sign-out deleted the session row', signedOutRow.rows[0].n === 0, signedOutRow.rows[0].n);
		} finally {
			// Self-contained cleanup: remove every row this run created, in
			// FK-safe order. `app_users` has no FK to `auth.user`; `auth.session` /
			// `auth.account` / `auth.twoFactor` cascade from it.
			for (const id of createdUserIds) {
				await db.query(`DELETE FROM public.app_users WHERE subject = $1`, [id]).catch(() => {});
				await db.query(`DELETE FROM auth."session" WHERE "userId" = $1`, [id]).catch(() => {});
				await db.query(`DELETE FROM "auth"."user" WHERE id = $1`, [id]).catch(() => {});
			}
			if (createdUserIds.length > 0) {
				const residue = await db
					.query(
						`SELECT (SELECT count(*)::int FROM auth."user" WHERE id = ANY($1::text[])) AS users,
						        (SELECT count(*)::int FROM auth."session" WHERE "userId" = ANY($1::text[])) AS sessions,
						        (SELECT count(*)::int FROM public.app_users WHERE subject = ANY($1::text[])) AS app_users`,
						[createdUserIds]
					)
					.catch(() => null);
				check(
					'cleanup: no auth.user / auth.session / app_users rows remain for this run',
					residue !== null && residue.rows[0].users === 0 && residue.rows[0].sessions === 0 && residue.rows[0].app_users === 0,
					residue?.rows?.[0] ?? 'residue query failed'
				);
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
