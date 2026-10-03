package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// defaultMaxKeys bounds how many distinct keys a limiter tracks so that a
// flood of rotating keys (e.g. ip|username on login) cannot grow the map
// without bound between sweeps.
const defaultMaxKeys = 100_000

// RateLimiter is a small in-memory sliding-window limiter used to protect the
// authentication endpoints from brute force. Keys are typically
// "ip|username" for login and "ip" for registration.
//
// Memory safety: stale keys are reclaimed by an amortized sweep that runs at
// most once per window (no background goroutine), and the tracked-key count is
// capped by maxKeys as a hard backstop.
type RateLimiter struct {
	mu        sync.Mutex
	window    time.Duration
	max       int
	maxKeys   int
	hits      map[string][]time.Time
	lastSeen  map[string]time.Time
	lastSweep time.Time
}

// NewRateLimiter builds a limiter allowing max hits per window.
func NewRateLimiter(window time.Duration, max int) *RateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	if max <= 0 {
		max = 1
	}
	return &RateLimiter{
		window:    window,
		max:       max,
		maxKeys:   defaultMaxKeys,
		hits:      map[string][]time.Time{},
		lastSeen:  map[string]time.Time{},
		lastSweep: time.Now(),
	}
}

// Allow reports whether the key may proceed. Old hits are pruned on access and
// keys idle for a full window are dropped, so the map does not grow without
// bound.
func (r *RateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sweepLocked(now)

	h := r.hits[key]
	cut := now.Add(-r.window)
	p := 0
	for _, t := range h {
		if t.After(cut) {
			h[p] = t
			p++
		}
	}
	h = h[:p]

	if len(h) >= r.max {
		r.hits[key] = h
		r.lastSeen[key] = now
		return false
	}

	// Hard cap: refuse to track brand-new keys once the table is full. An
	// attacker rotating keys hits this ceiling instead of exhausting memory.
	if _, exists := r.hits[key]; !exists && len(r.hits) >= r.maxKeys {
		return false
	}

	r.hits[key] = append(h, now)
	r.lastSeen[key] = now
	return true
}

// sweepLocked drops keys with no activity within the window. It runs at most
// once per window to keep the cost amortized to O(1) per request.
func (r *RateLimiter) sweepLocked(now time.Time) {
	if now.Sub(r.lastSweep) < r.window {
		return
	}
	r.lastSweep = now
	cut := now.Add(-r.window)
	for k, ts := range r.lastSeen {
		if ts.Before(cut) {
			delete(r.hits, k)
			delete(r.lastSeen, k)
		}
	}
}

// RateLimit returns a Gin middleware enforcing rl for the key from keyFn,
// responding 429 when the budget is exhausted.
func RateLimit(rl *RateLimiter, keyFn func(c *gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.Allow(keyFn(c)) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, try again later"})
			return
		}
		c.Next()
	}
}

// ClientKey returns the client IP as the rate-limit key prefix.
func ClientKey(c *gin.Context) string { return c.ClientIP() }

// ClientPlusUsername keys login attempts on IP + username to blunt targeted
// brute force while keeping overall IP budgets separate. Login bodies are
// JSON, so the username is peeked from the (restored) request body rather than
// c.PostForm, which only sees form-encoded input.
func ClientPlusUsername(c *gin.Context) string {
	username := ""
	if c.Request.Body != nil {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4096))
		if err == nil {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			var payload struct {
				Username string `json:"username"`
			}
			_ = json.Unmarshal(body, &payload)
			username = payload.Username
		}
	}
	if username == "" {
		username = c.PostForm("username")
	}
	return c.ClientIP() + "|" + username
}
