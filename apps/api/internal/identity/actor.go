// Package identity provides request-scoped context helpers for authentication,
// authorization, and telemetry. The single non-stdlib dependency is the uuid
// type carried by FacilityGrant, whose layout must match the DB-resolved grant
// set produced by internal/auth.
//
// Import path: github.com/sigap/sigap/apps/api/internal/identity
package identity

import (
	"context"

	"github.com/google/uuid"
)

// ActorType classifies who is performing the request.
type ActorType string

const (
	ActorSystem ActorType = "system"
	ActorUser   ActorType = "user"
	ActorDev    ActorType = "dev"
)

// FacilityGrant is the facility provenance of a single permission key held by
// an actor. It is the answer to "at which facilities does this key actually
// apply?", which a flat key set cannot express: user_roles allows a user to
// hold DIFFERENT roles at DIFFERENT facilities, so a flat union silently
// widens every key to every facility.
//
// Exactly one of the following shapes is meaningful:
//
//   - Unrestricted == true and FacilityID == nil: a global assignment
//     (user_roles.facility_id IS NULL). The key applies at every facility.
//   - Unrestricted == false and FacilityID != nil: a facility-scoped
//     assignment. The key applies at that facility and nowhere else.
//   - Unrestricted == false and FacilityID == nil: the key is known but has no
//     facility provenance. It is deliberately NOT a global grant; a zero
//     (facility_id IS NULL) assignment for a non-super_admin role is scoped to
//     nothing. See HasPermissionAtFacility.
type FacilityGrant struct {
	// Key is the permission key, e.g. "queue.manage".
	Key string
	// FacilityID is the single facility the grant is scoped to, or nil for a
	// global (unscoped) assignment.
	FacilityID *uuid.UUID
	// Unrestricted reports that the key came from a global assignment and
	// therefore applies at every facility.
	Unrestricted bool
}

// Actor represents the authenticated/identified principal for a request.
// Zero value means no actor is present (unauthenticated).
//
// UserID holds the external identity for the actor (from the validated token
// subject or dev header). In JWT mode it MUST NOT be used as an authorization
// source: permissions are resolved server-side. AppUserID is the server-side
// application user id that the subject mapped to (empty when unknown or when
// no DB-backed RBAC is available); it is preserved so facility scope can be
// resolved from trusted state.
type Actor struct {
	UserID string
	Type   ActorType
	// Permissions is the flat, provenance-free key set. It remains the
	// authorization source for non-facility-scoped routes only; facility-scoped
	// mutations MUST use FacilityGrants via HasPermissionAtFacility.
	Permissions []string
	// FacilityGrants is the DB-resolved permission set carrying facility
	// provenance. It is empty for actors whose permissions were not resolved
	// from trusted server-side RBAC (zero, disabled, deleted, resolver error,
	// or a synthetic non-dev provider), which fails closed.
	FacilityGrants []FacilityGrant
	IsDev          bool // true when the request was authenticated via the dev identity header
	AppUserID      string
}

// IsZero reports whether the actor is absent (no identity attached).
func (a Actor) IsZero() bool {
	return a.UserID == "" && a.Type == "" && len(a.Permissions) == 0
}

// HasPermission reports whether the actor holds the named permission key
// anywhere. It ignores facility provenance and MUST NOT be used to authorize a
// facility-scoped mutation: a key granted only at facility B satisfies this
// predicate even when the target resource lives at facility A.
func (a Actor) HasPermission(key string) bool {
	for _, p := range a.Permissions {
		if p == key {
			return true
		}
	}
	return false
}

// HasPermissionAtFacility reports whether the actor holds the named permission
// key at the given facility, using the DB-resolved grant provenance.
//
// A grant satisfies the check when either:
//   - it is a global (unscoped) assignment, which applies everywhere; or
//   - its FacilityID equals facilityID.
//
// A grant carrying neither a facility id nor Unrestricted (a zero-scope
// non-super_admin assignment) never satisfies the check: it grants nothing at
// any facility. The result is fail-closed for actors without provenance.
func (a Actor) HasPermissionAtFacility(key string, facilityID uuid.UUID) bool {
	if facilityID == uuid.Nil {
		return false
	}
	for _, grant := range a.FacilityGrants {
		if grant.Key != key {
			continue
		}
		if grant.Unrestricted {
			return true
		}
		if grant.FacilityID != nil && *grant.FacilityID == facilityID {
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
