package auth

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// testDBURL returns the PostgreSQL DSN for integration tests, or "" when not
// configured so the test can skip. Tests share one dedicated database so the
// live dev database is never touched.
func testDBURL() string {
	return os.Getenv("DATABASE_URL")
}

// applyMigrations builds the resilient RBAC schema the resolver depends on
// from the real migration files 0001 (facilities), 0003 (identity/RBAC) and
// 0008 (external subject mapping). It does NOT run the full migration chain:
// 0007_checkin_constraints.sql has a pre-existing defect (a STABLE
// appointment_time::date cast inside a partial unique index, rejected on fresh
// DBs with SQLSTATE 42P17) that is unrelated to AUDIT-101 and is out of scope
// to modify here. It drops and rebuilds a dedicated, throwaway schema
// (test_rbac) — never the shared "public" schema that other suites (e.g.
// booking check-in) rely on as a pre-built full schema — so setup is
// repeatable and does not race with those suites.
func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) error {
	t.Helper()
	dir, err := migrate.DefaultDir()
	if err != nil {
		t.Fatalf("migrate dir: %v", err)
	}

	// The dedicated test schema is owned by these tests; drop and rebuild it
	// so every run starts from a clean, known state. The pool's search_path is
	// set to test_rbac, so all unqualified relation references land there.
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS test_rbac CASCADE`); err != nil {
		return fmt.Errorf("drop test_rbac schema: %w", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA test_rbac`); err != nil {
		return fmt.Errorf("create test_rbac schema: %w", err)
	}

	// Apply the complete tracked chain, including existing 0010, so the
	// resolver integration test uses the same schema as CI.
	if _, err := migrate.Run(ctx, pool, dir); err != nil {
		return fmt.Errorf("apply complete migration chain: %w", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, `SELECT 1 FROM schema_migrations WHERE version = 10`).Scan(&applied); err != nil || applied != 1 {
		return fmt.Errorf("verify existing migration 0010: value=%d err=%w", applied, err)
	}
	return nil
}

// seedRBACUser creates an app user with the given external subject and grants
// it the named permission keys through a dedicated role. It returns the user's
// UUID. Any existing rows for this subject are removed first so tests are
// repeatable.
func seedRBACUser(t *testing.T, pool *pgxpool.Pool, subject string, perms ...string) string {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id IN (
		   SELECT id FROM app_users WHERE subject = $1
		 )`, subject); err != nil {
		t.Fatalf("cleanup user_roles: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM app_users WHERE subject = $1`, subject); err != nil {
		t.Fatalf("cleanup app_users: %v", err)
	}

	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO app_users (email, status, subject)
		VALUES (($1 || '@example.test'), 'active', $1)
		RETURNING id::text`, subject).Scan(&userID)
	if err != nil {
		t.Fatalf("insert app user: %v", err)
	}

	if len(perms) == 0 {
		return userID
	}

	// Reuse/assign each permission to one test role per user so role_changes
	// can be demonstrated without creating role collisions between tests.
	for _, perm := range perms {
		var permID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO permissions (key, description) VALUES ($1, 'test permission')
			 ON CONFLICT (key) DO NOTHING
			 RETURNING id::text`, perm).Scan(&permID); err != nil {
			// Permission already existed; fetch it.
			if err = pool.QueryRow(ctx, `SELECT id::text FROM permissions WHERE key = $1`, perm).Scan(&permID); err != nil {
				t.Fatalf("get permission %s: %v", perm, err)
			}
		}

		roleID := uuid.NewString()
		if _, err := pool.Exec(ctx,
			`INSERT INTO roles (id, name, description) VALUES ($1, $2, 'test role')`,
			roleID, testRoleName(perm, subject)); err != nil {
			t.Fatalf("insert role: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
			userID, roleID); err != nil {
			t.Fatalf("insert user_role: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`,
			roleID, permID); err != nil {
			t.Fatalf("insert role_permission: %v", err)
		}
	}

	return userID
}

// testRoleName builds a deterministic unique role name per (permission, subject).
func testRoleName(perm, subject string) string {
	return "role_" + perm + "_" + subjectHash(subject)
}

// subjectHash returns a short stable suffix derived from the subject so role
// names stay unique when the same permission is used across subjects.
func subjectHash(s string) string {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return string(rune('a'+h%26)) + string(rune('a'+(h/26)%26)) + string(rune('a'+(h/676)%26))
}

// grantPermission adds a permission to an existing app user's test role
// without touching the JWT. It mirrors how an admin would change server-side
// RBAC state.
func grantPermission(t *testing.T, pool *pgxpool.Pool, subject, perm string) {
	t.Helper()
	ctx := context.Background()

	var permID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO permissions (key, description) VALUES ($1, 'test permission')
		 ON CONFLICT (key) DO NOTHING
		 RETURNING id::text`, perm).Scan(&permID); err != nil {
		if err = pool.QueryRow(ctx, `SELECT id::text FROM permissions WHERE key = $1`, perm).Scan(&permID); err != nil {
			t.Fatalf("get permission %s: %v", perm, err)
		}
	}

	// Ensure the subject has a role; reuse the deterministic role if present.
	var roleID string
	err := pool.QueryRow(ctx, `
		SELECT r.id::text
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		JOIN app_users u ON u.id = ur.user_id
		WHERE u.subject = $1
		LIMIT 1`, subject).Scan(&roleID)
	if err != nil {
		t.Fatalf("find role for subject: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
		 ON CONFLICT (role_id, permission_id) DO NOTHING`,
		roleID, permID); err != nil {
		t.Fatalf("grant permission: %v", err)
	}
}

// newTestResolverPool connects to the dedicated integration DB and applies all
// migrations. It fails the test (rather than panic) if the DB is reachable.
// Returns the pool and a cleanup function.
func newTestResolverPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dbURL := testDBURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping resolver integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	// Route every unqualified relation/function reference to the throwaway
	// test_rbac schema. "public" is kept on the path only so that any
	// extension-backed functions (e.g. gen_random_uuid) resolve correctly;
	// DDL always targets test_rbac because it is first in the list.
	cfg.ConnConfig.RuntimeParams["search_path"] = "test_rbac, public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}

	if err := applyMigrations(t, ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}

	cleanup := func() { pool.Close() }
	return pool, cleanup
}

func TestRBACResolver_Integration(t *testing.T) {
	pool, cleanup := newTestResolverPool(t)
	defer cleanup()
	ctx := context.Background()

	resolver := NewRBACResolver(pool)

	t.Run("subject with role resolves granted permissions", func(t *testing.T) {
		seedRBACUser(t, pool, "alice", "queue.read", "facility.read")
		got, err := resolver.Resolve(ctx, "alice")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !contains(got.Permissions, "queue.read") || !contains(got.Permissions, "facility.read") {
			t.Errorf("permissions = %v, want {queue.read, facility.read}", got.Permissions)
		}
		if got.AppUserID == "" {
			t.Errorf("expected appUserID for known subject")
		}
	})

	t.Run("unknown subject fails closed with empty permissions", func(t *testing.T) {
		got, err := resolver.Resolve(ctx, "no-such-subject")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("expected zero permissions, got %v", got.Permissions)
		}
		if got.AppUserID != "" {
			t.Errorf("expected empty app user id, got %q", got.AppUserID)
		}
	})

	t.Run("disabled user fails closed", func(t *testing.T) {
		seedRBACUser(t, pool, "disabled-user", "queue.read")
		if _, err := pool.Exec(ctx,
			`UPDATE app_users SET status = 'disabled' WHERE subject = 'disabled-user'`); err != nil {
			t.Fatalf("disable user: %v", err)
		}
		got, err := resolver.Resolve(ctx, "disabled-user")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("expected zero permissions for disabled user, got %v", got.Permissions)
		}
	})

	t.Run("soft-deleted user fails closed", func(t *testing.T) {
		seedRBACUser(t, pool, "deleted-user", "queue.read")
		if _, err := pool.Exec(ctx,
			`UPDATE app_users SET deleted_at = now() WHERE subject = 'deleted-user'`); err != nil {
			t.Fatalf("soft-delete user: %v", err)
		}
		got, err := resolver.Resolve(ctx, "deleted-user")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("expected zero permissions for deleted user, got %v", got.Permissions)
		}
	})

	for _, tc := range []struct {
		name       string
		subject    string
		assignment string
		keepPerm   string
	}{
		{name: "inactive role assignment grants no permissions", subject: "inactive-assignment", assignment: `status = 'inactive'`},
		{name: "soft-deleted role assignment grants no permissions", subject: "deleted-assignment", assignment: `deleted_at = now()`},
		{name: "inactive assignment grants nothing alongside an active one", subject: "mixed-inactive", assignment: `status = 'inactive'`, keepPerm: "facility.read"},
		{name: "soft-deleted assignment grants nothing alongside an active one", subject: "mixed-deleted", assignment: `deleted_at = now()`, keepPerm: "facility.read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			perms := []string{"queue.read"}
			if tc.keepPerm != "" {
				perms = append(perms, tc.keepPerm)
			}
			userID := seedRBACUser(t, pool, tc.subject, perms...)
			if _, err := pool.Exec(ctx,
				`UPDATE user_roles SET `+tc.assignment+`
				 WHERE user_id IN (SELECT id FROM app_users WHERE subject = $1)
				   AND role_id IN (
				     SELECT rp.role_id
				     FROM role_permissions rp
				     JOIN permissions p ON p.id = rp.permission_id
				     WHERE p.key = 'queue.read'
				   )`, tc.subject); err != nil {
				t.Fatalf("apply assignment lifecycle: %v", err)
			}

			got, err := resolver.Resolve(ctx, tc.subject)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if contains(got.Permissions, "queue.read") {
				t.Errorf("lifecycle-dead assignment granted queue.read, permissions = %v", got.Permissions)
			}
			if len(got.Permissions) != boolToInt(tc.keepPerm != "") {
				t.Errorf("permissions = %v, want %d effective permission(s)", got.Permissions, boolToInt(tc.keepPerm != ""))
			}
			if got.AppUserID != userID {
				t.Errorf("AppUserID=%q want %q", got.AppUserID, userID)
			}
		})
	}

	t.Run("active user without a role resolves empty permissions and app user id", func(t *testing.T) {
		subject := "no-role-user"
		userID := seedRBACUser(t, pool, subject)
		got, err := resolver.Resolve(ctx, subject)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("expected zero permissions, got %v", got.Permissions)
		}
		if got.AppUserID != userID {
			t.Errorf("AppUserID=%q want %q", got.AppUserID, userID)
		}
	})

	t.Run("role change reflected without token change", func(t *testing.T) {
		// Subject starts with only facility.read.
		seedRBACUser(t, pool, "bob", "facility.read")
		before, err := resolver.Resolve(ctx, "bob")
		if err != nil {
			t.Fatalf("resolve before: %v", err)
		}
		if !contains(before.Permissions, "facility.read") {
			t.Fatalf("precondition: expected facility.read, got %v", before.Permissions)
		}
		if contains(before.Permissions, "facility.manage") {
			t.Fatalf("precondition: facility.manage should not be granted yet, got %v", before.Permissions)
		}

		// Admin grants facility.manage in the DB — the same "token" (subject)
		// now resolves an additional permission. No JWT was changed or re-signed.
		grantPermission(t, pool, "bob", "facility.manage")
		after, err := resolver.Resolve(ctx, "bob")
		if err != nil {
			t.Fatalf("resolve after: %v", err)
		}
		if !contains(after.Permissions, "facility.manage") {
			t.Errorf("DB grant not reflected: expected facility.manage, got %v", after.Permissions)
		}
		if _, err := pool.Exec(ctx,
			`DELETE FROM user_roles WHERE user_id IN (SELECT id FROM app_users WHERE subject = 'bob')`); err != nil {
			t.Fatalf("cleanup bob: %v", err)
		}
	})
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
