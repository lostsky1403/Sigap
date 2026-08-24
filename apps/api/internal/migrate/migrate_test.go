package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Unit tests (no database required) ---

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     int
	}{
		{"init", "0001_init.sql", 1},
		{"identity", "0003_identity_rbac.sql", 3},
		{"notifications", "0006_notifications.sql", 6},
		{"double digit", "0012_foo.sql", 12},
		{"not migration", "README.md", 0},
		{"no version", "foo.sql", 0},
		{"plain text", "schema.sql", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseVersion(tt.filename)
			if got != tt.want {
				t.Errorf("ParseVersion(%q) = %d, want %d", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsSQLFile(t *testing.T) {
	if !IsSQLFile("0001_init.sql") {
		t.Error("expected true for migration file")
	}
	if IsSQLFile("README.md") {
		t.Error("expected false for non-migration file")
	}
	if IsSQLFile("0001_init.sql.bak") {
		t.Error("expected false for .bak file")
	}
}

func TestFormatVersion(t *testing.T) {
	if got := FormatVersion(1); got != "0001" {
		t.Errorf("FormatVersion(1) = %q, want %q", got, "0001")
	}
	if got := FormatVersion(123); got != "0123" {
		t.Errorf("FormatVersion(123) = %q, want %q", got, "0123")
	}
}

func TestDiscoverMigrations(t *testing.T) {
	dir := t.TempDir()

	// Create some migration files.
	files := []string{"0001_init.sql", "0002_types.sql", "0003_rbac.sql", "README.md", "0003_rbac.sql.bak"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("-- "+f), 0644); err != nil {
			t.Fatal(err)
		}
	}

	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should find 3 migrations (0001, 0002, 0003), skip README.md and .bak
	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}

	// Should be sorted by version.
	if migrations[0].Version != 1 || migrations[1].Version != 2 || migrations[2].Version != 3 {
		t.Errorf("unexpected order: %v", migrations)
	}
}

func TestDiscoverMigrations_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestDiscoverMigrations_NonexistentDir(t *testing.T) {
	_, err := DiscoverMigrations("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestValidateFiles(t *testing.T) {
	dir := t.TempDir()

	// Valid migrations.
	os.WriteFile(filepath.Join(dir, "0001_init.sql"), []byte("-- init"), 0644)
	os.WriteFile(filepath.Join(dir, "0002_types.sql"), []byte("-- types"), 0644)

	if err := ValidateFiles(dir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateFiles_DuplicateVersions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0001_init.sql"), []byte("-- init"), 0644)
	// This won't actually happen with DiscoverMigrations since it walks files,
	// but ValidateFiles should catch it.

	// Create a dir entry that would result in duplicate versions
	// (different names but same version — not possible with versionRe since version comes from filename)
	// Instead test empty file validation.
	os.WriteFile(filepath.Join(dir, "0003_empty.sql"), []byte(""), 0644)

	err := ValidateFiles(dir)
	if err == nil {
		t.Error("expected error for empty migration file")
	}
}

func TestCount(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0001_init.sql"), []byte("-- init"), 0644)
	os.WriteFile(filepath.Join(dir, "0002_types.sql"), []byte("-- types"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("-- readme"), 0644)

	count, err := Count(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}
}

func TestMaxVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "0001_init.sql"), []byte("-- init"), 0644)
	os.WriteFile(filepath.Join(dir, "0003_rbac.sql"), []byte("-- rbac"), 0644)
	os.WriteFile(filepath.Join(dir, "0006_notif.sql"), []byte("-- notif"), 0644)

	v, err := MaxVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v != 6 {
		t.Errorf("MaxVersion() = %d, want 6", v)
	}
}

func TestMaxVersion_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	v, err := MaxVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0 {
		t.Errorf("MaxVersion() = %d, want 0", v)
	}
}

func TestDefaultDir_FindsMigrations(t *testing.T) {
	// The test runs from within the Go module, so DefaultDir should find
	// the migration directory.
	dir, err := DefaultDir()
	if err != nil {
		t.Skip("migration directory not found (expected in CI with full repo)")
	}

	// Verify it contains at least the 0001 migration.
	if _, err := os.Stat(filepath.Join(dir, "0001_init.sql")); err != nil {
		t.Errorf("expected 0001_init.sql in %s: %v", dir, err)
	}
}

func TestValidateFiles_NonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	// Only non-migration files.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Migrations"), 0644)

	if err := ValidateFiles(dir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
