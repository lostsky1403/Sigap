// Auth form action regression tests — boots the production adapter-node build
// against a local mock Supabase Auth API and exercises the real form actions
// (login/register/logout) so a missing default action fails here, not in prod.
// Usage: node tests/auth-actions.test.js  (builds first if build/ is absent)

import assert from 'node:assert';
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
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

const userPayload = {
	id: 'test-user-id',
	aud: 'authenticated',
	role: 'authenticated',
	email: 'test@example.com',
	email_confirmed_at: '2026-01-01T00:00:00Z',
	phone: '',
	confirmed_at: '2026-01-01T00:00:00Z',
	last_sign_in_at: '2026-01-01T00:00:00Z',
	app_metadata: { provider: 'email', providers: ['email'] },
	user_metadata: {},
	identities: [],
	created_at: '2026-01-01T00:00:00Z',
	updated_at: '2026-01-01T00:00:00Z'
};

const sessionPayload = {
	access_token: 'test-access-token',
	token_type: 'bearer',
	expires_in: 3600,
	expires_at: 4102444800,
	refresh_token: 'test-refresh-token',
	user: userPayload
};

const state = { tokenMode: 'ok' };

const mockSupabase = (req, res) => {
	let body = '';
	req.on('data', (chunk) => (body += chunk));
	req.on('end', () => {
		const url = new URL(req.url, 'http://127.0.0.1');
		const send = (status, payload) => {
			res.writeHead(status, { 'content-type': 'application/json' });
			res.end(payload === undefined ? '' : JSON.stringify(payload));
		};
		if (req.method === 'POST' && url.pathname === '/auth/v1/token') {
			if (state.tokenMode === 'fail') {
				send(400, { error: 'invalid_grant', error_description: 'Invalid credentials' });
			} else {
				send(200, sessionPayload);
			}
			return;
		}
		if (req.method === 'POST' && url.pathname === '/auth/v1/signup') {
			send(200, sessionPayload);
			return;
		}
		if (req.method === 'POST' && url.pathname === '/auth/v1/logout') {
			send(204);
			return;
		}
		send(404, { error: 'not found' });
	});
};

const [mockPort, webPort] = [await freePort(), await freePort()];
const mockUrl = `http://127.0.0.1:${mockPort}`;
const webUrl = `http://127.0.0.1:${webPort}`;

const mock = http.createServer(mockSupabase);
await new Promise((resolve, reject) => {
	mock.listen(mockPort, '127.0.0.1', resolve);
	mock.on('error', reject);
});

const child = spawn(process.execPath, [buildEntry], {
	cwd: root,
	env: {
		...process.env,
		PORT: String(webPort),
		HOST: '127.0.0.1',
		ORIGIN: webUrl,
		PUBLIC_SUPABASE_URL: mockUrl,
		PUBLIC_SUPABASE_ANON_KEY: 'test-anon-key'
	},
	stdio: ['ignore', 'ignore', 'inherit']
});

const post = (route, fields, cookies) =>
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
	let ready = false;
	for (let i = 0; i < 60; i++) {
		try {
			const r = await fetch(webUrl + '/auth/login');
			if (r.status === 200) {
				ready = true;
				break;
			}
		} catch {
			// not up yet
		}
		await new Promise((r) => setTimeout(r, 500));
	}
	assert(ready, 'web server should become ready');

	for (const p of ['/auth/login', '/auth/register']) {
		const r = await fetch(webUrl + p);
		check(`GET ${p} -> 200`, r.status === 200, r.status);
	}

	state.tokenMode = 'ok';
	let r = await post('/auth/login', { email: '', password: '' });
	let body = await r.text();
	check('POST /auth/login missing fields -> 400 (action reached, not 404)', r.status === 400, r.status);
	check('missing-fields message present', body.includes('wajib diisi'), body.slice(0, 80));

	state.tokenMode = 'fail';
	r = await post('/auth/login', { email: 'test@example.com', password: 'wrong-password' });
	body = await r.text();
	check('POST /auth/login bad credentials -> 401 (not 404)', r.status === 401, r.status);
	check('auth error message present', body.includes('Email atau kata sandi salah.'), body.slice(0, 80));

	state.tokenMode = 'ok';
	r = await post('/auth/login', { email: 'test@example.com', password: 'correct-password' });
	check('POST /auth/login success -> 303 redirect', r.status === 303, r.status);
	check('login redirects to /', r.headers.get('location') === '/', r.headers.get('location'));
	const setCookies = typeof r.headers.getSetCookie === 'function' ? r.headers.getSetCookie() : [];
	const authCookie = setCookies.find((c) => c.split(';')[0].includes('-auth-token'));
	check('Supabase session cookie set on login', Boolean(authCookie), setCookies.length);
	const cookieHeader = setCookies.map((c) => c.split(';')[0]).join('; ');

	r = await post('/auth/register', {
		email: 'new@example.com',
		password: 'valid-password',
		confirm: 'valid-password'
	});
	check('POST /auth/register -> 303 (action reachable, not 404)', r.status === 303, r.status);
	check(
		'register redirects to /auth/login?registered=1',
		r.headers.get('location') === '/auth/login?registered=1',
		r.headers.get('location')
	);

	r = await post('/auth/register', {
		email: 'new@example.com',
		password: 'valid-password',
		confirm: 'different-password'
	});
	check('POST /auth/register password mismatch -> 400 (action body ran)', r.status === 400, r.status);

	r = await post('/auth/logout', {}, cookieHeader);
	check('POST /auth/logout -> 303 (action reachable, not 404)', r.status === 303, r.status);
	check('logout redirects to /', r.headers.get('location') === '/', r.headers.get('location'));

	if (failed === 0) {
		console.log('✅ Auth form action regression tests passed');
	} else {
		console.log(`❌ ${failed} auth action check(s) failed`);
	}
} catch (err) {
	failed++;
	console.error('❌ Auth action test failure:', err.message);
} finally {
	child.kill();
	mock.close();
}

process.exit(failed === 0 ? 0 : 1);
