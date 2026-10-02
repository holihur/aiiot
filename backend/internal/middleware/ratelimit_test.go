package middleware

import (
	"testing"
	"time"
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
