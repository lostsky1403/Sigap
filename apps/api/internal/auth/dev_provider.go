package auth

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

const (
	devIdentityHeader = "X-Sigap-Dev-User-ID"
	devIdentityEnv    = "SIGAP_DEV_IDENTITY"
)

// DevIdentityProvider is a development-only auth provider that injects a
// synthetic Actor when the SIGAP_DEV_IDENTITY environment variable equals
// "true" and the request carries the X-Sigap-Dev-User-ID header.
//
// In production this MUST be disabled (SIGAP_DEV_IDENTITY unset or not
// "true") otherwise protected routes will be trivially bypassed.
//
// When active, the provider logs a loud warning on every authenticated
// request so that developers are constantly reminded that dev identity is
// in use.
type DevIdentityProvider struct {
	enabled bool
}

// NewDevIdentityProvider creates a DevIdentityProvider reading the
// SIGAP_DEV_IDENTITY environment variable once at construction time.
func NewDevIdentityProvider() *DevIdentityProvider {
	return &DevIdentityProvider{
		enabled: os.Getenv(devIdentityEnv) == "true",
	}
}

// Authenticate implements the Provider interface for dev identity.
func (p *DevIdentityProvider) Authenticate(r *http.Request) (identity.Actor, error) {
	if !p.enabled {
		return identity.Actor{}, nil
	}
	devUserID := r.Header.Get(devIdentityHeader)
	if devUserID == "" {
		return identity.Actor{}, nil
	}
	slog.Warn("dev identity is active — never use in production",
		slog.String("user_id", devUserID),
		slog.String("request_method", r.Method),
		slog.String("request_path", r.URL.Path),
	)
	return identity.Actor{
		UserID: devUserID,
		Type:   identity.ActorDev,
		IsDev:  true,
		// Dev identity gets full synthetic permission set for local testing.
		Permissions: []string{
			"queue.generate",
			"queue.read",
			"facility.read",
			"facility.manage",
			"audit.read",
			"notification.read",
			"notification.manage",
		},
	}, nil
}
