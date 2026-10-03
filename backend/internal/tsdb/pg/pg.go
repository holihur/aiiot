// Package pg implements tsdb.Store on top of the partitioned PostgreSQL
// telemetry tables (telemetry_data + telemetry_hourly). This is the default
// backend for self-hosted deployments; swapping to TimescaleDB or a dedicated
// TSDB means implementing tsdb.Store elsewhere — the rest of the core does not
// change.
package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/tsdb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store is the PostgreSQL implementation of tsdb.Store.
type Store struct {
	db *gorm.DB
}

// New wraps a GORM handle.
func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Close() error { return nil }

// Write batch-inserts rows and refreshes the latest-value cache in the same
// style the core previously did directly. Implementations must collapse
// duplicate (device_id, identifier) latest updates — PostgreSQL rejects a
// statement touching the same ON CONFLICT row twice.
func (s *Store) Write(ctx context.Context, rows []tsdb.Row) error {
	if len(rows) == 0 {
		return nil
	}
	trows := make([]*models.Telemetry, 0, len(rows))
	for _, r := range rows {
		trows = append(trows, &models.Telemetry{
			Time:        r.Time.UTC(),
			ProjectID:   r.ProjectID,
			WorkspaceID: r.WorkspaceID,
			ProductID:   r.ProductID,
			DeviceID:    r.DeviceID,
			Identifier:  r.Identifier,
			DataType:    r.DataType,
			NumValue:    r.NumValue,
			BoolValue:   r.BoolValue,
			StrValue:    r.StrValue,
			JSONValue:   r.JSONValue,
		})
	}
	if err := s.db.WithContext(ctx).CreateInBatches(trows, 200).Error; err != nil {
		return fmt.Errorf("insert telemetry: %w", err)
	}
	latestByKey := make(map[string]*models.DeviceLatestValue, len(trows))
	for _, r := range trows {
		key := fmt.Sprintf("%d\x00%s", r.DeviceID, r.Identifier)
		if cur, ok := latestByKey[key]; ok && cur.UpdatedAt.After(r.Time) {
			continue
		}
		latestByKey[key] = &models.DeviceLatestValue{
			DeviceID:    r.DeviceID,
			Identifier:  r.Identifier,
			ProjectID:   r.ProjectID,
			WorkspaceID: r.WorkspaceID,
			ProductID:   r.ProductID,
			DataType:    r.DataType,
			NumValue:    r.NumValue,
			BoolValue:   r.BoolValue,
			StrValue:    r.StrValue,
			JSONValue:   r.JSONValue,
			UpdatedAt:   r.Time,
		}
	}
	latest := make([]*models.DeviceLatestValue, 0, len(latestByKey))
	for _, v := range latestByKey {
		latest = append(latest, v)
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "device_id"}, {Name: "identifier"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"project_id", "workspace_id", "product_id", "data_type",
			"num_value", "bool_value", "str_value", "json_value", "updated_at",
		}),
	}).Create(&latest).Error
}

// tx builds the common WHERE scope.
func (s *Store) tx(ctx context.Context, q tsdb.Query) *gorm.DB {
	tx := s.db.WithContext(ctx)
	if len(q.DeviceIDs) == 1 {
		tx = tx.Where("device_id = ?", q.DeviceIDs[0])
	} else if len(q.DeviceIDs) > 1 {
		tx = tx.Where("device_id IN ?", q.DeviceIDs)
	}
	if len(q.Identifiers) > 0 {
		tx = tx.Where("identifier IN ?", q.Identifiers)
	}
	if !q.From.IsZero() {
		tx = tx.Where("time >= ?", q.From)
	}
	if !q.To.IsZero() {
		tx = tx.Where("time < ?", q.To)
	}
	return tx
}

func (s *Store) QueryRange(ctx context.Context, q tsdb.Query) ([]tsdb.RawPoint, error) {
	limit := q.Limit
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	var rows []models.Telemetry
	err := s.tx(ctx, q).Order("time ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]tsdb.RawPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, tsdb.RawPoint{
			Time:       r.Time,
			DeviceID:   r.DeviceID,
			Identifier: r.Identifier,
			DataType:   r.DataType,
			NumValue:   r.NumValue,
			BoolValue:  r.BoolValue,
			StrValue:   r.StrValue,
			JSONValue:  r.JSONValue,
		})
	}
	return out, nil
}

func (s *Store) QueryAggregate(ctx context.Context, q tsdb.Query, interval time.Duration) ([]tsdb.Point, error) {
	if interval <= 0 {
		interval = time.Minute
	}
	args, where := aggWhere(q, interval, "time")
	sql := `SELECT date_bin(?::interval, time, TIMESTAMPTZ '2000-01-01') AS bucket,
	               identifier, avg(num_value) AS avg, min(num_value) AS min,
	               max(num_value) AS max, count(*) AS count
	        FROM telemetry_data
	        WHERE ` + strings.Join(where, " AND ") + `
	        GROUP BY bucket, identifier ORDER BY bucket, identifier`
	var out []tsdb.Point
	err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&out).Error
	return out, err
}

// QueryAggregateRollup reads the hourly materialized view (rollup granularity
// is 1h; smaller intervals fall back to QueryAggregate).
func (s *Store) QueryAggregateRollup(ctx context.Context, q tsdb.Query, interval time.Duration) ([]tsdb.Point, error) {
	if interval < time.Hour {
		interval = time.Hour
	}
	args, where := aggWhere(q, interval, "bucket")
	sql := `SELECT date_bin(?::interval, bucket, TIMESTAMPTZ '2000-01-01') AS bucket,
	               identifier, avg(avg) AS avg, min(min) AS min, max(max) AS max, sum(count) AS count
	        FROM telemetry_hourly
	        WHERE ` + strings.Join(where, " AND ") + `
	        GROUP BY bucket, identifier ORDER BY bucket, identifier`
	var out []tsdb.Point
	err := s.db.WithContext(ctx).Raw(sql, args...).Scan(&out).Error
	return out, err
}

func (s *Store) Latest(ctx context.Context, deviceID uint) ([]tsdb.LatestRow, error) {
	var rows []models.DeviceLatestValue
	err := s.db.WithContext(ctx).Where("device_id = ?", deviceID).Order("identifier").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]tsdb.LatestRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, tsdb.LatestRow{
			Key:       r.Identifier,
			DataType:  r.DataType,
			NumValue:  r.NumValue,
			BoolValue: r.BoolValue,
			StrValue:  r.StrValue,
			JSONValue: r.JSONValue,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// aggWhere builds placeholders and WHERE clauses for aggregate queries. The
// first placeholder is the interval (used inside the SELECT projection).
func aggWhere(q tsdb.Query, interval time.Duration, timeCol string) ([]any, []string) {
	args := []any{interval.String()}
	where := []string{"1 = 1"}
	if !q.From.IsZero() {
		where = append(where, timeCol+" >= ?")
		args = append(args, q.From)
	}
	if !q.To.IsZero() {
		where = append(where, timeCol+" < ?")
		args = append(args, q.To)
	}
	if len(q.Identifiers) > 0 {
		where = append(where, "identifier IN (?)") // gorm expands the slice
		args = append(args, q.Identifiers)
	}
	return args, where
}
