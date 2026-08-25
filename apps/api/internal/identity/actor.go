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
//
// UserID holds the external identity for the actor (from the validated token
// subject or dev header). In JWT mode it MUST NOT be used as an authorization
// source: permissions are resolved server-side. AppUserID is the server-side
// application user id that the subject mapped to (empty when unknown or when
// no DB-backed RBAC is available); it is preserved so a later facility-scope
// PR (AUDIT-202) can resolve user_roles.facility_id from trusted state.
type Actor struct {
	UserID      string
	Type        ActorType
	Permissions []string // permission keys, e.g. "queue.generate"; zero/empty is fail-closed
	IsDev       bool     // true when the request was authenticated via the dev identity header
	AppUserID   string   // server-side app user UUID the external subject mapped to
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
