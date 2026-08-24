// Package router declares the API surface as data so access control can be
// reasoned about in one place.
package router

import "net/http"

// SecurityHeaders returns middleware that sets baseline security headers on
// every response.  These protect against clickjacking (X-Frame-Options,
// CSP frame-ancestors), MIME sniffing (X-Content-Type-Options), and
// downgrade attacks (HSTS when behind TLS).
//
// The CSP is intentionally permissive for an API that serves JSON and SSE:
//   - default-src 'self': scripts/styles are not served by the API
//   - frame-ancestors 'none': prevents framing entirely
//
// HSTS is only set when the request arrives over TLS (i.e. a reverse proxy
// has terminated TLS) or when SIGAP_TLS_TERMINATED is set.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking — no framing of API responses.
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

		// Prevent MIME sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Referrer policy — don't leak full URLs to third parties.
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// HSTS: only meaningful over HTTPS.  Set it when the request
		// scheme is https (reverse-proxy terminated TLS) or when the
		// operator has explicitly confirmed TLS termination.
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}
