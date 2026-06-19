// cmd/bootstrap/main.go
//
// One-time bootstrap admin creation.
//
// SAFETY: This tool is DISABLED BY DEFAULT. It only runs when
// SIGAP_BOOTSTRAP_ADMIN is explicitly set to "true".
// Never enable in production. Never commit with the env var set.
//
// Usage:
//   SIGAP_BOOTSTRAP_ADMIN=true DATABASE_URL=postgres://... go run ./cmd/bootstrap
//
// The tool creates a synthetic admin user (admin@sigap.local) and assigns the
// existing super_admin role from the RBAC seed. It is idempotent: rerunning
// will not create duplicates.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	adminEmail       = "admin@sigap.local"
	adminDisplayName = "Bootstrap Admin"
	adminRoleName    = "super_admin"
)

func checkEnabled() bool {
	return os.Getenv("SIGAP_BOOTSTRAP_ADMIN") == "true"
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	userID, created, err := upsertAdmin(ctx, pool)
	if err != nil {
		return fmt.Errorf("upsert admin: %w", err)
	}

	if created {
		log.Printf("Created bootstrap admin user: %s (%s)", userID, adminEmail)
	} else {
		log.Printf("Bootstrap admin user already exists: %s (%s)", userID, adminEmail)
	}

	assigned, err := assignSuperAdminRole(ctx, pool, userID)
	if err != nil {
		return fmt.Errorf("assign super_admin role: %w", err)
	}
	if assigned {
		log.Printf("Assigned super_admin role to %s", userID)
	} else {
		log.Printf("super_admin role already assigned to %s", userID)
	}
	return nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[bootstrap] ")

	if !checkEnabled() {
		log.Fatal("SIGAP_BOOTSTRAP_ADMIN is not set to true. This tool is disabled by default for safety.")
	}

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// upsertAdmin creates the synthetic bootstrap admin user if it does not exist.
// The email uses a .local TLD to prevent accidental delivery to real addresses.
// Returns the user ID and a boolean indicating whether a new row was created.
func upsertAdmin(ctx context.Context, pool *pgxpool.Pool) (string, bool, error) {
	// Try to find existing user by email.
	var userID string
	err := pool.QueryRow(ctx,
		`SELECT id FROM app_users WHERE email = $1 AND deleted_at IS NULL`,
		adminEmail,
	).Scan(&userID)
	if err == nil {
		return userID, false, nil
	}

	// Create new user.
	err = pool.QueryRow(ctx,
		`INSERT INTO app_users (email, display_name, status)
		 VALUES ($1, $2, 'active')
		 RETURNING id`,
		adminEmail, adminDisplayName,
	).Scan(&userID)
	if err != nil {
		return "", false, fmt.Errorf("insert app_users: %w", err)
	}
	return userID, true, nil
}

// assignSuperAdminRole assigns the super_admin role to the given user.
// Returns true if a new assignment was inserted.
func assignSuperAdminRole(ctx context.Context, pool *pgxpool.Pool, userID string) (bool, error) {
	var roleID string
	err := pool.QueryRow(ctx,
		`SELECT id FROM roles WHERE name = $1`,
		adminRoleName,
	).Scan(&roleID)
	if err != nil {
		return false, fmt.Errorf("find super_admin role: %w", err)
	}

	// Check if already assigned.
	var existing int
	err = pool.QueryRow(ctx,
		`SELECT 1 FROM user_roles WHERE user_id = $1 AND role_id = $2`,
		userID, roleID,
	).Scan(&existing)
	if err == nil {
		return false, nil
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`,
		userID, roleID,
	)
	if err != nil {
		return false, fmt.Errorf("insert user_roles: %w", err)
	}
	return true, nil
}
