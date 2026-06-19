package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ---------------------------------------------------------------------------
// JWKS fetcher and cache
// ---------------------------------------------------------------------------

// jwksCache stores public keys by Key ID with a TTL.
type jwksCache struct {
	url     string
	client  *http.Client
	mu      sync.RWMutex
	keys    map[string]any // kid -> *rsa.PublicKey | *ecdsa.PublicKey
	expires time.Time
	ttl     time.Duration
}

func newJWKSCache(url string) *jwksCache {
	return &jwksCache{
		url:     url,
		client:  &http.Client{Timeout: 10 * time.Second},
		keys:    make(map[string]any),
		ttl:     15 * time.Minute,
		expires: time.Now().Add(-time.Hour),
	}
}

// jwkEntry is a single key in a JWKS response.
type jwkEntry struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use,omitempty"`
	N   string `json:"n,omitempty"`   // RSA modulus
	E   string `json:"e,omitempty"`   // RSA exponent
	X   string `json:"x,omitempty"`   // EC x coordinate
	Y   string `json:"y,omitempty"`   // EC y coordinate
	Crv string `json:"crv,omitempty"` // EC curve
}

// jwksResponse is the top-level JWKS JSON structure.
type jwksResponse struct {
	Keys []jwkEntry `json:"keys"`
}

// get returns the public key for the given kid, refreshing the cache if stale.
func (c *jwksCache) get(kid string) (any, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	expires := c.expires
	c.mu.RUnlock()

	if ok && time.Now().Before(expires) {
		return key, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if key, ok := c.keys[kid]; ok && time.Now().Before(c.expires) {
		return key, nil
	}

	if err := c.refreshLocked(); err != nil {
		if ok {
			// Serve stale on transient refresh failure
			slog.Warn("jwks refresh failed, serving stale key", "kid", kid, "err", err)
			return key, nil
		}
		return nil, err
	}

	key, ok = c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("jwks: no key found for kid %q", kid)
	}
	return key, nil
}

func (c *jwksCache) refreshLocked() error {
	resp, err := c.client.Get(c.url)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch: HTTP %d", resp.StatusCode)
	}

	var body jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	newKeys := make(map[string]any, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kid == "" {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue // skip encryption keys
		}
		pub, err := parseJWK(k)
		if err != nil {
			slog.Warn("jwks: skipping unparsable key", "kid", k.Kid, "err", err)
			continue
		}
		newKeys[k.Kid] = pub
	}

	c.keys = newKeys
	c.expires = time.Now().Add(c.ttl)
	return nil
}

func parseJWK(k jwkEntry) (any, error) {
	switch k.Kty {
	case "RSA":
		if k.N == "" || k.E == "" {
			return nil, errors.New("RSA key missing n or e")
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("decode n: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("decode e: %w", err)
		}
		return &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}, nil
	case "EC":
		if k.X == "" || k.Y == "" || k.Crv == "" {
			return nil, errors.New("EC key missing x, y, or crv")
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, fmt.Errorf("decode x: %w", err)
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, fmt.Errorf("decode y: %w", err)
		}
		curve, err := ecCurve(k.Crv)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported kty %q", k.Kty)
	}
}

func ecCurve(name string) (elliptic.Curve, error) {
	switch name {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported curve %q", name)
	}
}

// ---------------------------------------------------------------------------
// JWT claims
// ---------------------------------------------------------------------------

// sigapClaims extends jwt.RegisteredClaims with Sigap-specific fields.
type sigapClaims struct {
	jwt.RegisteredClaims
	Permissions []string `json:"permissions,omitempty"`
}

// ---------------------------------------------------------------------------
// JWTProvider (production)
// ---------------------------------------------------------------------------

// JWTProvider validates JWT access tokens against an OIDC issuer JWKS endpoint.
type JWTProvider struct {
	cfg  AuthConfig
	jwks *jwksCache
}

// NewJWTProvider creates a JWT provider with the given configuration.
func NewJWTProvider(cfg AuthConfig) *JWTProvider {
	var cache *jwksCache
	if cfg.JWKSURL != "" {
		cache = newJWKSCache(cfg.JWKSURL)
	}
	return &JWTProvider{cfg: cfg, jwks: cache}
}

// Authenticate extracts and validates a Bearer JWT from the request.
// It enforces: alg ≠ none, signature verification, exp/nbf/iat, iss, aud.
func (p *JWTProvider) Authenticate(r *http.Request) (identity.Actor, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return identity.Actor{}, nil // unauthenticated, not an error
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return identity.Actor{}, fmt.Errorf("invalid authorization header format")
	}
	tokenStr := strings.TrimPrefix(h, prefix)

	token, err := jwt.ParseWithClaims(tokenStr, &sigapClaims{}, p.keyFunc,
		jwt.WithValidMethods(validJWTAlgs()),
		jwt.WithIssuer(p.cfg.Issuer),
		jwt.WithAudience(p.cfg.Audience),
	)
	if err != nil {
		slog.Warn("jwt validation failed", "err", err)
		return identity.Actor{}, nil // fail closed: treat as unauthenticated
	}

	claims, ok := token.Claims.(*sigapClaims)
	if !ok {
		return identity.Actor{}, nil
	}

	if !token.Valid {
		return identity.Actor{}, nil
	}

	// Build actor from validated claims.
	sub := ""
	if claims.Subject != "" {
		sub = claims.Subject
	}

	userType := identity.ActorUser
	// Heuristic: if issuer matches known dev pattern, mark as dev (for testing).
	if claims.Issuer == "sigap-dev" {
		userType = identity.ActorDev
	}

	return identity.Actor{
		UserID:      sub,
		Type:        userType,
		Permissions: claims.Permissions,
	}, nil
}

// keyFunc selects the verification key from the JWKS cache based on the token
// header's kid. Rejects tokens with no kid when JWKS is configured.
func (p *JWTProvider) keyFunc(token *jwt.Token) (any, error) {
	alg := token.Header["alg"]
	if alg == "none" || alg == "" {
		return nil, errors.New("alg=none is not allowed")
	}

	kid, _ := token.Header["kid"].(string)
	if kid == "" && p.jwks != nil {
		return nil, errors.New("missing kid in JWT header")
	}
	if p.jwks == nil {
		return nil, errors.New("no JWKS configured")
	}
	return p.jwks.get(kid)
}

// validJWTAlgs returns the allowed signing algorithm identifiers.
func validJWTAlgs() []string {
	return []string{
		"RS256", "RS384", "RS512",
		"ES256", "ES384", "ES512",
	}
}
