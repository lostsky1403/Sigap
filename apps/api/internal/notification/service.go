package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the outbox-side API. It owns the DB pool and the audit hook;
// the provider is wired separately so callers can swap in a fake for
// tests.
//
// Service.Enqueue is the only method that inserts into
// notification_outbox. Every other surface (List, Get, Retry, Cancel)
// is read- or state-machine-only.
type Service struct {
	pool *pgxpool.Pool
}

// NewService constructs a Service bound to the given pool. The pool
// must not be nil.
func NewService(pool *pgxpool.Pool) *Service {
	if pool == nil {
		panic("notification.NewService: pool is nil")
	}
	return &Service{pool: pool}
}

// Pool returns the underlying pgxpool.Pool. Exposed so that the HTTP
// handler can run read queries without rebuilding the pool.
func (s *Service) Pool() *pgxpool.Pool { return s.pool }

// Enqueue validates the input, computes the masked contact and the
// contact hash, inserts a row into notification_outbox with
// status='pending', and returns the new OutboxRow.
//
// Enqueue NEVER returns the raw contact or the hash to the caller. The
// raw contact is consumed by the masking / hashing helpers and is then
// eligible for garbage collection.
//
// Errors:
//   - ErrEmptySubject / ErrEmptyBodyTemplate / ErrEmptyTemplateKey
//   - ErrInvalidChannel / ErrInvalidStatus / ErrInvalidRecipientType
//   - ErrEmptyRecipientContact
//   - ErrSubjectLeakPhone / ErrBodyLeakPhone (denylist violations)
func (s *Service) Enqueue(ctx context.Context, in EnqueueInput) (OutboxRow, error) {
	if err := validateEnqueue(in); err != nil {
		return OutboxRow{}, err
	}
	if ContainsRawPhoneDigits(in.Subject) {
		return OutboxRow{}, ErrSubjectLeakPhone
	}
	if ContainsRawPhoneDigits(in.BodyTemplate) {
		return OutboxRow{}, ErrBodyLeakPhone
	}

	var mask string
	switch in.RecipientType {
	case RecipientStaff, RecipientFacilityAdmin:
		mask = MaskEmail(in.RecipientContact) // staff emails go through MaskEmail
	default:
		mask = MaskPhone(in.RecipientContact)
	}
	hash := HashContact(in.RecipientContact)
	facilityUUID := uuid.UUID{}
	if in.FacilityID != nil {
		facilityUUID = *in.FacilityID
	}

	now := time.Now().UTC()
	const insertSQL = `
INSERT INTO notification_outbox
    (facility_id, channel, template_key, subject, body_template,
     recipient_type, recipient_contact_masked, recipient_contact_hash,
     status, attempt_count, next_attempt_at,
     related_resource_type, related_resource_id,
     created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending',0,$9,$10,$11,$12,$13)
RETURNING id, created_at, updated_at`

	var out OutboxRow
	err := s.pool.QueryRow(ctx, insertSQL,
		nullableUUID(facilityUUID),
		string(in.Channel),
		in.TemplateKey,
		in.Subject,
		in.BodyTemplate,
		string(in.RecipientType),
		mask,
		hash,
		now,
		nullableString(in.RelatedResourceType),
		nullableString(in.RelatedResourceID),
		now,
		now,
	).Scan(&out.ID, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return OutboxRow{}, err
	}

	out.FacilityID = in.FacilityID
	out.Channel = in.Channel
	out.TemplateKey = in.TemplateKey
	out.Subject = in.Subject
	out.BodyTemplate = in.BodyTemplate
	out.RecipientType = in.RecipientType
	out.RecipientContactMasked = mask
	out.Status = StatusPending
	out.AttemptCount = 0
	out.NextAttemptAt = now
	if in.RelatedResourceType != "" {
		s := in.RelatedResourceType
		out.RelatedResourceType = &s
	}
	if in.RelatedResourceID != "" {
		s := in.RelatedResourceID
		out.RelatedResourceID = &s
	}
	return out, nil
}

// GetByID returns the outbox row for the given id. Returns
// (OutboxRow{}, ErrNotFound) when the row does not exist.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (OutboxRow, error) {
	const sel = `
SELECT id, facility_id, channel, template_key, subject, body_template,
       recipient_type, recipient_contact_masked, status, attempt_count,
       next_attempt_at, last_error_code, related_resource_type,
       related_resource_id, created_at, updated_at
FROM notification_outbox
WHERE id = $1`
	var out OutboxRow
	var fac *uuid.UUID
	var lastErr, relType, relID *string
	err := s.pool.QueryRow(ctx, sel, id).Scan(
		&out.ID, &fac, &out.Channel, &out.TemplateKey, &out.Subject, &out.BodyTemplate,
		&out.RecipientType, &out.RecipientContactMasked, &out.Status, &out.AttemptCount,
		&out.NextAttemptAt, &lastErr, &relType, &relID, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OutboxRow{}, ErrNotFound
		}
		return OutboxRow{}, err
	}
	out.FacilityID = fac
	out.LastErrorCode = lastErr
	out.RelatedResourceType = relType
	out.RelatedResourceID = relID
	return out, nil
}

// List returns the most recent N outbox rows, newest first, optionally
// scoped by the filters in p. A zero-valued field on ListParams is
// treated as "no filter"; the SQL relies on the empty/zero value to
// short-circuit each predicate, so callers do not have to distinguish
// "not provided" from "explicitly empty".
//
// The default limit is 100; values <= 0 or > 500 are clamped to 100.
func (s *Service) List(ctx context.Context, p ListParams) ([]OutboxRow, error) {
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const sel = `
SELECT id, facility_id, channel, template_key, subject, body_template,
       recipient_type, recipient_contact_masked, status, attempt_count,
       next_attempt_at, last_error_code, related_resource_type,
       related_resource_id, created_at, updated_at
FROM notification_outbox
WHERE ($1 = '00000000-0000-0000-0000-000000000000' OR facility_id = $1::uuid)
  AND ($2 = '' OR status = $2)
  AND ($3 = '' OR channel = $3)
  AND ($4 = '' OR template_key = $4)
  AND ($5::timestamptz IS NULL OR created_at >= $5)
  AND ($6::timestamptz IS NULL OR created_at <= $6)
ORDER BY created_at DESC
LIMIT $7`
	rows, err := s.pool.Query(ctx, sel,
		p.FacilityID.String(),
		p.Status,
		p.Channel,
		p.TemplateKey,
		nullableTime(p.CreatedFrom),
		nullableTime(p.CreatedTo),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OutboxRow{}
	for rows.Next() {
		var r OutboxRow
		var fac *uuid.UUID
		var lastErr, relType, relID *string
		if err := rows.Scan(
			&r.ID, &fac, &r.Channel, &r.TemplateKey, &r.Subject, &r.BodyTemplate,
			&r.RecipientType, &r.RecipientContactMasked, &r.Status, &r.AttemptCount,
			&r.NextAttemptAt, &lastErr, &relType, &relID, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.FacilityID = fac
		r.LastErrorCode = lastErr
		r.RelatedResourceType = relType
		r.RelatedResourceID = relID
		out = append(out, r)
	}
	return out, rows.Err()
}

// Summary returns a per-status count of the outbox, optionally scoped
// to a single facility. The result map is keyed by Status string
// ("pending", "processing", "delivered", "failed", "cancelled") and
// always contains an entry for every declared status — statuses with
// zero rows are reported as 0 so the UI can render the full card set
// without a second pass.
//
// Pass uuid.Nil as facilityID to count across all facilities (the
// super_admin path).
func (s *Service) Summary(ctx context.Context, facilityID uuid.UUID) (map[string]int, error) {
	const sel = `
SELECT status, COUNT(*) AS count
FROM notification_outbox
WHERE ($1 = '00000000-0000-0000-0000-000000000000' OR facility_id = $1::uuid)
GROUP BY status`
	rows, err := s.pool.Query(ctx, sel, facilityID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for _, s := range AllStatuses() {
		out[s] = 0
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

// Retry resets a failed (or pending) notification back to status='pending'
// and bumps the attempt counter. Returns ErrInvalidState if the current
// status is not in {failed, pending}. Returns ErrNotFound if the row
// does not exist.
func (s *Service) Retry(ctx context.Context, id uuid.UUID) (OutboxRow, error) {
	const upd = `
UPDATE notification_outbox
SET status = 'pending',
    attempt_count = attempt_count + 1,
    next_attempt_at = NOW(),
    last_error_code = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status IN ('failed','pending')
RETURNING id`
	var got uuid.UUID
	err := s.pool.QueryRow(ctx, upd, id).Scan(&got)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either the row doesn't exist or its current status is
			// not in the allowed set. Distinguish so the HTTP handler
			// can map to 404 vs 409.
			if exists, _ := s.exists(ctx, id); exists {
				return OutboxRow{}, ErrInvalidState
			}
			return OutboxRow{}, ErrNotFound
		}
		return OutboxRow{}, err
	}
	return s.GetByID(ctx, id)
}

// Cancel marks a pending or failed notification as cancelled. Idempotent:
// calling Cancel on an already-cancelled row succeeds and returns the
// current row. Returns ErrInvalidState only if the row is currently
// delivered.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (OutboxRow, error) {
	const upd = `
UPDATE notification_outbox
SET status = 'cancelled',
    updated_at = NOW()
WHERE id = $1
  AND status IN ('pending','failed','cancelled')
RETURNING id`
	var got uuid.UUID
	err := s.pool.QueryRow(ctx, upd, id).Scan(&got)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if exists, _ := s.exists(ctx, id); exists {
				return OutboxRow{}, ErrInvalidState
			}
			return OutboxRow{}, ErrNotFound
		}
		return OutboxRow{}, err
	}
	return s.GetByID(ctx, id)
}

func (s *Service) exists(ctx context.Context, id uuid.UUID) (bool, error) {
	const q = `SELECT 1 FROM notification_outbox WHERE id = $1`
	var x int
	err := s.pool.QueryRow(ctx, q, id).Scan(&x)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func validateEnqueue(in EnqueueInput) error {
	if strings.TrimSpace(in.TemplateKey) == "" {
		return ErrEmptyTemplateKey
	}
	if strings.TrimSpace(in.Subject) == "" {
		return ErrEmptySubject
	}
	if strings.TrimSpace(in.BodyTemplate) == "" {
		return ErrEmptyBodyTemplate
	}
	if !in.Channel.Valid() {
		return ErrInvalidChannel
	}
	if !in.RecipientType.Valid() {
		return ErrInvalidRecipientType
	}
	if strings.TrimSpace(in.RecipientContact) == "" {
		return ErrEmptyRecipientContact
	}
	return nil
}

// nullableUUID returns the zero UUID as nil so the SQL driver writes
// NULL into a UUID column when no facility context is provided.
func nullableUUID(id uuid.UUID) any {
	if id == (uuid.UUID{}) {
		return nil
	}
	return id
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableTime mirrors nullableString for time.Time: the SQL guards
// use `IS NULL` on the parameter so a zero-value time is treated as
// "no bound" rather than as the unix epoch.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
