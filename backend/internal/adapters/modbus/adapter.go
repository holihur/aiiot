package modbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
)

// Options configures the Modbus adapter.
type Options struct {
	// ConfigFile is the JSON device table (see Config).
	ConfigFile string
	// Logger, when nil, uses slog.Default().
	Logger *slog.Logger
}

// Adapter implements access.Adapter for Modbus TCP. Unlike the passive
// gateways it is master-driven: it dials each configured PLC and polls the
// mapped registers on an interval, reporting each value as a property uplink.
type Adapter struct {
	opts Options
	log  *slog.Logger

	uplink access.UplinkHandler
	auth   access.AuthHandler

	mu    sync.Mutex
	alive map[string]bool // deviceKey -> last poll success

	wg sync.WaitGroup
}

// New creates the adapter from a config file.
func New(opts Options) (*Adapter, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if _, err := LoadConfig(opts.ConfigFile); err != nil {
		return nil, err
	}
	return &Adapter{opts: opts, log: opts.Logger, alive: map[string]bool{}}, nil
}

func (a *Adapter) Name() string                    { return "modbus" }
func (a *Adapter) OnUplink(h access.UplinkHandler) { a.uplink = h }
func (a *Adapter) OnAuth(h access.AuthHandler)     { a.auth = h }

// Start validates the config and launches one poller goroutine per device.
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
	a.log.Info("modbus adapter started", "devices", len(cfg.Devices))
	return nil
}

func (a *Adapter) Stop(ctx context.Context) error {
	a.wg.Wait()
	return nil
}

// pollLoop dials (with reconnect backoff) and polls every register group.
func (a *Adapter) pollLoop(ctx context.Context, d DeviceCfg, every time.Duration) {
	defer a.wg.Done()
	if every <= 0 {
		every = 5 * time.Second
	}
	for {
		err := a.pollOnce(ctx, d)
		a.setAlive(d.DeviceKey, err == nil)
		if err != nil {
			a.log.Warn("modbus poll failed", "device", d.DeviceKey, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// pollOnce dials the PLC, reads all mapped registers and reports them. A
// single connection is reused for the whole pass; on failure the connection
// is closed so the next pass redials (with backoff via pollLoop pause).
func (a *Adapter) pollOnce(ctx context.Context, d DeviceCfg) error {
	c, err := Dial(d.Host, d.Port, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s:%d: %w", d.Host, d.Port, err)
	}
	defer c.Close()

	if a.auth != nil {
		ar, err := a.auth(ctx, &access.AuthRequest{
			ProductKey: "", DeviceKey: d.DeviceKey, Password: d.Secret,
			Protocol: "modbus", RemoteAddr: fmt.Sprintf("%s:%d", d.Host, d.Port),
		})
		if err != nil || ar == nil || !ar.Authorized {
			return fmt.Errorf("device %s not authorized", d.DeviceKey)
		}
	}

	// one read per register (simple, predictable error isolation)
	var reported int
	for _, reg := range d.Registers {
		fn, _ := reg.fn()
		words, err := c.ReadRegisters(d.UnitID, fn, reg.Address, uint16(reg.count()))
		if err != nil {
			return fmt.Errorf("read %s: %w", reg.Identifier, err)
		}
		val, err := reg.decode(words)
		if err != nil {
			return fmt.Errorf("decode %s: %w", reg.Identifier, err)
		}
		if err := a.reportProperty(ctx, d, reg, val); err != nil {
			a.log.Warn("modbus uplink failed", "device", d.DeviceKey, "identifier", reg.Identifier, "error", err)
			continue
		}
		reported++
	}
	a.log.Debug("modbus poll ok", "device", d.DeviceKey, "registers", reported)
	return nil
}

func (a *Adapter) reportProperty(ctx context.Context, d DeviceCfg, reg RegCfg, val float64) error {
	if a.uplink == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]float64{reg.Identifier: val})
	return a.uplink(ctx, &access.UplinkMessage{
		Protocol:   a.Name(),
		Device:     access.DeviceRef{ProductKey: "", DeviceKey: d.DeviceKey},
		Kind:       access.KindProperty,
		Identifier: reg.Identifier,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
		Metadata:   map[string]string{},
	})
}

// Downlink writes a holding register for a service/command targeting a mapped
// register. Payload may be {"value": number} or {"address": n, "value": v}.
func (a *Adapter) Downlink(ctx context.Context, msg *access.DownlinkMessage) error {
	if msg == nil {
		return fmt.Errorf("nil downlink")
	}
	cfg, err := LoadConfig(a.opts.ConfigFile)
	if err != nil {
		return err
	}
	d := findDevice(cfg, msg.Device.DeviceKey)
	if d == nil {
		return fmt.Errorf("modbus device %q not configured", msg.Device.DeviceKey)
	}
	reg := findReg(d, msg.Identifier)
	if reg == nil {
		return fmt.Errorf("register %q not mapped for device %s", msg.Identifier, d.DeviceKey)
	}

	var body struct {
		Value   *float64 `json:"value"`
		Address *uint16  `json:"address"`
	}
	if err := json.Unmarshal(msg.Payload, &body); err != nil {
		return fmt.Errorf("modbus downlink payload: %w", err)
	}
	if body.Value == nil {
		return fmt.Errorf("modbus downlink payload must include \"value\"")
	}
	addr := reg.Address
	if body.Address != nil {
		addr = *body.Address
	}
	raw := encodeForType(reg.Type, *body.Value)
	if reg.Type == "bool" {
		raw = 0
		if *body.Value != 0 {
			raw = 1
		}
	}

	c, err := Dial(d.Host, d.Port, 5*time.Second)
	if err != nil {
		return fmt.Errorf("modbus dial: %w", err)
	}
	defer c.Close()
	if err := c.WriteSingle(d.UnitID, addr, raw); err != nil {
		return fmt.Errorf("modbus write: %w", err)
	}
	a.log.Info("modbus register written", "device", d.DeviceKey, "register", msg.Identifier, "address", addr, "value", raw)
	return nil
}

func encodeForType(t string, v float64) uint16 {
	switch t {
	case "int16":
		return uint16(int16(v))
	case "uint32":
		return uint16(uint32(v) >> 16)
	case "int32":
		return uint16(int32(v) >> 16)
	case "float32":
		return uint16(math.Float32bits(float32(v)) >> 16)
	default:
		return uint16(v)
	}
}

func (a *Adapter) setAlive(deviceKey string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.alive[deviceKey] == ok {
		return
	}
	a.alive[deviceKey] = ok
	state := access.StateOnline
	if !ok {
		state = access.StateOffline
	}
	if a.uplink == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"state": state})
	_ = a.uplink(context.Background(), &access.UplinkMessage{
		Protocol:  a.Name(),
		Device:    access.DeviceRef{ProductKey: "", DeviceKey: deviceKey},
		Kind:      access.KindLifecycle,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"state": state},
	})
}

func findDevice(cfg *Config, key string) *DeviceCfg {
	for i := range cfg.Devices {
		if cfg.Devices[i].DeviceKey == key {
			return &cfg.Devices[i]
		}
	}
	return nil
}

func findReg(d *DeviceCfg, identifier string) *RegCfg {
	for i := range d.Registers {
		if d.Registers[i].Identifier == identifier {
			return &d.Registers[i]
		}
	}
	return nil
}
