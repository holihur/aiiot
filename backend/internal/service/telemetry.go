package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/aiiot/server/internal/config"
	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/tsdb"
	"github.com/aiiot/server/internal/tsdb/pg"
	"gorm.io/gorm"
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

// TelemetryService buffers and persists time-series samples and answers
// queries. All storage goes through the tsdb.Store backend, so the service
// (and every caller) is backend-agnostic: PostgreSQL today, TimescaleDB or a
// dedicated TSDB tomorrow.
type TelemetryService struct {
	store tsdb.Store
	cfg   config.TelemetryConfig
	log   *slog.Logger

	rollup tsdb.RollupStore // nil when the backend has no hourly rollup

	ch        chan *models.Telemetry
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func NewTelemetryService(store tsdb.Store, cfg config.TelemetryConfig, log *slog.Logger) *TelemetryService {
	if cfg.WriteBatchSize <= 0 {
		cfg.WriteBatchSize = 512
	}
	if cfg.WriteFlushEvery <= 0 {
		cfg.WriteFlushEvery = 2 * time.Second
	}
	rollup, _ := store.(tsdb.RollupStore)
	return &TelemetryService{
		store:  store,
		cfg:    cfg,
		log:    log,
		rollup: rollup,
		ch:     make(chan *models.Telemetry, cfg.WriteBatchSize*4),
	}
}

// NewPG builds the default partitioned-PostgreSQL backend.
func NewPG(db *gorm.DB) *pg.Store { return pg.New(db) }

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
	batch := make([]tsdb.Row, 0, len(rows))
	for _, r := range rows {
		batch = append(batch, tsdb.Row{
			Time:        r.Time,
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
	return s.store.Write(ctx, batch)
}

// RangeQuery filters a time-series query.
type RangeQuery struct {
	DeviceID    uint
	Identifier  string   // single identifier ("" = all)
	Identifiers []string // multiple identifiers (overrides Identifier)
	From        time.Time
	To          time.Time
	Limit       int
}

// QueryRange returns raw samples in [From, To), oldest first.
func (s *TelemetryService) QueryRange(ctx context.Context, q RangeQuery) ([]models.Telemetry, error) {
	if q.Limit <= 0 || q.Limit > 10000 {
		q.Limit = 1000
	}
	ids := q.Identifiers
	if len(ids) == 0 && q.Identifier != "" {
		ids = []string{q.Identifier}
	}
	rows, err := s.store.QueryRange(ctx, tsdb.Query{
		DeviceIDs:   []uint{q.DeviceID},
		Identifiers: ids,
		From:        q.From,
		To:          q.To,
		Limit:       q.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]models.Telemetry, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.Telemetry{
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

// Latest returns the cached most recent value per identifier for a device.
func (s *TelemetryService) Latest(ctx context.Context, deviceID uint) ([]models.DeviceLatestValue, error) {
	rows, err := s.store.Latest(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	out := make([]models.DeviceLatestValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.DeviceLatestValue{
			DeviceID:   deviceID,
			Identifier: r.Key,
			DataType:   r.DataType,
			NumValue:   r.NumValue,
			BoolValue:  r.BoolValue,
			StrValue:   r.StrValue,
			JSONValue:  r.JSONValue,
			UpdatedAt:  r.UpdatedAt,
		})
	}
	return out, nil
}

// AggregatePoint is one downsampled bucket for one identifier.
type AggregatePoint struct {
	Bucket     time.Time `json:"bucket"`
	Identifier string    `json:"identifier,omitempty"`
	Avg        *float64  `json:"avg"`
	Min        *float64  `json:"min"`
	Max        *float64  `json:"max"`
	Count      int64     `json:"count"`
}

// QueryAggregate downsamples numeric samples via the backend.
func (s *TelemetryService) QueryAggregate(ctx context.Context, q RangeQuery, interval time.Duration) ([]AggregatePoint, error) {
	points, err := s.store.QueryAggregate(ctx, toTSQuery(q), interval)
	if err != nil {
		return nil, err
	}
	return toServicePoints(points), nil
}

// QueryAggregateRollup reads the hourly materialized view when the backend
// provides one; otherwise it falls back to QueryAggregate.
func (s *TelemetryService) QueryAggregateRollup(ctx context.Context, q RangeQuery, interval time.Duration) ([]AggregatePoint, error) {
	if s.rollup == nil {
		return s.QueryAggregate(ctx, q, interval)
	}
	points, err := s.rollup.QueryAggregateRollup(ctx, toTSQuery(q), interval)
	if err != nil {
		return nil, err
	}
	return toServicePoints(points), nil
}

// toTSQuery converts the service query shape to the backend-agnostic one.
func toTSQuery(q RangeQuery) tsdb.Query {
	ids := q.Identifiers
	if len(ids) == 0 && q.Identifier != "" {
		ids = []string{q.Identifier}
	}
	return tsdb.Query{
		DeviceIDs:   []uint{q.DeviceID},
		Identifiers: ids,
		From:        q.From,
		To:          q.To,
		Limit:       q.Limit,
	}
}

func toServicePoints(points []tsdb.Point) []AggregatePoint {
	out := make([]AggregatePoint, 0, len(points))
	for _, p := range points {
		out = append(out, AggregatePoint{
			Bucket:     p.Bucket,
			Identifier: p.Identifier,
			Avg:        p.Avg,
			Min:        p.Min,
			Max:        p.Max,
			Count:      p.Count,
		})
	}
	return out
}
