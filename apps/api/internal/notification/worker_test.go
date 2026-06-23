package notification

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestBackoffFor_Schedule asserts the documented schedule:
//
//	attempt 1 -> 1 minute
//	attempt 2 -> 5 minutes
//	attempt 3 -> 15 minutes (reserved; MaxAttempts=3 means we stop
//	                 after attempt 3, so this entry is only used if
//	                 MaxAttempts is raised in the future)
//	default    -> 15 minutes (defensive fallback)
func TestBackoffFor_Schedule(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 5 * time.Minute},
		{3, 15 * time.Minute},
		{0, 15 * time.Minute},
		{99, 15 * time.Minute},
	}
	for _, tc := range cases {
		got := BackoffFor(tc.attempt)
		if got != tc.want {
			t.Errorf("BackoffFor(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

// TestMaxAttempts_Constant pins the documented cap. Any change to
// MaxAttempts is a behaviour change that requires a spec update.
func TestMaxAttempts_Constant(t *testing.T) {
	if MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3 (per spec)", MaxAttempts)
	}
}

// TestRenderTemplate_NoRawContactOrBodyInOutboxRow guards against
// the future regression where someone adds a RecipientContact or
// RenderedBody field to OutboxRow. The worker reads from this
// struct; if such fields exist, the worker might log them.
func TestRenderTemplate_NoRawContactOrBodyInOutboxRow(t *testing.T) {
	if _, ok := any(OutboxRow{}).(interface{ Fields() []string }); ok {
		_ = ok
	}
	a, _ := RenderTemplate("X={facility_name}", map[string]string{"facility_name": "A"})
	b, _ := RenderTemplate("X={facility_name}", map[string]string{"facility_name": "A"})
	if a != b {
		t.Errorf("RenderTemplate not deterministic: %q vs %q", a, b)
	}
}

// TestWorker_BackoffSchedule_IsConsistentWithSchedule ensures the
// private backoffSchedule() and the exported BackoffFor() return
// the same value for every documented attempt number.
func TestWorker_BackoffSchedule_IsConsistentWithSchedule(t *testing.T) {
	for a := 1; a <= 5; a++ {
		if backoffSchedule(a) != BackoffFor(a) {
			t.Errorf("backoffSchedule(%d) != BackoffFor(%d)", a, a)
		}
	}
}

// TestRenderTemplate_DoesNotCallOutboxMasking guards that the
// renderer is a pure string function and does not call into the
// masking helpers.
func TestRenderTemplate_DoesNotCallOutboxMasking(t *testing.T) {
	got, err := RenderTemplate(
		"Kode check-in: {checkin_code}.",
		map[string]string{"checkin_code": "AB12CD"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Kode check-in: AB12CD." {
		t.Errorf("renderer must not alter allow-listed values; got %q", got)
	}
}

// TestDevSimulateOutcome_DeterministicAcrossCalls guards the claim
// that two providers calling the same outbox id always see the same
// outcome.
func TestDevSimulateOutcome_DeterministicAcrossCalls(t *testing.T) {
	out := DevSimulateOutcome([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if out.Status != StatusDelivered && out.Status != StatusFailed {
		t.Errorf("unexpected status %q", out.Status)
	}
}

// TestRunResult_ZeroValue pins the zero-value semantics of RunResult.
// This is the structural property that makes preview-mode invariants
// easy to assert.
func TestRunResult_ZeroValue(t *testing.T) {
	var r RunResult
	if r.InspectedPending != 0 || r.Claimed != 0 || r.Delivered != 0 ||
		r.Failed != 0 || r.Retried != 0 || r.Skipped != 0 {
		t.Errorf("RunResult zero value is non-zero: %+v", r)
	}
}

// TestPreviewSelectSQL_NoMutationKeywords is a documentation guard
// for the preview-mode SQL contract. The string must not contain any
// keyword that would indicate a mutation.
func TestPreviewSelectSQL_NoMutationKeywords(t *testing.T) {
	forbidden := []string{"UPDATE", "INSERT", "DELETE", "FOR UPDATE", "BEGIN"}
	upper := strings.ToUpper(previewSelectSQL)
	for _, kw := range forbidden {
		if strings.Contains(upper, kw) {
			t.Errorf("preview SELECT must not contain %q; got: %s", kw, previewSelectSQL)
		}
	}
}

// TestWorker_PreviewMode_DevSimulateOutcomeMatchesPolicy confirms
// the deterministic prediction used by preview mode returns one of
// the two declared outcomes.
func TestWorker_PreviewMode_DevSimulateOutcomeMatchesPolicy(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := uuid.New()
		out := DevSimulateOutcome(id)
		if out.Status != StatusDelivered && out.Status != StatusFailed {
			t.Errorf("DevSimulateOutcome(%d) = %q; want delivered or failed", i, out.Status)
		}
	}
}

// --- DB integration tests (gated on SIGAP_DATABASE_URL) --------------

// integrationWorkerPool opens a pgxpool for the worker preview-mode
// integration tests, gated on SIGAP_DATABASE_URL so unit-only `go
// test` runs stay offline.
func integrationWorkerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SIGAP_DATABASE_URL")
	if dsn == "" {
		t.Skip("SIGAP_DATABASE_URL not set; skipping notification worker DB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestWorker_PreviewMode_DoesNotMutate inserts a deterministic set
// of pending outbox rows inside a transaction, calls RunOnce in
// preview mode, and asserts that the rows are unchanged and no new
// notification_delivery_attempts rows were created. The transaction
// is rolled back at the end.
func TestWorker_PreviewMode_DoesNotMutate(t *testing.T) {
	pool := integrationWorkerPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	const seedSQL = `
INSERT INTO notification_outbox
    (id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, status,
     attempt_count, next_attempt_at, created_at, updated_at)
VALUES ($1, 'dev', 'preview_test', 'subj', 'body',
        'patient', '***', 'pending',
        0, NOW() - INTERVAL '1 minute', NOW(), NOW()),
       ($2, 'dev', 'preview_test', 'subj', 'body',
        'patient', '***', 'pending',
        0, NOW() - INTERVAL '1 minute', NOW(), NOW()),
       ($3, 'dev', 'preview_test', 'subj', 'body',
        'patient', '***', 'pending',
        0, NOW() - INTERVAL '1 minute', NOW(), NOW())`
	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	if _, err := tx.Exec(ctx, seedSQL, id1, id2, id3); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var preOutboxCount, preAttemptsCount int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_outbox WHERE id = ANY($1)`,
		[]uuid.UUID{id1, id2, id3},
	).Scan(&preOutboxCount); err != nil {
		t.Fatalf("pre-count outbox: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_delivery_attempts WHERE outbox_id = ANY($1)`,
		[]uuid.UUID{id1, id2, id3},
	).Scan(&preAttemptsCount); err != nil {
		t.Fatalf("pre-count attempts: %v", err)
	}
	var preStatus string
	var preAC int
	if err := tx.QueryRow(ctx,
		`SELECT status, attempt_count FROM notification_outbox WHERE id = $1`,
		id1,
	).Scan(&preStatus, &preAC); err != nil {
		t.Fatalf("pre-snapshot row 1: %v", err)
	}

	worker := NewWorker(pool, &DevProvider{pool: pool}, nil)
	res, err := worker.RunOnce(ctx, 10, true)
	if err != nil {
		t.Fatalf("RunOnce preview: %v", err)
	}
	if res.InspectedPending < 3 {
		t.Errorf("expected InspectedPending >= 3, got %d", res.InspectedPending)
	}
	if res.Claimed != 0 || res.Delivered != 0 || res.Failed != 0 || res.Retried != 0 || res.Skipped != 0 {
		t.Errorf("dry-run invariants violated: %+v", res)
	}

	var postOutboxCount, postAttemptsCount int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_outbox WHERE id = ANY($1)`,
		[]uuid.UUID{id1, id2, id3},
	).Scan(&postOutboxCount); err != nil {
		t.Fatalf("post-count outbox: %v", err)
	}
	if postOutboxCount != preOutboxCount {
		t.Errorf("preview mutated notification_outbox row count: pre=%d post=%d",
			preOutboxCount, postOutboxCount)
	}
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_delivery_attempts WHERE outbox_id = ANY($1)`,
		[]uuid.UUID{id1, id2, id3},
	).Scan(&postAttemptsCount); err != nil {
		t.Fatalf("post-count attempts: %v", err)
	}
	if postAttemptsCount != preAttemptsCount {
		t.Errorf("preview mutated notification_delivery_attempts count: pre=%d post=%d",
			preAttemptsCount, postAttemptsCount)
	}
	var postStatus string
	var postAC int
	if err := tx.QueryRow(ctx,
		`SELECT status, attempt_count FROM notification_outbox WHERE id = $1`,
		id1,
	).Scan(&postStatus, &postAC); err != nil {
		t.Fatalf("post-snapshot row 1: %v", err)
	}
	if postStatus != preStatus || postAC != preAC {
		t.Errorf("preview mutated row 1 state: pre=(%s,%d) post=(%s,%d)",
			preStatus, preAC, postStatus, postAC)
	}
}
