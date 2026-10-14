package notification

// Lock and timeout assessment for migration 0011.
//
// 0011 converges the two phone constraints on notification_outbox by
// DROP CONSTRAINT + ADD CONSTRAINT ... CHECK. In PostgreSQL both of those take
// an ACCESS EXCLUSIVE lock on the table, and ADD CONSTRAINT with validation
// then scans every row while holding it. ACCESS EXCLUSIVE conflicts with every
// other lock mode, so while the migration runs:
//
//   * no reader can SELECT from notification_outbox
//   * no writer can INSERT or UPDATE it — including the notification worker's
//     status/attempt_count UPDATE and any Enqueue INSERT
//
// These tests establish that behaviour against a real PostgreSQL instance and
// prove the failure modes, rather than asserting it from documentation:
//
//   TestMigration0011_BlocksOnConcurrentReader
//       an open read transaction holds ACCESS SHARE -> migrate.Run does not
//       complete while it is open, and completes once it is released
//
//   TestMigration0011_BlocksOnConcurrentWriter
//       an open UPDATE transaction holds ROW EXCLUSIVE -> same blocking
//
//   TestMigration0011_LockTimeoutFailsClosed
//       with lock_timeout set, the migration abandons the lock wait with
//       SQLSTATE 55P03 (lock_not_available) and leaves NO partial state:
//       constraint definitions unchanged, version 11 NOT recorded
//
// The runner (migrate.Run) sets neither lock_timeout nor statement_timeout and
// contains no retry loop, so an unbounded wait is the default and a lock wait
// is bounded only by the caller's context. That is why the timeout behaviour is
// demonstrated explicitly: it is the operator's control, not the runner's.
//
// Each test needs its OWN database. Requires DATABASE_URL. Skips when unset.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/migrate"
)

// holdLock opens a transaction on a dedicated connection, runs `stmt` inside
// it, and returns a release function. The lock is held until release is called.
func holdLock(t *testing.T, pool *pgxpool.Pool, stmt string) func() {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire blocker connection: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		t.Fatalf("begin blocker tx: %v", err)
	}
	if _, err := tx.Exec(ctx, stmt); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("blocker statement %q: %v", stmt, err)
	}
	var released bool
	return func() {
		if released {
			return
		}
		released = true
		_ = tx.Rollback(context.Background())
		conn.Release()
	}
}

// runMigrationAsync starts migrate.Run over the full history and returns a
// channel carrying the result. The caller must release any blocker to let it
// finish.
func runMigrationAsync(t *testing.T, pool *pgxpool.Pool) <-chan error {
	t.Helper()
	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve full migration dir: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_, err := migrate.Run(ctx, pool, full)
		done <- err
	}()
	return done
}

// pendingAccessExclusive reports how many ungranted ACCESS EXCLUSIVE lock
// requests exist on notification_outbox. It reads pg_locks, which takes no lock
// on the table, so it can be used while the migration is blocked.
//
// This must NOT be replaced with a SELECT against notification_outbox:
// PostgreSQL's lock queue is FIFO, so once the migration's ACCESS EXCLUSIVE
// request is queued, every later lock request on that table queues BEHIND it.
// A plain read here would deadlock the test — which is itself the finding
// recorded in the runbook: the table is unavailable from the moment the ALTER
// requests the lock, not from the moment it acquires it.
func pendingAccessExclusive(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*)
		   FROM pg_locks l
		   JOIN pg_class c ON c.oid = l.relation
		  WHERE c.relname = 'notification_outbox'
		    AND l.mode = 'AccessExclusiveLock'
		    AND NOT l.granted`).Scan(&n); err != nil {
		t.Fatalf("read pg_locks: %v", err)
	}
	return n
}

// waitForPendingLock polls pg_locks until the migration's ACCESS EXCLUSIVE
// request is queued, or the deadline passes.
//
// This replaces a fixed sleep. The migration goroutine must acquire a pool
// connection, begin a transaction and run the DO block's prologue before it
// reaches the first ALTER, and none of that is bounded by a constant — on a
// loaded runner a fixed window would report "the migration is not blocked"
// for a migration that is merely slow. Polling keeps the assertion just as
// strong (it still requires the ungranted lock to exist) without racing a
// guessed duration.
func waitForPendingLock(t *testing.T, pool *pgxpool.Pool, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if pendingAccessExclusive(t, pool) >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no ungranted AccessExclusiveLock on notification_outbox within 15s; "+
				"the migration is not blocked on the %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestMigration0011_BlocksOnConcurrentReader(t *testing.T) {
	pool := migPool(t)
	buildProductionShape(t, pool)
	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Kode check-in: AB12CD.")

	// A plain SELECT inside an open transaction is enough: it takes ACCESS
	// SHARE, which conflicts with the ACCESS EXCLUSIVE 0011 needs.
	release := holdLock(t, pool, `SELECT count(*) FROM notification_outbox`)

	done := runMigrationAsync(t, pool)

	select {
	case err := <-done:
		release()
		t.Fatalf("migration completed while a reader held ACCESS SHARE (err=%v); "+
			"expected it to block", err)
	case <-time.After(800 * time.Millisecond):
		// Blocked, as expected.
	}

	// Prove the wait is real rather than a scheduling artefact: the migration's
	// ACCESS EXCLUSIVE request must be visible in pg_locks as ungranted.
	waitForPendingLock(t, pool, "reader")

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("migration failed after the reader released: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("migration did not complete after the reader released")
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("version 11 rows = %d after unblocked migration, want 1", got)
	}
}

func TestMigration0011_BlocksOnConcurrentWriter(t *testing.T) {
	pool := migPool(t)
	buildProductionShape(t, pool)
	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Kode check-in: AB12CD.")

	// An UPDATE holds ROW EXCLUSIVE, which also conflicts with ACCESS
	// EXCLUSIVE. This is the notification worker's real traffic shape: it
	// UPDATEs status/attempt_count/next_attempt_at.
	release := holdLock(t, pool,
		`UPDATE notification_outbox SET attempt_count = attempt_count WHERE false`)

	done := runMigrationAsync(t, pool)
	select {
	case err := <-done:
		release()
		t.Fatalf("migration completed while a writer held ROW EXCLUSIVE (err=%v)", err)
	case <-time.After(800 * time.Millisecond):
	}

	waitForPendingLock(t, pool, "writer")

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("migration failed after the writer released: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("migration did not complete after the writer released")
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("version 11 rows = %d, want 1", got)
	}
}

func TestMigration0011_LockTimeoutFailsClosed(t *testing.T) {
	pool := migPool(t)
	buildProductionShape(t, pool)
	insertHistorical(t, pool, "Konfirmasi Janji Temu Sigap", "Kode check-in: AB12CD.")

	defBefore, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")

	release := holdLock(t, pool, `SELECT count(*) FROM notification_outbox`)
	defer release()

	ctx := context.Background()

	// The timeout must be applied to the RUNNER's connections, not to a
	// hand-rolled transaction, because the "no partial state" claim is about the
	// version row — and the ONLY code that writes a version-11 row is
	// migrate.Run. Executing the migration body directly would leave
	// schema_migrations at versions 1..10 by construction, so a
	// `version11Count == 0` assertion after it would pass no matter what the
	// migration did. It would be a check that cannot fail.
	//
	// ALTER DATABASE ... SET applies to connections opened afterwards, so the
	// pool is rebuilt after it. The existing pool keeps its old setting, which
	// is why it is not reused here.
	dbName := pool.Config().ConnConfig.Database
	if _, err := pool.Exec(ctx, `ALTER DATABASE "`+dbName+`" SET lock_timeout = '300ms'`); err != nil {
		t.Fatalf("set lock_timeout on the database: %v", err)
	}

	timedPool, err := newPoolWithRuntimeParam(t, pool.Config().ConnString(), "lock_timeout", "300ms")
	if err != nil {
		t.Fatalf("open pool with lock_timeout: %v", err)
	}
	t.Cleanup(timedPool.Close)

	full, err := migrate.MigrateDir()
	if err != nil {
		t.Fatalf("resolve migration dir: %v", err)
	}

	_, runErr := migrate.Run(ctx, timedPool, full)
	if runErr == nil {
		t.Fatal("0011 acquired the lock and ran despite an open ACCESS SHARE holder; want lock_timeout")
	}
	var pgErr *pgconn.PgError
	if !errors.As(runErr, &pgErr) {
		t.Fatalf("expected a PostgreSQL error, got %T: %v", runErr, runErr)
	}
	if pgErr.Code != "55P03" {
		t.Fatalf("expected SQLSTATE 55P03 (lock_not_available), got %s: %v", pgErr.Code, runErr)
	}
	// The message TEXT is deliberately not asserted: PostgreSQL renders it from
	// the server's lc_messages catalog, so the same condition reads "canceling
	// statement due to lock timeout" or "pembatalan perintah karena kunci
	// kehabisan waktu tunggu" depending on the cluster's locale. SQLSTATE is the
	// stable contract and is asserted above.
	t.Logf("lock timeout reported (localized message): %s", pgErr.Message)

	// No partial state. This is now a real assertion: the runner DID attempt the
	// version-11 INSERT, and it must not have committed.
	if got := version11Count(t, pool); got != 0 {
		t.Fatalf("version 11 recorded despite the lock timeout: %d rows", got)
	}
	defAfter, _ := constraintDef(t, pool, "notification_outbox_no_raw_phone_in_subject_chk")
	if defAfter != defBefore {
		t.Fatalf("constraint changed despite the lock timeout:\n before=%q\n after =%q", defBefore, defAfter)
	}
	if strings.Contains(defAfter, "{10,}") {
		t.Fatalf("constraint was strengthened despite the lock timeout: %q", defAfter)
	}

	// The migration is retryable once the lock is free — this is the manual
	// retry an operator performs, NOT an automatic one: the runner has no
	// retry loop. Run it on the original pool (no lock_timeout) so the retry is
	// not itself racing the setting under test.
	release()
	if _, err := migrate.Run(context.Background(), pool, full); err != nil {
		t.Fatalf("0011 should succeed after the lock is released: %v", err)
	}
	if got := version11Count(t, pool); got != 1 {
		t.Fatalf("version 11 rows = %d after retry, want 1", got)
	}
}

// newPoolWithRuntimeParam opens a pool whose connections carry a runtime
// parameter (e.g. lock_timeout). Connection-level rather than session-level so
// every connection the pool hands out is bounded, not just the first.
func newPoolWithRuntimeParam(t *testing.T, dsn, key, value string) (*pgxpool.Pool, error) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams[key] = value
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return pgxpool.NewWithConfig(ctx, cfg)
}
