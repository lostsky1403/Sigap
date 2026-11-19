# Identity mapping — provider → subject → app_users

**Status:** Design / strategy. Pins the explicit subject-mapping rule for the
Supabase → Better Auth cutover (branch `feat/sigap-selfhosted-auth`). No schema is
changed here; the only runtime data step is the subject **backfill** in
`apps/web/scripts/backfill-auth-subjects.mjs`.

**Date:** 2026-11-11

---

## 1. The four-layer model

End-to-end, every authenticated request resolves identity through four layers:

```
(1) auth provider                  Better Auth (self-hosted)
        │  token (JWT): iss + sub  ← verified by Go JWTProvider
        ▼
(2) verified issuer + subject      JWT `iss` (must equal SIGAP_AUTH_ISSUER)
                                   JWT `sub` = Better Auth `auth.user.id`
        │  app_users.subject = sub (the mapping written by the backfill)
        ▼
(3) SIGAP app_users.id             the STABLE internal id (our UUID)
        │  rbac_resolver.Resolve(sub) → AppUserID = app_users.id
        ▼
(4) patient_account / RBAC roles   citizen → Actor.PatientAccountID()
                                   staff   → roles + facility/org grants
```

- **Layer 1 → 2** is verified cryptographically by `apps/api/internal/auth/jwt_provider.go`
  (`WithIssuer`, `WithAudience`, `WithValidMethods`, JWKS `kid` lookup, `alg=none`
  rejected). Token `permissions` claims are ignored (AUDIT-101); authorization is
  never derived from the token.
- **Layer 2 → 3** is the join key `WHERE u.subject = $1`
  (`apps/api/internal/auth/rbac_resolver.go:106`). This is the **only** place the
  provider subject is consumed.
- **Layer 3 → 4** is pure SIGAP state: `app_users.id` flows into `AppUserID` →
  `PatientAccountID()` for a grant-free citizen (`identity/patient_account.go:17-39`),
  or into `user_roles`/`role_permissions` grants for staff.

## 2. Two ids, two different lifetimes

| | `app_users.subject` | `app_users.id` |
|---|---|---|
| Who mints it | the **vendor** (Supabase `auth.users.id`, then Better Auth `auth.user.id`) | SIGAP (`0003_identity_rbac.sql`), an app-owned UUID |
| Lifetime | **changes with the vendor** — a Supabase→Better Auth swap rewrites it | **never changes** — independent of any auth vendor |
| Referenced by | the unique partial index `uq_app_users_subject_active` (`0008`) | FKs everywhere: `audit_events.actor_user_id`, `appointments.patient_account_id` (0014), `user_roles.user_id` |
| Role | proof target — "what the verified token names" | the durable row that RBAC/audit/patient data hang off |

**Rule:** `app_users.subject` is ALWAYS the vendor id (the token's `sub`). It is a
pointer that is rewritten when the vendor changes. `app_users.id` is the stable
internal id and is **never** re-written or re-assigned — that is what keeps every
foreign key, audit row, role grant and patient appointment valid across the cutover.

## 3. Explicit mapping — NEVER email auto-link

The backfill must **not** derive an `app_user ↔ subject` link from email equality.
It binds each `app_users.id` to a Better Auth `user.id` from a **reviewed,
hard-coded list** and writes `subject` by `id`:

```sql
UPDATE app_users SET subject = $betterAuthUserId WHERE id = $appUserId;  -- id, never email
```

Why email equality is forbidden as a linking rule:

- **Email is a self-asserted claim**, not the token-proven identity. The verified
  token proves `sub`, never an email.
- **Email ownership is transferable.** Auto-linking by email would silently re-point
  `app_users.id` at whatever Better Auth account currently holds that address — a
  hostile account-takeover primitive and a violation of the fail-closed subject seam.
- **A vendor can mint duplicate emails across providers.** Only `sub` is unique and
  token-proven; the resolver keys on `subject`, so the mapping must be written at that
  exact key.

Concretely, the backfill lists `{ email, appUserId }` pairs explicitly (the
synthetic citizen + staff fixtures) and does the `app_users` write strictly by
`WHERE id`. The email is used **only** to locate/create the corresponding `auth.user`
row (which is, by Better Auth's own schema, keyed on email); it is never the join key.

## 4. Supabase → Better Auth migration (the one data step)

Per account that must keep authenticating:

1. **Create** a Better Auth user with the **same email** (and synthetic password) —
   `auth.api.signUpEmail({ body: { email, password, name } })`, idempotently reusing
   the existing `auth.user` row if one is already present.
2. **Write** `app_users.subject = <better-auth user.id>`, overwriting the prior
   Supabase `auth.users.id`.
3. **Leave untouched** `app_users.id`, `patient_account`, `user_roles`,
   `role_permissions` and the `0001–0014` migration ledger (forward-only).

`app_users.id` therefore never moves; `patient_account` rows (keyed on
`appointments.patient_account_id REFERENCES app_users(id)`) and every RBAC grant stay
valid. Only the outward pointer (`subject`) changes, so the Go resolver maps the new
`sub` to the exact same internal actor.

## 5. Rollback note

Because the backfill changes **only** `app_users.subject` (and creates isolated,
non-authoritative `auth.*` rows), rollback is surgical:

1. **Before** applying the cutover, snapshot `app_users.id → old subject`
   (the Supabase `auth.users.id`) — the backfill script prints the old→new subject
   column for exactly this purpose.
2. To roll back: re-point `SIGAP_AUTH_ISSUER` / `SIGAP_AUTH_AUDIENCE` /
   `SIGAP_AUTH_JWKS_URL` at Supabase again, and `UPDATE app_users SET subject =
   <snapshotted old id>` for every affected row.
3. Better Auth rows may stay (they authorise nothing once `subject` has been restored)
   or the `auth` schema can be dropped — neither path touches `app_users.id`,
   `patient_account`, or RBAC state.

No step in either direction mutates the migration ledger (`0001–0014`), `app_users.id`,
roles, grants, or the Go trust boundary itself — only `subject` values and env wiring
move.