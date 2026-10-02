package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a small in-memory sliding-window limiter used to protect the
// authentication endpoints from brute force. Keys are typically
// "ip|username" for login and "ip" for registration.
type RateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
}

// NewRateLimiter builds a limiter allowing max hits per window.
func NewRateLimiter(window time.Duration, max int) *RateLimiter {
	return &RateLimiter{window: window, max: max, hits: map[string][]time.Time{}}
}

// Allow reports whether the key may proceed. Old hits are pruned on access so
// the map does not grow without bound.
func (r *RateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
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
		return false
	}
	r.hits[key] = append(h, now)
	return true
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
// brute force while keeping overall IP budgets separate.
func ClientPlusUsername(c *gin.Context) string {
	return c.ClientIP() + "|" + c.PostForm("username")
}
