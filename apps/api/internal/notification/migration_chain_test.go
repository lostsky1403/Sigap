package notification

// Fresh-chain migration tests. The historical 0011 transition tests in
// migration_transition_test.go deliberately stage only 0001..0011; these tests
// cover the CURRENT 0001..0016 chain through the real runner, its version-only
// ledger, checksum recording, ordering and idempotency.
//
// Requires DATABASE_URL (a server connection with CREATEDB); skips when unset.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// expectedAppliedChecksums are the SHA-256 values of migrations 0001..0011 as
// shipped on canonical main. They are pinned here so a silent edit to an
// already-applied migration fails this suite even if the runner (which is
// version-only) would skip it on an existing database.
var expectedAppliedChecksums = map[string]string{
	"0001_init.sql":                                "1481fe8af216b2512c4d4b047a7b0e7741c0801ac8c6e8149354fe0baf756dad",
	"0002_medical_records.sql":                     "bf7dc24edab9c01e3787cffa59e080451460f66aa900a10e420d92989b7a8d03",
	"0003_identity_rbac.sql":                       "17296851b6b9be15308c679518e93a4083152da68e6ee653c82879d187d7d2c2",
	"0004_audit_events.sql":                        "97880eb1f40c476061e3c4d296a0910fc2819b8a13084773b0dbbd8525727483",
	"0005_appointments.sql":                        "a9e2bf2e2fc5f18bfc88fb6c0dd97bde5e03f78fc5290b1e57587a8263682fc1",
	"0006_notifications.sql":                       "1389698344dff1f868c3ccea16f2f3c3fb6807a033699670994b03d2bf627119",
	"0007_checkin_constraints.sql":                 "cbd9865c4c1d3e2e389893aa449b244d55395691befbfe6cb421c3b63ab46e8d",
	"0008_identity_subject.sql":                    "016c9092b868cff0fae6062ff117598da7194f1218af930ef99192efdbed7f03",
	"0009_user_roles_lifecycle.sql":                "b7b5d0a043ba4499d6bd5a3f80c94d2c8de956c05d8a806e7b83c331642055f6",
	"0010_demo_notes_column.sql":                   "154a433dc6d2e9938d791e694de08118b44352ace0dce7bbe698226a3ac9c881",
	"0011_notification_outbox_phone_constraints.sql": "3568a910fed6a4e73baedeed093924cf8850597dd4bc7c0884fe1c740373d0b2",
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// TestFreshMigrationChain_0016 verifies that the real runner applies every
// migration 0001..0016 exactly once in forward order, records the correct
// checksum, is idempotent on re-run, and that the previously applied 0001..0011
// files are byte-for-byte the canonical versions.
func TestFreshMigrationChain_0016(t *testing.T) {
	pool := migPool(t)
	ctx := context.Background()

	dir, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}
	migrations, err := migrate.DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("discover migrations: %v", err)
	}
	if len(migrations) != 16 {
		t.Fatalf("expected 16 migrations in the chain, got %d", len(migrations))
	}
	for i, m := range migrations {
		if m.Version != i+1 {
			t.Fatalf("migration order/version: position %d has version %d", i+1, m.Version)
		}
	}

	n, err := migrate.Run(ctx, pool, dir)
	if err != nil {
		t.Fatalf("apply full chain: %v", err)
	}
	if n != 16 {
		t.Fatalf("expected 16 newly applied migrations, got %d", n)
	}

	var count int
	var distinct int
	var outOfOrder int
	if err := pool.QueryRow(ctx,
		`SELECT count(*), count(DISTINCT version),
		        count(*) FILTER (WHERE version < 1 OR version > 16)::int
		   FROM schema_migrations`).Scan(&count, &distinct, &outOfOrder); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if count != 16 || distinct != 16 || outOfOrder != 0 {
		t.Fatalf("ledger wrong: rows=%d distinct=%d out-of-range=%d", count, distinct, outOfOrder)
	}

	// Ledger checksums equal the SHA-256 of the actual applied files.
	for _, m := range migrations {
		var stored []byte
		if err := pool.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, m.Version).Scan(&stored); err != nil {
			t.Fatalf("read checksum for version %d: %v", m.Version, err)
		}
		if got := sha256File(t, filepath.Join(dir, filepath.Base(m.Path))); fmt.Sprintf("%x", stored) != got {
			t.Fatalf("version %d checksum mismatch: ledger=%x file=%s", m.Version, stored, got)
		}
	}

	// Previously applied migrations are unchanged from the canonical baseline.
	for name, want := range expectedAppliedChecksums {
		if got := sha256File(t, filepath.Join(dir, name)); got != want {
			t.Fatalf("applied migration %s was modified: got %s want %s", name, got, want)
		}
	}

	// Idempotency: the runner is version-only, so a re-run applies nothing and
	// does not duplicate or rewrite ledger rows.
	again, err := migrate.Run(ctx, pool, dir)
	if err != nil {
		t.Fatalf("re-run full chain: %v", err)
	}
	if again != 0 {
		t.Fatalf("re-run re-applied %d migrations; version-only behavior broken", again)
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&after); err != nil {
		t.Fatalf("re-read ledger: %v", err)
	}
	if after != 16 {
		t.Fatalf("ledger changed on re-run: %d rows, want 16", after)
	}

	// Schema compatibility: the objects introduced by 0012..0014 are present.
	boolProbes := []string{
		`SELECT to_regclass('public.organizations') IS NOT NULL`,
		`SELECT to_regclass('public.uq_active_booking_per_patient_day') IS NOT NULL`,
	}
	for _, probe := range boolProbes {
		var ok bool
		if err := pool.QueryRow(ctx, probe).Scan(&ok); err != nil || !ok {
			t.Fatalf("schema probe %q failed: ok=%v err=%v", probe, ok, err)
		}
	}
	var cols int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name='appointments' AND column_name='patient_account_id'`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("patient_account_id column probe failed: columns=%d err=%v", cols, err)
	}
}

// TestFreshMigrationChain_HistoricalFixtureIsolation proves the historical
// 0011 fixture is not disturbed by the presence of 0012..0014: staging 0001..0011
// from the same directory still yields exactly 11 discoverable migrations.
func TestFreshMigrationChain_HistoricalFixtureIsolation(t *testing.T) {
	staged := stage0011(t)
	migrations, err := migrate.DiscoverMigrations(staged)
	if err != nil {
		t.Fatalf("discover staged migrations: %v", err)
	}
	if len(migrations) != 11 {
		t.Fatalf("staged 0011 fixture has %d migrations, want 11", len(migrations))
	}
	if migrations[len(migrations)-1].Version != 11 {
		t.Fatalf("staged fixture last version=%d, want 11", migrations[len(migrations)-1].Version)
	}
}
