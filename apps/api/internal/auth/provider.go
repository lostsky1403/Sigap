// Package auth provides production-minded authentication abstractions for the
// Sigap API gateway. It defines provider interfaces that separate the mechanism
// of identity verification from the authorization and audit layers.
//
// Design goals:
//   - Pluggable providers: dev identity, JWT/OIDC, disabled/fail-closed.
//   - No custom password storage.
//   - All implementations must be testable without real network calls.
//
// Import path: github.com/sigap/sigap/apps/api/internal/auth
package auth

import (
	"net/http"

	"github.com/sigap/sigap/apps/api/internal/identity"
)

// Provider is the abstraction for extracting an identity.Actor from an HTTP
// request. Implementations decide how to authenticate — dev header, JWT
// verification, or synthetic system identity — and return a zero Actor when
// no authentication is present.
type Provider interface {
	// Authenticate extracts an Actor from the request. A zero Actor
	// (Actor.IsZero() == true) means the request carried no recognized
	// authentication.
	//
	// Errors are reserved for unexpected internal failures (e.g., JWKS
	// fetch failure). Authentication rejection should return a zero Actor
	// and nil error so the authz layer can render the correct 401/403.
	Authenticate(r *http.Request) (identity.Actor, error)
}

// Middleware wraps an http.Handler so that every incoming request carries an
// Actor in context. The Provider decides what that Actor is.
// If p is nil, the middleware is a no-op pass-through.
func Middleware(p Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}
			actor, err := p.Authenticate(r)
			if err != nil {
				// Best-effort: log and continue with zero actor.
				// The authz layer will reject the request later.
				actor = identity.Actor{}
			}
			ctx := identity.ContextWithActor(r.Context(), actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
