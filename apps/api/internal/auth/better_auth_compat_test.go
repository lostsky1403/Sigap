package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// Better Auth's jwt plugin is pinned to ES256 + JWKS. These values mirror the
// contract in design/better-auth-integration-contract.md: issuer is the web
// origin and audience is the fixed string "authenticated".
const (
	betterAuthIssuer   = "http://127.0.0.1:5173"
	betterAuthKid      = "ba-es256-key"
	betterAuthSub      = "7a1d2c3e-4f5a-4b6c-8d9e-0f1a2b3c4d5e"
	betterAuthAudience = "authenticated"
)

// testECKey generates a P-256 ECDSA keypair, the curve Better Auth uses by
// default for its ES256 JWKS.
func testECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p-256 key: %v", err)
	}
	return key
}

// testECJWKSServer serves a JWKS in the shape Better Auth's /api/auth/jwks
// emits: {"keys":[{"kty":"EC","crv":"P-256","x":..,"y":..,"kid":..}]}.
func testECJWKSServer(t *testing.T, key *ecdsa.PublicKey, kid string) *httptest.Server {
	t.Helper()

	x := base64.RawURLEncoding.EncodeToString(key.X.Bytes())
	y := base64.RawURLEncoding.EncodeToString(key.Y.Bytes())

	jwkSet := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "EC",
				"kid": kid,
				"use": "sig",
				"alg": "ES256",
				"crv": "P-256",
				"x":   x,
				"y":   y,
			},
		},
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwkSet)
	}))
}

// signES256Token creates a JWT string signed with the given P-256 private key.
func signES256Token(t *testing.T, claims jwt.Claims, key *ecdsa.PrivateKey, kid string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	s, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign es256 token: %v", err)
	}
	return s
}

// betterAuthClaims builds a valid Better-Auth-shaped claims set: iss (web
// origin), aud ["authenticated"], sub (UUID), exp, iat, nbf.
func betterAuthClaims() sigapClaims {
	now := time.Now()
	return sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   betterAuthSub,
			Issuer:    betterAuthIssuer,
			Audience:  jwt.ClaimStrings{betterAuthAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
		},
	}
}

// TestJWTProvider_BetterAuthES256 proves the existing production JWTProvider
// validates a Better-Auth-shaped ES256 token with NO production-code change:
//   - ES256 is already whitelisted in validJWTAlgs();
//   - parseJWK already decodes EC/P-256 keys from the JWKS;
//   - the token's sub resolves through the server-side resolver and yields an
//     ActorUser.
func TestJWTProvider_BetterAuthES256(t *testing.T) {
	key := testECKey(t)
	jwksSrv := testECJWKSServer(t, &key.PublicKey, betterAuthKid)
	defer jwksSrv.Close()

	resolver := &fakeResolver{
		perms:  []string{"queue.read"},
		appUID: "local-app-uuid",
	}

	cfg := AuthConfig{
		Mode:     AuthModeJWT,
		Issuer:   betterAuthIssuer,
		Audience: betterAuthAudience,
		JWKSURL:  jwksSrv.URL,
	}
	p := NewJWTProviderWithResolver(cfg, resolver)

	tokenStr := signES256Token(t, betterAuthClaims(), key, betterAuthKid)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	actor, err := p.Authenticate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actor.IsZero() {
		t.Fatal("expected authenticated actor, got zero")
	}
	if actor.UserID != betterAuthSub {
		t.Errorf("subject = %q, want %q", actor.UserID, betterAuthSub)
	}
	if actor.Type != identity.ActorUser {
		t.Errorf("type = %v, want %v", actor.Type, identity.ActorUser)
	}
	if resolver.sub != betterAuthSub {
		t.Errorf("resolver subject = %q, want %q", resolver.sub, betterAuthSub)
	}
	if actor.AppUserID != "local-app-uuid" {
		t.Errorf("appUserID = %q, want %q", actor.AppUserID, "local-app-uuid")
	}
	if !actor.HasPermission("queue.read") {
		t.Error("missing resolver-provided permission queue.read")
	}
}

// TestJWTProvider_BetterAuthES256_Negative asserts every malformed /
// mis-issuer / mis-audience / forged token fails closed (zero actor, nil
// error) rather than panicking or surfacing as a 500.
func TestJWTProvider_BetterAuthES256_Negative(t *testing.T) {
	key := testECKey(t)
	jwksSrv := testECJWKSServer(t, &key.PublicKey, betterAuthKid)
	defer jwksSrv.Close()

	newProvider := func() *JWTProvider {
		return NewJWTProvider(AuthConfig{
			Mode:     AuthModeJWT,
			Issuer:   betterAuthIssuer,
			Audience: betterAuthAudience,
			JWKSURL:  jwksSrv.URL,
		})
	}

	assertFailClosed := func(t *testing.T, tokenStr string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		actor, err := newProvider().Authenticate(req)
		if err != nil {
			t.Fatalf("expected fail-closed (nil error), got error: %v", err)
		}
		if !actor.IsZero() {
			t.Errorf("expected zero actor (unauthenticated), got %+v", actor)
		}
	}

	t.Run("wrong issuer", func(t *testing.T) {
		c := betterAuthClaims()
		c.Issuer = "http://evil.example.com"
		assertFailClosed(t, signES256Token(t, c, key, betterAuthKid))
	})

	t.Run("wrong audience", func(t *testing.T) {
		c := betterAuthClaims()
		c.Audience = jwt.ClaimStrings{"evil-audience"}
		assertFailClosed(t, signES256Token(t, c, key, betterAuthKid))
	})

	t.Run("alg none", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
			Subject:   betterAuthSub,
			Issuer:    betterAuthIssuer,
			Audience:  jwt.ClaimStrings{betterAuthAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		})
		headerJSON, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
		claimsJSON, _ := json.Marshal(tok.Claims)
		tokenStr := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
			base64.RawURLEncoding.EncodeToString(claimsJSON) + "."
		assertFailClosed(t, tokenStr)
	})

	t.Run("tampered signature", func(t *testing.T) {
		otherKey := testECKey(t)
		assertFailClosed(t, signES256Token(t, betterAuthClaims(), otherKey, betterAuthKid))
	})

	t.Run("missing kid", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, betterAuthClaims())
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		assertFailClosed(t, s)
	})
}