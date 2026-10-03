// Package opcua implements an OPC-UA client adapter: it connects to OPC-UA
// servers (industrial PLCs/PLCs) and polls mapped node values as thing-model
// property uplinks. Downlinks write node values.
package opcua

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// Options configures the OPC-UA adapter.
type Options struct {
	ConfigFile string
	Logger     *slog.Logger
}

// Adapter implements access.Adapter. Like the Modbus adapter it is
// master-driven: it dials each configured server, keeps a session, and polls
// the mapped nodes on an interval.
type Adapter struct {
	opts Options
	log  *slog.Logger

	uplink access.UplinkHandler
	auth   access.AuthHandler

	wg sync.WaitGroup
}

// New validates the config upfront.
func New(opts Options) (*Adapter, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if _, err := LoadConfig(opts.ConfigFile); err != nil {
		return nil, err
	}
	return &Adapter{opts: opts, log: opts.Logger}, nil
}

func (a *Adapter) Name() string                    { return "opcua" }
func (a *Adapter) OnUplink(h access.UplinkHandler) { a.uplink = h }
func (a *Adapter) OnAuth(h access.AuthHandler)     { a.auth = h }

// Start launches one poller goroutine per device.
func (a *Adapter) Start(ctx context.Context) error {
	cfg, err := LoadConfig(a.opts.ConfigFile)
	if err != nil {
		return err
	}
	for i := range cfg.Devices {
		d := cfg.Devices[i]
		every, _ := time.ParseDuration(d.PollEvery)
		a.wg.Add(1)
		go a.pollLoop(ctx, d, every)
	}
	a.log.Info("opcua adapter started", "devices", len(cfg.Devices))
	return nil
}

func (a *Adapter) Stop(ctx context.Context) error {
	a.wg.Wait()
	return nil
}

func (a *Adapter) pollLoop(ctx context.Context, d DeviceCfg, every time.Duration) {
	defer a.wg.Done()
	if every <= 0 {
		every = 5 * time.Second
	}
	for {
		err := a.pollOnce(ctx, d)
		if err != nil {
			a.log.Warn("opcua poll failed", "device", d.DeviceKey, "endpoint", d.Endpoint, "error", err)
			a.reportLifecycle(d.DeviceKey, access.StateOffline)
		} else {
			a.reportLifecycle(d.DeviceKey, access.StateOnline)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

func (a *Adapter) pollOnce(ctx context.Context, d DeviceCfg) error {
	if a.auth != nil {
		ar, err := a.auth(ctx, &access.AuthRequest{
			ProductKey: "", DeviceKey: d.DeviceKey, Password: d.Secret,
			Protocol: "opcua", RemoteAddr: d.Endpoint,
		})
		if err != nil || ar == nil || !ar.Authorized {
			return fmt.Errorf("device %s not authorized", d.DeviceKey)
		}
	}
	c, err := connect(ctx, d)
	if err != nil {
		return err
	}
	defer c.Close(context.Background())

	var reported int
	for _, node := range d.Nodes {
		val, err := readValue(c, node)
		if err != nil {
			a.log.Warn("opcua read failed", "device", d.DeviceKey, "node", node.Identifier, "error", err)
			continue
		}
		f, ok := coerce(node, val)
		if !ok {
			continue
		}
		if err := a.reportProperty(ctx, d, node, f); err != nil {
			a.log.Warn("opcua uplink failed", "device", d.DeviceKey, "identifier", node.Identifier, "error", err)
			continue
		}
		reported++
	}
	a.log.Debug("opcua poll ok", "device", d.DeviceKey, "nodes", reported)
	return nil
}

// connect builds and connects an OPC-UA client (security None + optional UA
// username/password authentication).
func connect(ctx context.Context, d DeviceCfg) (*opcua.Client, error) {
	opts := []opcua.Option{opcua.SecurityMode(ua.MessageSecurityModeNone)}
	if d.Username != "" {
		opts = append(opts, opcua.AuthUsername(d.Username, d.Password))
	}
	c, err := opcua.NewClient(d.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("opcua client: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := c.Connect(cctx); err != nil {
		return nil, fmt.Errorf("opcua connect: %w", err)
	}
	return c, nil
}

// readValue reads one node and returns its raw Go value.
func readValue(c *opcua.Client, node NodeCfg) (any, error) {
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: ua.MustParseNodeID(node.NodeID), AttributeID: ua.AttributeIDValue},
		},
	}
	resp, err := c.Read(context.Background(), req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0] == nil {
		return nil, fmt.Errorf("no result")
	}
	dv := resp.Results[0]
	if dv.Status != ua.StatusOK {
		return nil, fmt.Errorf("status %v", dv.Status)
	}
	if dv.Value == nil || dv.Value.Value() == nil {
		return nil, fmt.Errorf("empty value")
	}
	return dv.Value.Value(), nil
}

// coerce converts a UA value to a scaled float, applying the configured type.
func coerce(node NodeCfg, v any) (float64, bool) {
	switch node.DataType {
	case "bool":
		if b, ok := v.(bool); ok {
			f := 0.0
			if b {
				f = 1
			}
			return f, true
		}
	case "string":
		if s, ok := v.(string); ok {
			var f float64
			if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
				return f * node.scale(), true
			}
		}
	default: // double / any / int
		f := toFloat(v)
		if math.IsNaN(f) {
			return 0, false
		}
		return f * node.scale(), true
	}
	return 0, false
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n * 1
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case int:
		return float64(n)
	case uint32:
		return float64(n)
	case bool:
		if n {
			return 1
		}
		return 0
	}
	return math.NaN()
}

func (a *Adapter) reportProperty(ctx context.Context, d DeviceCfg, node NodeCfg, val float64) error {
	if a.uplink == nil {
		return nil
	}
	if math.IsNaN(val) {
		return nil
	}
	val = math.Round(val*1e6) / 1e6
	payload, _ := json.Marshal(map[string]float64{node.Identifier: val})
	return a.uplink(ctx, &access.UplinkMessage{
		Protocol:   a.Name(),
		Device:     access.DeviceRef{ProductKey: "", DeviceKey: d.DeviceKey},
		Kind:       access.KindProperty,
		Identifier: node.Identifier,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
		Metadata:   map[string]string{},
	})
}

func (a *Adapter) reportLifecycle(deviceKey, state string) {
	if a.uplink == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"state": state})
	_ = a.uplink(context.Background(), &access.UplinkMessage{
		Protocol:  a.Name(),
		Device:    access.DeviceRef{DeviceKey: deviceKey},
		Kind:      access.KindLifecycle,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"state": state},
	})
}

// Downlink writes a node value for a service/command targeting a mapped node.
// Payload: {"value": number}.
func (a *Adapter) Downlink(ctx context.Context, msg *access.DownlinkMessage) error {
	if msg == nil {
		return fmt.Errorf("nil downlink")
	}
	cfg, err := LoadConfig(a.opts.ConfigFile)
	if err != nil {
		return err
	}
	for i := range cfg.Devices {
		d := &cfg.Devices[i]
		if d.DeviceKey != msg.Device.DeviceKey {
			continue
		}
		for j := range d.Nodes {
			if d.Nodes[j].Identifier != msg.Identifier {
				continue
			}
			var body struct {
				Value *float64 `json:"value"`
			}
			if err := json.Unmarshal(msg.Payload, &body); err != nil || body.Value == nil {
				return fmt.Errorf("payload must include \"value\"")
			}
			c, err := connect(ctx, *d)
			if err != nil {
				return err
			}
			defer c.Close(context.Background())
			nid := ua.MustParseNodeID(d.Nodes[j].NodeID)
			_, err = c.Write(context.Background(), &ua.WriteRequest{
				NodesToWrite: []*ua.WriteValue{{
					NodeID:      nid,
					AttributeID: ua.AttributeIDValue,
					Value:       &ua.DataValue{Value: ua.MustVariant(*body.Value)},
				}},
			})
			a.log.Info("opcua node written", "device", d.DeviceKey, "node", msg.Identifier, "value", *body.Value, "error", err)
			return err
		}
	}
	return fmt.Errorf("node %q not mapped for device %s", msg.Identifier, msg.Device.DeviceKey)
}
