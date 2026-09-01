package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// XRequestIDHeader is the HTTP response header that carries the server-generated
// request ID back to the client for correlation.
const XRequestIDHeader = "X-Request-ID"

// RequestIDMiddleware returns middleware that generates a server-side request ID
// for every incoming HTTP request. The ID is stored in the request context (accessible
// via RequestIDFromContext) and written to the X-Request-ID response header.
//
// Trust model: client-supplied X-Request-ID headers are ALWAYS ignored.
// A fresh server-generated ID is produced for every request regardless of
// inbound headers. This avoids trusting arbitrary-length or malformed client input.
//
// This middleware MUST be the outermost layer in the HTTP chain so that every
// downstream middleware (auth, audit, RBAC, rate limiting) and handler receives
// a non-empty request ID in context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := NewRequestID()
		if err != nil {
			slog.Error("request id: CSPRNG failure", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"internal error"}`))
			return
		}

		ctx := ContextWithRequestID(r.Context(), id)
		w.Header().Set(XRequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDLen is the number of raw random bytes before hex encoding.
const requestIDLen = 16

// NewRequestID generates a cryptographically random request ID encoded as hex.
// Returns an error only if the OS CSPRNG fails (extremely unlikely).
func NewRequestID() (string, error) {
	b := make([]byte, requestIDLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("request id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// Request ID context helpers
// ---------------------------------------------------------------------------

type requestIDKey struct{}

// ContextWithRequestID returns ctx with the given request ID attached.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request ID stored in ctx, or empty string.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}
