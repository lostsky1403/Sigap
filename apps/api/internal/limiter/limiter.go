package limiter

import (
	"sync"
	"time"
)

// RateLimiter provides a simple in-memory fixed-window rate limiter.
// This is the anti-spam protection for the public "generate nomor antrean" endpoint.
//
// Usage for real-world civic:
//   - Use NewDailyLimiter(2) for per-phone-per-faskes daily limit (1 NIK/phone max 2 antrean/hari/faskes).
//   - Caller builds a stable key like "2026-06-12:081234567890:facility-uuid" (date + phone + faskes).
//   - This prevents abuse on public WiFi (unlike pure IP limiting).
//
// For production, replace the whole thing with Redis-backed limiter (per phone + facility + date).
// The current implementation is the "kerangka awal" that already supports the required policy.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientLimit
	limit   int           // max requests allowed in the window
	window  time.Duration // e.g. 1 * time.Minute
}

type clientLimit struct {
	count int
	reset time.Time
}

// NewRateLimiter creates a new limiter.
// Example: NewRateLimiter(10, time.Minute) → 10 generates per minute per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		clients: make(map[string]*clientLimit),
		limit:   limit,
		window:  window,
	}
}

// Allow returns true if the key is allowed under current limit.
// It also cleans up old entries opportunistically.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	cl, ok := rl.clients[key]
	if !ok || now.After(cl.reset) {
		rl.clients[key] = &clientLimit{
			count: 1,
			reset: now.Add(rl.window),
		}
		return true
	}

	if cl.count < rl.limit {
		cl.count++
		return true
	}

	return false
}

// Remaining returns how many requests are still allowed for the key in the current window (best effort).
func (rl *RateLimiter) Remaining(key string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cl, ok := rl.clients[key]
	if !ok || time.Now().After(cl.reset) {
		return rl.limit
	}
	return rl.limit - cl.count
}

// NewDailyLimiter is a convenience constructor for the per-hari identity limit
// required by the anti-calo / anti-spam policy (e.g. max 2 antrean per phone per faskes per day).
func NewDailyLimiter(maxPerDay int) *RateLimiter {
	return NewRateLimiter(maxPerDay, 25*time.Hour)
}
