// Package notification implements the Sigap notification outbox
// foundation. It is privacy-first by construction:
//
//   - The raw patient contact is consumed transiently by Enqueue, used to
//     compute a deterministic SHA-256 hash and a masked representation,
//     and then discarded. The raw contact is never persisted, never
//     returned to callers, and never logged.
//   - The masked representation is the only contact-shape field stored
//     in notification_outbox and is the only contact field returned by
//     the admin API.
//   - Subjects and bodies are checked against a denylist that rejects
//     raw-phone-like digit sequences. The same check is also enforced
//     at the database layer (CHECK constraints) for defence in depth.
//   - All audit events are written through audit.Service.SanitizeMetadata
//     with keys that do not include phone, name, email, address, or
//     patient display fields.
//
// The only delivery provider shipped in this package is DevProvider, an
// offline, deterministic simulator. Real vendor providers (Twilio,
// WhatsApp Cloud API, SMTP, SendGrid, …) are intentionally NOT shipped
// and are deferred to a later phase.
package notification

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Channel is the transport channel for a notification. The only
// production-meaningful value in this milestone is ChannelDev; the
// others exist as enum members so that future providers can be wired
// without a database migration.
type Channel string

const (
	ChannelDev      Channel = "dev"
	ChannelSMS      Channel = "sms"
	ChannelWhatsApp Channel = "whatsapp"
	ChannelEmail    Channel = "email"
)

// Valid returns true if c is one of the four declared channels.
func (c Channel) Valid() bool {
	switch c {
	case ChannelDev, ChannelSMS, ChannelWhatsApp, ChannelEmail:
		return true
	}
	return false
}

// Status is the lifecycle of an outbox row. Transitions:
//   pending → processing → delivered
//   pending → processing → failed → pending (retry)
//   pending → cancelled
//   failed  → cancelled
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDelivered  Status = "delivered"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// Valid returns true if s is one of the five declared statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusProcessing, StatusDelivered, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// RecipientType is who the notification is for.
type RecipientType string

const (
	RecipientPatient      RecipientType = "patient"
	RecipientStaff        RecipientType = "staff"
	RecipientFacilityAdmin RecipientType = "facility_admin"
)

// Valid returns true if t is one of the three declared recipient types.
func (t RecipientType) Valid() bool {
	switch t {
	case RecipientPatient, RecipientStaff, RecipientFacilityAdmin:
		return true
	}
	return false
}

// EnqueueInput is the only input the service accepts. RecipientContact
// is the raw contact (e.g. `+6281234567890`); the service computes the
// mask and the hash and discards the raw value before returning.
type EnqueueInput struct {
	FacilityID          *uuid.UUID
	Channel             Channel
	TemplateKey         string
	Subject             string
	BodyTemplate       string
	RecipientType       RecipientType
	RecipientContact    string // raw; transient; never persisted or returned
	RelatedResourceType string // e.g. "appointment" (audit-only metadata)
	RelatedResourceID   string // opaque ID string for the related resource
}

// OutboxRow is the post-mask, post-hash view of a notification_outbox
// row. It deliberately omits RecipientContactHash: the hash is an
// internal dedup key and is never returned to API callers.
type OutboxRow struct {
	ID                       uuid.UUID  `json:"id"`
	FacilityID               *uuid.UUID `json:"facility_id,omitempty"`
	Channel                  Channel    `json:"channel"`
	TemplateKey              string     `json:"template_key"`
	Subject                  string     `json:"subject"`
	BodyTemplate             string     `json:"body_template"`
	RecipientType            RecipientType `json:"recipient_type"`
	RecipientContactMasked   string     `json:"recipient_contact_masked"`
	Status                   Status     `json:"status"`
	AttemptCount             int        `json:"attempt_count"`
	NextAttemptAt            time.Time  `json:"next_attempt_at"`
	LastErrorCode            *string    `json:"last_error_code,omitempty"`
	RelatedResourceType      *string    `json:"related_resource_type,omitempty"`
	RelatedResourceID        *string    `json:"related_resource_id,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// DeliveryAttemptRow is the append-only audit of one delivery attempt.
type DeliveryAttemptRow struct {
	ID                    uuid.UUID `json:"id"`
	OutboxID              uuid.UUID `json:"outbox_id"`
	AttemptNumber         int       `json:"attempt_number"`
	Channel               Channel   `json:"channel"`
	Status                Status    `json:"status"`
	ProviderResponseExcerpt *string `json:"provider_response_excerpt,omitempty"`
	ErrorCode             *string    `json:"error_code,omitempty"`
	AttemptedAt           time.Time  `json:"attempted_at"`
	DurationMs            *int       `json:"duration_ms,omitempty"`
}

// Domain-level errors. The HTTP handler maps these to status codes.
var (
	ErrEmptySubject          = errors.New("notification: empty subject")
	ErrEmptyBodyTemplate     = errors.New("notification: empty body_template")
	ErrEmptyTemplateKey      = errors.New("notification: empty template_key")
	ErrInvalidChannel        = errors.New("notification: invalid channel")
	ErrInvalidStatus         = errors.New("notification: invalid status")
	ErrInvalidRecipientType  = errors.New("notification: invalid recipient_type")
	ErrEmptyRecipientContact = errors.New("notification: empty recipient contact")
	ErrSubjectLeakPhone      = errors.New("notification: subject contains raw-phone-like digits")
	ErrBodyLeakPhone         = errors.New("notification: body_template contains raw-phone-like digits")
	ErrInvalidState          = errors.New("notification: operation not allowed in current state")
	ErrNotFound              = errors.New("notification: not found")
)
