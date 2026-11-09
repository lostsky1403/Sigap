package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/identity"
)

// OrganizationScopeResult is the outcome of resolving an actor's organization
// reach: the set of organizations the actor administers or may otherwise
// observe, plus a global flag for the two unscoped roles that are allowed to
// span every organization (super_admin and regional_admin).
//
// Exactly one of the following is meaningful:
//
//   - Unrestricted == true: the actor is super_admin or regional_admin with an
//     unscoped (facility_id IS NULL AND organization_id IS NULL) assignment,
//     so every organization is visible. OrgIDs is ignored.
//   - Unrestricted == false: OrgIDs holds exactly the organizations covered by
//     an org-scoped (organization_id NOT NULL) assignment. An empty OrgIDs
//     therefore means "authorized for no organization", which callers must
//     render as an empty result (HTTP 200 + []) rather than an error so org
//     existence never leaks.
type OrganizationScopeResult struct {
	OrgIDs       []uuid.UUID
	Unrestricted bool
	Err          error
}

// OrganizationScope is the minimal organization-scope interface consumed by
// handlers. It mirrors FacilityScope and is satisfied by dbOrganizationScope.
type OrganizationScope interface {
	AllowedOrganizationIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error)
}

// OrganizationScopeResolver resolves organization scope in the richer result
// shape (with the unrestricted flag). Handlers type-assert for it exactly as
// they do for FacilityScopeResolver.
type OrganizationScopeResolver interface {
	ResolveOrganizationScope(ctx context.Context, appUserID string) OrganizationScopeResult
}

type dbOrganizationScope struct {
	pool *pgxpool.Pool
}

// NewDBOrganizationScope returns an OrganizationScope backed by pool.
func NewDBOrganizationScope(pool *pgxpool.Pool) OrganizationScope {
	return &dbOrganizationScope{pool: pool}
}

// organizationScopeQuery resolves an actor's organization reach from the
// trusted user_roles schema. It mirrors facilityScopeQuery's lifecycle filters
// and super_admin/regional_admin semantics so the two scope views can never
// disagree. The only rows that contribute are:
//
//   - an org-scoped assignment (organization_id NOT NULL), which yields its
//     organization; and
//   - an unscoped super_admin/regional_admin assignment
//     (facility_id IS NULL AND organization_id IS NULL), which sets the
//     unrestricted flag.
//
// A facility-scoped or zero-scope assignment contributes no organization
// reach, exactly as it contributes no global facility reach.
const organizationScopeQuery = `
SELECT DISTINCT
       ur.organization_id,
       (ur.organization_id IS NULL AND ur.facility_id IS NULL
        AND r.name IN ('super_admin', 'regional_admin')) AS unrestricted
FROM user_roles ur
JOIN app_users u ON u.id = ur.user_id
JOIN roles r ON r.id = ur.role_id
WHERE u.id = $1
  AND u.status = 'active'
  AND u.deleted_at IS NULL
  AND ur.status = 'active'
  AND ur.deleted_at IS NULL`

func (s *dbOrganizationScope) ResolveOrganizationScope(ctx context.Context, appUserID string) OrganizationScopeResult {
	if s == nil || s.pool == nil {
		return OrganizationScopeResult{Err: ErrClosed}
	}
	if appUserID == "" {
		return OrganizationScopeResult{}
	}

	rows, err := s.pool.Query(ctx, organizationScopeQuery, appUserID)
	if err != nil {
		return OrganizationScopeResult{Err: fmt.Errorf("resolve organization scope: %w", err)}
	}
	defer rows.Close()

	result := OrganizationScopeResult{}
	seen := make(map[uuid.UUID]struct{})
	for rows.Next() {
		var (
			organizationID *uuid.UUID
			unrestricted   bool
		)
		if err := rows.Scan(&organizationID, &unrestricted); err != nil {
			return OrganizationScopeResult{Err: fmt.Errorf("scan organization scope: %w", err)}
		}
		if unrestricted {
			result.Unrestricted = true
			continue
		}
		if organizationID != nil {
			if _, dup := seen[*organizationID]; dup {
				continue
			}
			seen[*organizationID] = struct{}{}
			result.OrgIDs = append(result.OrgIDs, *organizationID)
		}
	}
	if err := rows.Err(); err != nil {
		return OrganizationScopeResult{Err: fmt.Errorf("iterate organization scope: %w", err)}
	}
	return result
}

func (s *dbOrganizationScope) AllowedOrganizationIDs(ctx context.Context, appUserID string) ([]uuid.UUID, error) {
	result := s.ResolveOrganizationScope(ctx, appUserID)
	return result.OrgIDs, result.Err
}

// OrganizationScopeForActor resolves an actor's organization scope, honoring
// the dev bypass and resolver-interface dispatch exactly like
// FacilityScopeForActor. It fails closed: a nil resolver or an actor without
// an app user id returns Err.
func OrganizationScopeForActor(ctx context.Context, actor identity.Actor, resolver OrganizationScope) OrganizationScopeResult {
	if actor.IsDev {
		return OrganizationScopeResult{Unrestricted: true}
	}
	if actor.AppUserID == "" || resolver == nil {
		return OrganizationScopeResult{Err: ErrClosed}
	}
	if scopedResolver, ok := resolver.(OrganizationScopeResolver); ok {
		return scopedResolver.ResolveOrganizationScope(ctx, actor.AppUserID)
	}
	ids, err := resolver.AllowedOrganizationIDs(ctx, actor.AppUserID)
	if err != nil {
		return OrganizationScopeResult{Err: err}
	}
	return OrganizationScopeResult{OrgIDs: ids}
}

// OrganizationScopeResultAllows reports whether the organization is inside the
// actor's organization scope. It is the org-axis analogue of
// FacilityScopeResultAllows.
func OrganizationScopeResultAllows(result OrganizationScopeResult, organizationID uuid.UUID) bool {
	if result.Err != nil {
		return false
	}
	if result.Unrestricted {
		return true
	}
	if organizationID == uuid.Nil {
		return false
	}
	for _, id := range result.OrgIDs {
		if id == organizationID {
			return true
		}
	}
	return false
}

// DBOrganizationScopeMarker is implemented by the production DB-backed
// organization scope resolver, mirroring DBFacilityScopeMarker so handlers can
// detect live resolution.
type DBOrganizationScopeMarker interface {
	IsDBOrganizationScope()
}

func (s *dbOrganizationScope) IsDBOrganizationScope() {}

var _ OrganizationScope = (*dbOrganizationScope)(nil)
var _ OrganizationScopeResolver = (*dbOrganizationScope)(nil)