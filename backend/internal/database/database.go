package database

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aiiot/server/internal/config"
	"github.com/aiiot/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect opens a GORM connection and configures the connection pool.
func Connect(cfg config.DBConfig, appEnv string) (*gorm.DB, error) {
	logLevel := logger.Warn
	if appEnv == "development" {
		logLevel = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger:                 logger.Default.LogMode(logLevel),
		SkipDefaultTransaction: true,
		NowFunc:                func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// Migrate creates/updates the relational schema and the time-series tables.
func Migrate(db *gorm.DB, tcfg config.TelemetryConfig) error {
	if err := preMigrate(db); err != nil {
		return fmt.Errorf("pre-migrate: %w", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.ProjectMember{},
		&models.Workspace{},
		&models.Product{},
		&models.ThingModel{},
		&models.ThingModelElement{},
		&models.ThingModelVersion{},
		&models.Device{},
		&models.DeviceLatestValue{},
		&models.DeviceEvent{},
		&models.Rule{},
		&models.RuleExecutionLog{},
		&models.AccessConfig{},
		&models.GatewayInstance{},
		&models.DeviceShadow{},
		&models.DeviceShadowLog{},
		&models.DeviceDownlinkLog{},
		&models.IngestDedup{},
		&models.Alert{},
		&models.Firmware{},
		&models.OTATask{},
		&models.OTATaskDevice{},
		&models.NotifyChannel{},
		&models.NotificationLog{},
		&models.DeviceGroup{},
		&models.DeviceGroupMember{},
		&models.AuditLog{},
		&models.AdminUser{},
		&models.Setting{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	// Backfill legacy products (created before ownership was tracked) to the
	// earliest admin account.
	db.Exec("UPDATE products SET created_by = (SELECT min(id) FROM users) WHERE created_by = 0")

	if err := migrateTelemetry(db); err != nil {
		return fmt.Errorf("migrate telemetry: %w", err)
	}
	if err := InitTelemetryRollup(db); err != nil {
		return fmt.Errorf("init telemetry rollup: %w", err)
	}
	if err := EnsurePartitions(db, tcfg.PartitionAhead); err != nil {
		return fmt.Errorf("ensure partitions: %w", err)
	}
	return nil
}

// preMigrate applies schema changes that GORM AutoMigrate cannot express,
// such as dropping columns/indexes from earlier schema revisions. It is safe
// to run on a fresh database.
func preMigrate(db *gorm.DB) error {
	stmts := []string{
		// Products are global now: drop the legacy project scoping column
		// (this also drops the old idx_product_key index).
		`ALTER TABLE products DROP COLUMN IF EXISTS project_id`,
		// Business users are never administrators (administration has its own
		// identity store); normalise any legacy rows.
		`UPDATE users SET system_role = 'user' WHERE system_role = 'admin'`,
		// Workspace keys are unique per project now (the topic namespace no
		// longer depends on them); drop the legacy global unique index.
		`DROP INDEX IF EXISTS idx_workspace_key`,
		`DROP INDEX IF EXISTS idx_workspaces_key`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			// Ignore "relation does not exist" on a fresh database.
			if !isUndefinedTable(err) {
				return err
			}
		}
	}
	return nil
}

func isUndefinedTable(err error) bool {
	return err != nil && (contains(err.Error(), "does not exist") || contains(err.Error(), "SQLSTATE 42P01"))
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// migrateTelemetry creates the partitioned time-series parent table plus
// supporting indexes. It is idempotent.
func migrateTelemetry(db *gorm.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS telemetry_data (
			id           BIGSERIAL,
			time         TIMESTAMPTZ NOT NULL,
			project_id   BIGINT NOT NULL,
			workspace_id BIGINT NOT NULL,
			product_id   BIGINT NOT NULL,
			device_id    BIGINT NOT NULL,
			identifier   VARCHAR(128) NOT NULL,
			data_type    VARCHAR(32),
			num_value    DOUBLE PRECISION,
			bool_value   BOOLEAN,
			str_value    TEXT,
			json_value   JSONB,
			PRIMARY KEY (id, time)
		) PARTITION BY RANGE (time)`,
		`CREATE INDEX IF NOT EXISTS idx_telemetry_series ON telemetry_data (device_id, identifier, time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_telemetry_project ON telemetry_data (project_id, time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_telemetry_ws ON telemetry_data (workspace_id, time DESC)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// EnsurePartitions creates monthly partitions from the previous month up to
// `ahead` months into the future. Called at startup and by the retention job.
func EnsurePartitions(db *gorm.DB, ahead int) error {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	for i := 0; i <= ahead+1; i++ {
		from := start.AddDate(0, i, 0)
		to := from.AddDate(0, 1, 0)
		name := fmt.Sprintf("telemetry_data_%s", from.Format("200601"))
		stmt := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF telemetry_data FOR VALUES FROM ('%s') TO ('%s')`,
			name, from.Format("2006-01-02"), to.Format("2006-01-02"),
		)
		if err := db.Exec(stmt).Error; err != nil {
			// Ignore "partition already exists for range" type races.
			slog.Warn("create partition", "partition", name, "error", err)
		}
	}
	return nil
}

// DropOldPartitions removes partitions whose upper bound is older than the
// retention window.
func DropOldPartitions(db *gorm.DB, retentionDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	rows, err := db.Raw(`
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'telemetry_data'`).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		names = append(names, n)
	}
	for _, n := range names {
		var suffix string
		if _, err := fmt.Sscanf(n, "telemetry_data_%6s", &suffix); err != nil {
			continue
		}
		t, err := time.Parse("200601", suffix)
		if err != nil {
			continue
		}
		// Partition end (first day of next month) must be before the cutoff.
		if t.AddDate(0, 1, 0).Before(cutoff) {
			slog.Info("dropping old telemetry partition", "partition", n)
			if err := db.Exec("DROP TABLE IF EXISTS " + n).Error; err != nil {
				slog.Warn("drop partition", "partition", n, "error", err)
			}
		}
	}
	return nil
}
