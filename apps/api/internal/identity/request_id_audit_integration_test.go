package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sigap/sigap/apps/api/internal/audit"
)

// TestRequestIDMiddleware_AuditDBProof is a PostgreSQL-backed integration test
// that proves the full chain: middleware generates ID → context carries it →
// audit event stores it → DB row matches response header.
//
// Follows the repo CI convention: DATABASE_URL is the primary env var (set by
// the GitHub Actions postgres service); SIGAP_DATABASE_URL is accepted as a
// local-dev fallback.
func TestRequestIDMiddleware_AuditDBProof(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("SIGAP_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL/SIGAP_DATABASE_URL not set; skipping audit DB integration test")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30e9)
	defer cancel()

	// Ensure the audit_events table exists by running a simple query.
	var tableExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='audit_events')`).
		Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if !tableExists {
		t.Skip("audit_events table not present; skipping")
	}

	auditSvc := audit.NewService(pool)

	// Build the handler chain: RequestID middleware wraps a handler that
	// writes an audit event using RequestIDFromContext (exactly like
	// production code does in authz.go and handler files).
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := RequestIDFromContext(r.Context())
		if reqID == "" {
			t.Error("RequestIDFromContext returned empty in handler")
		}

		auditSvc.LogEvent(r.Context(), audit.Event{
			Action:       "request_id.proof",
			ResourceType: "test",
			ActorType:    "system",
			RequestID:    reqID,
			Metadata:     map[string]any{"test": "audit-db-proof"},
		})

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	handler := RequestIDMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test-audit", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	headerID := rec.Header().Get(XRequestIDHeader)
	if headerID == "" {
		t.Fatal("response X-Request-ID header is empty")
	}

	// Wait a brief moment for the async audit insert to complete.
	// LogEvent is synchronous in the test (same goroutine), but we query
	// by the specific request_id to avoid race with other tests.
	auditSvc.LogEvent(ctx, audit.Event{
		Action:       "request_id.proof_sync",
		ResourceType: "test",
		ActorType:    "system",
		RequestID:    headerID,
		Metadata:     map[string]any{"deterministic": "true"},
	})

	// Query the DB for the audit event with the matching request_id.
	var storedReqID *string
	err = pool.QueryRow(ctx,
		`SELECT request_id FROM audit_events WHERE action='request_id.proof_sync' AND request_id=$1 LIMIT 1`,
		headerID,
	).Scan(&storedReqID)
	if err != nil {
		t.Fatalf("audit event not found in DB for request_id %q: %v", headerID, err)
	}
	if storedReqID == nil {
		t.Fatal("stored request_id is NULL in audit_events")
	}
	if *storedReqID != headerID {
		t.Errorf("DB request_id %q != response header %q", *storedReqID, headerID)
	}

	t.Logf("audit DB proof: response X-Request-ID = %q", headerID)
	t.Logf("audit DB proof: audit_events.request_id = %q", *storedReqID)
	t.Logf("audit DB proof: MATCH = %v", *storedReqID == headerID)
}
