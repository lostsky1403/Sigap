// Task-owned: REAL MFA/TOTP session-creation acceptance against disposable DB.
// Built SvelteKit/Better Auth runtime over HTTP; no mocks. No secrets/PII printed.
import { spawn } from 'node:child_process';
import crypto from 'node:crypto';
import net from 'node:net';
import pg from 'pg';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const DB = process.env.SIGAP_DATABASE_URL;
const SECRET = process.env.BETTER_AUTH_SECRET;
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const buildEntry = path.join(root, 'build', 'index.js');
const stripped = DB.replace(/[?&]sslmode=[^&]+(&?)/, (_m, t) => (t ? '' : ''));
const pool = new pg.Pool({ connectionString: stripped, ssl: { rejectUnauthorized: false } });

const port = await new Promise((res) => { const s=net.createServer(); s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>res(p));}); });
const url = `http://127.0.0.1:${port}`;
const child = spawn(process.execPath, [buildEntry], { cwd: root, env: { ...process.env, PORT:String(port), HOST:'127.0.0.1', ORIGIN:url, BETTER_AUTH_URL:url, NODE_ENV:'production' }, stdio:['ignore','ignore','inherit'] });
const stop = async () => { if (child && !child.killed) { child.kill(); await new Promise(r=>child.once('exit',r)); } };
for (let i=0;i<60;i++){ try{ const r=await fetch(url+'/auth/login'); if(r.status===200) break; }catch{} await new Promise(r=>setTimeout(r,500)); }

// ---- cookie jar + HTTP helper ----
function makeJar(){ const m=new Map(); return {
  apply(res){ const raw = typeof res.headers.getSetCookie==='function'?res.headers.getSetCookie():[res.headers.get('set-cookie')]; for(const c of raw){ if(!c)continue; const p=c.split(';')[0], i=p.indexOf('='); if(i>0)m.set(p.slice(0,i),p.slice(i+1)); } },
  header(){ return [...m].map(([k,v])=>`${k}=${v}`).join('; '); }, clear(){ m.clear(); }
};}
let clientIp = '127.0.0.9';
let ipCtr=0;
async function api(p, body, jar, method='POST'){
  clientIp = `127.0.0.${(ipCtr++ % 200) + 1}`;
  const h={ 'content-type':'application/json', origin:url, 'x-forwarded-for':clientIp };
  if(jar && jar.header()) h.cookie=jar.header();
  const r=await fetch(url+p,{method,headers:h,body:body?JSON.stringify(body):undefined,redirect:'manual'});
  if(jar) jar.apply(r);
  let json=null; try{json=await r.json();}catch{}
  return {status:r.status,json,text:JSON.stringify(json)};
}

// ---- TOTP (RFC 6238, HMAC-SHA1, 6 digits, 30s) ----
function b32decode(s){ s=s.replace(/=+$/,'').toUpperCase(); const out=[]; let bits=0,val=0; const A='ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'; for(const ch of s){ const i=A.indexOf(ch); if(i<0)continue; val=(val<<5)|i; bits+=5; if(bits>=8){ out.push((val>>>(bits-8))&255); bits-=8; } } return Buffer.from(out); }
function totp(secretB32){ const key=b32decode(secretB32); const c=Math.floor(Date.now()/30000); const buf=Buffer.alloc(8); buf.writeBigUInt64BE(BigInt(c)); const h=crypto.createHmac('sha1',key).update(buf).digest(); const o=h[h.length-1]&15; const code=(h.readUInt32BE(o)&0x7fffffff)%1000000; return String(code).padStart(6,'0'); }

async function sessions(uid){ const {rows}=await pool.query('SELECT count(*)::int AS n FROM "auth"."session" WHERE "userId"=$1',[uid]); return rows[0].n; }
async function setStatus(uid,status,deleted=false){ await pool.query('UPDATE public.app_users SET status=$1, deleted_at=$2 WHERE subject=$3',[status,deleted?new Date():null,uid]); }

const runId=Date.now().toString(36)+crypto.randomBytes(3).toString('hex');
const pass='Mfa!Strong'+runId;
const results=[]; const check=(n,c)=>{results.push({n,ok:!!c}); if(!c) console.error('FAIL',n);};
const created=[];

try {
  await pool.query('select 1');
  const email=`mfatest-${runId}@example.test`;
  const jar=makeJar();
  let up=await api('/api/auth/sign-up/email',{email,password:pass,name:'mfa-staff'},jar);
  const {rows}=await pool.query('SELECT id FROM "auth"."user" WHERE email=$1',[email]); const uid=rows[0].id; created.push(uid);
  check('signup ok',up.status<400);
  // sign-up does NOT auto-sign-in (autoSignIn:false): establish a session first.
  const initialSignIn = await api('/api/auth/sign-in/email',{email,password:pass},jar);
  check('initial sign-in ok',initialSignIn.status===200);
  // enroll TOTP
  const en=await api('/api/auth/two-factor/enable',{method:'totp',password:pass},jar);
  check('two-factor enable 200',en.status===200);
  const uri=en.json?.totpURI||'';
  const secretMatch=uri.match(/[?&]secret=([A-Z2-7]+)/i);
  check('totpURI has base32 secret',!!secretMatch);
  const secret=secretMatch?.[1]||'';
  const code=totp(secret);
  const vf=await api('/api/auth/two-factor/verify-totp',{code},jar);
  check('verify-totp ok',vf.status===200);
  const {rows:tf}=await pool.query('SELECT "twoFactorEnabled","verified" FROM "auth"."user" u LEFT JOIN "auth"."twoFactor" t ON t."userId"=u.id WHERE u.id=$1',[uid]);
  check('TOTP enrolled',tf[0]?.twoFactorEnabled===true);

  // A: full MFA sign-in after sign-out
  await api('/api/auth/sign-out',{},jar);
  jar.clear();
  const si=await api('/api/auth/sign-in/email',{email,password:pass},jar);
  check('MFA sign-in returns twoFactorRedirect (no session yet)',si.json?.twoFactorRedirect===true);
  // consume two-factor cookie set by sign-in; verify with fresh code
  const code2=totp(secret);
  const vf2=await api('/api/auth/two-factor/verify-totp',{code:code2},jar);
  check('MFA verify creates session',vf2.status===200);
  check('MFA session row exists',(await sessions(uid))>=1);

  // C: deactivation revokes the MFA session
  const {rows:au}=await pool.query('SELECT id FROM public.app_users WHERE subject=$1',[uid]);
  const deact=spawn(process.execPath,['scripts/provision-staff.mjs','--deactivate',email,'--apply'],{cwd:root,env:process.env,stdio:['ignore','pipe','pipe']});
  let dout=''; deact.stdout.on('data',c=>dout+=c); deact.stderr.on('data',c=>dout+=c);
  const de=await new Promise(r=>deact.on('close',r));
  check('deactivate exit 0',de===0);
  check('MFA session revoked',(await sessions(uid))===0);
  const {rows:st}=await pool.query('SELECT status FROM public.app_users WHERE id=$1',[au[0].id]);
  check('status disabled',st[0]?.status==='disabled');

  // B: disabled before TOTP verification cannot mint session
  jar.clear();
  const si2=await api('/api/auth/sign-in/email',{email,password:pass},jar);
  check('disabled MFA sign-in does not succeed',si2.status!==200);
  const code3=totp(secret);
  const vf3=await api('/api/auth/two-factor/verify-totp',{code:code3},jar);
  check('disabled TOTP verify blocked',vf3.status>=400);
  check('no session after blocked verify',(await sessions(uid))===0);

  // D: reactivation -> new MFA login allowed, old session not resurrected
  await setStatus(uid,'active');
  jar.clear();
  const si3=await api('/api/auth/sign-in/email',{email,password:pass},jar);
  check('reactivated MFA sign-in redirects',si3.json?.twoFactorRedirect===true);
  const vf4=await api('/api/auth/two-factor/verify-totp',{code:totp(secret)},jar);
  check('reactivated MFA verify ok',vf4.status===200);
  check('fresh MFA session exists',(await sessions(uid))===1);

  const passed=results.filter(r=>r.ok).length;
  console.log(`MFA_RUNTIME=${passed===results.length?'PASS':'FAIL'} assertions=${results.length} passed=${passed} failed=${results.length-passed}`);
} finally {
  await stop();
  for(const id of created){ await pool.query('DELETE FROM "auth"."twoFactor" WHERE "userId"=$1',[id]).catch(()=>{}); await pool.query('DELETE FROM "auth"."session" WHERE "userId"=$1',[id]).catch(()=>{}); await pool.query('DELETE FROM "auth"."user" WHERE id=$1',[id]).catch(()=>{}); }
  await pool.end();
}
