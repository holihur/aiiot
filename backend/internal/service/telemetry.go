package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/aiiot/server/internal/config"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sample is a coerced telemetry value ready for persistence.
type Sample struct {
	DeviceID    uint
	ProjectID   uint
	WorkspaceID uint
	ProductID   uint
	Identifier  string
	DataType    string
	Time        time.Time

	Num  *float64
	Bool *bool
	Str  *string
	JSON models.JSONMap
}

func (s *Sample) row() *models.Telemetry {
	return &models.Telemetry{
		Time:        s.Time.UTC(),
		ProjectID:   s.ProjectID,
		WorkspaceID: s.WorkspaceID,
		ProductID:   s.ProductID,
		DeviceID:    s.DeviceID,
		Identifier:  s.Identifier,
		DataType:    s.DataType,
		NumValue:    s.Num,
		BoolValue:   s.Bool,
		StrValue:    s.Str,
		JSONValue:   s.JSON,
	}
}

// Coerce converts a raw JSON value into the typed representation implied by
// the thing-model data type. Unknown types fall back to JSON storage.
func Coerce(dataType string, raw any) *Sample {
	s := &Sample{DataType: dataType}
	switch dataType {
	case "int32", "int64", "int", "float", "double", "number":
		if f, ok := toFloat(raw); ok {
			s.Num = &f
			return s
		}
	case "bool", "boolean":
		if b, ok := toBool(raw); ok {
			s.Bool = &b
			return s
		}
	case "text", "string", "date", "enum":
		if str, ok := raw.(string); ok {
			s.Str = &str
			return s
		}
		enc, _ := json.Marshal(raw)
		v := string(enc)
		s.Str = &v
		return s
	}
	// json / struct / array / unknown
	if m, ok := raw.(map[string]any); ok {
		s.JSON = models.JSONMap(m)
		return s
	}
	s.JSON = models.JSONMap{"value": raw}
	return s
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func toBool(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		p, err := strconv.ParseBool(b)
		return p, err == nil
	case float64:
		return b != 0, true
	}
	return false, false
}

// TelemetryService persists time-series samples into the partitioned
// PostgreSQL table and maintains a latest-value cache.
type TelemetryService struct {
	db  *gorm.DB
	cfg config.TelemetryConfig
	log *slog.Logger

	ch        chan *models.Telemetry
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func NewTelemetryService(db *gorm.DB, cfg config.TelemetryConfig, log *slog.Logger) *TelemetryService {
	if cfg.WriteBatchSize <= 0 {
		cfg.WriteBatchSize = 512
	}
	if cfg.WriteFlushEvery <= 0 {
		cfg.WriteFlushEvery = 2 * time.Second
	}
	return &TelemetryService{
		db:  db,
		cfg: cfg,
		log: log,
		ch:  make(chan *models.Telemetry, cfg.WriteBatchSize*4),
	}
}

// Start launches the background flush loop.
func (s *TelemetryService) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.WriteFlushEvery)
		defer ticker.Stop()
		buf := make([]*models.Telemetry, 0, s.cfg.WriteBatchSize)
		flush := func() {
			if len(buf) == 0 {
				return
			}
			if err := s.persist(context.Background(), buf); err != nil {
				s.log.Error("telemetry flush failed", "error", err, "rows", len(buf))
			}
			buf = buf[:0]
		}
		for {
			select {
			case <-ctx.Done():
				for {
					select {
					case row := <-s.ch:
						buf = append(buf, row)
						if len(buf) >= s.cfg.WriteBatchSize {
							flush()
						}
					default:
						flush()
						return
					}
				}
			case row := <-s.ch:
				buf = append(buf, row)
				if len(buf) >= s.cfg.WriteBatchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

// Stop drains the buffer and waits for the flush loop to exit.
func (s *TelemetryService) Stop() {
	s.closeOnce.Do(func() {})
	s.wg.Wait()
}

// Write enqueues samples; if the buffer is full it falls back to a synchronous
// write so that no data is silently dropped.
func (s *TelemetryService) Write(ctx context.Context, samples []*Sample) error {
	if len(samples) == 0 {
		return nil
	}
	rows := make([]*models.Telemetry, 0, len(samples))
	for _, smp := range samples {
		rows = append(rows, smp.row())
	}
	select {
	case s.ch <- rows[0]:
	default:
		// buffer pressure: persist the whole batch synchronously
		return s.persist(ctx, rows)
	}
	// move the remaining rows without blocking where possible
	pending := rows[1:]
	for len(pending) > 0 {
		select {
		case s.ch <- pending[0]:
			pending = pending[1:]
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := s.persist(ctx, pending); err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func (s *TelemetryService) persist(ctx context.Context, rows []*models.Telemetry) error {
	if len(rows) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).CreateInBatches(rows, 200).Error; err != nil {
		return fmt.Errorf("insert telemetry: %w", err)
	}
	// A single flush batch can contain multiple samples for the same
	// (device_id, identifier). PostgreSQL's ON CONFLICT DO UPDATE rejects a
	// statement that touches the same row twice, so collapse to the newest.
	latestByKey := make(map[string]*models.DeviceLatestValue, len(rows))
	for _, r := range rows {
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

// RangeQuery filters a time-series query.
type RangeQuery struct {
	DeviceID   uint
	Identifier string
	From       time.Time
	To         time.Time
	Limit      int
}

// QueryRange returns raw samples in [From, To).
func (s *TelemetryService) QueryRange(ctx context.Context, q RangeQuery) ([]models.Telemetry, error) {
	if q.Limit <= 0 || q.Limit > 10000 {
		q.Limit = 1000
	}
	tx := s.db.WithContext(ctx).Where("device_id = ?", q.DeviceID)
	if q.Identifier != "" {
		tx = tx.Where("identifier = ?", q.Identifier)
	}
	if !q.From.IsZero() {
		tx = tx.Where("time >= ?", q.From)
	}
	if !q.To.IsZero() {
		tx = tx.Where("time < ?", q.To)
	}
	var out []models.Telemetry
	err := tx.Order("time DESC").Limit(q.Limit).Find(&out).Error
	return out, err
}

// Latest returns the cached most recent value per identifier for a device.
func (s *TelemetryService) Latest(ctx context.Context, deviceID uint) ([]models.DeviceLatestValue, error) {
	var out []models.DeviceLatestValue
	err := s.db.WithContext(ctx).Where("device_id = ?", deviceID).Order("identifier").Find(&out).Error
	return out, err
}

// AggregatePoint is one downsampled bucket.
type AggregatePoint struct {
	Bucket time.Time `json:"bucket"`
	Avg    *float64  `json:"avg"`
	Min    *float64  `json:"min"`
	Max    *float64  `json:"max"`
	Count  int64     `json:"count"`
}

// QueryAggregate downsamples numeric samples using PostgreSQL date_bin.
func (s *TelemetryService) QueryAggregate(ctx context.Context, q RangeQuery, interval time.Duration) ([]AggregatePoint, error) {
	if interval <= 0 {
		interval = time.Minute
	}
	sql := `SELECT date_bin(?::interval, time, TIMESTAMPTZ '2000-01-01') AS bucket,
	               avg(num_value) AS avg, min(num_value) AS min, max(num_value) AS max, count(*) AS count
	        FROM telemetry_data
	        WHERE device_id = ? AND identifier = ? AND time >= ? AND time < ?
	        GROUP BY bucket ORDER BY bucket`
	var out []AggregatePoint
	err := s.db.WithContext(ctx).Raw(sql, interval.String(), q.DeviceID, q.Identifier, q.From, q.To).Scan(&out).Error
	return out, err
}

// QueryAggregateRollup reads the hourly materialized view instead of the raw
// partitioned table. Only valid for intervals >= 1h (the rollup granularity).
func (s *TelemetryService) QueryAggregateRollup(ctx context.Context, q RangeQuery, interval time.Duration) ([]AggregatePoint, error) {
	if interval < time.Hour {
		interval = time.Hour
	}
	sql := `SELECT date_bin(?::interval, bucket, TIMESTAMPTZ '2000-01-01') AS bucket,
	               avg(avg) AS avg, min(min) AS min, max(max) AS max, sum(count) AS count
	        FROM telemetry_hourly
	        WHERE device_id = ? AND identifier = ? AND bucket >= ? AND bucket < ?
	        GROUP BY bucket ORDER BY bucket`
	var out []AggregatePoint
	err := s.db.WithContext(ctx).Raw(sql, interval.String(), q.DeviceID, q.Identifier, q.From, q.To).Scan(&out).Error
	return out, err
}
