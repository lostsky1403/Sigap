package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolvedPermissions is the result of resolving an external identity subject
// against the server-side RBAC state. It carries the effective permission key
// set plus the server-side application user the subject mapped to.
type ResolvedPermissions struct {
	// Permissions is the distinct set of permission keys granted to the subject
	// through its roles. It is empty when the subject is unknown, disabled, or
	// soft-deleted (fail closed).
	Permissions []string
	// AppUserID is the server-side app_users.id the subject mapped to. It is
	// empty when the subject is unknown. It is surfaced so a later facility
	// scope PR (AUDIT-202) can resolve user_roles.facility_id from trusted state.
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
const permissionsQuery = `
SELECT u.id,
       p.key
FROM app_users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN role_permissions rp ON rp.role_id = ur.role_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE u.subject = $1
  AND u.status = 'active'
  AND u.deleted_at IS NULL
ORDER BY p.key`

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
		permission string
		perms      []string
	)
	for rows.Next() {
		if err := rows.Scan(&appUserID, &permission); err != nil {
			return ResolvedPermissions{}, fmt.Errorf("scan resolved permission: %w", err)
		}
		// LEFT JOIN yields NULL permission keys for a subject with no role
		// grant. A NULL appUserID means the subject is unknown (no active row).
		if permission == "" {
			continue
		}
		perms = append(perms, permission)
	}
	if err := rows.Err(); err != nil {
		return ResolvedPermissions{}, fmt.Errorf("iterate resolved permissions: %w", err)
	}

	return ResolvedPermissions{
		Permissions: perms,
		AppUserID:   appUserID,
	}, nil
}

// verify interface conformance for the nil-safe wrapper.
var _ Resolver = (*rbacResolver)(nil)