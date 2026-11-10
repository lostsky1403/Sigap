// Auth form action regression tests — boots the production adapter-node build
// and exercises the REAL Better Auth (self-hosted `auth` schema) contract, with
// NO Supabase mock. Two boots drive the two environments that matter:
//
//   1. UNCONFIGURED — no BETTER_AUTH_SECRET and no Postgres URL. Asserts the
//      degraded "auth not configured" wiring: login/register pages render
//      authConfigured=false (the 'Layanan akun belum diaktifkan' notice), the
//      login action fails 503 (service unavailable, not a whole-site 500), and
//      logout still 303s.
//   2. DB-WIRED PLACEHOLDER — BETTER_AUTH_SECRET set and SIGAP_DATABASE_URL
//      pointed at a closed localhost port so `locals.auth` is truthy WITHOUT a
//      live Postgres. Asserts the form-validation short-circuit: empty /
//      too-short / mismatched credentials return 400 before any DB query.
//
// The successful sign-in cookie (`better-auth.session_token`) is intentionally
// NOT asserted here: producing it requires a live Postgres reachable at
// SIGAP_DATABASE_URL with the `auth` schema migrated. See the summary note /
// scripts/verify-better-auth-journey.mjs for the live-DB journey.
//
// Usage: node tests/auth-actions.test.js  (builds first if build/ is absent)

import assert from 'node:assert';
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');
const buildEntry = path.join(root, 'build', 'index.js');

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

// Build a child env with every auth-affecting var scrubbed, then the caller's
// overrides applied. This guarantees the UNCONFIGURED boot is genuinely absent
// of a DB/secret even if the parent process exported one.
const cleanEnv = (overrides) => {
	const env = { ...process.env };
	delete env.SIGAP_DATABASE_URL;
	delete env.DATABASE_URL;
	delete env.BETTER_AUTH_SECRET;
	delete env.BETTER_AUTH_URL;
	delete env.PUBLIC_SUPABASE_URL;
	delete env.PUBLIC_SUPABASE_ANON_KEY;
	return { ...env, ...overrides };
};

// Spawn the built server on a fresh port and block until /auth/login answers.
const startApp = async (overrides) => {
	const port = await freePort();
	const webUrl = `http://127.0.0.1:${port}`;
	const child = spawn(process.execPath, [buildEntry], {
		cwd: root,
		env: cleanEnv({
			PORT: String(port),
			HOST: '127.0.0.1',
			ORIGIN: webUrl,
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

const post = (webUrl, route, fields, cookies) =>
	fetch(webUrl + route, {
		method: 'POST',
		headers: {
			'content-type': 'application/x-www-form-urlencoded',
			// Emulate a real browser form submission: text/html accept puts
			// SvelteKit in form mode (redirect 303 / fail status + HTML) instead
			// of JSON action mode (HTTP 200 + envelope), matching production.
			accept: 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
			origin: webUrl,
			...(cookies ? { cookie: cookies } : {})
		},
		body: new URLSearchParams(fields).toString(),
		redirect: 'manual'
	});

let failed = 0;
const check = (name, ok, actual) => {
	if (ok) {
		console.log(`  PASS  ${name}`);
	} else {
		failed++;
		console.log(`  FAIL  ${name}  (actual: ${JSON.stringify(actual)})`);
	}
};

try {
	// ---- Phase 1: UNCONFIGURED (no Better Auth secret, no Postgres) ----
	{
		console.log('Phase 1: auth unconfigured (no BETTER_AUTH_SECRET / no DB)');
		const { child, webUrl } = await startApp({});

		for (const p of ['/auth/login', '/auth/register']) {
			const r = await fetch(webUrl + p);
			const body = await r.text();
			check(`GET ${p} -> 200`, r.status === 200, r.status);
			check(
				`GET ${p} renders authConfigured=false (unavailable notice)`,
				body.includes('data-testid="auth-unavailable"'),
				body.slice(0, 80)
			);
			check(
				`GET ${p} shows the "akun belum diaktifkan" copy (authConfigured===false path)`,
				body.includes('Layanan akun belum diaktifkan'),
				body.slice(0, 80)
			);
		}

		// The login action with a store absent must fail 503 (service
		// unavailable), never a 500 or an errant 404 (missing action).
		let r = await post(webUrl, '/auth/login', { email: 'a@b.c', password: 'whatever' });
		check('POST /auth/login unconfigured -> 503', r.status === 503, r.status);

		// Logout is store-independent: it must still 303 and land on /.
		r = await post(webUrl, '/auth/logout', {});
		check('POST /auth/logout -> 303', r.status === 303, r.status);
		check('logout redirects to /', r.headers.get('location') === '/', r.headers.get('location'));

		await stopApp(child);
	}

	// ---- Phase 2: DB-WIRED placeholder (validation short-circuit, no live DB) --
	{
		console.log('Phase 2: DB-wired placeholder (validation short-circuits; no live DB)');
		const { child, webUrl } = await startApp({
			BETTER_AUTH_SECRET: 'test-secret-not-used-to-sign-real-sessions',
			// Closed localhost port: `databaseConfigured()` is true so the action
			// body runs, but no real Postgres is reachable — the 400 assertions
			// below must return before any DB query would be issued.
			SIGAP_DATABASE_URL: 'postgres://sigap:test@127.0.0.1:59999/sigap'
		});

		// Auth is now "configured" (locals.auth truthy), so the page should NOT
		// render the unavailable notice.
		let r = await fetch(webUrl + '/auth/login');
		let body = await r.text();
		check(
			'GET /auth/login with DB-wired placeholder does NOT render unavailable notice',
			!body.includes('data-testid="auth-unavailable"'),
			body.slice(0, 80)
		);

		// Empty email/password: the action validates before signInEmail, so this
		// must 400 with no DB touch.
		r = await post(webUrl, '/auth/login', { email: '', password: '' });
		body = await r.text();
		check('POST /auth/login empty fields -> 400', r.status === 400, r.status);
		check('login missing-fields message present', body.includes('wajib diisi'), body.slice(0, 80));

		// Register validation ladder — none of these should reach signUpEmail.
		r = await post(webUrl, '/auth/register', { email: '', password: '', confirm: '' });
		body = await r.text();
		check('POST /auth/register empty fields -> 400', r.status === 400, r.status);
		check('register missing-fields message present', body.includes('wajib diisi'), body.slice(0, 80));

		r = await post(webUrl, '/auth/register', {
			email: 'new@example.com',
			password: 'short',
			confirm: 'short'
		});
		body = await r.text();
		check('POST /auth/register short password -> 400', r.status === 400, r.status);
		check('register min-length message present', body.includes('minimal 8 karakter'), body.slice(0, 80));

		r = await post(webUrl, '/auth/register', {
			email: 'new@example.com',
			password: 'valid-password',
			confirm: 'different-password'
		});
		body = await r.text();
		check('POST /auth/register password mismatch -> 400', r.status === 400, r.status);
		check('register mismatch message present', body.includes('tidak sama'), body.slice(0, 80));

		// Logout still 303s (signOut on a sessionless request is swallowed; the
		// redirect is unconditional).
		r = await post(webUrl, '/auth/logout', {});
		check('POST /auth/logout (DB-wired) -> 303', r.status === 303, r.status);

		await stopApp(child);
	}

	if (failed === 0) {
		console.log('✅ Auth form action regression tests passed (Better Auth contract)');
	} else {
		console.log(`❌ ${failed} auth action check(s) failed`);
	}
} catch (err) {
	failed++;
	console.error('❌ Auth action test failure:', err.message);
}

process.exit(failed === 0 ? 0 : 1);