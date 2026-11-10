package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// testRSAKey generates a 2048-bit RSA key for JWT signing in tests.
func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

// testJWKSServer spins up an HTTP server that serves a JWKS containing the
// given RSA public key. Returns the server and the expected kid.
func testJWKSServer(t *testing.T, key *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()

	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())

	jwkSet := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"n":   n,
				"e":   e,
			},
		},
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwkSet)
	}))
}

// signTestToken creates a JWT string signed with the given RSA private key.
func signTestToken(t *testing.T, claims jwt.Claims, key *rsa.PrivateKey, kid string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	s, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// fakeResolver is a Resolver stub for provider unit tests. Results are fixed
// in the test; it records the subject it was queried with.
type fakeResolver struct {
	perms  []string
	appUID string
	err    error
	sub    string
}

func (f *fakeResolver) Resolve(_ context.Context, subject string) (ResolvedPermissions, error) {
	f.sub = subject
	if f.err != nil {
		return ResolvedPermissions{}, f.err
	}
	return ResolvedPermissions{Permissions: f.perms, AppUserID: f.appUID}, nil
}

func TestJWTProvider_ValidToken(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	resolver := &fakeResolver{
		perms:  []string{"queue.read", "facility.manage"},
		appUID: "app-user-uuid",
	}

	// The token MAY carry a "permissions" claim, but those claims are NOT
	// authoritative: permissions come from the server-side resolver (AUDIT-101).
	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		Permissions: []string{"no-claim-permission"},
	}
	p := NewJWTProviderWithResolver(cfg, resolver)
	tokenStr := signTestToken(t, claims, key, "key-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.IsZero() {
		t.Fatal("expected authenticated actor, got zero")
	}
	if actor.UserID != "user-42" {
		t.Errorf("userID = %q, want %q", actor.UserID, "user-42")
	}
	if actor.Type != identity.ActorUser {
		t.Errorf("type = %v, want %v", actor.Type, identity.ActorUser)
	}
	if resolver.sub != "user-42" {
		t.Errorf("resolver subject = %q, want %q", resolver.sub, "user-42")
	}
	if actor.AppUserID != "app-user-uuid" {
		t.Errorf("appUserID = %q, want %q", actor.AppUserID, "app-user-uuid")
	}
	// Permissions come from the resolver, NOT the token claim.
	if actor.HasPermission("no-claim-permission") {
		t.Error("token-claimed permission leaked into actor; must NOT be authoritative")
	}
	if !actor.HasPermission("queue.read") {
		t.Error("missing resolver-provided permission queue.read")
	}
	if !actor.HasPermission("facility.manage") {
		t.Error("missing resolver-provided permission facility.manage")
	}
}

func TestJWTProvider_ExpiredToken(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // expired
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tokenStr := signTestToken(t, claims, key, "key-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for expired token, got %+v", actor)
	}
}

func TestJWTProvider_WrongIssuer(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://evil.example.com", // wrong issuer
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := signTestToken(t, claims, key, "key-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for wrong issuer, got %+v", actor)
	}
}

func TestJWTProvider_WrongAudience(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"other-api"}, // wrong audience
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := signTestToken(t, claims, key, "key-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for wrong audience, got %+v", actor)
	}
}

func TestJWTProvider_AlgNone(t *testing.T) {
	// alg=none token: no signature, just header+claims+empty signature
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   "user-42",
		Issuer:    "https://issuer.example.com",
		Audience:  jwt.ClaimStrings{"sigap-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	// SigningMethodNone requires unsafe allow; we build the token manually.
	headerJSON, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(token.Claims)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	tokenStr := headerB64 + "." + claimsB64 + "."

	// Need a dummy JWKS server so the provider doesn't fail on "no JWKS configured"
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for alg=none, got %+v", actor)
	}
}

func TestJWTProvider_MissingKid(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	// Sign without kid in header
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for missing kid, got %+v", actor)
	}
}

func TestJWTProvider_InvalidSignature(t *testing.T) {
	key := testRSAKey(t)
	otherKey := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	// Sign with a different key than the one in JWKS
	tokenStr := signTestToken(t, claims, otherKey, "key-1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for invalid signature, got %+v", actor)
	}
}

func TestJWTProvider_NoAuthorizationHeader(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for missing header, got %+v", actor)
	}
}

func TestJWTProvider_WrongKid(t *testing.T) {
	key := testRSAKey(t)
	jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
	defer jwksSrv.Close()

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   "https://issuer.example.com",
		Audience: "sigap-api",
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := signTestToken(t, claims, key, "key-999") // wrong kid

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actor.IsZero() {
		t.Errorf("expected zero actor for wrong kid, got %+v", actor)
	}
}

// TestJWTProvider_PermissionsFromResolver verifies the trust boundary for the
// two directions this remediation guarantees:
//
//  1. A token-claimed permission MUST NOT grant access the DB did not grant.
//  2. A DB-granted permission MUST be honoured even when the token carries no
//     permission claims (authorization comes from server-side RBAC, not claims).
func TestJWTProvider_PermissionsFromResolver(t *testing.T) {
	newAuth := func(t *testing.T, resolver Resolver) (*JWTProvider, *rsa.PrivateKey) {
		t.Helper()
		key := testRSAKey(t)
		jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
		t.Cleanup(jwksSrv.Close)
		cfg := AuthConfig{
			Mode:     AuthModeJWT,
			Issuer:   "https://issuer.example.com",
			Audience: "sigap-api",
			JWKSURL:  jwksSrv.URL,
		}
		p := NewJWTProviderWithResolver(cfg, resolver)
		return p, key
	}

	t.Run("forged permission claim denied (valid token)", func(t *testing.T) {
		// DB grants only facility.read. The token forges facility.manage.
		resolver := &fakeResolver{perms: []string{"facility.read"}, appUID: "local-uuid"}
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u1",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Permissions: []string{"facility.manage"}, // forged
		}
		actor := authenticActor(t, p, key, claims)

		if actor.HasPermission("facility.manage") {
			t.Errorf("forged facility.manage leaked into actor; must be denied")
		}
		if !actor.HasPermission("facility.read") {
			t.Errorf("expected DB-granted facility.read, got %v", actor.Permissions)
		}
	})

	t.Run("DB permission honoured when token has no permission claims", func(t *testing.T) {
		// DB grants admin-level permission. Token carries no permission claim.
		resolver := &fakeResolver{perms: []string{"facility.manage"}}
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u2",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			// no Permissions field
		}
		actor := authenticActor(t, p, key, claims)

		if !actor.HasPermission("facility.manage") {
			t.Errorf("expected DB-granted facility.manage, got %v", actor.Permissions)
		}
	})

	t.Run("forged admin role denied (valid token)", func(t *testing.T) {
		// DB says the user is a normal operator. Token forges an admin role.
		resolver := &fakeResolver{perms: []string{"queue.generate", "queue.read"}}
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u3",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Permissions: []string{"facility.manage", "audit.read"}, // forged admin
		}
		actor := authenticActor(t, p, key, claims)

		if actor.HasPermission("facility.manage") {
			t.Errorf("forged admin permission facility.manage leaked into actor; denied")
		}
		if actor.HasPermission("audit.read") {
			t.Errorf("forged admin permission audit.read leaked into actor; denied")
		}
		if !actor.HasPermission("queue.generate") {
			t.Errorf("expected DB-granted queue.generate, got %v", actor.Permissions)
		}
	})

	t.Run("unknown subject fails closed", func(t *testing.T) {
		// Valid token, but the subject is unknown to the DB (empty permissions).
		resolver := &fakeResolver{} // empty -> unknown subject
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "ghost",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Permissions: []string{"facility.manage"}, // forged on an unknown subject
		}
		actor := authenticActor(t, p, key, claims)

		if len(actor.Permissions) != 0 {
			t.Errorf("unknown subject: expected zero permissions (fail closed), got %v", actor.Permissions)
		}
		// Identity is still established, but no authorization is granted.
		if actor.UserID != "ghost" {
			t.Errorf("identity sub = %q, want %q", actor.UserID, "ghost")
		}
	})

	t.Run("DB resolution failure fails closed", func(t *testing.T) {
		// The token is cryptographically valid but the resolver errors (DB down).
		// AuthN must not fall back to the token's forged permissions.
		resolver := &fakeResolver{err: errResolverDown}
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u4",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Permissions: []string{"facility.manage"}, // would be dangerous to fall back to
		}
		actor := authenticActor(t, p, key, claims)

		if actor.IsZero() {
			// Acceptable: the provider treats resolver failure as fail-closed
			// and returns a zero actor so authz denies the request.
			return
		}
		if actor.HasPermission("facility.manage") {
			t.Errorf("failed-closed path leaked token permission; must not happen")
		}
	})

	t.Run("DB role change reflected with same token", func(t *testing.T) {
		// The same subject token is resolved twice. After the DB grants a new
		// permission, the identical token authorizes it — no JWT change needed.
		resolver := &fakeResolver{}
		p, key := newAuth(t, resolver)

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u5",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		tokenStr := signTestToken(t, claims, key, "key-1")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		// Before grant.
		resolver.perms = []string{"facility.read"}
		before, err := p.Authenticate(req)
		if err != nil {
			t.Fatalf("authenticate before: %v", err)
		}
		if !before.HasPermission("facility.read") || before.HasPermission("facility.manage") {
			t.Fatalf("precondition: want only facility.read, got %v", before.Permissions)
		}

		// After DB grant (same token instance, no re-sign).
		resolver.perms = []string{"facility.read", "facility.manage"}
		after, err := p.Authenticate(req)
		if err != nil {
			t.Fatalf("authenticate after: %v", err)
		}
		if !after.HasPermission("facility.manage") {
			t.Errorf("DB grant not reflected with same JWT: got %v", after.Permissions)
		}
	})

	t.Run("provider without resolver fails closed to token claims", func(t *testing.T) {
		// Even with a valid token, a provider with no DB-backed resolver must
		// NOT trust the token's permission claims (defense in depth).
		key := testRSAKey(t)
		jwksSrv := testJWKSServer(t, &key.PublicKey, "key-1")
		defer jwksSrv.Close()
		cfg := AuthConfig{
			Mode:     AuthModeJWT,
			Issuer:   "https://issuer.example.com",
			Audience: "sigap-api",
			JWKSURL:  jwksSrv.URL,
		}
		p := NewJWTProvider(cfg) // no resolver

		claims := sigapClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u6",
				Issuer:    "https://issuer.example.com",
				Audience:  jwt.ClaimStrings{"sigap-api"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Permissions: []string{"facility.manage"},
		}
		actor := authenticActor(t, p, key, claims)
		if actor.HasPermission("facility.manage") {
			t.Errorf("no-resolver provider leaked token claim; must fail closed")
		}
	})
}

// authenticActor builds a signed request for the given claims and returns the
// authenticated actor. It fails the test if the token is rejected outright.
func authenticActor(t *testing.T, p *JWTProvider, key *rsa.PrivateKey, claims jwt.Claims) identity.Actor {
	t.Helper()
	tokenStr := signTestToken(t, claims, key, "key-1")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return actor
}

var errResolverDown = errors.New("resolver backend unavailable")
