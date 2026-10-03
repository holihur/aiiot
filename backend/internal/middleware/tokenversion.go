package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// TokenVersionCache validates JWT token versions against the database, caching
// the current version per identity for a short TTL. This makes a password
// change or "sign out everywhere" take effect promptly without a database read
// on every request. The cache is bounded by the number of active identities,
// and expired entries are swept so it cannot grow without bound.
type TokenVersionCache struct {
	db  *gorm.DB
	ttl time.Duration

	mu        sync.Mutex
	entries   map[string]versionEntry
	lastSweep time.Time
}

type versionEntry struct {
	version int
	expires time.Time
	ok      bool
}

// NewTokenVersionCache builds a cache with the given TTL (default 30s).
func NewTokenVersionCache(db *gorm.DB, ttl time.Duration) *TokenVersionCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &TokenVersionCache{
		db:        db,
		ttl:       ttl,
		entries:   map[string]versionEntry{},
		lastSweep: time.Now(),
	}
}

// OK reports whether the presented token version matches the current one. A
// nil cache (or nil DB, e.g. in tests) accepts everything.
func (c *TokenVersionCache) OK(kind string, userID uint, version int) bool {
	if c == nil || c.db == nil {
		return true
	}
	key := kind + ":" + strconv.FormatUint(uint64(userID), 10)
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		if !e.ok {
			return false
		}
		// Token versions are monotonically increasing, so a token at least as
		// new as the cached value stays valid even if the cache is momentarily
		// stale right after a bump (password change / revoke).
		return version >= e.version
	}
	c.mu.Unlock()

	cur, ok := c.fetch(kind, userID)

	c.mu.Lock()
	c.sweepLocked(now)
	c.entries[key] = versionEntry{version: cur, expires: now.Add(c.ttl), ok: ok}
	c.mu.Unlock()
	return ok && cur == version
}

func (c *TokenVersionCache) fetch(kind string, userID uint) (int, bool) {
	if kind == auth.TokenKindAdmin {
		var a models.AdminUser
		if err := c.db.Select("id", "token_version").First(&a, userID).Error; err != nil {
			return 0, false
		}
		return a.TokenVersion, true
	}
	var u models.User
	if err := c.db.Select("id", "token_version").First(&u, userID).Error; err != nil {
		return 0, false
	}
	return u.TokenVersion, true
}

// sweepLocked drops expired entries at most once per TTL.
func (c *TokenVersionCache) sweepLocked(now time.Time) {
	if now.Sub(c.lastSweep) < c.ttl {
		return
	}
	c.lastSweep = now
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
		}
	}
}
