package database

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// RollupInfo reports the state of the hourly telemetry rollup materialized view.
type RollupInfo struct {
	Rows        int64      `json:"rows"`
	SizeBytes   int64      `json:"sizeBytes"`
	LastRefresh *time.Time `json:"lastRefresh"`
}

const rollupLastRefreshKey = "telemetry.rollup_last_refresh"

// InitTelemetryRollup creates the hourly continuous-aggregate materialized view
// and its unique index (required for CONCURRENTLY refresh). It is idempotent.
func InitTelemetryRollup(db *gorm.DB) error {
	stmts := []string{
		`CREATE MATERIALIZED VIEW IF NOT EXISTS telemetry_hourly AS
			SELECT project_id,
			       device_id,
			       identifier,
			       date_trunc('hour', time) AS bucket,
			       avg(num_value) AS avg,
			       min(num_value) AS min,
			       max(num_value) AS max,
			       count(*)       AS count
			FROM telemetry_data
			GROUP BY project_id, device_id, identifier, date_trunc('hour', time)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_telemetry_hourly_unique
			ON telemetry_hourly (project_id, device_id, identifier, bucket)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// RefreshRollup refreshes the materialized view concurrently when possible and
// records the refresh time.
func RefreshRollup(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).Exec("REFRESH MATERIALIZED VIEW CONCURRENTLY telemetry_hourly").Error; err != nil {
		// Fall back to a blocking refresh (e.g. first population).
		if err2 := db.WithContext(ctx).Exec("REFRESH MATERIALIZED VIEW telemetry_hourly").Error; err2 != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return db.WithContext(ctx).Exec(
		`INSERT INTO system_settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		rollupLastRefreshKey, now,
	).Error
}

// GetRollupInfo returns size/row estimates and the last refresh time.
func GetRollupInfo(ctx context.Context, db *gorm.DB) (RollupInfo, error) {
	var info RollupInfo
	err := db.WithContext(ctx).Raw(`
		SELECT c.reltuples::bigint AS rows,
		       pg_total_relation_size(c.oid) AS size_bytes
		FROM pg_class c
		WHERE c.relname = 'telemetry_hourly'`).Row().Scan(&info.Rows, &info.SizeBytes)
	if err != nil {
		return info, err
	}
	var val string
	if e := db.WithContext(ctx).Raw(`SELECT value FROM system_settings WHERE key = ?`, rollupLastRefreshKey).
		Row().Scan(&val); e == nil {
		if t, perr := time.Parse(time.RFC3339, val); perr == nil {
			info.LastRefresh = &t
		}
	}
	return info, nil
}
