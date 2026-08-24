// Package router declares the API surface as data so access control can be
// reasoned about in one place.
package router

import "net/http"

// LimitRequestBody returns middleware that caps the request body size using
// http.MaxBytesReader.  Requests exceeding the limit receive a 413 response.
//
// Typical limits:
//   - 64KB for public/unauthenticated endpoints
//   - 256KB for admin endpoints with JSON payloads
//
// This prevents trivial memory/CPU exhaustion from oversized uploads.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
