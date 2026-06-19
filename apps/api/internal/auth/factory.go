package auth

// NewProvider selects and constructs the appropriate auth Provider based on
// the configured AuthMode. The caller must have validated cfg beforehand
// (e.g., via LoadConfigFromEnv which calls Validate internally).
func NewProvider(cfg *AuthConfig) Provider {
	switch cfg.Mode {
	case AuthModeDev:
		return NewDevIdentityProvider()
	case AuthModeJWT:
		return NewJWTProvider(*cfg)
	case AuthModeDisabled:
		return nil // Middleware is a pass-through; authz layer denies by default.
	}
	return nil
}
