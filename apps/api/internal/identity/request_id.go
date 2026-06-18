package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

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
