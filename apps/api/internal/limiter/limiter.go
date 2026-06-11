package limiter

import (
	"sync"
	"time"
)

// RateLimiter provides a simple in-memory fixed-window rate limiter.
// This is the initial anti-spam protection framework for the public queue endpoint.
// For production, replace with Redis-backed or distributed limiter (e.g. token bucket per phone/facility).
// Keyed by IP (or X-Forwarded-For) for scaffold. Limits requests to the generate endpoint.
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
