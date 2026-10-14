package notification

// Production-shaped migration-transition tests.
//
// A database built from HEAD already carries the STRENGTHENED phone constraints,
// so applying 0011 there is a no-op and proves nothing about remediation. These
// tests instead reconstruct the OBSERVED PRODUCTION SHAPE:
//
//	* migrations 0001..0010 applied, schema_migrations records 1..10 only
//	* the two phone constraints in the pre-9d4e68e WEAK form
//	* synthetic historical notification rows present
//
// and then run the REAL migration runner (migrate.Run) to apply 0011.
//
// Two paths are covered, because they have opposite outcomes and BOTH matter:
//
//	TestMigration0011_Transition_CompatibleHistory
//	    historical rows all satisfy the strengthened predicate
//	    -> 0011 applies, version 11 is recorded, rows preserved
//
//	TestMigration0011_Transition_ViolatingHistory
//	    one historical row carries a formatted phone that only the WEAK
//	    constraint allowed -> 0011 FAILS CLOSED: the transaction rolls back,
//	    the schema and data are unchanged, and version 11 is NOT recorded.
//
// The second test is the important one. It demonstrates that 0011 is not
// guaranteed to apply to production, because production may hold rows the weak
// predicate admitted. Establishing whether it does requires a read-only
// production aggregate check, which is NOT performed here.
//
// Each test needs its OWN database. Requires DATABASE_URL. Skips when unset.

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// migPool provisions an ISOLATED database for one test and returns a pool
// connected to it.
//
// WHY THIS DOES NOT USE DATABASE_URL DIRECTLY. These tests assert that they just
// applied migrations 0001..0010 (`n == 10`). That is only true on a database
// that starts empty. The CI `api` job runs `go run ./cmd/ci-migrate` and THEN
// `go test ./...` against that one database, so by the time these tests run,
// version 11 is already applied and `migrate.Run` returns 0 — every migration
// test would fail with "expected 10 migrations applied, got 0". It also means
// two of these tests cannot share a database even outside CI, because the first
// one leaves version 11 recorded.
//
// So each test creates its own database from the server named by DATABASE_URL,
// and drops it on cleanup. DATABASE_URL is therefore a CONNECTION to a server,
// not a database under test. Requires a role with CREATEDB (the CI service user
// is the cluster superuser).
func migPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminURL := os.Getenv("DATABASE_URL")
	if adminURL == "" {
		t.Skip("DATABASE_URL not set; skipping migration-transition test")
	}

	base, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to server: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping server: %v", err)
	}
	t.Cleanup(admin.Close)

	// A distinct database per test. The suffix is random rather than derived
	// from t.Name() so a crashed earlier run cannot collide with this one.
	name := "sigap_mig_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]

	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("create isolated database %s: %v (does the DATABASE_URL role have CREATEDB?)", name, err)
	}

	child := *base
	child.Path = "/" + name
	pool, err := pgxpool.New(ctx, child.String())
	if err != nil {
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
		t.Fatalf("connect to isolated database %s: %v", name, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
		t.Fatalf("ping isolated database %s: %v", name, err)
	}

	t.Cleanup(func() {
		// Close the pool BEFORE dropping: a live connection would block the drop.
		pool.Close()
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		if _, err := admin.Exec(dctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`); err != nil {
			t.Logf("cleanup: drop database %s: %v", name, err)
		}
	})
	return pool
}

// stageMigrations copies migrations 0001..maxVersion into a temp dir so the
// runner can be pointed at a prefix of the real history.
func stageMigrations(t *testing.T, maxVersion int) string {
	t.Helper()
	src, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if migrate.ParseVersion(e.Name()) > maxVersion {
			continue
		}
		body, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), body, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
	return dst
}

func constraintDef(t *testing.T, pool *pgxpool.Pool, name string) (def string, valid bool) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT pg_get_constraintdef(oid), convalidated
		   FROM pg_constraint
		  WHERE conrelid = 'notification_outbox'::regclass AND conname = $1`, name).Scan(&def, &valid)
	if err != nil {
		t.Fatalf("read constraint %s: %v", name, err)
	}
	return def, valid
}

// buildProductionShape applies 0001..0010 through the real runner and then
// rewrites the two phone constraints into the pre-9d4e68e WEAK form, returning
// the observed weak definition.
func buildProductionShape(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()

	pre := stageMigrations(t, 10)
	n, err := migrate.Run(ctx, pool, pre)
	if err != nil {
		t.Fatalf("apply 0001..0010: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected 10 migrations applied, got %d", n)
	}

	weak := []string{
		`ALTER TABLE notification_outbox DROP CONSTRAINT IF EXISTS notification_outbox_no_raw_phone_in_subject_chk`,
		`ALTER TABLE notification_outbox DROP CONSTRAINT IF EXISTS notification_outbox_no_raw_phone_in_body_chk`,
		`ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk CHECK (subject !~ '[0-9]{8,}')`,
		`ALTER TABLE notification_outbox ADD CONSTRAINT notification_outbox_no_raw_phone_in_body_chk CHECK (body_template !~ '[0-9]{8,}')`,
	}
	for _, s := range weak {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("weaken constraint: %v", err)
		}
	}

	def, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")
	// The weak form is VALID — it is simply a weaker predicate. Weakness is the
	// absent second conjunct, not the absence of validation.
	if strings.Contains(def, "{10,}") {
		t.Fatalf("fixture is not weak: def=%q", def)
	}
	return def
}

func insertHistorical(t *testing.T, pool *pgxpool.Pool, subject, body string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO notification_outbox
		   (channel, template_key, subject, body_template, recipient_type,
		    recipient_contact_masked, recipient_contact_hash, status,
		    attempt_count, next_attempt_at)
		 VALUES ('dev', 'hist.' || substr(md5(random()::text), 1, 8), $1, $2, 'patient',
		         '+62••••1234', decode(repeat('00',32),'hex'), 'pending', 0, NOW())`,
		subject, body); err != nil {
		t.Fatalf("insert historical row: %v", err)
	}
}

func version11Count(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM schema_migrations WHERE version = 11`).Scan(&n); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return n
}

// TestMigration0011_Transition_CompatibleHistory is the happy path: every
// historical row satisfies the strengthened predicate, so 0011 converges.
func TestMigration0011_Transition_CompatibleHistory(t *testing.T) {
	pool := migPool(t)
	ctx := context.Background()

	weakDef := buildProductionShape(t, pool)
	t.Logf("reconstructed weak constraint: %s", weakDef)

	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Janji temu Anda berhasil dicatat. Kode check-in: AB12CD.")
	insertHistorical(t, pool, "Status Check-in Sigap", "Check-in Anda berhasil. Nomor antrean: FSK-0001.")
	insertHistorical(t, pool, "Kunjungan", "Kunjungan pada 9 Oktober 2026 pukul 14.30.")

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&before); err != nil {
		t.Fatalf("count rows: %v", err)
	}

	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve full migration dir: %v", err)
	}
	n, err := migrate.Run(ctx, pool, full)
	if err != nil {
		t.Fatalf("apply 0011 on compatible history: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 migration applied (0011), got %d", n)
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("schema_migrations version 11 rows = %d, want 1", got)
	}

	defS, validS := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")
	defB, validB := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if !validS || !validB {
		t.Fatalf("constraints not validated after 0011: subject=%v body=%v", validS, validB)
	}
	for _, d := range []string{defS, defB} {
		if !strings.Contains(d, "{10,}") || !strings.Contains(d, "{8,}") {
			t.Fatalf("constraint not strengthened: %q", d)
		}
	}

	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&after); err != nil {
		t.Fatalf("count rows after: %v", err)
	}
	if after != before {
		t.Fatalf("row count changed across 0011: %d -> %d", before, after)
	}

	// Legitimate writes still work.
	svc := NewService(pool)
	if _, err := svc.Enqueue(ctx, EnqueueInput{
		Channel:             ChannelDev,
		TemplateKey:         "transition.ok",
		Subject:             "Konfirmasi Janji Temu Sigap",
		BodyTemplate:        "Janji temu Anda berhasil dicatat. Kode check-in: {checkin_code}.",
		TemplateVars:        map[string]string{"checkin_code": "AB12CD"},
		RecipientType:       RecipientPatient,
		RecipientContact:    "+6281234567890",
		RelatedResourceType: "appointment",
		RelatedResourceID:   "00000000-0000-0000-0000-000000000001",
	}); err != nil {
		t.Fatalf("legitimate Enqueue after 0011 failed: %v", err)
	}

	// Phone-like content is refused by the application before it reaches SQL.
	if _, err := svc.Enqueue(ctx, EnqueueInput{
		Channel:             ChannelDev,
		TemplateKey:         "transition.bad",
		Subject:             "Status Check-in Sigap",
		BodyTemplate:        "Nomor antrean: {queue_number}.",
		TemplateVars:        map[string]string{"queue_number": "0812-3456-7890"},
		RecipientType:       RecipientPatient,
		RecipientContact:    "+6281234567890",
		RelatedResourceType: "appointment",
		RelatedResourceID:   "00000000-0000-0000-0000-000000000002",
	}); err == nil {
		t.Fatal("Enqueue accepted a formatted phone number in the body; want refusal")
	}

	var weakLeft int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_constraint
		  WHERE conrelid='notification_outbox'::regclass
		    AND conname LIKE '%phone%'
		    AND (pg_get_constraintdef(oid) NOT LIKE '%{10,}%' OR convalidated = false)`).Scan(&weakLeft); err != nil {
		t.Fatalf("classify: %v", err)
	}
	if weakLeft != 0 {
		t.Fatalf("%d phone constraint(s) still weak or unvalidated", weakLeft)
	}
}

// TestMigration0011_Transition_ViolatingHistory proves 0011 FAILS CLOSED when a
// historical row cannot satisfy the strengthened predicate. This is the path
// that decides whether the migration can be applied to production at all.
func TestMigration0011_Transition_ViolatingHistory(t *testing.T) {
	pool := migPool(t)
	ctx := context.Background()

	buildProductionShape(t, pool)

	// This body is representable ONLY under the weak form. The weak constraint
	// admits it; the strengthened one rejects it.
	insertHistorical(t, pool, "Riwayat lama", "Narahubung lama: 0812-3456-7890.")
	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Janji temu Anda berhasil dicatat. Kode check-in: AB12CD.")

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&before); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	defBefore, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")

	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve full migration dir: %v", err)
	}
	runErr := func() error {
		_, err := migrate.Run(ctx, pool, full)
		return err
	}()
	if runErr == nil {
		t.Fatal("0011 succeeded against a violating historical row; want a fail-closed error")
	}
	// Pin the CAUSE, not merely that something failed. Without this the test
	// would also pass if 0011 refused for an unrelated reason — its own
	// unrecognised-state RAISE, a syntax error, or a lock error.
	var pgErr *pgconn.PgError
	if !errors.As(runErr, &pgErr) {
		t.Fatalf("expected a PostgreSQL error, got %T: %v", runErr, runErr)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("expected SQLSTATE 23514 (check_violation from the ADD CONSTRAINT), got %s: %v", pgErr.Code, runErr)
	}
	if !strings.Contains(pgErr.Message, "notification_outbox_no_raw_phone_in_body_chk") {
		t.Fatalf("expected the failure to name the body constraint, got %q", pgErr.Message)
	}
	t.Logf("0011 refused as designed: %v", runErr)

	// --- no partial state -------------------------------------------------
	if got := version11Count(t, pool); got != 0 {
		t.Fatalf("version 11 recorded despite failure: %d rows (partial state)", got)
	}
	defAfter, validAfter := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if defAfter != defBefore {
		t.Fatalf("constraint definition changed despite failure:\n before=%q\n after =%q", defBefore, defAfter)
	}
	if strings.Contains(defAfter, "{10,}") {
		t.Fatalf("constraint was strengthened despite the failure: %q", defAfter)
	}
	_ = validAfter

	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&after); err != nil {
		t.Fatalf("count rows after: %v", err)
	}
	if after != before {
		t.Fatalf("rows changed despite failure: %d -> %d", before, after)
	}

	// The migration is retryable once the operator remediates the row.
	//
	// The remediation must be NON-DESTRUCTIVE and must actually satisfy the
	// predicate. Two traps this avoids:
	//
	//   * deleting the row would succeed, but notification records must never be
	//     deleted — that is a hard constraint of this change set, and the
	//     migration's own header declines to remediate rows at all; and
	//   * 'Narahubung lama: 0812 3456 7890.' is NOT a remediation: space is a
	//     member of the separator class, so that value still violates the
	//     strengthened predicate and the retry would fail again.
	//
	// So the operator rewrites the body into a form the predicate accepts — the
	// masked form, which is what the application-layer masking produces anyway.
	// The value is verified against the predicate BEFORE the retry, so the test
	// cannot pass because of an accidental acceptance.
	remediated := "Narahubung lama: +62••••1234."
	// The expression is `!~ c1 AND !~ c2`, i.e. it is TRUE when the value is
	// SAFE. Name the variable for what it holds.
	var safe bool
	if err := pool.QueryRow(ctx,
		`SELECT $1 !~ '[0-9]{8,}' AND $1 !~ '[0-9][0-9\-._() ]{10,}[0-9]'`,
		remediated).Scan(&safe); err != nil {
		t.Fatalf("evaluate remediation against the predicate: %v", err)
	}
	if !safe {
		t.Fatalf("test fixture is not a remediation: %q still violates the strengthened predicate", remediated)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE notification_outbox SET body_template = $1
		  WHERE body_template = 'Narahubung lama: 0812-3456-7890.'`, remediated); err != nil {
		t.Fatalf("operator remediation: %v", err)
	}

	// The row must still exist: remediation rewrites, it does not delete.
	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_outbox WHERE body_template = $1`, remediated).Scan(&remaining); err != nil {
		t.Fatalf("count remediated rows: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remediated rows = %d, want 1 (remediation must not delete the record)", remaining)
	}

	if _, err := migrate.Run(ctx, pool, full); err != nil {
		t.Fatalf("0011 should succeed after the operator remediates the violating row: %v", err)
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("version 11 rows = %d after successful retry, want 1", got)
	}
	// And the remediated record survived the migration.
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_outbox WHERE body_template = $1`, remediated).Scan(&remaining); err != nil {
		t.Fatalf("count remediated rows after migration: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remediated rows after migration = %d, want 1 (the migration must not rewrite or delete rows)", remaining)
	}
}

// TestMigration0011_Transition_MixedStateRefused covers the strongest
// fail-closed guard: a state the migration does not recognise must RAISE and
// change nothing. Here the subject constraint is strengthened while the body
// constraint is still weak, so neither the "already strengthened" nor the
// "weak" branch matches.
func TestMigration0011_Transition_MixedStateRefused(t *testing.T) {
	pool := migPool(t)
	ctx := context.Background()

	buildProductionShape(t, pool) // both constraints weak

	// Strengthen ONLY the subject constraint.
	if _, err := pool.Exec(ctx,
		`ALTER TABLE notification_outbox
		   DROP CONSTRAINT IF EXISTS notification_outbox_no_raw_phone_in_subject_chk`); err != nil {
		t.Fatalf("drop subject constraint: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`ALTER TABLE notification_outbox
		   ADD CONSTRAINT notification_outbox_no_raw_phone_in_subject_chk
		   CHECK (subject !~ $re$[0-9]{8,}$re$ AND subject !~ $re$[0-9][0-9\-._() ]{10,}[0-9]$re$)`); err != nil {
		t.Fatalf("strengthen subject constraint: %v", err)
	}

	subjBefore, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")
	bodyBefore, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")

	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve full migration dir: %v", err)
	}
	_, runErr := migrate.Run(ctx, pool, full)
	if runErr == nil {
		t.Fatal("0011 accepted a mixed strong/weak state; want a fail-closed RAISE")
	}
	// The migration's own RAISE is SQLSTATE P0001, not the 23514 an ADD failure
	// would produce — so this distinguishes "refused to guess" from "tried and
	// the rows violated it".
	var pgErr *pgconn.PgError
	if !errors.As(runErr, &pgErr) || pgErr.Code != "P0001" {
		t.Fatalf("expected the migration's own RAISE (SQLSTATE P0001), got %v", runErr)
	}
	if got := version11Count(t, pool); got != 0 {
		t.Fatalf("version 11 recorded despite the refusal: %d rows", got)
	}
	subjAfter, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")
	bodyAfter, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_body_chk")
	if subjAfter != subjBefore || bodyAfter != bodyBefore {
		t.Fatalf("constraints changed despite the refusal:\n subject %q -> %q\n body    %q -> %q",
			subjBefore, subjAfter, bodyBefore, bodyAfter)
	}
}

// TestMigration0011_Transition_AbsentConstraintsAdded covers the third
// recognised state: both phone constraints missing. The migration must ADD them
// strengthened rather than refuse.
func TestMigration0011_Transition_AbsentConstraintsAdded(t *testing.T) {
	pool := migPool(t)
	ctx := context.Background()

	buildProductionShape(t, pool)
	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Kode check-in: AB12CD.")

	for _, c := range []string{
		"notification_outbox_no_raw_phone_in_subject_chk",
		"notification_outbox_no_raw_phone_in_body_chk",
	} {
		if _, err := pool.Exec(ctx, `ALTER TABLE notification_outbox DROP CONSTRAINT IF EXISTS `+c); err != nil {
			t.Fatalf("drop %s: %v", c, err)
		}
	}

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&before); err != nil {
		t.Fatalf("count rows: %v", err)
	}

	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve full migration dir: %v", err)
	}
	if _, err := migrate.Run(ctx, pool, full); err != nil {
		t.Fatalf("0011 should ADD absent constraints, got: %v", err)
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("version 11 rows = %d, want 1", got)
	}
	for _, c := range []string{
		"notification_outbox_no_raw_phone_in_subject_chk",
		"notification_outbox_no_raw_phone_in_body_chk",
	} {
		def, valid := constraintDef(t, pool, c)
		if !valid {
			t.Errorf("%s is not validated after being added", c)
		}
		if !strings.Contains(def, "{10,}") {
			t.Errorf("%s was added without the strengthened conjunct: %q", c, def)
		}
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&after); err != nil {
		t.Fatalf("count rows after: %v", err)
	}
	if after != before {
		t.Fatalf("row count changed: %d -> %d", before, after)
	}
}
