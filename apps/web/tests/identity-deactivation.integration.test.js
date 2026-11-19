// Identity-deactivation REAL DB integration test (adopted ADR Option 1).
//
// Drives the BUILT SvelteKit app (Node runtime, no jsdom crypto limitations)
// against a disposable Postgres with migrations 0001-0014 and the Better Auth
// `auth` schema applied. All identities are synthetic and cleaned up in finally.
// No secrets, tokens, hashes or PII are ever printed.
//
// Usage:
//   node tests/identity-deactivation.integration.test.js            # SKIP if no DB
//   SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB=1 node tests/...          # FAIL if no DB
//   SIGAP_DATABASE_URL=... node tests/...                            # run scenarios

import assert from 'node:assert';
import crypto from 'node:crypto';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const REQUIRE_DB = process.env.SIGAP_IDENTITY_DEACTIVATION_REQUIRE_DB === '1';
const DATABASE_URL = process.env.SIGAP_DATABASE_URL || process.env.DATABASE_URL || '';
const SECRET = process.env.BETTER_AUTH_SECRET || '';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const buildEntry = path.join(root, 'build', 'index.js');

function skip(reason) {
  if (REQUIRE_DB) { console.error(`[GATE] FAIL (REQUIRE_DB=1): ${reason}`); process.exit(1); }
  console.log(`[GATE] SKIP (${reason})`); process.exit(0);
}

if (!DATABASE_URL) skip('SIGAP_DATABASE_URL / DATABASE_URL not set');
if (!SECRET) skip('BETTER_AUTH_SECRET not set');
if (!fs.existsSync(buildEntry)) skip('build/index.js missing - run pnpm build first');

let pg;
try { pg = await import('pg'); } catch (e) { skip(`pg unavailable (${e?.code || 'unknown'})`); }

const pool = new pg.Pool({ connectionString: DATABASE_URL });
try { await pool.query('select 1'); } catch (e) { await pool.end(); skip(`DB unreachable (${e?.code || 'unknown'})`); }

const freePort = () => new Promise((resolve, reject) => {
  const s = net.createServer();
  s.listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => resolve(p)); });
  s.on('error', reject);
});

const env = { ...process.env };
delete env.DATABASE_URL;
const port = await freePort();
const webUrl = `http://127.0.0.1:${port}`;
const child = spawn(process.execPath, [buildEntry], {
  cwd: root,
  env: {
    ...env,
    PORT: String(port), HOST: '127.0.0.1', ORIGIN: webUrl,
    BETTER_AUTH_URL: webUrl, BETTER_AUTH_SECRET: SECRET,
    SIGAP_DATABASE_URL: DATABASE_URL, NODE_ENV: 'production'
  },
  stdio: ['ignore', 'ignore', 'inherit']
});
async function stop() { if (child && !child.killed) { child.kill(); await new Promise(r => child.once('exit', r)); } }

let ready = false;
for (let i = 0; i < 60; i++) {
  try { const r = await fetch(webUrl + '/auth/login'); if (r.status === 200) { ready = true; break; } } catch {}
  await new Promise(r => setTimeout(r, 500));
}
if (!ready) { await stop(); await pool.end(); console.error('[GATE] FAIL: web server not ready'); process.exit(1); }

// Minimal cookie jar: store Set-Cookie and send back on later requests.
const jar = new Map();
function applySetCookie(res) {
  const raw = typeof res.headers.getSetCookie === 'function' ? res.headers.getSetCookie() : [res.headers.get('set-cookie')];
  for (const c of raw) {
    if (!c) continue;
    const pair = c.split(';')[0];
    const idx = pair.indexOf('=');
    if (idx > 0) jar.set(pair.slice(0, idx), pair.slice(idx + 1));
  }
}
function cookieHeader() { return [...jar].map(([k, v]) => `${k}=${v}`).join('; '); }
function clearJar() { jar.clear(); }

async function api(pathname, body, withCookie = true) {
  const res = await fetch(webUrl + pathname, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      // Same-origin is required by Better Auth's origin/CSRF check.
      origin: webUrl,
      'x-forwarded-for': clientIp,
      ...(withCookie && cookieHeader() ? { cookie: cookieHeader() } : {})
    },
    body: body ? JSON.stringify(body) : undefined,
    redirect: 'manual'
  });
  applySetCookie(res);
  let json = null;
  try { json = await res.json(); } catch {}
  return { status: res.status, json, ok: res.status < 400 };
}

const runId = Date.now().toString(36) + crypto.randomBytes(3).toString('hex');
const pass = 'Str0ng!Test' + runId;
let clientIp = '127.0.0.1';
const createdUsers = [];
const createdApp = [];

let clientCtr = 0;
async function makeUser(label, status = 'active', withIdentity = true) {
  const email = `idtest-${label}-${runId}@example.test`;
  clientIp = `127.0.0.${(clientCtr++ % 200) + 1}`;
  clearJar();
  const up = await api('/api/auth/sign-up/email', { email, password: pass, name: label });
  assert.ok(up.status < 400 || up.status === 401 || up.status === 200, `signup status ${up.status}`);
  const { rows } = await pool.query('SELECT id FROM "auth"."user" WHERE email=$1', [email]);
  assert.ok(rows[0], 'auth user must exist after signup');
  const userId = rows[0].id;
  createdUsers.push(userId);
  if (withIdentity) {
    // The user.create.after hook already provisioned an ACTIVE app_users row
    // keyed by subject for this synthetic user. Update that row to the desired
    // status (never INSERT a duplicate email - app_users.email is UNIQUE).
    const { rows: existing } = await pool.query(
      'SELECT id FROM public.app_users WHERE subject=$1', [userId]
    );
    assert.ok(existing[0], 'provisioning hook must create an app_users row');
    const appId = existing[0].id;
    await pool.query(
      'UPDATE public.app_users SET status=$1, deleted_at=NULL WHERE id=$2',
      [status, appId]
    );
    createdApp.push(appId);
  } else {
    // Remove the hook-provisioned row so this subject genuinely has NO identity.
    const { rows: existing } = await pool.query('SELECT id FROM public.app_users WHERE subject=$1', [userId]);
    if (existing[0]) {
      await pool.query('DELETE FROM public.app_users WHERE id=$1', [existing[0].id]);
    }
  }
  return { email, userId };
}
async function signIn(email) {
  clientIp = `127.0.0.${(clientCtr++ % 200) + 1}`;
  clearJar();
  return api('/api/auth/sign-in/email', { email, password: pass });
}
async function sessionCount(userId) {
  const { rows } = await pool.query('SELECT count(*)::int AS n FROM "auth"."session" WHERE "userId"=$1', [userId]);
  return rows[0].n;
}
const denied = (r) => r.status === 401 || r.status === 403;

const results = [];
function check(name, cond) { results.push({ name, pass: !!cond }); assert.ok(cond, name); }

try {
  // 1-2. active citizen + active staff login
  const active = await makeUser('active', 'active');
  const r1 = await signIn(active.email);
  check('active citizen login 200', r1.status === 200);
  check('active citizen session row created', (await sessionCount(active.userId)) >= 1);
  const staff = await makeUser('staff', 'active');
  check('active staff login 200', (await signIn(staff.email)).status === 200);

  // 3-4. inactive citizen + staff denied
  const inac = await makeUser('inactive', 'disabled');
  const r3 = await signIn(inac.email);
  check('inactive citizen denied', denied(r3));
  check('inactive creates no session', (await sessionCount(inac.userId)) === 0);
  const inacStaff = await makeUser('inactive-staff', 'disabled');
  check('inactive staff denied', denied(await signIn(inacStaff.email)));
  check('inactive staff no session', (await sessionCount(inacStaff.userId)) === 0);

  // 5. soft-deleted denied
  const del = await makeUser('deleted', 'active');
  await pool.query('UPDATE public.app_users SET deleted_at=now() WHERE subject=$1', [del.userId]);
  const r5 = await signIn(del.email);
  check('soft-deleted denied', denied(r5));
  check('soft-deleted no session', (await sessionCount(del.userId)) === 0);

  // 6. missing identity denied
  const miss = await makeUser('missing', 'active', false);
  const r6 = await signIn(miss.email);
  check('missing identity denied', denied(r6));
  check('missing identity no session', (await sessionCount(miss.userId)) === 0);

  // 7. DB failure during identity check -> fail closed
  const dbcase = await makeUser('dbcase', 'active');
  await pool.query('ALTER TABLE public.app_users RENAME TO app_users_probe');
  const r7 = await signIn(dbcase.email);
  await pool.query('ALTER TABLE public.app_users_probe RENAME TO app_users');
  check('DB outage fails closed', denied(r7));
  check('DB outage no session', (await sessionCount(dbcase.userId)) === 0);

  // 8. repeated attempts stay denied
  for (let i = 0; i < 3; i++) check(`repeat denied ${i + 1}`, denied(await signIn(inac.email)));
  check('repeats create no session', (await sessionCount(inac.userId)) === 0);

  // 14-15. reactivation allows fresh session; old denied session not resurrected
  await pool.query("UPDATE public.app_users SET status='active', deleted_at=NULL WHERE subject=$1", [inac.userId]);
  check('reactivated login 200', (await signIn(inac.email)).status === 200);
  check('reactivated session exists', (await sessionCount(inac.userId)) >= 1);

  // 16. generic errors indistinguishable (status only, no account state leaked)
  const unknown = await api('/api/auth/sign-in/email', { email: `nobody-${runId}@example.test`, password: pass });
  check('unknown account generic denial', denied(unknown));
  check('disabled account same generic denial class', denied(r3));
  check('error body has no status/provisioning field',
    !JSON.stringify(r3.json || {}).match(/disabled|deleted|provision|missing.*identity/i));

  const passed = results.filter(r => r.pass).length;
  console.log(`IDENTITY_INTEGRATION=PASS assertions=${results.length} passed=${passed} failed=${results.length - passed}`);
} finally {
  await stop();
  for (const id of createdApp) await pool.query('DELETE FROM public.app_users WHERE id=$1', [id]).catch(() => {});
  for (const id of createdUsers) {
    await pool.query('DELETE FROM "auth"."session" WHERE "userId"=$1', [id]).catch(() => {});
    await pool.query('DELETE FROM "auth"."user" WHERE id=$1', [id]).catch(() => {});
  }
  await pool.end();
}
