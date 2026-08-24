package migrate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single SQL migration file with its parsed version.
type Migration struct {
	Version int
	Name    string
	Path    string
}

// versionRe extracts the leading numeric version and optional name from a filename
// like "0003_identity_rbac.sql" → version=3, name="identity_rbac".
var versionRe = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

// DiscoverMigrations reads .sql files from the given directory and returns them
// sorted by version number.  Files that don't match the NNNN_name.sql pattern
// are silently skipped.
func DiscoverMigrations(dir string) ([]Migration, error) {
	var migrations []Migration

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		m := versionRe.FindStringSubmatch(d.Name())
		if m == nil {
			return nil // skip non-migration files
		}
		v, _ := strconv.Atoi(m[1])
		migrations = append(migrations, Migration{
			Version: v,
			Name:    m[2],
			Path:    path,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk migrations dir %s: %w", dir, err)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// ensureTrackingTable creates the schema_migrations tracking table if it doesn't exist.
func ensureTrackingTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version   INTEGER PRIMARY KEY,
			checksum  BYTEA NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	return nil
}

// appliedVersions returns a set of already-applied version numbers.
func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int]bool, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan schema_migrations version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// checksum computes a SHA-256 hash of the migration SQL content.
func checksum(content []byte) []byte {
	h := sha256.Sum256(content)
	return h[:]
}

// Run reads migrations from dir, determines which are pending, and applies
// them in order.  Each migration runs inside its own transaction.  The version
// and checksum are recorded in schema_migrations on success.
//
// Returns the number of newly applied migrations, or an error if any migration fails.
func Run(ctx context.Context, pool *pgxpool.Pool, dir string) (int, error) {
	if err := ensureTrackingTable(ctx, pool); err != nil {
		return 0, err
	}

	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return 0, err
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return 0, err
	}

	var count int
	for _, m := range migrations {
		if applied[m.Version] {
			slog.Debug("migration already applied", "version", m.Version, "name", m.Name)
			continue
		}

		content, err := os.ReadFile(m.Path)
		if err != nil {
			return count, fmt.Errorf("read migration %04d_%s: %w", m.Version, m.Name, err)
		}

		cs := checksum(content)
		slog.Info("applying migration", "version", m.Version, "name", m.Name)

		// Run each migration inside its own transaction.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return count, fmt.Errorf("begin migration %04d_%s: %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("apply migration %04d_%s: %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)",
			m.Version, cs,
		); err != nil {
			_ = tx.Rollback(ctx)
			return count, fmt.Errorf("record migration %04d_%s: %w", m.Version, m.Name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return count, fmt.Errorf("commit migration %04d_%s: %w", m.Version, m.Name, err)
		}

		slog.Info("migration applied", "version", m.Version, "name", m.Name)
		count++
	}

	if count == 0 {
		slog.Info("all migrations up to date")
	} else {
		slog.Info("migrations applied", "count", count)
	}

	return count, nil
}

// Status reports which migrations are applied and which are pending.
// Returns (applied, pending, error).
func Status(ctx context.Context, pool *pgxpool.Pool, dir string) ([]Migration, []Migration, error) {
	if err := ensureTrackingTable(ctx, pool); err != nil {
		return nil, nil, err
	}

	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return nil, nil, err
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, nil, err
	}

	var appliedList, pendingList []Migration
	for _, m := range migrations {
		if applied[m.Version] {
			appliedList = append(appliedList, m)
		} else {
			pendingList = append(pendingList, m)
		}
	}

	return appliedList, pendingList, nil
}

// DefaultDir returns the default migration directory relative to the Go module.
// It looks for packages/db/migrations from the module root.
func DefaultDir() (string, error) {
	// Try the working directory first.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up to find packages/db/migrations.
	for {
		candidate := filepath.Join(dir, "packages", "db", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback: assume we're run from repo root.
	fallback := filepath.Join("packages", "db", "migrations")
	if info, err := os.Stat(fallback); err == nil && info.IsDir() {
		return fallback, nil
	}

	return "", fmt.Errorf("migration directory not found (searched up from cwd for packages/db/migrations)")
}

// MigrateDir is a convenience that returns the migration dir path from the
// MIGRATION_DIR env var, or falls back to DefaultDir().
func MigrateDir() (string, error) {
	if d := os.Getenv("SIGAP_MIGRATION_DIR"); d != "" {
		return d, nil
	}
	return DefaultDir()
}

// ParseVersion extracts the numeric version prefix from a migration filename.
// e.g. "0003_identity_rbac.sql" → 3.  Returns 0 if the name doesn't match.
func ParseVersion(filename string) int {
	m := versionRe.FindStringSubmatch(filename)
	if m == nil {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

// FormatVersion formats a version number as a zero-padded 4-digit string.
func FormatVersion(v int) string {
	return fmt.Sprintf("%04d", v)
}

// IsSQLFile returns true if the filename looks like a migration SQL file.
func IsSQLFile(filename string) bool {
	return versionRe.MatchString(filename)
}

// MigrationDirFromModPath returns the migration directory by resolving
// relative to go.mod.  This is useful for tests that run from any cwd.
func MigrationDirFromModPath() (string, error) {
	// Find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "..", "..", "packages", "db", "migrations"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return DefaultDir()
}

// Count returns the total number of migration files in the directory.
func Count(dir string) (int, error) {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return 0, err
	}
	return len(migrations), nil
}

// MaxVersion returns the highest migration version in the directory, or 0 if empty.
func MaxVersion(dir string) (int, error) {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, nil
	}
	return migrations[len(migrations)-1].Version, nil
}

// ValidateFiles checks that all migration files follow the naming convention
// and are non-empty.
func ValidateFiles(dir string) error {
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		return err
	}

	seen := make(map[int]bool)
	for _, m := range migrations {
		if seen[m.Version] {
			return fmt.Errorf("duplicate version %04d", m.Version)
		}
		seen[m.Version] = true

		info, err := os.Stat(m.Path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", m.Path, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("migration %04d_%s is empty", m.Version, m.Name)
		}
	}
	return nil
}
