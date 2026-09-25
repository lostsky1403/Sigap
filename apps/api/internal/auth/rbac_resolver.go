package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// ResolvedPermissions is the result of resolving an external identity subject
// against the server-side RBAC state. It carries the effective permission key
// set, the per-key facility provenance of that set, plus the server-side
// application user the subject mapped to.
type ResolvedPermissions struct {
	// Permissions is the distinct set of permission keys granted to the subject
	// through its roles. It is empty when the subject is unknown, disabled, or
	// soft-deleted (fail closed).
	//
	// It is a flat union and therefore deliberately NOT sufficient to
	// authorize a facility-scoped mutation. Use Grants for that.
	Permissions []string
	// Grants is the same grant set with facility provenance attached: one entry
	// per (key, facility) pair the subject holds. A global assignment
	// (facility_id IS NULL) appears once with Unrestricted set. It is empty
	// under exactly the same fail-closed conditions as Permissions.
	Grants []identity.FacilityGrant
	// AppUserID is the server-side app_users.id the subject mapped to. It is
	// empty when the subject is unknown. It is surfaced so facility scope can
	// be resolved from the same trusted server-side state.
	AppUserID string
}

// Resolver resolves an external identity subject (JWT `sub`) to the
// server-side authorization state. Implementations MUST fail closed: an
// unknown/disabled subject returns empty permissions (not an error), while an
// unresolvable backend returns an error so the caller may treat the request as
// unauthenticated rather than fall back to any token-claimed permissions.
type Resolver interface {
	// Resolve returns the effective permissions and server-side app user id for
	// the given external subject. A non-nil error indicates the resolver could
	// not consult its backend (e.g. database unavailable); the caller should
	// fail closed and NOT fall back to JWT claims.
	Resolve(ctx context.Context, subject string) (ResolvedPermissions, error)
}

// ErrClosed indicates that no permissions could be resolved. It is used to
// make fail-closed behavior explicit where a caller needs to distinguish a
// closed/empty result from a real backend failure.
var ErrClosed = errors.New("permissions resolution is closed/failed closed")

// rbacResolver resolves permissions from the trusted database RBAC schema
// (app_users -> user_roles -> role_permissions -> permissions) with a single
// join query keyed by the external subject. It honors active/disabled status
// and soft-deletion. No caching is used: correctness and immediacy of role
// changes are preferred for this first secure implementation.
type rbacResolver struct {
	pool *pgxpool.Pool
}

// NewRBACResolver returns a Resolver backed by the given PostgreSQL pool.
func NewRBACResolver(pool *pgxpool.Pool) Resolver {
	return &rbacResolver{pool: pool}
}

// permissionsQuery resolves a subject's effective permission keys plus the
// app user id in one query. The app user id (id, status, deleted_at) is always
// read for the subject; when no active user exists, no permission rows are
// returned and the app user id is empty (fail closed).
//
// The role name and user_roles.facility_id are selected alongside the key
// because facility provenance is the whole point: user_roles is keyed
// (user_id, role_id), so one subject legitimately holds DIFFERENT roles at
// DIFFERENT facilities. Returning only p.key collapses that into a flat union
// in which every key appears to apply everywhere.
const permissionsQuery = `
SELECT u.id,
       p.key,
       ur.facility_id,
       r.name
FROM app_users u
LEFT JOIN user_roles ur
  ON ur.user_id = u.id
 AND ur.status = 'active'
 AND ur.deleted_at IS NULL
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = ur.role_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.subject = $1
  AND u.status = 'active'
  AND u.deleted_at IS NULL
ORDER BY p.key, ur.facility_id NULLS FIRST, r.name`

// Resolve implements Resolver.
func (r *rbacResolver) Resolve(ctx context.Context, subject string) (ResolvedPermissions, error) {
	if r == nil || r.pool == nil {
		return ResolvedPermissions{}, ErrClosed
	}
	if subject == "" {
		return ResolvedPermissions{}, nil // empty subject: fail closed, not an error
	}

	rows, err := r.pool.Query(ctx, permissionsQuery, subject)
	if err != nil {
		return ResolvedPermissions{}, fmt.Errorf("resolve permissions for subject: %w", err)
	}
	defer rows.Close()

	var (
		appUserID  string
		permission *string
		facilityID *uuid.UUID
		roleName   *string
		perms      []string
		grants     []identity.FacilityGrant
		seenPerm   = make(map[string]struct{})
		seenGrant  = make(map[string]struct{})
	)
	for rows.Next() {
		if err := rows.Scan(&appUserID, &permission, &facilityID, &roleName); err != nil {
			return ResolvedPermissions{}, fmt.Errorf("scan resolved permission: %w", err)
		}
		// The LEFT JOINs yield a NULL permission key when the active user has
		// no effective role assignment. Keep its app user id, grant nothing.
		if permission == nil || *permission == "" {
			continue
		}
		key := *permission

		if _, ok := seenPerm[key]; !ok {
			perms = append(perms, key)
			seenPerm[key] = struct{}{}
		}

		// A global assignment (facility_id IS NULL) only yields an
		// unrestricted grant for the super_admin role. Every other role
		// assigned without a facility is scoped to nothing and therefore
		// grants no facility provenance at all.
		unrestricted := facilityID == nil && roleName != nil && *roleName == superAdminRoleName

		grantKey := key + "\x00" + grantScopeKey(facilityID, unrestricted)
		if _, ok := seenGrant[grantKey]; ok {
			continue
		}
		seenGrant[grantKey] = struct{}{}

		grant := identity.FacilityGrant{Key: key, Unrestricted: unrestricted}
		if facilityID != nil {
			scoped := *facilityID
			grant.FacilityID = &scoped
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return ResolvedPermissions{}, fmt.Errorf("iterate resolved permissions: %w", err)
	}

	return ResolvedPermissions{
		Permissions: perms,
		Grants:      grants,
		AppUserID:   appUserID,
	}, nil
}

// superAdminRoleName is the only role whose unscoped (facility_id IS NULL)
// assignment confers global reach. It mirrors the predicate used by
// facilityScopeQuery so the permission view and the scope view can never
// disagree about who is globally unrestricted.
const superAdminRoleName = "super_admin"

// grantScopeKey builds the dedupe key distinguishing the three grant shapes:
// global, facility-scoped to a specific facility, and provenance-free.
func grantScopeKey(facilityID *uuid.UUID, unrestricted bool) string {
	if unrestricted {
		return "global"
	}
	if facilityID == nil {
		return "none"
	}
	return facilityID.String()
}

// verify interface conformance for the nil-safe wrapper.
var _ Resolver = (*rbacResolver)(nil)