package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// AppUserResolver is the OPTIONAL extension of Resolver that also resolves by
// the trusted internal app_users.id. It is deliberately kept out of Resolver so
// that adding live provenance does not break existing implementations (and test
// doubles) of the authentication-time lookup.
//
// Facility-scoped read authorization requires this capability: it must observe
// current RBAC state rather than the authentication-time snapshot. Callers type
// assert for it and fail closed when it is absent.
type AppUserResolver interface {
	Resolver
	ResolveByAppUserID(ctx context.Context, appUserID string) (ResolvedPermissions, error)
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
       ur.organization_id,
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
ORDER BY p.key, ur.facility_id NULLS FIRST, ur.organization_id NULLS FIRST, r.name`

// permissionsByAppUserQuery is permissionsQuery keyed by the trusted internal
// app_users.id instead of the external subject. It is intentionally the same
// projection and the same lifecycle filters so that scope, provenance, and the
// route-level permission view can never disagree about a single actor.
const permissionsByAppUserQuery = `
SELECT u.id,
       p.key,
       ur.facility_id,
       ur.organization_id,
       r.name
FROM app_users u
LEFT JOIN user_roles ur
  ON ur.user_id = u.id
 AND ur.status = 'active'
 AND ur.deleted_at IS NULL
LEFT JOIN roles r ON r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = ur.role_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.id = $1
  AND u.status = 'active'
  AND u.deleted_at IS NULL
ORDER BY p.key, ur.facility_id NULLS FIRST, ur.organization_id NULLS FIRST, r.name`

// ResolveByAppUserID resolves the effective grant set directly from the
// server-side app_users.id, bypassing the external subject lookup.
//
// It exists because request-scoped authorization carries AppUserID (the trusted
// internal identity) rather than the external subject, and facility-scoped
// authorization must observe live role changes rather than the
// authentication-time snapshot. The query and super_admin semantics are
// identical to Resolve, so the two views cannot disagree.
func (r *rbacResolver) ResolveByAppUserID(ctx context.Context, appUserID string) (ResolvedPermissions, error) {
	if r == nil || r.pool == nil {
		return ResolvedPermissions{}, ErrClosed
	}
	if appUserID == "" {
		return ResolvedPermissions{}, nil
	}
	rows, err := r.pool.Query(ctx, permissionsByAppUserQuery, appUserID)
	if err != nil {
		return ResolvedPermissions{}, fmt.Errorf("resolve permissions for app user: %w", err)
	}
	defer rows.Close()
	return r.scanResolvedPermissions(ctx, rows)
}

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
	return r.scanResolvedPermissions(ctx, rows)
}

// rawGrantRow is one unfolded (key, facility, org, role) tuple from the RBAC
// join. Facility and organization may both be nil for a zero-scope or global
// assignment; the role name disambiguates the two.
type rawGrantRow struct {
	key            string
	facilityID     *uuid.UUID
	organizationID *uuid.UUID
	roleName       *string
}

// scanResolvedPermissions folds the shared (key, facility, organization, role)
// result shape into a ResolvedPermissions. It preserves facility provenance,
// applies the global super_admin/regional_admin rule, and expands an
// org-scoped assignment into one concrete facility grant per facility of that
// organization — so the org axis stays a pure derivation of
// facilities.organization_id and never widens beyond it.
func (r *rbacResolver) scanResolvedPermissions(ctx context.Context, rows pgx.Rows) (ResolvedPermissions, error) {
	appUserID := ""
	perms := []string{}
	seenPerm := make(map[string]struct{})
	var raw []rawGrantRow

	for rows.Next() {
		var (
			permission     *string
			facilityID     *uuid.UUID
			organizationID *uuid.UUID
			roleName       *string
		)
		if err := rows.Scan(&appUserID, &permission, &facilityID, &organizationID, &roleName); err != nil {
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
		raw = append(raw, rawGrantRow{key: key, facilityID: facilityID, organizationID: organizationID, roleName: roleName})
	}
	if err := rows.Err(); err != nil {
		return ResolvedPermissions{}, fmt.Errorf("iterate resolved permissions: %w", err)
	}

	facilityByOrg, regionalAll, err := r.expansionSets(ctx, raw)
	if err != nil {
		return ResolvedPermissions{}, err
	}

	grants := []identity.FacilityGrant{}
	seenGrant := make(map[string]struct{})
	for _, row := range raw {
		switch {
		case row.facilityID != nil:
			scoped := *row.facilityID
			grants = appendUniqueGrant(grants, seenGrant, identity.FacilityGrant{Key: row.key, FacilityID: &scoped})
		case row.organizationID != nil:
			for _, f := range facilityByOrg[*row.organizationID] {
				fid := f
				grants = appendUniqueGrant(grants, seenGrant, identity.FacilityGrant{Key: row.key, FacilityID: &fid})
			}
		case row.roleName != nil && *row.roleName == regionalAdminRoleName:
			// Unscoped regional_admin: explicit per-facility grants over ALL
			// facilities, NOT the unrestricted flag (which stays super_admin
			// only). Operational read is preserved; no .manage keys are in its
			// seeded permission set, so nothing here can authorize a mutation.
			for _, f := range regionalAll {
				fid := f
				grants = appendUniqueGrant(grants, seenGrant, identity.FacilityGrant{Key: row.key, FacilityID: &fid})
			}
		case row.roleName != nil && *row.roleName == superAdminRoleName:
			grants = appendUniqueGrant(grants, seenGrant, identity.FacilityGrant{Key: row.key, Unrestricted: true})
		default:
			// zero-scope: facility_id NULL, org NULL, and not super_admin or
			// regional_admin. Grants no facility provenance at any facility.
		}
	}

	return ResolvedPermissions{
		Permissions: perms,
		Grants:      grants,
		AppUserID:   appUserID,
	}, nil
}

// appendUniqueGrant dedupes by (key, scope) and appends when new. It preserves
// the previous grantScopeKey semantics: one entry per (key, facility) or
// (key, global).
func appendUniqueGrant(grants []identity.FacilityGrant, seen map[string]struct{}, g identity.FacilityGrant) []identity.FacilityGrant {
	key := g.Key + "\x00" + grantScopeKey(g.FacilityID, g.Unrestricted)
	if _, ok := seen[key]; ok {
		return grants
	}
	seen[key] = struct{}{}
	return append(grants, g)
}

// expansionSets precomputes the two derived facility sets the grant fold needs:
//   - facilityByOrg: per-organization facility lists (for org-scoped grants);
//   - allFacilities: the full facility list (for an unscoped regional_admin),
//     nil when no unscoped regional_admin grant is present.
func (r *rbacResolver) expansionSets(ctx context.Context, raw []rawGrantRow) (map[uuid.UUID][]uuid.UUID, []uuid.UUID, error) {
	facilityByOrg := make(map[uuid.UUID][]uuid.UUID)
	orgSet := make(map[uuid.UUID]struct{})
	needAll := false
	for _, row := range raw {
		if row.organizationID != nil && row.facilityID == nil {
			orgSet[*row.organizationID] = struct{}{}
		}
		if row.roleName != nil && *row.roleName == regionalAdminRoleName &&
			row.facilityID == nil && row.organizationID == nil {
			needAll = true
		}
	}

	if len(orgSet) > 0 {
		orgIDs := make([]uuid.UUID, 0, len(orgSet))
		for id := range orgSet {
			orgIDs = append(orgIDs, id)
		}
		facilityRows, err := r.pool.Query(ctx,
			`SELECT organization_id, id FROM facilities WHERE organization_id = ANY($1)`, orgIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve org facilities: %w", err)
		}
		for facilityRows.Next() {
			var (
				orgID      uuid.UUID
				facilityID uuid.UUID
			)
			if err := facilityRows.Scan(&orgID, &facilityID); err != nil {
				facilityRows.Close()
				return nil, nil, fmt.Errorf("scan org facility: %w", err)
			}
			facilityByOrg[orgID] = append(facilityByOrg[orgID], facilityID)
		}
		if err := facilityRows.Err(); err != nil {
			facilityRows.Close()
			return nil, nil, fmt.Errorf("iterate org facilities: %w", err)
		}
		facilityRows.Close()
	}

	var allFacilities []uuid.UUID
	if needAll {
		rows, err := r.pool.Query(ctx, `SELECT id FROM facilities`)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve all facilities: %w", err)
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("scan facility: %w", err)
			}
			allFacilities = append(allFacilities, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("iterate all facilities: %w", err)
		}
		rows.Close()
	}

	return facilityByOrg, allFacilities, nil
}

// superAdminRoleName is the only role whose unscoped (facility_id IS NULL)
// assignment confers the GLOBAL/unrestricted reach. regionalAdminRoleName is
// regional over every facility but deliberately explicit, never unrestricted,
// so super_admin stays the sole operator of scope.Unrestricted (the gate behind
// CreateFacility/DeactivateFacility).
const (
	superAdminRoleName   = "super_admin"
	regionalAdminRoleName = "regional_admin"
)

// grantScopeKey builds the dedupe key distinguishing the grant shapes: global,
// facility-scoped to a specific facility, and provenance-free.
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
