# JWT live-switch configuration (local Supabase Auth) — CORRECTED

The local Supabase Auth is provisioned and running. User access tokens are **ES256
with a `kid`**, so the existing **JWKS** branch of the Go `JWTProvider` validates them
directly — no shared secret, no HS256 bridge.

## Live-verified (this session)
- `supabase start` → Auth `http://127.0.0.1:54321`, Studio `:54323`.
- Two synthetic accounts (`patient-a@/patient-b@sigap.local`) created; their `sub`
  ids backfilled to `app_users.subject`.
- Running the jwt-mode API with **`SIGAP_AUTH_JWKS_URL`** (not a secret):
  - anonymous `GET /api/v1/patient/appointments` → **401**.
  - `Authorization: Bearer <patient-a JWT>` → **200** (empty list), resolving through
    `JWT(sub=100efdef-…) → app_users.subject → Actor.AppUserID → PatientAccountID`.
- Token reality: `{alg: ES256, kid: b81269f1-…, iss: http://127.0.0.1:54321/auth/v1,
  aud: authentic, sub: <account-uuid>}`.

## Correct env (jwt mode via JWKS — no JWT_SECRET)
```
SIGAP_AUTH_MODE=jwt
SIGAP_AUTH_ISSUER=http://127.0.0.1:54321/auth/v1
SIGAP_AUTH_AUDIENCE=authenticated
SIGAP_AUTH_JWKS_URL=http://127.0.0.1:54321/auth/v1/.well-known/jwks.json
SIGAP_ENV=local
SIGAP_DEV_IDENTITY=                # OFF
SIGAP_LOCAL_RBAC_TEST_IDENTITY=    # OFF

# SvelteKit
PUBLIC_SUPABASE_URL=http://127.0.0.1:54321
PUBLIC_SUPABASE_ANON_KEY=<anon key from `supabase status`>
```

## Remaining for the full browser journey
1. Launch the API with the jwt env above (the `Start-V22Preview.ps1` pins `dev` —
   parameterise it or launch the binary directly).
2. Book for `patient-a` at both synthetic facilities, then sign in as patient A in the
   browser and confirm both appear under "Janji Saya"; verify patient B cannot see them.

## Honest state
- REAL_SUPABASE_AUTH=PASS (provisioned + running + accounts + live JWKS validation).
- JWT_VALIDATION=PASS (live ES256 → JWKS → actor → account, 200 on the owned endpoint).
- The HS256 shared-secret bridge was reverted (dead — local Supabase issues ES256; the
  unchanged JWKS-only provider is the single verification path).
- Synthetic dev accounts only; email confirmation is NOT real-world medical identity.