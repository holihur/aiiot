package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/bus"
	"github.com/aiiot/server/internal/metrics"
)

// HarnessConfig configures the gateway harness.
type HarnessConfig struct {
	CoreURL      string
	Token        string
	InstanceID   string
	Protocol     string
	Version      string
	DownlinkAddr string
	DownlinkURL  string
	// AdvertiseHost is the hostname/IP the core uses to reach this gateway's
	// downlink endpoint. Defaults to 127.0.0.1 for single-host setups.
	AdvertiseHost  string
	HeartbeatEvery time.Duration
	AuthCacheTTL   time.Duration
	Logger         *slog.Logger
	// NATS is the mandatory uplink bus. URL is required; the gateway refuses
	// to start without a reachable NATS server.
	NATS bus.Config
}

// Run wires an access.Adapter to the core control plane and blocks until ctx
// is cancelled or the adapter fails. A protocol gateway typically consists of
// nothing more than implementing Adapter and calling Run.
func Run(ctx context.Context, a access.Adapter, cfg HarnessConfig) error {
	if cfg.HeartbeatEvery <= 0 {
		cfg.HeartbeatEvery = 30 * time.Second
	}
	if cfg.AuthCacheTTL <= 0 {
		cfg.AuthCacheTTL = 5 * time.Minute
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	downlinkURL := cfg.DownlinkURL
	if downlinkURL == "" {
		host := cfg.AdvertiseHost
		if host == "" {
			host = "127.0.0.1"
		}
		downlinkURL = fmt.Sprintf("http://%s%s/downlink", host, normalizeAddr(cfg.DownlinkAddr))
	}

	client := NewClient(ClientConfig{
		CoreURL:     cfg.CoreURL,
		Token:       cfg.Token,
		InstanceID:  cfg.InstanceID,
		Protocol:    cfg.Protocol,
		Version:     cfg.Version,
		DownlinkURL: downlinkURL,
	})

	cache := newAuthCache(cfg.AuthCacheTTL)

	// NATS is a mandatory platform component: uplinks (the data plane) flow
	// through JetStream so the core can buffer and scale independently.
	pub, err := bus.NewPublisher(cfg.NATS)
	if err != nil {
		return fmt.Errorf("nats uplink bus: %w", err)
	}
	defer pub.Close()

	// Device authentication is delegated to the core, with a short TTL cache.
	a.OnAuth(func(ctx context.Context, req *access.AuthRequest) (*access.AuthResponse, error) {
		if resp, ok := cache.get(req); ok {
			return resp, nil
		}
		resp, err := client.Authenticate(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.Authorized {
			cache.set(req, resp)
		}
		return resp, nil
	})

	// Uplinks are published to NATS (the mandatory data plane). The HTTP
	// uplink remains as a fallback for transient NATS failures.
	a.OnUplink(func(ctx context.Context, msg *access.UplinkMessage) error {
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now().UTC()
		}
		data, err := json.Marshal(UplinkFromAccess(msg))
		if err != nil {
			return err
		}
		pubStart := time.Now()
		pubErr := pub.Publish(ctx, data)
		metrics.ObserveNATSPublish(pub.Subject(), pubErr, pubStart)
		if pubErr == nil {
			return nil
		} else {
			log.Warn("nats uplink publish failed, falling back to HTTP", "error", pubErr, "kind", msg.Kind, "device", msg.Device.DeviceKey)
		}
		var lastErr error
		backoff := 100 * time.Millisecond
		for attempt := 0; attempt < 3; attempt++ {
			if err := client.Uplink(ctx, msg); err == nil {
				return nil
			} else {
				lastErr = err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		log.Error("uplink delivery failed", "error", lastErr, "kind", msg.Kind, "device", msg.Device.DeviceKey)
		return lastErr
	})

	// Downlink HTTP endpoint: the core pushes commands here.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/downlink", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var env DownlinkEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			http.Error(w, "invalid downlink envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.Downlink(r.Context(), env.ToAccess()); err != nil {
			http.Error(w, "downlink failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := &http.Server{Addr: cfg.DownlinkAddr, Handler: mux}
	started := time.Now()
	errCh := make(chan error, 2)

	go func() {
		log.Info("gateway downlink listening", "addr", cfg.DownlinkAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("downlink server: %w", err)
		}
	}()

	// Register with the core, retrying until the core is reachable.
	if err := registerWithRetry(ctx, client, log); err != nil {
		return err
	}

	go heartbeatLoop(ctx, client, a, cfg.HeartbeatEvery, started, log)

	go func() {
		log.Info("adapter starting", "protocol", cfg.Protocol, "instance", cfg.InstanceID)
		if err := a.Start(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("adapter %s: %w", cfg.Protocol, err)
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = a.Stop(shutdownCtx)
		log.Info("gateway stopped", "protocol", cfg.Protocol)
		return nil
	case err := <-errCh:
		return err
	}
}

// normalizeAddr converts a listen address ("0.0.0.0:9101", ":9101",
// "127.0.0.1:9101") into a ":port" suffix.
func normalizeAddr(addr string) string {
	if addr == "" {
		return ""
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		if _, e := strconv.Atoi(addr); e == nil {
			return ":" + addr
		}
		return ""
	}
	return ":" + port
}

func registerWithRetry(ctx context.Context, client *Client, log *slog.Logger) error {
	backoff := time.Second
	for {
		resp, err := client.Register(ctx)
		if err == nil {
			log.Info("gateway registered", "instance", client.InstanceID(), "protocol", client.Protocol())
			if resp.HeartbeatIntervalSeconds > 0 {
				_ = resp
			}
			return nil
		}
		log.Warn("gateway register failed, retrying", "error", err, "in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func heartbeatLoop(ctx context.Context, client *Client, a access.Adapter, every time.Duration, started time.Time, log *slog.Logger) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			active := 0
			if s, ok := a.(interface{ ActiveDevices() int }); ok {
				active = s.ActiveDevices()
			}
			if err := client.Heartbeat(ctx, active, time.Since(started)); err != nil {
				log.Warn("heartbeat failed, re-registering", "error", err)
				if _, rerr := client.Register(ctx); rerr != nil {
					log.Warn("re-register failed", "error", rerr)
				}
			}
		}
	}
}

// --- auth cache -----------------------------------------------------------

type authCacheEntry struct {
	resp    *access.AuthResponse
	expires time.Time
}

type authCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]authCacheEntry
}

func newAuthCache(ttl time.Duration) *authCache {
	return &authCache{ttl: ttl, m: make(map[string]authCacheEntry)}
}

func authKey(req *access.AuthRequest) string {
	return req.ProductKey + "\x00" + req.DeviceKey + "\x00" + req.Password
}

func (c *authCache) get(req *access.AuthRequest) (*access.AuthResponse, bool) {
	c.mu.RLock()
	e, ok := c.m[authKey(req)]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.resp, true
}

func (c *authCache) set(req *access.AuthRequest, resp *access.AuthResponse) {
	c.mu.Lock()
	c.m[authKey(req)] = authCacheEntry{resp: resp, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
