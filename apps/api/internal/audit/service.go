package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service writes append-only audit events directly to PostgreSQL.
// All insertions are best-effort and should not block the caller's critical path.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates an audit service backed by a pgx connection pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Event carries the fields needed to append one audit row.
type Event struct {
	// Action is a namespaced verb like "queue.generate" or "authz.denied".
	Action string
	// ResourceType is a noun category like "queue", "facility", "authz".
	ResourceType string
	// ResourceID is an optional concrete identifier (queue ticket ID, facility UUID, etc).
	ResourceID string
	// ActorUserID is the nullable UUID from app_users. Empty means system/dev.
	ActorUserID string
	// ActorType is one of system, user, service, bot, dev.
	ActorType string
	// FacilityID is the nullable facility UUID context.
	FacilityID string
	// RequestID is the trace/request ID from identity.RequestIDFromContext.
	RequestID string
	// Metadata is sanitized free-form context. Keys with PII should be stripped.
	Metadata map[string]any
	// IP and UserAgent are raw; they are hashed before storage.
	IP         string
	UserAgent  string
}

// LogEvent inserts a sanitized, hashed audit row into audit_events.
// It queries the latest event_hash for the chain, computes a new event_hash,
// and performs a best-effort insert. Errors are logged but not returned to
// the caller so that audit failures do not break the request path.
func (s *Service) LogEvent(ctx context.Context, e Event) {
	if s == nil || s.pool == nil {
		return
	}

	e = sanitizeEvent(e)
	prevHash, err := s.previousHash(ctx)
	if err != nil {
		slog.Warn("audit: failed to read previous hash", "err", err)
	}

	hash := computeHash(e, prevHash)

	var actorUserID *string
	if e.ActorUserID != "" {
		actorUserID = &e.ActorUserID
	}
	var resourceID *string
	if e.ResourceID != "" {
		resourceID = &e.ResourceID
	}
	var facilityID *string
	if e.FacilityID != "" {
		facilityID = &e.FacilityID
	}
	var requestID *string
	if e.RequestID != "" {
		requestID = &e.RequestID
	}

	var ipHash, uaHash *string
	if e.IP != "" {
		h := sha256Hash(e.IP)
		ipHash = &h
	}
	if e.UserAgent != "" {
		h := sha256Hash(e.UserAgent)
		uaHash = &h
	}

	metaJSON, _ := json.Marshal(e.Metadata)

	const sql = `
		INSERT INTO audit_events
			(occurred_at, actor_type, action, resource_type, resource_id,
			 actor_user_id, facility_id, request_id, ip_hash, user_agent_hash,
			 metadata, previous_hash, event_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err = s.pool.Exec(ctx, sql,
		time.Now().UTC(),
		e.ActorType,
		e.Action,
		e.ResourceType,
		resourceID,
		actorUserID,
		facilityID,
		requestID,
		ipHash,
		uaHash,
		metaJSON,
		nullIfEmpty(prevHash),
		hash,
	)
	if err != nil {
		// Dev fallback: if the FK on actor_user_id failed (dev user not in
		// app_users), retry without actor_user_id. The actor_type='dev' column
		// is sufficient to identify the actor; the FK is not needed for dev.
		if actorUserID != nil && e.ActorType == "dev" {
			_, retryErr := s.pool.Exec(ctx, sql,
				time.Now().UTC(),
				e.ActorType,
				e.Action,
				e.ResourceType,
				resourceID,
				nil, // no actor_user_id
				facilityID,
				requestID,
				ipHash,
				uaHash,
				metaJSON,
				nullIfEmpty(prevHash),
				hash,
			)
			if retryErr == nil {
				return
			}
			slog.Warn("audit: dev fallback also failed",
				"action", e.Action,
				"resource_type", e.ResourceType,
				"err", retryErr)
			return
		}
		slog.Warn("audit: failed to insert event",
			"action", e.Action,
			"resource_type", e.ResourceType,
			"err", err)
	}
}

// previousHash returns the event_hash of the most recent audit event.
// This is best-effort; under concurrent writers the chain may have gaps.
func (s *Service) previousHash(ctx context.Context) (string, error) {
	if s == nil || s.pool == nil {
		return "", nil
	}
	var h *string
	row := s.pool.QueryRow(ctx,
		`SELECT event_hash FROM audit_events ORDER BY occurred_at DESC LIMIT 1`)
	if err := row.Scan(&h); err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if h == nil {
		return "", nil
	}
	return *h, nil
}

// computeHash creates a deterministic SHA-256 of the canonical event payload.
func computeHash(e Event, previousHash string) string {
	// Use a stable, explicit ordering so the hash is reproducible.
	payload := struct {
		Action       string         `json:"action"`
		ResourceType string         `json:"resource_type"`
		ResourceID   string         `json:"resource_id,omitempty"`
		ActorType    string         `json:"actor_type"`
		ActorUserID  string         `json:"actor_user_id,omitempty"`
		Metadata     map[string]any `json:"metadata"`
		PreviousHash string         `json:"previous_hash,omitempty"`
		At           time.Time      `json:"at"`
	}{
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		ActorType:    e.ActorType,
		ActorUserID:  e.ActorUserID,
		Metadata:     e.Metadata,
		PreviousHash: previousHash,
		At:           time.Now().UTC().Truncate(time.Millisecond),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte("{}")
	}
	return sha256Hash(string(b))
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// forbidList contains substrings that — when found in a metadata key — cause
// the key/value pair to be removed. This is a defense-in-depth privacy guard.
var forbidList = []string{
	"patient", "pasien", // citizen identity
	"phone", "telepon",   // contact
	"nik", "ktp",         // national ID
	"name", "nama",       // personal name
	"address", "alamat",  // location
	"email",              // contact
}

// sanitizeEvent removes any metadata keys that might carry PII.
func sanitizeEvent(e Event) Event {
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
		return e
	}
	clean := make(map[string]any, len(e.Metadata))
	for k, v := range e.Metadata {
		lower := strings.ToLower(k)
		forbidden := false
		for _, f := range forbidList {
			if strings.Contains(lower, f) {
				forbidden = true
				break
			}
		}
		if forbidden {
			clean[k] = "[REDACTED]"
			continue
		}
		clean[k] = v
	}
	e.Metadata = clean
	return e
}

// SanitizeMetadata is a standalone helper for callers that want to clean a
// metadata map before constructing an Event.
func SanitizeMetadata(m map[string]any) map[string]any {
	e := Event{Metadata: m}
	return sanitizeEvent(e).Metadata
}
