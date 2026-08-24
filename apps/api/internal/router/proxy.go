// Package router declares the API surface as data so access control can be
// reasoned about in one place.
package router

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// clientIPKey is the context key for the canonical client IP.
type clientIPKey struct{}

// ClientIPFromContext returns the sanitized client IP stored by
// TrustedProxy middleware.  Falls back to r.RemoteAddr if the
// middleware has not run.
func ClientIPFromContext(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return stripPort(r.RemoteAddr)
}

// TrustedProxy returns middleware that sanitizes X-Forwarded-For and
// X-Real-IP headers based on the deployment environment:
//
//   - SIGAP_ENV=local (or unset): all proxy headers are stripped; the
//     canonical IP comes from RemoteAddr.  This prevents IP spoofing
//     during local development.
//
//   - SIGAP_ENV != local: the middleware reads the number of trusted
//     proxy hops from SIGAP_TRUSTED_PROXIES (default 1).  It peels
//     that many entries off the right side of X-Forwarded-For and
//     uses the result as the client IP.  X-Real-IP is only trusted
//     when exactly one hop is configured.
//
// The middleware stores the canonical client IP in request context for
// downstream handlers (rate limiting, audit logging).
func TrustedProxy(next http.Handler) http.Handler {
	env := strings.TrimSpace(os.Getenv("SIGAP_ENV"))
	isLocal := strings.EqualFold(env, "local") || env == ""

	trustedHops := 1
	if !isLocal {
		if v := os.Getenv("SIGAP_TRUSTED_PROXIES"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				trustedHops = n
			}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var clientIP string

		if isLocal {
			// Local: never trust proxy headers.
			clientIP = stripPort(r.RemoteAddr)
			// Clear potentially spoofed headers so downstream code
			// cannot accidentally use them.
			r.Header.Del("X-Forwarded-For")
			r.Header.Del("X-Real-IP")
			r.Header.Del("X-Forwarded-Proto")
		} else {
			// Non-local: derive client IP from trusted proxy chain.
			clientIP = resolveClientIP(r, trustedHops)
		}

		ctx := context.WithValue(r.Context(), clientIPKey{}, clientIP)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClientIP extracts the client IP from proxy headers,
// respecting the configured number of trusted hops.
func resolveClientIP(r *http.Request, trustedHops int) string {
	if trustedHops == 0 {
		return stripPort(r.RemoteAddr)
	}

	// X-Forwarded-For: client, proxy1, proxy2, ...
	// With 1 trusted hop, we trust the last entry added by the
	// immediate proxy and take the one before it as the client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		// Trim whitespace from all parts.
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}			// The rightmost entry is the one closest to us (the proxy).
			// Peel off trustedHops entries from the right to get the client.
			idx := len(parts) - 1 - trustedHops
			if idx < 0 {
				idx = 0
			}
		if ip := parseIP(parts[idx]); ip != "" {
			return ip
		}
	}

	// X-Real-IP: only trust when exactly 1 hop (the immediate proxy).
	if trustedHops == 1 {
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			if ip := parseIP(strings.TrimSpace(xr)); ip != "" {
				return ip
			}
		}
	}

	return stripPort(r.RemoteAddr)
}

// parseIP validates and returns a clean IP string, or "" if invalid.
func parseIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Handle IPv6 bracket notation [::1].
	s = strings.Trim(s, "[]")
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return ""
}

// stripPort removes the port suffix from a host:port string.
func stripPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
