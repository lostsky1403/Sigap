# Staff provisioning — explicit role/scope grants, deactivation, and the no-takeover rule

**Status:** Normative for `apps/web/scripts/provision-staff.mjs`.
**Date:** 2026-11-11
**Subject:** how a staff identity acquires (a) a Better Auth account, (b) an
`app_users.subject` mapping, and (c) its roles/scopes — plus how it loses them
(deactivation, role revocation), how the change is audited, and how it is rolled
back. Companion to `design/identity-mapping.md` (the provider → subject → app_users
model) and `design/session-lifecycle-policy.md` (session lifetime, the JWT
revocation bound, and the deactivation gap).

---

## 1. What provisioning actually writes

Three facts make a staff identity work end-to-end, in three different stores:

| # | Fact | Store | Written by |
|---|---|---|---|
| 1 | the credential + account: `auth.user` (email, hashed password) | `auth` schema (Better Auth) | `auth.api.signUpEmail` |
| 2 | the mapping: `public.app_users.subject = <auth.user.id>` | `public` schema (SIGAP) | `UPDATE ... WHERE id = <reviewed UUID>` |
| 3 | the authorization: `public.user_roles` rows (role + facility/organization scope) | `public` schema (SIGAP) | `INSERT ... ON CONFLICT (user_id, role_id)` |

The Go API consumes all three: it verifies the ES256 JWT, resolves
`WHERE u.subject = $1` (`apps/api/internal/auth/rbac_resolver.go`), and re-resolves
permissions from `user_roles`/`role_permissions` **on every request** — the token's
own claims are ignored (AUDIT-101). Identity (facts 1–2) and authorization (fact 3)
are therefore separable failure domains, which is what makes the revocation latencies
in §6/§7 different.

`scripts/backfill-auth-subjects.mjs` performs (1) and (2) for the synthetic **citizen
and staff fixtures** across the Supabase → Better Auth cutover, with identity only.
`scripts/provision-staff.mjs` performs (1)–(3) for the reviewed **staff** identities,
and adds the offboarding paths. The two scripts share the mapping rule and the same
two-store shape; they never run against each other's identity set (the citizen
`patient-p1@sigap.local`, `app_users.id = …c307`, holds no staff role by design and
is not in the staff table — the seed asserts that with a `RAISE EXCEPTION`).

## 2. The four rules

### 2.1 Role and scope assignment is explicit, reviewed, and never inferred

Roles/scopes are **static data in the script** (`STAFF` in
`apps/web/scripts/provision-staff.mjs`, mirroring the seeds row for row). They are
never derived from the email, the display name, the domain, an `app_users` column, or
a command-line argument.

> **There is deliberately no `--role` flag.** `--role super_admin` fails with
> `unknown argument` (`provision-staff.mjs` `parseArgs`). Granting a role from a shell
> is not a supported path; the only path is editing the reviewed table (i.e. a code
> review) and re-running the script.

### 2.2 No account takeover via email-only linking

The `app_users.subject` write is **always** `WHERE id = <reviewed UUID>`:

```sql
UPDATE app_users SET subject = $1, updated_at = now() WHERE id = $2;  -- $2 = app_users.id
```

There is no `UPDATE app_users SET subject = ... WHERE email = ...` anywhere in the
script, and email matches no app_user rows: `readAppUser` takes an id and
`readAppUserByEmail` is reached only from the two **restricting** lifecycle
operations (§6/§7), where an email lookup can only take access away, never grant it.

Email is used for exactly one thing: locating/creating the Better Auth row, which
Better Auth's own schema keys on email. Rationale for the rule is in
`design/identity-mapping.md` §3 — email is a self-asserted, transferable claim, and
the verified token proves `sub`, never an address.

**Adoption is a separate, explicit act.** If a Better Auth user *already exists* for a
provisioned staff email, the account owning that address was created by whoever
registered it first — provisioning it silently would hand that account the staff
role. So:

* the run prints `adopt-existing`, the account id and its `createdAt`;
* `--apply` **refuses** that identity (exit code 1) unless `--adopt-existing` is
  passed explicitly;
* adoption is then recorded in the audit table, and the operator is expected to have
  verified out of band that the account belongs to the intended person;
* a pre-existing Better Auth user that is already claimed as the `subject` of a
  *different* app_user is refused outright (fail-closed) — there is no flag for that.

The correct deployment order is therefore: **migrate → seed → provision → open
registration**. A staff email that gets adopted after go-live should be treated as an
incident (verify ownership, then rotate the password and revoke sessions).

### 2.3 No public super-admin registration

`POST /auth/register` (`apps/web/src/routes/auth/register/+page.server.ts`) mints a
Better Auth user and nothing else — no `app_users` row, so `WHERE u.subject = $1`
matches no actor and the account authorizes **nothing** (the fail-closed seam of
`design/identity-mapping.md` §1). The public form cannot choose a role, cannot create
an app_user, and cannot reach `super_admin`.

The `super_admin` grant exists in exactly one place in the codebase: the reviewed
`STAFF` table (`global-admin@sigap.local`, `…d990`, unscoped). Its scope is
`facility_id IS NULL AND organization_id IS NULL` — the only shape that satisfies
`AdminHandler.CreateFacility`'s unscoped-super-admin check — and the script prints a
loud `warning : super_admin grant` line whenever that row is processed.

### 2.4 Dry-run first, idempotent always

Nothing is written without `--apply`, in any mode. The default run is a plan: it reads
both stores and prints the exact change table (§8) it would write. Re-running
`--apply` on a provisioned database is a no-op: the subject write is skipped when the
value already matches, the grant upsert is `ON CONFLICT (user_id, role_id) DO UPDATE`
with the same values, and the audit table shows `old == new` for every row.

## 3. The reviewed staff table (12 identities)

Emails are the seeded synthetic addresses; `app_users.id` is the STABLE internal id
and the join key for every write. `role @ scope` is exactly what the seeds grant —
the same values, so the **grant writes** are a no-op against a seeded database and
the table is the authority for a database that was never seeded. (The subject write
is not a no-op there: the seeded `local-*` subjects are pre-cutover placeholders,
and the verified token's `sub` — the Better Auth user id — replaces them.)

| email | `app_users.id` | role @ scope | seed provenance |
|---|---|---|---|
| `global-admin@sigap.local` | `…d990` | `super_admin` @ global (unscoped) | `seed/dev.sql:41` + `seed/demo.sql:95` |
| `facility-admin@sigap.local` | `…d991` | `facility_admin` @ facility `…d000` (Sigap Demo Facility) | `seed/dev.sql:42` + `seed/demo.sql:96` |
| `zero-scope-admin@sigap.local` | `…d992` | `facility_admin` @ **no scope** (negative fixture) | `seed/dev.sql:43` + `seed/demo.sql:97` |
| `e2e-schedule-manager@sigap.local` | `…d993` | `facility_admin` @ facility `…d000` | `seed/dev.sql:114` + `seed/demo.sql:101` |
| `e2e-schedule-mixed@sigap.local` | `…d994` | `operator` @ facility `…e000` (RSUD Kota Sehat) **and** `facility_admin` @ facility `…d000` | `seed/dev.sql:115` + `seed/demo.sql:116-117` |
| `e2e-schedule-reader@sigap.local` | `…d995` | `operator` @ facility `…d000` | `seed/dev.sql:116` + `seed/demo.sql:147` |
| `org-admin-a@sigap.local` | `…c301` | `organization_admin` @ org `…c001` (RS Sehat Nusantara Group) | `seed/regional_tenancy.sql:147,186` |
| `org-admin-b@sigap.local` | `…c302` | `organization_admin` @ org `…c002` (Puskesmas Prima Sejahtera Network) | `seed/regional_tenancy.sql:148,188` |
| `regional-admin@sigap.local` | `…c303` | `regional_admin` @ global (unscoped, stats read-only) | `seed/regional_tenancy.sql:149,190` |
| `facility-admin-a1@sigap.local` | `…c304` | `facility_admin` @ facility `…c101` (RS Sehat Nusantara Cimahi) | `seed/regional_tenancy.sql:150,192` |
| `operator-a1@sigap.local` | `…c305` | `operator` @ facility `…c101` | `seed/regional_tenancy.sql:151,194` |
| `operator-b1@sigap.local` | `…c306` | `operator` @ facility `…c103` (Puskesmas Prima Tangerang) | `seed/regional_tenancy.sql:152,196` |

Roles come from `seed/rbac.sql:8-12` (`super_admin`, `facility_admin`, `operator`,
`viewer`) and `seed/regional_tenancy.sql:78-82` (`organization_admin`,
`regional_admin`). Scope semantics: `organization_id NOT NULL AND facility_id IS NULL`
= the whole organization; `facility_id NOT NULL` = that facility;
both NULL = global/unscoped (`packages/db/migrations/0013_organizations.sql:39-46`).

Two rows exist to keep negative fixtures honest and must not be "fixed":

* `…d992` — `facility_admin` with **no scope**: drives the empty-scope UI state
  (`seed/dev.sql:97-99` explains why a scoped actor cannot prove that state).
* `…d994` — **two roles at two different facilities** for one user: the shape the
  `user_roles` primary key `(user_id, role_id)` exists to express, and the regression
  the mixed-provenance schedule tests catch. Collapsing the two facilities would
  delete the test's reason for existing.

A new staff identity is added in two reviewed steps: (1) an `app_users` row with a
chosen app-owned UUID (seed or numbered migration — never generated by this script),
(2) an entry in the reviewed `STAFF` table. Changing someone's role or scope is step
(2) alone.

## 4. Modes and flags

```
node apps/web/scripts/provision-staff.mjs                       # plan (dry run) all 12 identities
node apps/web/scripts/provision-staff.mjs --apply               # create/map/grant
node apps/web/scripts/provision-staff.mjs --apply --log <path>  # + append the audit block
node apps/web/scripts/provision-staff.mjs --deactivate <email>  [--apply]
node apps/web/scripts/provision-staff.mjs --revoke-roles <email> [--apply]
node apps/web/scripts/provision-staff.mjs --deactivate <email> --revoke-roles <email> [--apply]
```

| Flag | Effect |
|---|---|
| `--apply` | write. **Without it every mode is a dry run.** |
| `--deactivate <email>` | revoke all Better Auth sessions + `app_users.status = 'disabled'` (§6); repeatable |
| `--revoke-roles <email>` | `DELETE FROM user_roles WHERE user_id = …` (§7); repeatable |
| `--adopt-existing` | allow linking to a pre-existing Better Auth account (§2.2) |
| `--log <path>` | append the run's markdown audit block to `<path>` (§8) |
| `--from-env-file` | resolve the DB target from the repo `.env` files only, ignoring ambient `SIGAP_DATABASE_URL`/`DATABASE_URL` |
| `--allow-remote-db` | allow a non-loopback target (staging/production) |
| `--help` | usage |

A lifecycle flag switches the run to **lifecycle mode**: the provisioning pass is not
executed in the same invocation, so one run never both grants and revokes. Grant
upserts are the only write the provisioning mode performs on `user_roles`; nothing is
ever deleted there.

### 4.1 Target resolution and interlocks

Precedence is `SIGAP_DATABASE_URL` → `DATABASE_URL` → repo `.env` (root, then
`apps/web`). Two guards run **before any socket is opened** (`resolveTarget`):

1. **Port 5434 is refused, with no override.** On a loopback host, 5434 is a
   *different* project's Postgres (this repo's dev database is
   `127.0.0.1:5433/sigap`). This guard exists because a developer shell in this
   monorepo can export a `DATABASE_URL` for another project — the ambient environment
   of the machine this script was developed on did exactly that. The refusal message
   names both remedies: set `SIGAP_DATABASE_URL` to the SIGAP database, or pass
   `--from-env-file`.
2. **A non-loopback host is refused** without `--allow-remote-db`.

The connection itself mirrors `apps/web/src/lib/server/db.ts`, including its
LOCAL-ONLY `sslmode` strip: Node's `pg` treats `sslmode=require` as verify-full and
rejects the dev server's self-signed certificate, so the parameter is removed on a
loopback target and `ssl.rejectUnauthorized = false` is set instead. A non-local
target keeps the URL untouched (full verification). (The backfill script omitted this
strip; this script does not repeat that.)

## 5. What a provisioning run prints (and writes)

Per identity:

```
[global-admin@sigap.local]
  app_user     : …d990  Local Global Super Admin (synthetic)   (seed/dev.sql:41 + seed/demo.sql:95)
  status       : active, Local Global Super Admin (synthetic)
  grants (now) : super_admin@global
  grants (new) : super_admin@global
  warning      : super_admin grant — full system access; this is the only supported path to it
  better-auth  : (absent — will be created)
  subject      : local-global-super-admin  →  (new better-auth id)
  action       : planned — auth=create, subject=update, grants +0
```

then the audit table of §8 and counts
(`identities / changed / audit rows / errors`).

Writes, in order, per identity: (a) `auth.api.signUpEmail` when no account exists,
(b) a sign-up session is **discarded** immediately (`auth.user` creation auto-creates
one; a CLI must not leave live sessions behind), (c) the `subject` write by id,
(d) the grant upserts. A grant already active in `user_roles` but **absent from the
reviewed table** is reported as `drift … left in place` and never deleted on the fly:
de-privileging silently is as dangerous as privileging silently.

Failing inputs fail loudly and per identity, never silently: a missing `app_users`
row, a soft-deleted app_user, an unknown role name, or a facility/organization UUID
that does not exist in the target DB stops that identity (and sets exit code 1) while
the rest of the run continues.

## 6. Deactivation (`--deactivate <email>`)

Two writes, one per store, plus the honest residual:

| Step | Statement / call | Effect |
|---|---|---|
| 1. revoke sessions | `(await auth.$context).internalAdapter.deleteUserSessions(userId)` | every `auth.session` row for that Better Auth user is deleted; the script verifies `count == 0` afterwards |
| 2. disable | `UPDATE app_users SET status = 'disabled', updated_at = now() WHERE id = $1` | the Go RBAC resolver's `u.status = 'active'` filter drops every permission on the **next** API request |

Step 1 uses the library's own adapter method — the exact call
`POST /api/auth/revoke-sessions` makes (`better-auth@1.7.20`,
`dist/api/routes/session.mjs` → `db/internal-adapter.mjs` `deleteUserSessions`),
because that endpoint is `sensitiveSessionMiddleware`-gated and revokes the *caller's*
sessions; a CLI has no caller session. If the method is ever absent (library upgrade),
the script throws instead of guessing a revocation path. `status` is `'disabled'`, not
`'inactive'`: `packages/db/migrations/0003_identity_rbac.sql:11` constrains it to
`('active','disabled')`.

What deactivation must NOT be mistaken for:

* **Better Auth does not ban.** better-auth 1.7.20's base config has no ban field on
  `auth.user` (`banned`/`banReason`/`banExpires` arrive with the admin plugin, which is
  not installed) — verified on the dev database: `auth."user"` has exactly
  `id, name, email, emailVerified, image, createdAt, updatedAt`. A deactivated account
  can therefore still *create* a session; it authorizes nothing, because
  authorization is resolved from `public` on every request
  (`design/session-lifecycle-policy.md` §3, which also records the two supported
  hard-gate mechanisms if that is ever required).
* **Already-minted JWTs keep authenticating the subject for ≤ 1 h.** Session
  revocation stops new tokens at once, but a token already handed out verifies until
  its `exp` — the accepted bound of `design/session-lifecycle-policy.md` §2.1
  (access-token TTL = 1 h, zero leeway).
* **Roles are not removed.** `status = 'disabled'` already resolves to zero
  permissions, but if the rows must be gone as well, run `--revoke-roles` too.

The script prints all three notes on every deactivation, plus a reminder listing any
grant that is still active.

## 7. Role revocation (`--revoke-roles <email>`)

`DELETE FROM user_roles WHERE user_id = $1` — a hard delete of **every** row for that
user (active, inactive, soft-deleted), followed by a re-read that must return zero
rows. A hard delete matches the seeds' own lifecycle convention
(`seed/demo.sql:78-84` deletes before re-inserting) and the fact that
`rbac_resolver.go` treats a missing row exactly like an inactive one.

Semantics to keep straight:

* **Permission loss is immediate** (next request): the resolver re-reads
  `user_roles` with `ur.status = 'active' AND ur.deleted_at IS NULL` on every call.
* **Identity is not touched**: the account keeps its sessions and can still sign in.
  For offboarding, pair `--revoke-roles` with `--deactivate`; the runbook order is
  roles first, then deactivate (so the two audit rows read in the direction of the
  access being removed).
* **Re-granting** is `--apply` of the provisioning mode, which upserts the reviewed
  grants back (idempotent), or a re-run of the seed files.

## 8. Audit trail

Two artefacts, both from the same run:

**1. The printed change log** — one row per changed fact, old value and new value
explicit:

```
=== CHANGE LOG (old → new) — APPLY (provisioning) ===
#   entity              identity                          old                         new
1   auth.user           global-admin@sigap.local …d990    (absent)                    created
2   app_users.subject   global-admin@sigap.local …d990    local-global-super-admin    <the better-auth user id>
3   user_roles          global-admin@sigap.local …d990    (absent)                    +super_admin @ global (unscoped)
```

In dry-run every `new` value is the plan (`(new better-auth id)`, `(would be created)`,
`+<role> @ <scope> (on --apply)`, `disabled (on --apply)`), so the table can be
reviewed before it becomes a record. Values are states (ids, statuses), not verbs, so
a re-run that changed nothing prints `old == new` for every row it did not touch.
Entities used: `auth.user`, `auth.user.adoption` (a pre-existing account linked under
`--adopt-existing`), `auth.session`, `app_users.subject`, `app_users.status`,
`user_roles`.

**2. The log convention** — `--log <path>` appends a markdown block with the UTC
timestamp, mode, target (`host:port/database` — never the credentials), operator,
the same change table, and any errors. The convention for this repo:

> `docs/operations/staff-provisioning-log.md` — one appended section per run, newest
> last, never edited retroactively. Where a change is reviewed before execution, the
> dry-run output may be pasted above the applied section for the reviewer's signature.

The operator is `SIGAP_PROVISION_OPERATOR` when set, otherwise the OS user running
the script (`os.userInfo().username`). Set the env var on shared machines, where the
login name may be a service account.

Writing the log file is deliberately **not** automatic: an audit trail the tool
invents for itself is not evidence. The section is appended only where the operator
points it, and the same block is printed to stdout so a CI job or a terminal capture
contains it either way.

## 9. Rollback

Every rollback uses the values the audit table already printed (`old` column), and no
step touches `app_users.id`, `patient_account`, `audit_events`, or the migration
ledger.

| Change | Rollback |
|---|---|
| `app_users.subject` re-pointed | `UPDATE app_users SET subject = '<old>' WHERE id = '<appUserId>'` (the previous value may be a Supabase `auth.users.id`, or the seeded `local-*` subject — see `design/identity-mapping.md` §5) |
| `app_users.status` disabled | `UPDATE app_users SET status = 'active' WHERE id = …` |
| `user_roles` rows deleted | re-run `--apply` (provisioning mode) or the seed file (`packages/db/seed/demo.sql`, `regional_tenancy.sql`) — both idempotent, both restore exactly the reviewed grant set |
| `auth.user` created | leave it (with `subject` reverted it authorizes nothing) or `DELETE FROM auth."user" WHERE id = …` as part of dropping the whole `auth` schema (`design/better-auth-schema.md` rollback) |
| sessions revoked | nothing to restore — signing in again mints a new session (the old tokens' `sub` is unaffected until `exp`) |

Deactivating the last unscoped `super_admin` cannot be undone by this script alone
(it will not re-enable a disabled account), so the rollback above is the recovery
path: the operator must be able to run SQL against the SIGAP database. Keep a second
unscoped `super_admin` or a database-level break-glass path before deactivating the
only one.

## 10. Preconditions and non-goals

**Preconditions** (in order): the SIGAP database is up; `0001–0014` migrated; `auth`
schema migrated (`npx auth@latest migrate --config ./apps/web/src/lib/server/auth.better.ts`);
seeds applied (`rbac.sql`, `dev.sql`, `demo.sql`, `regional_tenancy.sql`) so the
`app_users` rows, roles, and scopes exist; `SIGAP_DATABASE_URL` (or
`--from-env-file`) points at the SIGAP database; provisioning runs **before**
registration is opened to the public (§2.2).

**Non-goals:** citizen/patient identities (the backfill script's job — they hold no
staff role and the seed asserts it); password *delivery* (the script sets a
deterministic synthetic password, or `SIGAP_STAFF_PASSWORD`, and never prints it — the
account holder changes it via `POST /api/auth/change-password`, which is enabled in
`auth.better.ts`; a production bootstrap should rotate it out of band); an admin UI
for provisioning; disabling/reactivating accounts as a matter of routine
(`--deactivate` is a one-way offboarding step in this script — reactivation is the
SQL rollback above).

**Evidence anchors for the claims above:** `apps/web/scripts/provision-staff.mjs`
(this script), `apps/web/scripts/backfill-auth-subjects.mjs`,
`apps/web/src/lib/server/auth.better.ts`, `apps/web/src/lib/server/db.ts`,
`apps/web/src/routes/auth/register/+page.server.ts`,
`apps/api/internal/auth/rbac_resolver.go`, `packages/db/migrations/0003,0009,0013`,
`packages/db/seed/{rbac,dev,demo,regional_tenancy}.sql`,
`design/identity-mapping.md`, `design/session-lifecycle-policy.md`,
`design/better-auth-schema.md`, `apps/web/tests/session-lifecycle.test.js`.
