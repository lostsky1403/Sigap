package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
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
	p := NewJWTProvider(cfg)

	claims := sigapClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-42",
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"sigap-api"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		Permissions: []string{"queue.read", "facility.manage"},
	}
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
	if !actor.HasPermission("queue.read") {
		t.Error("missing permission queue.read")
	}
	if !actor.HasPermission("facility.manage") {
		t.Error("missing permission facility.manage")
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
