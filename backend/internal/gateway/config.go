package gateway

import (
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aiiot/server/internal/bus"
	"github.com/aiiot/server/internal/logset"
)

// LoadHarnessConfig builds a HarnessConfig from environment variables shared
// by all gateways.
//
//	GATEWAY_CORE_URL         core control-plane base URL
//	GATEWAY_TOKEN            shared secret (must match core GATEWAY_TOKEN)
//	GATEWAY_INSTANCE_ID      unique gateway instance id
//	GATEWAY_DOWNLINK_ADDR    local listen address for core->gateway downlinks
//	GATEWAY_ADVERTISE_HOST   host/IP the core uses to reach this gateway
//	GATEWAY_DOWNLINK_URL     fully-formed downlink URL (overrides the above)
//	GATEWAY_HEARTBEAT        heartbeat interval, e.g. 30s
//	GATEWAY_AUTH_CACHE_TTL   device auth cache TTL, e.g. 5m
//	NATS_URL                 NATS server URL (required; uplinks go through NATS)
//	NATS_USER / NATS_PASSWORD optional NATS credentials
//	NATS_CREDS               optional path to a .creds file
func LoadHarnessConfig(protocol, defaultDownlinkAddr, version string) HarnessConfig {
	return HarnessConfig{
		CoreURL:        GetStr("GATEWAY_CORE_URL", "http://127.0.0.1:8080"),
		Token:          GetStr("GATEWAY_TOKEN", "change-me-gateway-token"),
		InstanceID:     GetStr("GATEWAY_INSTANCE_ID", protocol+"-1"),
		Protocol:       protocol,
		Version:        version,
		DownlinkAddr:   GetStr("GATEWAY_DOWNLINK_ADDR", defaultDownlinkAddr),
		DownlinkURL:    GetStr("GATEWAY_DOWNLINK_URL", ""),
		AdvertiseHost:  GetStr("GATEWAY_ADVERTISE_HOST", "127.0.0.1"),
		HeartbeatEvery: GetDur("GATEWAY_HEARTBEAT", 30*time.Second),
		AuthCacheTTL:   GetDur("GATEWAY_AUTH_CACHE_TTL", 5*time.Minute),
		NATS: bus.Config{
			URL:      GetStr("NATS_URL", "nats://127.0.0.1:4222"),
			User:     GetStr("NATS_USER", ""),
			Password: GetStr("NATS_PASSWORD", ""),
			Creds:    GetStr("NATS_CREDS", ""),
		},
	}
}

func GetStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func GetInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func GetBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func GetDur(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// LogWriter returns the shared gateway log destination: stdout by default,
// or a lumberjack rolling file when LOG_FILE is set (rotation, pruning and
// gzip controlled by LOG_MAX_SIZE_MB / LOG_MAX_BACKUPS / LOG_MAX_AGE_DAYS /
// LOG_COMPRESS).
func LogWriter() io.Writer {
	return logset.New(logset.File{
		Path:       GetStr("LOG_FILE", ""),
		MaxSizeMB:  GetInt("LOG_MAX_SIZE_MB", 100),
		MaxBackups: GetInt("LOG_MAX_BACKUPS", 5),
		MaxAgeDays: GetInt("LOG_MAX_AGE_DAYS", 14),
		Compress:   GetBool("LOG_COMPRESS", true),
	})
}
