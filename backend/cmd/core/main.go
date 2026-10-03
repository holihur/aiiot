// Command core is the aiiot control plane: REST API, device registry,
// time-series ingestion, CEL rule engine and the gateway protocol server.
//
// Protocol gateways (mqtt-gateway, coap-gateway, custom-gateway) are separate
// programs that connect to this process.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/api"
	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/bus"
	"github.com/aiiot/server/internal/certs"
	"github.com/aiiot/server/internal/config"
	"github.com/aiiot/server/internal/database"
	"github.com/aiiot/server/internal/gateway"
	"github.com/aiiot/server/internal/logset"
	"github.com/aiiot/server/internal/metrics"
	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/service"
	"github.com/aiiot/server/internal/totp"
	"github.com/nats-io/nats.go"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	log := newLogger(cfg.LogLevel, logset.New(logset.File{
		Path:       cfg.LogFile,
		MaxSizeMB:  cfg.LogMaxSizeMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAgeDays: cfg.LogMaxAgeDays,
		Compress:   cfg.LogCompress,
	}))
	slog.SetDefault(log)
	certMgr, err := certs.LoadOrCreate(os.Getenv("CERTS_DIR"))
	if err != nil {
		log.Error("certificate manager init failed", "error", err)
		os.Exit(1)
	}
	totpCipher, err := totp.NewCipher(cfg.JWTSecret)
	if err != nil {
		log.Error("totp cipher init failed", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		log.Error("unsafe configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Connect(cfg.DB, cfg.AppEnv)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.Migrate(db, cfg.Telemetry); err != nil {
		log.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	log.Info("database ready")
	api.SeedAdmin(db, cfg.AdminUsername, cfg.AdminPassword, log)

	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTTTL)
	resolver := service.NewResolver(db)
	telemetry := service.NewTelemetryService(service.NewPG(db), cfg.Telemetry, log)
	registry := gateway.NewRegistry(db, cfg.Gateway.HeartbeatTimeout)
	downlink := service.NewDownlinkService(db, resolver, registry, log)
	shadow := service.NewShadowService(db, downlink, log)
	ota := service.NewOTAService(db, downlink, resolver, cfg.OTA, log)
	notifier := service.NewNotifier(db, log)
	alerter := service.NewAlerter(db, log)
	settings := service.NewSettings(db)
	hub := service.NewHub()

	rules, err := service.NewRuleEngine(db, downlink, shadow, resolver, notifier, alerter, log)
	if err != nil {
		log.Error("rule engine init failed", "error", err)
		os.Exit(1)
	}
	// Rules can trigger on shadow-delta state changes.
	shadow.SetDeltaHook(func(hctx context.Context, dc *service.DeviceContext, delta map[string]any) {
		rules.Evaluate(hctx, &service.RuleEvent{
			Kind:        access.KindShadowDelta,
			ProjectID:   dc.Project.ID,
			WorkspaceID: dc.Workspace.ID,
			ProductID:   dc.Product.ID,
			DeviceID:    dc.Device.ID,
			DeviceKey:   dc.Device.Key,
			Params:      delta,
			Value:       delta,
			Now:         time.Now().UTC(),
			Device:      dc.Device,
		})
	})
	geofence := service.NewGeofenceService(db, log)
	ingest := service.NewIngestService(db, resolver, telemetry, rules, downlink, shadow, ota, hub, geofence, cfg.Ingest.DeviceOfflineAfter, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// NATS is a mandatory platform component: the core consumes device uplinks
	// from the JetStream stream via a durable, queue-grouped consumer.
	// Real-time event bus: every replica subscribes to aiiot.events and
	// forwards events to its local SSE hub, so multi-replica deployments show
	// the same live feed. A plain (non-JetStream) fan-out connection is used.
	evConn, err := bus.NewEventConn(cfg.NATS)
	if err != nil {
		log.Warn("nats event bus unavailable; SSE limited to this replica", "error", err)
	} else {
		defer evConn.Close()
		ingest.SetEventBus(evConn)
		if _, err := evConn.Subscribe(service.EventSubject, func(m *nats.Msg) {
			var ev service.Event
			if err := json.Unmarshal(m.Data, &ev); err != nil {
				log.Warn("event decode failed", "error", err)
				return
			}
			hub.Publish(ev)
		}); err != nil {
			log.Warn("subscribe event subject failed", "error", err)
		}
	}

	natsSubject := cfg.NATS.Subject
	if natsSubject == "" {
		natsSubject = bus.DefaultSubject
	}
	uplinkSub, err := bus.NewSubscriber(cfg.NATS, func(data []byte, delivered uint64) error {
		start := time.Now()
		var env gateway.UplinkEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			// Malformed frames cannot be fixed by redelivery; drop and log.
			log.Warn("nats uplink decode failed", "error", err)
			metrics.ObserveNATSConsume(natsSubject, err, delivered, start)
			return nil
		}
		msg := env.ToAccess()
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now().UTC()
		}
		ingestErr := ingest.Handle(context.Background(), msg)
		metrics.ObserveNATSConsume(natsSubject, ingestErr, delivered, start)
		if ingestErr != nil {
			// Deterministic failures (bad payload, unknown kind) are
			// acknowledged and dropped; redelivering them would loop forever
			// (JetStream Nak storm). Logs are rate-limited per device via
			// ratelimitIngestLog.
			if errors.Is(ingestErr, service.ErrInvalidUplink) {
				rateLimitedIngestLog(log, msg.Device.DeviceKey, ingestErr)
				return nil
			}
			log.Warn("ingest failed", "error", ingestErr, "device", msg.Device.DeviceKey, "kind", msg.Kind)
			return ingestErr // transient failure -> redeliver
		}
		return nil
	})
	if err != nil {
		log.Error("nats uplink subscriber failed", "error", err)
		os.Exit(1)
	}
	defer uplinkSub.Close()

	telemetry.Start(ctx)
	notifier.Start(ctx)
	if err := registry.Load(ctx); err != nil {
		log.Warn("load gateway registry failed", "error", err)
	}
	if err := rules.Reload(ctx); err != nil {
		log.Warn("initial rule reload failed", "error", err)
	}
	if err := geofence.Reload(ctx); err != nil {
		log.Warn("initial geofence reload failed", "error", err)
	}

	handlers := &api.Handlers{
		DB:               db,
		Tokens:           tokens,
		Resolver:         resolver,
		Telemetry:        telemetry,
		Ingest:           ingest,
		Rules:            rules,
		Shadow:           shadow,
		OTA:              ota,
		Notifier:         notifier,
		Settings:         settings,
		Hub:              hub,
		Downlink:         downlink,
		Registry:         registry,
		GatewayToken:     cfg.Gateway.Token,
		Certs:            certMgr,
		TOTP:             totpCipher,
		Geofence:         geofence,
		PublicHost:       cfg.PublicHost,
		RetentionDefault: cfg.Telemetry.RetentionDays,
		Log:              log,
		Bus:              uplinkSub,
		NATSSubject:      natsSubject,
		CORSAllowed:      cfg.CORSAllowedOrigins,
		AppEnv:           cfg.AppEnv,
	}
	handlers.RestoreDemo(ctx)
	router := api.NewRouter(handlers)
	if api.RegisterStatic(router, cfg.WebDir) {
		log.Info("serving frontend", "dir", cfg.WebDir)
	} else if cfg.WebDir != "" {
		log.Warn("frontend directory not found, static hosting disabled", "dir", cfg.WebDir)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go backgroundJobs(ctx, db, cfg, ingest, registry, alerter, rules, log)

	go func() {
		log.Info("core HTTP server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down core")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	telemetry.Stop()
	log.Info("core stopped")
}

// backgroundJobs runs maintenance loop the same schedule on every replica; a
// PostgreSQL advisory lock (database.Exclusive) ensures each job actually runs
// on exactly one replica at a time.
// rateLimitedIngestLog throttles per-device invalid-uplink warnings so a
// poisoned device can never flood the log (one line per device / 30s).
func rateLimitedIngestLog(log *slog.Logger, deviceKey string, err error) {
	key := deviceKey
	now := time.Now()
	last, _ := ingestLogStamp.LoadOrStore(key, now)
	if lt, ok := last.(time.Time); ok && now.Sub(lt) < 30*time.Second {
		return
	}
	ingestLogStamp.Store(key, now)
	log.Warn("ingest failed (throttled)", "error", err, "device", deviceKey)
}

var ingestLogStamp sync.Map

func backgroundJobs(ctx context.Context, db *gorm.DB, cfg *config.Config, ingest *service.IngestService, registry *gateway.Registry, alerter *service.Alerter, rules *service.RuleEngine, log *slog.Logger) {
	scheduleTicker := time.NewTicker(30 * time.Second)
	offlineTicker := time.NewTicker(time.Minute)
	partitionTicker := time.NewTicker(6 * time.Hour)
	retentionTicker := time.NewTicker(12 * time.Hour)
	rollupTicker := time.NewTicker(5 * time.Minute)
	dedupTicker := time.NewTicker(time.Hour)
	metricsTicker := time.NewTicker(30 * time.Second)
	escalateTicker := time.NewTicker(time.Minute)
	defer offlineTicker.Stop()
	defer partitionTicker.Stop()
	defer retentionTicker.Stop()
	defer rollupTicker.Stop()
	defer dedupTicker.Stop()
	defer metricsTicker.Stop()
	defer escalateTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-scheduleTicker.C:
			rules.ScheduleTick(ctx, now)
		case <-offlineTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyOfflineSweep, func() error {
				ingest.SweepOffline(ctx)
				return nil
			}); err != nil {
				log.Warn("offline sweep lock failed", "error", err)
			}
		case <-metricsTicker.C:
			updateGauges(ctx, db, registry)
		case <-escalateTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyAlertEscalation, func() error {
				after := time.Duration(service.NewSettings(db).GetInt(ctx, "alert.escalate_after_minutes", 15)) * time.Minute
				if _, err := alerter.EscalateDue(ctx, after); err != nil {
					log.Warn("alert escalation failed", "error", err)
					return err
				}
				return nil
			}); err != nil {
				log.Warn("alert escalation job failed", "error", err)
			}
		case <-partitionTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyPartitions, func() error {
				if err := database.EnsurePartitions(db, cfg.Telemetry.PartitionAhead); err != nil {
					log.Warn("ensure partitions failed", "error", err)
					return err
				}
				return nil
			}); err != nil {
				log.Warn("partition job failed", "error", err)
			}
		case <-rollupTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyRollup, func() error {
				if err := database.RefreshRollup(ctx, db); err != nil {
					log.Warn("refresh telemetry rollup failed", "error", err)
					return err
				}
				return nil
			}); err != nil {
				log.Warn("rollup job failed", "error", err)
			}
		case <-retentionTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyRetention, func() error {
				days := service.NewSettings(db).GetInt(ctx, "telemetry.retention_days", cfg.Telemetry.RetentionDays)
				if days > 0 {
					if err := database.DropOldPartitions(db, days); err != nil {
						log.Warn("drop old partitions failed", "error", err)
						return err
					}
				}
				return nil
			}); err != nil {
				log.Warn("retention job failed", "error", err)
			}
		case <-dedupTicker.C:
			if err := database.Exclusive(ctx, db, database.LockKeyDedupCleanup, func() error {
				if err := db.WithContext(ctx).Exec("DELETE FROM ingest_dedup WHERE ingested_at < now() - interval '24 hours'").Error; err != nil {
					log.Warn("dedup cleanup failed", "error", err)
					return err
				}
				return nil
			}); err != nil {
				log.Warn("dedup cleanup job failed", "error", err)
			}
		}
	}
}

// updateGauges refreshes the periodically-computed Prometheus gauges.
func updateGauges(ctx context.Context, db *gorm.DB, registry *gateway.Registry) {
	var online int64
	if err := db.WithContext(ctx).Model(&models.Device{}).Where("online = ?", true).Count(&online).Error; err == nil {
		metrics.SetDevicesOnline(float64(online))
	}
	healthy := 0
	for _, inst := range registry.List() {
		if inst.Healthy {
			healthy++
		}
	}
	metrics.SetGatewaysHealthy(float64(healthy))
}

func newLogger(level string, out io.Writer) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: lvl}))
}
