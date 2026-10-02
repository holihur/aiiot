package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aiiot/server/internal/bus"
)

// Config is the core (control-plane) application configuration. Protocol
// gateways (MQTT / CoAP / custom) are separate programs and load their own
// configuration via gateway.Config.
type Config struct {
	AppEnv   string
	LogLevel string
	HTTPAddr string
	// PublicHost is the host used to build connection instructions handed to
	// devices. Empty means derive it from the request Host header.
	PublicHost string
	// AdminUsername/AdminPassword seed the initial administrator account.
	AdminUsername string
	AdminPassword string
	// WebDir is the directory containing the built frontend (index.html +
	// assets). When it exists the core serves it as an SPA. Empty disables it.
	WebDir    string
	JWTSecret string
	JWTTTL    time.Duration
	DB        DBConfig
	Gateway   GatewayServerConfig
	Ingest    IngestConfig
	OTA       OTAConfig
	Telemetry TelemetryConfig
	// NATS is the mandatory uplink bus between gateways and the core.
	NATS bus.Config
	// CORSAllowedOrigins is an explicit allow-list for credentialed browser
	// requests (comma-separated, from CORS_ALLOWED_ORIGINS). Empty in
	// production means no cross-origin credentials are granted.
	CORSAllowedOrigins []string
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	TimeZone string
	MaxOpen  int
	MaxIdle  int
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode, c.TimeZone,
	)
}

// GatewayServerConfig configures the core side of the gateway protocol.
type GatewayServerConfig struct {
	// Token is the shared secret gateways present as X-Gateway-Token.
	Token string
	// HeartbeatTimeout is how long a gateway may stay silent before it is
	// considered unhealthy.
	HeartbeatTimeout time.Duration
}

// OTAConfig controls firmware storage and the download URL handed to devices.
type OTAConfig struct {
	// Dir is the directory where firmware binaries are stored.
	Dir string
	// PublicBaseURL is the externally reachable core base URL used to build
	// firmware download URLs, e.g. http://10.0.0.5:8080.
	PublicBaseURL string
	// MaxUploadBytes caps a single firmware upload.
	MaxUploadBytes int64
}

// IngestConfig controls how uplinks received from gateways are handled.
type IngestConfig struct {
	// DeviceOfflineAfter marks a device offline when no uplink arrives within
	// this window (checked by a background sweeper).
	DeviceOfflineAfter time.Duration
}

type TelemetryConfig struct {
	RetentionDays   int
	PartitionAhead  int
	WriteBatchSize  int
	WriteFlushEvery time.Duration
}

func Load() *Config {
	loadDotEnv(".env")
	return &Config{
		AppEnv:        getStr("APP_ENV", "development"),
		LogLevel:      getStr("LOG_LEVEL", "info"),
		HTTPAddr:      getStr("HTTP_ADDR", "0.0.0.0:8080"),
		PublicHost:    getStr("PUBLIC_HOST", ""),
		AdminUsername: getStr("ADMIN_USERNAME", "admin"),
		AdminPassword: getStr("ADMIN_PASSWORD", ""),
		WebDir:        getStr("WEB_DIR", "./web/dist"),
		JWTSecret:     getStr("JWT_SECRET", "change-me-in-production-please-32chars"),
		JWTTTL:        getDur("JWT_TTL", 72*time.Hour),
		DB: DBConfig{
			Host:     getStr("DB_HOST", "127.0.0.1"),
			Port:     getInt("DB_PORT", 5432),
			User:     getStr("DB_USER", "postgres"),
			Password: getStr("DB_PASSWORD", "postgres"),
			Name:     getStr("DB_NAME", "aiiot"),
			SSLMode:  getStr("DB_SSLMODE", "disable"),
			TimeZone: getStr("DB_TIMEZONE", "UTC"),
			MaxOpen:  getInt("DB_MAX_OPEN", 25),
			MaxIdle:  getInt("DB_MAX_IDLE", 5),
		},
		Gateway: GatewayServerConfig{
			Token:            getStr("GATEWAY_TOKEN", "change-me-gateway-token"),
			HeartbeatTimeout: getDur("GATEWAY_HEARTBEAT_TIMEOUT", 90*time.Second),
		},
		Ingest: IngestConfig{
			DeviceOfflineAfter: getDur("DEVICE_OFFLINE_AFTER", 5*time.Minute),
		},
		OTA: OTAConfig{
			Dir:            getStr("OTA_DIR", "./ota"),
			PublicBaseURL:  getStr("OTA_PUBLIC_BASE_URL", "http://127.0.0.1:8080"),
			MaxUploadBytes: int64(getInt("OTA_MAX_UPLOAD_MB", 256)) << 20,
		},
		Telemetry: TelemetryConfig{
			RetentionDays:   getInt("TELEMETRY_RETENTION_DAYS", 30),
			PartitionAhead:  getInt("TELEMETRY_PARTITION_AHEAD", 1),
			WriteBatchSize:  getInt("TELEMETRY_WRITE_BATCH_SIZE", 512),
			WriteFlushEvery: getDur("TELEMETRY_FLUSH_INTERVAL", 2*time.Second),
		},
		CORSAllowedOrigins: splitList(getStr("CORS_ALLOWED_ORIGINS", "")),
		NATS: bus.Config{
			URL:      getStr("NATS_URL", "nats://127.0.0.1:4222"),
			User:     getStr("NATS_USER", ""),
			Password: getStr("NATS_PASSWORD", ""),
			Creds:    getStr("NATS_CREDS", ""),
			Stream:   getStr("NATS_STREAM", bus.DefaultStream),
			Subject:  getStr("NATS_UPLINK_SUBJECT", bus.DefaultSubject),
			Durable:  getStr("NATS_DURABLE", bus.DefaultDurable),
			Queue:    getStr("NATS_QUEUE", bus.DefaultQueue),
		},
	}
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func getStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// splitList splits a comma-separated environment value into trimmed entries.
func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// DefaultSecrets are the weak demo credentials shipped in .env.example.
const (
	DefaultJWTSecret    = "change-me-in-production-please-32chars"
	DefaultGatewayToken = "change-me-gateway-token"
)

// IsDefaultSecret reports whether the secret still holds the demo value.
func IsDefaultSecret(v string) bool { return v == DefaultJWTSecret || v == DefaultGatewayToken }

// Validate returns an error for unsafe production settings. The process must
// refuse to start before any traffic is served.
func (c *Config) Validate() error {
	if c.AppEnv != "production" {
		return nil
	}
	if IsDefaultSecret(c.JWTSecret) || len(c.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be replaced by a random secret of at least 32 characters in production")
	}
	if IsDefaultSecret(c.Gateway.Token) || len(c.Gateway.Token) < 16 {
		return errors.New("GATEWAY_TOKEN must be replaced by a random secret of at least 16 characters in production")
	}
	return nil
}
