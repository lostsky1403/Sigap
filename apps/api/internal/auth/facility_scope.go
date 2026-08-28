package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// FacilityScope answers the "where" question of authorization: which facility
// UUIDs is a given application user permitted to operate within?
//
// Permission answer: "What may this user do?"
// Facility answer:   "Where may this user do it?"
//
// Both must pass. FacilityScope is intentionally separate from permission
// resolution: a user may hold a permission key (e.g. facility.read) but still
// be denied if the resource's facility is outside their active role assignments.
type FacilityScope interface {
	// AllowedFacilityIDs returns the set of facility UUIDs the given app user
	// is permitted to access, derived from active (status='active',
	// deleted_at IS NULL) user_roles assignments. Returns an empty slice when
	// the user has no facility assignments, when the subject is unknown, or
	// when the resolver fails closed.
	AllowedFacilityIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error)

	// CanAccessFacility reports whether the given app user is permitted to
	// access the specified facility. Returns false on any error (fail closed)
	// so callers may simply gate on the result.
	CanAccessFacility(ctx context.Context, appUserID string, facilityID uuid.UUID) (bool, error)
}

// dbFacilityScope resolves facility scope from the trusted PostgreSQL RBAC
// schema. It reads user_roles with active/lifecycle filters joined to app_users
// by the server-side AppUserID (never from request parameters).
type dbFacilityScope struct {
	pool *pgxpool.Pool
}

// NewDBFacilityScope returns a FacilityScope backed by the given PostgreSQL pool.
func NewDBFacilityScope(pool *pgxpool.Pool) FacilityScope {
	return &dbFacilityScope{pool: pool}
}

// allowedFacilityIDsQuery reads the distinct facility_id values for a given app
// user where the role assignment is active and not soft-deleted. A NULL
// facility_id (global role) is excluded: global roles convey no facility scope.
const allowedFacilityIDsQuery = `
SELECT DISTINCT ur.facility_id
FROM user_roles ur
JOIN app_users u ON u.id = ur.user_id
WHERE u.id = $1
  AND ur.status = 'active'
  AND ur.deleted_at IS NULL
  AND ur.facility_id IS NOT NULL`

// AllowedFacilityIDs implements FacilityScope.
func (s *dbFacilityScope) AllowedFacilityIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error) {
	if s == nil || s.pool == nil {
		return nil, ErrClosed
	}
	if appUserID == "" {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, allowedFacilityIDsQuery, appUserID)
	if err != nil {
		return nil, fmt.Errorf("query allowed facility IDs: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan facility id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate allowed facility IDs: %w", err)
	}

	return ids, nil
}

// CanAccessFacility implements FacilityScope.
func (s *dbFacilityScope) CanAccessFacility(ctx context.Context, appUserID string, facilityID uuid.UUID) (bool, error) {
	if s == nil || s.pool == nil {
		return false, ErrClosed
	}
	if appUserID == "" {
		return false, nil
	}

	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN app_users u ON u.id = ur.user_id
			WHERE u.id = $1
			  AND ur.facility_id = $2
			  AND ur.status = 'active'
			  AND ur.deleted_at IS NULL
		)`, appUserID, facilityID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check facility access: %w", err)
	}
	return exists, nil
}

// AllowedFacilityIDsForActor is the centralized, fail-closed helper that
// handlers must use for all scope resolution. It returns:
//   - ids: the facility UUIDs the actor may access (may be empty)
//   - unrestricted: true only when actor.IsDev == true (dev bypass)
//   - err: non-nil when scope resolution failed (fail closed → deny all)
//
// Dev bypass is ONLY permitted because PR #40 guarantees dev identity cannot
// run outside SIGAP_ENV=local. There is no bypass for non-dev actors.
func AllowedFacilityIDsForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope) ([]uuid.UUID, bool, error) {
	// Dev mode bypass: local synthetic identity has no DB-backed role
	// assignments. PR #40 ensures dev identity is unavailable outside local.
	if actor.IsDev {
		return nil, true, nil
	}

	// No server-side app user id → cannot resolve facility scope → fail closed.
	if actor.AppUserID == "" {
		return nil, false, ErrClosed
	}

	if resolver == nil {
		return nil, false, ErrClosed
	}

	ids, err := resolver.AllowedFacilityIDs(ctx, actor.AppUserID)
	if err != nil {
		return nil, false, err
	}

	return ids, false, nil
}

// CanAccessFacilityForActor is the centralized helper for single-resource
// ownership checks (detail/mutation endpoints). Returns true if the actor is
// permitted to access the given facility. Mirrors AllowedFacilityIDsForActor:
// dev bypass only, nil/empty AppUserID fails closed.
func CanAccessFacilityForActor(ctx context.Context, actor identity.Actor, resolver FacilityScope, facilityID uuid.UUID) bool {
	if actor.IsDev {
		return true
	}
	if actor.AppUserID == "" || resolver == nil {
		return false
	}

	ok, err := resolver.CanAccessFacility(ctx, actor.AppUserID, facilityID)
	if err != nil {
		return false
	}
	return ok
}

var _ FacilityScope = (*dbFacilityScope)(nil)
