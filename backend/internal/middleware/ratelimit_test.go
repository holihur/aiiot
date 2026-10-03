package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterAllowsWithinBudget(t *testing.T) {
	rl := NewRateLimiter(time.Minute, 3)
	for i := 0; i < 3; i++ {
		if !rl.Allow("k") {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	if rl.Allow("k") {
		t.Fatal("4th hit within window should be denied")
	}
	// different key has its own budget
	if !rl.Allow("other") {
		t.Fatal("other key should be allowed")
	}
}

func TestRateLimiterWindowExpires(t *testing.T) {
	// Tiny window: after it passes, the budget resets.
	rl := NewRateLimiter(50*time.Millisecond, 1)
	if !rl.Allow("k") {
		t.Fatal("first should be allowed")
	}
	if rl.Allow("k") {
		t.Fatal("second within window should be denied")
	}
	time.Sleep(80 * time.Millisecond)
	if !rl.Allow("k") {
		t.Fatal("after window expiry should be allowed again")
	}
}

func TestRateLimiterEvictsStaleKeys(t *testing.T) {
	rl := NewRateLimiter(30*time.Millisecond, 5)
	rl.Allow("stale")
	if len(rl.hits) != 1 {
		t.Fatalf("expected 1 tracked key, got %d", len(rl.hits))
	}
	time.Sleep(80 * time.Millisecond)
	// A subsequent request triggers the amortized sweep.
	rl.Allow("fresh")
	if len(rl.hits) != 1 {
		t.Fatalf("stale key should have been evicted, got %d keys", len(rl.hits))
	}
	if _, ok := rl.hits["fresh"]; !ok {
		t.Fatal("fresh key must remain tracked")
	}
}

func TestRateLimiterCapsTrackedKeys(t *testing.T) {
	rl := NewRateLimiter(time.Minute, 5)
	rl.maxKeys = 3
	for i := 0; i < 3; i++ {
		if !rl.Allow(string(rune('a' + i))) {
			t.Fatalf("key %d should be allowed", i)
		}
	}
	if rl.Allow("overflow") {
		t.Fatal("a brand-new key beyond maxKeys must be rejected")
	}
	if len(rl.hits) != 3 {
		t.Fatalf("tracked keys must stay capped, got %d", len(rl.hits))
	}
	// Existing keys keep their own budget.
	if !rl.Allow("a") {
		t.Fatal("existing key should still be allowed")
	}
}

func TestClientPlusUsernameReadsJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	key := ClientPlusUsername(c)
	if !strings.HasSuffix(key, "|alice") {
		t.Fatalf("expected key to include username, got %q", key)
	}
	// The handler must still be able to read the body afterwards.
	body, _ := io.ReadAll(c.Request.Body)
	if !strings.Contains(string(body), "alice") {
		t.Fatal("request body was consumed by the key function")
	}
}
