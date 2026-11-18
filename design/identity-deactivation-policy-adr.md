# ADR — Identity deactivation & revocation policy for SIGAP self-hosted auth

**Status:** PROPOSED — owner decision required. Not implemented by this review.
**Date:** 2026-11-18
**Scope:** `feat/sigap-selfhosted-auth` (Draft PR #97), head `cff2d35…`.
**Related:** `design/session-lifecycle-policy.md` (normative clocks/§3 gap),
`design/auth-gap-matrix-v2.md` (gap classification), `docs/operations/git-attribution.md`.

---

## 1. Verified current behaviour (the problem statement)

The following is confirmed from source, not assumed:

1. **Deactivation truth lives in the Go layer.** `public.app_users.status = 'active'`
   AND `deleted_at IS NULL`, plus active role assignments, are re-resolved from
   PostgreSQL on **every** API request by
   `apps/api/internal/auth/rbac_resolver.go`. Removing a role, disabling an
   assignment, deactivating or soft-deleting an `app_users` row therefore takes
   effect on the **next** request — privilege revocation latency is **0 s**.
2. **The Better Auth identity layer does not refuse session creation for a
   deactivated account.** better-auth 1.7.20 base config has no `banned`/
   `status` field on `auth.user` (those arrive only with the uninstalled admin
   plugin), and `user.validateUserInfo` is skipped for email/password sign-in.
   A disabled identity still signs in, receives a session and a JWT; Go then
   resolves zero permissions for it. This is the OPEN product-policy gap that
   keeps `SESSION_LIFECYCLE=PARTIAL`.
3. **A revoked Better Auth session is not a revoked JWT.** Sign-out /
   revoke deletes the `auth.session` row, so `hooks.server.ts` stops minting
   new Bearers immediately. An already-issued ES256 JWT keeps verifying at the
   Go API until `exp`, because the validator checks signature, `iss`, `aud`,
   `exp` only (`jwt_provider.go`, zero leeway) and ignores the token's
   `permissions` claim (AUDIT-101). The accepted bound is the 1 h JWT TTL.
4. **JWKS rotation is pinned at 30 days** with a 15-minute Go cache bound;
   issuer/audience/expiry/rotation negatives are covered in
   `better_auth_compat_test.go`.
5. Two historical Strix findings that intersect this area are already
   remediated in this head: the dev-identity header grants only read-only
   permissions and is production-blocked by envguard; patient status lookup is
   keyed only by the random 6-char `checkin_code`, never by sequential
   `formatted_number`.

**Consequence:** an offboarded staff member's *authority* ends immediately, but
their already-held token is still *accepted as an identity* for up to 1 hour,
and their account can still produce a fresh session/JWT at the identity layer
until a hard gate exists. Both facts must be stated in any security attestation.

---

## 2. Decision to be made by the owner

> Should a deactivated SIGAP identity see (A) a technically successful sign-in
> that authorizes nothing — today's behaviour — or (B) an explicit
> authentication error at sign-in / session creation?

Option B closes the identity-layer gap but reveals provisioning state (an
account-existence/stdeactivation oracle) and requires a policy choice about
error wording, enumeration resistance and audit. Option A preserves the
anti-enumeration posture and relies on the immediate Go RBAC control. This ADR
**recommends** a specific coherent Option B design below; it does not enact it.

---

## 3. Options compared

| # | Option | Boundary enforced | New JWTs for disabled identity | Existing JWT latency | Staff deactivation | Multi-facility role change | Migration | Verdict |
|---|--------|-------------------|-------------------------------|---------------------|--------------------|----------------------------|-----------|---------|
| 0 | Status quo (Go RBAC only) | Authorization only | Still minted | ≤1 h (identity), 0 s (privilege) | Immediate for authority; identity still accepted | Immediate (re-resolved) | none | Compensating control works; product gap stays OPEN. Recommended only if owner accepts the oracle trade-off. |
| 1 | `databaseHooks.session.create.before` SQL gate in `auth.better.ts` | Identity + session creation | Refused at creation | still ≤1 h for *already-issued* tokens | Immediate at creation for staff with an `app_users` row | unchanged | none | **Recommended.** Smallest change; fail-open for identities without an `app_users` row (citizens), fail-closed for provisioned staff; keeps all clocks. |
| 2 | Better Auth admin plugin (`banned` / `banReason` / `banExpires`) | Identity layer, native | Refused natively; expiry supported | still ≤1 h | Tied to `auth.user.banned`, must be synced with `app_users.status` | unchanged | **Yes** — `auth.user` schema change + dual-source-of-truth sync | Heavier; two deactivation sources to keep consistent. Reject unless native ban expiry is required. |
| 3 | Server-side session introspection in Go (every request checks `auth.session`) | Identity, zero-lag | n/a | 0 s (no JWT accepted after revoke) | Immediate | Immediate | none | Strongest, but couples the API to the web tier's session store and puts the web tier in every API request path. Reject for now; revisit if the 1 h bound is ever unacceptable. |
| 4 | Shorten JWT TTL to 15 m | Identity (bounded tighter) | Still minted | ≤15 m | 0 s privilege / ≤15 m identity | Immediate | none | Cheap complementary lever, not a deactivation gate. Can be layered on Option 1 if the owner wants a shorter identity bound. |

**Recommendation: Option 1, with Option 4 as an optional complementary lever;
explicitly reject Options 2 and 3 for this milestone.**

---

## 4. Recommended design (Option 1, owner-gated)

### 4.1 Identity-layer account disabling
Add a `databaseHooks.session.create.before` hook in
`apps/web/src/lib/server/auth.better.ts`. Before a session row is created,
query `public.app_users` by the auth user id as `subject`:

```sql
SELECT 1 FROM public.app_users
WHERE subject = $1 AND (status <> 'active' OR deleted_at IS NOT NULL);
```

- **Fail-closed** for provisioned staff (an `app_users` row exists and is not
  active) → throw, so no session is created.
- **Fail-open** for identities with no `app_users` row (citizen/patient
  accounts provisioned outside staff provisioning) → session creation proceeds,
  matching today's citizen behaviour.
- The check must run against the same DB the Go resolver reads; never a cached
  copy.

### 4.2 Existing Better Auth session invalidation
On staff deactivation, the deactivation operation (whichever admin surface
owns it) must also delete the user's `auth.session` rows. There is no new
session to block after 4.1, but existing sessions must be revoked so
`hooks.server.ts` stops minting. Reuse the existing Better Auth
`revoke-sessions` mechanism; do not invent a parallel store.

### 4.3 Previously issued JWTs
Unchanged: they remain valid to `exp` (≤1 h). Enforced by signature/iss/aud/exp
only; `permissions` claim ignored; RBAC re-resolved from DB. The web tier
stops minting the moment the session row is gone (4.2). Documented as the
accepted bound — do not claim zero-lag identity revocation.

### 4.4 Maximum revocation latency
- Privilege / facility / role changes: **0 s** (next API request).
- Identity acceptance of a pre-revocation token: **≤1 h** (JWT TTL, zero
  leeway). Optional lever: lower `jwt.expirationTime` to 15 m (Option 4).

### 4.5 Staff deactivation
Deactivation remains a single authoritative act on `app_users.status` /
`deleted_at`. The 4.1 hook makes the identity tier agree; the Go resolver stays
the authorization authority. There must be **one** source of truth
(`app_users`), not a second `auth.user.banned` flag.

### 4.6 Multi-facility role changes
No code change: facility/org scope is resolved per request from `user_roles`
and its `facility_id`/`organization_id`, with deny-by-default. A role change or
assignment disable therefore also takes effect on the next request. Regression
tests must keep covering cross-facility denial and sibling-org 404s.

### 4.7 Audit events
Deactivation, session revocation and the hook's refusal must produce an audit
record. Note the existing OPEN owner decision (gap 9): Better Auth emits no
event the Go hash-chained `audit_events` table can see. Until an audit owner/
chain is chosen, the available record is the `auth.session` rows plus the
treatment in 4.1; this ADR does not pretend the Go audit sink is wired.

### 4.8 Recovery and reactivation
Reactivation = set `app_users.status='active'`, clear `deleted_at`, restore the
needed role assignments. The 4.1 hook then permits new sessions again. No
automatic re-issue of a previously revoked JWT — the user signs in afresh.
Record reactivation as an audit event (subject to 4.7).

### 4.9 MFA implications
`STAFF_MFA=PARTIAL`: enrol/verify/backup-code sign-in is proven, but
`/two-factor/disable`, `/generate-backup-codes`, lockout ceiling, token-expiry
clocks and MFA recovery remain unverified. A deactivation gate at session
creation does **not** by itself revoke TOTP secrets or backup codes; after
reactivation those may still be usable. Any MFA recovery policy must be decided
together with this ADR. Identity-layer deactivation does not weaken any MFA
assertion.

---

## 5. Regression tests required before this could be accepted
1. Disabled `app_users` row → session creation refused by the 4.1 hook; assert
   no `auth.session` row and no JWT minted.
2. Identity with no `app_users` row → session still created (fail-open citizen
   path).
3. Deactivation with an existing session → `revoke-sessions` removes the row;
   replayed signed cookie rejected (`get-session` null, `/token` 401).
4. Go RBAC still denies a disabled subject (existing `rbac_resolver_test.go`
   negatives remain green).
5. Cross-facility / sibling-org denial unchanged after role changes.
6. Reactivation → new session allowed; old revoked JWT not resurrected.

---

## 6. What this ADR does NOT do
- No code change, no commit, no push, no deployment.
- No change to `auth.user` schema (Option 2 rejected).
- No shortening of JWT TTL (Option 4 optional, owner-gated).
- No claim that `SESSION_LIFECYCLE` or `STAFF_MFA` is now READY — both stay
  **PARTIAL** until the owner decision and the tests above land.

**Owner decision needed:** choose 0 / 1 / 4 and approve error-wording and audit
treatment. Until then `IDENTITY_DEACTIVATION_POLICY=OPEN`.
