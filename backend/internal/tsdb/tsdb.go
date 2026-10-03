// Package tsdb defines the time-series storage contract. The ingestion and
// query paths depend only on this interface; concrete backends (PostgreSQL
// partitions, TimescaleDB, InfluxDB, ...) implement it. Nothing outside the
// store implementation talks SQL.
package tsdb

import (
	"context"
	"time"

	"github.com/aiiot/server/internal/models"
)

// Kind enumerates the logical categories of stored time-series rows.
const (
	KindProperty = "property"
	KindEvent    = "event"
)

// Row is one normalized time-series sample ready for persistence.
type Row struct {
	Time        time.Time
	ProjectID   uint
	WorkspaceID uint
	ProductID   uint
	DeviceID    uint
	Kind        string
	Identifier  string
	DataType    string
	NumValue    *float64
	BoolValue   *bool
	StrValue    *string
	JSONValue   models.JSONMap
}

// RawPoint is a stored sample returned by a range query.
type RawPoint struct {
	Time       time.Time
	DeviceID   uint
	Identifier string
	DataType   string
	NumValue   *float64
	BoolValue  *bool
	StrValue   *string
	JSONValue  models.JSONMap
}

// Point is one downsampled bucket for one identifier.
type Point struct {
	Bucket     time.Time
	Identifier string
	Avg        *float64
	Min        *float64
	Max        *float64
	Count      int64
}

// LatestRow is the cached latest value for a (device, identifier) pair.
type LatestRow struct {
	Key       string
	DataType  string
	NumValue  *float64
	BoolValue *bool
	StrValue  *string
	JSONValue models.JSONMap
	UpdatedAt time.Time
}

// Query is a time-boxed, optionally filtered read.
type Query struct {
	DeviceIDs   []uint
	Identifiers []string
	From        time.Time
	To          time.Time
	Limit       int
}

// Store is the time-series backend contract. Implementations must be safe for
// concurrent use. Writes are batched by the caller; backends may further
// optimize internally.
type Store interface {
	// Write persists a batch of samples at-least-once (callers dedupe).
	Write(ctx context.Context, rows []Row) error

	// QueryRange returns raw samples ordered by time ascending, capped by
	// q.Limit (<= 0 means the store's default cap).
	QueryRange(ctx context.Context, q Query) ([]RawPoint, error)

	// QueryAggregate downsamples numeric samples by interval.
	QueryAggregate(ctx context.Context, q Query, interval time.Duration) ([]Point, error)

	// Latest returns the cached latest value per identifier for a device.
	Latest(ctx context.Context, deviceID uint) ([]LatestRow, error)

	// Close releases backend resources.
	Close() error
}

// RollupStore is an optional capability: aggregated reads served from a
// pre-aggregated (hourly) store instead of raw rows. Backends that lack it are
// still valid: callers fall back to QueryAggregate.
type RollupStore interface {
	QueryAggregateRollup(ctx context.Context, q Query, interval time.Duration) ([]Point, error)
}
