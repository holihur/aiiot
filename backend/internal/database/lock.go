package database

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

// Advisory lock keys, one per background job. Multiple core replicas all run
// the same tickers; the advisory lock guarantees each job executes on exactly
// one replica at a time (partition maintenance, retention, rollup refresh,
// offline sweep, dedup cleanup).
const (
	LockKeyOfflineSweep    int64 = 101
	LockKeyPartitions      int64 = 102
	LockKeyRetention       int64 = 103
	LockKeyRollup          int64 = 104
	LockKeyDedupCleanup    int64 = 105
	LockKeyAlertEscalation int64 = 106
)

// Exclusive runs fn while holding a PostgreSQL advisory lock on a single
// pooled connection. If another replica already holds the lock, fn is skipped
// and nil is returned. Uses pg_try_advisory_lock so a replica never blocks.
func Exclusive(ctx context.Context, db *gorm.DB, key int64, fn func() error) error {
	sqlDB, ok := db.ConnPool.(*sql.DB)
	if !ok {
		// No connection pool (tests / unusual setups): run inline.
		return fn()
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// The lock is session-scoped, so lock -> work -> unlock must happen on the
	// same connection. That is why we take a dedicated *sql.Conn.
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return nil // another replica is already running this job
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", key) //nolint:errcheck
	return fn()
}
