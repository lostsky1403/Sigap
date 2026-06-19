// Package identity provides request-scoped context helpers for authentication,
// authorization, and telemetry. All symbols here are stdlib-only.
//
// Import path: github.com/sigap/sigap/apps/api/internal/identity
package identity

import (
	"context"
)

// ActorType classifies who is performing the request.
type ActorType string

const (
	ActorSystem ActorType = "system"
	ActorUser   ActorType = "user"
	ActorDev    ActorType = "dev"
)

// Actor represents the authenticated/identified principal for a request.
// Zero value means no actor is present (unauthenticated).
type Actor struct {
	UserID      string
	Type        ActorType
	Permissions []string // permission keys, e.g. "queue.generate"
	IsDev       bool     // true when the request was authenticated via the dev identity header
}

// IsZero reports whether the actor is absent (no identity attached).
func (a Actor) IsZero() bool {
	return a.UserID == "" && a.Type == "" && len(a.Permissions) == 0
}

// HasPermission reports whether the actor holds the named permission key.
func (a Actor) HasPermission(key string) bool {
	for _, p := range a.Permissions {
		if p == key {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

type actorKey struct{}

// ContextWithActor returns ctx with the given Actor attached.
// Safe to call with a zero Actor (means unauthenticated).
func ContextWithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFromContext returns the Actor stored in ctx. If none is stored,
// returns a zero Actor (IsZero() == true).
func ActorFromContext(ctx context.Context) Actor {
	v, _ := ctx.Value(actorKey{}).(Actor)
	return v
}
