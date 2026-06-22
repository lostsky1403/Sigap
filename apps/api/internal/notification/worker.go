package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Worker drains the notification_outbox table by selecting rows whose
// `next_attempt_at` is due, claiming them with SELECT ... FOR UPDATE
// SKIP LOCKED, and handing them to the existing DevProvider.
//
// Worker is intentionally small:
//
//   - It does NOT maintain its own delivery queue.
//   - It does NOT introduce new columns or a new migration.
//   - It does NOT touch the raw patient contact (which is never in
//     notification_outbox).
//   - It only ever logs ids, template keys, status transitions, and
//     error codes — never the rendered body or any PII.
//
// Concurrency: two workers running at the same time will not
// double-process a row. The SELECT FOR UPDATE SKIP LOCKED clause
// ensures that a row currently being claimed by worker A is invisible
// to worker B until worker A's claim transaction commits.
type Worker struct {
	pool     *pgxpool.Pool
	provider Provider // dev provider in this milestone; a future vendor provider implements the same interface.
	// RenderSubject / RenderBody produce the (already-stored or
	// freshly-rendered) subject and body. Both default to identity
	// (return the row's stored subject/body) but can be overridden in
	// tests. The default path is what the worker uses.
	RenderSubject func(*Service, OutboxRow) (string, error)
	RenderBody    func(*Service, OutboxRow) (string, error)
}

// WorkerOption configures a Worker at construction time.
type WorkerOption func(*Worker)

// WithRenderer overrides the render hooks. Mostly useful in tests.
func WithRenderer(renderSubject, renderBody func(*Service, OutboxRow) (string, error)) WorkerOption {
	return func(w *Worker) {
		w.RenderSubject = renderSubject
		w.RenderBody = renderBody
	}
}

// NewWorker constructs a Worker bound to the given pool and provider.
// `svc` is used to render templates via the existing Service
// primitives (so RenderTemplate is the only place where substitution
// happens). svc may be nil if the caller intends to set its own render
// hooks via WithRenderer.
func NewWorker(pool *pgxpool.Pool, provider Provider, svc *Service, opts ...WorkerOption) *Worker {
	if pool == nil {
		panic("notification.NewWorker: pool is nil")
	}
	if provider == nil {
		panic("notification.NewWorker: provider is nil")
	}
	w := &Worker{pool: pool, provider: provider}
	// Default render hooks: the rendered subject and body are the
	// row's stored subject and body_template. The worker does NOT
	// substitute placeholders itself — the template engine is the
	// Service.RenderTemplate caller, and callers upstream are
	// expected to pass already-substituted subjects/body when
	// enqueueing. This keeps the worker free of any PII lookup.
	if svc != nil {
		w.RenderSubject = func(_ *Service, row OutboxRow) (string, error) { return row.Subject, nil }
		w.RenderBody = func(_ *Service, row OutboxRow) (string, error) { return row.BodyTemplate, nil }
	} else {
		w.RenderSubject = func(_ *Service, row OutboxRow) (string, error) { return row.Subject, nil }
		w.RenderBody = func(_ *Service, row OutboxRow) (string, error) { return row.BodyTemplate, nil }
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// MaxAttempts is the cap on per-row delivery attempts. After
// MaxAttempts failures, the row is moved to status='failed' and
// no further automatic retry is scheduled.
const MaxAttempts = 3

// backoffSchedule returns the duration to wait before the next attempt
// given the attempt count that just FAILED. The schedule is fixed:
//
//	attempt 1 -> 1 minute
//	attempt 2 -> 5 minutes
//	attempt 3 -> 15 minutes (reserved; MaxAttempts=3 means we
//	                 stop after attempt 3, so this entry is only
//	                 used if MaxAttempts is raised in the future)
//
// The schedule is intentionally hard-coded and exported for tests.
func backoffSchedule(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

// BackoffFor is the exported form of backoffSchedule so tests can
// assert the schedule without depending on private internals.
func BackoffFor(attempt int) time.Duration { return backoffSchedule(attempt) }

// RunOnce drains up to batchSize due rows. It returns the number of
// rows processed (claimed + delivered-or-failed or skipped) and the
// first error encountered, if any. Per-row errors are logged via
// slog.Warn and the loop continues; RunOnce only returns a non-nil
// error if the initial SELECT itself fails.
func (w *Worker) RunOnce(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 25
	}

	processed := 0
	for i := 0; i < batchSize; i++ {
		// Claim one row per iteration. SKIP LOCKED means a row that
		// is currently locked by another worker (or by our own
		// in-flight claim) is invisible; if no row is due, we exit
		// early.
		row, ok, err := w.claim(ctx)
		if err != nil {
			return processed, fmt.Errorf("worker: claim: %w", err)
		}
		if !ok {
			// No more due rows.
			break
		}
		w.processRow(ctx, row)
		processed++
	}
	return processed, nil
}

// claim selects one due row, transitions it to status='processing',
// sets next_attempt_at to a short safety window (so a crashed worker
// eventually recovers the row), and returns the row data. Returns
// ok=false when no due rows exist.
func (w *Worker) claim(ctx context.Context) (OutboxRow, bool, error) {
	const claimSQL = `
UPDATE notification_outbox
SET status = 'processing',
    next_attempt_at = NOW() + INTERVAL '15 minutes'
WHERE id = (
    SELECT id FROM notification_outbox
    WHERE status = 'pending' AND next_attempt_at <= NOW()
    ORDER BY next_attempt_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, facility_id, channel, template_key, subject, body_template,
          recipient_type, recipient_contact_masked, status, attempt_count,
          next_attempt_at, last_error_code, related_resource_type,
          related_resource_id, created_at, updated_at`

	var row OutboxRow
	var fac *uuid.UUID
	var lastErr, relType, relID *string

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return OutboxRow{}, false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := tx.QueryRow(ctx, claimSQL).Scan(
		&row.ID, &fac, &row.Channel, &row.TemplateKey, &row.Subject, &row.BodyTemplate,
		&row.RecipientType, &row.RecipientContactMasked, &row.Status, &row.AttemptCount,
		&row.NextAttemptAt, &lastErr, &relType, &relID, &row.CreatedAt, &row.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No due row. Roll back the empty tx and return ok=false.
			return OutboxRow{}, false, nil
		}
		return OutboxRow{}, false, err
	}
	row.FacilityID = fac
	row.LastErrorCode = lastErr
	row.RelatedResourceType = relType
	row.RelatedResourceID = relID

	if err := tx.Commit(ctx); err != nil {
		return OutboxRow{}, false, fmt.Errorf("commit: %w", err)
	}
	return row, true, nil
}

// processRow renders subject/body, calls the provider, and applies
// post-delivery state-machine transitions. Errors are logged via
// slog.Warn with no PII; the row is left in whatever status the
// provider set.
//
// Note on rendering: at this milestone, the default render hooks
// are identity (they return the row's stored Subject and
// BodyTemplate). The renderer API is exported (`RenderTemplate`,
// `RenderSubject`, `RenderBody`) so a future phase can pre-render
// at Enqueue time without changing the worker's shape. The worker
// itself never substitutes placeholders; substitution is the
// caller's responsibility (because substitution requires an
// `appointment` / `queue` lookup which the worker does not perform).
func (w *Worker) processRow(ctx context.Context, row OutboxRow) {
	if w.RenderSubject != nil {
		if s, err := w.RenderSubject(nil, row); err == nil && s != "" {
			row.Subject = s
		}
	}
	if w.RenderBody != nil {
		if b, err := w.RenderBody(nil, row); err == nil && b != "" {
			row.BodyTemplate = b
		}
	}
	// Pass the (possibly overridden) row to the provider. The
	// provider does not consume the raw contact (which is never in
	// the row) and writes a single notification_delivery_attempts row
	// plus an UPDATE on notification_outbox.status / attempt_count /
	// next_attempt_at / last_error_code.
	attempt, err := w.provider.Deliver(ctx, row)
	if err != nil {
		slog.Warn("worker: provider.Deliver error",
			"notification_id", row.ID.String(),
			"template_key", row.TemplateKey,
			"err", err.Error())
		return
	}

	// The provider has already updated the row to delivered / failed
	// and incremented attempt_count. We now apply the backoff
	// schedule if (a) the provider failed and (b) we still have
	// attempts left under MaxAttempts.
	if attempt.Status != StatusFailed {
		// Delivered: nothing to schedule.
		slog.Info("worker: delivered",
			"notification_id", row.ID.String(),
			"template_key", row.TemplateKey,
			"attempt_number", attempt.AttemptNumber)
		return
	}

	// attempt.AttemptNumber is the just-completed attempt (1-based).
	// If it is >= MaxAttempts, the row is terminal 'failed'. Otherwise
	// schedule a retry.
	if attempt.AttemptNumber >= MaxAttempts {
		slog.Warn("worker: terminal failure (max attempts reached)",
			"notification_id", row.ID.String(),
			"template_key", row.TemplateKey,
			"attempt_number", attempt.AttemptNumber)
		return
	}

	delay := backoffSchedule(attempt.AttemptNumber)
	if err := w.scheduleRetry(ctx, row.ID, delay); err != nil {
		slog.Warn("worker: schedule retry error",
			"notification_id", row.ID.String(),
			"template_key", row.TemplateKey,
			"err", err.Error())
		return
	}
	slog.Info("worker: retry scheduled",
		"notification_id", row.ID.String(),
		"template_key", row.TemplateKey,
		"attempt_number", attempt.AttemptNumber,
		"next_attempt_in", delay.String())
}

// scheduleRetry resets a row to status='pending' with a new
// next_attempt_at.
func (w *Worker) scheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) error {
	const sql = `
UPDATE notification_outbox
SET status = 'pending',
    next_attempt_at = NOW() + ($2 || ' seconds')::interval,
    updated_at = NOW()
WHERE id = $1`
	_, err := w.pool.Exec(ctx, sql, id, fmt.Sprintf("%d", int(delay.Seconds())))
	return err
}

// Run loops every interval until ctx is cancelled. Each tick calls
// RunOnce. Errors are logged via slog.Warn and the loop continues.
func (w *Worker) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := w.RunOnce(ctx, 25)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				slog.Warn("worker: RunOnce error", "err", err.Error())
				continue
			}
			if n == 0 {
				slog.Debug("worker: idle tick", "interval", interval.String())
			} else {
				slog.Info("worker: tick processed", "count", n)
			}
		}
	}
}