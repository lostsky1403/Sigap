package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ---------------------------------------------------------------------------
// RequestIDMiddleware tests
// ---------------------------------------------------------------------------

func TestRequestIDMiddleware_GeneratesNonEmptyID(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Fatal("expected non-empty X-Request-ID header")
	}
	if len(got) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars: %q", len(got), got)
	}
}

func TestRequestIDMiddleware_ContextMatchesHeader(t *testing.T) {
	var ctxID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headerID := rec.Header().Get(XRequestIDHeader)
	if ctxID != headerID {
		t.Errorf("context ID %q != header ID %q", ctxID, headerID)
	}
	if ctxID == "" {
		t.Error("context ID is empty")
	}
}

func TestRequestIDMiddleware_DifferentIDsPerRequest(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ids := make([]string, 5)
	for i := range ids {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		ids[i] = rec.Header().Get(XRequestIDHeader)
	}

	seen := make(map[string]bool)
	for i, id := range ids {
		if id == "" {
			t.Errorf("request %d: empty ID", i)
		}
		if seen[id] {
			t.Errorf("request %d: duplicate ID %q", i, id)
		}
		seen[id] = true
	}
}

func TestRequestIDMiddleware_IgnoresInboundHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "client-supplied-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(XRequestIDHeader)
	if got == "client-supplied-id" {
		t.Error("server should not echo client-supplied X-Request-ID")
	}
	if got == "" {
		t.Error("expected server-generated ID in response header")
	}
}

func TestRequestIDMiddleware_AuthFailureStillHasID(t *testing.T) {
	// Simulate an auth middleware that rejects with 401.
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"error":"auth failed"}`))
	})

	handler := RequestIDMiddleware(authHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Error("X-Request-ID missing on 401 response")
	}
}

func TestRequestIDMiddleware_ForbiddenStillHasID(t *testing.T) {
	// Simulate a permission failure (403).
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"error":"forbidden"}`))
	})

	handler := RequestIDMiddleware(forbiddenHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/facilities", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Error("X-Request-ID missing on 403 response")
	}
}

func TestRequestIDMiddleware_NotFoundStillHasID(t *testing.T) {
	// Simulate a deny-by-default (401 for unknown routes).
	denyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"error":"route not recognized"}`))
	})

	handler := RequestIDMiddleware(denyHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Error("X-Request-ID missing on deny-by-default response")
	}
}

func TestRequestIDMiddleware_500StillHasID(t *testing.T) {
	// Simulate a handler that panics / returns 500.
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"internal"}`))
	})

	handler := RequestIDMiddleware(errHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Error("X-Request-ID missing on 500 response")
	}
}

func TestRequestIDMiddleware_RateLimit429StillHasID(t *testing.T) {
	// Simulate a rate limit rejection (429).
	rlHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"error":"rate limit exceeded"}`))
	})

	handler := RequestIDMiddleware(rlHandler)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/queues/generate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	got := rec.Header().Get(XRequestIDHeader)
	if got == "" {
		t.Error("X-Request-ID missing on 429 response")
	}
}

func TestRequestIDMiddleware_SetsHeaderBeforeHandlerRuns(t *testing.T) {
	// Verify the header is set before next.ServeHTTP is called,
	// which is critical for panics in handlers.
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler checks the header is already present.
		id := w.Header().Get(XRequestIDHeader)
		if id == "" {
			panic("X-Request-ID not set before handler")
		}
		ctxID := RequestIDFromContext(r.Context())
		if ctxID != id {
			panic("context ID != response header ID")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequestIDMiddleware_HeaderIsHex(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	id := rec.Header().Get(XRequestIDHeader)
	if len(id) != 32 {
		t.Fatalf("expected 32 chars, got %d", len(id))
	}
	for i, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("position %d: %q is not lowercase hex", i, c)
		}
	}
	if strings.ToLower(id) != id {
		t.Error("ID should be lowercase hex")
	}
}
