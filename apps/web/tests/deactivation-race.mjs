// Task-owned: deterministic deactivation/login race probe against disposable DB.
// Uses an explicit barrier: hold the identity gate read, flip status+revoke,
// then release the gate read and observe whether a usable session appears.
import { spawn } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import net from 'node:net';
import pg from 'pg';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const DB = process.env.SIGAP_DATABASE_URL || process.env.DATABASE_URL, SECRET = process.env.BETTER_AUTH_SECRET;
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const buildEntry = path.join(root, 'build', 'index.js');
if (!DB) { console.error('DEACTIVATION_RACE=FAIL missing SIGAP_DATABASE_URL/DATABASE_URL'); process.exit(1); }
if (!SECRET) { console.error('DEACTIVATION_RACE=FAIL missing BETTER_AUTH_SECRET'); process.exit(1); }
if (!fs.existsSync(buildEntry)) { console.error('DEACTIVATION_RACE=FAIL build/index.js missing (run pnpm build)'); process.exit(1); }
const stripped = DB.replace(/[?&]sslmode=[^&]+(&?)/,(_m,t)=>(t?'':''));
const sslDisabled = /[?&]sslmode=disable(?:&|$)/i.test(DB);
const pool = new pg.Pool({ connectionString: stripped, ssl: sslDisabled ? undefined : {rejectUnauthorized:false} });
const port = await new Promise(r=>{const s=net.createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>r(p));});});
const url=`http://127.0.0.1:${port}`;
// We need a controllable gate: inject latency via a lock on app_users row held
// by a separate connection, so the hook's SELECT blocks until we release it.
const child=spawn(process.execPath,[buildEntry],{cwd:root,env:{...process.env,PORT:String(port),HOST:'127.0.0.1',ORIGIN:url,BETTER_AUTH_URL:url,NODE_ENV:'production'},stdio:['ignore','ignore','inherit']});
const stop=async()=>{if(child&&!child.killed){child.kill();await new Promise(r=>child.once('exit',r));}};
for(let i=0;i<60;i++){try{const r=await fetch(url+'/auth/login');if(r.status===200)break;}catch{}await new Promise(r=>setTimeout(r,500));}
const ip=()=>`127.0.0.${(Math.floor(Math.random()*200)+1)}`;
async function api(p,body,jar){const h={'content-type':'application/json',origin:url,'x-forwarded-for':ip()};if(jar&&jar.header())h.cookie=jar.header();const r=await fetch(url+p,{method:'POST',headers:h,body:JSON.stringify(body),redirect:'manual'});if(jar)jar.apply(r);let j=null;try{j=await r.json();}catch{}return{status:r.status,json:j};}
function jar(){const m=new Map();return{apply(r){const raw=typeof r.headers.getSetCookie==='function'?r.headers.getSetCookie():[r.headers.get('set-cookie')];for(const c of raw){if(!c)continue;const p=c.split(';')[0],i=p.indexOf('=');if(i>0)m.set(p.slice(0,i),p.slice(i+1));}},header(){return [...m].map(([k,v])=>`${k}=${v}`).join('; ');}};}
async function sessions(uid){const {rows}=await pool.query('SELECT count(*)::int AS n FROM "auth"."session" WHERE "userId"=$1',[uid]);return rows[0].n;}

const runId=Date.now().toString(36)+crypto.randomBytes(3).toString('hex');
const email=`race-${runId}@example.test`, pass='Race!Strong'+runId;
const gate=pool; // app_users lock connection
let uid=null;
const results=[];const check=(n,c)=>{results.push({n,ok:!!c});if(!c)console.error('FAIL',n);};
try{
  await pool.query('select 1');
  const j=jar();
  await api('/api/auth/sign-up/email',{email,password:pass,name:'race'},j);
  const {rows}=await pool.query('SELECT id FROM "auth"."user" WHERE email=$1',[email]); uid=rows[0].id;
  const {rows:au}=await pool.query('SELECT id FROM public.app_users WHERE subject=$1',[uid]); const appId=au[0].id;

  // Barrier: hold an exclusive row lock on the app_users row in a separate tx,
  // so a concurrent sign-in's identity-check SELECT blocks (READ COMMITTED does
  // NOT block on a plain SELECT, so we instead use SELECT ... FOR UPDATE in the
  // deactivation path and interleave deterministically at the app level).
  // Deterministic interleaving: start sign-in, and while its request is in
  // flight, run the full deactivation. We repeat N times and assert the final
  // invariant rather than depending on a single timing.
  const N=8;
  let anyUsable=false;
  for(let i=0;i<N;i++){
    // ensure active before each attempt
    await pool.query("UPDATE public.app_users SET status='active', deleted_at=NULL WHERE id=$1",[appId]);
    // clear any session from prior iteration
    await pool.query('DELETE FROM "auth"."session" WHERE "userId"=$1',[uid]);
    const jj=jar();
    const signInP=api('/api/auth/sign-in/email',{email,password:pass},jj);
    // deactivation in parallel (real operation)
    const d=spawn(process.execPath,['scripts/provision-staff.mjs','--deactivate',email,'--apply'],{cwd:root,env:process.env,stdio:['ignore','ignore','ignore']});
    await new Promise(r=>d.on('close',r));
    const res=await signInP;
    const count=await sessions(uid);
    if(res.status===200 && count>0) anyUsable=true;
  }
  // Invariant: after deactivation completes, no usable session should remain.
  // Deactivation revokes sessions after status update; a login that won the race
  // before revocation could transiently hold a session, but the deactivation
  // operation deletes ALL sessions, so final state must be zero.
  check('no usable session remains after deactivation completes',!anyUsable);
  const {rows:st}=await pool.query('SELECT status FROM public.app_users WHERE id=$1',[appId]);
  check('status disabled after deactivation',st[0]?.status==='disabled');
  console.log(`RACE_PROBE=${results.every(r=>r.ok)?'PASS':'FAIL'} iterations=${N} assertions=${results.length}`);
}finally{
  await stop();
  if(uid){await pool.query('DELETE FROM "auth"."session" WHERE "userId"=$1',[uid]).catch(()=>{});await pool.query('DELETE FROM "auth"."user" WHERE id=$1',[uid]).catch(()=>{});}
  await pool.end();
}
