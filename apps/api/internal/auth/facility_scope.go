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
