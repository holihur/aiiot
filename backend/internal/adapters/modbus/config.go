package modbus

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

// Config is the JSON device table consumed by the gateway:
//
//	{
//	  "devices": [
//	    {
//	      "deviceKey": "plc-line1",
//	      "host": "10.0.0.12",
//	      "port": 502,
//	      "unitId": 1,
//	      "pollEvery": "5s",
//	      "registers": [
//	        {"identifier":"temperature","fn":"holding","address":0,"type":"int16","scale":0.1},
//	        {"identifier":"pressure","fn":"input","address":2,"type":"float32"},
//	        {"identifier":"running","fn":"holding","address":10,"type":"bool"}
//	      ]
//	    }
//	  ]
//	}
//
// Values are reported as thing-model property reports; a register identifier
// must exist in the product thing model for rules/telemetry to consume it.
type Config struct {
	Devices []DeviceCfg `json:"devices"`
}

type DeviceCfg struct {
	DeviceKey string `json:"deviceKey"`
	// Secret is the device secret shown in the core UI; required when the
	// device has one set (mirrors MQTT password auth).
	Secret string `json:"secret,omitempty"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	UnitID byte   `json:"unitId"`
	// PollEvery is a duration string such as "5s" or "30s".
	PollEvery string   `json:"pollEvery"`
	Registers []RegCfg `json:"registers"`
}

type RegCfg struct {
	// Identifier is the thing-model property identifier this register maps to.
	Identifier string `json:"identifier"`
	// Function is "holding" (0x03) or "input" (0x04).
	Function string `json:"fn"`
	Address  uint16 `json:"address"`
	// Type is one of: int16, uint16, int32, uint32, float32, bool.
	Type string `json:"type"`
	// Scale multiplies the decoded value before reporting (default 1).
	Scale float64 `json:"scale"`
}

func (r RegCfg) fn() (byte, error) {
	switch r.Function {
	case "holding", "":
		return fnReadHolding, nil
	case "input":
		return fnReadInput, nil
	}
	return 0, fmt.Errorf("unknown register function %q", r.Function)
}

func (r RegCfg) count() int {
	switch r.Type {
	case "int32", "uint32", "float32":
		return 2
	default:
		return 1
	}
}

func (r RegCfg) scale() float64 {
	if r.Scale == 0 {
		return 1
	}
	return r.Scale
}

// LoadConfig reads and validates the device table file.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("modbus config file not set (MODBUS_CONFIG_FILE)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read modbus config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse modbus config: %w", err)
	}
	if len(cfg.Devices) == 0 {
		return nil, errors.New("modbus config has no devices")
	}
	for i := range cfg.Devices {
		d := &cfg.Devices[i]
		if d.DeviceKey == "" || d.Host == "" || d.Port == 0 {
			return nil, fmt.Errorf("device %d: deviceKey/host/port required", i)
		}
		if d.PollEvery == "" {
			d.PollEvery = "5s"
		}
		if _, err := time.ParseDuration(d.PollEvery); err != nil {
			return nil, fmt.Errorf("device %s: pollEvery: %w", d.DeviceKey, err)
		}
		if len(d.Registers) == 0 {
			return nil, fmt.Errorf("device %s: no registers", d.DeviceKey)
		}
		for j := range d.Registers {
			r := &d.Registers[j]
			if r.Identifier == "" {
				return nil, fmt.Errorf("device %s: register %d missing identifier", d.DeviceKey, j)
			}
			if _, err := r.fn(); err != nil {
				return nil, fmt.Errorf("device %s: register %s: %w", d.DeviceKey, r.Identifier, err)
			}
			switch r.Type {
			case "int16", "uint16", "int32", "uint32", "float32", "bool":
			default:
				return nil, fmt.Errorf("device %s: register %s: unsupported type %q", d.DeviceKey, r.Identifier, r.Type)
			}
		}
	}
	return &cfg, nil
}

// decode converts register words into the configured value scaled for the
// thing-model property (big-endian word order per Modbus convention).
func (r RegCfg) decode(regs []uint16) (float64, error) {
	if len(regs) < r.count() {
		return 0, fmt.Errorf("register %s: need %d words, got %d", r.Identifier, r.count(), len(regs))
	}
	w := func(i int) uint32 { return uint32(regs[i]) }
	var raw float64
	switch r.Type {
	case "bool":
		raw = float64(regs[0] & 0x01)
	case "uint16":
		raw = float64(regs[0])
	case "int16":
		raw = float64(int16(regs[0]))
	case "uint32":
		raw = float64(w(0)<<16 | w(1))
	case "int32":
		raw = float64(int32(w(0)<<16 | w(1)))
	case "float32":
		bits := w(0)<<16 | w(1)
		raw = float64(math.Float32frombits(bits))
	default:
		return 0, fmt.Errorf("register %s: unsupported type %q", r.Identifier, r.Type)
	}
	return math.Round(raw*r.scale()*1e6) / 1e6, nil
}
