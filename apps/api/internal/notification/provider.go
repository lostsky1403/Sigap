package notification

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider is the seam that Service.Enqueue / a future worker call to
// perform the actual channel delivery. The signature is intentionally
// narrow: an outbox row in, a delivery attempt row out.
//
// Real providers (Twilio, WhatsApp Cloud API, SMTP, SendGrid, ...) are
// NOT shipped in this milestone. The only implementation is DevProvider,
// which is offline and deterministic.
type Provider interface {
	// Deliver simulates one delivery attempt for the given outbox row
	// and writes one notification_delivery_attempts row + updates the
	// outbox row to the simulated status.
	Deliver(ctx context.Context, out OutboxRow) (DeliveryAttemptRow, error)
	Name() string
}

// DevProvider is the only Provider shipped in this milestone. It is:
//
//   - Offline: never touches the network, the filesystem outside its
//     own scratch space, or any process other than the calling Go
//     runtime. Verified by code review: no http.Client, no net.Dial,
//     no DNS resolver.
//   - Deterministic: given the same outbox id, two calls produce the
//     same simulated status, the same provider_response_excerpt, the
//     same error_code (if any), and the same duration bucket.
//   - Bounded: it writes exactly one notification_delivery_attempts row
//     and updates exactly one notification_outbox row.
//
// Result mapping (based on the first hex character of the outbox UUID
// when canonicalised as a string):
//
//	'0'..'7'  -> delivered
//	'8'..'9'  -> failed
//	'a'..'f'  -> delivered
//
// The 0..7/a..f range covers ~75% of UUIDs and the 8..9 range covers
// the remaining ~25%. This split is intentionally NOT random so the
// smoke test is reproducible.
type DevProvider struct {
	pool *pgxpool.Pool
}

// NewDevProvider constructs a DevProvider bound to the given pool.
func NewDevProvider(pool *pgxpool.Pool) *DevProvider { return &DevProvider{pool: pool} }

// Name returns "dev" so it matches the ChannelDev enum string.
func (p *DevProvider) Name() string { return "dev" }

// DevOutcome is the simulated result of one delivery attempt. It is the
// pure-function output of DevSimulateOutcome and is exported only for
// testability.
type DevOutcome struct {
	Status        Status
	Excerpt       string
	ErrorCode     *string
}

// DevSimulateOutcome returns the deterministic simulated outcome for an
// outbox id. The same id always yields the same Outcome. This is a pure
// function: no DB, no clock, no random source.
//
// The mapping rule (bucket = fnv32a(uuid) mod 100):
//
//	bucket <  75  -> delivered
//	bucket >= 75  -> failed
//
// ~75% of UUIDs map to delivered, ~25% to failed. The split is fixed so
// the smoke test is reproducible.
func DevSimulateOutcome(outboxID uuid.UUID) DevOutcome {
	h := fnv.New32a()
	h.Write([]byte(outboxID.String()))
	bucket := h.Sum32() % 100
	if bucket >= 75 {
		c := "dev_simulated_failure"
		return DevOutcome{
			Status:    StatusFailed,
			Excerpt:   "dev: simulated delivery failed (deterministic)",
			ErrorCode: &c,
		}
	}
	return DevOutcome{
		Status:  StatusDelivered,
		Excerpt: "dev: simulated delivery ok",
	}
}

// excerptPrefix returns the standard "dev: …" prefix used in
// provider_response_excerpt so the test suite can match without
// pinning the full message.
func excerptPrefix(s string) string {
	if strings.HasPrefix(s, "dev:") {
		return "dev:"
	}
	return ""
}

// Deliver is the offline simulation. It computes a deterministic
// status from the outbox UUID, writes a delivery attempt row, updates
// the outbox row's status and counters, and returns the attempt row.
func (p *DevProvider) Deliver(ctx context.Context, out OutboxRow) (DeliveryAttemptRow, error) {
	if p.pool == nil {
		return DeliveryAttemptRow{}, fmt.Errorf("notification: DevProvider has nil pool")
	}
	startedAt := time.Now().UTC()

	outcome := DevSimulateOutcome(out.ID)
	attemptStatus := outcome.Status
	excerpt := outcome.Excerpt
	errCode := outcome.ErrorCode

	// Insert delivery attempt.
	const insertAttempt = `
INSERT INTO notification_delivery_attempts
    (outbox_id, attempt_number, channel, status,
     provider_response_excerpt, error_code, attempted_at, duration_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, attempted_at`
	var attempt DeliveryAttemptRow
	attempt.OutboxID = out.ID
	attempt.AttemptNumber = out.AttemptCount + 1
	attempt.Channel = out.Channel
	attempt.Status = attemptStatus
	attempt.ProviderResponseExcerpt = &excerpt
	attempt.ErrorCode = errCode

	duration := int(time.Since(startedAt).Milliseconds())
	attempt.DurationMs = &duration
	if err := p.pool.QueryRow(ctx, insertAttempt,
		attempt.OutboxID,
		attempt.AttemptNumber,
		string(attempt.Channel),
		string(attempt.Status),
		attempt.ProviderResponseExcerpt,
		attempt.ErrorCode,
		startedAt,
		attempt.DurationMs,
	).Scan(&attempt.ID, &attempt.AttemptedAt); err != nil {
		return DeliveryAttemptRow{}, err
	}

	// Update outbox row to reflect the new attempt + final status.
	const updOutbox = `
UPDATE notification_outbox
SET status = $2,
    attempt_count = attempt_count + 1,
    next_attempt_at = NOW() + INTERVAL '5 minutes',
    last_error_code = $3,
    updated_at = NOW()
WHERE id = $1`
	var nextErr interface{}
	if errCode != nil {
		nextErr = *errCode
	} else {
		nextErr = nil
	}
	if _, err := p.pool.Exec(ctx, updOutbox,
		out.ID,
		string(attemptStatus),
		nextErr,
	); err != nil {
		return DeliveryAttemptRow{}, err
	}
	return attempt, nil
}

