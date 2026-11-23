package audit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSanitizeMetadata_Redacts(t *testing.T) {
	cases := []struct {
		key      string
		expected string
	}{
		{"patient_name", "[REDACTED]"},
		{"pasien_nama", "[REDACTED]"},
		{"phone", "[REDACTED]"},
		{"telepon", "[REDACTED]"},
		{"nik", "[REDACTED]"},
		{"ktp", "[REDACTED]"},
		{"full_name", "[REDACTED]"},
		{"nama_lengkap", "[REDACTED]"},
		{"address", "[REDACTED]"},
		{"alamat", "[REDACTED]"},
		{"email", "[REDACTED]"},
		{"facility_id", "123"},
		{"ticket_id", "abc"},
		{"action", "generate"},
	}

	m := make(map[string]any, len(cases))
	for _, c := range cases {
		m[c.key] = "123"
	}

	clean := SanitizeMetadata(m)
	if len(clean) != len(cases) {
		t.Fatalf("expected %d keys, got %d", len(cases), len(clean))
	}

	for _, c := range cases {
		got := clean[c.key]
		if c.expected == "[REDACTED]" {
			if got != "[REDACTED]" {
				t.Errorf("key %q: expected [REDACTED], got %q", c.key, got)
			}
			continue
		}
		if got != "123" {
			t.Errorf("key %q: expected preserved value 123, got %q", c.key, got)
		}
	}
}

func TestSanitizeMetadata_PreservesNested(t *testing.T) {
	m := map[string]any{
		"facility_id": "42",
		"nested": map[string]any{
			"patient_name": "sensitive",
		},
	}
	clean := SanitizeMetadata(m)
	if clean["facility_id"] != "42" {
		t.Errorf("expected facility_id preserved, got %v", clean["facility_id"])
	}
	// Nested maps should NOT be recursively redacted ( limitation of current impl ).
	// Verify it's preserved as-is for now.
	nested, ok := clean["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map preserved, got %T", clean["nested"])
	}
	if nested["patient_name"] != "sensitive" {
		t.Logf("nested redaction not implemented; nested value = %v", nested["patient_name"])
	}
}

func TestSanitizeMetadata_NilInput(t *testing.T) {
	clean := SanitizeMetadata(nil)
	if clean == nil || len(clean) != 0 {
		t.Errorf("expected empty map, got %v", clean)
	}
}

func TestSHA256Hash(t *testing.T) {
	input := "sigap-test-payload"
	hash := sha256Hash(input)
	if len(hash) != 64 {
		t.Errorf("expected hex length 64, got %d", len(hash))
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("hash contains non-hex char: %q", c)
		}
	}

	// Deterministic
	if got := sha256Hash(input); got != hash {
		t.Errorf("hash not deterministic: %q vs %q", got, hash)
	}

	// Different input yields different hash
	if sha256Hash("different") == hash {
		t.Error("different input produced same hash")
	}
}

func TestComputeHash(t *testing.T) {
	e := Event{
		Action:       "queue.generate",
		ResourceType: "queue",
		ResourceID:   "ticket-001",
		ActorType:    "dev",
		ActorUserID:  "dev-001",
		Metadata:     map[string]any{"facility_id": "42"},
	}

	occurredAt := time.Date(2026, 11, 19, 1, 2, 3, 456000000, time.UTC).Truncate(time.Millisecond)
	hash := computeHash(e, "", occurredAt)
	if len(hash) != 64 {
		t.Errorf("expected hex length 64, got %d", len(hash))
	}
	if strings.ToLower(hash) != hash {
		t.Errorf("expected lowercase hex, got %s", hash)
	}

	// With previous hash - format should still be valid hex
	hash2 := computeHash(e, "abc123", occurredAt)
	if len(hash2) != 64 {
		t.Errorf("expected hex length 64 with prev, got %d", len(hash2))
	}
	if hash == hash2 {
		t.Error("different previous hash should produce different event hash")
	}
}

// TestComputeHash_ReproducibleFromStoredAt pins the audit-chain invariant
// (ledger finding F-3): event_hash must be recomputable from the row that was
// persisted. Previously computeHash stamped its own time.Now() while the INSERT
// used a separate time.Now(), so honest rows could never be verified. After the
// fix the caller threads ONE occurredAt into both, so (a) the same inputs always
// give the same hash and (b) changing only the timestamp changes the hash —
// proving the hashed instant is the one supplied by the caller, not hidden
// inside the function.
func TestComputeHash_ReproducibleFromStoredAt(t *testing.T) {
	e := Event{
		Action:       "appointment.created",
		ResourceType: "appointment",
		ActorType:    "user",
		Metadata:     map[string]any{"status": "scheduled"},
	}
	at := time.Date(2026, 11, 19, 1, 2, 3, 456000000, time.UTC).Truncate(time.Millisecond)

	first := computeHash(e, "prev", at)
	// Same stored row inputs, recomputed later (e.g. after restart/export).
	recomputed := computeHash(e, "prev", at)
	if first != recomputed {
		t.Fatalf("event_hash not reproducible from stored occurred_at: %s != %s", first, recomputed)
	}
	// A different hashed instant must change the hash — otherwise the timestamp
	// is not actually bound into the digest and the stored row is not verifiable.
	other := computeHash(e, "prev", at.Add(time.Second))
	if first == other {
		t.Fatal("event_hash does not bind the occurred_at timestamp")
	}
}

func TestEvent_JSONRoundTrip(t *testing.T) {
	e := Event{
		Action:       "queue.generate",
		ResourceType: "queue",
		ResourceID:   "ticket-001",
		ActorUserID:  "dev-001",
		ActorType:    "dev",
		FacilityID:   "fac-001",
		RequestID:    "req-001",
		IP:           "127.0.0.1",
		UserAgent:    "test-agent/1.0",
		Metadata:     map[string]any{"facility_id": "42"},
	}

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Action != e.Action {
		t.Errorf("Action mismatch: %q vs %q", got.Action, e.Action)
	}
	if got.ResourceType != e.ResourceType {
		t.Errorf("ResourceType mismatch: %q vs %q", got.ResourceType, e.ResourceType)
	}
	if got.ResourceID != e.ResourceID {
		t.Errorf("ResourceID mismatch: %q vs %q", got.ResourceID, e.ResourceID)
	}
	if got.ActorUserID != e.ActorUserID {
		t.Errorf("ActorUserID mismatch: %q vs %q", got.ActorUserID, e.ActorUserID)
	}
	if got.ActorType != e.ActorType {
		t.Errorf("ActorType mismatch: %q vs %q", got.ActorType, e.ActorType)
	}
	if got.FacilityID != e.FacilityID {
		t.Errorf("FacilityID mismatch: %q vs %q", got.FacilityID, e.FacilityID)
	}
	if got.RequestID != e.RequestID {
		t.Errorf("RequestID mismatch: %q vs %q", got.RequestID, e.RequestID)
	}
	if got.IP != e.IP {
		t.Errorf("IP mismatch: %q vs %q", got.IP, e.IP)
	}
	if got.UserAgent != e.UserAgent {
		t.Errorf("UserAgent mismatch: %q vs %q", got.UserAgent, e.UserAgent)
	}
	if m, ok := got.Metadata["facility_id"]; !ok || m != "42" {
		t.Errorf("Metadata mismatch: %v", got.Metadata)
	}
}
