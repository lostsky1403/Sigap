# Staff provisioning — live verification (JWT mode, local selector OFF)

Evidence of record for a **live** run of `apps/web/scripts/provision-staff.mjs`
against the local SIGAP dev database, followed by four real sign-ins through the
Better Auth form, four JWTs minted from those sessions, and the Bearer-authenticated
Go API calls listed in §3–§4.

Date of run: 2026-11-11, 11:47–11:59 WIB (04:47–04:59 UTC).
Worktree: `F:\sigap-design-v2` — branch `feat/sigap-selfhosted-auth`, commit `d376c1b`.
Shell: git-bash (`sh`/`curl.exe`); PowerShell only for the launcher. Secrets are
redacted everywhere below (session tokens, JWT signatures, `DATABASE_URL` password).

Related documents:

- `design/staff-provisioning.md` — the reviewed staff table and the four
  non-negotiable rules this run exercises.
- `design/selfhosted-auth-e2e.md` — the four-actor browser journey over the same
  login → JWT → `app_users.subject` chain.
- `design/better-auth-integration-contract.md` — the auth seam.

---

## 0. Environment under test

| Component | Endpoint | Note |
| --- | --- | --- |
| Go API | `http://127.0.0.1:18080` | started by the launcher with `-Jwt` |
| SvelteKit web | `http://127.0.0.1:5173` | Better Auth issuer + JWKS |
| Queue engine | `127.0.0.1:50051` | already listening, untouched |
| PostgreSQL | `127.0.0.1:5433/sigap` | the SIGAP dev DB |
| PostgreSQL 5434 | **not touched** | a different project's data (see §2.6) |

Auth facts that make the run meaningful:

- The web tier mints an **ES256** JWT from the Better Auth session
  (`GET /api/auth/token`), issuer `http://127.0.0.1:5173`, audience
  `authenticated`, `exp` +3600s; JWKS at `GET /api/auth/jwks`.
- The Go API verifies that JWT (`SIGAP_AUTH_ISSUER` / `_AUDIENCE` / `_JWKS_URL`)
  and resolves `sub` → `app_users.subject` → `user_roles` → facility/org scope.
- `-Jwt` sets `SIGAP_DEV_IDENTITY=""` and `SIGAP_LOCAL_RBAC_TEST_IDENTITY=""`, so
  the local test-actor selector (`X-Sigap-Local-Test-Subject`) is **inert**: the
  Bearer JWT is the only thing that can name an actor.

---

## 1. Boot (jwt mode, no local admin actor)

```
$ pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor

SIGAP V2.2 local preview
========================

  [ok] PostgreSQL on 5433
  [ok] queue engine already listening on 50051
  stopping PID 12936 sigap-api
  [..] starting API on 18080
  [ok] API ready (audit=enabled)
  [..] stopping stale vite PID 28256
  [..] starting SvelteKit on 5173
  [ok] SvelteKit listening

Verifying the browser path (SvelteKit -> API -> PostgreSQL)...
  [ok] /api/v1/public/facilities -> 11 facilities
  [ok] /api/v1/patient/status -> RSUD Kota Sehat

=================================================
 SIGAP V2.2 preview: http://127.0.0.1:5173
 API:                http://127.0.0.1:18080
 Admin sign-in:      /auth/login (SIGAP_AUTH_MODE=dev)
=================================================
```

Wall time 13.7 s. The banner's `SIGAP_AUTH_MODE=dev` line is a hardcoded string
in the launcher, **not** the API's mode — the API's own log is authoritative:

```
$ grep -E "auth_mode|auth provider" .local-preview/api.log
2026/11/11 11:47:33 INFO sigap-api starting env=local auth_mode=jwt dev_identity="" engine_fallback=dev
2026/11/11 11:47:33 INFO jwt provider wired with server-side RBAC resolver
2026/11/11 11:47:33 INFO auth provider configured mode=jwt
```

Behavioural confirmation that the selector is off (before the restart the same
header granted `200` — see §4.2):

```
$ curl -s -w '\nHTTP=%{http_code}\n' http://127.0.0.1:18080/api/v1/admin/facilities
{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}
HTTP=403
```

---

## 2. Provisioning: apply, then prove idempotency

Every invocation below passes the 5433 URL explicitly. The tool refuses 5434 by
design and the guard has no override (§2.6).

### 2.1 Pre-apply plan (dry run)

```
$ SIGAP_DATABASE_URL='postgresql://sigap:<redacted>@127.0.0.1:5433/sigap?sslmode=require' \
    node apps/web/scripts/provision-staff.mjs

  target   : 127.0.0.1:5433/sigap  (local)
  mode     : DRY RUN (provisioning)

[operator-a1@sigap.local]
  app_user     : …c305  Operator A1 (synthetic)   (seed/regional_tenancy.sql:151,194)
  status       : active, Operator A1 (synthetic)
  grants (now) : operator@facility:…c101
  grants (new) : operator@facility:…c101
  better-auth  : (absent — will be created)
  subject      : local-operator-a1  →  (new better-auth id)
  action       : planned — auth=create, subject=update, grants +0

counts: identities=12  changed=24  audit rows=37  errors=0
DRY RUN — no Better Auth users created, no subject changed, no grants or statuses written.
```

The plan covers **12 reviewed identities**. 11 had no Better Auth account yet;
`global-admin@sigap.local` already had one (created `2026-11-10T19:09:44.190Z` by
an earlier session) and the script planned the `adopt-existing` refusal — that is
why the dry run reports `changed=24` (11 × 2 would-be writes + `global-admin`'s
two planning rows: the `auth.user.adoption` row and its `app_users.subject` row,
37 audit rows = 12 × 3 + 1) while the APPLY below reports `changed=22` (the
refused identity writes nothing at all).

### 2.2 First `--apply`

```
$ SIGAP_DATABASE_URL='postgresql://sigap:<redacted>@127.0.0.1:5433/sigap?sslmode=require' \
    node apps/web/scripts/provision-staff.mjs --apply

  target   : 127.0.0.1:5433/sigap  (local)
  mode     : APPLY (provisioning)
  reviewed : 12 staff identities (roles/scopes are static in this script; there is no --role flag)

=== CHANGE LOG (old → new) — APPLY (provisioning) ===   (rows 12–44 of 44, verbatim)
12  user_roles         e2e-schedule-manager@sigap.local …d993    facility_admin@facility:…d000                            facility_admin@facility:…d000
13  auth.session       sign-up session e2e-schedule-mixed@sigap.local   1 live                                              0 live (discarded)
14  auth.user          e2e-schedule-mixed@sigap.local …d994    xXMOLCiWhWokcmbKVWwcpdcZrbjLtWpp                        xXMOLCiWhWokcmbKVWwcpdcZrbjLtWpp
15  app_users.subject  e2e-schedule-mixed@sigap.local …d994    e2e-schedule-mixed                                      xXMOLCiWhWokcmbKVWwcpdcZrbjLtWpp
16  user_roles         e2e-schedule-mixed@sigap.local …d994    operator@facility:…e000, facility_admin@facility:…d000    operator@facility:…e000, facility_admin@facility:…d000
17  auth.session       sign-up session e2e-schedule-reader@sigap.local  1 live                                              0 live (discarded)
18  auth.user          e2e-schedule-reader@sigap.local …d995   Eac9p7faAWAGlzQt9eyE1SlgW5wYWX7C                        Eac9p7faAWAGlzQt9eyE1SlgW5wYWX7C
19  app_users.subject  e2e-schedule-reader@sigap.local …d995   e2e-schedule-reader                                     Eac9p7faAWAGlzQt9eyE1SlgW5wYWX7C
20  user_roles         e2e-schedule-reader@sigap.local …d995   operator@facility:…d000                                 operator@facility:…d000
21  auth.session       sign-up session org-admin-a@sigap.local         1 live                                              0 live (discarded)
22  auth.user          org-admin-a@sigap.local …c301           fPJ0y53x5R2eW8vCRYQ33ZidSGfrqALh                        fPJ0y53x5R2eW8vCRYQ33ZidSGfrqALh
23  app_users.subject  org-admin-a@sigap.local …c301           local-org-admin-a                                      fPJ0y53x5R2eW8vCRYQ33ZidSGfrqALh
24  user_roles         org-admin-a@sigap.local …c301           organization_admin@org:…c001                            organization_admin@org:…c001
25  auth.session       sign-up session org-admin-b@sigap.local         1 live                                              0 live (discarded)
26  auth.user          org-admin-b@sigap.local …c302           0gGepR3pfz1V8ZFnjt3B5UyssR0FIiI                        0gGepR3pfz1V8ZFnjt3B5UyssR0FIiI
27  app_users.subject  org-admin-b@sigap.local …c302           local-org-admin-b                                      0gGepR3pfz1V8ZFnjt3B5UyssR0FIiI
28  user_roles         org-admin-b@sigap.local …c302           organization_admin@org:…c002                            organization_admin@org:…c002
29  auth.session       sign-up session regional-admin@sigap.local      1 live                                              0 live (discarded)
30  auth.user          regional-admin@sigap.local …c303        ncRruJOmCW0ZqGEAwptahiYkQ3Pk6vl6                        ncRruJOmCW0ZqGEAwptahiYkQ3Pk6vl6
31  app_users.subject  regional-admin@sigap.local …c303        local-regional-admin                                    ncRruJOmCW0ZqGEAwptahiYkQ3Pk6vl6
32  user_roles         regional-admin@sigap.local …c303        regional_admin@global                                   regional_admin@global
33  auth.session       sign-up session facility-admin-a1@sigap.local   1 live                                              0 live (discarded)
34  auth.user          facility-admin-a1@sigap.local …c304     tvUTNRLUQsvXZceW5PBbrzlewYB97bAL                        tvUTNRLUQsvXZceW5PBbrzlewYB97bAL
35  app_users.subject  facility-admin-a1@sigap.local …c304     local-facility-admin-a1                                 tvUTNRLUQsvXZceW5PBbrzlewYB97bAL
36  user_roles         facility-admin-a1@sigap.local …c304     facility_admin@facility:…c101                           facility_admin@facility:…c101
37  auth.session       sign-up session operator-a1@sigap.local        1 live                                              0 live (discarded)
38  auth.user          operator-a1@sigap.local …c305           HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir                        HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir
39  app_users.subject  operator-a1@sigap.local …c305           local-operator-a1                                       HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir
40  user_roles         operator-a1@sigap.local …c305           operator@facility:…c101                                 operator@facility:…c101
41  auth.session       sign-up session operator-b1@sigap.local        1 live                                              0 live (discarded)
42  auth.user          operator-b1@sigap.local …c306           9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS                        9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS
43  app_users.subject  operator-b1@sigap.local …c306           local-operator-b1                                       9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS
44  user_roles         operator-b1@sigap.local …c306           operator@facility:…c103                                 operator@facility:…c103

counts: identities=12  changed=22  audit rows=44  errors=1
```

Transcript boundary (stated so this document is not read as more than it is): run
#1 was captured with `tail -35`, so rows **1–11** of its change log — the four
rows for `facility-admin@sigap.local`, the four for `zero-scope-admin@sigap.local`
and the first three for `e2e-schedule-manager@sigap.local` — are not reproduced
here. Per-identity `[email] … action :` blocks were not retained for run #1
either; the equivalent table for run #2 is reproduced in full in §2.3. The
`errors=1` above is the same `global-admin` adoption refusal shown verbatim from
runs #2 and #3 (§2.3, §2.4).

Result of run #1, stated precisely:

- **11 Better Auth users created** (`auth.user` rows) — all 12 reviewed emails
  except `global-admin@sigap.local`.
- **11 `app_users.subject` mappings written** (`local-*` placeholder → the new
  Better Auth id).
- **0 grants added.** Every `user_roles` row is `old == new`: the database was
  already seeded, and this script is deliberately a no-op on the authorization
  axis against a seeded DB (the reviewed table mirrors the seed exactly).
- **11 sign-up sessions discarded** — 8 of those `auth.session` rows
  (`1 live → 0 live (discarded)`) are visible in the rows reproduced above and 3
  sit in the elided head. Each provisioned account's automatic sign-up session is
  revoked, so no credential ends the run with a live session.
- **1 error** — the by-design `adopt-existing` refusal for `global-admin`.
  Nothing was written for that identity; its `app_users.subject` still reads
  `local-global-super-admin`.

Accounting of the 22 "changed" rows (arithmetic on the four rows per identity:
`auth.session` + `auth.user` + `app_users.subject` + `user_roles`, 11 identities ×
4 = 44): the 11 `auth.session` rows (`1 live → 0 live`) and the 11
`app_users.subject` rows (`local-* → id`) are the only `old != new` pairs —
11 + 11 = 22, and `global-admin` contributes no rows at all.

### 2.3 Second `--apply` — the idempotency proof

```
$ SIGAP_DATABASE_URL='postgresql://sigap:<redacted>@127.0.0.1:5433/sigap?sslmode=require' \
    node apps/web/scripts/provision-staff.mjs --apply

  target   : 127.0.0.1:5433/sigap  (local)
  mode     : APPLY (provisioning)
  reviewed : 12 staff identities (roles/scopes are static in this script; there is no --role flag)

[global-admin@sigap.local]
  better-auth  : …QUeK7HsQXO (adopt-existing, created 2026-11-10T19:09:44.190Z)
  subject      : local-global-super-admin  →  EybtHQze3NDTVxvqn5wbknQUeK7HsQXO
  !! refusing to adopt the pre-existing Better Auth user …QUeK7HsQXO for global-admin@sigap.local. … (same refusal)

[facility-admin@sigap.local]
  better-auth  : …qcqIRBz3sd (already-linked, created 2026-11-11T04:48:15.956Z)
  subject      : kAuov60FWQNoajYBoaCCyMqcqIRBz3sd  →  kAuov60FWQNoajYBoaCCyMqcqIRBz3sd
  action       : applied — auth=already-linked, subject=unchanged, grants +0

[zero-scope-admin@sigap.local]
  better-auth  : …UsHwqbt6RW (already-linked, created 2026-11-11T04:48:16.113Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[e2e-schedule-manager@sigap.local]
  better-auth  : …gSmInZyOu1 (already-linked, created 2026-11-11T04:48:16.249Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[e2e-schedule-mixed@sigap.local]
  better-auth  : …cZrbjLtWpp (already-linked, created 2026-11-11T04:48:16.382Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[e2e-schedule-reader@sigap.local]
  better-auth  : …lgW5wYWX7C (already-linked, created 2026-11-11T04:48:16.513Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[org-admin-a@sigap.local]
  better-auth  : …idSGfrqALh (already-linked, created 2026-11-11T04:48:16.643Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[org-admin-b@sigap.local]
  better-auth  : …8yssR0FIiI (already-linked, created 2026-11-11T04:48:16.771Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[regional-admin@sigap.local]
  better-auth  : …YkQ3Pk6vl6 (already-linked, created 2026-11-11T04:48:16.898Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[facility-admin-a1@sigap.local]
  better-auth  : …lewYB97bAL (already-linked, created 2026-11-11T04:48:17.032Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[operator-a1@sigap.local]
  better-auth  : …dy4AIi3bir (already-linked, created 2026-11-11T04:48:17.161Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0
[operator-b1@sigap.local]
  better-auth  : …f51kqqGBZS (already-linked, created 2026-11-11T04:48:17.316Z)
  action       : applied — auth=already-linked, subject=unchanged, grants +0

=== CHANGE LOG (old → new) — APPLY (provisioning) ===
#   entity             identity                                old                                                     new
1   auth.user          facility-admin@sigap.local …d991        kAuov60FWQNoajYBoaCCyMqcqIRBz3sd                        kAuov60FWQNoajYBoaCCyMqcqIRBz3sd
2   app_users.subject  facility-admin@sigap.local …d991        kAuov60FWQNoajYBoaCCyMqcqIRBz3sd                        kAuov60FWQNoajYBoaCCyMqcqIRBz3sd
3   user_roles         facility-admin@sigap.local …d991        facility_admin@facility:…d000                           facility_admin@facility:…d000
…
28  auth.user          operator-a1@sigap.local …c305           HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir                        HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir
29  app_users.subject  operator-a1@sigap.local …c305           HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir                        HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir
30  user_roles         operator-a1@sigap.local …c305           operator@facility:…c101                                 operator@facility:…c101
31  auth.user          operator-b1@sigap.local …c306           9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS                        9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS
32  app_users.subject  operator-b1@sigap.local …c306           9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS                        9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS
33  user_roles         operator-b1@sigap.local …c306           operator@facility:…c103                                 operator@facility:…c103

counts: identities=12  changed=0  audit rows=33  errors=1
```

**Idempotency: proven.** `changed=0`; all 33 rows (11 identities × 3) print
`old == new`; every identity reports `auth=already-linked, subject=unchanged,
grants +0`; no user, subject, grant or status was written. The single `errors=1`
is again the unchanged `global-admin` adoption refusal — a refusal, not a write.

Abridgements in the block above: each identity's `app_user` / `status` / `grants`
lines are elided (keeping `better-auth` and `action`), and the 33-row change log
is shown as rows 1–3 and 28–33 only. Every number quoted in this section
(`changed=0`, `audit rows=33`, `errors=1`) is the verbatim `counts:` line.

### 2.4 Third `--apply` (absolute Windows path in the invocation)

```
$ SIGAP_DATABASE_URL='postgresql://sigap:<redacted>@127.0.0.1:5433/sigap?sslmode=require' \
    node F:/sigap-design-v2/apps/web/scripts/provision-staff.mjs --apply

  target   : 127.0.0.1:5433/sigap  (local)
  mode     : APPLY (provisioning)
  !! refusing to adopt the pre-existing Better Auth user …QUeK7HsQXO for global-admin@sigap.local. …
counts: identities=12  changed=0  audit rows=33  errors=1
```

### 2.5 Database state after the runs

```sql
SELECT count(*) FROM auth."user" WHERE email = ANY(<the 12 reviewed emails>);
-- 12
SELECT count(*) FROM auth."user" WHERE email = ANY(...) AND "createdAt" >= '2026-11-11T04:48:00Z';
-- 11            (run #1 created these)
SELECT email FROM auth."user" WHERE email = ANY(...) AND "createdAt" < '2026-11-11T04:48:00Z';
-- global-admin@sigap.local @ 2026-11-10T19:09:44.190Z   (pre-existing, not adopted)
SELECT count(*) FROM app_users u WHERE lower(u.email) = ANY(...)
  AND u.subject = (SELECT id FROM auth."user" x WHERE x.email = u.email);
-- 11            (subject now points at the same-email Better Auth user)
SELECT email FROM app_users WHERE lower(email) = ANY(...) AND subject LIKE 'local-%';
-- global-admin@sigap.local     (still on the local-* placeholder)
```

### 2.6 The 5434 guard (never connects)

```
$ SIGAP_DATABASE_URL='postgresql://sigap:whatever@127.0.0.1:5434/odoo?sslmode=require' \
    node apps/web/scripts/provision-staff.mjs
provision-staff: refusing to run against 127.0.0.1:5434/odoo: port 5434 is a different project's Postgres (this repo's dev database is 127.0.0.1:5433/sigap).
  Fix one of:
    * set SIGAP_DATABASE_URL to this repo's dev database (see .env → DATABASE_URL), or
    * re-run with --from-env-file to resolve the target from the repo .env files only.
```

The refusal happens in `resolveTarget()` before any pool is constructed, so no
connection to 5434 was attempted; the process exits without touching it.

---

## 3. Sign-in through the REAL Better Auth form, then the Go API

### 3.1 Mechanics actually used

`POST /auth/login` is the SvelteKit form action (`+page.server.ts`), which calls
`auth.api.signInEmail` and lets the `svelteKitCookies` plugin set the session
cookie.

> **Note on `Accept`.** `@sveltejs/kit` decides between the HTML and the JSON
> response for a form action with `negotiate(accept, ['application/json',
> 'text/html'])` and picks `application/json` for curl's default `Accept: */*`.
> All sign-ins below therefore send a browser-style `Accept` header, which is
> what a real browser sends: a successful sign-in is a real `303` redirect.

```
# success (operator-a1)
$ curl -s -o /dev/null -w '%{http_code}\n' -c jar -X POST http://127.0.0.1:5173/auth/login \
    -H 'Content-Type: application/x-www-form-urlencoded' -H 'Origin: http://127.0.0.1:5173' \
    -H 'Accept: text/html,application/xhtml+xml;q=0.9,*/*;q=0.8' \
    --data-urlencode 'email=operator-a1@sigap.local' --data-urlencode 'password=<synthetic>'
303

# response headers (redacted)
HTTP/1.1 303 See Other
set-cookie: better-auth.session_token=<REDACTED>; Max-Age=604800; Path=/; HttpOnly; SameSite=Lax

# the session cookie buys a session, and the session buys the ES256 JWT
$ curl -s -b jar http://127.0.0.1:5173/api/auth/token
{"token":"<REDACTED, 661 bytes>"}          HTTP=200
```

Password used: the script's deterministic local default
`sigap-local-pw-<local-part>` (never printed; `SIGAP_STAFF_PASSWORD` was unset).

### 3.2 JWT claims (signature not reproduced)

```
header : {"alg":"ES256","kid":"nHpnIeNhokVUKtOET9QNgKciFCuoQgik"}
op-a1  : {"iat":1794373057,"name":"Operator A1 (synthetic)","email":"operator-a1@sigap.local",
          "emailVerified":false,"id":"HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir",
          "sub":"HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir","exp":1794376657,
          "iss":"http://127.0.0.1:5173","aud":"authenticated"}
org-a  : sub=fPJ0y53x5R2eW8vCRYQ33ZidSGfrqALh  (org-admin-a@sigap.local)
op-b1  : sub=9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS  (operator-b1@sigap.local)
org-b  : sub=0gGepR3pfz1V8ZFnjt3B5UyssR0FIiI  (org-admin-b@sigap.local)
sig    : <ES256 signature redacted>   exp − iat = 3600 s
```

`iss`, `aud` and the ES256 header are exactly what the Go API is configured to
trust in `-Jwt` mode (§1).

### 3.3 The identity chain, verified in the database

```
auth.user.email                  auth.user.id = JWT sub = app_users.subject        app_users.status   user_roles
operator-a1@sigap.local          HpCsuoD2zXEcjfooDKU1HYdy4AIi3bir                 active             operator@facility:00000000-0000-0000-0000-00000000c101
operator-b1@sigap.local          9WTOkv8zpUeoCL6Yq6DW55f51kqqGBZS                 active             operator@facility:00000000-0000-0000-0000-00000000c103
org-admin-a@sigap.local          fPJ0y53x5R2eW8vCRYQ33ZidSGfrqALh                 active             organization_admin@org:00000000-0000-0000-0000-00000000c001
org-admin-b@sigap.local          0gGepR3pfz1V8ZFnjt3B5UyssR0FIiI                  active             organization_admin@org:00000000-0000-0000-0000-00000000c002
```

The three ids in each row are byte-identical, and they are exactly the ids the
JWT carries in `sub` (§3.2).

### 3.4 Facility-scoped operator — sees ONE facility

```
$ curl -s -H "Authorization: Bearer $JWT_op_a1" http://127.0.0.1:18080/api/v1/admin/facilities
HTTP=200
{"data":[{"id":"00000000-0000-0000-0000-00000000c101","name":"RS Sehat Nusantara Cimahi",
  "type":"rumah_sakit","kecamatan":"Cimahi Utara","kabupaten_kota":"Kota Cimahi",
  "provinsi":"Jawa Barat","total_beds":120,"available_beds":40,"is_active":true,
  "short_code":"RSNCM"}],"success":true}
```

One row — its own facility, and nothing else, out of 11 facilities in the table.

| # | Call as `operator-a1` (operator @ facility `c101`) | Status | Body (abridged) |
| --- | --- | --- | --- |
| 1 | `GET /api/v1/admin/facilities` | **200** | **1 row: `c101` only** |
| 2 | `GET /api/v1/admin/facilities/c101` (own) | **200** | full detail of `c101` |
| 3 | `GET /api/v1/admin/facilities/c103` (sibling facility, org **B**) | **404** | `{"error":"Fasilitas tidak ditemukan.","success":false}` |
| 4 | `GET /api/v1/admin/facilities/c102` (sibling facility, its own org A) | **404** | `{"error":"Fasilitas tidak ditemukan.","success":false}` |
| 5 | `GET /api/v1/admin/organizations` (needs `organization.read`) | **403** | `{"success":false,"error":"Akses ditolak: izin tidak mencukupi."}` |
| 6 | `GET /api/v1/admin/service-units` | **200** | 2 rows, both `facility_id = c101` |
| 7 | `GET /api/v1/admin/schedules` | **200** | 1 row, `facility_id = c101` |
| 8 | `GET /api/v1/admin/appointments` | **200** | rows all `facility_id = c101` |
| 9 | `GET /api/v1/admin/queues` | **200** | `{"data":null,"success":true}` (no queues for `c101`; scope-correct empty) |

Row 3/4 are the sibling denial at **facility granularity** (a same-org sibling is
denied too — the operator's grant is the single facility, not the org), and row 5
is the capability denial (`operator` carries `schedule.read` but not
`organization.read`).

### 3.5 Org admin — sees exactly its organization

(Note on naming: the task description wrote `org-a-admin@sigap.local`; the email
in the reviewed `STAFF` table is **`org-admin-a@sigap.local`** — that is the
account used here, whose AppUser id is the seeded `…c301`.)

| # | Call as `org-admin-a` (organization_admin @ org `c001`) | Status | Body (abridged) |
| --- | --- | --- | --- |
| 1 | `GET /api/v1/admin/facilities` | **200** | **2 rows: `c102` (Bogor), `c101` (Cimahi)** — both org A, org B's `c103`/`c104` absent |
| 2 | `GET /api/v1/admin/organizations` | **200** | **1 row: `c001` "RS Sehat Nusantara Group"** of 2 orgs |
| 3 | `GET /api/v1/admin/facilities/c101` (own org) | **200** | full detail |
| 4 | `GET /api/v1/admin/facilities/c103` (sibling org **B**) | **404** | `{"error":"Fasilitas tidak ditemukan.","success":false}` |
| 5 | `GET /api/v1/admin/organizations/c002` (sibling org **B**) | **404** | `{"error":"Organisasi tidak ditemukan.","success":false}` |
| 6 | `GET /api/v1/admin/appointments` | **200** | org-A facilities only |

The org grant is expanded to the org's facility set (`facility_scope.go`
comments this expansion) so the org admin sees *both* of its facilities and
neither of its sibling's — the scope seam, not just a permission check.

### 3.6 Mirror pair — the isolation is symmetric

| Call | Status | Result |
| --- | --- | --- |
| `operator-b1` `GET /admin/facilities` | **200** | **1 row: `c103`** (own) |
| `operator-b1` `GET /admin/facilities/c103` | **200** | own detail |
| `operator-b1` `GET /admin/facilities/c101` | **404** | sibling org A denied |
| `org-admin-b` `GET /admin/facilities` | **200** | **2 rows: `c104`, `c103`** (org B) |
| `org-admin-b` `GET /admin/organizations` | **200** | **1 row: `c002`** |
| `org-admin-b` `GET /admin/organizations/c001` | **404** | sibling org A denied |

---

## 4. Negatives

### 4.1 No credentials / bad credentials — the API answers **403**, not 401

```
$ curl -s -w '\nHTTP=%{http_code}\n' http://127.0.0.1:18080/api/v1/admin/facilities
{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}
HTTP=403

$ curl -s -w '\nHTTP=%{http_code}\n' -H 'Authorization: Bearer not.a.jwt' \
    http://127.0.0.1:18080/api/v1/admin/facilities
{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}
HTTP=403

$ curl -s -w '\nHTTP=%{http_code}\n' -H 'Authorization: Bearer ' \
    http://127.0.0.1:18080/api/v1/admin/facilities
{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}
HTTP=403
```

**Recorded honestly:** on the admin surface the Go API returns **403** for a
missing, empty or malformed bearer — that is its documented behaviour
(`apps/api/internal/identity/authz.go:78` writes `http.StatusForbidden` with
exactly this message). The expectations of "401 for an unauthenticated call" hold
on two *other* boundaries, both observed:

```
# unrouted path — the deny-by-default router guard (apps/api/internal/router/router.go:130)
$ curl -s -w '\nHTTP=%{http_code}\n' http://127.0.0.1:18080/api/v1/nope
{"success":false,"error":"Akses ditolak: rute tidak dikenali atau memerlukan otorisasi."}
HTTP=401

# the auth tier's own token endpoint, no session cookie
$ curl -s -w '\nHTTP=%{http_code}\n' http://127.0.0.1:5173/api/auth/token
{"message":"Unauthorized","code":"UNAUTHORIZED"}
HTTP=401
```

A valid token is also required for a *routed* admin path: the same 403 (not a
404) is what an anonymous caller gets for `/api/v1/admin/queues/abc`, i.e. the
authz check runs before any resource lookup.

### 4.2 Forged `X-Sigap-Local-Test-Subject` grants nothing

```
$ curl -s -w '\nHTTP=%{http_code}\n' \
    -H 'X-Sigap-Local-Test-Subject: local-global-super-admin' \
    http://127.0.0.1:18080/api/v1/admin/facilities
{"success":false,"error":"Akses ditolak: autentikasi diperlukan."}
HTTP=403
```

Identical to the no-header call — byte for byte. The same request **before**
this boot (stack in `dev` mode, `SIGAP_LOCAL_RBAC_TEST_IDENTITY=true`) returned
`HTTP=200`, which is what makes this run evidence: the header is inert because of
the mode, not because the API ignores it in general.

### 4.3 The unmapped principal — `global-admin` (no HTTP observation)

`global-admin@sigap.local` was **not** signed in successfully (see §5). Its
Better Auth account exists but its `app_users.subject` is still the
`local-global-super-admin` placeholder, so even a successful sign-in would resolve
no actor — which is the provisioning rule "roles are never inferred from email"
holding at the API boundary (`WHERE u.subject = $1` matches nothing). No session
was established, so this is stated as a consequence of the refusal, not as a
separate HTTP observation.

### 4.4 Out-of-scope observation — a wrong password reports a **503 outage**

Not part of this ticket's acceptance, recorded because it was observed while
asserting the negatives:

```
$ curl -s -o /dev/null -w 'HTTP=%{http_code}\n' -X POST http://127.0.0.1:5173/auth/login \
    -H 'Content-Type: application/x-www-form-urlencoded' -H 'Origin: http://127.0.0.1:5173' \
    -H 'Accept: text/html,application/xhtml+xml;q=0.9,*/*;q=0.8' \
    --data-urlencode 'email=operator-a1@sigap.local' --data-urlencode 'password=wrong-password'
HTTP=503            # body contains "Layanan sedang bermasalah. Silakan coba lagi."

# Better Auth itself, same wrong password, native endpoint:
$ curl -s -w '\nHTTP=%{http_code}\n' -X POST http://127.0.0.1:5173/api/auth/sign-in/email \
    -H 'Content-Type: application/json' -d '{"email":"operator-a1@sigap.local","password":"wrong-password"}'
{"message":"Invalid email or password","code":"INVALID_EMAIL_OR_PASSWORD"}
HTTP=401
```

Two facts, both verified in the installed packages rather than inferred:

1. Better Auth throws `APIError.from("UNAUTHORIZED", …)`, and in
   `better-call@1.4.5` `InternalAPIError.status` is the **status-name string**
   (`"UNAUTHORIZED"`); the numeric code lives in `statusCode`
   (`better-call/dist/error.mjs:97-115`).
2. `apps/web/src/routes/auth/login/+page.server.ts` reads `err.status` and tests
   `status === 401`. `"UNAUTHORIZED" === 401` is never true, so the action always
   falls through to `fail(503, { message: 'Layanan sedang bermasalah…' })`.

Net effect: a wrong password is reported to the user as a service outage, and the
`401 "Email atau kata sandi salah."` branch is unreachable for a Better Auth
credential rejection (it requires a numeric `status`, which `APIError` never
carries). Not fixed here (outside this ticket's write scope) — flagged for the
auth-flow owner.

---

## 5. Identities NOT signed in (8 of 12), and why

Signed in: `operator-a1`, `operator-b1`, `org-admin-a`, `org-admin-b`.

| Not signed in | Reason |
| --- | --- |
| `global-admin@sigap.local` | Deliberately **unlinked**: `--apply` refused to adopt its pre-existing Better Auth account, so `app_users.subject` is still `local-global-super-admin`. A sign-in attempt with the deterministic synthetic password did not establish a session (503 page; no cookie written; `/api/auth/token` → 401), consistent with that account having been created by an earlier session with a different credential. Left untouched on purpose — adopting or deleting it is an out-of-band decision, and rule 3 exists precisely so a script does not make it. |
| `regional-admin@sigap.local` | Provisioned and signable; its grant is the unscoped `regional_admin@global`, which says nothing about facility/org isolation. Not required by this ticket's acceptance. |
| `facility-admin@sigap.local` | Provisioned and signable (`facility_admin@facility:…d000`). Not signed in. |
| `facility-admin-a1@sigap.local` | Provisioned and signable (`facility_admin@facility:…c101`). Not signed in. |
| `zero-scope-admin@sigap.local` | Provisioned and signable. Deliberate empty-scope fixture (`facility_admin` with no facility) — its own separate empty-scope assertion, not this ticket's. |
| `e2e-schedule-manager@sigap.local` | Provisioned and signable. Fixture for the schedule e2e pair. |
| `e2e-schedule-mixed@sigap.local` | Provisioned and signable. Mixed-provenance fixture (operator @ `e000`, facility_admin @ `d000`). |
| `e2e-schedule-reader@sigap.local` | Provisioned and signable (operator @ `d000`). Fixture for the schedule e2e pair. |

The acceptance asked for **at least one** facility-scoped operator and **at
least one** org admin; four were signed in because the second of each pair
(`operator-b1`, `org-admin-b`) is what turns "A sees only A" into "A sees only A
*and* B sees only B" — a one-sided result cannot distinguish scoping from a
globally broken filter. The remaining eight were skipped as redundant to that
claim. Seven of those eight (`regional-admin`, `facility-admin`,
`facility-admin-a1`, `zero-scope-admin`, and the three `e2e-schedule-*`
fixtures) have a working Better Auth account and the deterministic local password
after run #1, so any of them can be signed in by repeating §3.1 with a different
email; `global-admin` is the one exception, for the reason in its row above. No
identity was skipped because it could not be reached other than that one.

---

## 6. Findings

1. **Provisioning works live and is idempotent.** Run #1 created 11 Better Auth
   users and wrote 11 `app_users.subject` mappings; runs #2 and #3 wrote nothing
   (`changed=0`), each identity reporting `auth=already-linked,
   subject=unchanged, grants +0`.
2. **Authorization is id-keyed, never email-keyed.** All 11 subjects written are
   the Better Auth ids of the accounts the script created for the reviewed
   `{email, appUserId}` pairs, and the `global-admin` refusal proves the script
   will not link an account merely because it owns the address.
3. **The reviewed table matched the seeded grants exactly** — 0 grant rows
   changed in any of the three runs, as designed.
4. **The JWT → `app_users.subject` → scope chain works end to end** in `-Jwt`
   mode with the local selector off: `operator-a1` saw exactly 1 of 11 facilities,
   `org-admin-a` exactly its org's 2 facilities and 1 of 2 organizations, and the
   mirror pair reproduced it in the other direction. Sibling reads 404 rather than
   leaking existence; a missing capability 403s.
5. **An unauthenticated admin call is 403, not 401** (§4.1). 401 belongs to the
   router's deny-by-default guard and to `/api/auth/token`.
6. **The forged local test header grants nothing in `-Jwt` mode** (§4.2).
7. **Defect (out of scope, §4.4):** a wrong password returns the 503 outage
   message, and the intended 401 branch in the login action is unreachable
   because Better Auth's `APIError.status` is a status-*name* string.

## 7. Exact reproduction

```sh
# 1. boot (PowerShell)
pwsh -NoProfile -File scripts/dev/Start-V22Preview.ps1 -Jwt -NoAdminActor

# 2. plan, apply, then prove idempotency (bash; the password stays out of the doc)
export SIGAP_DATABASE_URL='postgresql://sigap:<password>@127.0.0.1:5433/sigap?sslmode=require'
node F:/sigap-design-v2/apps/web/scripts/provision-staff.mjs            # dry run
node F:/sigap-design-v2/apps/web/scripts/provision-staff.mjs --apply    # run #1
node F:/sigap-design-v2/apps/web/scripts/provision-staff.mjs --apply    # run #2 -> changed=0

# 3. real sign-in + JWT + API call (repeat per identity)
curl -s -o /dev/null -w '%{http_code}\n' -c jar -X POST http://127.0.0.1:5173/auth/login \
  -H 'Content-Type: application/x-www-form-urlencoded' -H 'Origin: http://127.0.0.1:5173' \
  -H 'Accept: text/html,application/xhtml+xml;q=0.9,*/*;q=0.8' \
  --data-urlencode 'email=operator-a1@sigap.local' \
  --data-urlencode 'password=sigap-local-pw-operator-a1'      # 303

JWT=$(curl -s -b jar http://127.0.0.1:5173/api/auth/token | sed -E 's/.*"token":"([^"]+)".*/\1/')
curl -s -w '\nHTTP=%{http_code}\n' -H "Authorization: Bearer $JWT" \
  http://127.0.0.1:18080/api/v1/admin/facilities               # 200, 1 row: c101

# 4. negatives
curl -s -w '\nHTTP=%{http_code}\n' http://127.0.0.1:18080/api/v1/admin/facilities    # 403
curl -s -w '\nHTTP=%{http_code}\n' -H 'X-Sigap-Local-Test-Subject: local-global-super-admin' \
  http://127.0.0.1:18080/api/v1/admin/facilities                                     # 403
```

Transient artifacts for this run live under
`F:\sigap-design-v2\.scratch\` (`provision-apply-2.log`, `rbac-probe.log`,
`rbac-probe2.log`, `rbac/`). Outside `.scratch/`, this document and the database
writes described in §2, the verification produced no side effects.
