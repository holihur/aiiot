// Package mqtt implements the MQTT access adapter. It runs an embedded MQTT
// broker (mochi-mqtt) and is intended to be run as a standalone gateway
// process under package gateway.
package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aiiot/server/internal/access"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// Options configures the MQTT gateway adapter.
type Options struct {
	InstanceID string
	TCPAddr    string
	WSAddr     string
	EnableWS   bool
	// TLSAddr enables an MQTT-over-TLS listener; the workspace/tenant is
	// derived from the TLS SNI (e.g. factory1.example.com -> factory1).
	TLSAddr string
	TLSCert string
	TLSKey  string
	// TLSCAFile, when set, enables mutual TLS: connecting devices must present
	// a client certificate signed by this CA (the platform CA.crt). The
	// certificate CN is then used for device authentication.
	TLSCAFile string
	Logger    *slog.Logger
}

// Adapter is the MQTT access adapter.
type Adapter struct {
	opts   Options
	log    *slog.Logger
	server *mqtt.Server

	uplink access.UplinkHandler
	auth   access.AuthHandler

	sessions sync.Map // clientID -> *access.AuthResponse
	active   int64
}

// New creates a new MQTT adapter.
func New(opts Options) *Adapter {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	if opts.InstanceID == "" {
		opts.InstanceID = "mqtt-1"
	}
	return &Adapter{opts: opts, log: log}
}

var _ access.Adapter = (*Adapter)(nil)

func (a *Adapter) Name() string { return access.ProtocolMQTTName }

func (a *Adapter) OnUplink(h access.UplinkHandler) { a.uplink = h }
func (a *Adapter) OnAuth(h access.AuthHandler)     { a.auth = h }

// ActiveDevices reports the number of currently authenticated device sessions.
func (a *Adapter) ActiveDevices() int { return int(atomic.LoadInt64(&a.active)) }

func (a *Adapter) Start(ctx context.Context) error {
	server := mqtt.New(&mqtt.Options{
		InlineClient: true,
		Logger:       a.log,
	})
	if err := server.AddHook(&brokerHook{a: a}, nil); err != nil {
		return fmt.Errorf("add mqtt hook: %w", err)
	}
	if a.opts.TCPAddr != "" {
		if err := server.AddListener(listeners.NewTCP(listeners.Config{
			ID:      a.opts.InstanceID + "-tcp",
			Address: a.opts.TCPAddr,
		})); err != nil {
			return fmt.Errorf("add tcp listener: %w", err)
		}
	}
	if a.opts.EnableWS && a.opts.WSAddr != "" {
		if err := server.AddListener(listeners.NewWebsocket(listeners.Config{
			ID:      a.opts.InstanceID + "-ws",
			Address: a.opts.WSAddr,
		})); err != nil {
			return fmt.Errorf("add websocket listener: %w", err)
		}
	}
	if a.opts.TLSAddr != "" && a.opts.TLSCert != "" && a.opts.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(a.opts.TLSCert, a.opts.TLSKey)
		if err != nil {
			return fmt.Errorf("load tls cert: %w", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		if a.opts.TLSCAFile != "" {
			caPEM, err := os.ReadFile(a.opts.TLSCAFile)
			if err != nil {
				return fmt.Errorf("read client CA: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return fmt.Errorf("invalid client CA file %s", a.opts.TLSCAFile)
			}
			tlsCfg.ClientCAs = pool
			tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		}
		if err := server.AddListener(listeners.NewTCP(listeners.Config{
			ID:        a.opts.InstanceID + "-tls",
			Address:   a.opts.TLSAddr,
			TLSConfig: tlsCfg,
		})); err != nil {
			return fmt.Errorf("add tls listener: %w", err)
		}
	}
	a.server = server

	if err := server.Serve(); err != nil {
		return fmt.Errorf("serve mqtt: %w", err)
	}
	if err := waitForTCP(a.opts.TCPAddr, 3*time.Second); err != nil {
		a.log.Warn("mqtt tcp listener not confirmed ready", "addr", a.opts.TCPAddr, "error", err)
	}

	<-ctx.Done()
	return nil
}

func (a *Adapter) Stop(ctx context.Context) error {
	if a.server == nil {
		return nil
	}
	return a.server.Close()
}

// Downlink publishes a message to a device topic.
func (a *Adapter) Downlink(ctx context.Context, msg *access.DownlinkMessage) error {
	if a.server == nil {
		return fmt.Errorf("mqtt server not started")
	}
	dt := DeviceTopic{
		ProductKey: msg.Device.ProductKey,
		DeviceKey:  msg.Device.DeviceKey,
	}
	var suffix string
	switch msg.Kind {
	case access.KindProperty:
		suffix = "properties/set"
	case access.KindServiceCall:
		suffix = fmt.Sprintf("services/%s/call", msg.Identifier)
	case access.KindPeer:
		from := msg.Metadata["from"]
		if from == "" {
			from = "platform"
		}
		suffix = fmt.Sprintf("peer/%s", from)
	case access.KindOTA:
		suffix = "ota/upgrade"
	default:
		suffix = "commands"
	}
	topic := joinTopic(dt, suffix)
	return a.server.Publish(topic, msg.Payload, false, 1)
}

// detectUplink maps a topic suffix to a normalized uplink message.
func (a *Adapter) detectUplink(auth *access.AuthResponse, info *topicInfo, payload []byte) *access.UplinkMessage {
	dev := access.DeviceRef{
		WorkspaceKey: auth.WorkspaceKey,
		ProductKey:   auth.ProductKey,
		DeviceKey:    auth.DeviceKey,
	}
	msg := &access.UplinkMessage{
		Protocol:  access.ProtocolMQTTName,
		Device:    dev,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"topic": joinTopic(DeviceTopic{auth.ProductKey, auth.DeviceKey}, strings.Join(info.Suffix, "/"))},
	}
	s := info.Suffix
	switch {
	case len(s) == 2 && s[0] == "properties" && s[1] == "post":
		msg.Kind = access.KindProperty
	case len(s) == 2 && s[0] == "properties":
		msg.Kind = access.KindProperty
		msg.Identifier = s[1]
	case len(s) == 3 && s[0] == "events" && s[2] == "post":
		msg.Kind = access.KindEvent
		msg.Identifier = s[1]
	case len(s) == 3 && s[0] == "services" && s[2] == "reply":
		msg.Kind = access.KindServiceReply
		msg.Identifier = s[1]
	case len(s) == 2 && s[0] == "peer" && s[1] == "broadcast":
		msg.Kind = access.KindPeer
		msg.Metadata["broadcast"] = "true"
	case len(s) == 2 && s[0] == "peer":
		msg.Kind = access.KindPeer
		msg.Metadata["to"] = s[1]
	case len(s) == 1 && s[0] == "lifecycle":
		msg.Kind = access.KindLifecycle
	case len(s) == 2 && s[0] == "ota":
		msg.Kind = access.KindOTA
		msg.Identifier = s[1]
	default:
		return nil
	}
	return msg
}

func (a *Adapter) forwardUplink(msg *access.UplinkMessage) {
	if a.uplink == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.uplink(ctx, msg); err != nil {
		a.log.Error("mqtt uplink handling failed", "error", err, "device", msg.Device.DeviceKey, "kind", msg.Kind)
	}
}

func waitForTCP(addr string, timeout time.Duration) error {
	if addr == "" {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

// parseCredentials extracts productKey/deviceKey from the MQTT username.
// Convention: username = "{productKey}/{deviceKey}", password = device secret.
func parseCredentials(username, clientID string) (productKey, deviceKey string) {
	if strings.Contains(username, "/") {
		parts := strings.SplitN(username, "/", 2)
		return parts[0], parts[1]
	}
	if strings.Contains(clientID, "/") {
		parts := strings.SplitN(clientID, "/", 2)
		return parts[0], parts[1]
	}
	return username, clientID
}
