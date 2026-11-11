# Auth production readiness v2 — what is VERIFIED at HEAD, and what still blocks go-live

**Status:** evidence report. Nothing was deployed, no production host was contacted, and no
gate or formatter was run. Everything below is either a command that was executed in this
worktree (with its real output) or a line of source read from this worktree — the two are
labelled separately. **`VISUAL_QA` remains OPEN**: PNGs were produced and measured, never
inspected.
**Date:** 2026-11-11
**Worktree / head at the moment the stack was booted:**
`F:\sigap-design-v2`, `feat/sigap-selfhosted-auth` @ `a5423ec35b197395d4d506269d6dd26a1b56b17f`,
**with uncommitted peer edits in the tree** — `M apps/api/internal/auth/better_auth_compat_test.go`,
`M apps/api/internal/config/envguard_test.go`, `M apps/web/src/lib/server/auth.better.ts`,
`?? apps/web/tests/session-security.test.js`, `?? design/auth-gap-matrix-v2.md`.

**Head advanced during the pass** (peers committed while these runs were in flight):
`08671d8` → `ca6fc0e fix(auth): provision a citizen app_users row on registration + fail-closed
secret` → `b994926` → `42d43d2 test(auth): self-contained session-security suite + JWKS rotation
+ gap ledger` → `940d4b1` (14:15:59+07:00). Every code claim below was re-checked against the
**committed** files at that tip, where they are unchanged: `apps/web/src/lib/server/auth.better.ts`
in the working tree is byte-identical to `HEAD`, `git show 42d43d2:…auth.better.ts` already
contains both hardening markers this report relies on (`resolveSecret()` and
`rotationInterval: 60 * 60 * 24 * 30`), and the two files that were untracked at boot
(`apps/web/tests/session-security.test.js`, `design/auth-gap-matrix-v2.md`) are now tracked. The
browser evidence in §1 was produced against the `a5423ec`+working-tree stack, which for the auth
files is the same content that `42d43d2` committed.

**Sources this report is built on** (read, not re-derived): `design/auth-deployment-readiness.md`
(the ten-item assessment, 1 READY / 9 GAP — quoted verbatim in §2),
`design/auth-gap-matrix-v2.md` (the peer classification matrix),
`design/session-lifecycle-policy.md`, `design/email-mfa-flows-verification.md`,
`design/selfhosted-auth-e2e.md` (the E2E procedure of record), `design/better-auth-schema.md`.

**Companion evidence artefacts from this run**

| What | Where |
|---|---|
| E2E + fixture preparation | `apps/web/e2e/selfhosted-auth-journey.spec.ts`, `apps/web/scripts/prepare-auth-e2e.mjs` (§1.2) |
| Visual QA run A (`-Jwt -NoAdminActor`) | `F:\review-shots\auth-qa\` (16 PNG + `report.json`) |
| Visual QA run B (admin actor armed, cold server) | `review-shots\auth-qa-armed\` (16 PNG + `report.json`) |
| Visual QA run C (admin actor armed, warm server) | `review-shots\auth-qa-armed-warm\` (16 PNG + `report.json`) |

---

## 0. Verdict in one screen

| Question | Answer at this HEAD |
|---|---|
| Does the real browser sign-in journey pass? | **Yes — `4 passed (23.0s)`**, 4/4 cases, no retry (§1.2). One cold-dev-server flake preceded it and is documented (§1.2.1). |
| Do the negative security checks hold? | **15/15 as expected** (§1.4, §1.5): unauthenticated admin read 403; every forged identity header 403 byte-identical to no header; wrong password 401 (not 503); `redirectTo=/\evil.com` → `/`; a real cookie→JWT→Go-API chain returns the owner's own 2 rows. |
| Are the four screens captured at four widths? | **Yes, three times** (§1.3); geometry clean on the two warm runs; **pixels were not inspected — `VISUAL_QA=OPEN`**. |
| How many of the quoted 9 gaps remain? | **8 of 9 open**; #2 (cookie attributes) was READY as shipped; #4 and #5 have their **code half closed at this HEAD** and remain INFRA-BLOCKED. Classes: **5 CODE-FIXABLE, 3 TEST-FIXABLE, 7 INFRA-BLOCKED, 2 OWNER-BLOCKED** (an item may need more than one class) (§2). |
| Is the deployment envelope ready? | **No.** The compose `web` service forwards **no** `BETTER_AUTH_*` and no `SIGAP_DATABASE_URL`; `.env.example` names **no** `BETTER_AUTH_*`; the preflight script asserts **no** auth variable (§3). |
| Are the credentials/services present? | **No.** No SMTP transport, no production secret, no TLS at the edge in this stack, no least-privilege DB role (§4). |
| Is the identity store recoverable? | **Dump coverage measured ✓; restore verification ✗** — every drill query is a `public` table (§5). |
| Is sign-in auditable? | **No.** Only `auth.session` rows; the single `databaseHooks` entry is citizen provisioning, not audit (§2, gap 9). |

---

## 1. B4 — real browser acceptance, run at this HEAD

### 1.1 The stack that was booted

```
$ pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor
SIGAP V2.2 local preview
  [ok] PostgreSQL on 5433
  [ok] queue engine already listening on 50051
  stopping PID 16268 sigap-api
  [..] starting API on 18080
  [ok] API ready (audit=enabled)
  [..] stopping stale vite PID 27932
  [..] starting SvelteKit on 5173
  [ok] SvelteKit listening
Verifying the browser path (SvelteKit -> API -> PostgreSQL)...
  [ok] /api/v1/public/facilities -> 11 facilities
  [ok] /api/v1/patient/status -> RSUD Kota Sehat
```

`-Jwt` is the mode that matters: `Run-V22Service.ps1` then sets `SIGAP_AUTH_MODE=jwt`,
`SIGAP_AUTH_ISSUER=http://127.0.0.1:5173`, `SIGAP_AUTH_AUDIENCE=authenticated`,
`SIGAP_AUTH_JWKS_URL=http://127.0.0.1:5173/api/auth/jwks`, and empties `SIGAP_DEV_IDENTITY` /
`SIGAP_LOCAL_RBAC_TEST_IDENTITY`. `-NoAdminActor` passes an empty actor, so
`SIGAP_LOCAL_E2E_ACTOR` is **not** set for the web process — which is the only configuration in
which the journey proves anything (`design/selfhosted-auth-e2e.md` §1).

Component versions observed: Node `v24.19.0`, pnpm `9.0.0`, Vite `5.4.21`, better-auth
`1.7.20` (resolved from `apps/web/node_modules/better-auth`), Postgres `16-alpine` in container
`sigap-postgres` on `0.0.0.0:5433`.

**Fixture preparation.**

```
$ node apps/web/scripts/prepare-auth-e2e.mjs --apply --from-env-file
SIGAP self-hosted-auth E2E preparation
  target   : 127.0.0.1:5433/sigap  (local)
  mode     : APPLY
  reviewed : 4 identities, 2 citizen fixture appointments
  ...
  password     : verified (probe session discarded; 1 session(s) revoked)   # all four actors
  [citizen fixture — the two reviewed appointments]
  RS Sehat Nusantara Cimahi (3056): scheduled  (unchanged)
  Puskesmas Prima Tangerang (7f1e): cancelled → scheduled  (cancelled_at cleared)
  owner …742e: 2 appointment(s), 2 scheduled  (the spec asserts 2 cards, then 1 cancelled + 1 scheduled)

counts: identities=4  appointments=2  changed=3  audit rows=14  errors=0
```

Two details worth recording, because they are the difference between "it worked" and "it
worked for the wrong reason":

- `--from-env-file` was required. This agent shell exports a `DATABASE_URL` naming the
  **neighbouring project's** Postgres, and the script refused it exactly as designed:
  `refusing to run against 127.0.0.1:5434/sigap: port 5434 is a different project's Postgres`.
  The flag resolves the target from this repo's `.env` instead (`127.0.0.1:5433/sigap`).
- `changed=3` on the first apply were the two staff subject rewrites
  (`…c301`: `fPJ0y53x…` → `tqwaokeHX…`, `…d990`: `local-global-super-admin` → `EybtHQze3…`)
  and the one appointment reset. The re-apply before the accepted E2E run printed
  `changed=0`, i.e. the fixture state was already the asserted one.

### 1.2 The journey — both runs, honestly

Command (as specified):

```
$ SIGAP_E2E_SELFHOSTED_AUTH=1 SIGAP_E2E_BASE_URL=http://127.0.0.1:5173 \
    pnpm -C apps/web e2e selfhosted-auth-journey.spec.ts
```

This shell exports `CI=true`, and `playwright.config.ts` sets `retries: process.env.CI ? 1 : 0`,
so **run 1 got one automatic retry**; run 2 needed none.

#### 1.2.1 Run 1 — FAILED (a documented cold-dev-server flake, twice)

```
  1 failed
    [chromium] › e2e\selfhosted-auth-journey.spec.ts:248:2 › self-hosted auth journey
      (real Better Auth sessions) › patient A: two cards, and cancelling one through the UI
      leaves one scheduled
  3 did not run
 ELIFECYCLE  Command failed with exit code 1.
```

Attempt 0 and `Retry #1` failed identically, in `waitForHydration` (`e2e/support/test.ts:72`):

```
Error: expect(locator).toHaveAttribute(expected) failed
Locator:  locator('html')
Expected: "true"
Received: ""
  6 × locator resolved to <html lang="id">…</html>   - unexpected value "null"
  Test timeout of 30000ms exceeded.
   at support\test.ts:72  ← waitForHydration
   at support\test.ts:81  ← page.goto
   at selfhosted-auth-journey.spec.ts:117  ← signIn
```

Diagnosis, measured rather than assumed:

- `.local-preview/web.log` shows the dev server was ~20 s old and re-optimizing:
  `13.38.09 [vite] ✨ new dependencies optimized: lucide-svelte`, followed by
  `[vite] ✨ optimized dependencies changed. reloading` and repeated
  `[vite] page reload .svelte-kit/generated/...`. A full document reload mid-flight drops the
  hydration marker the assertion waits for.
- Immediately afterwards, a bare `chromium.launch()` load of the same URL hydrated in
  **737 ms** with **zero** console errors, page errors or failed requests.
- This is exactly the failure mode already written down in `design/selfhosted-auth-e2e.md` §9
  ("the first run after a fresh `vite dev` boot times out in `waitForHydration` … Vite's
  dependency re-optimization … Not a spec or app defect"), including the remedy that worked:
  warm the module graph, then re-run.

Honest reading: **not** a product or spec defect, and **not** evidence that the journey works.
It is evidence that the first E2E run after a fresh `vite dev` boot is unreliable at this HEAD.

#### 1.2.2 Run 2 — PASSED 4/4 (the accepted result)

```
Running 4 tests using 1 worker
  ok 1 [chromium] › e2e\selfhosted-auth-journey.spec.ts:248:2 › self-hosted auth journey
       (real Better Auth sessions) › patient A: two cards, and cancelling one through the UI
       leaves one scheduled (6.4s)
  ok 2 [chromium] › e2e\selfhosted-auth-journey.spec.ts:354:2 › … › patient B: an empty list,
       and A's appointment cannot be cancelled (4.7s)
  ok 3 [chromium] › e2e\selfhosted-auth-journey.spec.ts:408:2 › … › org A admin: exactly one
       organization, and a sibling org id is refused (5.6s)
  ok 4 [chromium] › e2e\selfhosted-auth-journey.spec.ts:455:2 › … › super admin: the directory
       holds both organizations (5.0s)

  4 passed (23.0s)
```

**Result line: `4 passed (23.0s)`**, serial, single worker, no retry.

#### 1.2.3 The data each case actually asserted

Read from the spec body (`apps/web/e2e/selfhosted-auth-journey.spec.ts`) plus the fixture state
printed by the preparation step above. Every item below is an `expect(...)` that passed in run 2.

| Case | Identity typed into the real form | Asserted data |
|---|---|---|
| **1. patient A** | `smoke@sigap.local` | Server pre-state via the proxy: **exactly 2** `scheduled` appointments. UI `h1` = `Janji saya`; **exactly 2** cards; **one** card at `RS Sehat Nusantara`, **one** at `Puskesmas Prima`; every card reads `Terjadwal`; the rendered card facilities **equal the API's own list** (same set, no client invention/omission); **2** cards offer `Batalkan`. Cancel through the dialog (dialog heading `Batalkan janji temu` and it names the appointment) → that card's badge becomes `Dibatalkan`. Server re-read: **2** appointments still listed, **exactly 1** still `scheduled`, and the still-scheduled one is **not** the cancelled facility. UI: 2 cards, **1** `Terjadwal` + **1** `Dibatalkan`. After sign-out, `/patient/appointments` shows the alert `Silakan masuk` and the list region is hidden. |
| **2. patient B** | `smoke-b@sigap.local` | Own list length **0**; UI text `Belum ada janji temu`; **0** cards; the body contains **neither** `RS Sehat Nusantara` nor `Puskesmas Prima`. B POSTs **A's** live check-in code to `/api/v1/appointments/cancel` → **403** with body `Anda tidak berhak membatalkan janji temu ini.` and the refusal does **not** name the appointment. The code-keyed status read (`/api/v1/patient/status?code=…`) still answers **`scheduled`** — the denied cancel changed nothing. After sign-out → `Silakan masuk`. |
| **3. org A admin** | `org-a-admin@sigap.local` | Scoped directory is exactly `['RS Sehat Nusantara Group']`; UI table has **1** body row; that row links to `/admin/organizations/00000000-0000-0000-0000-00000000c001`; the sibling name `Puskesmas Prima Sejahtera Network` is **absent** from the table; the table contains **no raw UUID**. Direct fetch of the **sibling** id → **404** `Organisasi tidak ditemukan.` with the sibling name absent from the body. Signed out → `Silakan masuk`, table hidden. |
| **4. super admin** | `global-admin@sigap.local` | Scoped directory is **both** org names; table has **2** body rows, each linked to its own `/admin/organizations/<id>`. Signed out → `Silakan masuk`, table hidden. |

Denials exercised: **403** (non-owner cancel of an owned appointment), **404** (out-of-scope
organization id, indistinguishable from a forged one), and the signed-out `Silakan masuk`
refusal on both the citizen and the admin surface.

### 1.3 Visual QA — three runs, geometry only

Harness: `node apps/web/scripts/capture-auth-visual-qa.mjs [baseUrl] [outDir]`. Its own header
says what it is: *"a geometry + artifact producer … it never inspects the PNGs it writes"*.

| Run | Stack | Output dir | Shot records | Ready |
|---|---|---|---|---|
| A | `-Jwt -NoAdminActor` (warm) | `F:\review-shots\auth-qa` | 16 | **16/16** |
| B | `-Jwt` (admin actor armed) — captured ~2 s after the restart | `review-shots\auth-qa-armed` | 16 | 14/16 |
| C | same armed stack, warm | `review-shots\auth-qa-armed-warm` | 16 | **16/16** |

**The 16 PNGs** (identical names in all three runs):
`auth-qa-login-{1440,768,390,360}.png`,
`auth-qa-register-{1440,768,390,360}.png`,
`auth-qa-appointments-signed-out-{1440,768,390,360}.png`,
`auth-qa-admin-{1440,768,390,360}.png`.

**Geometry summary, run C** (runs A and C are identical row-for-row):

```
view                        w  pageOverflow   clipped smallCtl /total  primaryVisible
login                    1440  no                   0        0     12  true
register                 1440  no                   0        0     13  true
appointments-signed-out  1440  no                   0        0      9  true
admin                    1440  no                   0        0     10  true
login                     768  no                   0        0     11  true
register                  768  no                   0        0     12  true
appointments-signed-out   768  no                   0        0      8  true
admin                     768  no                   0        0     10  true
login                     390  no                   0        0     11  true
register                  390  no                   0        0     12  true
appointments-signed-out   390  no                   0        0      8  true
admin                     390  no                   0        0     10  true
login                     360  no                   0        0     11  true
register                  360  no                   0        0     12  true
appointments-signed-out   360  no                   0        0      8  true
admin                     360  no                   0        0     10  true
Pixels were NOT visually inspected here; that review is the owner's step.
```

Read that as: no page-level horizontal overflow at any of the four widths, **zero** elements
clipped outside a scroll region, **zero** interactive controls under the 36 px touch floor, and
the primary affordance of every view inside the first viewport.

**Run B's two not-ready rows** (reported, not hidden):
`appointments-signed-out@1440` → `page.goto: net::ERR_ABORTED` and
`appointments-signed-out@360` → `page.screenshot: Protocol error (Page.captureScreenshot)`.
Both occurred while the freshly restarted Vite server was still re-optimizing dependencies;
run C, same stack and same code seconds later, was clean 16/16.

**PIXEL INSPECTION WAS NOT PERFORMED.** No colour, contrast, hierarchy, spacing or
"does it look right" judgement was made here, by this harness or by this author.
**`VISUAL_QA` stays OPEN** and the owner's review of the 48 PNGs is still owed.

**Harness defect found while doing this** (worth fixing before anyone cites it): the
`adminActorArmed` field in `report.json` is
`document.body.innerText.includes('local-global-super-admin')`, and that exact string is
**static copy** in `apps/web/src/routes/admin/+layout.svelte:69` (the non-dismissible dev
banner). It read `true` on run A, whose stack was started with `-NoAdminActor` and therefore
had **no** local actor. It cannot be used as evidence that the local selector was armed; use
`GET /admin` + `Silakan masuk`, or the API's 403, instead.

### 1.4 Negative checks over HTTP

Live stack: preview in `-Jwt -NoAdminActor`; API `http://127.0.0.1:18080`, web
`http://127.0.0.1:5173`. All requests are plain HTTP from Node (`fetch`), no browser.

| # | Check | Request | Outcome | Expected |
|---|---|---|---|---|
| N1 | Unauthenticated admin call (API direct) | `GET :18080/api/v1/admin/organizations` | **403** `{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}` | 401/403 ✓ |
| N2 | Forged local-test-subject, **real** seeded subject | same + `X-Sigap-Local-Test-Subject: local-global-super-admin` | **403**, body **byte-identical** to N1 | 401/403, nothing granted ✓ |
| N3 | Forged local-test-subject, **bogus** subject | same + `X-Sigap-Local-Test-Subject: made-up-subject` | **403**, byte-identical to N1 | ✓ |
| N4 | Forged dev-identity header | same + `X-Sigap-Dev-User-ID: local-global-super-admin` | **403**, byte-identical to N1 | ✓ |
| N5 | Unauthenticated admin call (browser-reachable proxy) | `GET :5173/api/v1/admin/organizations` | **403** | 401/403 ✓ |
| N6 | Browser supplies the selector itself | same + `X-Sigap-Local-Test-Subject: local-global-super-admin` from the **client** | **403** | ✓ |
| N7 | Wrong password, raw Better Auth surface | `POST /api/auth/sign-in/email {email, password: wrong}` | **401** | 401 ✓ |
| N8 | Wrong password, SvelteKit form action | `POST /auth/login` (form) | HTTP 200 carrying the action result `{"type":"failure","status":401,"data":[{"message":1},"Email atau kata sandi salah."]}` | **401, not 503** ✓ |
| N9 | Off-site bounce via `redirectTo` | native form POST, `redirectTo=/\evil.com`, `accept: text/html` | **303**, `location: /` | refused ✓ |
| N10 | Protocol-relative bounce | `redirectTo=//evil.com` | **303**, `location: /` | refused ✓ |
| N11 | Absolute URL bounce | `redirectTo=https://evil.com` | **303**, `location: /` | refused ✓ |
| N12 | Same-origin return-to (positive control) | `redirectTo=/patient/appointments` | **303**, `location: /patient/appointments` | honoured ✓ |

**The forged-header result is the important one.** `-Jwt` selects the JWT provider
unconditionally (`apps/api/cmd/server/main.go:264`), so the
`LocalRBACTestIdentityProvider` is not even constructed; the header reaches the server and is
ignored. N2/N3/N4 are byte-identical to N1 — the check is not "did it 403", it is "did the
response differ at all", and it did not. N6 is the same proof on the browser-reachable surface:
`proxyHeaders()` (`apps/web/src/lib/server/auth.ts:55`) builds the upstream headers from the
**process** environment through the gate at `auth.ts:121-143` and never forwards an inbound
`X-Sigap-*` value, so a browser cannot select an identity.

Two methodological notes, because they were wrong in a first draft and the correction matters:

- N8's **HTTP** status is 200. SvelteKit returns the *action result envelope* and the 401 lives
  inside it (`status: 401`). Judging the action by the HTTP status alone would have produced a
  false "not 401". The raw endpoint (N7) answers a real 401.
- An earlier draft ran the redirect cases immediately after the login cases and read
  `200 / no location`. That was the **login rate limiter** (`apps/web/src/lib/server/rate-limit.ts`,
  60 s / 10 per `login:<ip>`) rejecting the request, not a redirect failure. After waiting out
  the window the same requests answered 303 as shown above. The limiter is real, per-process and
  in-memory — see gap 8.

### 1.5 The trust boundary, exercised end to end (not asserted from source)

```
POST /api/auth/sign-in/email (correct credential)  → 200 + better-auth.session_token cookie
GET  /api/auth/token            (that cookie)      → 200 + ES256 JWT, 3 parts
     claims: iss=http://127.0.0.1:5173  aud=authenticated
             sub=ZYKICkM3YsiO1xlpPjolrojavaA6g8v9   exp-iat=3600s
GET  http://127.0.0.1:18080/api/v1/patient/appointments   Authorization: Bearer <jwt>
                                                    → 200, 2 rows
GET  http://127.0.0.1:5173/api/v1/patient/appointments   Cookie: <session cookie>
                                                    → 200, 2 rows   (cookie → hooks.server.ts mints the JWT → proxy → Go)
GET  http://127.0.0.1:5173/api/v1/admin/organizations    Cookie: <same session>
                                                    → 403          (correct: patient A holds no admin grant)
```

This closes the one item the gap matrix listed as unverified — *"the Go API trust boundary —
SvelteKit `getToken` → ES256 JWT → JWKS → `app_users.subject` path not exercised
end-to-end"* — for a non-MFA citizen subject. `sub` is the Better Auth user id; the 2 rows are
the ones the fixture assigns to that subject, so `app_users.subject` resolution ran. The JWT
claims match the configured `iss`/`aud` and the 1 h `expirationTime`. The `kid` header is the
`auth.jwks` row id (verified in §2.4), which is how the Go verifier selects the ES256 key the
live `auth.jwks` row advertises.

---

## 2. B5(a) — the exact remaining blocker list

### 2.1 The quoted gaps, and their state at this HEAD

Quoted verbatim from `design/auth-deployment-readiness.md` (verdict table). "State" is what
this run **verified** in the working tree and in the live stack at the moment of the runs — not
what any earlier document claimed — and each row was re-checked against the committed files at
the tip `940d4b1`, where nothing below changed.

| # | Gap (quoted one-line reason) | State at `a5423ec` + working tree | Class |
|---|---|---|---|
| 1 | HTTPS termination, proxy trust, HSTS — *"API half is implemented and tested; nothing asserts the web tier's public origin is `https`, and the compose comment claiming `SIGAP_TLS_TERMINATED` drives web HSTS is false."* | **Still open, unchanged.** `apps/web/src/hooks.server.ts` reads no TLS flag; HSTS is emitted only `if (event.url.protocol === 'https:')`. `docker-compose.yml` still carries the false comment and `ORIGIN: ${SIGAP_WEB_ORIGIN:-http://localhost:3005}`. `scripts/ops/preflight-production-env.sh` contains no `https://` prefix check. The Go half is intact (`GuardTLS`, `router/headers.go`). | **CODE-FIXABLE** (preflight prefix check + comment) + **INFRA-BLOCKED** (edge TLS, real origin) |
| 2 | Cookie security — *"Library-enforced HttpOnly + SameSite=Lax; `Secure` + `__Secure-` prefix derive from `BETTER_AUTH_URL` being `https`."* | **READY, as shipped.** No `advanced`/`defaultCookieAttributes` override exists in `auth.better.ts`; cookies are written through `sveltekitCookies(getRequestEvent)` registered last. Conditional on gap 1's value only. | **CLOSED** |
| 3 | CSRF / origin — *"`trustedOrigins` is unset and `BETTER_AUTH_URL` is not even passed by compose → the allow-list is a single fallback origin."* | **Still open, both halves.** `grep trustedOrigins apps/web/src` → **no match**; `auth.better.ts` declares only `secret/baseURL/database/databaseHooks/emailAndPassword/emailVerification/session/user/rateLimit/plugins`. `grep BETTER_AUTH docker-compose*.yml` → **no match**, so a compose deploy falls back to `http://127.0.0.1:5173` while the browser origin is `ORIGIN`. | **CODE-FIXABLE** (declare `trustedOrigins`) + **INFRA-BLOCKED** (compose must pass `BETTER_AUTH_URL`) |
| 4 | JWT/JWKS key rotation — *"`jwks.rotationInterval` unset → the ES256 signing key never expires and is never rotated; Go cache bounds retirement pickup at 15 min."* | **Code half CLOSED:** `jwks: { keyPairConfig: { alg: "ES256" }, rotationInterval: 60*60*24*30 }` is present. **But rotation has not started** — see §2.4: the persisted key has `expiresAt = null` and remains the selected signing key. | **CODE CLOSED** + residual **INFRA-BLOCKED** (operator must regenerate `auth.jwks`) + **TEST-FIXABLE** (assert the served JWKS key carries `expiresAt`) |
| 5 | Secrets — *"A public build placeholder is a silent fallback; compose/web env supply no real secret."* | **Code half CLOSED:** `resolveSecret()` returns `BETTER_AUTH_SECRET`, else the placeholder **only** while `building` or `NODE_ENV !== "production"`, else **throws** at module construction. **Envelope half still open:** compose forwards no `BETTER_AUTH_SECRET` and no `SIGAP_DATABASE_URL`; `.env.example` names neither. | **CODE CLOSED** + residual **INFRA-BLOCKED** (inject the real secret) + **CODE-FIXABLE** (compose, `.env.example`, preflight) |
| 6 | DB backup / restore of `auth` — *"Whole-DB `pg_dump` covers `auth`, but no restore verification queries `auth.*` — identity-store loss would pass the drill."* | **Coverage now MEASURED ✓; verification still absent ✗.** `docker exec sigap-postgres pg_dump -U sigap -d sigap --schema-only` emits **105** `CREATE TABLE`, including all six `auth.*` tables. `scripts/ops/restore-postgres.sh:158-167` counts only `schema_migrations, facilities, service_units, appointments, audit_events`; `Drill-PostgresRestore.ps1:81` iterates the same list; `docs/operations/BACKUP_RESTORE.md` has **no** `auth.` reference. | **TEST-FIXABLE** (add `auth.*` counts) + **INFRA-BLOCKED** (a live restore drill) |
| 7 | Separate `auth`-schema privileges — *"No least-privilege role exists anywhere; Better Auth runs as the same `sigap` role that owns `public` and the ledger."* | **Still open, confirmed at the live object level:** `pg_namespace` → `auth` owner `sigap`, `public` owner `pg_database_owner`; all **6** `auth` tables are owned by `sigap`; `information_schema.role_usage_grants` for schema `auth` is **empty**. The only `CREATE USER` anywhere is the dev bootstrap (`docs/DEV_SETUP.md:65`). | **INFRA-BLOCKED** (DB-admin role + grants; the DDL/DML split is an ops choice) |
| 8 | Rate limiting — *"Three layers exist (Better Auth native, web form limiter, Go limiter) but all are per-process in-memory."* | **Still open, confirmed live:** the `auth` schema contains **no** `rateLimit` table (matching `rateLimit.storage: "memory"`), and the 60 s / 10 per-IP web limiter was observably exhausted by probe requests. Single-replica invariant undocumented and unenforced. | **INFRA-BLOCKED** (shared store) + **OWNER-BLOCKED** (store choice / accept one replica) |
| 9 | Audit logs for sign-in / sign-out — *"Better Auth writes only `auth.session` rows; nothing reaches the Go `audit_events` table."* | **Still open.** The only `databaseHooks` entry in `auth.better.ts` is `user.create.after` (citizen `app_users` provisioning); there is no `session.create/delete` hook and no audit writer anywhere under `apps/web/src`. | **OWNER-BLOCKED** (who appends to the hash-chained table, with what identity) → **CODE-FIXABLE** once decided |
| 10 | Production config assertions — *"`preflight-production-env.sh` asserts the Go/API envelope only; no check covers any auth variable."* | **Still open, unchanged.** The script's whole assertion set is: `SIGAP_ENV` non-local, `SIGAP_DEPLOY_DIR` is a compose dir, `SIGAP_AUTH_MODE == jwt`, and the four dev flags absent/false. It never mentions `BETTER_AUTH_*`, `SIGAP_DATABASE_URL`, `SIGAP_AUTH_ISSUER`/`AUDIENCE`/`JWKS_URL`, `SIGAP_TLS_TERMINATED`, or an `https://` origin. | **CODE-FIXABLE** + **TEST-FIXABLE** |

**Of the nine quoted gaps: 8 open, 1 READY as shipped; two of the eight have their code half
closed at this HEAD (§4, §5) and remain infra-blocked in the deployment envelope.**

### 2.2 The two skipped checks from `design/session-lifecycle-policy.md`

1. **No `auth.user.banned` column (deactivation at the identity boundary).** Unchanged: the base
   config has no ban field (the `admin` plugin is not installed), so a deactivated account can
   still obtain a session and a JWT; the Go API refuses to authorize it because RBAC re-resolves
   `app_users.status='active' AND deleted_at IS NULL` per request. **OWNER-BLOCKED** (which
   failure the citizen should see) + **INFRA-BLOCKED** (a schema migration if the admin plugin
   is chosen).
2. **The 1 h `freshAge` window cannot be exercised live.** Unchanged; `session.freshAge` is
   pinned to 3600 s and the gate is asserted statically. **TEST-FIXABLE**, and closed for
   practical purposes by static assertion — exercising it needs a 3600 s wait.

### 2.3 The twelve unverified flow steps from `design/email-mfa-flows-verification.md` §6

Unchanged by this run (no mail, MFA or reset flow was touched). Classification per item:

| Item | Class |
|---|---|
| 1 real SMTP delivery | **INFRA-BLOCKED** (a transport + provider decision; `mail.ts` deliberately has none) |
| 2 `requireEmailVerification: true` branch | **OWNER-BLOCKED** (turning it on before a transport exists locks every account out) |
| 3 trusted device | **INFRA-BLOCKED** (a device + a policy decision) |
| 4 account lockout to ceiling | **TEST-FIXABLE** (`accountLockout: 10 / 15 min` is configured) |
| 5 verification-link expiry | **TEST-FIXABLE** (needs time travel or a 3600 s wait) |
| 6 TOTP drift window | **TEST-FIXABLE** |
| 7 resend endpoint | **TEST-FIXABLE** |
| 8 `two-factor/disable` + `generate-backup-codes` | **TEST-FIXABLE** |
| 9 Go trust boundary for an **MFA** subject | **TEST-FIXABLE** — note §1.5 now covers the non-MFA path end to end |
| 10 concurrent reset-token replay | **TEST-FIXABLE** |
| 11 backup-code exhaustion / encryption at rest | **TEST-FIXABLE** |
| 12 anti-enumeration (known vs unknown address) | **TEST-FIXABLE** |

### 2.4 Finding: the pinned rotation cannot start on a pre-existing key

This is the one place where reading the code fix is not enough, so it was verified three ways
(installed source, live table, request path).

- `apps/web/node_modules/better-auth/dist/plugins/jwt/utils.mjs` (`createJwk`) stamps
  `expiresAt` **only** when `options.jwks.rotationInterval` is set:
  `...options?.jwks?.rotationInterval ? { expiresAt: new Date(Date.now() + options.jwks.rotationInterval * 1e3) } : {}`.
- `dist/plugins/jwt/adapter.mjs` — `getLatestKey` / `getLatestKeyByAlg` filter with
  `!k.expiresAt || k.expiresAt > now` and pick the **newest live** key by `createdAt`.
- `dist/plugins/jwt/sign.mjs` — `resolveSigningKey` mints a fresh key only when the resolved key
  is **expired or absent**; `dist/plugins/jwt/index.mjs` serves a key while
  `expiresAt + gracePeriod (2592e3 s = 30 d default) > now`.
- **Live row**: `auth.jwks` = `{ id: nHpnIeNhokVUKtOET9QNgKciFCuoQgik, alg: ES256, crv: P-256,
  createdAt: 2026-11-10T18:20:32.912Z, expiresAt: null }`.

A row with `expiresAt = null` is live forever; if it is also the newest, it is the key that
signs, and no replacement is ever minted. **Consequence:** the 30-day rotation is a real
improvement for keys created from now on, but the key already in the database keeps signing
indefinitely until an operator deletes/regenerates that row. The cutover step is therefore:
`DELETE FROM auth.jwks` (or `npx auth@latest migrate`-adjacent regeneration) at a controlled
moment, after which `GET /api/auth/jwks` mints a key whose `expiresAt` is set and the rotation
cycle begins. The Go verifier's bounds are as documented: unknown `kid` ⇒ immediate refresh
(`jwt_provider.go:64-96`), cached key served at most 15 minutes (`:41`), stale key served on a
refresh failure rather than failing closed (`jwt_provider.go:82-86`).

**Second consequence, operational:** the same secret encrypts the `auth.jwks` private key
(`symmetricEncrypt`, `utils.mjs`) and the 2FA backup codes. `resolveSigningKey` fails hard with
*"Failed to decrypt private key. Make sure the secret currently in use is the same as the one
used to encrypt the private key"* if `BETTER_AUTH_SECRET` changes under an existing keyrow.
The secret is therefore **not** freely rotatable: rotating it requires regenerating the JWKS key
row **and** every enrollee's backup codes in the same window.

---

## 3. B5(b) — the verified deployment configuration

### 3.1 What the web tier reads (measured: every `process.env.*` read under `apps/web/src`)

| Variable | Consumer | Required production value | Source |
|---|---|---|---|
| `BETTER_AUTH_SECRET` | `auth.better.ts` → `resolveSecret()` | real secret, ≥ 32 chars, high entropy; **must not** be the build placeholder (a production launch without it throws) | `auth.better.ts:85-99` |
| `BETTER_AUTH_URL` | `auth.better.ts` (`baseURL`), Better Auth's cookie `Secure`/`__Secure-` derivation, and the CSRF allow-list's only origin | the **exact public `https` origin**, with a trailing-path-free form | `auth.better.ts:118` |
| `SIGAP_DATABASE_URL` (fallback `DATABASE_URL`) | `db.ts:24` → the Kysely `PostgresDialect` pool for the `auth` schema | reachable Postgres URL for `auth` | `db.ts:24` |
| `SIGAP_AUTH_ISSUER` | `jwt({ jwt.issuer })` | **exactly** the value the Go API verifies (`SIGAP_AUTH_ISSUER`); default `http://127.0.0.1:5173` | `auth.better.ts:348` |
| `SIGAP_AUTH_AUDIENCE` | `jwt({ jwt.audience })` | **exactly** the Go API's `SIGAP_AUTH_AUDIENCE`; default `authenticated` | `auth.better.ts:349` |
| `SIGAP_ENV` | `localE2eActorHeader()` gate | `staging` / `production` (anything but `local`) | `auth.ts:121-143` |
| `SIGAP_LOCAL_E2E_ACTOR` | the local selector — the only source of `X-Sigap-Local-Test-Subject` | **absent** | `auth.ts:121-143` |
| `SIGAP_API_INTERNAL` | every `+server.ts` proxy | internal API base | `auth.ts:apiBase()` |
| `SIGAP_REQUIRE_EMAIL_VERIFICATION` | `emailAndPassword.requireEmailVerification` | `true` only once a transport exists | `auth.better.ts` |
| `SIGAP_SMTP_URL` | `mail.ts:smtpConfigured()` | an SMTP URL — **but setting it delivers nothing today** (§4.1) | `mail.ts` |
| `SIGAP_MAIL_LOG` | capture-path override | unset in production | `mail.ts` |
| `NODE_ENV` | `resolveSecret()`'s production branch | `production` | `auth.better.ts:88` |
| `PUBLIC_SUPABASE_URL`, `PUBLIC_SUPABASE_ANON_KEY` | legacy Supabase seam still read at build/serve | unused by the self-hosted path | `apps/web/src` |

### 3.2 The Go/API contract (read from `apps/api`)

| Variable | Rule | Source |
|---|---|---|
| `SIGAP_ENV` | `TrimSpace` + case-insensitive `local` decides every dev gate | `envguard.go` `GuardDevCapabilities` |
| `SIGAP_AUTH_MODE` | `dev` \| `jwt` \| `disabled`; empty ⇒ `disabled` | `config.go:56-59` |
| `SIGAP_AUTH_ISSUER` | **required** in jwt mode | `config.go:83-86` |
| `SIGAP_AUTH_AUDIENCE` | **required** in jwt mode | `config.go:87-89` |
| `SIGAP_AUTH_JWKS_URL` | optional to `Validate()`, **fatal in practice**: the token path returns `"no JWKS configured"` | `jwt_provider.go:331-332` |
| `SIGAP_TLS_TERMINATED` | must equal `true` (case-insensitive) whenever `SIGAP_ENV` is non-local and non-empty | `envguard.go` `GuardTLS` |
| `SIGAP_API_HOST` | must be loopback while any dev capability is active | `envguard.go` `GuardDevNetworkBinding` |
| `SIGAP_TRUSTED_PROXIES` | proxy hop count peeled off `X-Forwarded-For`; code default 1, compose default 2 | `router/proxy.go:48` |
| `SIGAP_DATABASE_URL`, `SIGAP_ENGINE_ADDR`, `SIGAP_ENGINE_FALLBACK`, `SIGAP_WEB_ORIGIN`/`SIGAP_CORS_ORIGINS`, `SIGAP_API_PORT` | runtime envelope | `cmd/server/main.go`, `Run-V22Service.ps1` |
| **Refused outside `SIGAP_ENV=local`** | `SIGAP_AUTH_MODE=dev`, `SIGAP_DEV_IDENTITY=true`, `SIGAP_ENGINE_FALLBACK=dev`, `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true` | `envguard.go:22-57` |

Consistency note worth keeping: the Go dev-flag registry has **four** entries and does **not**
include `SIGAP_LOCAL_E2E_ACTOR`. That variable is the **web** tier's, guarded by
`localE2eActorHeader()` (`SIGAP_ENV=local` **and** a non-empty subject) and asserted absent by
`preflight-production-env.sh`. Nothing in the Go process refuses to start because of it, because
the Go process never reads it.

### 3.3 What a compose deployment actually forwards today

`docker-compose.yml`, `web` service, complete environment block:

```
PORT, HOST, ORIGIN, SIGAP_ENV, SIGAP_TLS_TERMINATED, SIGAP_API_INTERNAL,
PUBLIC_SIGAP_API_BASE, PUBLIC_SUPABASE_URL, PUBLIC_SUPABASE_ANON_KEY
```

No `BETTER_AUTH_SECRET`, no `BETTER_AUTH_URL`, no `SIGAP_DATABASE_URL`,
no `SIGAP_AUTH_ISSUER`/`SIGAP_AUTH_AUDIENCE`. `grep BETTER_AUTH docker-compose*.yml` → **no
match**. Therefore, on the documented production path:

- the web container cannot construct Better Auth's `database` (no Postgres URL) and
  `resolveSecret()` **throws at module construction** under `NODE_ENV=production` —
  i.e. gap 5 now fails **closed** instead of silently signing with a public constant, which is
  the right failure but still a deploy that cannot boot;
- if a URL is bolted on without `BETTER_AUTH_URL`, the CSRF allow-list is
  `http://127.0.0.1:5173` while the browser origin is `ORIGIN` — the origin check refuses every
  sign-in (fails closed, but as "sign-in does nothing");
- `.env.example` documents Supabase and the Go envelope and names **none** of the auth
  variables, so an operator following it cannot discover the requirement.

**Preflight, verbatim scope.** `scripts/ops/preflight-production-env.sh` asserts exactly:
`SIGAP_ENV` set / non-local (trimmed + lowercased), `SIGAP_DEPLOY_DIR` is a directory containing
`docker-compose.yml`, `SIGAP_AUTH_MODE` is exactly `jwt`, and `SIGAP_DEV_IDENTITY`,
`SIGAP_LOCAL_RBAC_TEST_IDENTITY`, `SIGAP_LOCAL_E2E_ACTOR`, `SIGAP_ENGINE_FALLBACK` are
absent/false. **Nothing about auth.** The assertion set required for production is §3.1 + §3.2
above, plus: `SIGAP_WEB_ORIGIN`/`BETTER_AUTH_URL` start with `https://`; `SIGAP_AUTH_ISSUER` and
`SIGAP_AUTH_AUDIENCE` equal the values the web tier signs with; `SIGAP_AUTH_JWKS_URL` equals
`${BETTER_AUTH_URL}/api/auth/jwks`; `BETTER_AUTH_SECRET` is present, ≥ 32 chars, and not equal
to `sigap-build-only-placeholder-secret`; `BETTER_AUTH_TRUSTED_ORIGINS` is absent unless
deliberately reviewed (it silently widens the allow-list); and the `auth` schema exists with its
six tables before the container serves traffic.

---

## 4. B5(c) — credentials and services still required

1. **A real `BETTER_AUTH_SECRET`.** Today the only secret anywhere is
   `.env`'s local dev value, and the compose path supplies none. Required: a generated
   high-entropy secret in the deploy secret store, injected as the service environment, never
   in a compose file, never in `.env.example`. Because it encrypts the JWKS private key and the
   2FA backup codes (§2.4), it must be handled as long-lived; a rotation procedure is part of
   this item, not a follow-up.
2. **SMTP, and a decision about what "delivered" means.** `mail.ts` is explicit: *"There is NO
   SMTP transport"*; `SIGAP_SMTP_URL` is a documented seam, and setting it today logs
   *loudly undelivered* and deliberately does **not** write the captured mail to disk.
   `SIGAP_SMTP_URL` is unset in `.env`. Required: a provider + credentials + the deliverability
   work (SPF/DKIM/DMARC on the sending domain) before `SIGAP_REQUIRE_EMAIL_VERIFICATION=true`
   can be flipped — otherwise verification and password reset are unreachable and turning the
   gate on locks every account out.
3. **TLS at the edge and one canonical public origin.** This stack is plain HTTP on
   `127.0.0.1:5173`; `BETTER_AUTH_URL` is `http://127.0.0.1:5173`, so cookies are non-`Secure`
   and un-prefixed here by design. Required: a TLS-terminating proxy (the compose overlays
   describe Traefik, `ENABLE_EDGE_ROUTING`), a real certificate, and
   `BETTER_AUTH_URL == ORIGIN == SIGAP_WEB_ORIGIN` at the `https` origin — the single lever that
   flips `Secure` cookies, the `__Secure-` prefix, the CSRF allow-list, the JWT `iss`, and the
   JWKS URL together.
4. **A least-privilege Postgres role for the `auth` schema.** Today `sigap` owns `auth` **and**
   `public` (measured: 6/6 `auth` tables owned by `sigap`, zero explicit grants), so the
   component that faces untrusted browsers holds write privileges over the clinical tables.
   Required: a dedicated login role with `USAGE, CREATE` on `auth` and nothing on `public`, its
   password in the secret store, and the web service pointed at it.
5. **A durable rate-limit store (or a written single-replica invariant).** All three limiters
   are per-process in memory; the `auth` schema has no `rateLimit` table. Required: Redis or the
   auth-schema store, or an explicit, reviewed "one web replica, one API replica" constraint.
6. **Access to a restore target** for the `auth`-schema drill (§5).

---

## 5. B5(d) — `auth`-schema backup/restore: evidence status

| Question | Status | Evidence |
|---|---|---|
| Does the `auth` schema exist, with which tables? | **Verified** | live `information_schema`: `auth.account`, `auth.jwks`, `auth.session`, `auth.twoFactor`, `auth.user`, `auth.verification` — **6** tables; rows 22 / 1 / 28 / 1 / 22 / 2. No `rateLimit` table (matches `storage: "memory"`). |
| Does a whole-DB dump include it? | **Measured, yes** | `docker exec sigap-postgres pg_dump -U sigap -d sigap --schema-only` → **105** `CREATE TABLE`, of which all six `CREATE TABLE auth."user"`, `auth.session`, `auth.account`, `auth.verification`, `auth.jwks`, `auth."twoFactor"` are present. No schema exclusion exists in `backup-postgres.sh` / `Backup-Postgres.ps1`. |
| Does a **restore** prove the identity store came back? | **No — gap open** | `scripts/ops/restore-postgres.sh:158-167` counts `schema_migrations`, `facilities`, `service_units`, `appointments`, `audit_events` (all `public`); `Drill-PostgresRestore.ps1:81` iterates the same list; `docs/operations/BACKUP_RESTORE.md` contains no `auth.` string. |
| Has a drill ever asserted `auth.*`? | **No evidence anywhere in the repo** | the closure evidence quoted by `auth-deployment-readiness.md` §6 ("6/6 tables match") is six `public` tables. |
| Was a restore drill performed here? | **No** | out of scope for this pass and INFRA-BLOCKED: it needs a restore target, and it must not be run against the shared dev database. |

Net: **dump-side coverage is now measured rather than assumed; restore-side verification is
absent, so an identity-store loss would still pass every existing drill assertion.**

---

## 6. B5(e) — security-regression results (commands and outcomes)

### 6.1 What was executed in this pass

| Command / probe | Outcome |
|---|---|
| `pnpm -C apps/web e2e selfhosted-auth-journey.spec.ts` with `SIGAP_E2E_SELFHOSTED_AUTH=1` (run 1) | **1 failed, 3 did not run** — hydration timeout, twice (§1.2.1) |
| same, run 2 after re-applying fixtures on the warm server | **4 passed (23.0s)** |
| 15 HTTP probes (§1.4, §1.5): unauthenticated admin 403; three forged identity headers 403 byte-identical to no-header; client-supplied selector 403; wrong password 401 at the raw endpoint **and** `status: 401` in the form-action result; four `redirectTo` cases (three refused, one honoured); cookie→JWT→API 200/2 rows; cookie→proxy citizen read 200/2 rows; cookie→proxy admin read 403 | **15/15 as expected** |
| `node apps/web/scripts/capture-auth-visual-qa.mjs` ×3 | geometry clean on 2/3 runs; **pixels not inspected** |
| read-only DB introspection (schemas, table ownership, grants, `auth.jwks` row) | as reported in §2.4, §5 |
| `pg_dump --schema-only` coverage measurement | 6/6 `auth` tables present |

### 6.2 What was NOT run, and why

The repo's own unit/integration suites were **not** executed in this pass, so no result is
claimed for them:

- `apps/web/tests/auth-config-assertions.test.js` (exists; would assert `resolveSecret()`'s
  fail-closed behaviour), `auth-actions.test.js`, `proxy-contracts.test.js`,
  `session-lifecycle.test.js`, `build-verification.test.js`, and `session-security.test.js`
  (untracked peer work at boot, since committed in `42d43d2`);
- the Go side: `apps/api/internal/auth/better_auth_compat_test.go` and
  `apps/api/internal/config/envguard_test.go` — both **modified and uncommitted** while these
  runs happened, so any pass/fail they produced would have described in-flight edits rather than
  a fixed commit (`envguard_test.go` was still modified at the end of the pass);
- any formatter, linter, build or deployment gate (explicitly out of scope for this assignment).

`design/auth-gap-matrix-v2.md` §5 maps each negative to the file that covers it at source level
(logout, expiration, refresh, revocation, session switching, password-change invalidation,
deactivation characterization, JWT TTL, JWKS rotation, wrong issuer/audience). This document
corroborates two of those claims live — the JWT `exp`s and `iss`/`aud` (§1.5), and the ES256 /
P-256 key shape (§2.4) — and does not restate the rest as if re-run.

---

## 7. B5(f) — authenticated-browser evidence

What exists at this HEAD, in ascending order of strength:

1. **Real form sign-in, four actors, four sessions** — `4 passed (23.0s)`, with identity-scoped
   data and three distinct denials (§1.2.3). Every case signs out through the UI and then
   observes an API-backed `Silakan masuk` refusal, so the session really was the credential.
2. **The whole credential chain, measured outside Playwright** (§1.5): session cookie →
   `/api/auth/token` → ES256 JWT (`iss`, `aud`, `sub`, 3600 s) → Go API `200` with the subject's
   own 2 rows; and the same cookie through the SvelteKit proxy → `200`/2 rows for the citizen
   endpoint, `403` for an admin endpoint the citizen may not read.
3. **Visual capture** of the four screens at four widths in three runs (§1.3) — geometry only.

What is **not** evidenced: any pixel-level review (`VISUAL_QA=OPEN`), any MFA-enrolled subject
through the trust boundary, any mail-delivered flow, and any run against a TLS-fronted origin.

---

## 8. B5(g) — remaining external dependencies

Nothing below can be finished by editing this repository alone; each is a person, an account, a
certificate, a host or a decision.

| # | Dependency | Blocks |
|---|---|---|
| 1 | TLS-terminating edge + certificate + the final `https` hostname | gaps 1, 3; cookie `Secure`; HSTS; the JWT `iss`; the JWKS URL |
| 2 | The real `BETTER_AUTH_SECRET` in the deploy secret store (and a rotation procedure, §2.4) | gap 5; any production boot at all |
| 3 | An SMTP provider with credentials and domain deliverability (SPF/DKIM/DMARC) | verification, password reset, `requireEmailVerification`, flow steps 1–2 |
| 4 | A Postgres role + grants for `auth`, issued by whoever administers the database | gap 7 |
| 5 | A durable rate-limit store, or a ratified single-replica invariant | gap 8; horizontal scaling |
| 6 | An owner decision on where sign-in/out audit events are written, and with what identity | gap 9 |
| 7 | A restore target and a scheduled window for a drill that asserts `auth.*` | gap 6 |
| 8 | An owner decision on the deactivation-at-identity gate (`admin` plugin + `banned`, or the status quo) | session-policy skip 1 |
| 9 | Replica counts for `web` and `api` (the three limiters assume one process each) | gap 8, and any capacity plan |
| 10 | A decision that staff may enrol trusted devices (30-day cookie is the longest-lived credential in the config) | MFA flow step 3 |
| 11 | Whoever operates the edge must confirm `X-Forwarded-Proto: https` and that `SIGAP_TRUSTED_PROXIES` matches the real hop count | gap 1 header trust, `getClientAddress()` rate-limit keys |

---

## 9. Blocker count by class

Counting the **nine quoted gaps** (the universe the acceptance criterion names), an item may
require more than one class:

| Class | Items | Count |
|---|---|---|
| **CLOSED** | #2 (READY as shipped); #4 and #5 code halves now closed at this HEAD | 3 (1 whole, 2 halves) |
| **CODE-FIXABLE** | #1 (https-origin assertion + compose comment), #3 (`trustedOrigins`), #5 envelope (compose + `.env.example` + preflight), #9 once the owner decides, #10 (preflight) | **5** |
| **TEST-FIXABLE** | #4 (assert the served JWKS key carries `expiresAt`), #6 (`auth.*` counts in the drill), #10 (Go tripwire) | **3** |
| **INFRA-BLOCKED** | #1, #3, #4 (regenerate `auth.jwks`), #5 (inject the secret), #6 (live drill), #7 (role), #8 (shared store) | **7** |
| **OWNER-BLOCKED** | #8 (store vs single replica), #9 (audit sink + identity) | **2** |

**Headline: 8 of 9 open; the go-live blocker set is 7 infra items + 5 code items + 3 test items +
2 decisions, and no single one of them is a mystery — each has a named owner-facing action.**

Extending to the other two source documents (2 session-policy skips + 12 flow steps), the tracked
total is **23 items**: 1 READY as shipped, 2 with the code half closed, and **20 open in whole or
part** — of which 13 are cheap live tests (MFA/flow steps 4–12, the lockout ceiling, the JWKS
`expiresAt` assertion, the drill query change) and the rest are the infra/owner items above.

---

## 10. Reproduce

```pwsh
# 1. stack — JWT authoritative, no local actor
pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor

# 2. fixtures (the flag is required when the shell exports a foreign DATABASE_URL)
cd apps/web
node scripts/prepare-auth-e2e.mjs --from-env-file            # plan
node scripts/prepare-auth-e2e.mjs --from-env-file --apply    # write

# 3. the journey — warm the module graph first (see §1.2.1)
$env:SIGAP_E2E_SELFHOSTED_AUTH = '1'
$env:SIGAP_E2E_BASE_URL = 'http://127.0.0.1:5173'
pnpm -C apps/web e2e selfhosted-auth-journey.spec.ts --retries=0

# 4. geometry / screenshots
node apps/web/scripts/capture-auth-visual-qa.mjs http://127.0.0.1:5173 review-shots/auth-qa

# 5. the negatives, in one line each
#   API 403 with and without a forged selector; web-proxy 403; wrong password 401
#   (`/api/auth/sign-in/email` → 401, form action → action status 401)
#   `POST /auth/login` with redirectTo=/\evil.com  → 303 location=/   (needs a valid fixture credential)
#   cookie → GET /api/auth/token → Bearer to :18080/api/v1/patient/appointments → 200, 2 rows

# 6. dump coverage
docker exec sigap-postgres pg_dump -U sigap -d sigap --schema-only | grep -c '^CREATE TABLE auth\.'
```

---

## 11. What this document does not claim

- No deployment, migration, rollback, restore or production contact was performed.
- No pixel review: **`VISUAL_QA=OPEN`**.
- No unit/integration suite result is claimed at this commit; the auth files those suites cover
  were uncommitted peer work when these runs started and were committed mid-pass (§6.2).
- Everything in §1 was measured on **one** loopback dev stack with **local** configuration
  values (`http` origin, dev secret, single replica); none of it is evidence about a
  TLS-fronted, multi-replica, SMTP-connected deployment.
- The run-1 E2E failure is reported as observed. Its cause (Vite dependency re-optimization) is
  supported by the server log, by a direct hydration measurement, and by the same failure mode
  already written down in `design/selfhosted-auth-e2e.md` §9 — not by a bisect.

## Changelog

- 2026-11-11 — first version. B4 executed (preview boot, fixture preparation, E2E ×2 runs,
  visual capture ×3, 15 HTTP probes, live DB introspection, dump-coverage measurement) against
  the `a5423ec` + working-tree stack; peers committed `ca6fc0e`/`b994926`/`42d43d2` mid-pass and
  every code claim was re-checked against the committed files at `940d4b1`, where they are
  unchanged. No deployment, no gate, no formatter. `VISUAL_QA=OPEN`.
