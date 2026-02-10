package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Rate limiting constants
const (
	DefaultRateLimit       = 100             // requests per window
	DefaultRateLimitWindow = 1 * time.Minute // time window
	CleanupInterval        = 5 * time.Minute // cleanup old entries
)

// RateLimiter implements a sliding window rate limiter
type RateLimiter struct {
	requests map[string]*clientRate
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

// clientRate tracks requests for a single client
type clientRate struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = DefaultRateLimit
	}
	if window <= 0 {
		window = DefaultRateLimitWindow
	}

	rl := &RateLimiter{
		requests: make(map[string]*clientRate),
		limit:    limit,
		window:   window,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given identifier is allowed
func (rl *RateLimiter) Allow(identifier string) bool {
	now := time.Now()

	rl.mu.Lock()
	client, exists := rl.requests[identifier]
	if !exists {
		client = &clientRate{
			timestamps: make([]time.Time, 0, rl.limit),
		}
		rl.requests[identifier] = client
	}
	rl.mu.Unlock()

	client.mu.Lock()
	defer client.mu.Unlock()

	// Remove timestamps outside the window
	cutoff := now.Add(-rl.window)
	validTimestamps := make([]time.Time, 0, len(client.timestamps))
	for _, ts := range client.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	client.timestamps = validTimestamps

	// Check if limit exceeded
	if len(client.timestamps) >= rl.limit {
		return false
	}

	// Add current request
	client.timestamps = append(client.timestamps, now)
	return true
}

// cleanup periodically removes stale entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window * 2) // Keep entries for 2x window

		for id, client := range rl.requests {
			client.mu.Lock()
			if len(client.timestamps) == 0 || client.timestamps[len(client.timestamps)-1].Before(cutoff) {
				delete(rl.requests, id)
			}
			client.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// Middleware returns an HTTP middleware that applies rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use IP address as identifier
		identifier := getClientIP(r)

		if !rl.Allow(identifier) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
			w.Header().Set("X-RateLimit-Window", rl.window.String())
			w.WriteHeader(http.StatusTooManyRequests)
			respondError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header (for proxies)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr as fallback
	return r.RemoteAddr
}
