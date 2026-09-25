package identity

import (
	"log/slog"
	"net/http"
	"os"
)

const (
	devIdentityHeader = "X-Sigap-Dev-User-ID"
	devIdentityEnv    = "SIGAP_DEV_IDENTITY"
)

// DevIdentity is a development-only middleware that injects a synthetic Actor
// when the SIGAP_DEV_IDENTITY environment variable equals "true" and the
// request carries the X-Sigap-Dev-User-ID header.
//
// In production this MUST be disabled (SIGAP_DEV_IDENTITY unset or not "true")
// otherwise protected routes will be trivially bypassed.
//
// When active, the middleware logs a loud warning on every request so that
// developers are constantly reminded that dev identity is in use.
func DevIdentity(next http.Handler) http.Handler {
	enabled := os.Getenv(devIdentityEnv) == "true"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enabled {
			devUserID := r.Header.Get(devIdentityHeader)
			if devUserID != "" {
				slog.Warn("dev identity is active — never use in production",
					slog.String("user_id", devUserID),
					slog.String("request_method", r.Method),
					slog.String("request_path", r.URL.Path),
				)
				// actor type=dev
				ctx := ContextWithActor(r.Context(), Actor{
					UserID: devUserID,
					Type:   ActorDev,
					IsDev:  true,
					// Dev identity gets full synthetic permission set for local testing.
					Permissions: []string{
						"queue.generate",
						"queue.read",
						"facility.read",
						"facility.manage",
						"audit.read",
					},
					FacilityGrants: unrestrictedDevGrants(),
				})
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// unrestrictedDevGrants returns the dev synthetic permission set paired with
// unrestricted facility provenance, so a dev actor satisfies facility-scoped
// mutation checks on both axes (permission-at-facility and facility scope).
func unrestrictedDevGrants() []FacilityGrant {
	keys := []string{
		"queue.generate",
		"queue.read",
		"facility.read",
		"facility.manage",
		"audit.read",
	}
	grants := make([]FacilityGrant, 0, len(keys))
	for _, key := range keys {
		grants = append(grants, FacilityGrant{Key: key, Unrestricted: true})
	}
	return grants
}
