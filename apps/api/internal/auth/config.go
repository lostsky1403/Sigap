package auth

import (
	"fmt"
	"os"
)

// AuthMode controls how authentication is handled by the API gateway.
type AuthMode string

const (
	// AuthModeDev uses DevIdentityProvider for local development only.
	// Enabled only when SIGAP_DEV_IDENTITY=true.
	AuthModeDev AuthMode = "dev"

	// AuthModeJWT uses JWT/OIDC token verification for production.
	// Requires SIGAP_AUTH_ISSUER, SIGAP_AUTH_AUDIENCE, and exactly one
	// of SIGAP_AUTH_JWKS_URL (asymmetric JWKS) or SIGAP_AUTH_JWT_SECRET
	// (HMAC shared secret, local only).
	AuthModeJWT AuthMode = "jwt"

	// AuthModeDisabled disables authentication entirely and fails
	// closed: all requests are treated as unauthenticated.
	AuthModeDisabled AuthMode = "disabled"
)

// IsValid returns true for known mode values.
func (m AuthMode) IsValid() bool {
	switch m {
	case AuthModeDev, AuthModeJWT, AuthModeDisabled:
		return true
	}
	return false
}

// AuthConfig holds all authentication-related configuration parsed from
// environment variables. It is self-validating: calling Validate on a
// zero Config returns an error for every missing required field.
type AuthConfig struct {
	// Mode selects the authentication strategy.
	Mode AuthMode

	// JWT-specific settings (required when Mode == AuthModeJWT).
	Issuer   string // e.g. "https://accounts.google.com"
	Audience string // e.g. "sigap-api-prod"
	JWKSURL  string // e.g. "https://example.com/.well-known/jwks.json"
	// Secret is an HMAC shared secret (HS256/384/512) used as the signing key.
	// It is mutually exclusive with JWKSURL and only honoured when
	// SIGAP_ENV=local (local Supabase Auth). Never log or surface this value.
	Secret   string
}

// LoadConfigFromEnv constructs an AuthConfig from environment variables.
//
// Environment variables:
//   SIGAP_AUTH_MODE      - "dev", "jwt", or "disabled" (default: "disabled")
//   SIGAP_AUTH_ISSUER    - OIDC issuer URL (required for jwt mode)
//   SIGAP_AUTH_AUDIENCE  - expected token audience (required for jwt mode)
//   SIGAP_AUTH_JWKS_URL  - JWKS endpoint URL (one of two key sources for jwt mode)
//   SIGAP_AUTH_JWT_SECRET - HMAC shared secret (alternative key source for jwt
//                           mode; mutually exclusive with SIGAP_AUTH_JWKS_URL
//                           and only accepted when SIGAP_ENV=local)
func LoadConfigFromEnv() (*AuthConfig, error) {
	mode := AuthMode(os.Getenv("SIGAP_AUTH_MODE"))
	if mode == "" {
		mode = AuthModeDisabled
	}

	cfg := &AuthConfig{
		Mode:     mode,
		Issuer:   os.Getenv("SIGAP_AUTH_ISSUER"),
		Audience: os.Getenv("SIGAP_AUTH_AUDIENCE"),
		JWKSURL:  os.Getenv("SIGAP_AUTH_JWKS_URL"),
		Secret:   os.Getenv("SIGAP_AUTH_JWT_SECRET"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate ensures the configuration is internally consistent.
// For dev mode, no extra fields are required.
// For jwt mode, Issuer and Audience must be non-empty and exactly one of
// JWKSURL or Secret must be set (the two key sources are mutually exclusive
// to prevent algorithm confusion). A shared secret is only accepted when
// SIGAP_ENV=local; the JWKS path is allowed in every environment.
// For disabled mode, the config is always valid.
func (c *AuthConfig) Validate() error {
	if !c.Mode.IsValid() {
		return fmt.Errorf("invalid auth mode %q: must be one of dev, jwt, disabled", c.Mode)
	}

	if c.Mode != AuthModeJWT {
		return nil
	}

	if c.Issuer == "" {
		return fmt.Errorf("auth mode is jwt but SIGAP_AUTH_ISSUER is not set")
	}
	if c.Audience == "" {
		return fmt.Errorf("auth mode is jwt but SIGAP_AUTH_AUDIENCE is not set")
	}

	hasJWKS := c.JWKSURL != ""
	hasSecret := c.Secret != ""
	switch {
	case hasJWKS && hasSecret:
		return fmt.Errorf("auth mode is jwt but SIGAP_AUTH_JWKS_URL and SIGAP_AUTH_JWT_SECRET are both set; key sources are mutually exclusive")
	case !hasJWKS && !hasSecret:
		return fmt.Errorf("auth mode is jwt but neither SIGAP_AUTH_JWKS_URL nor SIGAP_AUTH_JWT_SECRET is set; exactly one key source is required")
	case hasSecret && os.Getenv("SIGAP_ENV") != "local":
		return fmt.Errorf("auth mode is jwt with a shared secret (SIGAP_AUTH_JWT_SECRET) but SIGAP_ENV is not \"local\"")
	}

	return nil
}
