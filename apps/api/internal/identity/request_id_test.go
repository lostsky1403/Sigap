package identity

import (
	"context"
	"testing"
)

func TestNewRequestID(t *testing.T) {
	id, err := NewRequestID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty request ID")
	}
	if len(id) < 16 {
		t.Errorf("expected request ID length >= 16, got %d", len(id))
	}
	// NewRequestID returns hex (0-9, a-f)
	for i, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("character %d (%q) is not hex", i, c)
		}
	}
}

func TestRequestID_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Empty context
	if got := RequestIDFromContext(ctx); got != "" {
		t.Errorf("expected empty request ID from empty context, got %q", got)
	}

	const want = "req-test-001"
	ctx = ContextWithRequestID(ctx, want)
	got := RequestIDFromContext(ctx)
	if got != want {
		t.Errorf("request ID mismatch: got %q want %q", got, want)
	}
}

func TestNewRequestID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id, err := NewRequestID()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[id] {
			t.Errorf("duplicate request ID %q", id)
		}
		seen[id] = true
	}
}

func TestContextWithRequestID_Overwrite(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "first")
	ctx = ContextWithRequestID(ctx, "second")
	if got := RequestIDFromContext(ctx); got != "second" {
		t.Errorf("expected overwritten value second, got %q", got)
	}
}
