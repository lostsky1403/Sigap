package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

type FacilityScopeResult struct {
	IDs          []uuid.UUID
	Unrestricted bool
	Err          error
}

type FacilityScope interface {
	AllowedFacilityIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error)
	CanAccessFacility(ctx context.Context, appUserID string, facilityID uuid.UUID) (bool, error)
}

type FacilityScopeResolver interface {
	ResolveFacilityScope(ctx context.Context, appUserID string) FacilityScopeResult
}

type dbFacilityScope struct {
	pool *pgxpool.Pool
}

func NewDBFacilityScope(pool *pgxpool.Pool) FacilityScope {
	return &dbFacilityScope{pool: pool}
}

const facilityScopeQuery = `
SELECT DISTINCT ur.facility_id,
       (ur.facility_id IS NULL AND r.name = 'super_admin') AS global_super_admin
FROM user_roles ur
JOIN app_users u ON u.id = ur.user_id
JOIN roles r ON r.id = ur.role_id
WHERE u.id = $1
  AND u.status = 'active'
  AND u.deleted_at IS NULL
  AND ur.status = 'active'
  AND ur.deleted_at IS NULL`

// FacilityMutationDecision is the outcome of authorizing a facility-scoped
// mutation. It requires BOTH an in-scope facility AND permission-at-facility
// provenance: holding a key that was granted at a different facility must not
// authorize a mutation here, even when this facility is inside the actor's
// scope.
type FacilityMutationDecision struct {
	// Allowed reports whether the mutation may proceed.
	Allowed bool
	// Reason is a short machine-friendly explanation, safe to log. It is
	// empty when Allowed is true.
	Reason string
}

// AuthorizeFacilityMutation applies the cross-facility privilege rule to a
// single facility-scoped mutation.
//
// It enforces two independent conditions and requires both:
//
//  1. The target facility must be inside the actor's facility scope
//     (auth.FacilityScopeResult), which is what stops out-of-scope resources.
//  2. The actor must hold the required permission key AT that facility, using
//     DB-resolved grant provenance. This is what stops the cross-facility
//     privilege escalation: because user_roles is keyed (user_id, role_id), a
//     viewer at facility A and an operator at facility B for the SAME user
//     yields a flat key union in which queue.manage appears to apply at A.
//
// The dev actor is unrestricted on both axes and is therefore allowed, which
// preserves the local demo flow. Every other fail-closed condition propagates:
// a scope error or an empty scope denies, and an actor with no facility
// provenance denies even when it holds the key flatly (e.g. claims-only).
func AuthorizeFacilityMutation(actor identity.Actor, scope FacilityScopeResult, permission string, facilityID uuid.UUID) FacilityMutationDecision {
	if actor.IsDev {
		return FacilityMutationDecision{Allowed: true}
	}
	if scope.Err != nil {
		return FacilityMutationDecision{Reason: "scope resolution failed"}
	}
	if !FacilityScopeResultAllows(scope, facilityID) {
		return FacilityMutationDecision{Reason: "facility out of scope"}
	}
	if !actor.HasPermissionAtFacility(permission, facilityID) {
		return FacilityMutationDecision{Reason: "permission not granted at facility"}
	}
	return FacilityMutationDecision{Allowed: true}
}

// ---------------------------------------------------------------------------
// Read authorization
//
// Phase 3B0.2 extends the same provenance rule that AuthorizeFacilityMutation
// enforces to every facility-scoped READ. A facility-scoped permission grant
// applies ONLY to the facility attached to that grant, so a key granted at
// facility B must never authorize a read at facility A even when the actor's
// scope happens to contain both.
//
// The two helpers below are the ONLY sanctioned entry points for
// facility-scoped read authorization. Handlers must not re-derive this logic:
// duplicating the scope/provenance intersection per handler is exactly how the
// read path regressed after 3B0.1 fixed the mutation path.
// ---------------------------------------------------------------------------

// FacilityReadDecision is the outcome of authorizing a single facility-scoped
// read of one known facility.
type FacilityReadDecision struct {
	// Allowed reports whether the read may proceed.
	Allowed bool
	// Reason is a short machine-friendly explanation, safe to log. It is
	// empty when Allowed is true.
	Reason string
}

// FacilityReadSet is the authorization result for a facility-scoped LIST read.
//
// Exactly one of the following is meaningful:
//
//   - Unrestricted == true: the actor holds the permission from a global
//     (unscoped) assignment, so every facility row is readable. IDs is
//     ignored.
//   - Unrestricted == false: IDs holds exactly the facilities where BOTH
//     conditions hold — the facility is in the actor's scope AND the
//     permission is granted at that facility. An empty IDs therefore means
//     "authorized for nothing", which callers must render as an empty result
//     (HTTP 200 + []) rather than an error, so row existence never leaks.
type FacilityReadSet struct {
	IDs          []uuid.UUID
	Unrestricted bool
	Err          error
}

// HasFacilities reports whether the read set selects at least one facility.
func (s FacilityReadSet) HasFacilities() bool {
	return s.Err == nil && (s.Unrestricted || len(s.IDs) > 0)
}

// DBFacilityScopeMarker is implemented by the production DB-backed facility
// scope resolver. Handlers use it to detect that scope is resolved live from
// the database, so they can wire the matching live grant resolver and keep
// permission provenance equally live. Test doubles deliberately do not
// implement it, so synthetic scopes keep using the actor snapshot.
type DBFacilityScopeMarker interface {
	IsDBFacilityScope()
}

func (s *dbFacilityScope) IsDBFacilityScope() {}

// FacilityGrantResolver resolves the live, DB-authoritative grant set for an
// actor's app user. It exists so facility-scoped READ authorization observes
// role changes immediately, exactly as the facility SCOPE resolver already
// does.
//
// Why this is required: the actor snapshot on the request context is resolved
// once at authentication time. A grant added or revoked in the database after
// that snapshot must take effect on the next request, otherwise a revoked
// facility read would remain usable until the process re-authenticates, and a
// newly granted one would be wrongly denied. Scope resolution was already
// live; provenance must be live for the same reason.
//
// Implementations MUST fail closed: an unresolvable backend returns an error.
type FacilityGrantResolver interface {
	ResolveFacilityGrants(ctx context.Context, appUserID string) ([]identity.FacilityGrant, error)
}

// dbFacilityGrantResolver resolves grants from the trusted RBAC schema using
// the same query and the same super_admin semantics as the request-time
// permission resolver, so provenance and scope can never disagree.
type dbFacilityGrantResolver struct {
	pool *pgxpool.Pool
}

// NewDBFacilityGrantResolver returns a FacilityGrantResolver backed by pool.
func NewDBFacilityGrantResolver(pool *pgxpool.Pool) FacilityGrantResolver {
	return &dbFacilityGrantResolver{pool: pool}
}

func (r *dbFacilityGrantResolver) ResolveFacilityGrants(ctx context.Context, appUserID string) ([]identity.FacilityGrant, error) {
	if r == nil || r.pool == nil {
		return nil, ErrClosed
	}
	if appUserID == "" {
		return nil, nil
	}
	resolver, ok := NewRBACResolver(r.pool).(AppUserResolver)
	if !ok {
		return nil, ErrClosed
	}
	resolved, err := resolver.ResolveByAppUserID(ctx, appUserID)
	if err != nil {
		return nil, err
	}
	return resolved.Grants, nil
}

// ActorWithLiveGrants returns a copy of actor whose FacilityGrants reflect the
// current database state, so a read decision is made against live RBAC rather
// than a possibly stale authentication-time snapshot.
//
// It deliberately does NOT refresh actor.Permissions: the route-level coarse
// gate keeps using the snapshot, and widening that behavior is out of scope for
// this change. Provenance is what facility-scoped authorization depends on.
//
// A nil resolver, a dev actor, or an actor without an app user id returns the
// actor unchanged. A resolver error is returned so the caller can fail closed.
func ActorWithLiveGrants(ctx context.Context, actor identity.Actor, resolver FacilityGrantResolver) (identity.Actor, error) {
	if actor.IsDev || resolver == nil || actor.AppUserID == "" {
		return actor, nil
	}
	grants, err := resolver.ResolveFacilityGrants(ctx, actor.AppUserID)
	if err != nil {
		return actor, err
	}
	actor.FacilityGrants = grants
	return actor, nil
}

// AuthorizeFacilityRead applies the cross-facility privilege rule to a single
// facility-scoped read whose facility is already known (a detail endpoint, or
// a list narrowed to one facility).
//
// It enforces the same two independent conditions as
// AuthorizeFacilityMutation, with read semantics:
//
//  1. The target facility must be inside the actor's facility scope.
//  2. The actor must hold the required permission key AT that facility.
//
// A nil facilityID is treated as a resource that belongs to no facility and is
// therefore never authorized for a scoped actor; it remains readable only by an
// unrestricted global grant, which mirrors the mutation rule.
func AuthorizeFacilityRead(actor identity.Actor, scope FacilityScopeResult, permission string, facilityID *uuid.UUID) FacilityReadDecision {
	if actor.IsDev {
		return FacilityReadDecision{Allowed: true}
	}
	if scope.Err != nil {
		return FacilityReadDecision{Reason: "scope resolution failed"}
	}
	if facilityID == nil || *facilityID == uuid.Nil {
		if scope.Unrestricted {
			return FacilityReadDecision{Allowed: true}
		}
		return FacilityReadDecision{Reason: "resource has no facility"}
	}
	if !FacilityScopeResultAllows(scope, *facilityID) {
		return FacilityReadDecision{Reason: "facility out of scope"}
	}
	if !actor.HasPermissionAtFacility(permission, *facilityID) {
		return FacilityReadDecision{Reason: "permission not granted at facility"}
	}
	return FacilityReadDecision{Allowed: true}
}

// AuthorizedFacilityIDsForPermission computes the facility set a scoped read may
// observe, which is the INTERSECTION of:
//
//   - the actor's facility scope (which facilities the assignment covers), and
//   - the facilities at which the actor actually holds `permission`.
//
// The intersection is what stops the union bug. An actor who is a viewer at A
// and a notification reader at B must see B notifications only, never A + B,
// because notification.read has no provenance at A.
//
// A global (unscoped) assignment carrying the permission is the only way to
// obtain Unrestricted == true, and it then preserves global read access. Every
// other outcome is fail-closed: a scope error propagates as Err, and an actor
// without matching provenance yields an empty ID set.
func AuthorizedFacilityIDsForPermission(actor identity.Actor, scope FacilityScopeResult, permission string) FacilityReadSet {
	if actor.IsDev {
		return FacilityReadSet{Unrestricted: true}
	}
	if scope.Err != nil {
		return FacilityReadSet{Err: scope.Err}
	}
	if scope.Unrestricted {
		// Unrestricted scope comes from a global super_admin assignment, but
		// scope alone must not grant a read: the global grant must actually
		// carry this permission. HasPermissionAtFacility cannot be used here
		// because it needs a concrete facility, so consult the provenance set
		// directly for an Unrestricted grant of this key.
		for _, grant := range actor.FacilityGrants {
			if grant.Key == permission && grant.Unrestricted {
				return FacilityReadSet{Unrestricted: true}
			}
		}
		return FacilityReadSet{}
	}

	ids := make([]uuid.UUID, 0, len(scope.IDs))
	seen := make(map[uuid.UUID]struct{}, len(scope.IDs))
	for _, id := range scope.IDs {
		if id == uuid.Nil {
			continue
		}
		if !actor.HasPermissionAtFacility(permission, id) {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return FacilityReadSet{IDs: ids}
}

func (s *dbFacilityScope) ResolveFacilityScope(ctx context.Context, appUserID string) FacilityScopeResult {
	if s == nil || s.pool == nil {
		return FacilityScopeResult{Err: ErrClosed}
	}
	if appUserID == "" {
		return FacilityScopeResult{}
	}

	rows, err := s.pool.Query(ctx, facilityScopeQuery, appUserID)
	if err != nil {
		return FacilityScopeResult{Err: fmt.Errorf("resolve facility scope: %w", err)}
	}
	defer rows.Close()

	result := FacilityScopeResult{}
	for rows.Next() {
		var facilityID *uuid.UUID
		var globalSuperAdmin bool
		if err := rows.Scan(&facilityID, &globalSuperAdmin); err != nil {
			return FacilityScopeResult{Err: fmt.Errorf("scan facility scope: %w", err)}
		}
		if globalSuperAdmin {
			result.Unrestricted = true
			continue
		}
		if facilityID != nil {
			result.IDs = append(result.IDs, *facilityID)
		}
	}
	if err := rows.Err(); err != nil {
		return FacilityScopeResult{Err: fmt.Errorf("iterate facility scope: %w", err)}
	}
	return result
}

func (s *dbFacilityScope) AllowedFacilityIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error) {
	result := s.ResolveFacilityScope(ctx, appUserID)
	return result.IDs, result.Err
}

func (s *dbFacilityScope) CanAccessFacility(ctx context.Context, appUserID string, facilityID uuid.UUID) (bool, error) {
	result := s.ResolveFacilityScope(ctx, appUserID)
	if result.Err != nil {
		return false, result.Err
	}
	return FacilityScopeResultAllows(result, facilityID), nil
}

func FacilityScopeForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope) FacilityScopeResult {
	if actor.IsDev {
		return FacilityScopeResult{Unrestricted: true}
	}
	if actor.AppUserID == "" || resolver == nil {
		return FacilityScopeResult{Err: ErrClosed}
	}
	if scopedResolver, ok := resolver.(FacilityScopeResolver); ok {
		return scopedResolver.ResolveFacilityScope(ctx, actor.AppUserID)
	}

	ids, err := resolver.AllowedFacilityIDs(ctx, actor.AppUserID)
	if err != nil {
		return FacilityScopeResult{Err: err}
	}
	return FacilityScopeResult{IDs: ids}
}

func ResolveFacilityScopeForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope) FacilityScopeResult {
	return FacilityScopeForActor(ctx, actor, resolver)
}

func FacilityScopeResultAllows(result FacilityScopeResult, facilityID uuid.UUID) bool {
	if result.Err != nil {
		return false
	}
	if result.Unrestricted {
		return true
	}
	if facilityID == uuid.Nil {
		return false
	}
	for _, id := range result.IDs {
		if id == facilityID {
			return true
		}
	}
	return false
}

func FacilityScopeResultAllowsOptional(result FacilityScopeResult, facilityID *uuid.UUID) bool {
	if result.Err != nil {
		return false
	}
	if result.Unrestricted {
		return true
	}
	if facilityID == nil {
		return false
	}
	return FacilityScopeResultAllows(result, *facilityID)
}

func AllowedFacilityIDsForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope) ([]uuid.UUID, bool, error) {
	result := FacilityScopeForActor(ctx, actor, resolver)
	return result.IDs, result.Unrestricted, result.Err
}

func CanAccessFacilityForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope, facilityID uuid.UUID) bool {
	return FacilityScopeResultAllows(FacilityScopeForActor(ctx, actor, resolver), facilityID)
}

var _ FacilityScope = (*dbFacilityScope)(nil)
var _ FacilityScopeResolver = (*dbFacilityScope)(nil)
